package render

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestPNGIsDeterministicAndDrawsBorder(t *testing.T) {
	spec := Spec{Width: 256, Height: 256, Lines: []string{"scheme: WebMercatorQuad", "z/x/y: 2/1/1", "time: 2026-08-20"}}
	first, err := PNG(spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PNG(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("same render request produced different PNG bytes")
	}
	img, err := png.Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatalf("unexpected dimensions: %v", img.Bounds())
	}
	for _, point := range [][2]int{{0, 0}, {255, 0}, {0, 255}, {255, 255}, {128, 0}} {
		r, g, b, a := img.At(point[0], point[1]).RGBA()
		if r != 0xffff || g != 0xffff || b != 0 || a != 0xffff {
			t.Fatalf("border pixel %v is not yellow: %x %x %x %x", point, r, g, b, a)
		}
	}
	_, _, _, centerAlpha := img.At(128, 128).RGBA()
	if centerAlpha != 0 && centerAlpha != 0xffff {
		t.Fatalf("unexpected alpha at center: %x", centerAlpha)
	}
}

func TestPNGShrinksTextToFit(t *testing.T) {
	lines := []string{
		"service: WMS 1.3.0",
		"layers: debug",
		"crs: EPSG:3857",
		"bbox: -20037508.342789244,-20037508.342789244,20037508.342789244,20037508.342789244",
		"size: 96x64",
		"custom: a deliberately long parameter value that needs wrapping",
	}
	data, err := PNG(Spec{Width: 96, Height: 64, Lines: lines})
	if err != nil {
		t.Fatalf("adaptive layout failed: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	interiorYellow := 0
	for y := 2; y < img.Bounds().Dy()-2; y++ {
		for x := 2; x < img.Bounds().Dx()-2; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if r > 0 && g > 0 && b == 0 && a > 0 {
				interiorYellow++
			}
		}
	}
	if interiorYellow == 0 {
		t.Fatal("rendered image has no text pixels inside the border")
	}

	ttf, err := monoFont()
	if err != nil {
		t.Fatal(err)
	}
	fitted, err := fitLayout(ttf, lines, 80, 48)
	if err != nil {
		t.Fatal(err)
	}
	defer fitted.face.Close()
	if fitted.height > 48 {
		t.Fatalf("layout exceeds available height: %d", fitted.height)
	}
	actual := strings.ReplaceAll(strings.Join(fitted.lines, ""), " ", "")
	expected := strings.ReplaceAll(strings.Join(lines, ""), " ", "")
	if actual != expected {
		t.Fatalf("adaptive layout lost text:\nwant %q\n got %q", expected, actual)
	}
}
