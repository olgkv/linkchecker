package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	f, err := os.CreateTemp("", "tasks-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = f.Close()

	st := NewStorage(f.Name())
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return st
}

func TestStorageCreateAndGet(t *testing.T) {
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

func TestHandleLinks(t *testing.T) {
	st := newTestStorage(t)

	body, _ := json.Marshal(LinksRequest{Links: []string{"example.com"}})
	req := httptest.NewRequest(http.MethodPost, "/links", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h := handleLinks(st)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp LinksResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.LinksNum == 0 {
		t.Fatalf("expected non-zero links_num")
	}
	if len(resp.Links) != 1 {
		t.Fatalf("expected 1 link in response, got %d", len(resp.Links))
	}
}

func TestHandleReport(t *testing.T) {
	st := newTestStorage(t)

	// создаем задачу
	links := []string{"example.com"}
	task, err := st.CreateTask(links)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := st.UpdateTaskResult(task.ID, map[string]string{"example.com": string(StatusAvailable)}); err != nil {
		t.Fatalf("UpdateTaskResult: %v", err)
	}

	body, _ := json.Marshal(ReportRequest{LinksList: []int{task.ID}})
	req := httptest.NewRequest(http.MethodPost, "/report", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h := handleReport(st)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type = %q, want application/pdf", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatalf("empty pdf body")
	}
}

func TestCheckLinkWithInvalidURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	status := checkLink(ctx, "http://invalid.invalid")
	if status != StatusNotAvailable {
		t.Fatalf("status = %q, want %q", status, StatusNotAvailable)
	}
}
