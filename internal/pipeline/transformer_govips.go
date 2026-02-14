//go:build govips && cgo

package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/dunamismax/pixelflow/internal/domain"
)

type govipsTransformer struct{}

func (t govipsTransformer) Transform(ctx context.Context, input []byte, step domain.PipelineStep) ([]byte, string, int, int, error) {
	select {
	case <-ctx.Done():
		return nil, "", 0, 0, ctx.Err()
	default:
	}

	img, err := vips.NewImageFromBuffer(input)
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("decode source image: %w", err)
	}
	defer img.Close()

	switch strings.ToLower(strings.TrimSpace(step.Action)) {
	case "resize":
		err = applyGovipsResize(img, step.Width)
	case "watermark":
		err = applyGovipsWatermark(img, step.Watermark)
	default:
		return nil, "", 0, 0, fmt.Errorf("%w: %q", ErrInvalidStepAction, step.Action)
	}
	if err != nil {
		return nil, "", 0, 0, err
	}

	format := formatForStep(step.Format, input)
	data, err := exportGovipsImage(img, format, step.Quality)
	if err != nil {
		return nil, "", 0, 0, err
	}

	return data, format, img.Width(), img.Height(), nil
}

func applyGovipsResize(img *vips.ImageRef, targetWidth int) error {
	if targetWidth <= 0 {
		return fmt.Errorf("resize action requires width > 0")
	}
	if img.Width() <= 0 {
		return fmt.Errorf("source image has invalid width")
	}

	scale := float64(targetWidth) / float64(img.Width())
	if scale <= 0 {
		return fmt.Errorf("invalid resize scale")
	}

	if err := img.Resize(scale, vips.KernelLanczos3); err != nil {
		return fmt.Errorf("resize image: %w", err)
	}
	return nil
}

func applyGovipsWatermark(img *vips.ImageRef, wm *domain.Watermark) error {
	if wm == nil {
		return fmt.Errorf("watermark action requires watermark settings")
	}

	text := strings.TrimSpace(wm.Text)
	if text == "" {
		return fmt.Errorf("watermark action requires watermark.text")
	}

	opacity := wm.Opacity
	if opacity <= 0 {
		opacity = 0.65
	}
	if opacity > 1 {
		opacity = 1
	}

	label := &vips.LabelParams{
		Text:      text,
		Font:      "sans 24",
		Opacity:   float32(opacity),
		Color:     vips.Color{R: 255, G: 255, B: 255},
		Alignment: alignmentFromGravity(wm.Gravity),
	}
	label.Width.SetInt(max(1, img.Width()-24))
	label.Height.SetInt(max(1, img.Height()-24))
	label.OffsetX.SetInt(12)
	label.OffsetY.SetInt(12)

	if err := img.Label(label); err != nil {
		return fmt.Errorf("apply watermark: %w", err)
	}
	return nil
}

func alignmentFromGravity(gravity string) vips.Align {
	gravity = strings.ToLower(strings.TrimSpace(gravity))
	switch {
	case strings.Contains(gravity, "west"):
		return vips.AlignLow
	case strings.Contains(gravity, "center"), strings.HasSuffix(gravity, "north"), strings.HasSuffix(gravity, "south"):
		return vips.AlignCenter
	default:
		return vips.AlignHigh
	}
}

func formatForStep(stepFormat string, input []byte) string {
	if strings.TrimSpace(stepFormat) != "" {
		return normalizeOutputFormat(strings.ToLower(strings.TrimSpace(stepFormat)))
	}

	switch vips.DetermineImageType(input) {
	case vips.ImageTypeJPEG:
		return "jpeg"
	case vips.ImageTypeWEBP:
		return "webp"
	default:
		return "png"
	}
}

func exportGovipsImage(img *vips.ImageRef, format string, quality int) ([]byte, error) {
	switch format {
	case "jpeg":
		params := vips.NewJpegExportParams()
		if quality > 0 && quality <= 100 {
			params.Quality = quality
		}
		data, _, err := img.ExportJpeg(params)
		if err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
		return data, nil
	case "png":
		params := vips.NewPngExportParams()
		if quality > 0 && quality <= 100 {
			params.Quality = quality
		}
		data, _, err := img.ExportPng(params)
		if err != nil {
			return nil, fmt.Errorf("encode png: %w", err)
		}
		return data, nil
	case "webp":
		params := vips.NewWebpExportParams()
		if quality > 0 && quality <= 100 {
			params.Quality = quality
		}
		data, _, err := img.ExportWebp(params)
		if err != nil {
			return nil, fmt.Errorf("encode webp: %w", err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported output format: %s", format)
	}
}
