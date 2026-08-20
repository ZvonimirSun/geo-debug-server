package server

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

const (
	wmtsNamespace  = "http://www.opengis.net/wmts/1.0"
	owsNamespace   = "http://www.opengis.net/ows/1.1"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
	wmsNamespace   = "http://www.opengis.net/wms"
)

type wmtsCapabilities struct {
	XMLName               xml.Name                 `xml:"Capabilities"`
	XMLNS                 string                   `xml:"xmlns,attr"`
	XMLNSOWS              string                   `xml:"xmlns:ows,attr"`
	XMLNSXLink            string                   `xml:"xmlns:xlink,attr"`
	Version               string                   `xml:"version,attr"`
	ServiceIdentification owsServiceIdentification `xml:"ows:ServiceIdentification"`
	OperationsMetadata    owsOperationsMetadata    `xml:"ows:OperationsMetadata"`
	Contents              wmtsContents             `xml:"Contents"`
}

type owsServiceIdentification struct {
	Title              string `xml:"ows:Title"`
	ServiceType        string `xml:"ows:ServiceType"`
	ServiceTypeVersion string `xml:"ows:ServiceTypeVersion"`
}

type owsOperationsMetadata struct {
	Operations []owsOperation `xml:"ows:Operation"`
}

type owsOperation struct {
	Name       string         `xml:"name,attr"`
	DCP        owsDCP         `xml:"ows:DCP"`
	Parameters []owsParameter `xml:"ows:Parameter"`
}

type owsDCP struct {
	HTTP owsHTTP `xml:"ows:HTTP"`
}

type owsHTTP struct {
	Get owsGet `xml:"ows:Get"`
}

type owsGet struct {
	Href       string        `xml:"xlink:href,attr"`
	Constraint owsConstraint `xml:"ows:Constraint"`
}

type owsConstraint struct {
	Name          string           `xml:"name,attr"`
	AllowedValues owsAllowedValues `xml:"ows:AllowedValues"`
}

type owsAllowedValues struct {
	Values []string `xml:"ows:Value"`
}

type owsParameter struct {
	Name          string           `xml:"name,attr"`
	AllowedValues owsAllowedValues `xml:"ows:AllowedValues"`
}

type wmtsContents struct {
	Layer          wmtsLayer           `xml:"Layer"`
	TileMatrixSets []wmtsTileMatrixSet `xml:"TileMatrixSet"`
}

type wmtsLayer struct {
	Title          string                  `xml:"ows:Title"`
	Identifier     string                  `xml:"ows:Identifier"`
	BoundingBoxes  []owsBoundingBox        `xml:"ows:BoundingBox"`
	Styles         []wmtsStyle             `xml:"Style"`
	Formats        []string                `xml:"Format"`
	MatrixSetLinks []wmtsTileMatrixSetLink `xml:"TileMatrixSetLink"`
	ResourceURLs   []wmtsResourceURL       `xml:"ResourceURL"`
}

type owsBoundingBox struct {
	CRS         string `xml:"crs,attr"`
	LowerCorner string `xml:"ows:LowerCorner"`
	UpperCorner string `xml:"ows:UpperCorner"`
}

type wmtsStyle struct {
	IsDefault  bool   `xml:"isDefault,attr"`
	Identifier string `xml:"ows:Identifier"`
}

type wmtsTileMatrixSetLink struct {
	TileMatrixSet string `xml:"TileMatrixSet"`
}

type wmtsResourceURL struct {
	Format       string `xml:"format,attr"`
	ResourceType string `xml:"resourceType,attr"`
	Template     string `xml:"template,attr"`
}

type wmtsTileMatrixSet struct {
	Title        string           `xml:"ows:Title"`
	Identifier   string           `xml:"ows:Identifier"`
	SupportedCRS string           `xml:"ows:SupportedCRS"`
	Matrices     []wmtsTileMatrix `xml:"TileMatrix"`
}

type wmtsTileMatrix struct {
	Identifier       string  `xml:"ows:Identifier"`
	ScaleDenominator float64 `xml:"ScaleDenominator"`
	TopLeftCorner    string  `xml:"TopLeftCorner"`
	TileWidth        int     `xml:"TileWidth"`
	TileHeight       int     `xml:"TileHeight"`
	MatrixWidth      int64   `xml:"MatrixWidth"`
	MatrixHeight     int64   `xml:"MatrixHeight"`
}

type wmsCapabilities struct {
	XMLName      xml.Name      `xml:"WMS_Capabilities"`
	XMLNS        string        `xml:"xmlns,attr"`
	XMLNSXLink   string        `xml:"xmlns:xlink,attr"`
	Version      string        `xml:"version,attr"`
	Service      wmsService    `xml:"Service"`
	Capabilities wmsCapability `xml:"Capability"`
}

type wms111Capabilities struct {
	XMLName      xml.Name         `xml:"WMT_MS_Capabilities"`
	XMLNSXLink   string           `xml:"xmlns:xlink,attr"`
	Version      string           `xml:"version,attr"`
	Service      wms111Service    `xml:"Service"`
	Capabilities wms111Capability `xml:"Capability"`
}

type wms111Service struct {
	Name              string            `xml:"Name"`
	Title             string            `xml:"Title"`
	OnlineResource    wmsOnlineResource `xml:"OnlineResource"`
	Fees              string            `xml:"Fees"`
	AccessConstraints string            `xml:"AccessConstraints"`
}

type wms111Capability struct {
	Request   wmsRequest   `xml:"Request"`
	Exception wmsException `xml:"Exception"`
	Layer     wms111Layer  `xml:"Layer"`
}

type wms111Layer struct {
	Queryable     bool                    `xml:"queryable,attr"`
	Opaque        bool                    `xml:"opaque,attr"`
	Name          string                  `xml:"Name"`
	Title         string                  `xml:"Title"`
	SRS           []string                `xml:"SRS"`
	LatLonBounds  wms111LatLonBoundingBox `xml:"LatLonBoundingBox"`
	BoundingBoxes []wms111BoundingBox     `xml:"BoundingBox"`
	Styles        []wmsStyle              `xml:"Style"`
}

type wms111LatLonBoundingBox struct {
	MinX float64 `xml:"minx,attr"`
	MinY float64 `xml:"miny,attr"`
	MaxX float64 `xml:"maxx,attr"`
	MaxY float64 `xml:"maxy,attr"`
}

type wms111BoundingBox struct {
	SRS  string  `xml:"SRS,attr"`
	MinX float64 `xml:"minx,attr"`
	MinY float64 `xml:"miny,attr"`
	MaxX float64 `xml:"maxx,attr"`
	MaxY float64 `xml:"maxy,attr"`
}

type wmsService struct {
	Name              string            `xml:"Name"`
	Title             string            `xml:"Title"`
	OnlineResource    wmsOnlineResource `xml:"OnlineResource"`
	Fees              string            `xml:"Fees"`
	AccessConstraints string            `xml:"AccessConstraints"`
	MaxWidth          int               `xml:"MaxWidth"`
	MaxHeight         int               `xml:"MaxHeight"`
}

type wmsCapability struct {
	Request   wmsRequest   `xml:"Request"`
	Exception wmsException `xml:"Exception"`
	Layer     wmsLayer     `xml:"Layer"`
}

type wmsRequest struct {
	GetCapabilities wmsOperation `xml:"GetCapabilities"`
	GetMap          wmsOperation `xml:"GetMap"`
}

type wmsOperation struct {
	Formats  []string     `xml:"Format"`
	DCPTypes []wmsDCPType `xml:"DCPType"`
}

type wmsDCPType struct {
	HTTP wmsHTTP `xml:"HTTP"`
}

type wmsHTTP struct {
	Get wmsHTTPGet `xml:"Get"`
}

type wmsHTTPGet struct {
	OnlineResource wmsOnlineResource `xml:"OnlineResource"`
}

type wmsOnlineResource struct {
	Type string `xml:"xlink:type,attr"`
	Href string `xml:"xlink:href,attr"`
}

type wmsException struct {
	Formats []string `xml:"Format"`
}

type wmsLayer struct {
	Queryable             bool                     `xml:"queryable,attr"`
	Opaque                bool                     `xml:"opaque,attr"`
	Name                  string                   `xml:"Name"`
	Title                 string                   `xml:"Title"`
	CRS                   []string                 `xml:"CRS"`
	GeographicBoundingBox wmsGeographicBoundingBox `xml:"EX_GeographicBoundingBox"`
	BoundingBoxes         []wmsBoundingBox         `xml:"BoundingBox"`
	Styles                []wmsStyle               `xml:"Style"`
}

type wmsGeographicBoundingBox struct {
	West  float64 `xml:"westBoundLongitude"`
	East  float64 `xml:"eastBoundLongitude"`
	South float64 `xml:"southBoundLatitude"`
	North float64 `xml:"northBoundLatitude"`
}

type wmsBoundingBox struct {
	CRS  string  `xml:"CRS,attr"`
	MinX float64 `xml:"minx,attr"`
	MinY float64 `xml:"miny,attr"`
	MaxX float64 `xml:"maxx,attr"`
	MaxY float64 `xml:"maxy,attr"`
}

type wmsStyle struct {
	Name  string `xml:"Name"`
	Title string `xml:"Title"`
}

func (s *Server) writeWMTSMetadata(w http.ResponseWriter, r *http.Request) {
	schemes, err := s.store.Schemes(r.Context())
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	root := s.publicRoot(r)
	endpoint := root + "/wmts"
	matrixSetIDs := make([]string, 0, len(schemes))
	for _, scheme := range schemes {
		matrixSetIDs = append(matrixSetIDs, scheme.ID)
	}
	capabilities := wmtsCapabilities{
		XMLNS: wmtsNamespace, XMLNSOWS: owsNamespace, XMLNSXLink: xlinkNamespace, Version: "1.0.0",
		ServiceIdentification: owsServiceIdentification{
			Title: "Geo Debug WMTS", ServiceType: "OGC WMTS", ServiceTypeVersion: "1.0.0",
		},
		OperationsMetadata: owsOperationsMetadata{Operations: []owsOperation{
			wmtsOperation("GetCapabilities", endpoint,
				owsParameterValues("AcceptVersions", "1.0.0"),
				owsParameterValues("AcceptFormats", "application/xml")),
			wmtsOperation("GetTile", endpoint,
				owsParameterValues("Layer", "debug"),
				owsParameterValues("Style", "default"),
				owsParameterValues("Format", "image/png"),
				owsParameterValues("TileMatrixSet", matrixSetIDs...)),
		}},
		Contents: wmtsContents{Layer: wmtsLayer{
			Title: "Geo Debug Layer", Identifier: "debug",
			Styles:  []wmtsStyle{{IsDefault: true, Identifier: "default"}},
			Formats: []string{"image/png"},
			ResourceURLs: []wmtsResourceURL{{
				Format: "image/png", ResourceType: "tile",
				Template: root + "/wmts/debug/default/{TileMatrixSet}/{TileMatrix}/{TileRow}/{TileCol}.png",
			}},
		}},
	}
	for _, scheme := range schemes {
		crs := wmtsCRS(scheme.CRS)
		capabilities.Contents.Layer.BoundingBoxes = append(capabilities.Contents.Layer.BoundingBoxes, owsBoundingBox{
			CRS: crs, LowerCorner: coordinatePair(scheme.MinX, scheme.MinY), UpperCorner: coordinatePair(scheme.MaxX, scheme.MaxY),
		})
		capabilities.Contents.Layer.MatrixSetLinks = append(capabilities.Contents.Layer.MatrixSetLinks,
			wmtsTileMatrixSetLink{TileMatrixSet: scheme.ID})
		matrixSet := wmtsTileMatrixSet{Title: scheme.Name, Identifier: scheme.ID, SupportedCRS: crs}
		for _, level := range scheme.Levels {
			matrixSet.Matrices = append(matrixSet.Matrices, wmtsTileMatrix{
				Identifier: level.Identifier, ScaleDenominator: level.ScaleDenominator,
				TopLeftCorner: coordinatePair(scheme.OriginX, scheme.OriginY),
				TileWidth:     scheme.TileWidth, TileHeight: scheme.TileHeight,
				MatrixWidth: level.MatrixWidth, MatrixHeight: level.MatrixHeight,
			})
		}
		capabilities.Contents.TileMatrixSets = append(capabilities.Contents.TileMatrixSets, matrixSet)
	}
	data, err := xml.MarshalIndent(capabilities, "", "  ")
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "NoApplicableCode", "encode WMTS capabilities: "+err.Error())
		return
	}
	data = append([]byte(xml.Header), data...)
	writeResponse(w, r, http.StatusOK, "application/xml; charset=utf-8", data)
}

func wmtsOperation(name, endpoint string, parameters ...owsParameter) owsOperation {
	return owsOperation{Name: name, Parameters: parameters, DCP: owsDCP{HTTP: owsHTTP{Get: owsGet{
		Href: endpoint + "?", Constraint: owsConstraint{
			Name: "GetEncoding", AllowedValues: owsAllowedValues{Values: []string{"KVP"}},
		},
	}}}}
}

func owsParameterValues(name string, values ...string) owsParameter {
	return owsParameter{Name: name, AllowedValues: owsAllowedValues{Values: values}}
}

func wmtsCRS(value string) string {
	if strings.EqualFold(value, "CRS:84") {
		return "urn:ogc:def:crs:OGC:1.3:CRS84"
	}
	if parts := strings.SplitN(value, ":", 2); len(parts) == 2 && strings.EqualFold(parts[0], "EPSG") {
		return "urn:ogc:def:crs:EPSG::" + parts[1]
	}
	return value
}

func coordinatePair(first, second float64) string {
	return fmt.Sprintf("%.15g %.15g", first, second)
}

func (s *Server) writeWMSMetadata(w http.ResponseWriter, r *http.Request, version string) {
	root := s.publicRoot(r)
	endpoint := root + "/wms"
	onlineResource := wmsOnlineResource{Type: "simple", Href: endpoint + "?"}
	var capabilities any
	switch version {
	case "1.1.1":
		capabilities = wms111Capabilities{
			XMLNSXLink: xlinkNamespace, Version: version,
			Service: wms111Service{
				Name: "OGC:WMS", Title: "Geo Debug WMS", OnlineResource: onlineResource,
				Fees: "none", AccessConstraints: "none",
			},
			Capabilities: wms111Capability{
				Request: wmsRequest{
					GetCapabilities: newWMSOperation(onlineResource, "application/vnd.ogc.wms_xml"),
					GetMap:          newWMSOperation(onlineResource, "image/png"),
				},
				Exception: wmsException{Formats: []string{"application/vnd.ogc.se_xml"}},
				Layer: wms111Layer{
					Queryable: false, Opaque: false, Name: "debug", Title: "Geo Debug Layer",
					SRS:          []string{"EPSG:3857", "EPSG:4326", "CRS:84"},
					LatLonBounds: wms111LatLonBoundingBox{MinX: -180, MinY: -90, MaxX: 180, MaxY: 90},
					BoundingBoxes: []wms111BoundingBox{
						{SRS: "EPSG:3857", MinX: -20037508.342789244, MinY: -20037508.342789244, MaxX: 20037508.342789244, MaxY: 20037508.342789244},
						{SRS: "EPSG:4326", MinX: -180, MinY: -90, MaxX: 180, MaxY: 90},
						{SRS: "CRS:84", MinX: -180, MinY: -90, MaxX: 180, MaxY: 90},
					},
					Styles: []wmsStyle{{Name: "default", Title: "Default Style"}},
				},
			},
		}
	case "1.3.0":
		capabilities = wmsCapabilities{
			XMLNS: wmsNamespace, XMLNSXLink: xlinkNamespace, Version: version,
			Service: wmsService{
				Name: "WMS", Title: "Geo Debug WMS", OnlineResource: onlineResource,
				Fees: "none", AccessConstraints: "none", MaxWidth: 4096, MaxHeight: 4096,
			},
			Capabilities: wmsCapability{
				Request: wmsRequest{
					GetCapabilities: newWMSOperation(onlineResource, "application/xml"),
					GetMap:          newWMSOperation(onlineResource, "image/png"),
				},
				Exception: wmsException{Formats: []string{"XML"}},
				Layer: wmsLayer{
					Queryable: false, Opaque: false, Name: "debug", Title: "Geo Debug Layer",
					CRS:                   []string{"EPSG:3857", "EPSG:4326", "CRS:84"},
					GeographicBoundingBox: wmsGeographicBoundingBox{West: -180, East: 180, South: -90, North: 90},
					BoundingBoxes: []wmsBoundingBox{
						{CRS: "EPSG:3857", MinX: -20037508.342789244, MinY: -20037508.342789244, MaxX: 20037508.342789244, MaxY: 20037508.342789244},
						{CRS: "EPSG:4326", MinX: -90, MinY: -180, MaxX: 90, MaxY: 180},
						{CRS: "CRS:84", MinX: -180, MinY: -90, MaxX: 180, MaxY: 90},
					},
					Styles: []wmsStyle{{Name: "default", Title: "Default Style"}},
				},
			},
		}
	default:
		s.writeError(w, r, http.StatusBadRequest, "InvalidParameterValue", "supported WMS versions are 1.3.0 and 1.1.1")
		return
	}
	data, err := xml.MarshalIndent(capabilities, "", "  ")
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "NoApplicableCode", "encode WMS capabilities: "+err.Error())
		return
	}
	data = append([]byte(xml.Header), data...)
	writeResponse(w, r, http.StatusOK, "application/xml; charset=utf-8", data)
}

func newWMSOperation(onlineResource wmsOnlineResource, formats ...string) wmsOperation {
	return wmsOperation{
		Formats:  formats,
		DCPTypes: []wmsDCPType{{HTTP: wmsHTTP{Get: wmsHTTPGet{OnlineResource: onlineResource}}}},
	}
}

func (s *Server) publicRoot(r *http.Request) string {
	if s.publicURL != "" {
		return s.publicURL
	}
	scheme := firstForwarded(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := firstForwarded(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host + s.basePath
}

func firstForwarded(value string) string {
	if index := strings.IndexByte(value, ','); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}
