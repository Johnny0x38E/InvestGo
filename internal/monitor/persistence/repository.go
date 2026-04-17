package persistence

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Repository abstracts persisted state storage so Store logic does not depend on a specific backend format.
type Repository interface {
	Load(target any) (bool, error)
	Save(source any) error
	Path() string
}

// JSONRepository persists state as a single JSON document on disk.
type JSONRepository struct {
	path string
}

// NewJSONRepository returns the default file-based repository implementation.
func NewJSONRepository(path string) *JSONRepository {
	return &JSONRepository{path: path}
}

// Path returns the storage path used by the repository.
func (r *JSONRepository) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Load loads state from disk into target and reports whether an existing file was found.
func (r *JSONRepository) Load(target any) (bool, error) {
	if r == nil {
		return false, errors.New("state repository is not configured")
	}

	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return false, err
	}

	payload, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal(payload, target); err != nil {
		return false, err
	}
	return true, nil
}

// Save persists source to disk using a temporary file plus atomic replace.
func (r *JSONRepository) Save(source any) error {
	if r == nil {
		return errors.New("state repository is not configured")
	}

	payload, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return err
	}

	tempPath := r.path + ".tmp"
	if err := os.WriteFile(tempPath, payload, 0o644); err != nil {
		return err
	}

	return os.Rename(tempPath, r.path)
}
