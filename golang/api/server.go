package api

import (
	"context"
	"encoding/json"
	"errors"
	"mail_rag/golang/mongodb"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Service interface {
	Sync(ctx context.Context) (int, error)
	ListEmails(ctx context.Context, limit, offset int) ([]mongodb.EmailSummary, int64, error)
	Search(ctx context.Context, query string, limit int, threshold float32) ([]mongodb.SearchResult, error)
}

type Server struct {
	service        Service
	frontendOrigin string
	syncMu         sync.Mutex
	syncing        bool
}

func NewServer(service Service, frontendOrigin string) *Server {
	origin := strings.TrimSpace(frontendOrigin)
	if origin == "" {
		origin = "http://localhost:3000"
	}
	return &Server{service: service, frontendOrigin: origin}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync", s.handleSync)
	mux.HandleFunc("/api/emails", s.handleEmails)
	mux.HandleFunc("/api/search", s.handleSearch)
	return s.withCORS(mux)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" && origin == s.frontendOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func parseIntWithDefault(value string, defaultValue int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func parseFloatWithDefault(value string, defaultValue float32) (float32, error) {
	if strings.TrimSpace(value) == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil {
		return 0, err
	}
	return float32(parsed), nil
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	s.syncMu.Lock()
	if s.syncing {
		s.syncMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"status": "error", "synced_count": 0, "error": "sync already running"})
		return
	}
	s.syncing = true
	s.syncMu.Unlock()

	defer func() {
		s.syncMu.Lock()
		s.syncing = false
		s.syncMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	count, err := s.service.Sync(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "synced_count": count, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "synced_count": count})
}

func (s *Server) handleEmails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit, err := parseIntWithDefault(r.URL.Query().Get("limit"), 20)
	if err != nil || limit <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid limit"})
		return
	}
	if limit > 100 {
		limit = 100
	}
	offset, err := parseIntWithDefault(r.URL.Query().Get("offset"), 0)
	if err != nil || offset < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid offset"})
		return
	}

	emails, total, err := s.service.ListEmails(r.Context(), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"emails": emails, "total": total})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]any{"query": "", "results": []mongodb.SearchResult{}})
		return
	}

	limit, err := parseIntWithDefault(r.URL.Query().Get("limit"), 20)
	if err != nil || limit <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid limit"})
		return
	}
	if limit > 100 {
		limit = 100
	}

	threshold, err := parseFloatWithDefault(r.URL.Query().Get("threshold"), 0.6)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid threshold"})
		return
	}
	if threshold < 0 || threshold > 1 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid threshold"})
		return
	}

	results, err := s.service.Search(r.Context(), query, limit, threshold)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"query": query, "results": results})
}
