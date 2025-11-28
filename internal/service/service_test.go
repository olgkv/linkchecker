package service

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"webserver/internal/domain"
	"webserver/internal/ports"
)

type integrationStorageMock struct {
	taskID      int
	createCalls int
	updateCalls int
	lastResult  map[string]string
}

func (m *integrationStorageMock) Load() error { return nil }

func (m *integrationStorageMock) CreateTask(links []string) (*ports.TaskDTO, error) {
	m.createCalls++
	copied := append([]string(nil), links...)
	return &ports.TaskDTO{ID: m.taskID, Links: copied, Result: map[string]string{}}, nil
}

func (m *integrationStorageMock) UpdateTaskResult(id int, result map[string]string) error {
	m.updateCalls++
	m.lastResult = domain.CopyStringMap(result)
	return nil
}

func (m *integrationStorageMock) GetTasks(ids []int) ([]*ports.TaskDTO, error) { return nil, nil }

type httpClientMock struct {
	calls   []string
	codes   map[string]int
}

func (m *httpClientMock) Do(req *http.Request) (*http.Response, error) {
	if m.codes == nil {
		m.codes = map[string]int{}
	}
	url := req.URL.String()
	m.calls = append(m.calls, url)
	status := m.codes[url]
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

func TestService_CheckLinks_Success(t *testing.T) {
	storage := &integrationStorageMock{taskID: 101}
	client := &httpClientMock{codes: map[string]int{
		"https://example.com": http.StatusOK,
		"https://go.dev":     http.StatusOK,
	}}

	svc := &Service{
		storage:     storage,
		httpClient:  client,
		maxWorkers:  4,
		httpTimeout: 2 * time.Second,
	}

	links := []string{"example.com", "go.dev"}
	id, result, err := svc.CheckLinks(context.Background(), links)
	if err != nil {
		t.Fatalf("CheckLinks returned error: %v", err)
	}

	if id != 101 {
		t.Fatalf("expected task ID 101, got %d", id)
	}

	if len(result) != len(links) {
		t.Fatalf("expected %d results, got %d", len(links), len(result))
	}

	for _, link := range links {
		if result[link] != domain.StatusAvailable {
			t.Fatalf("expected %s to be available, got %s", link, result[link])
		}
	}

	if storage.createCalls != 1 {
		t.Fatalf("expected CreateTask called once, got %d", storage.createCalls)
	}

	if storage.updateCalls != 1 {
		t.Fatalf("expected UpdateTaskResult called once, got %d", storage.updateCalls)
	}

	expectedResult := map[string]string{
		"example.com": string(domain.StatusAvailable),
		"go.dev":     string(domain.StatusAvailable),
	}

	if !reflect.DeepEqual(storage.lastResult, expectedResult) {
		t.Fatalf("unexpected stored result: %#v", storage.lastResult)
	}

	if len(client.calls) != len(links) {
		t.Fatalf("expected %d HTTP calls, got %d", len(links), len(client.calls))
	}
}
