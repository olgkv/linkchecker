package storage

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"webserver/internal/domain"
)

// JSONRepository persists tasks into a JSON file on disk.
type JSONRepository struct {
	path string
}

func NewJSONRepository(path string) *JSONRepository {
	return &JSONRepository{path: path}
}

func (r *JSONRepository) Load() ([]*domain.Task, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var tasks []*domain.Task
	if err := dec.Decode(&tasks); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	return tasks, nil
}

func (r *JSONRepository) Save(tasks []*domain.Task) error {
	tmp := r.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tasks); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}
