package server

import (
	"bytes"
	"html/template"
	"net/http"
)

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>geo-debug-server</title>
  <style>
    :root { color-scheme: light dark; font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace; letter-spacing: 0; }
    * { box-sizing: border-box; }
    body { margin: 0; background: #101214; color: #e8eaed; }
    main { width: min(960px, calc(100% - 32px)); margin: 0 auto; padding: 48px 0 64px; }
    header { padding-bottom: 28px; border-bottom: 2px solid #f4d03f; }
    h1 { margin: 0; font-size: 30px; line-height: 1.2; font-weight: 700; }
    header p { margin: 10px 0 0; color: #aeb4bb; font-size: 14px; }
    section { padding: 28px 0; border-bottom: 1px solid #343a40; }
    .section-heading { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; margin-bottom: 14px; }
    h2 { margin: 0; font-size: 18px; line-height: 1.4; }
    .version { color: #f4d03f; font-size: 12px; white-space: nowrap; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; table-layout: fixed; }
    th, td { padding: 12px 10px; text-align: left; vertical-align: top; border-top: 1px solid #2c3136; }
    th { width: 28%; color: #aeb4bb; font-weight: 500; }
    code { color: #f5f6f7; white-space: normal; overflow-wrap: anywhere; }
    a { color: #f4d03f; text-decoration-thickness: 1px; text-underline-offset: 3px; }
    a:hover { color: #fff1a6; }
    footer { padding-top: 24px; color: #8f969e; font-size: 12px; overflow-wrap: anywhere; }
    @media (max-width: 640px) {
      main { width: min(100% - 24px, 960px); padding-top: 28px; }
      h1 { font-size: 24px; }
      th, td { display: block; width: 100%; padding-left: 0; padding-right: 0; }
      th { padding-bottom: 4px; }
      td { padding-top: 0; border-top: 0; }
      tr { display: block; padding: 10px 0; border-top: 1px solid #2c3136; }
    }
  </style>
</head>
<body>
  <main>
    <header>
      <h1>geo-debug-server</h1>
      <p>地图瓦片与动态地图服务调试端点</p>
    </header>

    <section>
      <div class="section-heading"><h2>XYZ</h2><span class="version">PATH</span></div>
      <table>
        <tbody>
          <tr><th>默认方案</th><td><a href="{{.Root}}/xyz/0/0/0.png"><code>{{.Root}}/xyz/{z}/{x}/{y}.png</code></a></td></tr>
          <tr><th>指定方案</th><td><code>{{.Root}}/xyz/{scheme}/{z}/{x}/{y}.png</code></td></tr>
        </tbody>
      </table>
    </section>

    <section>
      <div class="section-heading"><h2>WMTS</h2><span class="version">1.0.0</span></div>
      <table>
        <tbody>
          <tr><th>KVP Capabilities</th><td><a href="{{.Root}}/wmts"><code>{{.Root}}/wmts</code></a></td></tr>
          <tr><th>REST Capabilities</th><td><a href="{{.Root}}/wmts/1.0.0/WMTSCapabilities.xml"><code>{{.Root}}/wmts/1.0.0/WMTSCapabilities.xml</code></a></td></tr>
          <tr><th>KVP 瓦片</th><td><a href="{{.Root}}/wmts?TILEMATRIX=0&amp;TILEROW=0&amp;TILECOL=0"><code>{{.Root}}/wmts?TILEMATRIX={z}&amp;TILEROW={y}&amp;TILECOL={x}</code></a></td></tr>
          <tr><th>REST 瓦片</th><td><a href="{{.Root}}/wmts/0/0/0.png"><code>{{.Root}}/wmts/{TileMatrix}/{TileRow}/{TileCol}.png</code></a></td></tr>
          <tr><th>完整 REST 模板</th><td><code>{{.Root}}/wmts/{layer}/{style}/{TileMatrixSet}/{TileMatrix}/{TileRow}/{TileCol}.png</code></td></tr>
        </tbody>
      </table>
    </section>

    <section>
      <div class="section-heading"><h2>WMS</h2><span class="version">1.3.0 / 1.1.1</span></div>
      <table>
        <tbody>
          <tr><th>1.3.0 Capabilities</th><td><a href="{{.Root}}/wms"><code>{{.Root}}/wms</code></a></td></tr>
          <tr><th>1.1.1 Capabilities</th><td><a href="{{.Root}}/wms?REQUEST=GetCapabilities&amp;VERSION=1.1.1"><code>{{.Root}}/wms?REQUEST=GetCapabilities&amp;VERSION=1.1.1</code></a></td></tr>
          <tr><th>GetMap</th><td><a href="{{.Root}}/wms?REQUEST=GetMap&amp;WIDTH=512&amp;HEIGHT=256"><code>{{.Root}}/wms?REQUEST=GetMap&amp;WIDTH={width}&amp;HEIGHT={height}</code></a></td></tr>
        </tbody>
      </table>
    </section>

    <footer>Service root: {{.Root}}</footer>
  </main>
</body>
</html>`))

func (s *Server) writeIndex(w http.ResponseWriter, r *http.Request) {
	var output bytes.Buffer
	if err := indexTemplate.Execute(&output, struct{ Root string }{Root: s.publicRoot(r)}); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "NoApplicableCode", "render service index: "+err.Error())
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeResponse(w, r, http.StatusOK, "text/html; charset=utf-8", output.Bytes())
}
