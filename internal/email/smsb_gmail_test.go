package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSmsbGmailWaitForCodeCompletesActivation(t *testing.T) {
	var calls []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Query())
		switch r.URL.Path {
		case "/getActivation":
			writeJSON(t, w, map[string]any{"status": 1, "mail": "leased@gmail.com", "mailId": 7711939})
		case "/getCode":
			writeJSON(t, w, map[string]any{"status": 1, "code": "123456"})
		case "/setStatus":
			writeJSON(t, w, map[string]any{"status": 1})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	withSmsbMailAPIBase(t, server.URL)

	provider, err := NewSmsbGmailProvider(context.Background(), SmsbGmailConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewSmsbGmailProvider() error = %v", err)
	}
	if got := provider.GetAddress(); got != "leased@gmail.com" {
		t.Fatalf("GetAddress() = %q", got)
	}
	code, err := provider.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode() error = %v", err)
	}
	if code != "123456" {
		t.Fatalf("WaitForCode() = %q", code)
	}
	assertSetStatus(t, calls, "7711939", "3")
}

func TestSmsbGmailWaitForCodeTimeoutCancelsActivation(t *testing.T) {
	var calls []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Query())
		switch r.URL.Path {
		case "/getActivation":
			writeJSON(t, w, map[string]any{"status": 1, "mail": "timeout@gmail.com", "mailId": "mail-2"})
		case "/getCode":
			writeJSON(t, w, map[string]any{"status": 0, "error": "Code has not been received yet, please try again later"})
		case "/setStatus":
			writeJSON(t, w, map[string]any{"status": 1})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	withSmsbMailAPIBase(t, server.URL)

	provider, err := NewSmsbGmailProvider(context.Background(), SmsbGmailConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewSmsbGmailProvider() error = %v", err)
	}
	if _, err := provider.WaitForCode(1, 1); err == nil {
		t.Fatal("WaitForCode() expected timeout error, got nil")
	}
	assertSetStatus(t, calls, "mail-2", "2")
}

func withSmsbMailAPIBase(t *testing.T, base string) {
	t.Helper()
	old := smsbMailAPIBase
	smsbMailAPIBase = base
	t.Cleanup(func() { smsbMailAPIBase = old })
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("writeJSON() error = %v", err)
	}
}

func assertSetStatus(t *testing.T, calls []url.Values, id, status string) {
	t.Helper()
	for _, q := range calls {
		if q.Get("id") == id && q.Get("status") == status {
			return
		}
	}
	t.Fatalf("setStatus id=%s status=%s not called; calls=%v", id, status, calls)
}
