package server

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) writeWMTSMetadata(w http.ResponseWriter, r *http.Request) {
	schemes, err := s.store.Schemes(r.Context())
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	root := s.publicRoot(r)
	var output bytes.Buffer
	output.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	output.WriteString(`<Capabilities service="WMTS" version="1.0.0">` + "\n")
	output.WriteString("  <Layer>\n    <Identifier>debug</Identifier>\n    <Title>Geo Debug Layer</Title>\n    <Format>image/png</Format>\n")
	for _, scheme := range schemes {
		fmt.Fprintf(&output, "    <TileMatrixSetLink><TileMatrixSet>%s</TileMatrixSet></TileMatrixSetLink>\n", xmlText(scheme.ID))
		template := fmt.Sprintf("%s/wmts/debug/default/%s/{TileMatrix}/{TileRow}/{TileCol}.png", root, scheme.ID)
		fmt.Fprintf(&output, "    <ResourceURL format=\"image/png\" resourceType=\"tile\" template=\"%s\"/>\n", xmlText(template))
	}
	output.WriteString("  </Layer>\n")
	for _, scheme := range schemes {
		fmt.Fprintf(&output, "  <TileMatrixSet id=\"%s\">\n", xmlText(scheme.ID))
		fmt.Fprintf(&output, "    <Identifier>%s</Identifier>\n", xmlText(scheme.ID))
		fmt.Fprintf(&output, "    <Title>%s</Title>\n", xmlText(scheme.Name))
		fmt.Fprintf(&output, "    <SupportedCRS>%s</SupportedCRS>\n", xmlText(scheme.CRS))
		fmt.Fprintf(&output, "    <Extent>%.12g,%.12g,%.12g,%.12g</Extent>\n", scheme.MinX, scheme.MinY, scheme.MaxX, scheme.MaxY)
		for _, level := range scheme.Levels {
			fmt.Fprintf(&output, "    <TileMatrix id=\"%s\" resolution=\"%.15g\" scaleDenominator=\"%.15g\" matrixWidth=\"%d\" matrixHeight=\"%d\" tileWidth=\"%d\" tileHeight=\"%d\"/>\n",
				xmlText(level.Identifier), level.Resolution, level.ScaleDenominator,
				level.MatrixWidth, level.MatrixHeight, scheme.TileWidth, scheme.TileHeight)
		}
		output.WriteString("  </TileMatrixSet>\n")
	}
	fmt.Fprintf(&output, "  <KVPResourceURL>%s/wmts</KVPResourceURL>\n", xmlText(root))
	output.WriteString("</Capabilities>\n")
	writeResponse(w, r, http.StatusOK, "application/xml; charset=utf-8", output.Bytes())
}

func (s *Server) writeWMSMetadata(w http.ResponseWriter, r *http.Request) {
	root := s.publicRoot(r)
	endpoint := root + "/wms"
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<WMS_Capabilities version="1.3.0">
  <Service>
    <Name>WMS</Name>
    <Title>Geo Debug WMS</Title>
    <OnlineResource>%s</OnlineResource>
  </Service>
  <Capability>
    <Request>
      <GetMap>
        <Format>image/png</Format>
        <ResourceURL>%s?SERVICE=WMS&amp;REQUEST=GetMap&amp;LAYERS=debug&amp;CRS=EPSG:3857&amp;BBOX={bbox}&amp;WIDTH={width}&amp;HEIGHT={height}&amp;FORMAT=image/png</ResourceURL>
      </GetMap>
    </Request>
    <Layer>
      <Name>debug</Name>
      <Title>Geo Debug Layer</Title>
      <CRS>EPSG:3857</CRS>
      <CRS>EPSG:4326</CRS>
      <CRS>CRS:84</CRS>
    </Layer>
  </Capability>
</WMS_Capabilities>
`, xmlText(endpoint), xmlText(endpoint))
	writeResponse(w, r, http.StatusOK, "application/xml; charset=utf-8", []byte(content))
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

func xmlText(value string) string {
	var output bytes.Buffer
	_ = xmlEscape(&output, value)
	return output.String()
}

func xmlEscape(output *bytes.Buffer, value string) error {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	output.WriteString(replacer.Replace(value))
	return nil
}
