package storage

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"webserver/internal/domain"
)

type TaskRepository interface {
	Load() ([]*domain.Task, error)
	Save(tasks []*domain.Task) error
}

type FileStorage struct {
	mu     sync.RWMutex
	repo   TaskRepository
	nextID int
	tasks  map[int]*domain.Task
}

func NewFileStorage(repo TaskRepository) *FileStorage {
	return &FileStorage{
		repo:   repo,
		nextID: 1,
		tasks:  make(map[int]*domain.Task),
	}
}

func (s *FileStorage) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.repo.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	maxID := 0
	for _, t := range list {
		if t.ID > maxID {
			maxID = t.ID
		}
		s.tasks[t.ID] = t
	}
	s.nextID = maxID + 1
	return nil
}

func (s *FileStorage) persistLocked() error {
	var list []*domain.Task
	for _, t := range s.tasks {
		list = append(list, t)
	}
	return s.repo.Save(list)
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
