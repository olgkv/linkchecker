package storage

import (
	"encoding/json"
	"errors"
	"io"
	"os"
)

// JSONRepository stores log entries in a newline-delimited JSON file.
type JSONRepository struct {
	path string
}

func NewJSONRepository(path string) *JSONRepository {
	return &JSONRepository{path: path}
}

func (r *JSONRepository) Load() ([]*LogEntry, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var entries []*LogEntry
	for {
		var entry LogEntry
		if err := dec.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		entryCopy := entry
		entries = append(entries, &entryCopy)
	}
	return entries, nil
}

func (r *JSONRepository) Append(entry *LogEntry) error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(entry); err != nil {
		return err
	}
	return f.Sync()
}
