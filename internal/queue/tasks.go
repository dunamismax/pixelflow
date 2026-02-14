package queue

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dunamismax/pixelflow/internal/domain"
	"github.com/hibiken/asynq"
)

const TypeProcessImage = "image:process"

type ProcessImagePayload struct {
	JobID       string                `json:"job_id"`
	SourceType  string                `json:"source_type"`
	WebhookURL  string                `json:"webhook_url,omitempty"`
	ObjectKey   string                `json:"object_key"`
	Pipeline    []domain.PipelineStep `json:"pipeline"`
	RequestedAt time.Time             `json:"requested_at"`
}

func NewProcessImageTask(payload ProcessImagePayload) (*asynq.Task, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal process payload: %w", err)
	}
	return asynq.NewTask(TypeProcessImage, body), nil
}

func ParseProcessImagePayload(task *asynq.Task) (ProcessImagePayload, error) {
	var payload ProcessImagePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return ProcessImagePayload{}, fmt.Errorf("unmarshal process payload: %w", err)
	}
	return payload, nil
}
