package ports

import "webserver/internal/domain"

// TaskStorage describes persistence operations required by services dealing with tasks.
type TaskStorage interface {
	Load() error
	CreateTask(links []string) (*domain.Task, error)
	UpdateTaskResult(id int, result map[string]string) error
	GetTasks(ids []int) ([]*domain.Task, error)
}
