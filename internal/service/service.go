package service

import (
	"context"
	"net/http"
	"time"

	"webserver/internal/domain"
	pdfgen "webserver/internal/pdf"
)

type TaskStorage interface {
	Load() error
	CreateTask(links []string) (*domain.Task, error)
	UpdateTaskResult(id int, result map[string]string) error
	GetTasks(ids []int) ([]*domain.Task, error)
}

type Service struct {
	storage    TaskStorage
	httpClient *http.Client
}

func New(storage TaskStorage, client *http.Client) *Service {
	return &Service{storage: storage, httpClient: client}
}

func (s *Service) CheckLinks(ctx context.Context, links []string) (int, map[string]domain.LinkStatus, error) {
	task, err := s.storage.CreateTask(links)
	if err != nil {
		return 0, nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result := make(map[string]domain.LinkStatus, len(links))
	for _, link := range links {
		result[link] = s.checkLink(ctx, link)
	}

	strResult := make(map[string]string, len(result))
	for k, v := range result {
		strResult[k] = string(v)
	}
	_ = s.storage.UpdateTaskResult(task.ID, strResult)

	return task.ID, result, nil
}

func (s *Service) checkLink(ctx context.Context, link string) domain.LinkStatus {
	url := link
	if !(len(url) >= 7 && (url[:7] == "http://" || (len(url) >= 8 && url[:8] == "https://"))) {
		url = "https://" + link
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.StatusNotAvailable
	}

	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return domain.StatusNotAvailable
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return domain.StatusAvailable
	}
	return domain.StatusNotAvailable
}

func (s *Service) GenerateReport(ctx context.Context, ids []int) ([]byte, error) {
	tasks, err := s.storage.GetTasks(ids)
	if err != nil {
		return nil, err
	}
	return pdfgen.BuildLinksReport(tasks)
}
