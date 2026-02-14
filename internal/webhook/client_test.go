package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSendAddsSigningHeaders(t *testing.T) {
	var (
		gotSig string
		gotTS  string
		gotEvt string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get(HeaderSignature)
		gotTS = r.Header.Get(HeaderTimestamp)
		gotEvt = r.Header.Get(HeaderEvent)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(Config{
		SigningSecret:  "test-secret",
		Timeout:        2 * time.Second,
		MaxAttempts:    1,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
	})

	err := client.Send(context.Background(), srv.URL, "job.completed", map[string]any{"job_id": "job-1"})
	if err != nil {
		t.Fatalf("send returned error: %v", err)
	}

	if gotSig == "" {
		t.Fatal("expected signature header")
	}
	if gotTS == "" {
		t.Fatal("expected timestamp header")
	}
	if gotEvt != "job.completed" {
		t.Fatalf("expected event header job.completed, got %q", gotEvt)
	}
}
