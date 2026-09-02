package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"best-post-manager/internal/app"
)

func TestCreateRestoreAndRefuseCorruptOrNonEmptyDestination(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	store, err := app.Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	profile, _ := store.CreateProfile(ctx, app.Profile{Name: "backup"})
	t.Setenv("BACKUP_TEST_TOKEN", "secret-must-not-enter-archive")
	if _, err = store.CreateTranslationConfig(ctx, app.TranslationConfig{Provider: "openai", TokenEnv: "BACKUP_TEST_TOKEN"}); err != nil {
		t.Fatal(err)
	}
	topic, _ := store.CreateTopic(ctx, app.Topic{ProfileID: profile.ID, Title: "topic"})
	mediaPath := filepath.Join(store.MediaDir, "local-image.png")
	if err = os.WriteFile(mediaPath, []byte("image bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddMedia(ctx, app.Media{TopicID: topic.ID, Filename: "image.png", MIME: "image/png", Path: mediaPath, Size: 11}); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	if err = Create(ctx, source, archivePath); err != nil {
		t.Fatal(err)
	}
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil || bytes.Contains(archiveBytes, []byte("secret-must-not-enter-archive")) {
		t.Fatalf("backup leaked credential value: %v", err)
	}
	if err = Create(ctx, source, archivePath); err == nil {
		t.Fatal("backup create must not overwrite an existing archive")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if err = Restore(ctx, archivePath, restored); err != nil {
		t.Fatal(err)
	}
	reopened, err := app.Open(ctx, restored)
	if err != nil {
		t.Fatal(err)
	}
	if profiles, e := reopened.ListProfiles(ctx); e != nil || len(profiles) != 1 || profiles[0].Name != "backup" {
		t.Fatalf("restored profile missing: %+v %v", profiles, e)
	}
	if media, e := reopened.ListMedia(ctx, topic.ID); e != nil || len(media) != 1 {
		t.Fatalf("restored media missing: %+v %v", media, e)
	} else if !strings.HasPrefix(media[0].Path, filepath.Join(restored, "media")+string(filepath.Separator)) {
		t.Fatalf("restored media path was not rebased: %s", media[0].Path)
	} else if contents, readErr := os.ReadFile(media[0].Path); readErr != nil || string(contents) != "image bytes" {
		t.Fatalf("restored media is unreadable: %q %v", contents, readErr)
	}
	_ = reopened.Close()

	nonEmpty := filepath.Join(t.TempDir(), "non-empty")
	if err = os.MkdirAll(nonEmpty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(nonEmpty, "keep"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = Restore(ctx, archivePath, nonEmpty); err == nil {
		t.Fatal("restore must refuse a non-empty destination")
	}

	corrupt := filepath.Join(t.TempDir(), "corrupt.zip")
	if err = corruptArchive(archivePath, corrupt); err != nil {
		t.Fatal(err)
	}
	if err = Restore(ctx, corrupt, filepath.Join(t.TempDir(), "bad")); err == nil {
		t.Fatal("restore must reject a checksum-corrupt archive")
	}
}

func corruptArchive(source, destination string) error {
	r, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := os.Create(destination)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for _, entry := range r.File {
		reader, readErr := entry.Open()
		if readErr != nil {
			_ = zw.Close()
			_ = f.Close()
			return readErr
		}
		contents, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			return readErr
		}
		if entry.Name == "media/local-image.png" {
			contents[0] ^= 0xff
		}
		w, writeErr := zw.Create(entry.Name)
		if writeErr != nil {
			return writeErr
		}
		if _, writeErr = w.Write(contents); writeErr != nil {
			return writeErr
		}
	}
	if err = zw.Close(); err == nil {
		err = f.Close()
	} else {
		_ = f.Close()
	}
	return err
}
