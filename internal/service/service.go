package service

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	urlpkg "net/url"
	"strings"
	"sync"
	"time"

	"webserver/internal/domain"
	pdfgen "webserver/internal/pdf"
	"webserver/internal/ports"
)

var sleep = time.Sleep

type Service struct {
	storage     ports.TaskStorage
	httpClient  ports.HTTPClient
	maxWorkers  int
	httpTimeout time.Duration
	breaker     *circuitBreaker
	persistWG   sync.WaitGroup
	reportJobs  chan reportJob
	pdfBuilder  func([]*domain.Task) ([]byte, error)
}

var ErrResultPersistDeferred = errors.New("result persistence deferred")

const resultRetryAttempts = 5

func isPrivateIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10:
			return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return true
		case ip4[0] == 192 && ip4[1] == 168:
			return true
		case ip4[0] == 127:
			return true
		case ip4[0] == 169 && ip4[1] == 254:
			return true
		}
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func isPrivateHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return isPrivateIP(host)
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return true // fail-safe
	}
	if len(ips) == 0 {
		return true
	}
	// Проверяем, что ВСЕ адреса приватные
	for _, ip := range ips {
		if !isPrivateIP(ip.String()) {
			return false // публичный IP найден
		}
	}
	return true // все приватные
}

func New(storage ports.TaskStorage, client ports.HTTPClient, maxWorkers int, httpTimeout time.Duration, reportWorkers int) *Service {
	if maxWorkers <= 0 {
		maxWorkers = 100
	}
	if httpTimeout <= 0 {
		httpTimeout = 5 * time.Second
	}
	if reportWorkers <= 0 {
		reportWorkers = 2
	}

	s := &Service{
		storage:     storage,
		httpClient:  client,
		maxWorkers:  maxWorkers,
		httpTimeout: httpTimeout,
		breaker:     newCircuitBreaker(3, 30*time.Second),
		reportJobs:  make(chan reportJob, reportWorkers),
		pdfBuilder:  pdfgen.BuildLinksReport,
	}
	for i := 0; i < reportWorkers; i++ {
		go s.reportWorker()
	}
	return s
}

func (s *Service) CheckLinks(ctx context.Context, links []string) (int, map[string]domain.LinkStatus, error) {
	task, err := s.storage.CreateTask(links)
	if err != nil {
		return 0, nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.httpTimeout)
	defer cancel()

	result := make(map[string]domain.LinkStatus, len(links))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, s.maxWorkers)

	for _, link := range links {
		link := link
		wg.Add(1)
		go func(link string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				status := s.checkLink(ctx, link)
				mu.Lock()
				result[link] = status
				mu.Unlock()
			case <-ctx.Done():
				return
			}

		}(link)
	}

	wg.Wait()

	strResult := make(map[string]string, len(result))
	for k, v := range result {
		strResult[k] = string(v)
	}
	if err := s.storage.UpdateTaskResult(task.ID, strResult); err != nil {
		slog.Error("update task result failed", "task_id", task.ID, "err", err)
		s.persistWG.Add(1)
		go func(id int, res map[string]string) {
			defer s.persistWG.Done()
			s.retryUpdateTaskResult(id, res)
		}(task.ID, domain.CopyStringMap(strResult))
		return task.ID, result, ErrResultPersistDeferred
	}

	return task.ID, result, nil
}

func (s *Service) retryUpdateTaskResult(id int, result map[string]string) {
	backoff := time.Second
	var lastErr error
	for attempt := 1; attempt <= resultRetryAttempts; attempt++ {
		if err := s.storage.UpdateTaskResult(id, result); err == nil {
			if attempt > 1 {
				slog.Info("task result persisted after retries", "task_id", id, "attempt", attempt)
			}
			return
		} else {
			lastErr = err
			sleep(backoff)
			backoff *= 2
		}
	}
	slog.Error("giving up on persisting task result", "task_id", id, "attempts", resultRetryAttempts, "err", lastErr)
}

// Wait blocks until all deferred persistence retries finish.
func (s *Service) Wait() {
	s.persistWG.Wait()
}

func (s *Service) checkLink(ctx context.Context, link string) domain.LinkStatus {
	clean := strings.TrimSpace(link)
	if !validateURL(clean) {
		return domain.StatusNotAvailable
	}

	url := clean
	if !(len(url) >= 7 && (url[:7] == "http://" || (len(url) >= 8 && url[:8] == "https://"))) {
		url = "https://" + clean
	}
	parsed, err := urlpkg.Parse(url)
	if err != nil {
		return domain.StatusNotAvailable
	}
	host := parsed.Hostname()
	if isPrivateHost(host) {
		return domain.StatusNotAvailable
	}
	if s.breaker != nil && !s.breaker.allow(host) {
		return domain.StatusNotAvailable
	}

	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	// небольшой backoff-retry для временных сетевых сбоев
	backoffs := []time.Duration{100 * time.Millisecond, 300 * time.Millisecond, 900 * time.Millisecond}
	for i, d := range backoffs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return domain.StatusNotAvailable
		}

		resp, err := client.Do(req)
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			if s.breaker != nil {
				s.breaker.failure(host)
			}
			// если контекст отменен — дальше не ретраим
			select {
			case <-ctx.Done():
				return domain.StatusNotAvailable
			default:
			}
		} else {
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				if s.breaker != nil {
					s.breaker.success(host)
				}
				return domain.StatusAvailable
			}
			if s.breaker != nil {
				s.breaker.failure(host)
			}
		}

		// если это не последняя попытка — подождать backoff или выход, если контекст отменен
		if i < len(backoffs)-1 {
			select {
			case <-ctx.Done():
				return domain.StatusNotAvailable
			case <-time.After(d):
			}
		}
	}

	return domain.StatusNotAvailable
}

func (s *Service) GenerateReport(ctx context.Context, ids []int) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	job := reportJob{
		ctx:  ctx,
		ids:  ids,
		resp: make(chan reportResult, 1),
	}
	select {
	case s.reportJobs <- job:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case res := <-job.resp:
		return res.data, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func dtoToDomain(tasks []*ports.TaskDTO) []*domain.Task {
	res := make([]*domain.Task, 0, len(tasks))
	for _, t := range tasks {
		if t == nil {
			continue
		}
		res = append(res, &domain.Task{
			ID:     t.ID,
			Links:  append([]string(nil), t.Links...),
			Result: domain.CopyStringMap(t.Result),
		})
	}
	return res
}

func validateURL(link string) bool {
	if link == "" {
		return false
	}
	if strings.ContainsAny(link, "/?#:") {
		return false
	}
	return true
}

type reportJob struct {
	ctx  context.Context
	ids  []int
	resp chan reportResult
}

type reportResult struct {
	data []byte
	err  error
}

func (s *Service) reportWorker() {
	for job := range s.reportJobs {
		s.handleReportJob(job)
	}
}

func (s *Service) handleReportJob(job reportJob) {
	if err := job.ctx.Err(); err != nil {
		job.respond(nil, err)
		return
	}
	tasks, err := s.storage.GetTasks(job.ids)
	if err != nil {
		job.respond(nil, err)
		return
	}
	if err := job.ctx.Err(); err != nil {
		job.respond(nil, err)
		return
	}
	data, err := s.pdfBuilder(dtoToDomain(tasks))
	job.respond(data, err)
}

func (j reportJob) respond(data []byte, err error) {
	select {
	case j.resp <- reportResult{data: data, err: err}:
	case <-j.ctx.Done():
	}
}
