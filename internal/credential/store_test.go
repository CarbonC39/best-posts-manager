package credential

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStorePersistsSecretsWithPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Set("ref", "secret"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get("ref"); err != nil || got != "secret" {
		t.Fatalf("get=%q err=%v", got, err)
	}
	info, err := os.Stat(filepath.Join(dir, "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	s2, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s2.Get("ref"); err != nil || got != "secret" {
		t.Fatalf("reopen get=%q err=%v", got, err)
	}
	if err = s2.Delete("ref"); err != nil {
		t.Fatal(err)
	}
	if _, err = s2.Get("ref"); err == nil {
		t.Fatal("deleted secret still available")
	}
}

func TestFileStoreRejectsUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path, []byte(`{"x":"y"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(dir); err == nil {
		t.Fatal("accepted world-readable credentials")
	}
	_ = os.Remove(path)
	if err := os.Symlink(filepath.Join(dir, "other"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(dir); err == nil {
		t.Fatal("accepted credentials symlink")
	}
}
