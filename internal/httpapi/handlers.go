package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/olgkv/linkchecker/internal/domain"
	"github.com/olgkv/linkchecker/internal/service"
)

type contextKey struct{ name string }

var LinksNumContextKey = &contextKey{name: "links_num"}

const reportGenerationTimeout = 30 * time.Second

type LinksRequest struct {
	Links []string `json:"links"`
}

type LinksResponse struct {
	Links     map[string]domain.LinkStatus `json:"links"`
	LinksNum  int                          `json:"links_num"`
	Persisted bool                         `json:"persisted"`
}

type ReportRequest struct {
	LinksList []int `json:"links_list"`
}

type Handler struct {
	svc      *service.Service
	maxLinks int
}

func NewHandler(svc *service.Service, maxLinks int) *Handler {
	if maxLinks <= 0 {
		maxLinks = 50
	}
	return &Handler{svc: svc, maxLinks: maxLinks}
}

func (h *Handler) Links(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	var req LinksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(req.Links) == 0 || len(req.Links) > h.maxLinks {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id, result, err := h.svc.CheckLinks(r.Context(), req.Links)
	if err != nil && !errors.Is(err, service.ErrResultPersistDeferred) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	ctxWithNum := context.WithValue(r.Context(), LinksNumContextKey, id)
	*r = *r.WithContext(ctxWithNum)

	resp := LinksResponse{Links: result, LinksNum: id, Persisted: err == nil}
	status := http.StatusOK
	if err != nil {
		status = http.StatusAccepted
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Report(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	var req ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(req.LinksList) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	for _, id := range req.LinksList {
		if id <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), reportGenerationTimeout)
	defer cancel()

	data, err := h.svc.GenerateReport(ctx, req.LinksList)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			http.Error(w, "report generation timeout", http.StatusGatewayTimeout)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=report.pdf")
	_, _ = w.Write(data)
}
