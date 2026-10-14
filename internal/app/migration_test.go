package app

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"
	"path/filepath"
	"testing"
)

func TestV1DatabaseMigratesToV10WithoutDataLoss(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY);INSERT INTO schema_migrations VALUES(1);CREATE TABLE variants(id TEXT PRIMARY KEY,topic_id TEXT,account_id TEXT,text TEXT,format TEXT,status TEXT,relation TEXT,source_revision TEXT,visibility TEXT,spoiler_text TEXT,created_at TEXT,updated_at TEXT);INSERT INTO variants VALUES('v1','t1','a1','old','plain','draft','linked','1','public','','now','now');CREATE TABLE external_posts(id TEXT PRIMARY KEY,topic_id TEXT,account_id TEXT,platform TEXT,remote_id TEXT,url TEXT,text_snapshot TEXT,captured_by TEXT,created_at TEXT);INSERT INTO external_posts VALUES('e1','t1','','x','','https://x.com/1','old','manual','now');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	st, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var local int
	if err = st.DB.QueryRow(`SELECT local_only FROM variants WHERE id='v1'`).Scan(&local); err != nil {
		t.Fatal(err)
	}
	if local != 0 {
		t.Fatalf("unexpected migrated local_only: %d", local)
	}
	var version int
	if err = st.DB.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 10 {
		t.Fatalf("migration version=%d err=%v", version, err)
	}
	var pairings int
	if err = st.DB.QueryRow(`SELECT count(*) FROM bridge_pairings`).Scan(&pairings); err != nil || pairings != 0 {
		t.Fatalf("bridge pairing migration failed: %d %v", pairings, err)
	}
	var cid string
	if err = st.DB.QueryRow(`SELECT remote_cid FROM publications LIMIT 1`).Scan(&cid); err != sql.ErrNoRows {
		// The old fixture has no publication rows; this query must be valid after
		// the migration (ErrNoRows proves the column exists).
		if err != nil {
			t.Fatalf("remote_cid column was not migrated: %v", err)
		}
	}
	var mediaJSON string
	if err = st.DB.QueryRow(`SELECT media_snapshot FROM external_posts WHERE id='e1'`).Scan(&mediaJSON); err != nil || mediaJSON != "[]" {
		t.Fatalf("external snapshot migration failed: %q %v", mediaJSON, err)
	}
	var credentialRef string
	if err = st.DB.QueryRow(`SELECT credential_ref FROM accounts LIMIT 1`).Scan(&credentialRef); err != sql.ErrNoRows {
		if err != nil {
			t.Fatalf("credential_ref column was not migrated: %v", err)
		}
	}
	var links int
	if err = st.DB.QueryRow(`SELECT count(*) FROM profile_accounts`).Scan(&links); err != nil || links != 0 {
		t.Fatalf("profile account migration failed: %d %v", links, err)
	}
}

func TestV9DatabaseMigratesBridgeRequests(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(9);`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var version, tables int
	if err = store.DB.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 10 {
		t.Fatalf("migration version=%d err=%v", version, err)
	}
	if err = store.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='bridge_requests'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("bridge request migration failed: %d %v", tables, err)
	}
}

func TestV5MigrationFailureRollsBackVersion(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(4); CREATE TRIGGER block_v5 BEFORE INSERT ON schema_migrations WHEN NEW.version=5 BEGIN SELECT RAISE(ABORT,'block v5'); END; CREATE TABLE profiles(id TEXT PRIMARY KEY, name TEXT NOT NULL, default_language TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL); INSERT INTO profiles VALUES('p','preserved','','','now');`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(context.Background(), dir); err == nil {
		t.Fatal("migration should fail when a conflicting object blocks v5")
	}
	check, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version int
	if err = check.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 4 {
		t.Fatalf("failed migration advanced version: %d %v", version, err)
	}
	var name string
	if err = check.QueryRow(`SELECT name FROM profiles WHERE id='p'`).Scan(&name); err != nil || name != "preserved" {
		t.Fatalf("failed migration damaged existing data: %q %v", name, err)
	}
	var tables int
	if err = check.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('translation_configs','translations')`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("failed migration left partial tables: count=%d err=%v", tables, err)
	}
}

func TestV6MigrationFailureRollsBackCredentialColumns(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(5); CREATE TABLE profiles(id TEXT PRIMARY KEY, name TEXT NOT NULL, default_language TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL); CREATE TABLE accounts(id TEXT PRIMARY KEY, profile_id TEXT, platform TEXT, name TEXT, remote_id TEXT, base_url TEXT, token_env TEXT, created_at TEXT); CREATE TABLE translation_configs(id TEXT PRIMARY KEY, provider TEXT, base_url TEXT, project_id TEXT, location TEXT, token_env TEXT, key_env TEXT, model TEXT, created_at TEXT); CREATE TRIGGER block_v6 BEFORE INSERT ON schema_migrations WHEN NEW.version=6 BEGIN SELECT RAISE(ABORT,'block v6'); END; INSERT INTO profiles VALUES('p','preserved','','','now');`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	if _, err = Open(context.Background(), dir); err == nil {
		t.Fatal("migration should fail")
	}
	check, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version int
	if err = check.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 5 {
		t.Fatalf("version=%d err=%v", version, err)
	}
	var columns int
	if err = check.QueryRow(`SELECT count(*) FROM pragma_table_info('accounts') WHERE name='credential_ref'`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("partial credential migration: %d %v", columns, err)
	}
}

func TestV7MigrationFailureRollsBackDisabledColumn(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(6); CREATE TABLE accounts(id TEXT PRIMARY KEY, profile_id TEXT, platform TEXT, name TEXT, remote_id TEXT, base_url TEXT, token_env TEXT, credential_ref TEXT, created_at TEXT); CREATE TRIGGER block_v7 BEFORE INSERT ON schema_migrations WHEN NEW.version=7 BEGIN SELECT RAISE(ABORT,'block v7'); END;`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(context.Background(), dir); err == nil {
		t.Fatal("migration should fail")
	}
	check, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version, columns int
	if err = check.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 6 {
		t.Fatalf("version=%d err=%v", version, err)
	}
	if err = check.QueryRow(`SELECT count(*) FROM pragma_table_info('accounts') WHERE name='disabled'`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("partial account deactivation migration: %d %v", columns, err)
	}
}

func TestV8MigrationFailureRollsBackBridgeTable(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(7); CREATE TRIGGER block_v8 BEFORE INSERT ON schema_migrations WHEN NEW.version=8 BEGIN SELECT RAISE(ABORT,'block v8'); END;`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(context.Background(), dir); err == nil {
		t.Fatal("migration should fail")
	}
	check, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version, tables int
	if err = check.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("version=%d err=%v", version, err)
	}
	if err = check.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='bridge_pairings'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("partial bridge migration: %d %v", tables, err)
	}
}

func TestV8DatabaseSeedsProfileAccountLinks(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "best-post-manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY); INSERT INTO schema_migrations VALUES(8); CREATE TABLE profiles(id TEXT PRIMARY KEY, name TEXT NOT NULL, default_language TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL); CREATE TABLE accounts(id TEXT PRIMARY KEY, profile_id TEXT NOT NULL REFERENCES profiles(id), platform TEXT NOT NULL, name TEXT NOT NULL, remote_id TEXT NOT NULL DEFAULT '', base_url TEXT NOT NULL DEFAULT '', token_env TEXT NOT NULL DEFAULT '', credential_ref TEXT NOT NULL DEFAULT '', disabled INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL); INSERT INTO profiles VALUES('p1','one','','','now'); INSERT INTO accounts VALUES('a1','p1','mastodon','shared','','','','',0,'now');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var profileID string
	if err = store.DB.QueryRow(`SELECT profile_id FROM profile_accounts WHERE account_id='a1'`).Scan(&profileID); err != nil || profileID != "p1" {
		t.Fatalf("seeded profile link=%q err=%v", profileID, err)
	}
}
