package pipeline

import (
	"context"

	"github.com/dunamismax/pixelflow/internal/domain"
)

type Transformer interface {
	Transform(ctx context.Context, input []byte, step domain.PipelineStep) (data []byte, format string, width, height int, err error)
}

func normalizeOutputFormat(format string) string {
	switch format {
	case "jpg":
		return "jpeg"
	case "jpeg", "png", "webp":
		return format
	default:
		return "png"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
