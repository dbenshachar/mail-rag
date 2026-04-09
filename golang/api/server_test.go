package api

import (
	"context"
	"encoding/json"
	"errors"
	"mail_rag/golang/mongodb"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type mockService struct {
	syncFn   func(ctx context.Context) (int, error)
	listFn   func(ctx context.Context, limit, offset int) ([]mongodb.EmailSummary, int64, error)
	searchFn func(ctx context.Context, query string, limit int, threshold float32) ([]mongodb.SearchResult, error)
}

func (m mockService) Sync(ctx context.Context) (int, error) {
	return m.syncFn(ctx)
}

func (m mockService) ListEmails(ctx context.Context, limit, offset int) ([]mongodb.EmailSummary, int64, error) {
	return m.listFn(ctx, limit, offset)
}

func (m mockService) Search(ctx context.Context, query string, limit int, threshold float32) ([]mongodb.SearchResult, error) {
	return m.searchFn(ctx, query, limit, threshold)
}

func TestHandleEmailsReturnsExpectedShape(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	server := NewServer(mockService{
		syncFn: func(ctx context.Context) (int, error) { return 0, nil },
		listFn: func(ctx context.Context, limit, offset int) ([]mongodb.EmailSummary, int64, error) {
			if limit != 20 || offset != 0 {
				t.Fatalf("unexpected pagination: limit=%d offset=%d", limit, offset)
			}
			return []mongodb.EmailSummary{{
				EmailID: "e1",
				Subject: "Hello",
				From:    "a@example.com",
				To:      "b@example.com",
				Date:    now,
				Snippet: "Body",
			}}, 1, nil
		},
		searchFn: func(ctx context.Context, query string, limit int, threshold float32) ([]mongodb.SearchResult, error) {
			return nil, nil
		},
	}, "http://localhost:3000")

	req := httptest.NewRequest(http.MethodGet, "/api/emails", nil)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload struct {
		Emails []mongodb.EmailSummary `json:"emails"`
		Total  int64                  `json:"total"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(payload.Emails) != 1 {
		t.Fatalf("expected one email, got %d", len(payload.Emails))
	}
	if payload.Emails[0].EmailID != "e1" {
		t.Fatalf("unexpected email id: %s", payload.Emails[0].EmailID)
	}
	if payload.Total != 1 {
		t.Fatalf("unexpected total: %d", payload.Total)
	}
}

func TestHandleSearchScenarios(t *testing.T) {
	server := NewServer(mockService{
		syncFn: func(ctx context.Context) (int, error) { return 0, nil },
		listFn: func(ctx context.Context, limit, offset int) ([]mongodb.EmailSummary, int64, error) {
			return nil, 0, nil
		},
		searchFn: func(ctx context.Context, query string, limit int, threshold float32) ([]mongodb.SearchResult, error) {
			if query != "internship" {
				return nil, errors.New("unexpected query")
			}
			if limit != 5 {
				return nil, errors.New("unexpected limit")
			}
			if threshold != float32(0.7) {
				return nil, errors.New("unexpected threshold")
			}
			return []mongodb.SearchResult{{
				Score: 0.91,
				Email: mongodb.EmailSummary{EmailID: "e2", Subject: "Internship"},
			}}, nil
		},
	}, "http://localhost:3000")

	t.Run("empty query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/search?q=", nil)
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.Code)
		}
		if !strings.Contains(res.Body.String(), `"results":[]`) {
			t.Fatalf("expected empty results, got %s", res.Body.String())
		}
	})

	t.Run("normal query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/search?q=internship&limit=5&threshold=0.7", nil)
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.Code)
		}
		if !strings.Contains(res.Body.String(), `"email_id":"e2"`) {
			t.Fatalf("missing expected result: %s", res.Body.String())
		}
	})

	t.Run("bad params", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/search?q=internship&threshold=wat", nil)
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", res.Code)
		}
	})
}

func TestHandleSyncReturnsStatusAndCount(t *testing.T) {
	server := NewServer(mockService{
		syncFn: func(ctx context.Context) (int, error) { return 3, nil },
		listFn: func(ctx context.Context, limit, offset int) ([]mongodb.EmailSummary, int64, error) {
			return nil, 0, nil
		},
		searchFn: func(ctx context.Context, query string, limit int, threshold float32) ([]mongodb.SearchResult, error) {
			return nil, nil
		},
	}, "http://localhost:3000")

	req := httptest.NewRequest(http.MethodPost, "/api/sync", nil)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"synced_count":3`) {
		t.Fatalf("unexpected body: %s", res.Body.String())
	}
}
