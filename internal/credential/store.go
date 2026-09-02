// Package credential stores local secrets separately from SQLite.
package credential

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Store interface {
	Get(ref string) (string, error)
	Set(ref, secret string) error
	Delete(ref string) error
}

type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(dataDir string) (*FileStore, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(dataDir); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("credential directory has unsafe permissions or type")
	}
	path := filepath.Join(dataDir, "credentials.json")
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("credentials file has unsafe permissions or type")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return &FileStore{path: path}, nil
}

func NewRef() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "cred-" + hex.EncodeToString(b[:]), nil
}

func (s *FileStore) load() (map[string]string, error) {
	info, err := os.Lstat(s.path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("credentials file has unsafe permissions or type")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	if len(data) == 0 {
		return values, nil
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, errors.New("credentials file is invalid")
	}
	return values, nil
}
func (s *FileStore) save(values map[string]string) error {
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".credentials-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
func (s *FileStore) Get(ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.load()
	if err != nil {
		return "", err
	}
	secret, ok := values[ref]
	if !ok || secret == "" {
		return "", fmt.Errorf("credential %q is unavailable", ref)
	}
	return secret, nil
}
func (s *FileStore) Set(ref, secret string) error {
	if ref == "" || secret == "" {
		return errors.New("credential reference and secret are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.load()
	if err != nil {
		return err
	}
	values[ref] = secret
	return s.save(values)
}
func (s *FileStore) Delete(ref string) error {
	if ref == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.load()
	if err != nil {
		return err
	}
	delete(values, ref)
	return s.save(values)
}
