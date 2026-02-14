package api

import "testing"

func TestExtractJobIDFromStartPath(t *testing.T) {
	jobID, err := extractJobIDFromStartPath("/v1/jobs/abc123/start")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if jobID != "abc123" {
		t.Fatalf("expected abc123, got %s", jobID)
	}

	if _, err := extractJobIDFromStartPath("/v1/jobs/abc123"); err == nil {
		t.Fatal("expected error for invalid path")
	}
}
