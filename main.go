package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jung-kurt/gofpdf"
)

type LinkStatus string

const (
	StatusAvailable    LinkStatus = "available"
	StatusNotAvailable LinkStatus = "not available"
)

type LinksRequest struct {
	Links []string `json:"links"`
}

type LinksResponse struct {
	Links    map[string]LinkStatus `json:"links"`
	LinksNum int                   `json:"links_num"`
}

type ReportRequest struct {
	LinksList []int `json:"links_list"`
}

type Task struct {
	ID     int               `json:"id"`
	Links  []string          `json:"links"`
	Result map[string]string `json:"result"`
}

type Storage struct {
	mu       sync.RWMutex
	filePath string
	nextID   int
	tasks    map[int]*Task
}

func NewStorage(path string) *Storage {
	return &Storage{
		filePath: path,
		nextID:   1,
		tasks:    make(map[int]*Task),
	}
}

func (s *Storage) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var tasks []*Task
	if err := dec.Decode(&tasks); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	maxID := 0
	for _, t := range tasks {
		if t.ID > maxID {
			maxID = t.ID
		}
		s.tasks[t.ID] = t
	}
	s.nextID = maxID + 1
	return nil
}

func (s *Storage) persistLocked() error {
	tmp := s.filePath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	var list []*Task
	for _, t := range s.tasks {
		list = append(list, t)
	}
	if err := enc.Encode(list); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

func (s *Storage) CreateTask(links []string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++
	t := &Task{ID: id, Links: links, Result: make(map[string]string)}
	s.tasks[id] = t
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Storage) UpdateTaskResult(id int, result map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %d not found", id)
	}
	t.Result = result
	return s.persistLocked()
}

func (s *Storage) GetTasks(ids []int) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*Task, 0, len(ids))
	for _, id := range ids {
		if t, ok := s.tasks[id]; ok {
			res = append(res, t)
		}
	}
	return res, nil
}

func checkLink(ctx context.Context, link string) LinkStatus {
	url := link
	if !(len(url) >= 7 && (url[:7] == "http://" || (len(url) >= 8 && url[:8] == "https://"))) {
		url = "https://" + link
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return StatusNotAvailable
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return StatusNotAvailable
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return StatusAvailable
	}
	return StatusNotAvailable
}

func handleLinks(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req LinksRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(req.Links) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		task, err := storage.CreateTask(req.Links)
		if err != nil {
			log.Println("create task:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		result := make(map[string]LinkStatus, len(req.Links))
		for _, link := range req.Links {
			result[link] = checkLink(ctx, link)
		}

		strResult := make(map[string]string, len(result))
		for k, v := range result {
			strResult[k] = string(v)
		}
		if err := storage.UpdateTaskResult(task.ID, strResult); err != nil {
			log.Println("update task:", err)
		}

		resp := LinksResponse{Links: result, LinksNum: task.ID}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func generatePDF(tasks []*Task) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "", 12)

	pdf.Cell(40, 10, "Links report")
	pdf.Ln(12)

	for _, t := range tasks {
		pdf.Cell(40, 10, fmt.Sprintf("Task #%d", t.ID))
		pdf.Ln(8)
		for _, link := range t.Links {
			status := t.Result[link]
			if status == "" {
				status = string(StatusNotAvailable)
			}
			pdf.Cell(40, 8, fmt.Sprintf("%s - %s", link, status))
			pdf.Ln(8)
		}
		pdf.Ln(4)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func handleReport(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req ReportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(req.LinksList) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		tasks, err := storage.GetTasks(req.LinksList)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		data, err := generatePDF(tasks)
		if err != nil {
			log.Println("generate pdf:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "attachment; filename=report.pdf")
		w.Write(data)
	}
}

func main() {
	storage := NewStorage("tasks.json")
	if err := storage.Load(); err != nil {
		log.Fatal("load storage:", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/links", handleLinks(storage))
	mux.Handle("/report", handleReport(storage))

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		log.Println("server listening on :8080")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Println("server shutdown error:", err)
	}
}
