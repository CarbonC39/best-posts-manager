package bridge

import (
	"context"
	"testing"
	"time"

	"best-post-manager/internal/app"
)

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
	return NewManager(st), topic, a, v
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
	done, err := m.Result(context.Background(), r.ID, "https://x.com/i/1", "remote-1")
	if err != nil || done.Status != "published" {
		t.Fatalf("result: %+v %v", done, err)
	}
	if _, err = m.Result(context.Background(), r.ID, "https://x.com/i/1", "remote-1"); err != nil {
		t.Fatalf("same result should be idempotent: %v", err)
	}
	if _, err = m.Result(context.Background(), r.ID, "https://x.com/i/2", "remote-2"); err == nil {
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

func TestAllowedPageSuffixes(t *testing.T) {
	for _, tc := range []struct {
		platform, page string
		ok             bool
	}{
		{"x", "https://x.com/a", true}, {"x", "https://sub.twitter.com/a", true}, {"x", "https://x.com.evil/a", false},
		{"weibo", "https://weibo.com/u/1", true}, {"weibo", "https://weibo.com.evil/u/1", false},
		{"xiaohongshu", "https://www.xiaohongshu.com/explore/1", true}, {"qzone", "https://user.qzone.qq.com/a", true}, {"qzone", "https://qq.com/a", false}, {"qzone", "https://notqq.com/a", false},
	} {
		if got := AllowedPage(tc.platform, tc.page); got != tc.ok {
			t.Errorf("%s %s: got %v", tc.platform, tc.page, got)
		}
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
