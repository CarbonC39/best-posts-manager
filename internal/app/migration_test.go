package app

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"
	"path/filepath"
	"testing"
)

func TestV1DatabaseMigratesToV4WithoutDataLoss(t *testing.T) {
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
	if err = st.DB.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 4 {
		t.Fatalf("migration version=%d err=%v", version, err)
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
}
