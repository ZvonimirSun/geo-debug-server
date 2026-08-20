package render

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	yellow     = color.RGBA{R: 255, G: 255, B: 0, A: 255}
	parsedFont *opentype.Font
	fontErr    error
	fontOnce   sync.Once
)

type Spec struct {
	Width  int
	Height int
	Lines  []string
}

type layout struct {
	face       font.Face
	lines      []string
	lineHeight int
	ascent     int
	height     int
}

func PNG(spec Spec) ([]byte, error) {
	if spec.Width < 8 || spec.Height < 8 {
		return nil, fmt.Errorf("image dimensions must be at least 8x8")
	}
	if len(spec.Lines) == 0 {
		spec.Lines = []string{"debug"}
	}
	ttf, err := monoFont()
	if err != nil {
		return nil, err
	}

	img := image.NewRGBA(image.Rect(0, 0, spec.Width, spec.Height))
	drawBorder(img, 2, yellow)

	availableWidth := max(1, spec.Width-16)
	availableHeight := max(1, spec.Height-16)
	result, err := fitLayout(ttf, spec.Lines, availableWidth, availableHeight)
	if err != nil {
		return nil, err
	}
	defer result.face.Close()

	startY := (spec.Height-result.height)/2 + result.ascent
	drawer := &font.Drawer{Dst: img, Src: image.NewUniform(yellow), Face: result.face}
	for index, line := range result.lines {
		advance := drawer.MeasureString(line).Ceil()
		x := (spec.Width - advance) / 2
		y := startY + index*result.lineHeight
		drawer.Dot = fixed.P(x, y)
		drawer.DrawString(line)
	}

	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		return nil, fmt.Errorf("encode debug PNG: %w", err)
	}
	return output.Bytes(), nil
}

func monoFont() (*opentype.Font, error) {
	fontOnce.Do(func() {
		parsedFont, fontErr = opentype.Parse(gomono.TTF)
	})
	if fontErr != nil {
		return nil, fmt.Errorf("parse embedded font: %w", fontErr)
	}
	return parsedFont, nil
}

func fitLayout(ttf *opentype.Font, source []string, width, height int) (layout, error) {
	startSize := math.Min(18, math.Max(4, float64(height)/3))
	for size := startSize; size >= 1; size -= 0.5 {
		candidate, err := makeLayout(ttf, source, width, size)
		if err != nil {
			return layout{}, err
		}
		if candidate.height <= height {
			return candidate, nil
		}
		candidate.face.Close()
	}

	// Sub-pixel font sizes keep accepted requests complete even on very small images.
	for size := 0.9; size >= 0.1; size -= 0.1 {
		candidate, err := makeLayout(ttf, source, width, size)
		if err != nil {
			return layout{}, err
		}
		if candidate.height <= height {
			return candidate, nil
		}
		candidate.face.Close()
	}
	return layout{}, fmt.Errorf("debug text cannot fit inside %dx%d pixels", width, height)
}

func makeLayout(ttf *opentype.Font, source []string, width int, size float64) (layout, error) {
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{
		Size: size,
		DPI:  72,
	})
	if err != nil {
		return layout{}, fmt.Errorf("create font face: %w", err)
	}
	var lines []string
	for _, line := range source {
		lines = append(lines, wrapLine(face, line, width)...)
	}
	metrics := face.Metrics()
	fontHeight := max(1, (metrics.Ascent + metrics.Descent).Ceil())
	lineGap := max(0, int(math.Round(size*0.15)))
	lineHeight := fontHeight + lineGap
	return layout{
		face: face, lines: lines, lineHeight: lineHeight,
		ascent: metrics.Ascent.Ceil(), height: fontHeight + max(0, len(lines)-1)*lineHeight,
	}, nil
}

func wrapLine(face font.Face, value string, width int) []string {
	if value == "" {
		return []string{""}
	}
	var result []string
	remaining := value
	for remaining != "" {
		if font.MeasureString(face, remaining).Ceil() <= width {
			result = append(result, remaining)
			break
		}
		cut := fittingPrefix(face, remaining, width)
		if cut <= 0 {
			_, runeSize := utf8.DecodeRuneInString(remaining)
			cut = runeSize
		}
		result = append(result, strings.TrimRight(remaining[:cut], " "))
		remaining = strings.TrimLeft(remaining[cut:], " ")
	}
	return result
}

func fittingPrefix(face font.Face, value string, width int) int {
	lastFit := 0
	lastBreak := 0
	for offset, r := range value {
		end := offset + utf8.RuneLen(r)
		if font.MeasureString(face, value[:end]).Ceil() > width {
			if lastBreak > 0 {
				return lastBreak
			}
			return lastFit
		}
		lastFit = end
		if r == ' ' || r == ',' || r == '/' || r == '&' {
			lastBreak = end
		}
	}
	return len(value)
}

func drawBorder(img *image.RGBA, thickness int, border color.Color) {
	bounds := img.Bounds()
	for offset := 0; offset < thickness; offset++ {
		draw.Draw(img, image.Rect(offset, offset, bounds.Max.X-offset, offset+1), image.NewUniform(border), image.Point{}, draw.Src)
		draw.Draw(img, image.Rect(offset, bounds.Max.Y-offset-1, bounds.Max.X-offset, bounds.Max.Y-offset), image.NewUniform(border), image.Point{}, draw.Src)
		draw.Draw(img, image.Rect(offset, offset, offset+1, bounds.Max.Y-offset), image.NewUniform(border), image.Point{}, draw.Src)
		draw.Draw(img, image.Rect(bounds.Max.X-offset-1, offset, bounds.Max.X-offset, bounds.Max.Y-offset), image.NewUniform(border), image.Point{}, draw.Src)
	}
}
