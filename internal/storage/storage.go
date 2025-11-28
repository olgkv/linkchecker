package storage

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"webserver/internal/domain"
	"webserver/internal/ports"
)

type TaskRepository interface {
	Load() ([]*LogEntry, error)
	Append(entry *LogEntry) error
}

type LogEntry struct {
	Op        string            `json:"op"`
	Task      *domain.Task      `json:"task,omitempty"`
	TaskID    int               `json:"task_id,omitempty"`
	Result    map[string]string `json:"result,omitempty"`
	Timestamp time.Time         `json:"ts"`
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

	entries, err := s.repo.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	s.tasks = make(map[int]*domain.Task)
	s.nextID = 1
	for _, entry := range entries {
		s.applyEntry(entry)
	}
	return nil
}

func (s *FileStorage) applyEntry(entry *LogEntry) {
	switch entry.Op {
	case "create":
		if entry.Task == nil {
			return
		}
		if entry.Task.ID >= s.nextID {
			s.nextID = entry.Task.ID + 1
		}
		s.tasks[entry.Task.ID] = &domain.Task{
			ID:     entry.Task.ID,
			Links:  append([]string(nil), entry.Task.Links...),
			Result: copyMap(entry.Task.Result),
		}
	case "update":
		if entry.TaskID == 0 {
			return
		}
		if t, ok := s.tasks[entry.TaskID]; ok {
			t.Result = copyMap(entry.Result)
		}
	}
}

func copyMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func taskToDTO(t *domain.Task) *ports.TaskDTO {
	if t == nil {
		return nil
	}
	return &ports.TaskDTO{
		ID:     t.ID,
		Links:  append([]string(nil), t.Links...),
		Result: copyMap(t.Result),
	}
}

func (s *FileStorage) CreateTask(links []string) (*ports.TaskDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++
	t := &domain.Task{ID: id, Links: links, Result: make(map[string]string)}
	s.tasks[id] = t
	if err := s.repo.Append(&LogEntry{Op: "create", Task: t, Timestamp: time.Now()}); err != nil {
		return nil, err
	}
	return taskToDTO(t), nil
}

func (s *FileStorage) UpdateTaskResult(id int, result map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %d not found", id)
	}
	t.Result = result
	return s.repo.Append(&LogEntry{Op: "update", TaskID: id, Result: result, Timestamp: time.Now()})
}

func (s *FileStorage) GetTasks(ids []int) ([]*ports.TaskDTO, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*ports.TaskDTO, 0, len(ids))
	for _, id := range ids {
		if t, ok := s.tasks[id]; ok {
			res = append(res, taskToDTO(t))
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
