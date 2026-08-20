# geo-debug-server

用于定位地图瓦片请求问题的轻量调试服务。服务提供 XYZ、WMTS 和 WMS 接口，返回透明 PNG，并在图片内以黄色边框和居中文字标出切片方案、`z/x/y`、坐标范围、请求尺寸和附加参数。

## 运行

需要 Go 1.27.0：

```powershell
go run ./cmd/geo-debug-server
```

默认监听 `:8080`，基础路径为 `/geo-debug-server`，SQLite 数据库为 `./data/geo-debug.db`。

```text
Usage:
  -listen string       HTTP 监听地址，默认 :8080
  -base-path string    服务基础路径，默认 /geo-debug-server
  -public-url string   元数据中使用的公开基础 URL
  -database string     SQLite 文件路径，默认 ./data/geo-debug.db
```

对应环境变量为 `GEO_DEBUG_LISTEN`、`GEO_DEBUG_BASE_PATH`、`GEO_DEBUG_PUBLIC_URL` 和 `GEO_DEBUG_DATABASE`。命令行参数优先于环境变量。

查看当前二进制的版本、Git commit、构建时间、Go 版本和目标平台：

```sh
go run ./cmd/geo-debug-server --version
```

正式发布的容器镜像会在构建时写入 `VERSION` 和对应的 Git commit：

```sh
docker run --rm ghcr.io/<owner>/geo_debug_server:0.1.0 --version
```

## 当前支持的服务端点

以下路径均以默认基础路径 `/geo-debug-server` 为前缀：

| 方法 | 端点 | 功能 |
| --- | --- | --- |
| `GET` / `HEAD` | `/xyz/{z}/{x}/{y}.png` | 使用默认切片方案生成 XYZ 调试瓦片 |
| `GET` / `HEAD` | `/xyz/{scheme}/{z}/{x}/{y}.png` | 使用指定切片方案生成 XYZ 调试瓦片 |
| `GET` / `HEAD` | `/wmts` | 返回图层、切片方案和 `ResourceURL` 等 WMTS 元数据 |
| `GET` / `HEAD` | `/wmts?TILEMATRIX={z}&TILEROW={y}&TILECOL={x}` | 通过 KVP 参数生成 WMTS 调试瓦片 |
| `GET` / `HEAD` | `/wmts/{z}/{y}/{x}.png` | 使用默认图层、样式和切片方案生成 WMTS 调试瓦片 |
| `GET` / `HEAD` | `/wmts/{scheme}/{z}/{y}/{x}.png` | 使用指定切片方案和默认图层、样式生成 WMTS 调试瓦片 |
| `GET` / `HEAD` | `/wmts/{layer}/{style}/{scheme}/{z}/{y}/{x}.png` | 通过 REST 路径生成 WMTS 调试瓦片 |
| `GET` / `HEAD` | `/wms` | 返回图层、坐标系和 `GetMap` 资源模板等 WMS 元数据 |
| `GET` / `HEAD` | `/wms?REQUEST=GetMap&WIDTH={width}&HEIGHT={height}&...` | 按请求尺寸和参数生成 WMS 调试图片 |
| `OPTIONS` | 以上全部端点 | 返回跨域预检响应 |

`GET` 返回响应正文；`HEAD` 返回相同响应头但不返回正文。修改 `-base-path` 或 `GEO_DEBUG_BASE_PATH` 后，使用配置后的基础路径替换上述默认前缀。

## 默认切片方案

首次启动会自动创建数据库并补齐以下方案。已有方案不会被初始化逻辑覆盖。

| 标识 | 坐标系 | z0 矩阵 | 层级 | 瓦片大小 |
| --- | --- | --- | --- | --- |
| `WebMercatorQuad` | `EPSG:3857` | 1x1 | 0-22 | 256x256 |
| `WorldCRS84Quad` | `CRS:84` | 2x1 | 0-22 | 256x256 |

切片方案保存在 `tile_schemes`，每一级参数保存在 `tile_matrix_levels`。首版不提供管理 API，可以直接查看 SQLite 数据确认方案参数。

## XYZ

默认 Web Mercator：

```text
http://localhost:8080/geo-debug-server/xyz/{z}/{x}/{y}.png
```

指定切片方案：

```text
http://localhost:8080/geo-debug-server/xyz/{scheme}/{z}/{x}/{y}.png
```

## WMTS

REST 模板：

```text
http://localhost:8080/geo-debug-server/wmts/debug/default/{TileMatrixSet}/{TileMatrix}/{TileRow}/{TileCol}.png
```

省略图层、样式和切片方案时使用默认值：

```text
http://localhost:8080/geo-debug-server/wmts/{TileMatrix}/{TileRow}/{TileCol}.png
```

仅指定切片方案：

```text
http://localhost:8080/geo-debug-server/wmts/{TileMatrixSet}/{TileMatrix}/{TileRow}/{TileCol}.png
```

KVP 请求中以下参数可以省略：

```text
SERVICE=WMTS
REQUEST=GetTile
VERSION=1.0.0
LAYER=debug
STYLE=default
FORMAT=image/png
TILEMATRIXSET=WebMercatorQuad
```

`TILEMATRIX`、`TILEROW`、`TILECOL` 必须提供。例如：

```text
http://localhost:8080/geo-debug-server/wmts?TILEMATRIX=2&TILEROW=1&TILECOL=1
```

打开以下地址可以查看调试图层、数据库中的切片方案、各级矩阵信息和可复制的 `ResourceURL`：

```text
http://localhost:8080/geo-debug-server/wmts
```

## WMS

`GetMap` 支持常见 WMS 1.1.1/1.3.0 参数。`WIDTH`、`HEIGHT` 缺省为 256，`CRS/SRS` 缺省为 `EPSG:3857`，`BBOX` 缺省为对应坐标系的全球范围。

```text
http://localhost:8080/geo-debug-server/wms?REQUEST=GetMap&LAYERS=debug&CRS=EPSG:3857&BBOX=0,0,1000,1000&WIDTH=512&HEIGHT=256
```

打开 `/geo-debug-server/wms` 可以查看图层、支持的坐标系和 `GetMap` 资源模板。

## 调试参数

查询参数名不区分大小写。标准参数之外的参数会按名称排序后显示在图片中，`time` 仅在显式传入时显示：

```text
http://localhost:8080/geo-debug-server/xyz/3/4/2.png?time=step-1&source=leaflet
```

文字会先按像素宽度换行；内容超出高度时自动缩小字号并重新排版，不按高度省略正常请求内容。所有响应允许跨域访问，并支持 `GET`、`HEAD` 和 `OPTIONS`。

## Docker 部署

构建镜像：

```sh
docker build -t geo-debug-server .
```

使用命名卷持久化 SQLite 数据并启动服务：

```sh
docker run -d --name geo-debug-server --restart unless-stopped -p 8080:8080 -v geo-debug-data:/data geo-debug-server
```

容器默认监听 `:8080`，数据库位于 `/data/geo-debug.db`。运行时可以通过环境变量覆盖配置，例如：

```sh
docker run -d --name geo-debug-server --restart unless-stopped -p 9000:9000 -v geo-debug-data:/data -e GEO_DEBUG_LISTEN=:9000 -e GEO_DEBUG_BASE_PATH=/maps -e GEO_DEBUG_PUBLIC_URL=https://example.com/maps geo-debug-server
```

当服务位于反向代理之后时，建议设置 `GEO_DEBUG_PUBLIC_URL`，确保 WMTS/WMS 元数据中的资源地址使用外部可访问 URL。

### GitHub Container Registry

项目通过 `.github/workflows/docker.yml` 发布 `linux/amd64` 和 `linux/arm64` 镜像：

```text
ghcr.io/<owner>/geo_debug_server:<version>
```

镜像版本读取自根目录的 `VERSION` 文件。向 `main` 分支推送非文档改动时会自动发布完整版本、主次版本和主版本标签；已经存在的完整版本不会重复构建。工作流也支持在 GitHub Actions 页面手动触发。

发布新版本前先更新 `VERSION`，例如：

```text
0.2.0
```

拉取并运行已发布镜像：

```sh
docker pull ghcr.io/<owner>/geo_debug_server:0.1.0
docker run -d --name geo-debug-server --restart unless-stopped -p 8080:8080 -v geo-debug-data:/data ghcr.io/<owner>/geo_debug_server:0.1.0
```

## 测试

```powershell
go test ./...
go vet ./...
```
