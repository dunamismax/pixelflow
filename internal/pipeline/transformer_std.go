package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"strings"

	"github.com/dunamismax/pixelflow/internal/domain"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"
)

type stdlibTransformer struct{}

func (t stdlibTransformer) Transform(ctx context.Context, input []byte, step domain.PipelineStep) ([]byte, string, int, int, error) {
	select {
	case <-ctx.Done():
		return nil, "", 0, 0, ctx.Err()
	default:
	}

	src, srcFormat, err := image.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("decode source image: %w", err)
	}

	var out image.Image
	switch strings.ToLower(strings.TrimSpace(step.Action)) {
	case "resize":
		out, err = resizeToWidth(src, step.Width)
		if err != nil {
			return nil, "", 0, 0, err
		}
	case "watermark":
		out, err = watermarkText(src, step.Watermark)
		if err != nil {
			return nil, "", 0, 0, err
		}
	default:
		return nil, "", 0, 0, fmt.Errorf("%w: %q", ErrInvalidStepAction, step.Action)
	}

	format := normalizeOutputFormat(strings.ToLower(strings.TrimSpace(step.Format)))
	if strings.TrimSpace(step.Format) == "" {
		format = normalizeOutputFormat(strings.ToLower(srcFormat))
	}

	output, err := encodeImage(out, format, step.Quality)
	if err != nil {
		return nil, "", 0, 0, err
	}

	bounds := out.Bounds()
	return output, format, bounds.Dx(), bounds.Dy(), nil
}

func resizeToWidth(src image.Image, width int) (image.Image, error) {
	if width <= 0 {
		return nil, errors.New("resize action requires width > 0")
	}

	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW == 0 || srcH == 0 {
		return nil, errors.New("source image has invalid dimensions")
	}

	if width == srcW {
		return cloneImage(src), nil
	}

	scale := float64(width) / float64(srcW)
	height := int(math.Round(float64(srcH) * scale))
	if height < 1 {
		height = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcY := srcBounds.Min.Y + (y*srcH)/height
		for x := 0; x < width; x++ {
			srcX := srcBounds.Min.X + (x*srcW)/width
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst, nil
}

func watermarkText(src image.Image, wm *domain.Watermark) (image.Image, error) {
	if wm == nil {
		return nil, errors.New("watermark action requires watermark settings")
	}
	text := strings.TrimSpace(wm.Text)
	if text == "" {
		return nil, errors.New("watermark action requires watermark.text")
	}

	opacity := wm.Opacity
	if opacity <= 0 {
		opacity = 0.65
	}
	if opacity > 1 {
		opacity = 1
	}

	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)

	face := basicfont.Face7x13
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	height := metrics.Height.Ceil()

	drawer := &font.Drawer{
		Dst:  dst,
		Face: face,
	}
	width := drawer.MeasureString(text).Ceil()

	x, baselineY := watermarkPosition(dst.Bounds(), width, height, ascent, wm.Gravity)

	alpha := uint8(math.Round(opacity * 255))
	drawer.Src = image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: alpha})
	drawer.Dot = fixed.P(x, baselineY)
	drawer.DrawString(text)

	return dst, nil
}

func watermarkPosition(bounds image.Rectangle, textWidth, textHeight, ascent int, gravity string) (int, int) {
	const pad = 12

	minX, minY := bounds.Min.X, bounds.Min.Y
	maxX, maxY := bounds.Max.X, bounds.Max.Y
	availW := maxX - minX
	availH := maxY - minY

	leftX := minX + pad
	centerX := minX + (availW-textWidth)/2
	rightX := maxX - textWidth - pad

	topBaseline := minY + pad + ascent
	centerBaseline := minY + (availH-textHeight)/2 + ascent
	bottomBaseline := maxY - pad

	gravity = strings.ToLower(strings.TrimSpace(gravity))
	switch gravity {
	case "northwest":
		return clamp(leftX, minX, maxX), clamp(topBaseline, minY+ascent, maxY)
	case "north":
		return clamp(centerX, minX, maxX), clamp(topBaseline, minY+ascent, maxY)
	case "northeast":
		return clamp(rightX, minX, maxX), clamp(topBaseline, minY+ascent, maxY)
	case "west":
		return clamp(leftX, minX, maxX), clamp(centerBaseline, minY+ascent, maxY)
	case "center":
		return clamp(centerX, minX, maxX), clamp(centerBaseline, minY+ascent, maxY)
	case "east":
		return clamp(rightX, minX, maxX), clamp(centerBaseline, minY+ascent, maxY)
	case "southwest":
		return clamp(leftX, minX, maxX), clamp(bottomBaseline, minY+ascent, maxY)
	case "south":
		return clamp(centerX, minX, maxX), clamp(bottomBaseline, minY+ascent, maxY)
	default:
		return clamp(rightX, minX, maxX), clamp(bottomBaseline, minY+ascent, maxY)
	}
}

func encodeImage(img image.Image, format string, quality int) ([]byte, error) {
	var buf bytes.Buffer

	switch format {
	case "jpeg":
		if quality <= 0 || quality > 100 {
			quality = 80
		}
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
	case "png":
		encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
		if err := encoder.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("encode png: %w", err)
		}
	case "webp":
		return nil, errors.New("webp export requires govips build tag")
	default:
		return nil, fmt.Errorf("unsupported output format: %s", format)
	}

	return buf.Bytes(), nil
}

func cloneImage(src image.Image) image.Image {
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
