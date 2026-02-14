package pipeline

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dunamismax/pixelflow/internal/domain"
)

func TestLocalProcessor_FileInTransformFileOut(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "input.png")
	outputDir := filepath.Join(tmp, "out")

	srcBytes := buildTestPNG(t, 240, 120)
	if err := os.WriteFile(inputPath, srcBytes, 0o644); err != nil {
		t.Fatalf("write input image: %v", err)
	}

	processor, err := NewLocalProcessor(outputDir)
	if err != nil {
		t.Fatalf("new local processor: %v", err)
	}

	req := Request{
		JobID:      "job-local-1",
		SourceType: SourceTypeLocalFile,
		ObjectKey:  inputPath,
		Pipeline: []domain.PipelineStep{
			{
				ID:      "thumb_small",
				Action:  "resize",
				Width:   80,
				Format:  "jpeg",
				Quality: 75,
			},
			{
				ID:     "watermarked",
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

	result, err := processor.Process(context.Background(), req)
	if err != nil {
		t.Fatalf("process request: %v", err)
	}

	if len(result.Outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(result.Outputs))
	}

	resized := result.Outputs[0]
	if resized.Format != "jpeg" {
		t.Fatalf("expected jpeg output format, got %s", resized.Format)
	}
	verifyImageWidth(t, resized.Path, 80)

	watermarked := result.Outputs[1]
	if watermarked.Format != "png" {
		t.Fatalf("expected png output format, got %s", watermarked.Format)
	}

	watermarkedBytes, err := os.ReadFile(watermarked.Path)
	if err != nil {
		t.Fatalf("read watermarked image: %v", err)
	}
	if bytes.Equal(srcBytes, watermarkedBytes) {
		t.Fatal("expected watermark output to differ from source image bytes")
	}
}

func TestLocalProcessor_UnsupportedSourceType(t *testing.T) {
	processor, err := NewLocalProcessor(t.TempDir())
	if err != nil {
		t.Fatalf("new local processor: %v", err)
	}

	_, err = processor.Process(context.Background(), Request{
		JobID:      "job-unsupported",
		SourceType: "s3_presigned",
		ObjectKey:  "uploads/job/source",
		Pipeline: []domain.PipelineStep{
			{
				ID:     "thumb_small",
				Action: "resize",
				Width:  120,
			},
		},
	})
	if err == nil {
		t.Fatal("expected unsupported source_type error")
	}
}

func buildTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()

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
		t.Fatalf("encode source png: %v", err)
	}
	return buf.Bytes()
}

func verifyImageWidth(t *testing.T, path string, want int) {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open image %s: %v", path, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode image %s: %v", path, err)
	}

	if got := img.Bounds().Dx(); got != want {
		t.Fatalf("expected width %d, got %d", want, got)
	}
}
