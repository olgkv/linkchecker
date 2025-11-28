package ports

// TaskDTO represents link-checking task data without depending on the domain layer.
type TaskDTO struct {
	ID     int
	Links  []string
	Result map[string]string
}

// TaskStorage describes persistence operations required by services dealing with tasks.
type TaskStorage interface {
	Load() error
	CreateTask(links []string) (*TaskDTO, error)
	UpdateTaskResult(id int, result map[string]string) error
	GetTasks(ids []int) ([]*TaskDTO, error)
}
