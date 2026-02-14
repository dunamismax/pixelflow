package queue

import (
	"testing"
	"time"

	"github.com/dunamismax/pixelflow/internal/domain"
)

func TestProcessImageTaskRoundTrip(t *testing.T) {
	payload := ProcessImagePayload{
		JobID:      "job-123",
		SourceType: "s3_presigned",
		ObjectKey:  "uploads/job-123/source",
		Pipeline: []domain.PipelineStep{
			{
				ID:     "thumb_small",
				Action: "resize",
			},
		},
		RequestedAt: time.Now().UTC(),
	}

	task, err := NewProcessImageTask(payload)
	if err != nil {
		t.Fatalf("NewProcessImageTask returned error: %v", err)
	}

	parsed, err := ParseProcessImagePayload(task)
	if err != nil {
		t.Fatalf("ParseProcessImagePayload returned error: %v", err)
	}

	if parsed.JobID != payload.JobID {
		t.Fatalf("expected job_id %q, got %q", payload.JobID, parsed.JobID)
	}
	if len(parsed.Pipeline) != 1 {
		t.Fatalf("expected one pipeline step, got %d", len(parsed.Pipeline))
	}
}
