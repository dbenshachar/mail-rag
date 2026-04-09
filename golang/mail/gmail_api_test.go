package mail

import (
	"encoding/base64"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
)

func TestExtractEmailRecordParsesMetadata(t *testing.T) {
	body := "hello from test body"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(body))
	msg := &gmail.Message{
		Id:           "email-1",
		Snippet:      "snippet value",
		InternalDate: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC).UnixMilli(),
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "Internship update"},
				{Name: "From", Value: "recruiter@example.com"},
				{Name: "To", Value: "me@example.com"},
				{Name: "Date", Value: "Mon, 02 Feb 2026 04:05:06 +0000"},
			},
			Parts: []*gmail.MessagePart{
				{MimeType: "text/plain", Body: &gmail.MessagePartBody{Data: encoded}},
			},
		},
	}

	record, err := ExtractEmailRecord(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.EmailID != "email-1" {
		t.Fatalf("unexpected email id: %s", record.EmailID)
	}
	if record.Subject != "Internship update" {
		t.Fatalf("unexpected subject: %s", record.Subject)
	}
	if record.From != "recruiter@example.com" {
		t.Fatalf("unexpected from: %s", record.From)
	}
	if record.To != "me@example.com" {
		t.Fatalf("unexpected to: %s", record.To)
	}
	if record.Snippet != "snippet value" {
		t.Fatalf("unexpected snippet: %s", record.Snippet)
	}
	if record.Contents != body {
		t.Fatalf("unexpected body: %s", record.Contents)
	}
	expectedDate := time.Date(2026, 2, 2, 4, 5, 6, 0, time.UTC)
	if !record.Date.Equal(expectedDate) {
		t.Fatalf("unexpected date: %s", record.Date.String())
	}
}

func TestExtractEmailRecordBuildsSnippetFromBody(t *testing.T) {
	body := "this is the body when snippet is missing"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(body))
	msg := &gmail.Message{
		Id: "email-2",
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "No snippet"},
			},
			Parts: []*gmail.MessagePart{
				{MimeType: "text/plain", Body: &gmail.MessagePartBody{Data: encoded}},
			},
		},
	}

	record, err := ExtractEmailRecord(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Snippet != body {
		t.Fatalf("expected snippet from body, got: %s", record.Snippet)
	}
}
