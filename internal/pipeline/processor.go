package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dunamismax/pixelflow/internal/domain"
)

const SourceTypeLocalFile = "local_file"

var (
	ErrUnsupportedSourceType = errors.New("unsupported source_type")
	ErrInvalidStepAction     = errors.New("invalid pipeline action")
)

type Request struct {
	JobID      string
	SourceType string
	ObjectKey  string
	Pipeline   []domain.PipelineStep
}

type Output struct {
	StepID  string
	Action  string
	Format  string
	Path    string
	Bytes   int
	Width   int
	Height  int
	Success bool
}

type Result struct {
	Outputs []Output
}

type Fetcher interface {
	Fetch(ctx context.Context, req Request) ([]byte, error)
}

type Emitter interface {
	Emit(ctx context.Context, req Request, step domain.PipelineStep, data []byte, format string, width, height int) (Output, error)
}

type Processor struct {
	fetcher     Fetcher
	transformer Transformer
	emitter     Emitter
}

func NewLocalProcessor(outputDir string) (*Processor, error) {
	transformer, err := newTransformer()
	if err != nil {
		return nil, fmt.Errorf("build transformer: %w", err)
	}

	return &Processor{
		fetcher:     LocalFileFetcher{},
		transformer: transformer,
		emitter:     LocalFileEmitter{OutputDir: outputDir},
	}, nil
}

func (p *Processor) Process(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.JobID) == "" {
		return Result{}, errors.New("job_id is required")
	}
	if len(req.Pipeline) == 0 {
		return Result{}, errors.New("pipeline must contain at least one step")
	}

	sourceBytes, err := p.fetcher.Fetch(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("fetch stage: %w", err)
	}

	out := Result{Outputs: make([]Output, 0, len(req.Pipeline))}
	for _, step := range req.Pipeline {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}

		transformed, format, width, height, err := p.transformer.Transform(ctx, sourceBytes, step)
		if err != nil {
			return Result{}, fmt.Errorf("transform stage step=%s action=%s: %w", step.ID, step.Action, err)
		}

		written, err := p.emitter.Emit(ctx, req, step, transformed, format, width, height)
		if err != nil {
			return Result{}, fmt.Errorf("emit stage step=%s action=%s: %w", step.ID, step.Action, err)
		}
		out.Outputs = append(out.Outputs, written)
	}

	return out, nil
}

type LocalFileFetcher struct{}

func (LocalFileFetcher) Fetch(ctx context.Context, req Request) ([]byte, error) {
	if !strings.EqualFold(req.SourceType, SourceTypeLocalFile) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedSourceType, req.SourceType)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	data, err := os.ReadFile(req.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("read input file %s: %w", req.ObjectKey, err)
	}
	return data, nil
}

type LocalFileEmitter struct {
	OutputDir string
}

func (e LocalFileEmitter) Emit(_ context.Context, req Request, step domain.PipelineStep, data []byte, format string, width, height int) (Output, error) {
	if strings.TrimSpace(e.OutputDir) == "" {
		return Output{}, errors.New("output directory is required")
	}
	if strings.TrimSpace(step.ID) == "" {
		return Output{}, errors.New("pipeline step id is required")
	}

	jobDir := filepath.Join(e.OutputDir, sanitizePathToken(req.JobID))
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return Output{}, fmt.Errorf("create output dir: %w", err)
	}

	filename := fmt.Sprintf("%s.%s", sanitizePathToken(step.ID), normalizeOutputFormat(format))
	fullPath := filepath.Join(jobDir, filename)
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return Output{}, fmt.Errorf("write output file: %w", err)
	}

	return Output{
		StepID:  step.ID,
		Action:  step.Action,
		Format:  normalizeOutputFormat(format),
		Path:    fullPath,
		Bytes:   len(data),
		Width:   width,
		Height:  height,
		Success: true,
	}, nil
}

func sanitizePathToken(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return "unknown"
	}

	var b strings.Builder
	b.Grow(len(in))
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
