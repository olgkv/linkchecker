package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"webserver/internal/domain"
	"webserver/internal/service"
)

type LinksRequest struct {
	Links []string `json:"links"`
}

type LinksResponse struct {
	Links    map[string]domain.LinkStatus `json:"links"`
	LinksNum int                         `json:"links_num"`
}

type ReportRequest struct {
	LinksList []int `json:"links_list"`
}

type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Links(w http.ResponseWriter, r *http.Request) {
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

	id, result, err := h.svc.CheckLinks(r.Context(), req.Links)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// сохраняем links_num в контексте для логов
	r = r.WithContext(context.WithValue(r.Context(), "links_num", id))

	resp := LinksResponse{Links: result, LinksNum: id}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Report(w http.ResponseWriter, r *http.Request) {
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

	data, err := h.svc.GenerateReport(r.Context(), req.LinksList)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=report.pdf")
	_, _ = w.Write(data)
}
