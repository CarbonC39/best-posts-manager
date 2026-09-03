package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")
var credentialEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Store struct {
	DB                *sql.DB
	DataDir, MediaDir string
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	media := filepath.Join(dataDir, "media")
	if err := os.MkdirAll(media, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "best-post-manager.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, DataDir: dataDir, MediaDir: media}
	if err := s.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.RecoverPublishing(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS profiles (id TEXT PRIMARY KEY, name TEXT NOT NULL, default_language TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS accounts (id TEXT PRIMARY KEY, profile_id TEXT NOT NULL REFERENCES profiles(id), platform TEXT NOT NULL, name TEXT NOT NULL, remote_id TEXT NOT NULL DEFAULT '', base_url TEXT NOT NULL DEFAULT '', token_env TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS accounts_remote_identity ON accounts(profile_id, platform, remote_id) WHERE remote_id <> '';
CREATE TABLE IF NOT EXISTS topics (id TEXT PRIMARY KEY, profile_id TEXT NOT NULL REFERENCES profiles(id), title TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'active', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS base_drafts (id TEXT PRIMARY KEY, topic_id TEXT NOT NULL UNIQUE REFERENCES topics(id) ON DELETE CASCADE, text TEXT NOT NULL DEFAULT '', format TEXT NOT NULL DEFAULT 'plain', language TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 1, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS variants (id TEXT PRIMARY KEY, topic_id TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE, account_id TEXT NOT NULL REFERENCES accounts(id), text TEXT NOT NULL DEFAULT '', format TEXT NOT NULL DEFAULT 'plain', status TEXT NOT NULL DEFAULT 'draft', relation TEXT NOT NULL DEFAULT 'detached', source_revision TEXT NOT NULL DEFAULT '', visibility TEXT NOT NULL DEFAULT 'public', spoiler_text TEXT NOT NULL DEFAULT '', local_only INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS media (id TEXT PRIMARY KEY, topic_id TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE, variant_id TEXT REFERENCES variants(id) ON DELETE SET NULL, filename TEXT NOT NULL, mime TEXT NOT NULL, path TEXT NOT NULL, alt_text TEXT NOT NULL DEFAULT '', size INTEGER NOT NULL DEFAULT 0, width INTEGER NOT NULL DEFAULT 0, height INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS publications (id TEXT PRIMARY KEY, variant_id TEXT NOT NULL REFERENCES variants(id), account_id TEXT NOT NULL REFERENCES accounts(id), status TEXT NOT NULL, remote_id TEXT NOT NULL DEFAULT '', remote_url TEXT NOT NULL DEFAULT '', remote_cid TEXT NOT NULL DEFAULT '', error_category TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '', idempotency_key TEXT NOT NULL UNIQUE, source TEXT NOT NULL DEFAULT 'internal', published_at TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS external_posts (id TEXT PRIMARY KEY, topic_id TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE, account_id TEXT REFERENCES accounts(id), platform TEXT NOT NULL, remote_id TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '', text_snapshot TEXT NOT NULL DEFAULT '', media_snapshot TEXT NOT NULL DEFAULT '[]', captured_by TEXT NOT NULL DEFAULT 'manual', incomplete INTEGER NOT NULL DEFAULT 0, incomplete_reason TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);`)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version) VALUES(1)`)
	if err != nil {
		return err
	}
	var version int
	if err = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return err
	}
	if version < 2 {
		if _, err = s.DB.ExecContext(ctx, `ALTER TABLE variants ADD COLUMN local_only INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
		if _, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version) VALUES(2)`); err != nil {
			return err
		}
	}
	if version < 3 {
		if _, err = s.DB.ExecContext(ctx, `ALTER TABLE publications ADD COLUMN remote_cid TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
		if _, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version) VALUES(3)`); err != nil {
			return err
		}
	}
	if version < 4 {
		for _, statement := range []string{
			`ALTER TABLE external_posts ADD COLUMN media_snapshot TEXT NOT NULL DEFAULT '[]'`,
			`ALTER TABLE external_posts ADD COLUMN incomplete INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE external_posts ADD COLUMN incomplete_reason TEXT NOT NULL DEFAULT ''`,
		} {
			if _, err = s.DB.ExecContext(ctx, statement); err != nil && !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
		if _, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version) VALUES(4)`); err != nil {
			return err
		}
	}
	if version < 5 {
		tx, txErr := s.DB.BeginTx(ctx, nil)
		if txErr != nil {
			return txErr
		}
		rollback := true
		defer func() {
			if rollback {
				_ = tx.Rollback()
			}
		}()
		if _, txErr = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS translation_configs (id TEXT PRIMARY KEY, provider TEXT NOT NULL, base_url TEXT NOT NULL DEFAULT '', project_id TEXT NOT NULL DEFAULT '', location TEXT NOT NULL DEFAULT '', token_env TEXT NOT NULL DEFAULT '', key_env TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL)`); txErr != nil {
			return txErr
		}
		if _, txErr = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS translations (id TEXT PRIMARY KEY, source_variant_id TEXT NOT NULL REFERENCES variants(id), target_account_id TEXT NOT NULL REFERENCES accounts(id), target_variant_id TEXT NOT NULL REFERENCES variants(id), source_language TEXT NOT NULL DEFAULT '', target_language TEXT NOT NULL, provider TEXT NOT NULL, status TEXT NOT NULL, error_category TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL)`); txErr != nil {
			return txErr
		}
		if _, txErr = tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version) VALUES(5)`); txErr != nil {
			return txErr
		}
		if txErr = tx.Commit(); txErr != nil {
			return txErr
		}
		rollback = false
	}
	if version < 6 {
		tx, txErr := s.DB.BeginTx(ctx, nil)
		if txErr != nil {
			return txErr
		}
		rollback := true
		defer func() {
			if rollback {
				_ = tx.Rollback()
			}
		}()
		for _, statement := range []string{
			`ALTER TABLE accounts ADD COLUMN credential_ref TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE translation_configs ADD COLUMN credential_ref TEXT NOT NULL DEFAULT ''`,
		} {
			if _, txErr = tx.ExecContext(ctx, statement); txErr != nil && !strings.Contains(txErr.Error(), "duplicate column") {
				return txErr
			}
		}
		if _, txErr = tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version) VALUES(6)`); txErr != nil {
			return txErr
		}
		if txErr = tx.Commit(); txErr != nil {
			return txErr
		}
		rollback = false
	}
	if version < 7 {
		tx, txErr := s.DB.BeginTx(ctx, nil)
		if txErr != nil {
			return txErr
		}
		rollback := true
		defer func() {
			if rollback {
				_ = tx.Rollback()
			}
		}()
		if _, txErr = tx.ExecContext(ctx, `ALTER TABLE accounts ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`); txErr != nil && !strings.Contains(txErr.Error(), "duplicate column") {
			return txErr
		}
		if _, txErr = tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version) VALUES(7)`); txErr != nil {
			return txErr
		}
		if txErr = tx.Commit(); txErr != nil {
			return txErr
		}
		rollback = false
	}
	if version < 8 {
		tx, txErr := s.DB.BeginTx(ctx, nil)
		if txErr != nil {
			return txErr
		}
		rollback := true
		defer func() {
			if rollback {
				_ = tx.Rollback()
			}
		}()
		if _, txErr = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS bridge_pairings (id TEXT PRIMARY KEY, platform TEXT NOT NULL UNIQUE, token_hash TEXT NOT NULL DEFAULT '', token_expires_at TEXT NOT NULL DEFAULT '', revoked INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`); txErr != nil {
			return txErr
		}
		if _, txErr = tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version) VALUES(8)`); txErr != nil {
			return txErr
		}
		if txErr = tx.Commit(); txErr != nil {
			return txErr
		}
		rollback = false
	}
	return nil
}

// RecoverPublishing makes an interrupted final publish visible to the user.
// It deliberately never retries: the remote outcome may already be successful.
func (s *Store) RecoverPublishing(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE publications SET status='needs_confirmation', error_category='unknown', error_message='程序重启时发布请求仍在进行；请到平台确认结果后再手动关联，勿盲目重试' WHERE status='publishing'`)
	return err
}

func (s *Store) ListBridgePairings(ctx context.Context) ([]BridgePairing, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,platform,token_hash,token_expires_at,revoked,created_at FROM bridge_pairings ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BridgePairing, 0)
	for rows.Next() {
		var p BridgePairing
		var expires, created string
		var revoked int
		if err = rows.Scan(&p.ID, &p.Platform, &p.TokenHash, &expires, &revoked, &created); err != nil {
			return nil, err
		}
		p.TokenExpiresAt, p.CreatedAt = parseTime(expires), parseTime(created)
		p.Revoked = revoked != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) SaveBridgePairing(ctx context.Context, p BridgePairing) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO bridge_pairings(id,platform,token_hash,token_expires_at,revoked,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(platform) DO UPDATE SET id=excluded.id, token_hash=excluded.token_hash, token_expires_at=excluded.token_expires_at, revoked=excluded.revoked`, p.ID, p.Platform, p.TokenHash, p.TokenExpiresAt.Format(time.RFC3339Nano), boolInt(p.Revoked), p.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) RevokeBridgePairing(ctx context.Context, id string) error {
	result, err := s.DB.ExecContext(ctx, `UPDATE bridge_pairings SET revoked=1 WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func id() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%x", time.Now().UnixNano(), b)
}
func parseTime(v string) time.Time { t, _ := time.Parse(time.RFC3339Nano, v); return t }
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Store) CreateProfile(ctx context.Context, p Profile) (Profile, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return p, fmt.Errorf("profile name is required")
	}
	if p.ID == "" {
		p.ID = id()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO profiles(id,name,default_language,description,created_at) VALUES(?,?,?,?,?)`, p.ID, p.Name, p.DefaultLanguage, p.Description, p.CreatedAt.Format(time.RFC3339Nano))
	return p, err
}
func (s *Store) ListProfiles(ctx context.Context) ([]Profile, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,name,default_language,description,created_at FROM profiles ORDER BY created_at`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]Profile, 0)
	for rows.Next() {
		var p Profile
		var t string
		if e = rows.Scan(&p.ID, &p.Name, &p.DefaultLanguage, &p.Description, &t); e != nil {
			return nil, e
		}
		p.CreatedAt = parseTime(t)
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) profileExists(ctx context.Context, pid string) bool {
	var x string
	return s.DB.QueryRowContext(ctx, `SELECT id FROM profiles WHERE id=?`, pid).Scan(&x) == nil
}
func (s *Store) CreateAccount(ctx context.Context, a Account) (Account, error) {
	if !s.profileExists(ctx, a.ProfileID) {
		return a, ErrNotFound
	}
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || !supportedPlatform(a.Platform) {
		return a, fmt.Errorf("a name and supported platform are required")
	}
	if a.ID == "" {
		a.ID = id()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO accounts(id,profile_id,platform,name,remote_id,base_url,token_env,credential_ref,disabled,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, a.ID, a.ProfileID, a.Platform, a.Name, a.RemoteID, a.BaseURL, a.TokenEnv, a.CredentialRef, boolInt(a.Disabled), a.CreatedAt.Format(time.RFC3339Nano))
	return a, e
}
func supportedPlatform(name string) bool {
	switch name {
	case "mastodon", "mastodon-compatible", "misskey", "bluesky", "x", "weibo", "xiaohongshu", "qzone":
		return true
	default:
		return false
	}
}
func (s *Store) ListAccounts(ctx context.Context, profileID string) ([]Account, error) {
	q := `SELECT id,profile_id,platform,name,remote_id,base_url,token_env,credential_ref,disabled,created_at FROM accounts`
	var args []any
	if profileID != "" {
		q += ` WHERE profile_id=?`
		args = append(args, profileID)
	}
	q += ` ORDER BY created_at`
	rows, e := s.DB.QueryContext(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]Account, 0)
	for rows.Next() {
		var a Account
		var t string
		var disabled int
		if e = rows.Scan(&a.ID, &a.ProfileID, &a.Platform, &a.Name, &a.RemoteID, &a.BaseURL, &a.TokenEnv, &a.CredentialRef, &disabled, &t); e != nil {
			return nil, e
		}
		a.CreatedAt = parseTime(t)
		a.Disabled = disabled != 0
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) GetAccount(ctx context.Context, aid string) (Account, error) {
	var a Account
	var t string
	var disabled int
	e := s.DB.QueryRowContext(ctx, `SELECT id,profile_id,platform,name,remote_id,base_url,token_env,credential_ref,disabled,created_at FROM accounts WHERE id=?`, aid).Scan(&a.ID, &a.ProfileID, &a.Platform, &a.Name, &a.RemoteID, &a.BaseURL, &a.TokenEnv, &a.CredentialRef, &disabled, &t)
	if e == sql.ErrNoRows {
		e = ErrNotFound
	}
	a.CreatedAt = parseTime(t)
	a.Disabled = disabled != 0
	return a, e
}

// DeactivateAccount preserves all foreign-key history while preventing future use.
func (s *Store) DeactivateAccount(ctx context.Context, aid string) (Account, error) {
	if _, err := s.GetAccount(ctx, aid); err != nil {
		return Account{}, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE accounts SET disabled=1, credential_ref='' WHERE id=?`, aid); err != nil {
		return Account{}, err
	}
	return s.GetAccount(ctx, aid)
}
func (s *Store) CreateTopic(ctx context.Context, t Topic) (Topic, error) {
	if !s.profileExists(ctx, t.ProfileID) {
		return t, ErrNotFound
	}
	t.Title = strings.TrimSpace(t.Title)
	if t.Title == "" {
		return t, fmt.Errorf("topic title is required")
	}
	if t.ID == "" {
		t.ID = id()
	}
	if t.Status == "" {
		t.Status = "active"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	t.UpdatedAt = t.CreatedAt
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return t, e
	}
	defer tx.Rollback()
	stamp := t.CreatedAt.Format(time.RFC3339Nano)
	if _, e = tx.ExecContext(ctx, `INSERT INTO topics(id,profile_id,title,status,created_at,updated_at) VALUES(?,?,?,?,?,?)`, t.ID, t.ProfileID, t.Title, t.Status, stamp, stamp); e != nil {
		return t, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO base_drafts(id,topic_id,revision,updated_at) VALUES(?,?,1,?)`, id(), t.ID, stamp); e != nil {
		return t, e
	}
	return t, tx.Commit()
}
func (s *Store) ListTopics(ctx context.Context, profileID string) ([]Topic, error) {
	q := `SELECT id,profile_id,title,status,created_at,updated_at FROM topics`
	var args []any
	if profileID != "" {
		q += ` WHERE profile_id=?`
		args = append(args, profileID)
	}
	q += ` ORDER BY updated_at DESC`
	rows, e := s.DB.QueryContext(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]Topic, 0)
	for rows.Next() {
		var t Topic
		var c, u string
		if e = rows.Scan(&t.ID, &t.ProfileID, &t.Title, &t.Status, &c, &u); e != nil {
			return nil, e
		}
		t.CreatedAt = parseTime(c)
		t.UpdatedAt = parseTime(u)
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) GetTopic(ctx context.Context, tid string) (Topic, error) {
	var t Topic
	var c, u string
	e := s.DB.QueryRowContext(ctx, `SELECT id,profile_id,title,status,created_at,updated_at FROM topics WHERE id=?`, tid).Scan(&t.ID, &t.ProfileID, &t.Title, &t.Status, &c, &u)
	if e == sql.ErrNoRows {
		e = ErrNotFound
	}
	t.CreatedAt = parseTime(c)
	t.UpdatedAt = parseTime(u)
	return t, e
}
func (s *Store) GetBaseDraft(ctx context.Context, topicID string) (BaseDraft, error) {
	var b BaseDraft
	var u string
	e := s.DB.QueryRowContext(ctx, `SELECT id,topic_id,text,format,language,revision,updated_at FROM base_drafts WHERE topic_id=?`, topicID).Scan(&b.ID, &b.TopicID, &b.Text, &b.Format, &b.Language, &b.Revision, &u)
	if e != nil {
		if e == sql.ErrNoRows {
			e = ErrNotFound
		}
		return b, e
	}
	b.UpdatedAt = parseTime(u)
	return b, nil
}
func (s *Store) UpdateBaseDraft(ctx context.Context, b BaseDraft) (BaseDraft, error) {
	old, e := s.GetBaseDraft(ctx, b.TopicID)
	if e != nil {
		return b, e
	}
	stamp := now()
	_, e = s.DB.ExecContext(ctx, `UPDATE base_drafts SET text=?,format=?,language=?,revision=?,updated_at=? WHERE topic_id=?`, b.Text, b.Format, b.Language, old.Revision+1, stamp, b.TopicID)
	b.ID = old.ID
	b.Revision = old.Revision + 1
	b.UpdatedAt = parseTime(stamp)
	return b, e
}
func (s *Store) accountTopicOK(ctx context.Context, topicID, accountID string) bool {
	var x int
	e := s.DB.QueryRowContext(ctx, `SELECT 1 FROM topics t JOIN accounts a ON a.profile_id=t.profile_id WHERE t.id=? AND a.id=?`, topicID, accountID).Scan(&x)
	return e == nil
}
func (s *Store) CreateVariant(ctx context.Context, v Variant, sourceVariantID string) (Variant, error) {
	if !s.accountTopicOK(ctx, v.TopicID, v.AccountID) {
		return v, ErrNotFound
	}
	account, e := s.GetAccount(ctx, v.AccountID)
	if e != nil {
		return v, e
	}
	if v.ID == "" {
		v.ID = id()
	}
	v.Status = "draft"
	v.Visibility = "public"
	v.CreatedAt = time.Now().UTC()
	v.UpdatedAt = v.CreatedAt
	if sourceVariantID != "" {
		source, e := s.getVariant(ctx, sourceVariantID)
		if e != nil {
			return v, e
		}
		if source.TopicID != v.TopicID {
			return v, fmt.Errorf("source variant must belong to the same topic")
		}
		v.Text, v.Format, v.Visibility, v.SpoilerText, v.LocalOnly = source.Text, source.Format, source.Visibility, source.SpoilerText, source.LocalOnly
		v.Relation = "detached"
	} else {
		b, e := s.GetBaseDraft(ctx, v.TopicID)
		if e != nil {
			return v, e
		}
		v.Text, v.Format, v.Relation, v.SourceRevision = b.Text, b.Format, "linked", strconv.Itoa(b.Revision)
	}
	normalizeVariantForPlatform(&v, account.Platform)
	_, e = s.DB.ExecContext(ctx, `INSERT INTO variants(id,topic_id,account_id,text,format,status,relation,source_revision,visibility,spoiler_text,local_only,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.TopicID, v.AccountID, v.Text, v.Format, v.Status, v.Relation, v.SourceRevision, v.Visibility, v.SpoilerText, boolInt(v.LocalOnly), v.CreatedAt.Format(time.RFC3339Nano), v.UpdatedAt.Format(time.RFC3339Nano))
	return v, e
}
func (s *Store) getVariant(ctx context.Context, vid string) (Variant, error) {
	var v Variant
	var c, u string
	var local int
	e := s.DB.QueryRowContext(ctx, `SELECT id,topic_id,account_id,text,format,status,relation,source_revision,visibility,spoiler_text,local_only,created_at,updated_at FROM variants WHERE id=?`, vid).Scan(&v.ID, &v.TopicID, &v.AccountID, &v.Text, &v.Format, &v.Status, &v.Relation, &v.SourceRevision, &v.Visibility, &v.SpoilerText, &local, &c, &u)
	v.LocalOnly = local != 0
	if e != nil {
		if e == sql.ErrNoRows {
			e = ErrNotFound
		}
		return v, e
	}
	v.CreatedAt = parseTime(c)
	v.UpdatedAt = parseTime(u)
	return v, nil
}
func (s *Store) GetVariant(ctx context.Context, vid string) (Variant, error) {
	v, e := s.getVariant(ctx, vid)
	if e != nil {
		return v, e
	}
	if v.Relation == "linked" {
		b, be := s.GetBaseDraft(ctx, v.TopicID)
		v.Outdated = be == nil && v.SourceRevision != strconv.Itoa(b.Revision)
	}
	return v, nil
}
func (s *Store) ListVariants(ctx context.Context, topicID string) ([]Variant, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id FROM variants WHERE topic_id=? ORDER BY created_at`, topicID)
	if e != nil {
		return nil, e
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return nil, e
	}
	if e = rows.Close(); e != nil {
		return nil, e
	}
	out := make([]Variant, 0)
	for _, variantID := range ids {
		v, e := s.GetVariant(ctx, variantID)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) UpdateVariant(ctx context.Context, v Variant) (Variant, error) {
	old, e := s.getVariant(ctx, v.ID)
	if e != nil {
		return v, e
	}
	if old.Relation == "linked" {
		v.Relation = "detached"
		v.SourceRevision = ""
	}
	if v.Status == "" {
		v.Status = old.Status
	}
	if v.Status != "draft" && v.Status != "ready" {
		return v, fmt.Errorf("invalid variant status")
	}
	if v.Visibility == "" {
		v.Visibility = old.Visibility
	}
	account, e := s.GetAccount(ctx, old.AccountID)
	if e != nil {
		return v, e
	}
	if !validVisibility(account.Platform, v.Visibility) {
		return v, fmt.Errorf("invalid visibility")
	}
	switch account.Platform {
	case "misskey":
		v.Format = "mfm"
	case "bluesky":
		v.Format, v.SpoilerText, v.LocalOnly = "plain", "", false
	default:
		v.Format, v.LocalOnly = "plain", false
	}
	stamp := now()
	_, e = s.DB.ExecContext(ctx, `UPDATE variants SET text=?,format=?,status=?,relation=?,source_revision=?,visibility=?,spoiler_text=?,local_only=?,updated_at=? WHERE id=?`, v.Text, v.Format, v.Status, v.Relation, v.SourceRevision, v.Visibility, v.SpoilerText, boolInt(v.LocalOnly), stamp, v.ID)
	v.TopicID, v.AccountID, v.CreatedAt = old.TopicID, old.AccountID, old.CreatedAt
	v.UpdatedAt = parseTime(stamp)
	v.Outdated = false
	return v, e
}

func normalizeVariantForPlatform(v *Variant, platform string) {
	switch platform {
	case "misskey":
		v.Format = "mfm"
		if !validVisibility(platform, v.Visibility) {
			v.Visibility = "public"
		}
	case "bluesky":
		v.Format, v.Visibility = "plain", "public"
		v.SpoilerText, v.LocalOnly = "", false
	default:
		v.Format = "plain"
		v.LocalOnly = false
		if !validVisibility(platform, v.Visibility) {
			v.Visibility = "public"
		}
	}
}

func validVisibility(platform, visibility string) bool {
	var allowed []string
	switch platform {
	case "misskey":
		allowed = []string{"public", "home", "followers"}
	case "bluesky", "x", "weibo", "xiaohongshu", "qzone":
		allowed = []string{"public"}
	default:
		allowed = []string{"public", "unlisted", "private", "direct"}
	}
	for _, value := range allowed {
		if visibility == value {
			return true
		}
	}
	return false
}
func (s *Store) ApplyVariantUpdate(ctx context.Context, vid string) (Variant, error) {
	v, e := s.getVariant(ctx, vid)
	if e != nil {
		return v, e
	}
	if v.Relation != "linked" {
		return v, fmt.Errorf("only linked variants can apply base draft updates")
	}
	b, e := s.GetBaseDraft(ctx, v.TopicID)
	if e != nil {
		return v, e
	}
	v.Text, v.Format, v.Relation, v.SourceRevision = b.Text, b.Format, "linked", strconv.Itoa(b.Revision)
	if account, ae := s.GetAccount(ctx, v.AccountID); ae == nil {
		normalizeVariantForPlatform(&v, account.Platform)
	}
	return s.updateLinked(ctx, v)
}
func (s *Store) updateLinked(ctx context.Context, v Variant) (Variant, error) {
	stamp := now()
	_, e := s.DB.ExecContext(ctx, `UPDATE variants SET text=?,format=?,relation='linked',source_revision=?,visibility=?,spoiler_text=?,local_only=?,updated_at=? WHERE id=?`, v.Text, v.Format, v.SourceRevision, v.Visibility, v.SpoilerText, boolInt(v.LocalOnly), stamp, v.ID)
	v.UpdatedAt = parseTime(stamp)
	v.Outdated = false
	return v, e
}
func (s *Store) AddMedia(ctx context.Context, m Media) (Media, error) {
	if !strings.HasPrefix(strings.ToLower(m.MIME), "image/") {
		return m, fmt.Errorf("only image media is supported")
	}
	if m.Size <= 0 {
		if info, e := os.Stat(m.Path); e == nil {
			m.Size = info.Size()
		}
	}
	if m.Size > 32<<20 {
		return m, fmt.Errorf("image exceeds 32 MiB")
	}
	if _, e := s.GetTopic(ctx, m.TopicID); e != nil {
		return m, e
	}
	if m.VariantID != "" {
		variant, e := s.getVariant(ctx, m.VariantID)
		if e != nil {
			return m, e
		}
		if variant.TopicID != m.TopicID {
			return m, fmt.Errorf("media variant must belong to the same topic")
		}
	}
	var count int
	query := `SELECT count(*) FROM media WHERE topic_id=? AND variant_id IS NULL`
	args := []any{m.TopicID}
	if m.VariantID != "" {
		query = `SELECT count(*) FROM media WHERE topic_id=? AND (variant_id=? OR variant_id IS NULL)`
		args = append(args, m.VariantID)
	}
	if e := s.DB.QueryRowContext(ctx, query, args...).Scan(&count); e != nil {
		return m, e
	}
	if count >= 4 {
		return m, fmt.Errorf("at most 4 media items are supported")
	}
	if m.ID == "" {
		m.ID = id()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO media(id,topic_id,variant_id,filename,mime,path,alt_text,size,width,height,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, m.ID, m.TopicID, nullString(m.VariantID), m.Filename, m.MIME, m.Path, m.AltText, m.Size, m.Width, m.Height, m.CreatedAt.Format(time.RFC3339Nano))
	return m, e
}
func (s *Store) ListMedia(ctx context.Context, topicID string) ([]Media, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,topic_id,COALESCE(variant_id,''),filename,mime,path,alt_text,size,width,height,created_at FROM media WHERE topic_id=? ORDER BY created_at`, topicID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]Media, 0)
	for rows.Next() {
		var m Media
		var t string
		if e = rows.Scan(&m.ID, &m.TopicID, &m.VariantID, &m.Filename, &m.MIME, &m.Path, &m.AltText, &m.Size, &m.Width, &m.Height, &t); e != nil {
			return nil, e
		}
		m.CreatedAt = parseTime(t)
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMedia removes one local media record and its file. Files are accepted
// only when they are direct children of the configured media directory.
func (s *Store) DeleteMedia(ctx context.Context, mediaID string) error {
	var path string
	if err := s.DB.QueryRowContext(ctx, `SELECT path FROM media WHERE id=?`, mediaID).Scan(&path); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if filepath.Dir(path) != s.MediaDir || filepath.Base(path) == "." {
		return errors.New("media path is outside the local media directory")
	}
	if info, err := os.Lstat(path); err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing to delete symlink media")
	}
	result, err := s.DB.ExecContext(ctx, `DELETE FROM media WHERE id=?`, mediaID)
	if err != nil {
		return err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		if rowsErr != nil {
			return rowsErr
		}
		return ErrNotFound
	}
	var remaining int
	if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM media WHERE path=?`, path).Scan(&remaining); err != nil {
		return fmt.Errorf("media record deleted but shared-file check failed: %w", err)
	}
	if remaining > 0 {
		return nil
	}
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("media record deleted but local file cleanup failed: %w", err)
	}
	return nil
}

// DeleteVariant removes an unpublished variant, its local media, and translation
// records that refer to it. Publication history prevents deletion.
func (s *Store) DeleteVariant(ctx context.Context, variantID string) error {
	var path string
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM variants WHERE id=?`, variantID).Scan(new(string)); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	var publications int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM publications WHERE variant_id=?`, variantID).Scan(&publications); err != nil {
		return err
	}
	if publications > 0 {
		return errors.New("variant has publication history and cannot be deleted; keep it for audit")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT path FROM media WHERE variant_id=?`, variantID)
	if err != nil {
		return err
	}
	var paths []string
	for rows.Next() {
		if err = rows.Scan(&path); err != nil {
			rows.Close()
			return err
		}
		if filepath.Dir(path) != s.MediaDir || filepath.Base(path) == "." {
			rows.Close()
			return errors.New("media path is outside the local media directory")
		}
		if info, statErr := os.Lstat(path); statErr != nil && !os.IsNotExist(statErr) {
			rows.Close()
			return statErr
		} else if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			rows.Close()
			return errors.New("refusing to delete symlink media")
		}
		paths = append(paths, path)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM translations WHERE source_variant_id=? OR target_variant_id=?`, variantID, variantID); err == nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM media WHERE variant_id=?`, variantID)
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM variants WHERE id=?`, variantID)
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	for _, mediaPath := range paths {
		var remaining int
		if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM media WHERE path=?`, mediaPath).Scan(&remaining); err != nil {
			return fmt.Errorf("variant deleted but shared-file check failed: %w", err)
		}
		if remaining > 0 {
			continue
		}
		if err = os.Remove(mediaPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("variant deleted but local media cleanup failed: %w", err)
		}
	}
	return nil
}
func (s *Store) CreatePublication(ctx context.Context, p Publication) (Publication, error) {
	if p.ID == "" {
		p.ID = id()
	}
	if p.Status == "" {
		p.Status = "pending"
	}
	if p.Source == "" {
		p.Source = "internal"
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.IdempotencyKey == "" {
		p.IdempotencyKey = p.VariantID + ":" + p.AccountID + ":" + p.CreatedAt.Format(time.RFC3339Nano)
	}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO publications(id,variant_id,account_id,status,remote_id,remote_url,remote_cid,error_category,error_message,idempotency_key,source,published_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, p.VariantID, p.AccountID, p.Status, p.RemoteID, p.RemoteURL, p.RemoteCID, p.ErrorCategory, p.ErrorMessage, p.IdempotencyKey, p.Source, "", p.CreatedAt.Format(time.RFC3339Nano))
	return p, e
}
func (s *Store) UpdatePublication(ctx context.Context, p Publication) error {
	_, e := s.DB.ExecContext(ctx, `UPDATE publications SET status=?,remote_id=?,remote_url=?,remote_cid=?,error_category=?,error_message=?,published_at=? WHERE id=?`, p.Status, p.RemoteID, p.RemoteURL, p.RemoteCID, p.ErrorCategory, p.ErrorMessage, formatTime(p.PublishedAt), p.ID)
	return e
}

func (s *Store) GetPublication(ctx context.Context, pid string) (Publication, error) {
	var p Publication
	var published, created string
	err := s.DB.QueryRowContext(ctx, `SELECT id,variant_id,account_id,status,remote_id,remote_url,remote_cid,error_category,error_message,idempotency_key,source,published_at,created_at FROM publications WHERE id=?`, pid).Scan(&p.ID, &p.VariantID, &p.AccountID, &p.Status, &p.RemoteID, &p.RemoteURL, &p.RemoteCID, &p.ErrorCategory, &p.ErrorMessage, &p.IdempotencyKey, &p.Source, &published, &created)
	if err == sql.ErrNoRows {
		err = ErrNotFound
	}
	p.PublishedAt = parseTime(published)
	p.CreatedAt = parseTime(created)
	return p, err
}
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
func (s *Store) ListPublications(ctx context.Context) ([]Publication, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,variant_id,account_id,status,remote_id,remote_url,remote_cid,error_category,error_message,idempotency_key,source,published_at,created_at FROM publications ORDER BY created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]Publication, 0)
	for rows.Next() {
		var p Publication
		var pub, c string
		if e = rows.Scan(&p.ID, &p.VariantID, &p.AccountID, &p.Status, &p.RemoteID, &p.RemoteURL, &p.RemoteCID, &p.ErrorCategory, &p.ErrorMessage, &p.IdempotencyKey, &p.Source, &pub, &c); e != nil {
			return nil, e
		}
		p.PublishedAt = parseTime(pub)
		p.CreatedAt = parseTime(c)
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) CreateExternalPost(ctx context.Context, p ExternalPost) (ExternalPost, error) {
	topic, e := s.GetTopic(ctx, p.TopicID)
	if e != nil {
		return p, e
	}
	if strings.TrimSpace(p.Platform) == "" || (strings.TrimSpace(p.URL) == "" && strings.TrimSpace(p.RemoteID) == "") {
		return p, fmt.Errorf("platform and URL or remote ID are required")
	}
	if p.AccountID != "" {
		account, accountErr := s.GetAccount(ctx, p.AccountID)
		if accountErr != nil {
			return p, accountErr
		}
		if account.ProfileID != topic.ProfileID {
			return p, fmt.Errorf("external post account must belong to the topic profile")
		}
	}
	if p.ID == "" {
		p.ID = id()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.MediaSnapshot == nil {
		p.MediaSnapshot = []ExternalMedia{}
	}
	if p.Incomplete && strings.TrimSpace(p.IncompleteReason) == "" {
		p.IncompleteReason = "snapshot is incomplete"
	}
	mediaJSON, marshalErr := json.Marshal(p.MediaSnapshot)
	if marshalErr != nil {
		return p, marshalErr
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO external_posts(id,topic_id,account_id,platform,remote_id,url,text_snapshot,media_snapshot,captured_by,incomplete,incomplete_reason,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, p.TopicID, nullString(p.AccountID), p.Platform, p.RemoteID, p.URL, p.TextSnapshot, string(mediaJSON), p.CapturedBy, boolInt(p.Incomplete), p.IncompleteReason, p.CreatedAt.Format(time.RFC3339Nano))
	return p, e
}
func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (s *Store) ListExternalPosts(ctx context.Context, topicID string) ([]ExternalPost, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,topic_id,COALESCE(account_id,''),platform,remote_id,url,text_snapshot,media_snapshot,captured_by,incomplete,incomplete_reason,created_at FROM external_posts WHERE topic_id=? ORDER BY created_at DESC`, topicID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]ExternalPost, 0)
	for rows.Next() {
		var p ExternalPost
		var t, mediaJSON string
		var incomplete int
		if e = rows.Scan(&p.ID, &p.TopicID, &p.AccountID, &p.Platform, &p.RemoteID, &p.URL, &p.TextSnapshot, &mediaJSON, &p.CapturedBy, &incomplete, &p.IncompleteReason, &t); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(mediaJSON), &p.MediaSnapshot); e != nil {
			return nil, e
		}
		p.Incomplete = incomplete != 0
		p.CreatedAt = parseTime(t)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreateTranslationConfig(ctx context.Context, c TranslationConfig) (TranslationConfig, error) {
	if c.ID == "" {
		c.ID = id()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	switch c.Provider {
	case "google", "google-cloud", "openai", "openai-compatible":
		c.CredentialRef = strings.TrimSpace(c.CredentialRef)
		c.TokenEnv = strings.TrimSpace(c.TokenEnv)
		c.KeyEnv = ""
		if c.CredentialRef == "" && !credentialEnvName.MatchString(c.TokenEnv) {
			return c, fmt.Errorf("token_env must be an environment variable name, not a credential value")
		}
		if c.CredentialRef != "" {
			c.TokenEnv = ""
		}
	case "deepl":
		c.CredentialRef = strings.TrimSpace(c.CredentialRef)
		c.KeyEnv = strings.TrimSpace(c.KeyEnv)
		c.TokenEnv = ""
		if c.CredentialRef == "" && !credentialEnvName.MatchString(c.KeyEnv) {
			return c, fmt.Errorf("key_env must be an environment variable name, not a credential value")
		}
		if c.CredentialRef != "" {
			c.KeyEnv = ""
		}
	case "google-gtx":
		c.CredentialRef, c.TokenEnv, c.KeyEnv = "", "", ""
	default:
		return c, fmt.Errorf("unsupported translation provider %q", c.Provider)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO translation_configs(id,provider,base_url,project_id,location,token_env,key_env,credential_ref,model,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, c.ID, c.Provider, c.BaseURL, c.ProjectID, c.Location, c.TokenEnv, c.KeyEnv, c.CredentialRef, c.Model, c.CreatedAt.Format(time.RFC3339Nano))
	return c, err
}

func (s *Store) ListTranslationConfigs(ctx context.Context) ([]TranslationConfig, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,provider,base_url,project_id,location,token_env,key_env,credential_ref,model,created_at FROM translation_configs ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TranslationConfig, 0)
	for rows.Next() {
		var c TranslationConfig
		var created string
		if err := rows.Scan(&c.ID, &c.Provider, &c.BaseURL, &c.ProjectID, &c.Location, &c.TokenEnv, &c.KeyEnv, &c.CredentialRef, &c.Model, &created); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(created)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetTranslationConfig(ctx context.Context, id string) (TranslationConfig, error) {
	var c TranslationConfig
	var created string
	err := s.DB.QueryRowContext(ctx, `SELECT id,provider,base_url,project_id,location,token_env,key_env,credential_ref,model,created_at FROM translation_configs WHERE id=?`, id).Scan(&c.ID, &c.Provider, &c.BaseURL, &c.ProjectID, &c.Location, &c.TokenEnv, &c.KeyEnv, &c.CredentialRef, &c.Model, &created)
	if err == sql.ErrNoRows {
		err = ErrNotFound
	}
	c.CreatedAt = parseTime(created)
	return c, err
}

func (s *Store) CreateTranslatedVariant(ctx context.Context, source Variant, target Account, text, provider, sourceLanguage, targetLanguage string) (Variant, Translation, error) {
	if source.TopicID == "" || target.ID == "" || strings.TrimSpace(targetLanguage) == "" {
		return Variant{}, Translation{}, fmt.Errorf("source, target account and target language are required")
	}
	if !s.accountTopicOK(ctx, source.TopicID, target.ID) {
		return Variant{}, Translation{}, ErrNotFound
	}
	created := time.Now().UTC()
	v := Variant{ID: id(), TopicID: source.TopicID, AccountID: target.ID, Text: text, Format: source.Format, Status: "draft", Relation: "detached", CreatedAt: created, UpdatedAt: created}
	normalizeVariantForPlatform(&v, target.Platform)
	if target.Platform == "misskey" {
		v.Format = "mfm"
	}
	t := Translation{ID: id(), SourceVariantID: source.ID, TargetAccountID: target.ID, TargetVariantID: v.ID, SourceLanguage: sourceLanguage, TargetLanguage: targetLanguage, Provider: provider, Status: "succeeded", CreatedAt: created}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Variant{}, Translation{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO variants(id,topic_id,account_id,text,format,status,relation,source_revision,visibility,spoiler_text,local_only,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.TopicID, v.AccountID, v.Text, v.Format, v.Status, v.Relation, "", v.Visibility, v.SpoilerText, boolInt(v.LocalOnly), created.Format(time.RFC3339Nano), created.Format(time.RFC3339Nano)); err != nil {
		return Variant{}, Translation{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO translations(id,source_variant_id,target_account_id,target_variant_id,source_language,target_language,provider,status,error_category,error_message,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, t.ID, t.SourceVariantID, t.TargetAccountID, t.TargetVariantID, t.SourceLanguage, t.TargetLanguage, t.Provider, t.Status, "", "", created.Format(time.RFC3339Nano)); err != nil {
		return Variant{}, Translation{}, err
	}
	if err = tx.Commit(); err != nil {
		return Variant{}, Translation{}, err
	}
	return v, t, nil
}

func (s *Store) ListTranslations(ctx context.Context) ([]Translation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,source_variant_id,target_account_id,target_variant_id,source_language,target_language,provider,status,error_category,error_message,created_at FROM translations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Translation, 0)
	for rows.Next() {
		var t Translation
		var created string
		if err := rows.Scan(&t.ID, &t.SourceVariantID, &t.TargetAccountID, &t.TargetVariantID, &t.SourceLanguage, &t.TargetLanguage, &t.Provider, &t.Status, &t.ErrorCategory, &t.ErrorMessage, &created); err != nil {
			return nil, err
		}
		t.CreatedAt = parseTime(created)
		out = append(out, t)
	}
	return out, rows.Err()
}
