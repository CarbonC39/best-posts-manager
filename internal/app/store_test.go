package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTopicMetadataExternalPostDeletionAndPublicationConfirmation(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "before"})

	updated, err := store.UpdateTopic(ctx, Topic{ID: topic.ID, Title: "  after  ", Status: "archived"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "after" || updated.Status != "archived" || updated.ProfileID != profile.ID || !updated.UpdatedAt.After(topic.UpdatedAt) {
		t.Fatalf("unexpected topic update: %+v", updated)
	}
	if _, err = store.UpdateTopic(ctx, Topic{ID: topic.ID, Title: "", Status: "active"}); err == nil {
		t.Fatal("empty topic title must be rejected")
	}
	if _, err = store.UpdateTopic(ctx, Topic{ID: topic.ID, Title: "valid", Status: "deleted"}); err == nil {
		t.Fatal("invalid topic status must be rejected")
	}

	external, err := store.CreateExternalPost(ctx, ExternalPost{TopicID: topic.ID, Platform: "x", URL: "https://example.test/post/1"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteExternalPost(ctx, external.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteExternalPost(ctx, external.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second deletion should be not found, got %v", err)
	}

	variant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	publication, _ := store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "needs_confirmation", ErrorCategory: "unknown", ErrorMessage: "check remotely"})
	if _, err = store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "publishing"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a second unresolved publication should conflict, got %v", err)
	}
	confirmed, err := store.ConfirmPublication(ctx, publication.ID, "https://example.test/post/2", "remote-2", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "published" || confirmed.RemoteURL == "" || confirmed.RemoteID != "remote-2" || confirmed.ErrorCategory != "" || confirmed.ErrorMessage != "" || confirmed.PublishedAt.IsZero() {
		t.Fatalf("publication confirmation incomplete: %+v", confirmed)
	}
	if _, err = store.ConfirmPublication(ctx, publication.ID, "https://example.test/post/2", "", time.Now().UTC()); !errors.Is(err, ErrConflict) {
		t.Fatalf("repeated confirmation should conflict, got %v", err)
	}
	toCancel, err := store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "needs_confirmation", RemoteID: "uncertain", RemoteURL: "https://example.test/uncertain", RemoteCID: "cid", ErrorCategory: "unknown", ErrorMessage: "uncertain"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.CancelPublication(ctx, toCancel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" || cancelled.ErrorCategory != "" || cancelled.ErrorMessage != "用户确认未发布" || cancelled.RemoteID != "" || cancelled.RemoteURL != "" || cancelled.RemoteCID != "" || !cancelled.PublishedAt.IsZero() {
		t.Fatalf("publication cancellation incomplete: %+v", cancelled)
	}
	if _, err = store.CancelPublication(ctx, toCancel.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("repeated cancellation should conflict, got %v", err)
	}
}

func TestCapturedExternalPostAndBridgeRequestCommitTogether(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "topic"})
	expiresAt := time.Now().UTC().Add(time.Minute)
	pending := BridgeRequestRecord{ID: "capture-request", PairingID: "pair", Status: "pending", ExpiresAt: expiresAt, Payload: []byte(`{"status":"pending"}`)}
	if err = store.SaveBridgeRequest(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.ExecContext(ctx, `CREATE TRIGGER reject_capture_request BEFORE UPDATE ON bridge_requests BEGIN SELECT RAISE(ABORT,'forced request update failure'); END`); err != nil {
		t.Fatal(err)
	}
	post := ExternalPost{ID: "captured-post", TopicID: topic.ID, AccountID: account.ID, Platform: "x", URL: "https://x.com/user/status/1", CapturedBy: "browser"}
	captured := BridgeRequestRecord{ID: pending.ID, PairingID: pending.PairingID, Status: "captured", ExpiresAt: expiresAt, Payload: []byte(`{"status":"captured","external_post_id":"captured-post"}`)}
	if _, err = store.CreateExternalPostAndBridgeRequest(ctx, post, captured); err == nil {
		t.Fatal("forced request update failure was ignored")
	}
	posts, err := store.ListExternalPosts(ctx, topic.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("external post was not rolled back: %+v %v", posts, err)
	}
	requests, err := store.ListBridgeRequests(ctx)
	if err != nil || len(requests) != 1 || requests[0].Status != "pending" || string(requests[0].Payload) != string(pending.Payload) {
		t.Fatalf("pending request changed after rollback: %+v %v", requests, err)
	}
	if _, err = store.DB.ExecContext(ctx, `DROP TRIGGER reject_capture_request`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateExternalPostAndBridgeRequest(ctx, post, captured); err != nil {
		t.Fatal(err)
	}
	secondPost := post
	secondPost.ID = "captured-post-2"
	secondCapture := captured
	secondCapture.Payload = []byte(`{"status":"captured","external_post_id":"captured-post-2"}`)
	if _, err = store.CreateExternalPostAndBridgeRequest(ctx, secondPost, secondCapture); !errors.Is(err, ErrConflict) {
		t.Fatalf("repeated request should conflict without duplicating the post: %v", err)
	}
	posts, err = store.ListExternalPosts(ctx, topic.ID)
	if err != nil || len(posts) != 1 || posts[0].ID != post.ID {
		t.Fatalf("captured posts=%+v err=%v", posts, err)
	}
}

func TestPublicationStatusValidationAndTransitions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "mastodon", Name: "main"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "topic"})
	variant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	if _, err = store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "typo"}); err == nil {
		t.Fatal("unknown publication status was accepted")
	}
	published, err := store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: PublicationPublished})
	if err != nil {
		t.Fatal(err)
	}
	published.Status = PublicationFailed
	if err = store.UpdatePublication(ctx, published); !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal publication transition error=%v", err)
	}
}

func TestOperationalSummariesIncludeTopicProgressAndPublicationContext(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "mastodon", Name: "main"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "Campaign"})
	publishedVariant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	publishedVariant.Text = "published body"
	if publishedVariant, err = store.UpdateVariant(ctx, publishedVariant); err != nil {
		t.Fatal(err)
	}
	waitingVariant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, publishedVariant.ID)
	if _, err = store.CreatePublication(ctx, Publication{VariantID: publishedVariant.ID, AccountID: account.ID, Status: "published"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreatePublication(ctx, Publication{VariantID: waitingVariant.ID, AccountID: account.ID, Status: "needs_confirmation"}); err != nil {
		t.Fatal(err)
	}
	publishedVariant, err = store.GetVariant(ctx, publishedVariant.ID)
	if err != nil || !publishedVariant.Published || !publishedVariant.PublicationLocked || publishedVariant.PublicationStatus != "published" {
		t.Fatalf("successful publication state missing: %+v %v", publishedVariant, err)
	}
	waitingVariant, err = store.GetVariant(ctx, waitingVariant.ID)
	if err != nil || waitingVariant.Published || !waitingVariant.PublicationLocked || waitingVariant.PublicationStatus != "needs_confirmation" {
		t.Fatalf("unresolved publication state confused with success: %+v %v", waitingVariant, err)
	}

	topics, err := store.ListTopicSummaries(ctx, profile.ID)
	if err != nil || len(topics) != 1 {
		t.Fatalf("topic summaries: %+v %v", topics, err)
	}
	if topics[0].TargetCount != 2 || topics[0].PublishedCount != 1 || topics[0].NeedsActionCount != 1 || len(topics[0].Platforms) != 1 || topics[0].Platforms[0] != "mastodon" {
		t.Fatalf("unexpected topic progress: %+v", topics[0])
	}
	publications, err := store.ListPublicationSummaries(ctx)
	if err != nil || len(publications) != 2 {
		t.Fatalf("publication summaries: %+v %v", publications, err)
	}
	if publications[0].Status != "needs_confirmation" || publications[0].TopicID != topic.ID || publications[0].TopicTitle != topic.Title {
		t.Fatalf("actionable publication should be first and contextual: %+v", publications[0])
	}
	if publications[1].Text != "published body" {
		t.Fatalf("publication text snapshot context missing: %+v", publications[1])
	}
}

func TestPublicationListsAndStatusSummaryAreProfileScoped(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	createPublication := func(profile Profile, name, status string) Publication {
		account, createErr := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "mastodon", Name: name})
		if createErr != nil {
			t.Fatal(createErr)
		}
		topic, createErr := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: name})
		if createErr != nil {
			t.Fatal(createErr)
		}
		variant, createErr := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
		if createErr != nil {
			t.Fatal(createErr)
		}
		publication, createErr := store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: status})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return publication
	}
	first, err := store.CreateProfile(ctx, Profile{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateProfile(ctx, Profile{Name: "second"})
	if err != nil {
		t.Fatal(err)
	}
	failed := createPublication(first, "first-failure", PublicationFailed)
	createPublication(first, "first-confirmation", PublicationNeedsConfirmation)
	secondFailure := createPublication(second, "second-failure", PublicationFailed)

	rows, err := store.ListPublicationSummariesForProfile(ctx, first.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("first profile publications=%+v err=%v", rows, err)
	}
	for _, row := range rows {
		if row.ID == secondFailure.ID {
			t.Fatalf("second profile publication leaked into first profile: %+v", row)
		}
	}
	if rows[0].ID != failed.ID && rows[1].ID != failed.ID {
		t.Fatalf("first profile failure missing: %+v", rows)
	}
	summary, err := store.PublicationStatusSummary(ctx, first.ID)
	if err != nil || summary.ActionCount != 2 || summary.FailedCount != 1 {
		t.Fatalf("first profile summary=%+v err=%v", summary, err)
	}
	secondSummary, err := store.PublicationStatusSummary(ctx, second.ID)
	if err != nil || secondSummary.ActionCount != 1 || secondSummary.FailedCount != 1 {
		t.Fatalf("second profile summary=%+v err=%v", secondSummary, err)
	}
	unknown, err := store.PublicationStatusSummary(ctx, "missing")
	if err != nil || unknown.ActionCount != 0 || unknown.FailedCount != 0 {
		t.Fatalf("unknown profile summary=%+v err=%v", unknown, err)
	}
}

func TestStoreCoreAndVariantRelations(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.CreateProfile(ctx, Profile{Name: "personal"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateAccount(ctx, Account{ProfileID: "missing", Platform: "mastodon", Name: "bad"}); err != ErrNotFound {
		t.Fatalf("expected profile constraint, got %v", err)
	}
	a, err := s.CreateAccount(ctx, Account{ProfileID: p.ID, Platform: "mastodon", Name: "main", BaseURL: "http://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateAccount(ctx, Account{ProfileID: p.ID, Platform: "mastodon", Name: "second", BaseURL: "http://example.test"}); err != nil {
		t.Fatalf("a profile must allow multiple accounts on the same platform: %v", err)
	}
	topic, err := s.CreateTopic(ctx, Topic{ProfileID: p.ID, Title: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: a.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Relation != "linked" || v.SourceRevision != "1" {
		t.Fatalf("unexpected linked variant: %+v", v)
	}
	initialDraft, err := s.GetBaseDraft(ctx, topic.ID)
	if err != nil {
		t.Fatal(err)
	}
	unchangedDraft, err := s.UpdateBaseDraft(ctx, initialDraft)
	if err != nil || unchangedDraft.Revision != initialDraft.Revision {
		t.Fatalf("unchanged draft should be a no-op: before=%+v after=%+v err=%v", initialDraft, unchangedDraft, err)
	}
	if _, err = s.UpdateBaseDraft(ctx, BaseDraft{TopicID: topic.ID, Text: "changed", Format: "plain"}); err != nil {
		t.Fatal(err)
	}
	topicAfterDraft, err := s.GetTopic(ctx, topic.ID)
	if err != nil || !topicAfterDraft.UpdatedAt.After(topic.UpdatedAt) {
		t.Fatalf("draft edit did not touch topic: %+v %v", topicAfterDraft, err)
	}
	v, err = s.GetVariant(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Outdated {
		t.Fatal("linked variant should become outdated")
	}
	if _, err = s.ApplyVariantUpdate(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	v, err = s.GetVariant(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Outdated || v.Text != "changed" {
		t.Fatalf("update was not applied: %+v", v)
	}
	v.Text = "platform edit"
	v, err = s.UpdateVariant(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	if v.Relation != "detached" {
		t.Fatal("first direct edit should detach")
	}
	copyV, err := s.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: a.ID}, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if copyV.Relation != "detached" {
		t.Fatal("variant copy should be detached")
	}
	if _, err = s.CreateExternalPost(ctx, ExternalPost{TopicID: topic.ID, Platform: "x", URL: "https://x.test/post/1", TextSnapshot: "visible", MediaSnapshot: []ExternalMedia{{Filename: "p.png", MIME: "image/png"}}, CapturedBy: "browser", Incomplete: true, IncompleteReason: "collapsed"}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(dir, "image.png")
	if err = os.WriteFile(mediaPath, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddMedia(ctx, Media{TopicID: topic.ID, VariantID: v.ID, Filename: "image.png", MIME: "image/png", Path: mediaPath, Size: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddMedia(ctx, Media{TopicID: topic.ID, Filename: "shared.png", MIME: "image/png", Path: mediaPath, Size: 3}); err != nil {
		t.Fatalf("topic-level media should use a NULL variant reference: %v", err)
	}
	if _, err = s.CreatePublication(ctx, Publication{VariantID: v.ID, AccountID: a.ID, Status: "pending", RemoteCID: "cid-test"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, e := reopened.GetBaseDraft(ctx, topic.ID); e != nil || got.Text != "changed" {
		t.Fatalf("draft did not persist: %+v %v", got, e)
	}
	if got, e := reopened.ListVariants(ctx, topic.ID); e != nil || len(got) != 2 {
		t.Fatalf("variants did not persist: %d %v", len(got), e)
	}
	if got, e := reopened.ListMedia(ctx, topic.ID); e != nil || len(got) != 2 {
		t.Fatalf("media did not persist: %d %v", len(got), e)
	}
	if got, e := reopened.ListPublications(ctx); e != nil || len(got) != 1 || got[0].RemoteCID != "cid-test" {
		t.Fatalf("publication history did not persist: %d %v", len(got), e)
	}
	if got, e := reopened.ListExternalPosts(ctx, topic.ID); e != nil || len(got) != 1 || got[0].TextSnapshot != "visible" || !got[0].Incomplete || len(got[0].MediaSnapshot) != 1 {
		t.Fatalf("external snapshot did not persist: %+v %v", got, e)
	}
}

func TestStoreRejectsCrossTopicReferences(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p1, _ := s.CreateProfile(ctx, Profile{Name: "one"})
	p2, _ := s.CreateProfile(ctx, Profile{Name: "two"})
	a1, _ := s.CreateAccount(ctx, Account{ProfileID: p1.ID, Platform: "mastodon", Name: "one"})
	a2, _ := s.CreateAccount(ctx, Account{ProfileID: p2.ID, Platform: "mastodon", Name: "two"})
	t1, _ := s.CreateTopic(ctx, Topic{ProfileID: p1.ID, Title: "one"})
	t2, _ := s.CreateTopic(ctx, Topic{ProfileID: p2.ID, Title: "two"})
	v1, _ := s.CreateVariant(ctx, Variant{TopicID: t1.ID, AccountID: a1.ID}, "")
	if _, err = s.CreateVariant(ctx, Variant{TopicID: t2.ID, AccountID: a2.ID}, v1.ID); err == nil {
		t.Fatal("cross-topic variant copy must be rejected")
	}
	mediaPath := filepath.Join(s.MediaDir, "image")
	if err = os.WriteFile(mediaPath, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddMedia(ctx, Media{TopicID: t2.ID, VariantID: v1.ID, Filename: "x.png", MIME: "image/png", Path: mediaPath, Size: 3}); err == nil {
		t.Fatal("cross-topic media reference must be rejected")
	}
	if _, err = s.CreateExternalPost(ctx, ExternalPost{TopicID: t2.ID, AccountID: a1.ID, Platform: "x", URL: "https://x.test/1"}); err == nil {
		t.Fatal("cross-profile external account must be rejected")
	}
	if _, err = s.CreateExternalPost(ctx, ExternalPost{TopicID: t1.ID, AccountID: a1.ID, Platform: "misskey", URL: "https://example.test/1"}); err == nil {
		t.Fatal("external platform must match its account")
	}
	for _, invalidURL := range []string{"/relative", "file:///tmp/post", "https:///missing-host"} {
		if _, err = s.CreateExternalPost(ctx, ExternalPost{TopicID: t1.ID, Platform: "x", URL: invalidURL}); err == nil {
			t.Fatalf("invalid external URL accepted: %q", invalidURL)
		}
	}
}

func TestPublicationHistoryLocksVariantAndHistoricalMedia(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "t"})
	variant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	variantPath := filepath.Join(store.MediaDir, "variant-history")
	sharedPath := filepath.Join(store.MediaDir, "shared-history")
	if err = os.WriteFile(variantPath, []byte("variant"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(sharedPath, []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	variantMedia, _ := store.AddMedia(ctx, Media{TopicID: topic.ID, VariantID: variant.ID, Filename: "variant.png", MIME: "image/png", Path: variantPath})
	sharedMedia, _ := store.AddMedia(ctx, Media{TopicID: topic.ID, Filename: "shared.png", MIME: "image/png", Path: sharedPath})
	_, _ = store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "failed"})

	locked, err := store.GetVariant(ctx, variant.ID)
	if err != nil || locked.Published || !locked.PublicationLocked || locked.PublicationStatus != "failed" {
		t.Fatalf("publication-derived field missing: %+v %v", locked, err)
	}
	locked.Text = "rewrite history"
	if _, err = store.UpdateVariant(ctx, locked); !errors.Is(err, ErrConflict) {
		t.Fatalf("published variant edit should conflict, got %v", err)
	}
	if _, err = store.ApplyVariantUpdate(ctx, locked.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("published variant draft apply should conflict, got %v", err)
	}
	if _, err = store.AddMedia(ctx, Media{TopicID: topic.ID, VariantID: locked.ID, Filename: "new.png", MIME: "image/png", Path: filepath.Join(store.MediaDir, "new")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("published variant media attachment should conflict, got %v", err)
	}
	if err = store.DeleteMedia(ctx, variantMedia.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("variant publication media deletion should conflict, got %v", err)
	}
	if err = store.DeleteMedia(ctx, sharedMedia.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("shared publication media deletion should conflict, got %v", err)
	}
	lateSharedPath := filepath.Join(store.MediaDir, "late-shared")
	if err = os.WriteFile(lateSharedPath, []byte("late"), 0o600); err != nil {
		t.Fatal(err)
	}
	lateShared, err := store.AddMedia(ctx, Media{TopicID: topic.ID, Filename: "late.png", MIME: "image/png", Path: lateSharedPath})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteMedia(ctx, lateShared.ID); err != nil {
		t.Fatalf("topic media added after the publication should remain deletable: %v", err)
	}
}

func TestVariantRulesFollowTargetPlatform(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, _ := s.CreateProfile(ctx, Profile{Name: "personal"})
	mastodon, _ := s.CreateAccount(ctx, Account{ProfileID: p.ID, Platform: "mastodon", Name: "mastodon"})
	misskey, _ := s.CreateAccount(ctx, Account{ProfileID: p.ID, Platform: "misskey", Name: "misskey"})
	bluesky, _ := s.CreateAccount(ctx, Account{ProfileID: p.ID, Platform: "bluesky", Name: "bluesky", RemoteID: "me.test"})
	topic, _ := s.CreateTopic(ctx, Topic{ProfileID: p.ID, Title: "rules"})
	source, err := s.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: mastodon.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	source.Visibility, source.SpoilerText, source.LocalOnly = "private", "cw", true
	source, err = s.UpdateVariant(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if source.LocalOnly {
		t.Fatal("Mastodon must not persist Misskey-only local_only")
	}
	mfm, err := s.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: misskey.ID}, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mfm.Format != "mfm" || mfm.Visibility != "public" || mfm.SpoilerText != "cw" {
		t.Fatalf("Misskey copy was not normalized: %+v", mfm)
	}
	mfm.Visibility = "home"
	if _, err = s.UpdateVariant(ctx, mfm); err != nil {
		t.Fatalf("Misskey home visibility must be accepted: %v", err)
	}
	mfm.Visibility = "specified"
	if _, err = s.UpdateVariant(ctx, mfm); err == nil {
		t.Fatal("specified visibility without recipient support must be rejected for Misskey")
	}
	bsky, err := s.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: bluesky.ID}, mfm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bsky.Format != "plain" || bsky.Visibility != "public" || bsky.SpoilerText != "" || bsky.LocalOnly {
		t.Fatalf("Bluesky copy was not normalized: %+v", bsky)
	}
}

func TestRestartRecoversInterruptedPublishingWithoutRetry(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	a, _ := store.CreateAccount(ctx, Account{ProfileID: p.ID, Platform: "mastodon", Name: "a"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: p.ID, Title: "recovery"})
	v, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: a.ID}, "")
	publication, err := store.CreatePublication(ctx, Publication{VariantID: v.ID, AccountID: a.ID, Status: "publishing"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetPublication(ctx, publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "needs_confirmation" || got.ErrorCategory != "unknown" || got.ErrorMessage == "" {
		t.Fatalf("interrupted publication was not recovered: %+v", got)
	}
}

func TestTranslationConfigStoresOnlyCredentialReferences(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.CreateTranslationConfig(ctx, TranslationConfig{Provider: "openai", TokenEnv: "sk-secret-value"}); err == nil {
		t.Fatal("a credential value must not be accepted as an environment variable reference")
	}
	config, err := store.CreateTranslationConfig(ctx, TranslationConfig{Provider: "openai", TokenEnv: "OPENAI_TRANSLATION_KEY", KeyEnv: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if config.TokenEnv != "OPENAI_TRANSLATION_KEY" || config.KeyEnv != "" {
		t.Fatalf("unexpected persisted credential references: %+v", config)
	}
}

func TestDeactivateAccountPreservesHistory(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p, err := store.CreateProfile(ctx, Profile{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.CreateAccount(ctx, Account{ProfileID: p.ID, Platform: "x", Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: p.ID, Title: "history"})
	v, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: a.ID, Text: "old"}, "")
	if _, err = store.DeactivateAccount(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAccount(ctx, a.ID)
	if err != nil || !got.Disabled {
		t.Fatalf("account=%+v err=%v", got, err)
	}
	if _, err = store.GetVariant(ctx, v.ID); err != nil {
		t.Fatalf("variant history lost: %v", err)
	}
}

func TestDeleteMediaAndUnpublishedVariantRemoveLocalFiles(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "t"})
	variant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	firstPath := filepath.Join(store.MediaDir, "first")
	if err = os.WriteFile(firstPath, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := store.AddMedia(ctx, Media{TopicID: topic.ID, Filename: "first.png", MIME: "image/png", Path: firstPath})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteMedia(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatalf("deleted media file remains: %v", err)
	}
	secondPath := filepath.Join(store.MediaDir, "second")
	if err = os.WriteFile(secondPath, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddMedia(ctx, Media{TopicID: topic.ID, VariantID: variant.ID, Filename: "second.png", MIME: "image/png", Path: secondPath}); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteVariant(ctx, variant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetVariant(ctx, variant.ID); err != ErrNotFound {
		t.Fatalf("variant was not deleted: %v", err)
	}
	if _, err = os.Stat(secondPath); !os.IsNotExist(err) {
		t.Fatalf("variant media file remains: %v", err)
	}
}

func TestDeleteVariantRemovesLocalPublicationHistory(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "t"})
	variant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	if _, err = store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "published"}); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteVariant(ctx, variant.ID); err != nil {
		t.Fatalf("local published variant should be removable: %v", err)
	}
	if _, err = store.GetVariant(ctx, variant.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("variant was not removed: %v", err)
	}
	if rows, err := store.ListPublications(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("local publication history remains: %v %v", rows, err)
	}
}

func TestDeleteVariantPreservesUnresolvedPublication(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "t"})
	variant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	publication, _ := store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "needs_confirmation"})
	if err = store.DeleteVariant(ctx, variant.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("unresolved variant deletion error=%v", err)
	}
	if _, err = store.GetVariant(ctx, variant.ID); err != nil {
		t.Fatalf("variant removed despite unresolved publication: %v", err)
	}
	rows, err := store.ListPublications(ctx)
	if err != nil || len(rows) != 1 || rows[0].ID != publication.ID {
		t.Fatalf("unresolved publication changed: %v %v", rows, err)
	}
}

func TestDeletePublicationOnlyAllowsTerminalStatuses(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "t"})
	variant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	pending, _ := store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "publishing"})
	if err = store.DeletePublication(ctx, pending.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("pending deletion error=%v", err)
	}
	terminal, _ := store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "failed"})
	if err = store.DeletePublication(ctx, terminal.ID); err != nil {
		t.Fatalf("terminal deletion: %v", err)
	}
}

func TestDeleteDisabledAccountRemovesLocalGraph(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	account, _ := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "x", Name: "x"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "t"})
	variant, _ := store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, "")
	if _, err = store.CreatePublication(ctx, Publication{VariantID: variant.ID, AccountID: account.ID, Status: "published"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DeactivateAccount(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DeleteAccountPermanently(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetAccount(ctx, account.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("account remains: %v", err)
	}
	if _, err = store.GetVariant(ctx, variant.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("variant remains: %v", err)
	}
	if rows, err := store.ListPublications(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("publication remains: %v %v", rows, err)
	}
}

func TestCreateAccountReactivatesMatchingDisabledRemoteIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	original, err := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "misskey", Name: "old", RemoteID: "remote-1", BaseURL: "https://old.example", CredentialRef: "old-ref"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DeactivateAccount(ctx, original.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := store.CreateAccount(ctx, Account{ProfileID: profile.ID, Platform: "misskey", Name: "new", RemoteID: "remote-1", BaseURL: "https://new.example", CredentialRef: "new-ref"})
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != original.ID || restored.Disabled || restored.Name != "new" || restored.CredentialRef != "new-ref" {
		t.Fatalf("account was not reactivated in place: %+v", restored)
	}
}

func TestAccountCanBeAttachedToAnotherProfileWithoutDuplicatingConnection(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, _ := store.CreateProfile(ctx, Profile{Name: "first"})
	second, _ := store.CreateProfile(ctx, Profile{Name: "second"})
	account, err := store.CreateAccount(ctx, Account{ProfileID: first.ID, Platform: "mastodon", Name: "shared", CredentialRef: "one-secret"})
	if err != nil {
		t.Fatal(err)
	}
	attached, err := store.AttachAccount(ctx, second.ID, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attached.ProfileIDs) != 2 || attached.CredentialRef != "one-secret" {
		t.Fatalf("attached account=%+v", attached)
	}
	listed, err := store.ListAccounts(ctx, second.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != account.ID {
		t.Fatalf("second profile accounts=%+v err=%v", listed, err)
	}
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: second.ID, Title: "shared target"})
	if _, err = store.CreateVariant(ctx, Variant{TopicID: topic.ID, AccountID: account.ID}, ""); err != nil {
		t.Fatalf("attached account was not usable: %v", err)
	}
	if _, err = store.CreateExternalPost(ctx, ExternalPost{TopicID: topic.ID, AccountID: account.ID, Platform: account.Platform, URL: "https://example.test/post/1"}); err != nil {
		t.Fatalf("attached account could not store an external post: %v", err)
	}
}

func TestAccountBelongsToTopicReturnsDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AccountBelongsToTopic(ctx, "topic", "account"); err == nil {
		t.Fatal("membership lookup must preserve database errors")
	}
}

func TestDeletingMediaDoesNotRemoveAFileStillReferencedByAnotherRecord(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile, _ := store.CreateProfile(ctx, Profile{Name: "p"})
	topic, _ := store.CreateTopic(ctx, Topic{ProfileID: profile.ID, Title: "t"})
	path := filepath.Join(store.MediaDir, "shared")
	if err = os.WriteFile(path, []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, _ := store.AddMedia(ctx, Media{TopicID: topic.ID, Filename: "one.png", MIME: "image/png", Path: path})
	second, _ := store.AddMedia(ctx, Media{TopicID: topic.ID, Filename: "two.png", MIME: "image/png", Path: path})
	if err = store.DeleteMedia(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("shared file was removed while another record referenced it: %v", err)
	}
	if err = store.DeleteMedia(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unreferenced file remains: %v", err)
	}
}
