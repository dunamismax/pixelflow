package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/dunamismax/pixelflow/internal/domain"
)

func BenchmarkProcessorResize(b *testing.B) {
	source := benchmarkPNG(b, 1920, 1080)
	processor, err := NewLocalProcessor(b.TempDir())
	if err != nil {
		b.Fatalf("new local processor: %v", err)
	}
	processor.fetcher = staticFetcher{data: source}
	processor.emitter = discardEmitter{}

	req := Request{
		JobID:      "bench",
		SourceType: SourceTypeLocalFile,
		ObjectKey:  "ignored.png",
		Pipeline: []domain.PipelineStep{
			{
				ID:      "resize_640_jpeg",
				Action:  "resize",
				Width:   640,
				Format:  "jpeg",
				Quality: 82,
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.JobID = fmt.Sprintf("bench-resize-%d", i)
		if _, err := processor.Process(context.Background(), req); err != nil {
			b.Fatalf("process: %v", err)
		}
	}
}

func BenchmarkProcessorWatermark(b *testing.B) {
	source := benchmarkPNG(b, 1920, 1080)
	processor, err := NewLocalProcessor(b.TempDir())
	if err != nil {
		b.Fatalf("new local processor: %v", err)
	}
	processor.fetcher = staticFetcher{data: source}
	processor.emitter = discardEmitter{}

	req := Request{
		JobID:      "bench",
		SourceType: SourceTypeLocalFile,
		ObjectKey:  "ignored.png",
		Pipeline: []domain.PipelineStep{
			{
				ID:     "watermark_png",
				Action: "watermark",
				Format: "png",
				Watermark: &domain.Watermark{
					Text:    "PixelFlow",
					Opacity: 0.75,
					Gravity: "south",
				},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.JobID = fmt.Sprintf("bench-watermark-%d", i)
		if _, err := processor.Process(context.Background(), req); err != nil {
			b.Fatalf("process: %v", err)
		}
	}
}

type staticFetcher struct {
	data []byte
}

func (f staticFetcher) Fetch(_ context.Context, _ Request) ([]byte, error) {
	return f.data, nil
}

type discardEmitter struct{}

func (discardEmitter) Emit(_ context.Context, _ Request, step domain.PipelineStep, data []byte, format string, width, height int) (Output, error) {
	return Output{
		StepID:  step.ID,
		Action:  step.Action,
		Format:  normalizeOutputFormat(format),
		Path:    "",
		Bytes:   len(data),
		Width:   width,
		Height:  height,
		Success: true,
	}, nil
}

func benchmarkPNG(b *testing.B, w, h int) []byte {
	b.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / w),
				G: uint8((y * 255) / h),
				B: 140,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		b.Fatalf("encode source png: %v", err)
	}
	return buf.Bytes()
}
