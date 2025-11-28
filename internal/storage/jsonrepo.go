package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxLogFileSize   = 100 << 20 // 100MB
	logRetentionDays = 7
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
	if err := r.maybeRotate(); err != nil {
		return err
	}
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

func (r *JSONRepository) maybeRotate() error {
	info, err := os.Stat(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Size() < maxLogFileSize {
		return nil
	}
	return r.Rotate()
}

// Rotate renames the current log file to a timestamped filename in the same directory.
func (r *JSONRepository) Rotate() error {
	if _, err := os.Stat(r.path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	base := filepath.Base(r.path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		name = base
	}
	rotated := fmt.Sprintf("%s-%s%s", name, time.Now().Format("2006-01-02"), ext)
	if err := os.Rename(r.path, filepath.Join(filepath.Dir(r.path), rotated)); err != nil {
		return err
	}
	return cleanupOldLogs(filepath.Dir(r.path), logRetentionDays)
}

func cleanupOldLogs(dir string, keepDays int) error {
	if keepDays <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.Contains(name, ".json-") || filepath.Ext(name) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
