package server

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iszy/geo-debug-server/internal/config"
	"github.com/iszy/geo-debug-server/internal/store"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return New(database, config.Config{BasePath: "/geo-debug-server"})
}

func perform(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, nil)
	request.Host = "maps.example.test"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func performJSON(t *testing.T, handler http.Handler, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Host = "maps.example.test"
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestServiceIndex(t *testing.T) {
	handler := testHandler(t)
	response := perform(t, handler, http.MethodGet, "/geo-debug-server/")
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected service index status: %d %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("unexpected service index content type: %q", contentType)
	}
	if response.Header().Get("Cache-Control") != noStoreCache || response.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("unexpected service index security or cache headers: %v", response.Header())
	}
	for _, expected := range []string{
		"geo-debug-server", "XYZ", "WMTS", "WMS", "ArcGIS Tile MapServer", "ArcGIS Dynamic MapServer", "SuperMap REST Map", "1.0.0", "1.3.0 / 1.1.1", "扩展参数", "切片方案", "切片方案管理",
		"WebMercatorQuad", "WorldCRS84Quad", "CGCS2000Quad", "EPSG:3857", "CRS:84", "EPSG:4490", "y/x",
		"http://maps.example.test/geo-debug-server/xyz/{z}/{x}/{y}.png",
		"href=\"http://maps.example.test/geo-debug-server/xyz/WebMercatorQuad/0/0/0.png\"",
		"http://maps.example.test/geo-debug-server/wmts",
		"http://maps.example.test/geo-debug-server/wmts?REQUEST=GetCapabilities&amp;DPI=96",
		"href=\"http://maps.example.test/geo-debug-server/wmts/debug/default/WebMercatorQuad/0/0/0.png\"",
		"http://maps.example.test/geo-debug-server/wms",
		"http://maps.example.test/geo-debug-server/ags_tile?f=json",
		"href=\"http://maps.example.test/geo-debug-server/ags_tile/WebMercatorQuad/?f=pjson\"",
		"href=\"http://maps.example.test/geo-debug-server/ags_tile/WebMercatorQuad/tile/0/0/0\"",
		"href=\"http://maps.example.test/geo-debug-server/ags_rest?f=json\"",
		"href=\"http://maps.example.test/geo-debug-server/ags_rest/WebMercatorQuad/?f=pjson\"",
		"href=\"http://maps.example.test/geo-debug-server/ags_rest/export?bbox=-180,-90,180,90&amp;size=512,256&amp;format=png32&amp;transparent=true&amp;f=image\"",
		"href=\"http://maps.example.test/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad\"",
		"href=\"http://maps.example.test/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/WebMercatorQuad.json\"",
		"href=\"http://maps.example.test/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad/tileImage.png?width=512&amp;height=256&amp;scale=0.000001&amp;x=0&amp;y=0&amp;cacheEnabled=true&amp;transparent=true\"",
		"href=\"/iserver/manager/license.json\"",
		"href=\"http://maps.example.test/geo-debug-server/schemes\"", "POST http://maps.example.test/geo-debug-server/schemes",
		"transparent", "bgColor", "color", "time", "90.7142857142857", "ScaleDenominator",
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("service index does not contain %q", expected)
		}
	}

	withoutSlash := perform(t, handler, http.MethodGet, "/geo-debug-server")
	if withoutSlash.Code != http.StatusFound || withoutSlash.Header().Get("Location") != "/geo-debug-server/" {
		t.Fatalf("unexpected base path redirect: status=%d location=%q", withoutSlash.Code, withoutSlash.Header().Get("Location"))
	}
	head := perform(t, handler, http.MethodHead, "/geo-debug-server/")
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" {
		t.Fatalf("unexpected service index HEAD response: status=%d body=%d length=%q",
			head.Code, head.Body.Len(), head.Header().Get("Content-Length"))
	}
}

func TestServiceIndexUsesConfiguredBasePath(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "custom-base.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	handler := New(database, config.Config{BasePath: "/base-url"})
	response := perform(t, handler, http.MethodGet, "/base-url/")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "http://maps.example.test/base-url/wmts") {
		t.Fatalf("service index did not use configured base path: %d %s", response.Code, response.Body.String())
	}
	withoutSlash := perform(t, handler, http.MethodGet, "/base-url")
	if withoutSlash.Code != http.StatusFound || withoutSlash.Header().Get("Location") != "/base-url/" {
		t.Fatalf("unexpected configured base path redirect: status=%d location=%q", withoutSlash.Code, withoutSlash.Header().Get("Location"))
	}
	outsideBasePath := perform(t, handler, http.MethodGet, "/other/path")
	if outsideBasePath.Code != http.StatusFound || outsideBasePath.Header().Get("Location") != "/base-url/" {
		t.Fatalf("unexpected outside base path redirect: status=%d location=%q", outsideBasePath.Code, outsideBasePath.Header().Get("Location"))
	}
	if outsideBasePath.Header().Get("Cache-Control") != noStoreCache {
		t.Fatalf("outside base path redirect must not be cached: %q", outsideBasePath.Header().Get("Cache-Control"))
	}
	unknownService := perform(t, handler, http.MethodGet, "/base-url/unknown")
	if unknownService.Code != http.StatusNotFound {
		t.Fatalf("unknown path within base path must return 404, got %d", unknownService.Code)
	}
}

func TestXYZForLeaflet(t *testing.T) {
	handler := testHandler(t)
	response := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/2/1/1.png?time=demo")
	assertPNG(t, response, 256, 256)
	if response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing permissive CORS header")
	}
	if response.Header().Get("Access-Control-Allow-Headers") != "*" {
		t.Fatal("CORS does not allow arbitrary request headers")
	}
	if response.Header().Get("Access-Control-Expose-Headers") != "*" {
		t.Fatal("CORS does not expose arbitrary response headers")
	}

	crs84 := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/WorldCRS84Quad/1/1/0.png")
	assertPNG(t, crs84, 256, 256)
	cgcs2000 := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/CGCS2000Quad/1/1/0.png")
	assertPNG(t, cgcs2000, 256, 256)
	outOfRange := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/WorldCRS84Quad/0/1/0.png")
	if outOfRange.Code != http.StatusBadRequest {
		t.Fatalf("expected out-of-range status 400, got %d", outOfRange.Code)
	}
}

func TestTileLinesOnlyContainCoordinatesAndOptionalTime(t *testing.T) {
	withoutTime := tileLines("3", 4, 2, "")
	if actual := strings.Join(withoutTime, "|"); actual != "z: 3|x: 4|y: 2" {
		t.Fatalf("unexpected default tile lines: %q", actual)
	}
	withTime := tileLines("3", 4, 2, "step-1")
	if actual := strings.Join(withTime, "|"); actual != "z: 3|x: 4|y: 2|time: step-1" {
		t.Fatalf("unexpected timed tile lines: %q", actual)
	}
}

func TestTileRenderColors(t *testing.T) {
	handler := testHandler(t)

	transparent := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/0/0/0.png")
	transparentImage, err := png.Decode(bytes.NewReader(transparent.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, alpha := transparentImage.At(3, 3).RGBA(); alpha != 0 {
		t.Fatalf("default tile background is not transparent: alpha=%x", alpha)
	}

	fallback := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/0/0/0.png?transparent=false")
	fallbackImage, err := png.Decode(bytes.NewReader(fallback.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if actual := color.RGBAModel.Convert(fallbackImage.At(3, 3)).(color.RGBA); actual != (color.RGBA{A: 0x80}) {
		t.Fatalf("unexpected fallback background: %#v", actual)
	}

	custom := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/0/0/0.png?TrAnSpArEnT=FALSE&BgCoLoR=123456&CoLoR=00FF00")
	customImage, err := png.Decode(bytes.NewReader(custom.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if actual := color.RGBAModel.Convert(customImage.At(3, 3)).(color.RGBA); actual != (color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}) {
		t.Fatalf("unexpected custom background: %#v", actual)
	}
	greenPixels := 0
	for y := 2; y < customImage.Bounds().Dy()-2; y++ {
		for x := 2; x < customImage.Bounds().Dx()-2; x++ {
			pixel := color.RGBAModel.Convert(customImage.At(x, y)).(color.RGBA)
			if pixel.G > pixel.R && pixel.G > pixel.B {
				greenPixels++
			}
		}
	}
	if greenPixels == 0 {
		t.Fatal("tile contains no custom-colored text")
	}
}

func TestParseHexColorFormats(t *testing.T) {
	fallback := color.RGBA{R: 1, G: 2, B: 3, A: 4}
	tests := map[string]color.RGBA{
		"abc":      {R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff},
		"abc8":     {R: 0xaa, G: 0xbb, B: 0xcc, A: 0x88},
		"123456":   {R: 0x12, G: 0x34, B: 0x56, A: 0xff},
		"12345678": {R: 0x12, G: 0x34, B: 0x56, A: 0x78},
		"invalid":  fallback,
		"#fff":     fallback,
	}
	for value, expected := range tests {
		t.Run(value, func(t *testing.T) {
			if actual := parseHexColor(value, fallback); actual != expected {
				t.Fatalf("parseHexColor(%q) = %#v, want %#v", value, actual, expected)
			}
		})
	}
}

func TestWMTSKVPDefaultsAndRequiredCoordinates(t *testing.T) {
	handler := testHandler(t)
	response := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts?TILEMATRIX=1&TILEROW=0&TILECOL=1")
	assertPNG(t, response, 256, 256)

	missing := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts?REQUEST=GetTile&TILEMATRIX=1")
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "TILEROW") {
		t.Fatalf("unexpected missing-coordinate response: %d %s", missing.Code, missing.Body.String())
	}
}

func TestWMTSRESTPaths(t *testing.T) {
	handler := testHandler(t)
	paths := []string{
		"/geo-debug-server/wmts/3/2/4.png",
		"/geo-debug-server/wmts/WorldCRS84Quad/1/0/1.png",
		"/geo-debug-server/wmts/debug/default/WebMercatorQuad/3/2/4.png",
	}
	for _, path := range paths {
		response := perform(t, handler, http.MethodGet, path)
		assertPNG(t, response, 256, 256)
	}
}

func TestWMSGetMapForLeaflet(t *testing.T) {
	handler := testHandler(t)
	target := "/geo-debug-server/wms?SERVICE=WMS&REQUEST=GetMap&VERSION=1.3.0&LAYERS=debug&CRS=EPSG:3857&BBOX=0,0,10,10&WIDTH=320&HEIGHT=180&FORMAT=image/png"
	response := perform(t, handler, http.MethodGet, target)
	assertPNG(t, response, 320, 180)

	defaults := perform(t, handler, http.MethodGet, "/geo-debug-server/wms?REQUEST=GetMap")
	assertPNG(t, defaults, 256, 256)
}

func TestAGSTileMetadata(t *testing.T) {
	handler := testHandler(t)
	compact := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_tile?f=json")
	if compact.Code != http.StatusOK || compact.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected AGS metadata response: %d %v %s", compact.Code, compact.Header(), compact.Body.String())
	}
	var metadata arcGISTileMetadata
	if err := json.Unmarshal(compact.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.MapName != "CGCS2000 Quad" || !metadata.SingleFusedMapCache {
		t.Fatalf("unexpected AGS service metadata: %+v", metadata)
	}
	if metadata.SpatialReference.WKID != 4490 || metadata.SpatialReference.LatestWKID != 4490 {
		t.Fatalf("unexpected AGS spatial reference: %+v", metadata.SpatialReference)
	}
	if metadata.TileInfo.Rows != 256 || metadata.TileInfo.Cols != 256 || metadata.TileInfo.DPI != 96 || metadata.TileInfo.Format != "PNG32" {
		t.Fatalf("unexpected AGS tile info: %+v", metadata.TileInfo)
	}
	if len(metadata.TileInfo.LODs) != 24 || metadata.TileInfo.LODs[0].Level != 0 {
		t.Fatalf("unexpected AGS LODs: %+v", metadata.TileInfo.LODs)
	}
	expectedScale := scaleDenominator(1.40625, 111319.49079327358, arcGISTileDPI)
	if math.Abs(metadata.TileInfo.LODs[0].Scale-expectedScale) > expectedScale*1e-12 {
		t.Fatalf("unexpected AGS level 0 scale: got %.12f want %.12f", metadata.TileInfo.LODs[0].Scale, expectedScale)
	}
	if metadata.FullExtent.XMin != -180 || metadata.FullExtent.XMax != 180 {
		t.Fatalf("unexpected AGS full extent: %+v", metadata.FullExtent)
	}

	pretty := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_tile?F=PJSON")
	if pretty.Code != http.StatusOK || !strings.Contains(pretty.Body.String(), "\n  \"tileInfo\"") {
		t.Fatalf("unexpected AGS PJSON response: %d %s", pretty.Code, pretty.Body.String())
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, pretty.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	if compacted.String() != compact.Body.String() {
		t.Fatal("AGS JSON and PJSON metadata differ")
	}

	crs84 := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_tile/WorldCRS84Quad/?f=json")
	var crs84Metadata arcGISTileMetadata
	if crs84.Code != http.StatusOK {
		t.Fatalf("unexpected specified-scheme metadata status: %d %s", crs84.Code, crs84.Body.String())
	}
	if err := json.Unmarshal(crs84.Body.Bytes(), &crs84Metadata); err != nil {
		t.Fatal(err)
	}
	if crs84Metadata.SpatialReference.WKID != 4326 || len(crs84Metadata.TileInfo.LODs) != 24 || crs84Metadata.TileInfo.Origin.X != -180 {
		t.Fatalf("unexpected CRS84 AGS metadata: %+v", crs84Metadata)
	}

	head := perform(t, handler, http.MethodHead, "/geo-debug-server/ags_tile/WebMercatorQuad/?f=json")
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" {
		t.Fatalf("unexpected AGS metadata HEAD response: status=%d body=%d length=%q", head.Code, head.Body.Len(), head.Header().Get("Content-Length"))
	}
	missingFormat := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_tile")
	if missingFormat.Code != http.StatusBadRequest || missingFormat.Header().Get("Content-Type") != "application/json; charset=utf-8" || !strings.Contains(missingFormat.Body.String(), "\"error\"") {
		t.Fatalf("unexpected AGS missing format response: %d %s", missingFormat.Code, missingFormat.Body.String())
	}
	unknownScheme := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_tile/unknown/?f=pjson")
	if unknownScheme.Code != http.StatusNotFound || !strings.Contains(unknownScheme.Body.String(), "unknown tile scheme") {
		t.Fatalf("unexpected AGS unknown-scheme response: %d %s", unknownScheme.Code, unknownScheme.Body.String())
	}
}

func TestAGSTilePaths(t *testing.T) {
	handler := testHandler(t)
	for _, path := range []string{
		"/geo-debug-server/ags_tile/tile/3/2/4",
		"/geo-debug-server/ags_tile/WorldCRS84Quad/tile/1/0/1",
		"/geo-debug-server/ags_tile/CGCS2000Quad/tile/1/0/1?time=demo",
	} {
		response := perform(t, handler, http.MethodGet, path)
		assertPNG(t, response, 256, 256)
	}
	outOfRange := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_tile/tile/0/0/1")
	if outOfRange.Code != http.StatusBadRequest {
		t.Fatalf("expected AGS out-of-range status 400, got %d", outOfRange.Code)
	}
	unknownScheme := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_tile/unknown/tile/0/0/0")
	if unknownScheme.Code != http.StatusNotFound || !strings.Contains(unknownScheme.Body.String(), "unknown tile scheme") {
		t.Fatalf("unexpected AGS tile unknown-scheme response: %d %s", unknownScheme.Code, unknownScheme.Body.String())
	}
}

func TestAGSRestMetadata(t *testing.T) {
	handler := testHandler(t)
	compact := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_rest?f=json")
	if compact.Code != http.StatusOK || compact.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected AGS dynamic metadata response: %d %v %s", compact.Code, compact.Header(), compact.Body.String())
	}
	var metadata arcGISDynamicMetadata
	if err := json.Unmarshal(compact.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.MapName != "CGCS2000 Quad" || !metadata.SupportsDynamicLayers || metadata.SingleFusedMapCache {
		t.Fatalf("unexpected AGS dynamic service metadata: %+v", metadata)
	}
	if metadata.SpatialReference.WKID != 4490 || metadata.SpatialReference.LatestWKID != 4490 {
		t.Fatalf("unexpected AGS dynamic spatial reference: %+v", metadata.SpatialReference)
	}
	if strings.Contains(compact.Body.String(), "\"tileInfo\"") {
		t.Fatal("dynamic MapServer metadata must not include tileInfo")
	}

	pretty := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_rest?F=PJSON")
	if pretty.Code != http.StatusOK || !strings.Contains(pretty.Body.String(), "\n  \"supportsDynamicLayers\"") {
		t.Fatalf("unexpected AGS dynamic PJSON response: %d %s", pretty.Code, pretty.Body.String())
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, pretty.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	if compacted.String() != compact.Body.String() {
		t.Fatal("AGS dynamic JSON and PJSON metadata differ")
	}

	mercator := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_rest/WebMercatorQuad/?f=json")
	var mercatorMetadata arcGISDynamicMetadata
	if err := json.Unmarshal(mercator.Body.Bytes(), &mercatorMetadata); err != nil {
		t.Fatal(err)
	}
	if mercator.Code != http.StatusOK || mercatorMetadata.SpatialReference.WKID != 102100 || mercatorMetadata.SpatialReference.LatestWKID != 3857 {
		t.Fatalf("unexpected specified-scheme AGS dynamic metadata: %d %+v", mercator.Code, mercatorMetadata)
	}

	head := perform(t, handler, http.MethodHead, "/geo-debug-server/ags_rest?f=json")
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" {
		t.Fatalf("unexpected AGS dynamic metadata HEAD response: status=%d body=%d length=%q", head.Code, head.Body.Len(), head.Header().Get("Content-Length"))
	}
	for name, target := range map[string]string{
		"missing format": "/geo-debug-server/ags_rest",
		"invalid format": "/geo-debug-server/ags_rest?f=html",
		"unknown scheme": "/geo-debug-server/ags_rest/unknown/?f=pjson",
	} {
		t.Run(name, func(t *testing.T) {
			response := perform(t, handler, http.MethodGet, target)
			if response.Code != http.StatusBadRequest && response.Code != http.StatusNotFound {
				t.Fatalf("unexpected AGS dynamic error response: %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Content-Type") != "application/json; charset=utf-8" || !strings.Contains(response.Body.String(), "\"error\"") {
				t.Fatalf("unexpected AGS dynamic error payload: %v %s", response.Header(), response.Body.String())
			}
		})
	}
}

func TestAGSRestExport(t *testing.T) {
	handler := testHandler(t)
	imageTarget := "/geo-debug-server/ags_rest/export?bbox=-180,-90,180,90&size=320,180&bboxSR=4490&imageSR=4490&dpi=96&format=png32&transparent=true&time=demo&custom=value&f=image"
	imageResponse := perform(t, handler, http.MethodGet, imageTarget)
	assertPNG(t, imageResponse, 320, 180)
	if imageResponse.Header().Get("Cache-Control") != immutableImageCache {
		t.Fatalf("unexpected AGS dynamic image cache header: %q", imageResponse.Header().Get("Cache-Control"))
	}

	jsonTarget := "/geo-debug-server/ags_rest/export?bbox=-180,-90,180,90&size=320,180&bboxSR=4490&imageSR=4490&dpi=96&format=png32&F=PJSON"
	request := httptest.NewRequest(http.MethodGet, jsonTarget, nil)
	request.Host = "internal:8080"
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "maps.example.test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected AGS export result: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `\u0026`) || !strings.Contains(response.Body.String(), `&f=image&`) {
		t.Fatalf("AGS export href is not directly copyable: %s", response.Body.String())
	}
	var result arcGISExportResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Width != 320 || result.Height != 180 || result.Extent.SpatialReference.WKID != 4490 || result.Scale <= 0 {
		t.Fatalf("unexpected AGS export result fields: %+v", result)
	}
	if !strings.HasPrefix(result.Href, "https://maps.example.test/geo-debug-server/ags_rest/export?") || strings.Count(strings.ToLower(result.Href), "f=image") != 1 || strings.Contains(strings.ToLower(result.Href), "f=pjson") {
		t.Fatalf("unexpected AGS export image URL: %q", result.Href)
	}
	imageFromResult := perform(t, handler, http.MethodGet, strings.TrimPrefix(result.Href, "https://maps.example.test"))
	assertPNG(t, imageFromResult, 320, 180)

	mercator := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_rest/WebMercatorQuad/export?bbox=-1000,-500,1000,500&size=256,128&f=json")
	var mercatorResult arcGISExportResult
	if err := json.Unmarshal(mercator.Body.Bytes(), &mercatorResult); err != nil {
		t.Fatal(err)
	}
	if mercator.Code != http.StatusOK || mercatorResult.Extent.SpatialReference.WKID != 102100 {
		t.Fatalf("unexpected specified-scheme AGS export result: %d %+v", mercator.Code, mercatorResult)
	}

	for name, query := range map[string]string{
		"bbox":   "bbox=0,0,0,1&f=image",
		"size":   "size=4097,256&f=image",
		"dpi":    "dpi=invalid&f=image",
		"format": "format=jpg&f=image",
		"output": "f=html",
	} {
		t.Run(name, func(t *testing.T) {
			invalid := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_rest/export?"+query)
			if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "\"error\"") {
				t.Fatalf("unexpected invalid AGS export response: %d %s", invalid.Code, invalid.Body.String())
			}
		})
	}
}

func TestSuperMapRESTMetadata(t *testing.T) {
	handler := testHandler(t)
	for _, target := range []string{
		"/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad",
		"/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad.json",
	} {
		response := perform(t, handler, http.MethodGet, target)
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("unexpected SuperMap metadata response: %d %v %s", response.Code, response.Header(), response.Body.String())
		}
		var metadata superMapMetadata
		if err := json.Unmarshal(response.Body.Bytes(), &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata.Name != store.CGCS2000Quad || metadata.PrjCoordSys.EPSGCode != 4490 ||
			metadata.Bounds.Left != -180 || metadata.Bounds.Top != 90 || metadata.Origin.X != -180 || metadata.Origin.Y != 90 ||
			metadata.TileWidth != 256 || metadata.TileHeight != 256 || len(metadata.Resolutions) != 24 {
			t.Fatalf("unexpected SuperMap metadata: %+v", metadata)
		}
	}

	mercator := perform(t, handler, http.MethodGet, "/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/WebMercatorQuad.json")
	if mercator.Code != http.StatusOK {
		t.Fatalf("unexpected projected SuperMap metadata response: %d %s", mercator.Code, mercator.Body.String())
	}
	var mercatorMetadata superMapMetadata
	if err := json.Unmarshal(mercator.Body.Bytes(), &mercatorMetadata); err != nil {
		t.Fatal(err)
	}
	if mercatorMetadata.PrjCoordSys.EPSGCode != 3857 || mercatorMetadata.CoordUnit != "METER" ||
		mercatorMetadata.DistanceUnit != "METER" || mercatorMetadata.ViewBounds.Left != -20037508.342789244 ||
		mercatorMetadata.ViewBounds.Right != 20037508.342789244 || mercatorMetadata.DynamicProject ||
		mercatorMetadata.Origin.X != -20037508.342789244 || len(mercatorMetadata.Resolutions) != 23 {
		t.Fatalf("unexpected EPSG:3857 SuperMap metadata: %+v", mercatorMetadata)
	}
	head := perform(t, handler, http.MethodHead, "/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad")
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" {
		t.Fatalf("unexpected SuperMap metadata HEAD response: status=%d body=%d length=%q", head.Code, head.Body.Len(), head.Header().Get("Content-Length"))
	}
	invalid := perform(t, handler, http.MethodGet, "/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/unknown")
	if invalid.Code != http.StatusNotFound || !strings.Contains(invalid.Body.String(), "\"succeed\": false") {
		t.Fatalf("unexpected SuperMap invalid-path response: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestSuperMapRESTTileImage(t *testing.T) {
	handler := testHandler(t)
	target := "/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad/tileImage.png?width=320&height=180&scale=0.000001&x=-20&y=10&origin=-180,90&bounds=-180,-90,180,90&layersID=show:0&cacheEnabled=true&redirect=false&time=demo&custom=value"
	response := perform(t, handler, http.MethodGet, target)
	assertPNG(t, response, 320, 180)
	if response.Header().Get("Cache-Control") != immutableImageCache {
		t.Fatalf("unexpected SuperMap image cache header: %q", response.Header().Get("Cache-Control"))
	}

	defaults := perform(t, handler, http.MethodGet, "/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/WebMercatorQuad/tileImage.png")
	assertPNG(t, defaults, 256, 256)

	for name, target := range map[string]string{
		"width":  "/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad/tileImage.png?width=0",
		"height": "/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad/tileImage.png?height=4097",
		"path":   "/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad/image.png",
	} {
		t.Run(name, func(t *testing.T) {
			invalid := perform(t, handler, http.MethodGet, target)
			if invalid.Code != http.StatusBadRequest && invalid.Code != http.StatusNotFound {
				t.Fatalf("unexpected invalid SuperMap response: %d %s", invalid.Code, invalid.Body.String())
			}
		})
	}
}

func TestSuperMapLicenseCompatibilityPath(t *testing.T) {
	handler := testHandler(t)
	response := perform(t, handler, http.MethodGet, superMapLicensePath)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected SuperMap license response: %d %v %s", response.Code, response.Header(), response.Body.String())
	}
	if response.Header().Get("Cache-Control") != noStoreCache {
		t.Fatalf("SuperMap license response must not be cached: %q", response.Header().Get("Cache-Control"))
	}
	var license map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &license); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"iServerStandard", "iServerSpatialStreaming", "iServerPlot", "iServerProfessional",
		"iServerEnterprise", "iServerSpatialProcessing", "iServerBasic", "trialVersion", "iServerUltra",
	} {
		if enabled, ok := license[key].(bool); !ok || !enabled {
			t.Fatalf("SuperMap license %q is not enabled: %#v", key, license[key])
		}
	}
	if license["productType"] != "iServer" {
		t.Fatalf("unexpected SuperMap product type: %#v", license["productType"])
	}
	remote, ok := license["remoteSensingLicenseTypeStruct"].(map[string]any)
	if !ok || len(remote) != 7 {
		t.Fatalf("unexpected remote sensing license structure: %#v", license["remoteSensingLicenseTypeStruct"])
	}
	for key, value := range remote {
		if enabled, ok := value.(bool); !ok || !enabled {
			t.Fatalf("remote sensing license %q is not enabled: %#v", key, value)
		}
	}
	if builder, ok := license["builder"].(map[string]any); !ok || len(builder) != 0 {
		t.Fatalf("unexpected builder license value: %#v", license["builder"])
	}

	head := perform(t, handler, http.MethodHead, superMapLicensePath)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" {
		t.Fatalf("unexpected SuperMap license HEAD response: status=%d body=%d length=%q", head.Code, head.Body.Len(), head.Header().Get("Content-Length"))
	}
	post := perform(t, handler, http.MethodPost, superMapLicensePath)
	if post.Code != http.StatusMethodNotAllowed || !strings.Contains(post.Header().Get("Allow"), "GET") {
		t.Fatalf("unexpected SuperMap license method response: %d %v %s", post.Code, post.Header(), post.Body.String())
	}

	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "license-custom-base.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	customBase := New(database, config.Config{BasePath: "/base-url"})
	global := perform(t, customBase, http.MethodGet, superMapLicensePath)
	if global.Code != http.StatusOK {
		t.Fatalf("global SuperMap license path depends on base path: %d %s", global.Code, global.Body.String())
	}
	insideBase := perform(t, customBase, http.MethodGet, "/base-url"+superMapLicensePath)
	if insideBase.Code != http.StatusNotFound {
		t.Fatalf("base-prefixed SuperMap license path must not be served: %d %s", insideBase.Code, insideBase.Body.String())
	}
}

func TestSchemeManagementAPI(t *testing.T) {
	handler := testHandler(t)
	list := perform(t, handler, http.MethodGet, "/geo-debug-server/schemes")
	if list.Code != http.StatusOK || list.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected scheme list response: %d %v %s", list.Code, list.Header(), list.Body.String())
	}
	var initial []store.Scheme
	if err := json.Unmarshal(list.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if len(initial) != 3 || initial[0].ID != store.CGCS2000Quad || !initial[0].IsDefault {
		t.Fatalf("unexpected initial scheme list: %+v", initial)
	}
	head := perform(t, handler, http.MethodHead, "/geo-debug-server/schemes")
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" {
		t.Fatalf("unexpected scheme list HEAD response: status=%d body=%d length=%q", head.Code, head.Body.Len(), head.Header().Get("Content-Length"))
	}

	scheme := managedScheme("ManagedQuad")
	body, err := json.Marshal(scheme)
	if err != nil {
		t.Fatal(err)
	}
	created := performJSON(t, handler, http.MethodPost, "/geo-debug-server/schemes", body)
	if created.Code != http.StatusCreated || created.Header().Get("Location") != "/geo-debug-server/schemes/ManagedQuad" {
		t.Fatalf("unexpected create scheme response: %d %v %s", created.Code, created.Header(), created.Body.String())
	}
	var createdScheme store.Scheme
	if err := json.Unmarshal(created.Body.Bytes(), &createdScheme); err != nil {
		t.Fatal(err)
	}
	if createdScheme.ID != scheme.ID || createdScheme.IsDefault || len(createdScheme.Levels) != 2 {
		t.Fatalf("unexpected created scheme: %+v", createdScheme)
	}

	duplicate := scheme
	duplicate.ID = strings.ToLower(duplicate.ID)
	duplicateBody, _ := json.Marshal(duplicate)
	conflict := performJSON(t, handler, http.MethodPost, "/geo-debug-server/schemes", duplicateBody)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "already exists") {
		t.Fatalf("unexpected duplicate scheme response: %d %s", conflict.Code, conflict.Body.String())
	}
	invalid := performJSON(t, handler, http.MethodPost, "/geo-debug-server/schemes", []byte(`{"id":"broken"}`))
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "invalid tile scheme") {
		t.Fatalf("unexpected invalid scheme response: %d %s", invalid.Code, invalid.Body.String())
	}

	setDefault := perform(t, handler, http.MethodPut, "/geo-debug-server/schemes/managedquad/default")
	if setDefault.Code != http.StatusOK {
		t.Fatalf("unexpected set default response: %d %s", setDefault.Code, setDefault.Body.String())
	}
	var defaultScheme store.Scheme
	if err := json.Unmarshal(setDefault.Body.Bytes(), &defaultScheme); err != nil {
		t.Fatal(err)
	}
	if defaultScheme.ID != scheme.ID || !defaultScheme.IsDefault {
		t.Fatalf("unexpected default scheme response: %+v", defaultScheme)
	}
	metadata := perform(t, handler, http.MethodGet, "/geo-debug-server/ags_tile?f=json")
	var agsMetadata arcGISTileMetadata
	if err := json.Unmarshal(metadata.Body.Bytes(), &agsMetadata); err != nil {
		t.Fatal(err)
	}
	if agsMetadata.MapName != scheme.Name {
		t.Fatalf("default scheme cache was not invalidated: %+v", agsMetadata)
	}

	deleted := perform(t, handler, http.MethodDelete, "/geo-debug-server/schemes/MANAGEDQUAD")
	if deleted.Code != http.StatusNoContent || deleted.Body.Len() != 0 {
		t.Fatalf("unexpected delete scheme response: %d %s", deleted.Code, deleted.Body.String())
	}
	metadata = perform(t, handler, http.MethodGet, "/geo-debug-server/ags_tile?f=json")
	if err := json.Unmarshal(metadata.Body.Bytes(), &agsMetadata); err != nil {
		t.Fatal(err)
	}
	if agsMetadata.MapName != "CGCS2000 Quad" {
		t.Fatalf("deleting the default did not select a replacement: %+v", agsMetadata)
	}
	missing := perform(t, handler, http.MethodDelete, "/geo-debug-server/schemes/missing")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unexpected missing scheme deletion response: %d %s", missing.Code, missing.Body.String())
	}

	patch := perform(t, handler, http.MethodPatch, "/geo-debug-server/schemes")
	if patch.Code != http.StatusMethodNotAllowed || !strings.Contains(patch.Header().Get("Allow"), "POST") {
		t.Fatalf("unexpected scheme method response: %d %v %s", patch.Code, patch.Header(), patch.Body.String())
	}
	xyzPost := perform(t, handler, http.MethodPost, "/geo-debug-server/xyz/0/0/0.png")
	if xyzPost.Code != http.StatusMethodNotAllowed || !strings.Contains(xyzPost.Header().Get("Content-Type"), "application/xml") {
		t.Fatalf("write method leaked into tile endpoint: %d %v %s", xyzPost.Code, xyzPost.Header(), xyzPost.Body.String())
	}
}

func managedScheme(id string) store.Scheme {
	return store.Scheme{
		ID: id, Name: "Managed Quad", CRS: "EPSG:4326", MetersPerUnit: 111319.49079327358,
		TileWidth: 256, TileHeight: 256, MinZoom: 0, MaxZoom: 1,
		OriginX: -180, OriginY: 90, MinX: -180, MinY: -90, MaxX: 180, MaxY: 90,
		YCoordinateFirst: true,
		Levels: []store.MatrixLevel{
			{Zoom: 0, Identifier: "0", Resolution: 1.40625, MatrixWidth: 1, MatrixHeight: 1},
			{Zoom: 1, Identifier: "1", Resolution: 0.703125, MatrixWidth: 2, MatrixHeight: 1},
		},
	}
}

func TestMetadataListsSchemesAndResourceURLs(t *testing.T) {
	handler := testHandler(t)
	wmts := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts")
	if wmts.Code != http.StatusOK {
		t.Fatalf("unexpected WMTS metadata status: %d", wmts.Code)
	}
	var wmtsDocument struct {
		XMLName xml.Name
		Version string `xml:"version,attr"`
		Service struct {
			ServiceType string `xml:"ServiceType"`
		} `xml:"ServiceIdentification"`
		Operations struct {
			Items []struct {
				Name       string `xml:"name,attr"`
				Parameters []struct {
					Name   string   `xml:"name,attr"`
					Values []string `xml:"AllowedValues>Value"`
				} `xml:"Parameter"`
				DCP struct {
					HTTP struct {
						Gets []struct {
							Href       string `xml:"http://www.w3.org/1999/xlink href,attr"`
							Constraint struct {
								Name   string   `xml:"name,attr"`
								Values []string `xml:"AllowedValues>Value"`
							} `xml:"Constraint"`
						} `xml:"Get"`
					} `xml:"HTTP"`
				} `xml:"DCP"`
			} `xml:"Operation"`
		} `xml:"OperationsMetadata"`
		Contents struct {
			Layer struct {
				Identifier  string `xml:"Identifier"`
				WGS84Bounds struct {
					LowerCorner string `xml:"LowerCorner"`
					UpperCorner string `xml:"UpperCorner"`
				} `xml:"WGS84BoundingBox"`
				Styles []struct {
					IsDefault  bool   `xml:"isDefault,attr"`
					Identifier string `xml:"Identifier"`
				} `xml:"Style"`
				Formats     []string `xml:"Format"`
				MatrixLinks []struct {
					Identifier string `xml:"TileMatrixSet"`
				} `xml:"TileMatrixSetLink"`
				ResourceURLs []struct {
					ResourceType string `xml:"resourceType,attr"`
					Template     string `xml:"template,attr"`
				} `xml:"ResourceURL"`
			} `xml:"Layer"`
			MatrixSets []struct {
				Identifier   string `xml:"Identifier"`
				SupportedCRS string `xml:"SupportedCRS"`
				Matrices     []struct {
					Identifier       string  `xml:"Identifier"`
					ScaleDenominator float64 `xml:"ScaleDenominator"`
					TopLeftCorner    string  `xml:"TopLeftCorner"`
					TileWidth        int     `xml:"TileWidth"`
					MatrixWidth      int64   `xml:"MatrixWidth"`
				} `xml:"TileMatrix"`
			} `xml:"TileMatrixSet"`
		} `xml:"Contents"`
		MetadataURLs []struct {
			Href string `xml:"http://www.w3.org/1999/xlink href,attr"`
		} `xml:"ServiceMetadataURL"`
	}
	if err := xml.Unmarshal(wmts.Body.Bytes(), &wmtsDocument); err != nil {
		t.Fatalf("WMTS metadata is not XML: %v", err)
	}
	if wmtsDocument.XMLName.Local != "Capabilities" || wmtsDocument.XMLName.Space != wmtsNamespace || wmtsDocument.Version != "1.0.0" {
		t.Fatalf("unexpected WMTS capabilities root: %+v version=%q", wmtsDocument.XMLName, wmtsDocument.Version)
	}
	body := wmts.Body.String()
	for _, expected := range []string{
		`xmlns="http://www.opengis.net/wmts/1.0"`, `xmlns:ows="http://www.opengis.net/ows/1.1"`,
		"<Contents>", "<ows:Identifier>", "WebMercatorQuad", "WorldCRS84Quad", "CGCS2000Quad", "ResourceURL",
		"<ows:AnyValue></ows:AnyValue>", "http://maps.example.test/geo-debug-server/wmts",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("WMTS metadata does not contain %q", expected)
		}
	}
	if wmtsDocument.Service.ServiceType != "OGC WMTS" || len(wmtsDocument.Operations.Items) != 2 {
		t.Fatalf("unexpected WMTS service metadata: %+v", wmtsDocument.Service)
	}
	for _, operation := range wmtsDocument.Operations.Items {
		if len(operation.DCP.HTTP.Gets) != 2 {
			t.Fatalf("expected RESTFUL and KVP endpoints for %s: %+v", operation.Name, operation.DCP.HTTP.Gets)
		}
		encodings := make([]string, 0, len(operation.DCP.HTTP.Gets))
		for _, get := range operation.DCP.HTTP.Gets {
			if get.Href != "http://maps.example.test/geo-debug-server/wmts?" {
				t.Fatalf("unexpected %s operation URL: %q", operation.Name, get.Href)
			}
			if get.Constraint.Name != "GetEncoding" || len(get.Constraint.Values) != 1 {
				t.Fatalf("unexpected %s encoding constraint: %+v", operation.Name, get.Constraint)
			}
			encodings = append(encodings, get.Constraint.Values[0])
		}
		if strings.Join(encodings, ",") != "RESTFUL,KVP" {
			t.Fatalf("unexpected %s encodings: %v", operation.Name, encodings)
		}
		parameters := make(map[string][]string, len(operation.Parameters))
		for _, parameter := range operation.Parameters {
			parameters[parameter.Name] = parameter.Values
		}
		switch operation.Name {
		case "GetCapabilities":
			if strings.Join(parameters["AcceptVersions"], ",") != "1.0.0" || strings.Join(parameters["AcceptFormats"], ",") != "application/xml" {
				t.Fatalf("incomplete GetCapabilities parameters: %+v", parameters)
			}
			if _, ok := parameters["DPI"]; !ok {
				t.Fatalf("GetCapabilities does not advertise the DPI extension: %+v", parameters)
			}
		case "GetTile":
			if strings.Join(parameters["Layer"], ",") != "debug" || strings.Join(parameters["Style"], ",") != "default" ||
				strings.Join(parameters["Format"], ",") != "image/png" || len(parameters["TileMatrixSet"]) != 3 {
				t.Fatalf("incomplete GetTile parameters: %+v", parameters)
			}
		default:
			t.Fatalf("unexpected WMTS operation: %s", operation.Name)
		}
	}
	layer := wmtsDocument.Contents.Layer
	if layer.Identifier != "debug" || len(layer.Styles) != 1 || !layer.Styles[0].IsDefault || layer.Styles[0].Identifier != "default" {
		t.Fatalf("unexpected WMTS layer identity or style: %+v", layer)
	}
	if layer.WGS84Bounds.LowerCorner != "-180 -90" || layer.WGS84Bounds.UpperCorner != "180 90" {
		t.Fatalf("WGS84BoundingBox must use longitude/latitude order: %+v", layer.WGS84Bounds)
	}
	if len(layer.Formats) != 1 || layer.Formats[0] != "image/png" || len(layer.MatrixLinks) != 3 || len(layer.ResourceURLs) != 1 {
		t.Fatalf("unexpected WMTS layer resources: %+v", layer)
	}
	resourceURL := layer.ResourceURLs[0]
	expectedTemplate := "http://maps.example.test/geo-debug-server/wmts/debug/default/{TileMatrixSet}/{TileMatrix}/{TileRow}/{TileCol}.png"
	if resourceURL.ResourceType != "tile" || resourceURL.Template != expectedTemplate || strings.Contains(resourceURL.Template, "?") {
		t.Fatalf("ResourceURL must be a REST tile template: %+v", resourceURL)
	}
	if len(wmtsDocument.Contents.MatrixSets) != 3 {
		t.Fatalf("expected three tile matrix sets, got %d", len(wmtsDocument.Contents.MatrixSets))
	}
	expectedCRS := map[string]string{
		"WebMercatorQuad": "urn:ogc:def:crs:EPSG::3857",
		"WorldCRS84Quad":  "urn:ogc:def:crs:OGC:1.3:CRS84",
		"CGCS2000Quad":    "urn:ogc:def:crs:EPSG::4490",
	}
	expectedTopLeft := map[string]string{
		"WebMercatorQuad": "-20037508.3427892 20037508.3427892",
		"WorldCRS84Quad":  "-180 90",
		"CGCS2000Quad":    "90 -180",
	}
	expectedMatrixCount := map[string]int{
		"WebMercatorQuad": 23,
		"WorldCRS84Quad":  24,
		"CGCS2000Quad":    24,
	}
	var level0Scale float64
	for _, matrixSet := range wmtsDocument.Contents.MatrixSets {
		if len(matrixSet.Matrices) != expectedMatrixCount[matrixSet.Identifier] || matrixSet.Matrices[0].Identifier != "0" ||
			matrixSet.Matrices[0].ScaleDenominator <= 0 || matrixSet.Matrices[0].TopLeftCorner == "" ||
			matrixSet.Matrices[0].TileWidth != 256 || matrixSet.Matrices[0].MatrixWidth <= 0 {
			t.Fatalf("unexpected tile matrix set: %+v", matrixSet)
		}
		if matrixSet.SupportedCRS != expectedCRS[matrixSet.Identifier] {
			t.Fatalf("unexpected supported CRS for %s: %q", matrixSet.Identifier, matrixSet.SupportedCRS)
		}
		if matrixSet.Matrices[0].TopLeftCorner != expectedTopLeft[matrixSet.Identifier] {
			t.Fatalf("unexpected top-left corner for %s: %q", matrixSet.Identifier, matrixSet.Matrices[0].TopLeftCorner)
		}
		if level0Scale == 0 {
			level0Scale = matrixSet.Matrices[0].ScaleDenominator
		} else if math.Abs(matrixSet.Matrices[0].ScaleDenominator-level0Scale) > level0Scale*1e-12 {
			t.Fatalf("level 0 scale is not aligned for %s: got %.12f want %.12f",
				matrixSet.Identifier, matrixSet.Matrices[0].ScaleDenominator, level0Scale)
		}
	}
	if len(wmtsDocument.MetadataURLs) != 2 ||
		wmtsDocument.MetadataURLs[0].Href != "http://maps.example.test/geo-debug-server/wmts?SERVICE=WMTS&REQUEST=GetCapabilities&VERSION=1.0.0" ||
		wmtsDocument.MetadataURLs[1].Href != "http://maps.example.test/geo-debug-server/wmts/1.0.0/WMTSCapabilities.xml" {
		t.Fatalf("unexpected WMTS service metadata URLs: %+v", wmtsDocument.MetadataURLs)
	}
	if strings.Contains(body, "<Extent>") || strings.Contains(body, "<KVPResourceURL>") || strings.Contains(body, "<TileMatrix id=") {
		t.Fatal("WMTS capabilities contains legacy non-standard elements")
	}
	second := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts?REQUEST=GetCapabilities")
	if !bytes.Equal(wmts.Body.Bytes(), second.Body.Bytes()) {
		t.Fatal("equivalent WMTS capabilities requests returned different metadata")
	}
	rest := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts/1.0.0/WMTSCapabilities.xml")
	if !bytes.Equal(wmts.Body.Bytes(), rest.Body.Bytes()) {
		t.Fatal("REST WMTS capabilities request returned different metadata")
	}
}

func TestWMTSMetadataDPI(t *testing.T) {
	handler := testHandler(t)
	defaultResponse := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts")
	customResponse := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts?REQUEST=GetCapabilities&DPI=96")
	assertXMLStatus(t, customResponse, http.StatusOK)

	defaultScale := wmtsFirstScale(t, defaultResponse, store.WebMercatorQuad)
	customScale := wmtsFirstScale(t, customResponse, store.WebMercatorQuad)
	expectedScale := defaultScale * 96 / defaultWMTSDPI
	if math.Abs(customScale-expectedScale) > expectedScale*1e-12 {
		t.Fatalf("unexpected 96 DPI scale denominator: got %.12f want %.12f", customScale, expectedScale)
	}
	if !strings.Contains(customResponse.Body.String(), "DPI=96") {
		t.Fatal("96 DPI capabilities does not preserve DPI in metadata URLs")
	}

	restResponse := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts/1.0.0/WMTSCapabilities.xml?dpi=96")
	if !bytes.Equal(customResponse.Body.Bytes(), restResponse.Body.Bytes()) {
		t.Fatal("KVP and REST requests produced different 96 DPI capabilities")
	}

	invalid := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts?REQUEST=GetCapabilities&DPI=invalid")
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "DPI") {
		t.Fatalf("unexpected invalid DPI response: %d %s", invalid.Code, invalid.Body.String())
	}
}

func wmtsFirstScale(t *testing.T, response *httptest.ResponseRecorder, matrixSetID string) float64 {
	t.Helper()
	assertXMLStatus(t, response, http.StatusOK)
	var document struct {
		MatrixSets []struct {
			Identifier string `xml:"Identifier"`
			Matrices   []struct {
				ScaleDenominator float64 `xml:"ScaleDenominator"`
			} `xml:"TileMatrix"`
		} `xml:"Contents>TileMatrixSet"`
	}
	if err := xml.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	for _, matrixSet := range document.MatrixSets {
		if matrixSet.Identifier == matrixSetID && len(matrixSet.Matrices) > 0 {
			return matrixSet.Matrices[0].ScaleDenominator
		}
	}
	t.Fatalf("matrix set %q has no scale denominator", matrixSetID)
	return 0
}

func assertXMLStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("expected status %d, got %d: %s", status, response.Code, response.Body.String())
	}
}

func TestWMTSCoordinatePairAxisOrder(t *testing.T) {
	if actual := wmtsCoordinatePair(true, -180, 90); actual != "90 -180" {
		t.Fatalf("y-coordinate-first scheme must use y/x order, got %q", actual)
	}
	if actual := wmtsCoordinatePair(false, -180, 90); actual != "-180 90" {
		t.Fatalf("x-coordinate-first scheme must use x/y order, got %q", actual)
	}
}

func TestWMSCapabilitiesDescribeGetMapDefaults(t *testing.T) {
	handler := testHandler(t)
	wms := perform(t, handler, http.MethodGet, "/geo-debug-server/wms?REQUEST=GetCapabilities")
	if wms.Code != http.StatusOK {
		t.Fatalf("unexpected WMS metadata: %d %s", wms.Code, wms.Body.String())
	}
	type onlineResource struct {
		Type string `xml:"http://www.w3.org/1999/xlink type,attr"`
		Href string `xml:"http://www.w3.org/1999/xlink href,attr"`
	}
	type operation struct {
		Formats []string `xml:"Format"`
		DCP     struct {
			HTTP struct {
				Get struct {
					OnlineResource onlineResource `xml:"OnlineResource"`
				} `xml:"Get"`
			} `xml:"HTTP"`
		} `xml:"DCPType"`
	}
	var wmsDocument struct {
		XMLName xml.Name
		Version string `xml:"version,attr"`
		Service struct {
			Name           string         `xml:"Name"`
			OnlineResource onlineResource `xml:"OnlineResource"`
			MaxWidth       int            `xml:"MaxWidth"`
			MaxHeight      int            `xml:"MaxHeight"`
		} `xml:"Service"`
		Capability struct {
			Request struct {
				GetCapabilities operation `xml:"GetCapabilities"`
				GetMap          operation `xml:"GetMap"`
			} `xml:"Request"`
			Exception struct {
				Formats []string `xml:"Format"`
			} `xml:"Exception"`
			Layer struct {
				Name  string   `xml:"Name"`
				CRS   []string `xml:"CRS"`
				Style struct {
					Name string `xml:"Name"`
				} `xml:"Style"`
				GeographicBoundingBox struct {
					West  float64 `xml:"westBoundLongitude"`
					East  float64 `xml:"eastBoundLongitude"`
					South float64 `xml:"southBoundLatitude"`
					North float64 `xml:"northBoundLatitude"`
				} `xml:"EX_GeographicBoundingBox"`
				BoundingBoxes []struct {
					CRS  string  `xml:"CRS,attr"`
					MinX float64 `xml:"minx,attr"`
					MinY float64 `xml:"miny,attr"`
					MaxX float64 `xml:"maxx,attr"`
					MaxY float64 `xml:"maxy,attr"`
				} `xml:"BoundingBox"`
			} `xml:"Layer"`
		} `xml:"Capability"`
	}
	if err := xml.Unmarshal(wms.Body.Bytes(), &wmsDocument); err != nil {
		t.Fatalf("WMS metadata is not XML: %v", err)
	}
	if wmsDocument.XMLName.Local != "WMS_Capabilities" || wmsDocument.XMLName.Space != wmsNamespace || wmsDocument.Version != "1.3.0" {
		t.Fatalf("unexpected WMS capabilities root: %+v version=%q", wmsDocument.XMLName, wmsDocument.Version)
	}
	endpoint := "http://maps.example.test/geo-debug-server/wms?"
	if wmsDocument.Service.Name != "WMS" || wmsDocument.Service.OnlineResource.Type != "simple" ||
		wmsDocument.Service.OnlineResource.Href != endpoint ||
		wmsDocument.Service.MaxWidth != 4096 || wmsDocument.Service.MaxHeight != 4096 {
		t.Fatalf("incomplete WMS service metadata: %+v", wmsDocument.Service)
	}
	operations := []operation{wmsDocument.Capability.Request.GetCapabilities, wmsDocument.Capability.Request.GetMap}
	expectedFormats := []string{"application/xml", "image/png"}
	for index, operation := range operations {
		if len(operation.Formats) != 1 || operation.Formats[0] != expectedFormats[index] ||
			operation.DCP.HTTP.Get.OnlineResource.Type != "simple" || operation.DCP.HTTP.Get.OnlineResource.Href != endpoint {
			t.Fatalf("incomplete WMS operation: %+v", operation)
		}
	}
	layer := wmsDocument.Capability.Layer
	if layer.Name != "debug" || strings.Join(layer.CRS, ",") != "EPSG:3857,EPSG:4326,CRS:84" ||
		layer.Style.Name != "default" || len(layer.BoundingBoxes) != 3 {
		t.Fatalf("incomplete WMS layer metadata: %+v", layer)
	}
	geographic := layer.GeographicBoundingBox
	if geographic.West != -180 || geographic.East != 180 || geographic.South != -90 || geographic.North != 90 {
		t.Fatalf("unexpected WMS geographic extent: %+v", geographic)
	}
	if layer.BoundingBoxes[0].CRS != "EPSG:3857" || layer.BoundingBoxes[0].MinX >= 0 || layer.BoundingBoxes[0].MaxX <= 0 {
		t.Fatalf("unexpected WMS projected extent: %+v", layer.BoundingBoxes[0])
	}
	if len(wmsDocument.Capability.Exception.Formats) != 1 || wmsDocument.Capability.Exception.Formats[0] != "XML" {
		t.Fatalf("unexpected WMS exception formats: %+v", wmsDocument.Capability.Exception.Formats)
	}
	if strings.Contains(wms.Body.String(), "<ResourceURL>") {
		t.Fatal("WMS capabilities contains legacy non-standard ResourceURL")
	}
	bare := perform(t, handler, http.MethodGet, "/geo-debug-server/wms")
	if !bytes.Equal(wms.Body.Bytes(), bare.Body.Bytes()) {
		t.Fatal("equivalent WMS capabilities requests returned different metadata")
	}
	explicit := perform(t, handler, http.MethodGet, "/geo-debug-server/wms?REQUEST=GetCapabilities&VERSION=1.3.0")
	if !bytes.Equal(wms.Body.Bytes(), explicit.Body.Bytes()) {
		t.Fatal("explicit WMS 1.3.0 request returned different metadata")
	}
}

func TestWMS111Capabilities(t *testing.T) {
	handler := testHandler(t)
	response := perform(t, handler, http.MethodGet, "/geo-debug-server/wms?SERVICE=WMS&REQUEST=GetCapabilities&VERSION=1.1.1")
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected WMS 1.1.1 metadata: %d %s", response.Code, response.Body.String())
	}
	var document struct {
		XMLName xml.Name
		Version string `xml:"version,attr"`
		Service struct {
			Name string `xml:"Name"`
		} `xml:"Service"`
		Capability struct {
			Request struct {
				GetCapabilities struct {
					Formats []string `xml:"Format"`
				} `xml:"GetCapabilities"`
			} `xml:"Request"`
			Exception struct {
				Formats []string `xml:"Format"`
			} `xml:"Exception"`
			Layer struct {
				Name          string   `xml:"Name"`
				SRS           []string `xml:"SRS"`
				UnexpectedCRS []string `xml:"CRS"`
				LatLonBounds  struct {
					MinX float64 `xml:"minx,attr"`
					MinY float64 `xml:"miny,attr"`
					MaxX float64 `xml:"maxx,attr"`
					MaxY float64 `xml:"maxy,attr"`
				} `xml:"LatLonBoundingBox"`
				BoundingBoxes []struct {
					SRS  string  `xml:"SRS,attr"`
					MinX float64 `xml:"minx,attr"`
					MinY float64 `xml:"miny,attr"`
					MaxX float64 `xml:"maxx,attr"`
					MaxY float64 `xml:"maxy,attr"`
				} `xml:"BoundingBox"`
			} `xml:"Layer"`
		} `xml:"Capability"`
	}
	if err := xml.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("WMS 1.1.1 metadata is not XML: %v", err)
	}
	if document.XMLName.Local != "WMT_MS_Capabilities" || document.XMLName.Space != "" || document.Version != "1.1.1" {
		t.Fatalf("unexpected WMS 1.1.1 root: %+v version=%q", document.XMLName, document.Version)
	}
	if document.Service.Name != "OGC:WMS" || document.Capability.Layer.Name != "debug" ||
		strings.Join(document.Capability.Layer.SRS, ",") != "EPSG:3857,EPSG:4326,CRS:84" || len(document.Capability.Layer.UnexpectedCRS) != 0 {
		t.Fatalf("incomplete WMS 1.1.1 service or layer metadata: %+v", document)
	}
	latLon := document.Capability.Layer.LatLonBounds
	if latLon.MinX != -180 || latLon.MinY != -90 || latLon.MaxX != 180 || latLon.MaxY != 90 {
		t.Fatalf("unexpected WMS 1.1.1 geographic extent: %+v", latLon)
	}
	if len(document.Capability.Layer.BoundingBoxes) != 3 {
		t.Fatalf("expected three WMS 1.1.1 bounding boxes, got %d", len(document.Capability.Layer.BoundingBoxes))
	}
	epsg4326 := document.Capability.Layer.BoundingBoxes[1]
	if epsg4326.SRS != "EPSG:4326" || epsg4326.MinX != -180 || epsg4326.MinY != -90 || epsg4326.MaxX != 180 || epsg4326.MaxY != 90 {
		t.Fatalf("WMS 1.1.1 EPSG:4326 must use longitude/latitude order: %+v", epsg4326)
	}
	if strings.Join(document.Capability.Request.GetCapabilities.Formats, ",") != "application/vnd.ogc.wms_xml" ||
		strings.Join(document.Capability.Exception.Formats, ",") != "application/vnd.ogc.se_xml" {
		t.Fatalf("unexpected WMS 1.1.1 formats: request=%v exception=%v",
			document.Capability.Request.GetCapabilities.Formats, document.Capability.Exception.Formats)
	}
	for _, expected := range []string{`xmlns:xlink="http://www.w3.org/1999/xlink"`, `xlink:href="http://maps.example.test/geo-debug-server/wms?"`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("WMS 1.1.1 metadata does not contain %q", expected)
		}
	}
}

func TestSupportedProtocolVersions(t *testing.T) {
	handler := testHandler(t)
	for name, target := range map[string]string{
		"wmts capabilities version": "/geo-debug-server/wmts?SERVICE=WMTS&REQUEST=GetCapabilities&VERSION=1.0.0",
		"wmts accept versions":      "/geo-debug-server/wmts?SERVICE=WMTS&REQUEST=GetCapabilities&ACCEPTVERSIONS=1.0.0",
		"wmts tile version":         "/geo-debug-server/wmts?SERVICE=WMTS&REQUEST=GetTile&VERSION=1.0.0&TILEMATRIX=0&TILEROW=0&TILECOL=0",
		"wms 130 map":               "/geo-debug-server/wms?REQUEST=GetMap&VERSION=1.3.0",
		"wms 111 map":               "/geo-debug-server/wms?REQUEST=GetMap&VERSION=1.1.1",
	} {
		t.Run(name, func(t *testing.T) {
			response := perform(t, handler, http.MethodGet, target)
			if response.Code != http.StatusOK {
				t.Fatalf("expected supported version, got %d: %s", response.Code, response.Body.String())
			}
		})
	}

	for name, target := range map[string]string{
		"wmts capabilities": "/geo-debug-server/wmts?REQUEST=GetCapabilities&VERSION=1.1.0",
		"wmts tile":         "/geo-debug-server/wmts?REQUEST=GetTile&VERSION=2.0.0&TILEMATRIX=0&TILEROW=0&TILECOL=0",
		"wms capabilities":  "/geo-debug-server/wms?REQUEST=GetCapabilities&VERSION=1.1.0",
		"wms map":           "/geo-debug-server/wms?REQUEST=GetMap&VERSION=1.0.0",
	} {
		t.Run("unsupported "+name, func(t *testing.T) {
			response := perform(t, handler, http.MethodGet, target)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "supported") {
				t.Fatalf("unexpected unsupported-version response: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDefaultWMSBBOXAxisOrder(t *testing.T) {
	if actual := defaultBBOX("EPSG:4326", "1.3.0"); actual != "-90,-180,90,180" {
		t.Fatalf("unexpected WMS 1.3.0 EPSG:4326 BBOX: %s", actual)
	}
	if actual := defaultBBOX("EPSG:4326", "1.1.1"); actual != "-180,-90,180,90" {
		t.Fatalf("unexpected WMS 1.1.1 EPSG:4326 BBOX: %s", actual)
	}
	if actual := defaultBBOX("CRS:84", "1.3.0"); actual != "-180,-90,180,90" {
		t.Fatalf("unexpected WMS 1.3.0 CRS:84 BBOX: %s", actual)
	}
}

func TestHeadAndOptions(t *testing.T) {
	handler := testHandler(t)
	head := perform(t, handler, http.MethodHead, "/geo-debug-server/xyz/0/0/0.png")
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" {
		t.Fatalf("unexpected HEAD response: status=%d body=%d length=%q", head.Code, head.Body.Len(), head.Header().Get("Content-Length"))
	}
	options := perform(t, handler, http.MethodOptions, "/geo-debug-server/wmts")
	if options.Code != http.StatusNoContent {
		t.Fatalf("unexpected OPTIONS status: %d", options.Code)
	}
	if options.Header().Get("Access-Control-Allow-Origin") != "*" ||
		options.Header().Get("Access-Control-Allow-Headers") != "*" ||
		!strings.Contains(options.Header().Get("Access-Control-Allow-Methods"), "DELETE") ||
		options.Header().Get("Access-Control-Max-Age") != "86400" {
		t.Fatalf("unexpected permissive CORS headers: %v", options.Header())
	}
}

func TestImageCacheControl(t *testing.T) {
	handler := testHandler(t)
	target := "/geo-debug-server/xyz/0/0/0.png"

	defaultResponse := perform(t, handler, http.MethodGet, target)
	if actual := defaultResponse.Header().Get("Cache-Control"); actual != immutableImageCache {
		t.Fatalf("unexpected default image cache control: %q", actual)
	}

	for name, headers := range map[string]map[string]string{
		"no-cache":  {"Cache-Control": "no-cache"},
		"no-store":  {"Cache-Control": "no-store"},
		"max-age=0": {"Cache-Control": "public, max-age=0"},
		"pragma":    {"Pragma": "no-cache"},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, target, nil)
			for key, value := range headers {
				request.Header.Set(key, value)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if actual := response.Header().Get("Cache-Control"); actual != noStoreCache {
				t.Fatalf("unexpected bypass cache control: %q", actual)
			}
		})
	}

	metadata := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts")
	if actual := metadata.Header().Get("Cache-Control"); actual != noStoreCache {
		t.Fatalf("metadata must not be cached: %q", actual)
	}
}

func assertPNG(t *testing.T, response *httptest.ResponseRecorder, width, height int) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected content type: %s", response.Header().Get("Content-Type"))
	}
	image, err := png.Decode(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("invalid PNG: %v", err)
	}
	if image.Bounds().Dx() != width || image.Bounds().Dy() != height {
		t.Fatalf("unexpected dimensions: %v", image.Bounds())
	}
}
