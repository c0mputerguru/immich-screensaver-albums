package immich

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoRequestLogsRequestAndResponseWhenVerbose(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	var logs bytes.Buffer
	client := NewClient(server.URL, "test-key")
	client.Verbose = true
	client.LogWriter = &logs

	if _, err := client.doRequest(http.MethodPost, "/api/search/metadata", map[string]string{"page": "1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAuth != "test-key" {
		t.Errorf("expected api key header to be sent, got %q", receivedAuth)
	}

	out := logs.String()
	if !strings.Contains(out, "--> POST "+server.URL+"/api/search/metadata") {
		t.Errorf("verbose output missing request line:\n%s", out)
	}
	if !strings.Contains(out, `"page": "1"`) {
		t.Errorf("verbose output missing request body:\n%s", out)
	}
	if !strings.Contains(out, "<-- 200 POST "+server.URL+"/api/search/metadata") {
		t.Errorf("verbose output missing response line:\n%s", out)
	}
	if !strings.Contains(out, `"status": "ok"`) {
		t.Errorf("verbose output missing response body:\n%s", out)
	}
	if strings.Contains(out, "test-key") {
		t.Errorf("verbose output must not leak the api key:\n%s", out)
	}
}

func TestDoRequestIsSilentWhenNotVerbose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	var logs bytes.Buffer
	client := NewClient(server.URL, "test-key")
	client.LogWriter = &logs

	if _, err := client.doRequest(http.MethodGet, "/api/albums", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if logs.Len() != 0 {
		t.Errorf("expected no output when verbose is off, got:\n%s", logs.String())
	}
}

func TestDoRequestLogsFailedRequestWhenVerbose(t *testing.T) {
	var logs bytes.Buffer
	client := NewClient("http://127.0.0.1:0", "test-key")
	client.Verbose = true
	client.LogWriter = &logs

	if _, err := client.doRequest(http.MethodGet, "/api/albums", nil); err == nil {
		t.Fatal("expected an error for an unreachable server")
	}

	if !strings.Contains(logs.String(), "!! GET") {
		t.Errorf("expected a failure log line, got:\n%s", logs.String())
	}
}
