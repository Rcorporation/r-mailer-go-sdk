package rmailer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSendTemplateSuccess(t *testing.T) {
	var gotAuth string
	var gotReq SendTemplateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/send-template" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content-type = %q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeMessage(t, w, "msg-123")
	}))
	defer server.Close()

	client := New(server.URL)
	msg, err := client.SendTemplate(context.Background(), "token-123", SendTemplateRequest{
		From:           "no-reply@rcorp.cc",
		To:             []string{"user@example.com"},
		Template:       "identity.verify_email",
		Locale:         "en",
		Data:           map[string]any{"name": "Sewon"},
		IdempotencyKey: "courier-message-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer token-123" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotReq.From != "no-reply@rcorp.cc" || gotReq.Template != "identity.verify_email" || gotReq.IdempotencyKey != "courier-message-id" {
		t.Fatalf("request = %#v", gotReq)
	}
	if msg.ID != "msg-123" || msg.Status != "sent" {
		t.Fatalf("message = %#v", msg)
	}
}

func TestSendTemplateIncludesIdempotencyKeyWhenSet(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		writeMessage(t, w, "msg-123")
	}))
	defer server.Close()

	client := New(server.URL)
	_, err := client.SendTemplate(context.Background(), "token-123", SendTemplateRequest{
		From:           "no-reply@rcorp.cc",
		To:             []string{"user@example.com"},
		Template:       "identity.verify_email",
		Locale:         "en",
		Data:           map[string]any{"name": "Sewon"},
		IdempotencyKey: "kratos-courier-message-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["idempotency_key"] != "kratos-courier-message-id" {
		t.Fatalf("idempotency_key = %#v", got["idempotency_key"])
	}
}

func TestSendTemplateOmitsEmptyIdempotencyKey(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		writeMessage(t, w, "msg-123")
	}))
	defer server.Close()

	client := New(server.URL)
	_, err := client.SendTemplate(context.Background(), "token-123", SendTemplateRequest{
		From:     "no-reply@rcorp.cc",
		To:       []string{"user@example.com"},
		Template: "identity.verify_email",
		Locale:   "en",
		Data:     map[string]any{"name": "Sewon"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["idempotency_key"]; ok {
		t.Fatalf("idempotency_key should be omitted when empty: %#v", got)
	}
}

func TestGetMessageSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/messages/msg-123" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token-123" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		writeMessage(t, w, "msg-123")
	}))
	defer server.Close()

	client := New(server.URL)
	msg, err := client.GetMessage(context.Background(), "token-123", "msg-123")
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != "msg-123" {
		t.Fatalf("id = %q", msg.ID)
	}
}

func TestAPIErrorDecoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(APIError{Error: "forbidden", Message: "missing required scope"})
	}))
	defer server.Close()

	client := New(server.URL)
	_, err := client.SendTemplate(context.Background(), "token-123", SendTemplateRequest{})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T %v", err, err)
	}
	if httpErr.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", httpErr.StatusCode)
	}
	if httpErr.APIError.Error != "forbidden" || httpErr.APIError.Message != "missing required scope" {
		t.Fatalf("api error = %#v", httpErr.APIError)
	}
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writeMessage(t, w, "msg-123")
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := New(server.URL)
	_, err := client.SendTemplate(ctx, "token-123", SendTemplateRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestMissingAccessToken(t *testing.T) {
	client := New("https://mailer.example.com")
	_, err := client.GetMessage(context.Background(), "", "msg-123")
	if !errors.Is(err, ErrMissingAccessToken) {
		t.Fatalf("expected ErrMissingAccessToken, got %v", err)
	}
}

func writeMessage(t *testing.T, w http.ResponseWriter, id string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	err := json.NewEncoder(w).Encode(Message{
		ID:                id,
		ClientID:          "r-identity",
		FromAddress:       "no-reply@rcorp.cc",
		ToAddresses:       []string{"user@example.com"},
		TemplateKey:       "identity.verify_email",
		Locale:            "en",
		Subject:           "Verify your R Identity email",
		HTMLBody:          "<p>Hello Sewon</p>",
		TextBody:          "Hello Sewon",
		Status:            "sent",
		Provider:          "aws_ses",
		ProviderMessageID: "ses-123",
		IdempotencyKey:    "courier-message-id",
		CreatedAt:         now,
		SentAt:            now,
	})
	if err != nil {
		t.Fatal(err)
	}
}
