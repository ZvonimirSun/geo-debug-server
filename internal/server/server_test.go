package server

import (
	"bytes"
	"context"
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
