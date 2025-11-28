package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/olgkv/linkchecker/internal/ports"
	"github.com/olgkv/linkchecker/internal/service"
)

type stubStorage struct {
	ports.TaskStorage
	created       *ports.TaskDTO
	storedResults map[int]map[string]string
}

func (s *stubStorage) CreateTask(links []string) (*ports.TaskDTO, error) {
	if s.storedResults == nil {
		s.storedResults = make(map[int]map[string]string)
	}
	t := &ports.TaskDTO{ID: 1, Links: links, Result: make(map[string]string)}
	s.created = t
	return t, nil
}

func (s *stubStorage) UpdateTaskResult(id int, result map[string]string) error {
	if s.storedResults == nil {
		s.storedResults = make(map[int]map[string]string)
	}
	s.storedResults[id] = result
	return nil
}

func (s *stubStorage) GetTasks(ids []int) ([]*ports.TaskDTO, error) {
	if s.created == nil {
		return nil, nil
	}
	return []*ports.TaskDTO{s.created}, nil
}

// минимальный http.Client, чтобы не ходить в сеть в тестах

type dummyRoundTripper struct{}

func (d dummyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// всегда возвращаем 503, чтобы статус был not available
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	st := &stubStorage{}
	client := &http.Client{Transport: dummyRoundTripper{}}
	svc := service.New(st, client, 10, time.Second, 2)
	return NewHandler(svc, 5)
}

func TestLinksHandler(t *testing.T) {
	h := newTestHandler(t)

	tests := []struct {
		name      string
		links     []string
		wantCount int
	}{
		{"single", []string{"example.com"}, 1},
		{"multiple", []string{"example.com", "google.com"}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(LinksRequest{Links: tc.links})
			req := httptest.NewRequest(http.MethodPost, "/links", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			h.Links(rec, req)

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
			if len(resp.Links) != tc.wantCount {
				t.Fatalf("expected %d links in response, got %d", tc.wantCount, len(resp.Links))
			}
		})
	}
}

func TestReportHandler(t *testing.T) {
	h := newTestHandler(t)

	// подготовим задачу в заглушке через вызов CheckLinks
	bodyLinks, _ := json.Marshal(LinksRequest{Links: []string{"example.com"}})
	reqLinks := httptest.NewRequest(http.MethodPost, "/links", bytes.NewReader(bodyLinks))
	recLinks := httptest.NewRecorder()
	h.Links(recLinks, reqLinks)

	var lr LinksResponse
	if err := json.NewDecoder(recLinks.Body).Decode(&lr); err != nil {
		t.Fatalf("decode links resp: %v", err)
	}

	body, _ := json.Marshal(ReportRequest{LinksList: []int{lr.LinksNum}})
	req := httptest.NewRequest(http.MethodPost, "/report", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Report(rec, req)

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
