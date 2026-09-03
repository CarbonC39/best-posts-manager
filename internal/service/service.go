package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"best-post-manager/internal/app"
	"best-post-manager/internal/credential"
	"best-post-manager/internal/platform"
	"best-post-manager/internal/translation"
)

type Service struct {
	Store       *app.Store
	Mastodon    *platform.Client
	Misskey     *platform.MisskeyClient
	Bluesky     *platform.BlueskyClient
	Credentials credential.Store
}

// ConfirmPublication records a manually verified remote result after an
// earlier publish attempt had an unknown outcome.
func (s *Service) ConfirmPublication(ctx context.Context, publicationID, remoteURL, remoteID string) (app.Publication, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	parsed, err := url.ParseRequestURI(remoteURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return app.Publication{}, fmt.Errorf("remote_url must be an absolute http or https URL")
	}
	p, err := s.Store.GetPublication(ctx, publicationID)
	if err != nil {
		return app.Publication{}, err
	}
	if p.Status != "needs_confirmation" {
		return p, fmt.Errorf("%w: only publications needing confirmation can be confirmed", app.ErrConflict)
	}
	return s.Store.ConfirmPublication(ctx, publicationID, remoteURL, strings.TrimSpace(remoteID), time.Now().UTC())
}

func (s *Service) CancelPublication(ctx context.Context, publicationID string) (app.Publication, error) {
	p, err := s.Store.GetPublication(ctx, publicationID)
	if err != nil {
		return app.Publication{}, err
	}
	if p.Status != "needs_confirmation" {
		return p, fmt.Errorf("%w: only publications needing confirmation can be cancelled", app.ErrConflict)
	}
	return s.Store.CancelPublication(ctx, publicationID)
}

func New(s *app.Store) *Service {
	var secrets credential.Store
	if local, err := credential.NewFileStore(s.DataDir); err == nil {
		secrets = local
	}
	mastodon := platform.NewClient()
	mastodon.Credentials = secrets
	misskey := platform.NewMisskeyClient()
	misskey.Credentials = secrets
	bluesky := platform.NewBlueskyClient()
	bluesky.Credentials = secrets
	return &Service{Store: s, Mastodon: mastodon, Misskey: misskey, Bluesky: bluesky, Credentials: secrets}
}

type TranslationRequest struct {
	SourceVariantID string
	TargetAccountID string
	SourceLanguage  string
	TargetLanguage  string
	ConfigID        string
}

// Translate never edits the source. The provider call happens before the
// transaction that creates the detached target Variant and its audit record.
func (s *Service) Translate(ctx context.Context, req TranslationRequest) (app.Variant, app.Translation, error) {
	source, err := s.Store.GetVariant(ctx, req.SourceVariantID)
	if err != nil {
		return app.Variant{}, app.Translation{}, err
	}
	target, err := s.Store.GetAccount(ctx, req.TargetAccountID)
	if err != nil {
		return app.Variant{}, app.Translation{}, err
	}
	if target.Disabled {
		return app.Variant{}, app.Translation{}, errors.New("translation target account is deactivated")
	}
	sourceAccount, err := s.Store.GetAccount(ctx, source.AccountID)
	if err != nil {
		return app.Variant{}, app.Translation{}, err
	}
	if sourceAccount.ProfileID != target.ProfileID {
		return app.Variant{}, app.Translation{}, errors.New("translation target account must belong to the source profile")
	}
	config, err := s.Store.GetTranslationConfig(ctx, req.ConfigID)
	if err != nil {
		return app.Variant{}, app.Translation{}, err
	}
	provider, err := translation.NewProvider(config)
	if err != nil {
		return app.Variant{}, app.Translation{}, err
	}
	if httpProvider, ok := provider.(*translation.HTTPProvider); ok {
		httpProvider.Credentials = s.Credentials
	}
	sourceLanguage := req.SourceLanguage
	if sourceLanguage == "" {
		if draft, draftErr := s.Store.GetBaseDraft(ctx, source.TopicID); draftErr == nil {
			sourceLanguage = draft.Language
		}
	}
	translated, err := provider.Translate(ctx, translation.Request{Text: source.Text, SourceLanguage: sourceLanguage, TargetLanguage: req.TargetLanguage})
	if err != nil {
		return app.Variant{}, app.Translation{}, err
	}
	return s.Store.CreateTranslatedVariant(ctx, source, target, translated, config.Provider, sourceLanguage, req.TargetLanguage)
}

func (s *Service) Publish(ctx context.Context, variantID string) (app.Publication, error) {
	v, e := s.Store.GetVariant(ctx, variantID)
	if e != nil {
		return app.Publication{}, e
	}
	if unresolved, unresolvedErr := s.Store.HasUnresolvedPublication(ctx, variantID); unresolvedErr != nil {
		return app.Publication{}, unresolvedErr
	} else if unresolved {
		return app.Publication{}, fmt.Errorf("%w: this version already has an unresolved publication; confirm it before publishing again", app.ErrConflict)
	}
	account, e := s.Store.GetAccount(ctx, v.AccountID)
	if e != nil {
		return app.Publication{}, e
	}
	if account.Disabled {
		return app.Publication{}, errors.New("account is deactivated; restore or add an active account before publishing")
	}
	p, e := s.Store.CreatePublication(ctx, app.Publication{VariantID: v.ID, AccountID: account.ID, Status: "publishing", Source: "internal"})
	if e != nil {
		return p, e
	}
	media, e := s.Store.ListMedia(ctx, v.TopicID)
	if e != nil {
		p.Status = "failed"
		p.ErrorCategory = "storage"
		p.ErrorMessage = e.Error()
		_ = s.Store.UpdatePublication(ctx, p)
		return p, e
	}
	var selected []app.Media
	for _, m := range media {
		if (m.VariantID == "" || m.VariantID == v.ID) && !m.CreatedAt.After(p.CreatedAt) {
			selected = append(selected, m)
		}
	}
	if account.Platform != "mastodon" && account.Platform != "mastodon-compatible" && account.Platform != "misskey" && account.Platform != "bluesky" {
		p.Status = "failed"
		p.ErrorCategory = "config"
		p.ErrorMessage = "unsupported platform"
		_ = s.Store.UpdatePublication(ctx, p)
		return p, &platform.Error{Category: "config", Err: errUnsupported{account.Platform}}
	}
	var result platform.Result
	switch account.Platform {
	case "mastodon", "mastodon-compatible":
		result, e = s.Mastodon.Publish(ctx, account, v, selected, p.IdempotencyKey)
	case "misskey":
		result, e = s.Misskey.Publish(ctx, account, v, selected)
	case "bluesky":
		result, e = s.Bluesky.Publish(ctx, account, v, selected)
	default:
		e = &platform.Error{Category: "config", Stage: "validate", Err: errors.New("unsupported platform")}
	}
	if e != nil {
		p.Status = "failed"
		if pe, ok := e.(*platform.Error); ok {
			p.ErrorCategory = pe.Category
			p.ErrorMessage = pe.Err.Error()
			if pe.OutcomeUnknown {
				p.Status = "needs_confirmation"
			} else if pe.Retryable || (pe.Category == "network" && pe.Stage == "media") {
				p.ErrorCategory = "retryable"
			}
		} else {
			p.ErrorCategory = "unknown"
			p.ErrorMessage = e.Error()
		}
		_ = s.Store.UpdatePublication(ctx, p)
		return p, e
	}
	p.Status = "published"
	p.RemoteID, p.RemoteURL, p.RemoteCID = result.ID, result.URL, result.CID
	p.PublishedAt = time.Now().UTC()
	if e = s.Store.UpdatePublication(ctx, p); e != nil {
		return p, e
	}
	return p, nil
}

// DeactivateAccount removes its local secret and keeps the account row for history.
func (s *Service) DeactivateAccount(ctx context.Context, accountID string) (app.Account, error) {
	a, err := s.Store.GetAccount(ctx, accountID)
	if err != nil {
		return app.Account{}, err
	}
	if a.CredentialRef != "" && s.Credentials != nil {
		if err = s.Credentials.Delete(a.CredentialRef); err != nil {
			return app.Account{}, errors.New("could not remove local credential; account remains active")
		}
	}
	return s.Store.DeactivateAccount(ctx, accountID)
}

func (s *Service) ImportExternal(ctx context.Context, topicID, accountID, reference string) (app.ExternalPost, error) {
	a, err := s.Store.GetAccount(ctx, accountID)
	if err != nil {
		return app.ExternalPost{}, err
	}
	if a.Disabled {
		return app.ExternalPost{}, errors.New("account is deactivated; add an active account before importing")
	}
	topic, err := s.Store.GetTopic(ctx, topicID)
	if err != nil {
		return app.ExternalPost{}, err
	}
	if topic.ProfileID != a.ProfileID {
		return app.ExternalPost{}, errors.New("external post account must belong to the topic profile")
	}
	var post app.ExternalPost
	switch a.Platform {
	case "misskey":
		post, err = s.Misskey.Import(ctx, a, reference)
	case "bluesky":
		post, err = s.Bluesky.Import(ctx, a, reference)
	default:
		return post, &platform.Error{Category: "config", Stage: "import", Err: errors.New("official single-post import is supported for Misskey and Bluesky")}
	}
	if err != nil {
		return post, err
	}
	post.TopicID = topicID
	post.AccountID = accountID
	return s.Store.CreateExternalPost(ctx, post)
}

// Retry reuses the original publication and idempotency key. Unknown outcomes are never retried blindly.
func (s *Service) Retry(ctx context.Context, publicationID string) (app.Publication, error) {
	p, err := s.Store.GetPublication(ctx, publicationID)
	if err != nil {
		return p, err
	}
	if p.Status != "failed" || p.ErrorCategory != "retryable" {
		return p, errors.New("only failed retryable publications can be retried")
	}
	v, err := s.Store.GetVariant(ctx, p.VariantID)
	if err != nil {
		return p, err
	}
	account, err := s.Store.GetAccount(ctx, p.AccountID)
	if err != nil {
		return p, err
	}
	if account.Disabled {
		return p, errors.New("account is deactivated; add an active account before retrying")
	}
	p.Status = "publishing"
	p.ErrorCategory = ""
	p.ErrorMessage = ""
	if err = s.Store.UpdatePublication(ctx, p); err != nil {
		return p, err
	}
	media, err := s.Store.ListMedia(ctx, v.TopicID)
	if err != nil {
		p.Status = "failed"
		p.ErrorCategory = "storage"
		p.ErrorMessage = err.Error()
		_ = s.Store.UpdatePublication(ctx, p)
		return p, err
	}
	var selected []app.Media
	for _, m := range media {
		if (m.VariantID == "" || m.VariantID == v.ID) && !m.CreatedAt.After(p.CreatedAt) {
			selected = append(selected, m)
		}
	}
	var result platform.Result
	switch account.Platform {
	case "mastodon", "mastodon-compatible":
		result, err = s.Mastodon.Publish(ctx, account, v, selected, p.IdempotencyKey)
	case "misskey":
		result, err = s.Misskey.Publish(ctx, account, v, selected)
	case "bluesky":
		result, err = s.Bluesky.Publish(ctx, account, v, selected)
	default:
		err = &platform.Error{Category: "config", Stage: "validate", Err: errors.New("unsupported platform")}
	}
	if err != nil {
		p.Status = "failed"
		if pe, ok := err.(*platform.Error); ok {
			p.ErrorCategory = pe.Category
			p.ErrorMessage = pe.Err.Error()
			if pe.OutcomeUnknown {
				p.Status = "needs_confirmation"
			} else if pe.Retryable || (pe.Category == "network" && pe.Stage == "media") {
				p.ErrorCategory = "retryable"
			}
		} else {
			p.ErrorCategory = "unknown"
			p.ErrorMessage = err.Error()
		}
		_ = s.Store.UpdatePublication(ctx, p)
		return p, err
	}
	p.Status = "published"
	p.RemoteID, p.RemoteURL, p.RemoteCID = result.ID, result.URL, result.CID
	p.PublishedAt = time.Now().UTC()
	err = s.Store.UpdatePublication(ctx, p)
	return p, err
}

type errUnsupported struct{ p string }

func (e errUnsupported) Error() string { return "unsupported platform: " + e.p }
