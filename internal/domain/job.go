package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	JobStatusCreated = "created"
	JobStatusQueued  = "queued"
)

type CreateJobRequest struct {
	SourceType string         `json:"source_type"`
	WebhookURL string         `json:"webhook_url,omitempty"`
	Pipeline   []PipelineStep `json:"pipeline"`
}

type PipelineStep struct {
	ID        string     `json:"id"`
	Action    string     `json:"action"`
	Width     int        `json:"width,omitempty"`
	Format    string     `json:"format,omitempty"`
	Quality   int        `json:"quality,omitempty"`
	Watermark *Watermark `json:"watermark,omitempty"`
}

type Watermark struct {
	Text    string  `json:"text"`
	Opacity float64 `json:"opacity"`
	Gravity string  `json:"gravity"`
}

type Job struct {
	ID         string
	Status     string
	SourceType string
	WebhookURL string
	Pipeline   []PipelineStep
	ObjectKey  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (r CreateJobRequest) Validate() error {
	if strings.TrimSpace(r.SourceType) == "" {
		return errors.New("source_type is required")
	}
	if len(r.Pipeline) == 0 {
		return errors.New("pipeline must contain at least one step")
	}
	for i, step := range r.Pipeline {
		if strings.TrimSpace(step.ID) == "" {
			return fmt.Errorf("pipeline[%d].id is required", i)
		}
		if strings.TrimSpace(step.Action) == "" {
			return fmt.Errorf("pipeline[%d].action is required", i)
		}
	}
	return nil
}
