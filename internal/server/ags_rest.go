package server

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	debugrender "github.com/iszy/geo-debug-server/internal/render"
	"github.com/iszy/geo-debug-server/internal/store"
)

type arcGISDynamicMetadata struct {
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
	InitialExtent             arcGISExtent           `json:"initialExtent"`
	FullExtent                arcGISExtent           `json:"fullExtent"`
	MinScale                  float64                `json:"minScale"`
	MaxScale                  float64                `json:"maxScale"`
	Units                     string                 `json:"units"`
	SupportedImageFormatTypes string                 `json:"supportedImageFormatTypes"`
	Capabilities              string                 `json:"capabilities"`
	SupportedQueryFormats     string                 `json:"supportedQueryFormats"`
	MaxRecordCount            int                    `json:"maxRecordCount"`
	MaxImageHeight            int                    `json:"maxImageHeight"`
	MaxImageWidth             int                    `json:"maxImageWidth"`
	SupportedExtensions       string                 `json:"supportedExtensions"`
}

type arcGISExportResult struct {
	Href   string       `json:"href"`
	Width  int          `json:"width"`
	Height int          `json:"height"`
	Extent arcGISExtent `json:"extent"`
	Scale  float64      `json:"scale"`
}

type arcGISExportRequest struct {
	BBox                 [4]float64
	Width, Height        int
	BBoxSR, ImageSR      string
	DPI                  float64
	Format, Time, Output string
	Params               map[string][]string
}

func (s *Server) handleAGSRest(w http.ResponseWriter, r *http.Request, relative string) {
	parts := splitPath(relative)
	schemeID := ""
	isExport := false
	switch {
	case len(parts) == 1:
	case len(parts) == 2 && strings.EqualFold(parts[1], "export"):
		isExport = true
	case len(parts) == 2:
		schemeID = parts[1]
	case len(parts) == 3 && strings.EqualFold(parts[2], "export"):
		schemeID, isExport = parts[1], true
	default:
		s.writeAGSError(w, r, http.StatusNotFound, "expected /ags_rest, /ags_rest/{scheme}, or an /export subpath", false)
		return
	}
	scheme, err := s.resolveScheme(r.Context(), schemeID)
	if err != nil {
		s.writeAGSStoreError(w, r, err, false)
		return
	}
	if isExport {
		s.handleAGSRestExport(w, r, relative, scheme)
		return
	}
	params := normalizedQuery(r.URL.Query())
	format := strings.ToLower(first(params, "F", ""))
	if format != "json" && format != "pjson" {
		s.writeAGSError(w, r, http.StatusBadRequest, "f must be json or pjson", false)
		return
	}
	s.writeAGSJSON(w, r, http.StatusOK, newArcGISDynamicMetadata(scheme), format == "pjson")
}

func newArcGISDynamicMetadata(scheme store.Scheme) arcGISDynamicMetadata {
	spatialReference := newArcGISSpatialReference(scheme.CRS)
	extent := arcGISExtent{
		XMin: scheme.MinX, YMin: scheme.MinY, XMax: scheme.MaxX, YMax: scheme.MaxY,
		SpatialReference: spatialReference,
	}
	description := "Debug dynamic map service using " + scheme.ID
	return arcGISDynamicMetadata{
		CurrentVersion: 11.4, ServiceDescription: description, MapName: scheme.Name,
		Description: description, SupportsDynamicLayers: true,
		Layers: []arcGISLayer{{ID: 0, Name: "debug", ParentLayerID: -1, DefaultVisibility: true}},
		Tables: []arcGISLayer{}, SpatialReference: spatialReference,
		InitialExtent: extent, FullExtent: extent, Units: arcGISUnits(scheme),
		SupportedImageFormatTypes: "PNG32,PNG24,PNG8,PNG", Capabilities: "Map",
		SupportedQueryFormats: "JSON", MaxImageHeight: 4096, MaxImageWidth: 4096,
	}
}

func (s *Server) handleAGSRestExport(w http.ResponseWriter, r *http.Request, relative string, scheme store.Scheme) {
	request, err := parseArcGISExportRequest(r, scheme)
	if err != nil {
		s.writeAGSError(w, r, http.StatusBadRequest, err.Error(), false)
		return
	}
	switch request.Output {
	case "image":
		lines := []string{
			"service: ArcGIS Dynamic MapServer",
			"scheme: " + scheme.ID,
			fmt.Sprintf("bbox: %g,%g,%g,%g", request.BBox[0], request.BBox[1], request.BBox[2], request.BBox[3]),
			fmt.Sprintf("size: %dx%d", request.Width, request.Height),
			"bboxSR: " + request.BBoxSR,
			"imageSR: " + request.ImageSR,
			"dpi: " + strconv.FormatFloat(request.DPI, 'g', -1, 64),
			"format: " + request.Format,
		}
		if request.Time != "" {
			lines = append(lines, "time: "+request.Time)
		}
		lines = append(lines, extras(request.Params, arcGISExportCoreParameters)...)
		background, textColor := imageColors(request.Params)
		s.writePNG(w, r, debugrender.Spec{
			Width: request.Width, Height: request.Height, Lines: lines,
			Background: background, TextColor: textColor,
		})
	case "json", "pjson":
		spatialReference := arcGISSpatialReferenceParameter(request.ImageSR, scheme)
		extent := arcGISExtent{
			XMin: request.BBox[0], YMin: request.BBox[1], XMax: request.BBox[2], YMax: request.BBox[3],
			SpatialReference: spatialReference,
		}
		query := cloneURLValues(r.URL.Query())
		setURLValueCaseInsensitive(query, "f", "image")
		resolution := (request.BBox[2] - request.BBox[0]) / float64(request.Width)
		result := arcGISExportResult{
			Href:  s.publicRoot(r) + relative + "?" + query.Encode(),
			Width: request.Width, Height: request.Height, Extent: extent,
			Scale: scaleDenominator(resolution, scheme.MetersPerUnit, request.DPI),
		}
		s.writeAGSJSON(w, r, http.StatusOK, result, request.Output == "pjson")
	}
}

func parseArcGISExportRequest(r *http.Request, scheme store.Scheme) (arcGISExportRequest, error) {
	params := normalizedQuery(r.URL.Query())
	bbox, err := parseArcGISBBox(first(params, "BBOX", fmt.Sprintf("%g,%g,%g,%g", scheme.MinX, scheme.MinY, scheme.MaxX, scheme.MaxY)))
	if err != nil {
		return arcGISExportRequest{}, err
	}
	width, height, err := parseArcGISSize(first(params, "SIZE", "256,256"))
	if err != nil {
		return arcGISExportRequest{}, err
	}
	dpi, err := strconv.ParseFloat(first(params, "DPI", "96"), 64)
	if err != nil || dpi <= 0 || math.IsNaN(dpi) || math.IsInf(dpi, 0) {
		return arcGISExportRequest{}, fmt.Errorf("dpi must be a finite positive number")
	}
	format := strings.ToLower(first(params, "FORMAT", "png32"))
	if format != "png" && format != "png8" && format != "png24" && format != "png32" {
		return arcGISExportRequest{}, fmt.Errorf("format must be png, png8, png24, or png32")
	}
	output := strings.ToLower(first(params, "F", "image"))
	if output != "image" && output != "json" && output != "pjson" {
		return arcGISExportRequest{}, fmt.Errorf("f must be image, json, or pjson")
	}
	spatialReference := arcGISSpatialReferenceLabel(scheme)
	return arcGISExportRequest{
		BBox: bbox, Width: width, Height: height,
		BBoxSR:  first(params, "BBOXSR", spatialReference),
		ImageSR: first(params, "IMAGESR", first(params, "BBOXSR", spatialReference)),
		DPI:     dpi, Format: format, Time: first(params, "TIME", ""), Output: output, Params: params,
	}, nil
}

func parseArcGISBBox(value string) ([4]float64, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return [4]float64{}, fmt.Errorf("bbox must contain xmin,ymin,xmax,ymax")
	}
	var bbox [4]float64
	for index, part := range parts {
		number, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return [4]float64{}, fmt.Errorf("bbox coordinates must be finite numbers")
		}
		bbox[index] = number
	}
	if bbox[0] >= bbox[2] || bbox[1] >= bbox[3] {
		return [4]float64{}, fmt.Errorf("bbox minimums must be less than maximums")
	}
	return bbox, nil
}

func parseArcGISSize(value string) (int, int, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("size must contain width,height")
	}
	width, err := boundedInt(strings.TrimSpace(parts[0]), 8, 4096)
	if err != nil {
		return 0, 0, fmt.Errorf("width must be between 8 and 4096")
	}
	height, err := boundedInt(strings.TrimSpace(parts[1]), 8, 4096)
	if err != nil || width*height > maxWMSPixels {
		return 0, 0, fmt.Errorf("height must be between 8 and 4096 and total pixels must not exceed 16777216")
	}
	return width, height, nil
}

func arcGISSpatialReferenceLabel(scheme store.Scheme) string {
	reference := newArcGISSpatialReference(scheme.CRS)
	if reference.LatestWKID != 0 {
		return strconv.Itoa(reference.LatestWKID)
	}
	if reference.WKID != 0 {
		return strconv.Itoa(reference.WKID)
	}
	return scheme.CRS
}

func arcGISSpatialReferenceParameter(value string, scheme store.Scheme) arcGISSpatialReference {
	value = strings.TrimSpace(value)
	if wkid, err := strconv.Atoi(value); err == nil && wkid > 0 {
		return newArcGISSpatialReference("EPSG:" + value)
	}
	if strings.Contains(value, ":") {
		return newArcGISSpatialReference(value)
	}
	return newArcGISSpatialReference(scheme.CRS)
}

func cloneURLValues(values url.Values) url.Values {
	clone := make(url.Values, len(values))
	for key, items := range values {
		clone[key] = append([]string(nil), items...)
	}
	return clone
}

func setURLValueCaseInsensitive(values url.Values, target, value string) {
	for key := range values {
		if strings.EqualFold(key, target) {
			delete(values, key)
		}
	}
	values.Set(target, value)
}

var arcGISExportCoreParameters = map[string]bool{
	"F": true, "BBOX": true, "SIZE": true, "BBOXSR": true, "IMAGESR": true,
	"DPI": true, "FORMAT": true, "TRANSPARENT": true, "BGCOLOR": true,
	"COLOR": true, "TIME": true,
}
