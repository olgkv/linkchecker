package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"webserver/internal/domain"
)

type FileStorage struct {
	mu       sync.RWMutex
	filePath string
	nextID   int
	tasks    map[int]*domain.Task
}

func NewFileStorage(path string) *FileStorage {
	return &FileStorage{
		filePath: path,
		nextID:   1,
		tasks:    make(map[int]*domain.Task),
	}
}

func (s *FileStorage) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var tasks []*domain.Task
	if err := dec.Decode(&tasks); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	maxID := 0
	for _, t := range tasks {
		if t.ID > maxID {
			maxID = t.ID
		}
		s.tasks[t.ID] = t
	}
	s.nextID = maxID + 1
	return nil
}

func (s *FileStorage) persistLocked() error {
	tmp := s.filePath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	var list []*domain.Task
	for _, t := range s.tasks {
		list = append(list, t)
	}
	if err := enc.Encode(list); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

func (s *FileStorage) CreateTask(links []string) (*domain.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++
	t := &domain.Task{ID: id, Links: links, Result: make(map[string]string)}
	s.tasks[id] = t
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *FileStorage) UpdateTaskResult(id int, result map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %d not found", id)
	}
	t.Result = result
	return s.persistLocked()
}

func (s *FileStorage) GetTasks(ids []int) ([]*domain.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*domain.Task, 0, len(ids))
	for _, id := range ids {
		if t, ok := s.tasks[id]; ok {
			res = append(res, t)
		}
	}
	return res, nil
}

// Stats возвращает количество всех задач и количество задач, у которых заполнен результат.
func (s *FileStorage) Stats() (total int, completed int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, t := range s.tasks {
		total++
		if len(t.Result) > 0 {
			completed++
		}
	}
	return total, completed
}
