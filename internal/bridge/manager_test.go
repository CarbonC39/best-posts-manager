package bridge

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"best-post-manager/internal/app"
)

func mustNewManager(t *testing.T, store *app.Store) *Manager {
	t.Helper()
	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func bridgeFixture(t *testing.T) (*Manager, app.Topic, app.Account, app.Variant) {
	t.Helper()
	ctx := context.Background()
	st, err := app.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p, _ := st.CreateProfile(ctx, app.Profile{Name: "p"})
	a, err := st.CreateAccount(ctx, app.Account{ProfileID: p.ID, Platform: "x", Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	topic, _ := st.CreateTopic(ctx, app.Topic{ProfileID: p.ID, Title: "t"})
	_, _ = st.UpdateBaseDraft(ctx, app.BaseDraft{TopicID: topic.ID, Text: "hello", Format: "plain"})
	v, err := st.CreateVariant(ctx, app.Variant{TopicID: topic.ID, AccountID: a.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	return mustNewManager(t, st), topic, a, v
}

func TestPairingFillResultIsOneTimeAndIdempotent(t *testing.T) {
	m, _, _, v := bridgeFixture(t)
	pair, err := m.CreatePairing("x")
	if err != nil {
		t.Fatal(err)
	}
	hello, err := m.Hello(pair.Code, "x", "https://x.com/compose", "")
	if err != nil || hello.Token == "" || pair.Token != "" {
		t.Fatal(err)
	}
	if hello.PageURL != "https://x.com/compose" || !hello.Connected || hello.LastSeenAt.IsZero() {
		t.Fatalf("pairing diagnostics: %+v", hello)
	}
	if _, err = m.Hello(pair.Code, "x", "https://x.com/compose", ""); err == nil {
		t.Fatal("pairing code must be one-time")
	}
	r, err := m.CreateFill(context.Background(), pair.ID, v.ID, "https://x.com/compose")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pending" || r.PublicationID == "" {
		t.Fatalf("fill: %+v", r)
	}
	for attempt := 0; attempt < 2; attempt++ {
		pending := m.Pending(pair.ID)
		if len(pending) != 1 || pending[0].ID != r.ID {
			t.Fatalf("pending replay %d: %+v", attempt, pending)
		}
	}
	done, err := m.Result(context.Background(), r.ID, "https://x.com/user/status/1", "remote-1")
	if err != nil || done.Status != "published" {
		t.Fatalf("result: %+v %v", done, err)
	}
	if _, err = m.Result(context.Background(), r.ID, "https://x.com/user/status/1", "remote-1"); err != nil {
		t.Fatalf("same result should be idempotent: %v", err)
	}
	if _, err = m.Result(context.Background(), r.ID, "https://x.com/user/status/2", "remote-2"); err == nil {
		t.Fatal("conflicting result must fail")
	}
	if err = m.AuthenticatePairing("wrong", pair.ID, "x", "https://x.com/compose", ""); err == nil {
		t.Fatal("wrong token must be rejected")
	}
	if err = m.AuthenticatePairing(hello.Token, pair.ID, "x", "https://x.com/compose", ""); err != nil {
		t.Fatalf("issued token must authenticate: %v", err)
	}
	pub, err := m.Store.GetPublication(context.Background(), r.PublicationID)
	if err != nil || pub.Status != "published" || pub.RemoteID != "remote-1" || pub.PublishedAt.IsZero() {
		t.Fatalf("publication: %+v %v", pub, err)
	}
}

func TestFillCanUseLastConnectedPageAndFilledIsIdempotent(t *testing.T) {
	m, _, _, v := bridgeFixture(t)
	pair, _ := m.CreatePairing("x")
	if _, err := m.Hello(pair.Code, "x", "https://x.com/compose", ""); err != nil {
		t.Fatal(err)
	}
	r, err := m.CreateFill(context.Background(), pair.ID, v.ID, "")
	if err != nil || r.PageURL != "https://x.com/compose" {
		t.Fatalf("implicit page URL: %+v %v", r, err)
	}
	filled, err := m.Filled(r.ID, "图片未找到")
	if err != nil || filled.Status != "filled" || filled.Warning != "图片未找到" {
		t.Fatalf("filled: %+v %v", filled, err)
	}
	if _, err = m.Filled(r.ID, "图片未找到"); err != nil {
		t.Fatalf("filled retry: %v", err)
	}
	done, err := m.Result(context.Background(), r.ID, "https://x.com/user/status/filled", "")
	if err != nil || done.Status != "published" {
		t.Fatalf("result after filled: %+v %v", done, err)
	}
}

func TestPairingTokenSurvivesManagerRestart(t *testing.T) {
	m, _, _, _ := bridgeFixture(t)
	pair, err := m.CreatePairing("x")
	if err != nil {
		t.Fatal(err)
	}
	hello, err := m.Hello(pair.Code, "x", "https://x.com/compose", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := m.Store.DataDir
	if err = m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := app.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored := mustNewManager(t, reopened)
	if err = restored.AuthenticatePairing(hello.Token, hello.ID, "x", "https://x.com/compose", ""); err != nil {
		t.Fatalf("persisted token did not authenticate after restart: %v", err)
	}
	again, err := restored.CreatePairing("x")
	if err != nil || again.Code == "" || again.ID == hello.ID {
		t.Fatalf("explicit re-pair should issue a fresh one-time code: %+v %v", again, err)
	}
}

func TestBridgeRequestSurvivesManagerRestartWithoutRefillingCompletedEditor(t *testing.T) {
	m, _, _, variant := bridgeFixture(t)
	pair, _ := m.CreatePairing("x")
	if _, err := m.Hello(pair.Code, "x", "https://x.com/compose", ""); err != nil {
		t.Fatal(err)
	}
	request, err := m.CreateFill(context.Background(), pair.ID, variant.ID, "https://x.com/compose")
	if err != nil {
		t.Fatal(err)
	}
	dir := m.Store.DataDir
	if err = m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := app.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	restored := mustNewManager(t, reopened)
	if pending := restored.Pending(pair.ID); len(pending) != 1 || pending[0].ID != request.ID || pending[0].PublicationID != request.PublicationID {
		t.Fatalf("pending request was not restored: %+v", pending)
	}
	if _, err = restored.Filled(request.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err = app.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored = mustNewManager(t, reopened)
	if pending := restored.Pending(pair.ID); len(pending) != 0 {
		t.Fatalf("filled editor request must not be replayed: %+v", pending)
	}
	filled, err := restored.Request(request.ID)
	if err != nil || filled.Status != "filled" {
		t.Fatalf("filled request result channel was not restored: %+v %v", filled, err)
	}
}

func TestCapturedRequestIsIdempotentAcrossRestart(t *testing.T) {
	m, topic, account, _ := bridgeFixture(t)
	pair, _ := m.CreatePairing("x")
	if _, err := m.Hello(pair.Code, "x", "https://x.com/post/1", ""); err != nil {
		t.Fatal(err)
	}
	request, err := m.CreateCapture(context.Background(), pair.ID, topic.ID, account.ID, "x", "https://x.com/post/1")
	if err != nil {
		t.Fatal(err)
	}
	captured, err := m.Capture(context.Background(), request.ID, "visible", "https://x.com/post/1", nil, true, "  ")
	if err != nil || captured.Status != "captured" || captured.ExternalPostID == "" || captured.IncompleteReason != "snapshot is incomplete" {
		t.Fatalf("captured request=%+v err=%v", captured, err)
	}
	posts, err := m.Store.ListExternalPosts(context.Background(), topic.ID)
	if err != nil || len(posts) != 1 || posts[0].ID != captured.ExternalPostID || posts[0].IncompleteReason != captured.IncompleteReason {
		t.Fatalf("captured post=%+v err=%v", posts, err)
	}
	records, err := m.Store.ListBridgeRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var payload Request
	for _, record := range records {
		if record.ID == request.ID {
			if err = json.Unmarshal(record.Payload, &payload); err != nil {
				t.Fatal(err)
			}
		}
	}
	if payload.ExternalPostID != posts[0].ID || payload.IncompleteReason != posts[0].IncompleteReason || payload.Status != "captured" {
		t.Fatalf("persisted payload and post differ: payload=%+v post=%+v", payload, posts[0])
	}
	dir := m.Store.DataDir
	if err = m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := app.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored := mustNewManager(t, reopened)
	again, err := restored.Capture(context.Background(), request.ID, "different", "", nil, false, "")
	if err != nil || again.ExternalPostID != captured.ExternalPostID || again.Status != "captured" {
		t.Fatalf("repeated capture after restart=%+v err=%v", again, err)
	}
	posts, err = reopened.ListExternalPosts(context.Background(), topic.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("repeated capture created another post: %+v %v", posts, err)
	}
}

func TestNewManagerRejectsCorruptPersistedState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *Manager, Request)
	}{
		{name: "pairing hash", mutate: func(t *testing.T, manager *Manager, _ Request) {
			if _, err := manager.Store.DB.Exec(`UPDATE bridge_pairings SET token_hash='not-a-hash'`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "pairing lifecycle order", mutate: func(t *testing.T, manager *Manager, _ Request) {
			if _, err := manager.Store.DB.Exec(`UPDATE bridge_pairings SET token_expires_at=created_at`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "request json", mutate: func(t *testing.T, manager *Manager, request Request) {
			if _, err := manager.Store.DB.Exec(`UPDATE bridge_requests SET payload='{' WHERE id=?`, request.ID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "outer status mismatch", mutate: func(t *testing.T, manager *Manager, request Request) {
			if _, err := manager.Store.DB.Exec(`UPDATE bridge_requests SET status='filled' WHERE id=?`, request.ID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "protocol invariant", mutate: func(t *testing.T, manager *Manager, request Request) {
			request.Version = "v2"
			record, err := bridgeRequestRecord(request)
			if err != nil {
				t.Fatal(err)
			}
			if err = manager.Store.SaveBridgeRequest(context.Background(), record); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "terminal result", mutate: func(t *testing.T, manager *Manager, request Request) {
			request.Status = "published"
			request.ResultURL = ""
			request.RemoteID = ""
			record, err := bridgeRequestRecord(request)
			if err != nil {
				t.Fatal(err)
			}
			if err = manager.Store.SaveBridgeRequest(context.Background(), record); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "terminal result URL", mutate: func(t *testing.T, manager *Manager, request Request) {
			request.Status = "published"
			request.ResultURL = "https://x.com/compose"
			request.RemoteID = "remote"
			record, err := bridgeRequestRecord(request)
			if err != nil {
				t.Fatal(err)
			}
			if err = manager.Store.SaveBridgeRequest(context.Background(), record); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, _, _, variant := bridgeFixture(t)
			pair, _ := manager.CreatePairing("x")
			if _, err := manager.Hello(pair.Code, "x", "https://x.com/compose", ""); err != nil {
				t.Fatal(err)
			}
			request, err := manager.CreateFill(context.Background(), pair.ID, variant.ID, "https://x.com/compose")
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, manager, request)
			if _, err = NewManager(manager.Store); err == nil {
				t.Fatal("corrupt persisted bridge state was accepted")
			}
		})
	}
}

func TestNewManagerRejectsMalformedRowAmongValidRows(t *testing.T) {
	manager, topic, account, _ := bridgeFixture(t)
	pair, err := manager.CreatePairing("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Hello(pair.Code, "x", "https://x.com/post/1", ""); err != nil {
		t.Fatal(err)
	}
	valid, err := manager.CreateCapture(context.Background(), pair.ID, topic.ID, account.ID, "x", "https://x.com/post/1")
	if err != nil {
		t.Fatal(err)
	}
	malformed, err := manager.CreateCapture(context.Background(), pair.ID, topic.ID, account.ID, "x", "https://x.com/post/2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Store.DB.Exec(`UPDATE bridge_requests SET payload='{' WHERE id=?`, malformed.ID); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err = manager.Store.DB.QueryRow(`SELECT count(*) FROM bridge_requests WHERE id IN (?,?)`, valid.ID, malformed.ID).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("fixture rows=%d err=%v", rows, err)
	}
	loaded, err := NewManager(manager.Store)
	if err == nil || loaded != nil {
		t.Fatalf("manager accepted a malformed row among valid rows: manager=%v err=%v", loaded, err)
	}
}

func TestNewManagerAllowsRevokedPairingAndExpiredRequest(t *testing.T) {
	manager, topic, account, _ := bridgeFixture(t)
	now := time.Now().UTC()
	manager.Now = func() time.Time { return now }
	pair, _ := manager.CreatePairing("x")
	if _, err := manager.Hello(pair.Code, "x", "https://x.com/post/1", ""); err != nil {
		t.Fatal(err)
	}
	request, err := manager.CreateCapture(context.Background(), pair.ID, topic.ID, account.ID, "x", "https://x.com/post/1")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(RequestTTL + time.Second)
	if pending := manager.Pending(pair.ID); len(pending) != 0 {
		t.Fatalf("expired request remained pending: %+v", pending)
	}
	if err = manager.Revoke(pair.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := NewManager(manager.Store)
	if err != nil {
		t.Fatalf("valid expired/revoked state failed to load: %v", err)
	}
	loaded, err := restored.Request(request.ID)
	if err != nil || loaded.Status != "expired" || !restored.ListPairings()[0].Revoked {
		t.Fatalf("restored state request=%+v pairings=%+v err=%v", loaded, restored.ListPairings(), err)
	}
}

func TestNewPairingRotatesOldPlatformToken(t *testing.T) {
	m, _, _, _ := bridgeFixture(t)
	first, _ := m.CreatePairing("x")
	old, err := m.Hello(first.Code, "x", "https://x.com/compose", "")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := m.CreatePairing("x")
	current, err := m.Hello(second.Code, "x", "https://x.com/compose", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.AuthenticatePairing(old.Token, old.ID, "x", "https://x.com/compose", ""); err == nil {
		t.Fatal("rotated token must be rejected")
	}
	if err = m.AuthenticatePairing(current.Token, current.ID, "x", "https://x.com/compose", ""); err != nil {
		t.Fatalf("new token must authenticate: %v", err)
	}
}

func TestFillRejectsDeactivatedAccount(t *testing.T) {
	m, _, account, variant := bridgeFixture(t)
	pair, _ := m.CreatePairing("x")
	if _, err := m.Hello(pair.Code, "x", "https://x.com/compose?draft=secret#editor", ""); err != nil {
		t.Fatal(err)
	}
	if got := m.ListPairings()[0].PageURL; got != "https://x.com/compose" {
		t.Fatalf("diagnostic URL leaked query or fragment: %q", got)
	}
	if _, err := m.Store.DeactivateAccount(context.Background(), account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateFill(context.Background(), pair.ID, variant.ID, ""); err == nil {
		t.Fatal("deactivated account must not create a browser fill")
	}
}

func TestPairingOriginHostExpiryAndRevoke(t *testing.T) {
	m, _, _, _ := bridgeFixture(t)
	now := time.Now()
	m.Now = func() time.Time { return now }
	pair, err := m.CreatePairing("qzone")
	if err != nil {
		t.Fatal(err)
	}
	hello, err := m.Hello(pair.Code, "qzone", "https://qzone.qq.com/space", "https://qzone.qq.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.CreatePairing("x"); err != nil {
		t.Fatal(err)
	}
	bad, _ := m.CreatePairing("x")
	if _, err = m.Hello(bad.Code, "x", "https://x.com.evil/", ""); err == nil {
		t.Fatal("lookalike host must be rejected")
	}
	other, _ := m.CreatePairing("x")
	if _, err = m.Hello(other.Code, "x", "https://x.com/compose", "https://weibo.com"); err == nil {
		t.Fatal("origin from another platform must be rejected")
	}
	if err = m.Revoke(pair.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.AuthenticatePairing(hello.Token, pair.ID, "qzone", "https://qzone.qq.com/space", ""); err == nil {
		t.Fatal("revoked pairing must be rejected")
	}
	expired, _ := m.CreatePairing("x")
	now = now.Add(PairingTTL + time.Second)
	if _, err = m.Hello(expired.Code, "x", "https://x.com/compose", ""); err == nil {
		t.Fatal("expired pairing must be rejected")
	}
}

func TestPairingConnectedRequiresRecentHeartbeat(t *testing.T) {
	m, _, _, _ := bridgeFixture(t)
	now := time.Now()
	m.Now = func() time.Time { return now }
	pair, _ := m.CreatePairing("x")
	hello, err := m.Hello(pair.Code, "x", "https://x.com/compose", "")
	if err != nil || !hello.Connected {
		t.Fatalf("new pairing should be connected: %+v %v", hello, err)
	}
	now = now.Add(HeartbeatTTL + time.Second)
	if view := m.ListPairings()[0]; view.Connected || !view.CodeUsed {
		t.Fatalf("stale heartbeat should be visible without revoking authorization: %+v", view)
	}
	if err = m.AuthenticatePairing(hello.Token, pair.ID, "x", "https://x.com/compose", ""); err != nil {
		t.Fatal(err)
	}
	if !m.ListPairings()[0].Connected {
		t.Fatal("authenticated poll should restore connected status")
	}
}

func TestAllowedPageSuffixes(t *testing.T) {
	for _, tc := range []struct {
		platform, page string
		ok             bool
	}{
		{"x", "https://x.com/a", true}, {"x", "https://sub.twitter.com/a", true}, {"x", "https://x.com.evil/a", false},
		{"weibo", "https://weibo.com/u/1", true}, {"weibo", "https://weibo.com.evil/u/1", false},
		{"xiaohongshu", "https://www.xiaohongshu.com/explore/1", true}, {"xiaohongshu", "https://creator.rednote.com/login?source=official", true}, {"xiaohongshu", "https://rednote.com.evil/login", false}, {"qzone", "https://user.qzone.qq.com/a", true}, {"qzone", "https://qq.com/a", false}, {"qzone", "https://notqq.com/a", false},
	} {
		if got := AllowedPage(tc.platform, tc.page); got != tc.ok {
			t.Errorf("%s %s: got %v", tc.platform, tc.page, got)
		}
	}
}

func TestAllowedResultURLRejectsComposerAndHome(t *testing.T) {
	for _, raw := range []string{"https://x.com/", "https://x.com/compose/post", "https://x.com/home"} {
		if AllowedResultURL("x", raw) {
			t.Fatalf("non-permalink accepted: %s", raw)
		}
	}
	if !AllowedResultURL("x", "https://x.com/example/status/123") {
		t.Fatal("valid X permalink was rejected")
	}
	if !AllowedResultURL("qzone", "https://user.qzone.qq.com/123/mood/abc") {
		t.Fatal("valid Qzone permalink was rejected")
	}
}

func TestFillRequiresConnectedPairingAndTimeoutStaysUncertain(t *testing.T) {
	m, _, _, v := bridgeFixture(t)
	now := time.Now()
	m.Now = func() time.Time { return now }
	pair, _ := m.CreatePairing("x")
	if _, err := m.CreateFill(context.Background(), pair.ID, v.ID, "https://x.com/compose"); err == nil {
		t.Fatal("unconnected pairing must not create a publication")
	}
	if _, err := m.Hello(pair.Code, "x", "https://x.com/compose", ""); err != nil {
		t.Fatal(err)
	}
	request, err := m.CreateFill(context.Background(), pair.ID, v.ID, "https://x.com/compose")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(RequestTTL + time.Second)
	request, err = m.Request(request.ID)
	if err != nil || request.Status != "needs_confirmation" {
		t.Fatalf("timed-out fill: %+v %v", request, err)
	}
	publication, err := m.Store.GetPublication(context.Background(), request.PublicationID)
	if err != nil || publication.Status != "needs_confirmation" {
		t.Fatalf("timed-out publication: %+v %v", publication, err)
	}
}

func TestCaptureAcceptsAccountAttachedToTopicProfile(t *testing.T) {
	m, _, account, _ := bridgeFixture(t)
	ctx := context.Background()
	workspace, err := m.Store.CreateProfile(ctx, app.Profile{Name: "shared workspace"})
	if err != nil {
		t.Fatal(err)
	}
	topic, err := m.Store.CreateTopic(ctx, app.Topic{ProfileID: workspace.ID, Title: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := m.CreatePairing("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Hello(pair.Code, "x", "https://x.com/user/status/1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = m.CreateCapture(ctx, pair.ID, topic.ID, account.ID, "x", ""); err == nil {
		t.Fatal("unattached capture account must fail")
	}
	if _, err = m.Store.AttachAccount(ctx, workspace.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	request, err := m.CreateCapture(ctx, pair.ID, topic.ID, account.ID, "x", "")
	if err != nil || request.TopicID != topic.ID || request.AccountID != account.ID {
		t.Fatalf("attached capture account was rejected: %+v %v", request, err)
	}
}
