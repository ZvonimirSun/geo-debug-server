package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iszy/geo-debug-server/internal/store"
)

const arcGISTileDPI = 96

type arcGISSpatialReference struct {
	WKID       int    `json:"wkid,omitempty"`
	LatestWKID int    `json:"latestWkid,omitempty"`
	WKT        string `json:"wkt,omitempty"`
}

type arcGISPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type arcGISExtent struct {
	XMin             float64                `json:"xmin"`
	YMin             float64                `json:"ymin"`
	XMax             float64                `json:"xmax"`
	YMax             float64                `json:"ymax"`
	SpatialReference arcGISSpatialReference `json:"spatialReference"`
}

type arcGISLOD struct {
	Level      int     `json:"level"`
	Resolution float64 `json:"resolution"`
	Scale      float64 `json:"scale"`
}

type arcGISTileInfo struct {
	Rows               int                    `json:"rows"`
	Cols               int                    `json:"cols"`
	DPI                int                    `json:"dpi"`
	Format             string                 `json:"format"`
	CompressionQuality int                    `json:"compressionQuality"`
	Origin             arcGISPoint            `json:"origin"`
	SpatialReference   arcGISSpatialReference `json:"spatialReference"`
	LODs               []arcGISLOD            `json:"lods"`
}

type arcGISLayer struct {
	ID                int     `json:"id"`
	Name              string  `json:"name"`
	ParentLayerID     int     `json:"parentLayerId"`
	DefaultVisibility bool    `json:"defaultVisibility"`
	SubLayerIDs       []int   `json:"subLayerIds"`
	MinScale          float64 `json:"minScale"`
	MaxScale          float64 `json:"maxScale"`
}

type arcGISTileMetadata struct {
	CurrentVersion            float64                `json:"currentVersion"`
	ServiceDescription        string                 `json:"serviceDescription"`
	MapName                   string                 `json:"mapName"`
	Description               string                 `json:"description"`
	CopyrightText             string                 `json:"copyrightText"`
	SupportsDynamicLayers     bool                   `json:"supportsDynamicLayers"`
	Layers                    []arcGISLayer          `json:"layers"`
	Tables                    []arcGISLayer          `json:"tables"`
	SpatialReference          arcGISSpatialReference `json:"spatialReference"`
	SingleFusedMapCache       bool                   `json:"singleFusedMapCache"`
	TileInfo                  arcGISTileInfo         `json:"tileInfo"`
	InitialExtent             arcGISExtent           `json:"initialExtent"`
	FullExtent                arcGISExtent           `json:"fullExtent"`
	MinScale                  float64                `json:"minScale"`
	MaxScale                  float64                `json:"maxScale"`
	Units                     string                 `json:"units"`
	SupportedImageFormatTypes string                 `json:"supportedImageFormatTypes"`
	Capabilities              string                 `json:"capabilities"`
	SupportedQueryFormats     string                 `json:"supportedQueryFormats"`
	ExportTilesAllowed        bool                   `json:"exportTilesAllowed"`
	MaxRecordCount            int                    `json:"maxRecordCount"`
	MaxImageHeight            int                    `json:"maxImageHeight"`
	MaxImageWidth             int                    `json:"maxImageWidth"`
	SupportedExtensions       string                 `json:"supportedExtensions"`
}

type arcGISErrorResponse struct {
	Error arcGISError `json:"error"`
}

type arcGISError struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details"`
}

func (s *Server) handleAGSTile(w http.ResponseWriter, r *http.Request, relative string) {
	params := normalizedQuery(r.URL.Query())
	parts := splitPath(relative)
	if len(parts) == 1 || len(parts) == 2 {
		schemeID := ""
		if len(parts) == 2 {
			schemeID = parts[1]
		}
		format := strings.ToLower(first(params, "F", ""))
		if format != "json" && format != "pjson" {
			s.writeAGSError(w, r, http.StatusBadRequest, "f must be json or pjson", false)
			return
		}
		scheme, err := s.resolveScheme(r.Context(), schemeID)
		if err != nil {
			s.writeAGSStoreError(w, r, err, format == "pjson")
			return
		}
		s.writeAGSJSON(w, r, http.StatusOK, newArcGISTileMetadata(scheme), format == "pjson")
		return
	}

	var schemeID, zoom, row, column string
	switch {
	case len(parts) == 5 && strings.EqualFold(parts[1], "tile"):
		zoom, row, column = parts[2], parts[3], parts[4]
	case len(parts) == 6 && strings.EqualFold(parts[2], "tile"):
		schemeID, zoom, row, column = parts[1], parts[3], parts[4], parts[5]
	default:
		s.writeAGSError(w, r, http.StatusNotFound, "expected /ags_tile/tile/{z}/{y}/{x} or /ags_tile/{scheme}/tile/{z}/{y}/{x}", false)
		return
	}
	scheme, err := s.resolveScheme(r.Context(), schemeID)
	if err != nil {
		s.writeAGSStoreError(w, r, err, false)
		return
	}
	s.serveTile(w, r, tileRequest{
		Scheme: scheme,
		Zoom:   zoom, Row: row, Column: column,
		Time: first(params, "TIME", ""), Params: params,
	})
}

func newArcGISTileMetadata(scheme store.Scheme) arcGISTileMetadata {
	spatialReference := newArcGISSpatialReference(scheme.CRS)
	extent := arcGISExtent{
		XMin: scheme.MinX, YMin: scheme.MinY, XMax: scheme.MaxX, YMax: scheme.MaxY,
		SpatialReference: spatialReference,
	}
	lods := make([]arcGISLOD, 0, len(scheme.Levels))
	for _, level := range scheme.Levels {
		lods = append(lods, arcGISLOD{
			Level: level.Zoom, Resolution: level.Resolution,
			Scale: scaleDenominator(level.Resolution, scheme.MetersPerUnit, arcGISTileDPI),
		})
	}
	description := "Debug tile service using " + scheme.ID
	return arcGISTileMetadata{
		CurrentVersion: 11.4, ServiceDescription: description, MapName: scheme.Name,
		Description: description, Layers: []arcGISLayer{{
			ID: 0, Name: "debug", ParentLayerID: -1, DefaultVisibility: true,
		}}, Tables: []arcGISLayer{}, SpatialReference: spatialReference,
		SingleFusedMapCache: true,
		TileInfo: arcGISTileInfo{
			Rows: scheme.TileHeight, Cols: scheme.TileWidth, DPI: arcGISTileDPI,
			Format: "PNG32", Origin: arcGISPoint{X: scheme.OriginX, Y: scheme.OriginY},
			SpatialReference: spatialReference, LODs: lods,
		},
		InitialExtent: extent, FullExtent: extent, Units: arcGISUnits(scheme),
		SupportedImageFormatTypes: "PNG32", Capabilities: "Map,TilesOnly",
		SupportedQueryFormats: "JSON", MaxImageHeight: 4096, MaxImageWidth: 4096,
	}
}

func newArcGISSpatialReference(crs string) arcGISSpatialReference {
	normalized := strings.ToUpper(strings.TrimSpace(crs))
	if normalized == "CRS:84" {
		return arcGISSpatialReference{WKID: 4326, LatestWKID: 4326}
	}
	if strings.HasPrefix(normalized, "EPSG:") {
		wkid, err := strconv.Atoi(strings.TrimSpace(normalized[len("EPSG:"):]))
		if err == nil && wkid > 0 {
			if wkid == 3857 {
				return arcGISSpatialReference{WKID: 102100, LatestWKID: 3857}
			}
			return arcGISSpatialReference{WKID: wkid, LatestWKID: wkid}
		}
	}
	return arcGISSpatialReference{WKT: crs}
}

func arcGISUnits(scheme store.Scheme) string {
	if scheme.MetersPerUnit == 1 {
		return "esriMeters"
	}
	return "esriDecimalDegrees"
}

func (s *Server) writeAGSJSON(w http.ResponseWriter, r *http.Request, status int, value any, pretty bool) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(value); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "NoApplicableCode", "encode ArcGIS response: "+err.Error())
		return
	}
	data := output.Bytes()
	if !pretty {
		data = bytes.TrimSuffix(data, []byte{'\n'})
	}
	writeResponse(w, r, status, "application/json; charset=utf-8", data)
}

func (s *Server) writeAGSError(w http.ResponseWriter, r *http.Request, status int, message string, pretty bool) {
	s.writeAGSJSON(w, r, status, arcGISErrorResponse{Error: arcGISError{
		Code: status, Message: message, Details: []string{},
	}}, pretty)
}

func (s *Server) writeAGSStoreError(w http.ResponseWriter, r *http.Request, err error, pretty bool) {
	if errors.Is(err, store.ErrSchemeNotFound) {
		s.writeAGSError(w, r, http.StatusNotFound, "unknown tile scheme", pretty)
		return
	}
	s.writeAGSError(w, r, http.StatusInternalServerError, err.Error(), pretty)
}
