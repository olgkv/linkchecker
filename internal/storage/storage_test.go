package storage

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func newTestStorage(t *testing.T) *FileStorage {
	t.Helper()
	f, err := os.CreateTemp("", "tasks-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = f.Close()

	repo := NewJSONRepository(f.Name())
	st := NewFileStorage(repo)
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

func TestFileStorage_ConcurrentAccess(t *testing.T) {
	st := newTestStorage(t)

	var wg sync.WaitGroup
	createCount := 20
	ids := make(chan int, createCount)

	for i := 0; i < createCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task, err := st.CreateTask([]string{fmt.Sprintf("example-%d.com", idx)})
			if err != nil {
				t.Errorf("CreateTask: %v", err)
				return
			}
			ids <- task.ID
		}(i)
	}

	for i := 0; i < createCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case id := <-ids:
				if err := st.UpdateTaskResult(id, map[string]string{"ok": "true"}); err != nil {
					t.Errorf("UpdateTaskResult: %v", err)
				}
			case <-time.After(time.Second):
				t.Errorf("timeout waiting for id")
			}
		}()
	}

	for i := 0; i < createCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.GetTasks([]int{})
			if err != nil {
				t.Errorf("GetTasks: %v", err)
			}
		}()
	}

	wg.Wait()
}
