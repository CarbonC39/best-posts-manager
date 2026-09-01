package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

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
CREATE TABLE IF NOT EXISTS variants (id TEXT PRIMARY KEY, topic_id TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE, account_id TEXT NOT NULL REFERENCES accounts(id), text TEXT NOT NULL DEFAULT '', format TEXT NOT NULL DEFAULT 'plain', status TEXT NOT NULL DEFAULT 'draft', relation TEXT NOT NULL DEFAULT 'detached', source_revision TEXT NOT NULL DEFAULT '', visibility TEXT NOT NULL DEFAULT 'public', spoiler_text TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS media (id TEXT PRIMARY KEY, topic_id TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE, variant_id TEXT REFERENCES variants(id) ON DELETE SET NULL, filename TEXT NOT NULL, mime TEXT NOT NULL, path TEXT NOT NULL, alt_text TEXT NOT NULL DEFAULT '', size INTEGER NOT NULL DEFAULT 0, width INTEGER NOT NULL DEFAULT 0, height INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS publications (id TEXT PRIMARY KEY, variant_id TEXT NOT NULL REFERENCES variants(id), account_id TEXT NOT NULL REFERENCES accounts(id), status TEXT NOT NULL, remote_id TEXT NOT NULL DEFAULT '', remote_url TEXT NOT NULL DEFAULT '', error_category TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '', idempotency_key TEXT NOT NULL UNIQUE, source TEXT NOT NULL DEFAULT 'internal', published_at TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS external_posts (id TEXT PRIMARY KEY, topic_id TEXT NOT NULL REFERENCES topics(id) ON DELETE CASCADE, account_id TEXT REFERENCES accounts(id), platform TEXT NOT NULL, remote_id TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '', text_snapshot TEXT NOT NULL DEFAULT '', captured_by TEXT NOT NULL DEFAULT 'manual', created_at TEXT NOT NULL);`)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version) VALUES(1)`)
	return err
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
	var out []Profile
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
	if a.Name == "" || (a.Platform != "mastodon" && a.Platform != "mastodon-compatible") {
		return a, fmt.Errorf("a name and Mastodon-compatible platform are required")
	}
	if a.ID == "" {
		a.ID = id()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO accounts(id,profile_id,platform,name,remote_id,base_url,token_env,created_at) VALUES(?,?,?,?,?,?,?,?)`, a.ID, a.ProfileID, a.Platform, a.Name, a.RemoteID, a.BaseURL, a.TokenEnv, a.CreatedAt.Format(time.RFC3339Nano))
	return a, e
}
func (s *Store) ListAccounts(ctx context.Context, profileID string) ([]Account, error) {
	q := `SELECT id,profile_id,platform,name,remote_id,base_url,token_env,created_at FROM accounts`
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
	var out []Account
	for rows.Next() {
		var a Account
		var t string
		if e = rows.Scan(&a.ID, &a.ProfileID, &a.Platform, &a.Name, &a.RemoteID, &a.BaseURL, &a.TokenEnv, &t); e != nil {
			return nil, e
		}
		a.CreatedAt = parseTime(t)
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) GetAccount(ctx context.Context, aid string) (Account, error) {
	var a Account
	var t string
	e := s.DB.QueryRowContext(ctx, `SELECT id,profile_id,platform,name,remote_id,base_url,token_env,created_at FROM accounts WHERE id=?`, aid).Scan(&a.ID, &a.ProfileID, &a.Platform, &a.Name, &a.RemoteID, &a.BaseURL, &a.TokenEnv, &t)
	if e == sql.ErrNoRows {
		e = ErrNotFound
	}
	a.CreatedAt = parseTime(t)
	return a, e
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
	var out []Topic
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
		v.Text, v.Format, v.Visibility, v.SpoilerText = source.Text, source.Format, source.Visibility, source.SpoilerText
		v.Relation = "detached"
	} else {
		b, e := s.GetBaseDraft(ctx, v.TopicID)
		if e != nil {
			return v, e
		}
		v.Text, v.Format, v.Relation, v.SourceRevision = b.Text, b.Format, "linked", strconv.Itoa(b.Revision)
	}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO variants(id,topic_id,account_id,text,format,status,relation,source_revision,visibility,spoiler_text,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.TopicID, v.AccountID, v.Text, v.Format, v.Status, v.Relation, v.SourceRevision, v.Visibility, v.SpoilerText, v.CreatedAt.Format(time.RFC3339Nano), v.UpdatedAt.Format(time.RFC3339Nano))
	return v, e
}
func (s *Store) getVariant(ctx context.Context, vid string) (Variant, error) {
	var v Variant
	var c, u string
	e := s.DB.QueryRowContext(ctx, `SELECT id,topic_id,account_id,text,format,status,relation,source_revision,visibility,spoiler_text,created_at,updated_at FROM variants WHERE id=?`, vid).Scan(&v.ID, &v.TopicID, &v.AccountID, &v.Text, &v.Format, &v.Status, &v.Relation, &v.SourceRevision, &v.Visibility, &v.SpoilerText, &c, &u)
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
	var ids []string
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
	var out []Variant
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
	if v.Visibility != "public" && v.Visibility != "unlisted" && v.Visibility != "private" && v.Visibility != "direct" {
		return v, fmt.Errorf("invalid visibility")
	}
	stamp := now()
	_, e = s.DB.ExecContext(ctx, `UPDATE variants SET text=?,format=?,status=?,relation=?,source_revision=?,visibility=?,spoiler_text=?,updated_at=? WHERE id=?`, v.Text, v.Format, v.Status, v.Relation, v.SourceRevision, v.Visibility, v.SpoilerText, stamp, v.ID)
	v.TopicID, v.AccountID, v.CreatedAt = old.TopicID, old.AccountID, old.CreatedAt
	v.UpdatedAt = parseTime(stamp)
	v.Outdated = false
	return v, e
}
func (s *Store) ApplyVariantUpdate(ctx context.Context, vid string) (Variant, error) {
	v, e := s.getVariant(ctx, vid)
	if e != nil {
		return v, e
	}
	b, e := s.GetBaseDraft(ctx, v.TopicID)
	if e != nil {
		return v, e
	}
	v.Text, v.Format, v.Relation, v.SourceRevision = b.Text, b.Format, "linked", strconv.Itoa(b.Revision)
	return s.updateLinked(ctx, v)
}
func (s *Store) updateLinked(ctx context.Context, v Variant) (Variant, error) {
	stamp := now()
	_, e := s.DB.ExecContext(ctx, `UPDATE variants SET text=?,format=?,relation='linked',source_revision=?,visibility=?,spoiler_text=?,updated_at=? WHERE id=?`, v.Text, v.Format, v.SourceRevision, v.Visibility, v.SpoilerText, stamp, v.ID)
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
	var out []Media
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
	_, e := s.DB.ExecContext(ctx, `INSERT INTO publications(id,variant_id,account_id,status,remote_id,remote_url,error_category,error_message,idempotency_key,source,published_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, p.VariantID, p.AccountID, p.Status, p.RemoteID, p.RemoteURL, p.ErrorCategory, p.ErrorMessage, p.IdempotencyKey, p.Source, "", p.CreatedAt.Format(time.RFC3339Nano))
	return p, e
}
func (s *Store) UpdatePublication(ctx context.Context, p Publication) error {
	_, e := s.DB.ExecContext(ctx, `UPDATE publications SET status=?,remote_id=?,remote_url=?,error_category=?,error_message=?,published_at=? WHERE id=?`, p.Status, p.RemoteID, p.RemoteURL, p.ErrorCategory, p.ErrorMessage, formatTime(p.PublishedAt), p.ID)
	return e
}

func (s *Store) GetPublication(ctx context.Context, pid string) (Publication, error) {
	var p Publication
	var published, created string
	err := s.DB.QueryRowContext(ctx, `SELECT id,variant_id,account_id,status,remote_id,remote_url,error_category,error_message,idempotency_key,source,published_at,created_at FROM publications WHERE id=?`, pid).Scan(&p.ID, &p.VariantID, &p.AccountID, &p.Status, &p.RemoteID, &p.RemoteURL, &p.ErrorCategory, &p.ErrorMessage, &p.IdempotencyKey, &p.Source, &published, &created)
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
	rows, e := s.DB.QueryContext(ctx, `SELECT id,variant_id,account_id,status,remote_id,remote_url,error_category,error_message,idempotency_key,source,published_at,created_at FROM publications ORDER BY created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Publication
	for rows.Next() {
		var p Publication
		var pub, c string
		if e = rows.Scan(&p.ID, &p.VariantID, &p.AccountID, &p.Status, &p.RemoteID, &p.RemoteURL, &p.ErrorCategory, &p.ErrorMessage, &p.IdempotencyKey, &p.Source, &pub, &c); e != nil {
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
	_, e = s.DB.ExecContext(ctx, `INSERT INTO external_posts(id,topic_id,account_id,platform,remote_id,url,text_snapshot,captured_by,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, p.ID, p.TopicID, nullString(p.AccountID), p.Platform, p.RemoteID, p.URL, p.TextSnapshot, p.CapturedBy, p.CreatedAt.Format(time.RFC3339Nano))
	return p, e
}
func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (s *Store) ListExternalPosts(ctx context.Context, topicID string) ([]ExternalPost, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,topic_id,COALESCE(account_id,''),platform,remote_id,url,text_snapshot,captured_by,created_at FROM external_posts WHERE topic_id=? ORDER BY created_at DESC`, topicID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []ExternalPost
	for rows.Next() {
		var p ExternalPost
		var t string
		if e = rows.Scan(&p.ID, &p.TopicID, &p.AccountID, &p.Platform, &p.RemoteID, &p.URL, &p.TextSnapshot, &p.CapturedBy, &t); e != nil {
			return nil, e
		}
		p.CreatedAt = parseTime(t)
		out = append(out, p)
	}
	return out, rows.Err()
}
