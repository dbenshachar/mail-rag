package mongodb

import (
	"mail_rag/golang/mail"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestRankSearchResultsThresholdSortLimit(t *testing.T) {
	queryEmbedding := []float32{1, 0}
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	docs := []Document{
		{
			EmailID:   "a",
			Subject:   "A",
			Snippet:   "A",
			Date:      now.Add(-time.Hour),
			Embedding: []float32{1, 0},
		},
		{
			EmailID:   "b",
			Subject:   "B",
			Snippet:   "B",
			Date:      now,
			Embedding: []float32{1, 0},
		},
		{
			EmailID:   "c",
			Subject:   "C",
			Snippet:   "C",
			Date:      now,
			Embedding: []float32{0.2, 0.8},
		},
		{
			EmailID:   "d",
			Subject:   "D",
			Snippet:   "D",
			Date:      now,
			Embedding: []float32{1, 0, 0},
		},
	}

	results, err := RankSearchResults(queryEmbedding, docs, 0.6, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two results, got %d", len(results))
	}
	if results[0].Email.EmailID != "b" {
		t.Fatalf("expected newest tie-break first, got %s", results[0].Email.EmailID)
	}
	if results[1].Email.EmailID != "a" {
		t.Fatalf("expected second result to be a, got %s", results[1].Email.EmailID)
	}
}

func TestBuildUpsertSpecUsesEmailIDAndUpsert(t *testing.T) {
	record := mail.EmailRecord{
		EmailID:  "email-42",
		Subject:  "Subject",
		From:     "from@example.com",
		To:       "to@example.com",
		Date:     time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Snippet:  "Snippet",
		Contents: "Body",
	}
	filter, _, optsBuilder := BuildUpsertSpec(record, []float32{0.1, 0.2}, time.Now().UTC())
	if len(filter) == 0 {
		t.Fatalf("expected filter with email_id")
	}
	if filter[0].Key != "email_id" {
		t.Fatalf("unexpected filter key: %s", filter[0].Key)
	}
	if filter[0].Value != "email-42" {
		t.Fatalf("unexpected filter value: %v", filter[0].Value)
	}

	var opts options.UpdateOneOptions
	for _, apply := range optsBuilder.List() {
		if err := apply(&opts); err != nil {
			t.Fatalf("failed applying options: %v", err)
		}
	}
	if opts.Upsert == nil || !*opts.Upsert {
		t.Fatalf("expected upsert=true")
	}
}
