package service

import (
	"errors"
	"testing"
	"time"

	"webserver/internal/ports"
)

type mockTaskStorage struct {
	updateCalls int
	updateFunc  func(call int) error
}

func TestRetryUpdateTaskResult_SingleAttemptNoSleep(t *testing.T) {
	m := &mockTaskStorage{}
	svc := &Service{storage: m}
	originalSleep := sleep
	defer func() { sleep = originalSleep }()
	sleepCalled := false
	sleep = func(d time.Duration) { sleepCalled = true }

	svc.retryUpdateTaskResult(7, map[string]string{"ok": "1"})

	if m.updateCalls != 1 {
		t.Fatalf("expected single update attempt, got %d", m.updateCalls)
	}
	if sleepCalled {
		t.Fatalf("sleep should not be invoked when first attempt succeeds")
	}
}

func (m *mockTaskStorage) Load() error { return nil }

func (m *mockTaskStorage) CreateTask(links []string) (*ports.TaskDTO, error) {
	return &ports.TaskDTO{ID: 1, Links: links, Result: map[string]string{}}, nil
}

func (m *mockTaskStorage) UpdateTaskResult(id int, result map[string]string) error {
	m.updateCalls++
	if m.updateFunc != nil {
		return m.updateFunc(m.updateCalls)
	}
	return nil
}

func (m *mockTaskStorage) GetTasks(ids []int) ([]*ports.TaskDTO, error) { return nil, nil }

func TestRetryUpdateTaskResult_SucceedsAfterRetries(t *testing.T) {
	m := &mockTaskStorage{
		updateFunc: func(call int) error {
			if call < 3 {
				return errors.New("boom")
			}
			return nil
		},
	}
	svc := &Service{storage: m}
	originalSleep := sleep
	defer func() { sleep = originalSleep }()
	var slept []time.Duration
	sleep = func(d time.Duration) { slept = append(slept, d) }

	svc.retryUpdateTaskResult(42, map[string]string{"k": "v"})

	if m.updateCalls != 3 {
		t.Fatalf("expected 3 update attempts, got %d", m.updateCalls)
	}
	if len(slept) != 2 {
		t.Fatalf("expected 2 backoff sleeps, got %d", len(slept))
	}
	if slept[0] != time.Second || slept[1] != 2*time.Second {
		t.Fatalf("unexpected backoff sequence: %v", slept)
	}
}

func TestRetryUpdateTaskResult_GivesUpAfterMaxAttempts(t *testing.T) {
	m := &mockTaskStorage{
		updateFunc: func(call int) error {
			return errors.New("still failing")
		},
	}
	svc := &Service{storage: m}
	originalSleep := sleep
	defer func() { sleep = originalSleep }()
	var sleepCount int
	sleep = func(d time.Duration) { sleepCount++ }

	svc.retryUpdateTaskResult(100, map[string]string{"k": "v"})

	if m.updateCalls != resultRetryAttempts {
		t.Fatalf("expected %d attempts, got %d", resultRetryAttempts, m.updateCalls)
	}
	if sleepCount != resultRetryAttempts {
		t.Fatalf("expected %d sleeps, got %d", resultRetryAttempts, sleepCount)
	}
}
