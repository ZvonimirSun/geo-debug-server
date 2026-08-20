package server

import (
	"bytes"
	"context"
	"encoding/xml"
	"image/png"
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

func TestXYZForLeaflet(t *testing.T) {
	handler := testHandler(t)
	response := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/2/1/1.png?time=demo")
	assertPNG(t, response, 256, 256)
	if response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing permissive CORS header")
	}

	crs84 := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/WorldCRS84Quad/0/1/0.png")
	assertPNG(t, crs84, 256, 256)
	outOfRange := perform(t, handler, http.MethodGet, "/geo-debug-server/xyz/WorldCRS84Quad/0/2/0.png")
	if outOfRange.Code != http.StatusBadRequest {
		t.Fatalf("expected out-of-range status 400, got %d", outOfRange.Code)
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
		"/geo-debug-server/wmts/WorldCRS84Quad/0/0/1.png",
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

func TestMetadataListsSchemesAndResourceURLs(t *testing.T) {
	handler := testHandler(t)
	wmts := perform(t, handler, http.MethodGet, "/geo-debug-server/wmts")
	if wmts.Code != http.StatusOK {
		t.Fatalf("unexpected WMTS metadata status: %d", wmts.Code)
	}
	var document struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(wmts.Body.Bytes(), &document); err != nil {
		t.Fatalf("WMTS metadata is not XML: %v", err)
	}
	if document.XMLName.Local != "Capabilities" {
		t.Fatalf("unexpected WMTS metadata root: %s", document.XMLName.Local)
	}
	body := wmts.Body.String()
	for _, expected := range []string{"WebMercatorQuad", "WorldCRS84Quad", "ResourceURL", "http://maps.example.test/geo-debug-server/wmts"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("WMTS metadata does not contain %q", expected)
		}
	}

	wms := perform(t, handler, http.MethodGet, "/geo-debug-server/wms?REQUEST=GetCapabilities")
	if wms.Code != http.StatusOK || !strings.Contains(wms.Body.String(), "ResourceURL") {
		t.Fatalf("unexpected WMS metadata: %d %s", wms.Code, wms.Body.String())
	}
	if err := xml.Unmarshal(wms.Body.Bytes(), &document); err != nil {
		t.Fatalf("WMS metadata is not XML: %v", err)
	}
	if document.XMLName.Local != "WMS_Capabilities" {
		t.Fatalf("unexpected WMS metadata root: %s", document.XMLName.Local)
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
