package server

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image/color"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/iszy/geo-debug-server/internal/config"
	debugrender "github.com/iszy/geo-debug-server/internal/render"
	"github.com/iszy/geo-debug-server/internal/store"
)

const maxWMSPixels = 4096 * 4096

const (
	immutableImageCache = "public, max-age=31536000, immutable"
	noStoreCache        = "no-store"
)

type Server struct {
	store     *store.Store
	basePath  string
	publicURL string
}

func New(s *store.Store, cfg config.Config) http.Handler {
	return &Server{store: s, basePath: config.NormalizeBasePath(cfg.BasePath), publicURL: strings.TrimRight(cfg.PublicURL, "/")}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	relative, ok := s.relativePath(r.URL.Path)
	if !ok {
		w.Header().Set("Cache-Control", noStoreCache)
		http.Redirect(w, r, s.indexPath(), http.StatusFound)
		return
	}
	if relative == "/schemes" || strings.HasPrefix(relative, "/schemes/") {
		s.handleSchemes(w, r, relative)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		s.writeError(w, r, http.StatusMethodNotAllowed, "OperationNotSupported", "only GET, HEAD and OPTIONS are supported")
		return
	}
	switch {
	case relative == "/":
		s.writeIndex(w, r)
	case relative == "/xyz" || strings.HasPrefix(relative, "/xyz/"):
		s.handleXYZ(w, r, relative)
	case relative == "/wmts" || strings.HasPrefix(relative, "/wmts/"):
		s.handleWMTS(w, r, relative)
	case relative == "/wms":
		s.handleWMS(w, r)
	case relative == "/ags_tile" || strings.HasPrefix(relative, "/ags_tile/"):
		s.handleAGSTile(w, r, relative)
	case relative == "/ags_rest" || strings.HasPrefix(relative, "/ags_rest/"):
		s.handleAGSRest(w, r, relative)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) indexPath() string {
	if s.basePath == "" {
		return "/"
	}
	return s.basePath + "/"
}

func (s *Server) relativePath(path string) (string, bool) {
	if s.basePath == "" {
		return path, true
	}
	if path == s.basePath {
		return "", false
	}
	if !strings.HasPrefix(path, s.basePath+"/") {
		return "", false
	}
	return strings.TrimPrefix(path, s.basePath), true
}

func (s *Server) handleXYZ(w http.ResponseWriter, r *http.Request, relative string) {
	parts := splitPath(relative)
	var schemeID, zoomText, columnText, rowText string
	switch len(parts) {
	case 4:
		zoomText, columnText, rowText = parts[1], parts[2], trimPNG(parts[3])
	case 5:
		schemeID, zoomText, columnText, rowText = parts[1], parts[2], parts[3], trimPNG(parts[4])
	default:
		s.writeError(w, r, http.StatusBadRequest, "InvalidPath", "expected /xyz/{z}/{x}/{y}.png or /xyz/{scheme}/{z}/{x}/{y}.png")
		return
	}
	if rowText == "" {
		s.writeError(w, r, http.StatusBadRequest, "InvalidFormat", "XYZ tile path must end in .png")
		return
	}
	scheme, err := s.resolveScheme(r.Context(), schemeID)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	params := normalizedQuery(r.URL.Query())
	s.serveTile(w, r, tileRequest{
		Scheme: scheme,
		Zoom:   zoomText, Column: columnText, Row: rowText,
		Time: first(params, "TIME", ""), Params: params,
	})
}

func (s *Server) handleWMTS(w http.ResponseWriter, r *http.Request, relative string) {
	if strings.EqualFold(relative, "/wmts/1.0.0/WMTSCapabilities.xml") {
		s.writeWMTSMetadata(w, r)
		return
	}
	parts := splitPath(relative)
	if len(parts) > 1 {
		s.handleWMTSREST(w, r, parts)
		return
	}
	params := normalizedQuery(r.URL.Query())
	request := strings.ToUpper(first(params, "REQUEST", ""))
	version := first(params, "VERSION", "")
	if request == "GETCAPABILITIES" || r.URL.RawQuery == "" {
		version = first(params, "ACCEPTVERSIONS", first(params, "VERSION", "1.0.0"))
	} else if version == "" {
		version = "1.0.0"
	}
	if version != "1.0.0" {
		s.writeError(w, r, http.StatusBadRequest, "InvalidParameterValue", "supported WMTS version is 1.0.0")
		return
	}
	if r.URL.RawQuery == "" || request == "GETCAPABILITIES" {
		s.writeWMTSMetadata(w, r)
		return
	}
	if request == "" {
		request = "GETTILE"
	}
	if request != "GETTILE" {
		s.writeError(w, r, http.StatusBadRequest, "OperationNotSupported", "supported WMTS requests are GetTile and GetCapabilities")
		return
	}

	schemeID := first(params, "TILEMATRIXSET", "")
	scheme, err := s.resolveScheme(r.Context(), schemeID)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	zoom, zoomOK := required(params, "TILEMATRIX")
	row, rowOK := required(params, "TILEROW")
	column, columnOK := required(params, "TILECOL")
	if !zoomOK || !rowOK || !columnOK {
		s.writeError(w, r, http.StatusBadRequest, "MissingParameterValue", "TILEMATRIX, TILEROW and TILECOL are required")
		return
	}
	s.serveTile(w, r, tileRequest{
		Scheme: scheme,
		Zoom:   zoom, Row: row, Column: column,
		Time: first(params, "TIME", ""), Params: params,
	})
}

func (s *Server) handleWMTSREST(w http.ResponseWriter, r *http.Request, parts []string) {
	var schemeID, zoom, row, columnPath string
	switch len(parts) {
	case 4:
		zoom, row, columnPath = parts[1], parts[2], parts[3]
	case 5:
		schemeID, zoom, row, columnPath = parts[1], parts[2], parts[3], parts[4]
	case 7:
		schemeID = parts[3]
		zoom, row, columnPath = parts[4], parts[5], parts[6]
	default:
		s.writeError(w, r, http.StatusBadRequest, "InvalidPath", "expected /wmts/{z}/{y}/{x}.png, /wmts/{scheme}/{z}/{y}/{x}.png or /wmts/{layer}/{style}/{scheme}/{z}/{y}/{x}.png")
		return
	}
	column := trimPNG(columnPath)
	if column == "" {
		s.writeError(w, r, http.StatusBadRequest, "InvalidFormat", "WMTS tile path must end in .png")
		return
	}
	scheme, err := s.resolveScheme(r.Context(), schemeID)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	params := normalizedQuery(r.URL.Query())
	s.serveTile(w, r, tileRequest{
		Scheme: scheme,
		Zoom:   zoom, Row: row, Column: column,
		Time: first(params, "TIME", ""), Params: params,
	})
}

func (s *Server) handleWMS(w http.ResponseWriter, r *http.Request) {
	params := normalizedQuery(r.URL.Query())
	request := strings.ToUpper(first(params, "REQUEST", ""))
	version := first(params, "VERSION", "1.3.0")
	if version != "1.3.0" && version != "1.1.1" {
		s.writeError(w, r, http.StatusBadRequest, "InvalidParameterValue", "supported WMS versions are 1.3.0 and 1.1.1")
		return
	}
	if r.URL.RawQuery == "" || request == "GETCAPABILITIES" {
		s.writeWMSMetadata(w, r, version)
		return
	}
	if request == "" {
		request = "GETMAP"
	}
	if request != "GETMAP" {
		s.writeError(w, r, http.StatusBadRequest, "OperationNotSupported", "supported WMS requests are GetMap and GetCapabilities")
		return
	}

	width, err := boundedInt(first(params, "WIDTH", "256"), 8, 4096)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "InvalidParameterValue", "WIDTH must be between 8 and 4096")
		return
	}
	height, err := boundedInt(first(params, "HEIGHT", "256"), 8, 4096)
	if err != nil || width*height > maxWMSPixels {
		s.writeError(w, r, http.StatusBadRequest, "InvalidParameterValue", "HEIGHT must be between 8 and 4096 and total pixels must not exceed 16777216")
		return
	}
	crs := first(params, "CRS", first(params, "SRS", "EPSG:3857"))
	bbox := first(params, "BBOX", defaultBBOX(crs, version))
	layers := first(params, "LAYERS", "debug")
	format := first(params, "FORMAT", "image/png")
	timeValue := first(params, "TIME", "")
	lines := []string{
		"service: WMS " + version,
		"layers: " + layers,
		"crs: " + crs,
		"bbox: " + bbox,
		fmt.Sprintf("size: %dx%d", width, height),
		"format: " + format,
	}
	if timeValue != "" {
		lines = append(lines, "time: "+timeValue)
	}
	lines = append(lines, extras(params, wmsCoreParameters)...)
	background, textColor := imageColors(params)
	s.writePNG(w, r, debugrender.Spec{Width: width, Height: height, Lines: lines, Background: background, TextColor: textColor})
}

type tileRequest struct {
	Scheme            store.Scheme
	Zoom, Column, Row string
	Time              string
	Params            map[string][]string
}

func (s *Server) serveTile(w http.ResponseWriter, r *http.Request, request tileRequest) {
	level, ok := request.Scheme.Level(request.Zoom)
	if !ok {
		s.writeError(w, r, http.StatusBadRequest, "TileOutOfRange", "unknown tile matrix level "+request.Zoom)
		return
	}
	column, err := strconv.ParseInt(request.Column, 10, 64)
	if err != nil || column < 0 || column >= level.MatrixWidth {
		s.writeError(w, r, http.StatusBadRequest, "TileOutOfRange", fmt.Sprintf("tile column must be between 0 and %d", level.MatrixWidth-1))
		return
	}
	row, err := strconv.ParseInt(request.Row, 10, 64)
	if err != nil || row < 0 || row >= level.MatrixHeight {
		s.writeError(w, r, http.StatusBadRequest, "TileOutOfRange", fmt.Sprintf("tile row must be between 0 and %d", level.MatrixHeight-1))
		return
	}
	lines := tileLines(level.Identifier, column, row, request.Time)
	background, textColor := imageColors(request.Params)
	s.writePNG(w, r, debugrender.Spec{
		Width: request.Scheme.TileWidth, Height: request.Scheme.TileHeight, Lines: lines,
		Background: background, TextColor: textColor,
	})
}

func tileLines(zoom string, column, row int64, timeValue string) []string {
	lines := []string{
		"z: " + zoom,
		fmt.Sprintf("x: %d", column),
		fmt.Sprintf("y: %d", row),
	}
	if timeValue != "" {
		lines = append(lines, "time: "+timeValue)
	}
	return lines
}

func imageColors(params map[string][]string) (color.Color, color.Color) {
	var background color.Color
	if strings.EqualFold(first(params, "TRANSPARENT", "true"), "false") {
		background = parseHexColor(first(params, "BGCOLOR", ""), color.RGBA{A: 128})
	}
	var textColor color.Color
	if value := first(params, "COLOR", ""); value != "" {
		parsed := parseHexColor(value, color.RGBA{R: 255, G: 255, B: 0, A: 255})
		textColor = parsed
	}
	return background, textColor
}

func parseHexColor(value string, fallback color.RGBA) color.RGBA {
	if len(value) != 3 && len(value) != 4 && len(value) != 6 && len(value) != 8 {
		return fallback
	}
	number, err := strconv.ParseUint(value, 16, len(value)*4)
	if err != nil {
		return fallback
	}
	switch len(value) {
	case 3:
		return color.RGBA{
			R: uint8(number>>8) * 17,
			G: uint8(number>>4&0xf) * 17,
			B: uint8(number&0xf) * 17,
			A: 255,
		}
	case 4:
		return color.RGBA{
			R: uint8(number>>12) * 17,
			G: uint8(number>>8&0xf) * 17,
			B: uint8(number>>4&0xf) * 17,
			A: uint8(number&0xf) * 17,
		}
	case 6:
		return color.RGBA{R: uint8(number >> 16), G: uint8(number >> 8), B: uint8(number), A: 255}
	default:
		return color.RGBA{R: uint8(number >> 24), G: uint8(number >> 16), B: uint8(number >> 8), A: uint8(number)}
	}
}

func (s *Server) resolveScheme(ctx context.Context, id string) (store.Scheme, error) {
	if id == "" {
		return s.store.DefaultScheme(ctx)
	}
	return s.store.Scheme(ctx, id)
}

func (s *Server) writePNG(w http.ResponseWriter, r *http.Request, spec debugrender.Spec) {
	data, err := debugrender.PNG(spec)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "NoApplicableCode", err.Error())
		return
	}
	w.Header().Set("Cache-Control", imageCacheControl(r))
	writeResponse(w, r, http.StatusOK, "image/png", data)
}

func (s *Server) writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrSchemeNotFound) {
		s.writeError(w, r, http.StatusNotFound, "InvalidParameterValue", "unknown tile matrix set")
		return
	}
	s.writeError(w, r, http.StatusInternalServerError, "NoApplicableCode", err.Error())
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	type exception struct {
		XMLName xml.Name `xml:"ExceptionReport"`
		Code    string   `xml:"Exception>Code"`
		Text    string   `xml:"Exception>Text"`
	}
	data, _ := xml.MarshalIndent(exception{Code: code, Text: message}, "", "  ")
	data = append([]byte(xml.Header), data...)
	writeResponse(w, r, status, "application/xml; charset=utf-8", data)
}

func writeResponse(w http.ResponseWriter, r *http.Request, status int, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", noStoreCache)
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "*")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func imageCacheControl(r *http.Request) string {
	for _, directive := range strings.Split(strings.ToLower(r.Header.Get("Cache-Control")), ",") {
		directive = strings.ReplaceAll(strings.TrimSpace(directive), " ", "")
		if directive == "no-cache" || directive == "no-store" || directive == "max-age=0" {
			return noStoreCache
		}
	}
	for _, directive := range strings.Split(strings.ToLower(r.Header.Get("Pragma")), ",") {
		if strings.TrimSpace(directive) == "no-cache" {
			return noStoreCache
		}
	}
	return immutableImageCache
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func trimPNG(value string) string {
	if len(value) <= 4 || !strings.EqualFold(value[len(value)-4:], ".png") {
		return ""
	}
	return value[:len(value)-4]
}

func normalizedQuery(values url.Values) map[string][]string {
	result := make(map[string][]string, len(values))
	for key, value := range values {
		upper := strings.ToUpper(key)
		result[upper] = append(result[upper], value...)
	}
	return result
}

func first(params map[string][]string, key, fallback string) string {
	values := params[key]
	if len(values) == 0 || values[0] == "" {
		return fallback
	}
	return values[0]
}

func required(params map[string][]string, key string) (string, bool) {
	value := first(params, key, "")
	return value, value != ""
}

func extras(params map[string][]string, excluded map[string]bool) []string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if !excluded[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, strings.ToLower(key)+": "+strings.Join(params[key], ", "))
	}
	return result
}

func boundedInt(value string, minimum, maximum int) (int, error) {
	number, err := strconv.Atoi(value)
	if err != nil || number < minimum || number > maximum {
		return 0, fmt.Errorf("value must be between %d and %d", minimum, maximum)
	}
	return number, nil
}

func defaultBBOX(crs, version string) string {
	if strings.EqualFold(crs, "EPSG:3857") {
		return "-20037508.342789244,-20037508.342789244,20037508.342789244,20037508.342789244"
	}
	if version == "1.3.0" && strings.EqualFold(crs, "EPSG:4326") {
		return "-90,-180,90,180"
	}
	return "-180,-90,180,90"
}

var wmsCoreParameters = map[string]bool{
	"SERVICE": true, "REQUEST": true, "VERSION": true, "LAYERS": true,
	"STYLES": true, "FORMAT": true, "TRANSPARENT": true, "CRS": true,
	"SRS": true, "BBOX": true, "WIDTH": true, "HEIGHT": true, "TIME": true,
	"BGCOLOR": true, "COLOR": true,
}
