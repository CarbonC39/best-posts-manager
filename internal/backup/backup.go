// Package backup creates and restores offline, self-contained local backups.
package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

const formatVersion = 1
const currentSchemaVersion = 8

type manifest struct {
	FormatVersion int            `json:"format_version"`
	SchemaVersion int            `json:"schema_version"`
	Files         []manifestFile `json:"files"`
}
type manifestFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Create writes a consistent SQLite snapshot and all local media. It never
// reads environment variables, so credentials cannot enter the archive.
func Create(ctx context.Context, dataDir, archivePath string) error {
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	dbPath := filepath.Join(dataDir, "best-post-manager.db")
	if _, err = os.Stat(dbPath); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	tmp, err := os.CreateTemp(dataDir, ".bpm-backup-*.sqlite")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err = tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	if err = os.Remove(tmpPath); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	if _, err = db.ExecContext(ctx, `VACUUM INTO ?`, tmpPath); err != nil {
		return fmt.Errorf("snapshot database: %w", err)
	}
	schemaVersion := 0
	if err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVersion); err != nil {
		return errors.New("database is missing schema migration metadata")
	}

	files := []string{"best-post-manager.db"}
	mediaDir := filepath.Join(dataDir, "media")
	if err = filepath.WalkDir(mediaDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == mediaDir {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("media symlink is not allowed: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(mediaDir, path)
		if e != nil {
			return e
		}
		files = append(files, filepath.ToSlash(filepath.Join("media", rel)))
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(files)
	entries := make([]manifestFile, 0, len(files))
	for _, name := range files {
		path := tmpPath
		if name != "best-post-manager.db" {
			path = filepath.Join(mediaDir, filepath.FromSlash(strings.TrimPrefix(name, "media/")))
		}
		entry, err := checksumFile(path)
		if err != nil {
			return err
		}
		entry.Path = name
		entries = append(entries, entry)
	}
	meta, err := json.Marshal(manifest{FormatVersion: formatVersion, SchemaVersion: schemaVersion, Files: entries})
	if err != nil {
		return err
	}
	archivePath = mustAbs(archivePath)
	if err = os.MkdirAll(filepath.Dir(archivePath), 0o700); err != nil {
		return err
	}
	if _, statErr := os.Stat(archivePath); statErr == nil {
		return errors.New("backup archive already exists; refusing to overwrite it")
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	file, err := os.CreateTemp(filepath.Dir(archivePath), ".bpm-archive-*.zip")
	if err != nil {
		return err
	}
	archiveTemp := file.Name()
	defer os.Remove(archiveTemp)
	zw := zip.NewWriter(file)
	for _, entry := range entries {
		path := tmpPath
		if entry.Path != "best-post-manager.db" {
			path = filepath.Join(mediaDir, filepath.FromSlash(strings.TrimPrefix(entry.Path, "media/")))
		}
		if err = addFile(zw, entry.Path, path); err != nil {
			_ = zw.Close()
			_ = file.Close()
			return err
		}
	}
	if err = addBytes(zw, "manifest.json", meta); err != nil {
		_ = zw.Close()
		_ = file.Close()
		return err
	}
	if err = zw.Close(); err == nil {
		err = file.Close()
	} else {
		_ = file.Close()
	}
	if err != nil {
		return err
	}
	return os.Rename(archiveTemp, archivePath)
}

func mustAbs(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}

func checksumFile(path string) (manifestFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return manifestFile{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return manifestFile{}, err
	}
	return manifestFile{Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func addFile(zw *zip.Writer, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
func addBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err == nil {
		_, err = w.Write(data)
	}
	return err
}

// Restore verifies the whole archive in a staging directory before making the
// destination visible. Existing non-empty directories are always refused.
func Restore(ctx context.Context, archivePath, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(archivePath); err != nil {
		return err
	}
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(destination); statErr == nil {
		if !info.IsDir() {
			return errors.New("restore destination must be a directory")
		}
		entries, readErr := os.ReadDir(destination)
		if readErr != nil {
			return readErr
		}
		if len(entries) != 0 {
			return errors.New("restore destination must be empty; refusing to overwrite data")
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	files := make(map[string]*zip.File)
	var meta manifest
	for _, file := range r.File {
		if !safeArchivePath(file.Name) {
			return fmt.Errorf("unsafe archive path %q", file.Name)
		}
		if _, exists := files[file.Name]; exists {
			return fmt.Errorf("duplicate archive path %q", file.Name)
		}
		if file.Name == "manifest.json" {
			reader, readErr := file.Open()
			if readErr != nil {
				return readErr
			}
			data, readErr := io.ReadAll(io.LimitReader(reader, 1<<20))
			_ = reader.Close()
			if readErr != nil {
				return readErr
			}
			if readErr = json.Unmarshal(data, &meta); readErr != nil {
				return fmt.Errorf("invalid manifest: %w", readErr)
			}
			continue
		}
		files[file.Name] = file
	}
	if meta.FormatVersion != formatVersion || len(meta.Files) == 0 {
		return errors.New("unsupported or incomplete backup manifest")
	}
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".bpm-restore-*")
	if os.IsNotExist(err) {
		if mkdirErr := os.MkdirAll(filepath.Dir(destination), 0o700); mkdirErr != nil {
			return mkdirErr
		}
		stage, err = os.MkdirTemp(filepath.Dir(destination), ".bpm-restore-*")
	}
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	seen := make(map[string]bool)
	for _, expected := range meta.Files {
		if expected.Path != "best-post-manager.db" && !strings.HasPrefix(expected.Path, "media/") {
			return fmt.Errorf("manifest contains unsupported path %q", expected.Path)
		}
		file, ok := files[expected.Path]
		if !ok {
			return fmt.Errorf("manifest file missing: %s", expected.Path)
		}
		seen[expected.Path] = true
		outPath := filepath.Join(stage, filepath.FromSlash(expected.Path))
		if err = os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
			return err
		}
		reader, readErr := file.Open()
		if readErr != nil {
			return readErr
		}
		out, createErr := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if createErr == nil {
			_, createErr = io.Copy(out, io.LimitReader(reader, 64<<20))
			if closeErr := out.Close(); createErr == nil {
				createErr = closeErr
			}
		}
		_ = reader.Close()
		if createErr != nil {
			return createErr
		}
		got, hashErr := checksumFile(outPath)
		if hashErr != nil || got.Size != expected.Size || got.SHA256 != expected.SHA256 {
			return fmt.Errorf("checksum mismatch for %s", expected.Path)
		}
	}
	for name := range files {
		if !seen[name] {
			return fmt.Errorf("archive file is not listed in manifest: %s", name)
		}
	}
	if err = validateDatabase(filepath.Join(stage, "best-post-manager.db"), meta.SchemaVersion); err != nil {
		return err
	}
	if err = relocateMediaPaths(filepath.Join(stage, "best-post-manager.db"), filepath.Join(stage, "media"), filepath.Join(destination, "media")); err != nil {
		return err
	}
	if err = os.Chmod(stage, 0o700); err != nil {
		return err
	}
	if _, statErr := os.Stat(destination); statErr == nil {
		if err = os.Remove(destination); err != nil {
			return err
		}
	}
	return os.Rename(stage, destination)
}

func safeArchivePath(name string) bool {
	return name != "" && !strings.Contains(name, `\`) && !filepath.IsAbs(name) && filepath.Clean(name) == name && name != "." && !strings.HasPrefix(name, "../") && name != ".."
}

func validateDatabase(path string, schema int) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	var actual int
	if err = db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&actual); err != nil {
		return errors.New("backup database is not a Best Post Manager database")
	}
	if schema < 1 || schema > currentSchemaVersion || actual != schema {
		return errors.New("backup database schema version is inconsistent")
	}
	return nil
}

// Media paths are absolute in the live database. Restoring into another
// directory must rebase them after the archived database has been verified.
func relocateMediaPaths(dbPath, stagedMediaDir, destinationMediaDir string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id,path FROM media`)
	if err != nil {
		return err
	}
	type mediaPath struct{ id, path string }
	var paths []mediaPath
	for rows.Next() {
		var item mediaPath
		if err = rows.Scan(&item.id, &item.path); err != nil {
			_ = rows.Close()
			return err
		}
		paths = append(paths, item)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range paths {
		name := filepath.Base(item.path)
		if name == "." || name == string(filepath.Separator) {
			return fmt.Errorf("invalid media path for %s", item.id)
		}
		staged := filepath.Join(stagedMediaDir, name)
		if info, statErr := os.Stat(staged); statErr != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("backup media is missing for %s", item.id)
		}
		rebased := filepath.Join(destinationMediaDir, name)
		if _, err = tx.Exec(`UPDATE media SET path=? WHERE id=?`, rebased, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
