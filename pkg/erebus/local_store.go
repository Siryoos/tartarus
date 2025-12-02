package erebus

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

type LocalStore struct {
	BasePath string
}

func NewLocalStore(basePath string) (*LocalStore, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, err
	}
	return &LocalStore{BasePath: basePath}, nil
}

func (s *LocalStore) Put(ctx context.Context, key string, r io.Reader) error {
	path := filepath.Join(s.BasePath, key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Write to a temp file first
	tmpFile, err := os.CreateTemp(filepath.Dir(path), "tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name()) // Clean up if we fail before rename
	defer tmpFile.Close()

	if _, err = io.Copy(tmpFile, r); err != nil {
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tmpFile.Name(), path)
}

func (s *LocalStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(s.BasePath, key)
	return os.Open(path)
}

func (s *LocalStore) Exists(ctx context.Context, key string) (bool, error) {
	path := filepath.Join(s.BasePath, key)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	path := filepath.Join(s.BasePath, key)
	return os.Remove(path)
}
