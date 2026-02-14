package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/dunamismax/pixelflow/internal/domain"
	"github.com/dunamismax/pixelflow/internal/storage"
)

const (
	SourceTypeS3Presigned = domain.SourceTypeS3Presigned
)

type ObjectStoreFetcher struct {
	Storage *storage.Client
}

func (f ObjectStoreFetcher) Fetch(ctx context.Context, req Request) ([]byte, error) {
	if f.Storage == nil {
		return nil, errors.New("storage client is required")
	}
	if strings.EqualFold(req.SourceType, SourceTypeLocalFile) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedSourceType, req.SourceType)
	}
	return f.Storage.ReadObject(ctx, req.ObjectKey)
}

type ObjectStoreEmitter struct {
	Storage      *storage.Client
	OutputPrefix string
}

func (e ObjectStoreEmitter) Emit(ctx context.Context, req Request, step domain.PipelineStep, data []byte, format string, width, height int) (Output, error) {
	if e.Storage == nil {
		return Output{}, errors.New("storage client is required")
	}
	if strings.TrimSpace(step.ID) == "" {
		return Output{}, errors.New("pipeline step id is required")
	}

	objectKey := path.Join(
		defaultOutputPrefix(e.OutputPrefix),
		sanitizePathToken(req.JobID),
		fmt.Sprintf("%s.%s", sanitizePathToken(step.ID), normalizeOutputFormat(format)),
	)

	if err := e.Storage.WriteObject(ctx, objectKey, data, contentTypeForFormat(format)); err != nil {
		return Output{}, err
	}

	return Output{
		StepID:  step.ID,
		Action:  step.Action,
		Format:  normalizeOutputFormat(format),
		Path:    objectKey,
		Bytes:   len(data),
		Width:   width,
		Height:  height,
		Success: true,
	}, nil
}

func defaultOutputPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "outputs"
	}
	return prefix
}

func contentTypeForFormat(format string) string {
	switch normalizeOutputFormat(strings.ToLower(strings.TrimSpace(format))) {
	case "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}
