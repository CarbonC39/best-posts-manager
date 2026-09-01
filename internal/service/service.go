package service

import (
	"context"
	"errors"
	"time"

	"best-post-manager/internal/app"
	"best-post-manager/internal/platform"
)

type Service struct {
	Store    *app.Store
	Mastodon *platform.Client
}

func New(s *app.Store) *Service { return &Service{Store: s, Mastodon: platform.NewClient()} }

func (s *Service) Publish(ctx context.Context, variantID string) (app.Publication, error) {
	v, e := s.Store.GetVariant(ctx, variantID)
	if e != nil {
		return app.Publication{}, e
	}
	account, e := s.Store.GetAccount(ctx, v.AccountID)
	if e != nil {
		return app.Publication{}, e
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
		if m.VariantID == "" || m.VariantID == v.ID {
			selected = append(selected, m)
		}
	}
	if account.Platform != "mastodon" && account.Platform != "mastodon-compatible" {
		p.Status = "failed"
		p.ErrorCategory = "config"
		p.ErrorMessage = "节点1仅支持 Mastodon-compatible 账号"
		_ = s.Store.UpdatePublication(ctx, p)
		return p, &platform.Error{Category: "config", Err: errUnsupported{account.Platform}}
	}
	result, e := s.Mastodon.Publish(ctx, account, v, selected, p.IdempotencyKey)
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
	p.RemoteID, p.RemoteURL = result.ID, result.URL
	p.PublishedAt = time.Now().UTC()
	if e = s.Store.UpdatePublication(ctx, p); e != nil {
		return p, e
	}
	return p, nil
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
		if m.VariantID == "" || m.VariantID == v.ID {
			selected = append(selected, m)
		}
	}
	result, err := s.Mastodon.Publish(ctx, account, v, selected, p.IdempotencyKey)
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
	p.RemoteID, p.RemoteURL = result.ID, result.URL
	p.PublishedAt = time.Now().UTC()
	err = s.Store.UpdatePublication(ctx, p)
	return p, err
}

type errUnsupported struct{ p string }

func (e errUnsupported) Error() string { return "unsupported platform: " + e.p }
