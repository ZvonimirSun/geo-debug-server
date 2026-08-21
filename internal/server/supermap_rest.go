package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	debugrender "github.com/iszy/geo-debug-server/internal/render"
	"github.com/iszy/geo-debug-server/internal/store"
)

type superMapPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type superMapBounds struct {
	Left       float64       `json:"left"`
	Bottom     float64       `json:"bottom"`
	Right      float64       `json:"right"`
	Top        float64       `json:"top"`
	LeftBottom superMapPoint `json:"leftBottom"`
	RightTop   superMapPoint `json:"rightTop"`
}

type superMapPrjCoordSys struct {
	EPSGCode     int    `json:"epsgCode"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	CoordUnit    string `json:"coordUnit"`
	DistanceUnit string `json:"distanceUnit"`
}

type superMapMetadata struct {
	Name           string              `json:"name"`
	Caption        string              `json:"caption"`
	Description    string              `json:"description"`
	ViewBounds     superMapBounds      `json:"viewBounds"`
	Bounds         superMapBounds      `json:"bounds"`
	Center         superMapPoint       `json:"center"`
	CoordUnit      string              `json:"coordUnit"`
	DistanceUnit   string              `json:"distanceUnit"`
	PrjCoordSys    superMapPrjCoordSys `json:"prjCoordSys"`
	Scale          float64             `json:"scale"`
	VisibleScales  []float64           `json:"visibleScales"`
	IsMultiLayers  bool                `json:"isMultiLayers"`
	MaxVisibleText int                 `json:"maxVisibleTextSize"`
	CacheEnabled   bool                `json:"cacheEnabled"`
	DynamicProject bool                `json:"dynamicProjection"`
	Origin         superMapPoint       `json:"origin"`
	TileWidth      int                 `json:"tileWidth"`
	TileHeight     int                 `json:"tileHeight"`
	MinZoom        int                 `json:"minZoom"`
	MaxZoom        int                 `json:"maxZoom"`
	Resolutions    []float64           `json:"resolutions"`
}

type superMapTileset struct {
	MetaData     superMapTilesetMetadata `json:"metaData"`
	Name         string                  `json:"name"`
	TileVersions any                     `json:"tileVersions"`
}

type superMapTilesetMetadata struct {
	ScaleDenominators []float64           `json:"scaleDenominators"`
	OriginalPoint     superMapPoint       `json:"originalPoint"`
	Resolutions       []float64           `json:"resolutions"`
	TileWidth         int                 `json:"tileWidth"`
	MapStatusHashCode string              `json:"mapStatusHashCode"`
	Transparent       bool                `json:"transparent"`
	ScaleCaptionsMap  any                 `json:"scaleCaptionsMap"`
	MapParameter      any                 `json:"mapParameter"`
	TileType          string              `json:"tileType"`
	TileFormat        string              `json:"tileFormat"`
	Bounds            superMapBounds      `json:"bounds"`
	TileRuleVersion   any                 `json:"tileRuleVersion"`
	StorageType       string              `json:"storageType"`
	PrjCoordSys       superMapPrjCoordSys `json:"prjCoordSys"`
	MapName           string              `json:"mapName"`
	TilesetName       string              `json:"tilesetName"`
	TileHeight        int                 `json:"tileHeight"`
}

func (s *Server) handleSuperMap(w http.ResponseWriter, r *http.Request, relative string, tiled bool) {
	parts := splitPath(relative)
	if len(parts) != 7 && len(parts) != 8 {
		s.writeSuperMapError(w, r, http.StatusNotFound, "expected a SuperMap maps/{scheme} resource")
		return
	}
	servicePath := "spm_rest"
	if tiled {
		servicePath = "spm_tile"
	}
	expected := []string{servicePath, "iserver", "services", "map-debug", "rest", "maps"}
	for index, part := range expected {
		if !strings.EqualFold(parts[index], part) {
			s.writeSuperMapError(w, r, http.StatusNotFound, "unexpected SuperMap service path")
			return
		}
	}

	mapName := trimSuffixFold(parts[6], ".json")
	if mapName == "" {
		s.writeSuperMapError(w, r, http.StatusBadRequest, "tile scheme name is required")
		return
	}
	scheme, err := s.resolveScheme(r.Context(), mapName)
	if err != nil {
		s.writeSuperMapStoreError(w, r, err)
		return
	}
	if len(parts) == 7 {
		s.writeAGSJSON(w, r, http.StatusOK, newSuperMapMetadata(scheme, tiled), true)
		return
	}
	if strings.HasSuffix(strings.ToLower(parts[6]), ".json") {
		s.writeSuperMapError(w, r, http.StatusNotFound, "resource subpaths require a scheme name without .json")
		return
	}
	switch {
	case strings.EqualFold(parts[7], "tileImage.png"):
		s.writeSuperMapImage(w, r, scheme)
	case strings.EqualFold(parts[7], "tilesets.json"):
		if tiled {
			s.writeAGSJSON(w, r, http.StatusOK, newSuperMapTilesets(scheme), true)
		} else {
			writeResponse(w, r, http.StatusOK, "application/json; charset=utf-8", []byte("[]"))
		}
	default:
		s.writeSuperMapError(w, r, http.StatusNotFound, "expected a tileImage.png or tilesets.json resource")
	}
}

func newSuperMapMetadata(scheme store.Scheme, tiled bool) superMapMetadata {
	bounds := newSuperMapBounds(scheme.MinX, scheme.MinY, scheme.MaxX, scheme.MaxY)
	resolutions := make([]float64, 0, len(scheme.Levels))
	for _, level := range scheme.Levels {
		resolutions = append(resolutions, level.Resolution)
	}
	return superMapMetadata{
		Name: scheme.ID, Caption: scheme.Name,
		Description: "Debug SuperMap REST map service",
		ViewBounds:  bounds, Bounds: bounds,
		Center:    superMapPoint{X: (scheme.MinX + scheme.MaxX) / 2, Y: (scheme.MinY + scheme.MaxY) / 2},
		CoordUnit: superMapCoordUnit(scheme), DistanceUnit: "METER", PrjCoordSys: newSuperMapPrjCoordSys(scheme),
		VisibleScales: []float64{}, CacheEnabled: tiled, DynamicProject: false,
		Origin:    superMapPoint{X: scheme.OriginX, Y: scheme.OriginY},
		TileWidth: scheme.TileWidth, TileHeight: scheme.TileHeight,
		MinZoom: scheme.MinZoom, MaxZoom: scheme.MaxZoom, Resolutions: resolutions,
	}
}

func newSuperMapTilesets(scheme store.Scheme) []superMapTileset {
	resolutions := make([]float64, 0, len(scheme.Levels))
	scaleDenominators := make([]float64, 0, len(scheme.Levels))
	for _, level := range scheme.Levels {
		resolutions = append(resolutions, level.Resolution)
		scaleDenominators = append(scaleDenominators, scaleDenominator(level.Resolution, scheme.MetersPerUnit, arcGISTileDPI))
	}
	return []superMapTileset{{
		Name: "debug_tileset_" + scheme.ID,
		MetaData: superMapTilesetMetadata{
			ScaleDenominators: scaleDenominators,
			OriginalPoint:     superMapPoint{X: scheme.OriginX, Y: scheme.OriginY},
			Resolutions:       resolutions, TileWidth: scheme.TileWidth, TileHeight: scheme.TileHeight,
			MapStatusHashCode: "DEBUGFIX", Transparent: true,
			TileType: "Image", TileFormat: "PNG", StorageType: "Compact",
			Bounds:      newSuperMapBounds(scheme.MinX, scheme.MinY, scheme.MaxX, scheme.MaxY),
			PrjCoordSys: newSuperMapPrjCoordSys(scheme), MapName: scheme.ID, TilesetName: scheme.ID,
		},
	}}
}

func (s *Server) writeSuperMapImage(w http.ResponseWriter, r *http.Request, scheme store.Scheme) {
	params := normalizedQuery(r.URL.Query())
	width, err := boundedInt(first(params, "WIDTH", strconv.Itoa(scheme.TileWidth)), 8, 4096)
	if err != nil {
		s.writeSuperMapError(w, r, http.StatusBadRequest, "width must be between 8 and 4096")
		return
	}
	height, err := boundedInt(first(params, "HEIGHT", strconv.Itoa(scheme.TileHeight)), 8, 4096)
	if err != nil || width*height > maxWMSPixels {
		s.writeSuperMapError(w, r, http.StatusBadRequest, "height must be between 8 and 4096 and total pixels must not exceed 16777216")
		return
	}

	lines := []string{
		"service: SuperMap REST Map",
		"scheme: " + scheme.ID,
		"crs: " + scheme.CRS,
		fmt.Sprintf("size: %dx%d", width, height),
		fmt.Sprintf("origin: %g,%g", scheme.OriginX, scheme.OriginY),
		fmt.Sprintf("bounds: %g,%g,%g,%g", scheme.MinX, scheme.MinY, scheme.MaxX, scheme.MaxY),
	}
	for _, parameter := range []string{"SCALE", "X", "Y", "LAYERSID", "CACHEENABLED", "REDIRECT"} {
		if value := first(params, parameter, ""); value != "" {
			lines = append(lines, superMapParameterLabel(parameter)+": "+value)
		}
	}
	if timeValue := first(params, "TIME", ""); timeValue != "" {
		lines = append(lines, "time: "+timeValue)
	}
	lines = append(lines, extras(params, superMapCoreParameters)...)
	background, textColor := imageColors(params)
	s.writePNG(w, r, debugrender.Spec{
		Width: width, Height: height, Lines: lines,
		Background: background, TextColor: textColor,
	})
}

func newSuperMapBounds(minX, minY, maxX, maxY float64) superMapBounds {
	return superMapBounds{
		Left: minX, Bottom: minY, Right: maxX, Top: maxY,
		LeftBottom: superMapPoint{X: minX, Y: minY}, RightTop: superMapPoint{X: maxX, Y: maxY},
	}
}

func newSuperMapPrjCoordSys(scheme store.Scheme) superMapPrjCoordSys {
	coordUnit := superMapCoordUnit(scheme)
	reference := superMapPrjCoordSys{
		Name: scheme.CRS, Type: "PCS_USER_DEFINED", CoordUnit: coordUnit, DistanceUnit: "METER",
	}
	normalized := strings.ToUpper(strings.TrimSpace(scheme.CRS))
	if normalized == "CRS:84" {
		reference.EPSGCode = 4326
		reference.Type = "PCS_EARTH_LONGITUDE_LATITUDE"
		return reference
	}
	if strings.HasPrefix(normalized, "EPSG:") {
		reference.EPSGCode, _ = strconv.Atoi(strings.TrimSpace(normalized[len("EPSG:"):]))
	}
	switch reference.EPSGCode {
	case 3857:
		reference.Type = "PCS_WGS_1984_WEB_MERCATOR"
	case 4326, 4490:
		reference.Type = "PCS_EARTH_LONGITUDE_LATITUDE"
	}
	return reference
}

func superMapCoordUnit(scheme store.Scheme) string {
	if scheme.MetersPerUnit == 1 {
		return "METER"
	}
	return "DEGREE"
}

func (s *Server) writeSuperMapStoreError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrSchemeNotFound) {
		s.writeSuperMapError(w, r, http.StatusNotFound, "unknown tile scheme")
		return
	}
	s.writeSuperMapError(w, r, http.StatusInternalServerError, err.Error())
}

func (s *Server) writeSuperMapError(w http.ResponseWriter, r *http.Request, status int, message string) {
	s.writeAGSJSON(w, r, status, map[string]any{
		"succeed": false,
		"error":   map[string]any{"code": status, "errorMsg": message},
	}, true)
}

func trimSuffixFold(value, suffix string) string {
	if len(value) < len(suffix) || !strings.EqualFold(value[len(value)-len(suffix):], suffix) {
		return value
	}
	return value[:len(value)-len(suffix)]
}

func superMapParameterLabel(parameter string) string {
	labels := map[string]string{
		"SCALE": "scale", "X": "x", "Y": "y", "ORIGIN": "origin", "BOUNDS": "bounds",
		"LAYERSID": "layersID", "CACHEENABLED": "cacheEnabled", "REDIRECT": "redirect",
	}
	return labels[parameter]
}

var superMapCoreParameters = map[string]bool{
	"WIDTH": true, "HEIGHT": true, "SCALE": true, "X": true, "Y": true,
	"ORIGIN": true, "BOUNDS": true, "LAYERSID": true, "CACHEENABLED": true,
	"REDIRECT": true, "TRANSPARENT": true, "BGCOLOR": true, "COLOR": true, "TIME": true,
}
