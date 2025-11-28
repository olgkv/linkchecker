package storage

import (
	"os"
	"testing"
)

func newTestStorage(t *testing.T) *FileStorage {
	t.Helper()
	f, err := os.CreateTemp("", "tasks-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = f.Close()

	st := NewFileStorage(f.Name())
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return st
}

func TestFileStorageCreateAndGet(t *testing.T) {
	st := newTestStorage(t)

	links := []string{"google.com", "yandex.ru"}
	task, err := st.CreateTask(links)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	got, err := st.GetTasks([]int{task.ID})
	if err != nil {
		t.Fatalf("GetTasks: %v", err)
	}
	if len(got) != 1 || got[0].ID != task.ID {
		t.Fatalf("unexpected tasks: %#v", got)
	}
}
