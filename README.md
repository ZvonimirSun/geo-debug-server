# geo-debug-server

用于定位地图瓦片和动态地图请求问题的轻量调试服务。服务提供 XYZ、WMTS、WMS、ArcGIS Tile MapServer、ArcGIS Dynamic MapServer 和 SuperMap REST Map 接口，默认返回透明 PNG；切片图片以黄色边框和居中文字逐行标出 `z`、`x`、`y` 及可选的 `time`。

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
  -scheme-cache-ttl duration  切片方案内存缓存的滑动 TTL，默认 5m，设置 0 可禁用
```

对应环境变量为 `GEO_DEBUG_LISTEN`、`GEO_DEBUG_BASE_PATH`、`GEO_DEBUG_PUBLIC_URL`、`GEO_DEBUG_DATABASE` 和 `GEO_DEBUG_SCHEME_CACHE_TTL`。命令行参数优先于环境变量。

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
| `GET` / `HEAD` | `/` | 返回当前服务支持的全部服务地址说明页 |
| `GET` / `HEAD` | `/xyz/{z}/{x}/{y}.png` | 使用默认切片方案生成 XYZ 调试瓦片 |
| `GET` / `HEAD` | `/xyz/{scheme}/{z}/{x}/{y}.png` | 使用指定切片方案生成 XYZ 调试瓦片 |
| `GET` / `HEAD` | `/wmts` | 返回 WMTS 1.0.0 Capabilities，包含操作、默认参数、图层、切片方案和 `ResourceURL` |
| `GET` / `HEAD` | `/wmts/1.0.0/WMTSCapabilities.xml` | 通过 REST 路径返回 WMTS 1.0.0 Capabilities |
| `GET` / `HEAD` | `/wmts?TILEMATRIX={z}&TILEROW={y}&TILECOL={x}` | 通过 KVP 参数生成 WMTS 调试瓦片 |
| `GET` / `HEAD` | `/wmts/{z}/{y}/{x}.png` | 使用默认图层、样式和切片方案生成 WMTS 调试瓦片 |
| `GET` / `HEAD` | `/wmts/{scheme}/{z}/{y}/{x}.png` | 使用指定切片方案和默认图层、样式生成 WMTS 调试瓦片 |
| `GET` / `HEAD` | `/wmts/{layer}/{style}/{scheme}/{z}/{y}/{x}.png` | 通过 REST 路径生成 WMTS 调试瓦片 |
| `GET` / `HEAD` | `/wms` | 返回默认的 WMS 1.3.0 Capabilities，包含操作、默认图层和样式、坐标系、范围及尺寸限制 |
| `GET` / `HEAD` | `/wms?REQUEST=GetCapabilities&VERSION=1.1.1` | 返回 WMS 1.1.1 Capabilities |
| `GET` / `HEAD` | `/wms?REQUEST=GetMap&WIDTH={width}&HEIGHT={height}&...` | 按请求尺寸和参数生成 WMS 调试图片 |
| `GET` / `HEAD` | `/ags_tile?f=json` | 使用默认切片方案返回 ArcGIS Tile MapServer JSON 元数据 |
| `GET` / `HEAD` | `/ags_tile/{scheme}/?f=pjson` | 使用指定切片方案返回格式化的 ArcGIS Tile MapServer JSON 元数据 |
| `GET` / `HEAD` | `/ags_tile/tile/{z}/{y}/{x}` | 使用默认切片方案生成 ArcGIS REST 调试瓦片 |
| `GET` / `HEAD` | `/ags_tile/{scheme}/tile/{z}/{y}/{x}` | 使用指定切片方案生成 ArcGIS REST 调试瓦片 |
| `GET` / `HEAD` | `/ags_rest?f=json|pjson` | 使用默认切片方案返回 ArcGIS Dynamic MapServer 元数据 |
| `GET` / `HEAD` | `/ags_rest/{scheme}/?f=json|pjson` | 使用指定切片方案返回 ArcGIS Dynamic MapServer 元数据 |
| `GET` / `HEAD` | `/ags_rest/export?...&f=image` | 使用默认切片方案生成 ArcGIS REST 动态图片 |
| `GET` / `HEAD` | `/ags_rest/{scheme}/export?...&f=json|pjson` | 返回指定方案的 Export Map 结果及图片地址 |
| `GET` / `HEAD` | `/spm_rest/iserver/services/map-debug/rest/maps/{scheme}` | 根据 SQLite 切片方案返回 SuperMap REST 地图元数据 |
| `GET` / `HEAD` | `/spm_rest/iserver/services/map-debug/rest/maps/{scheme}/tileImage.png?...` | 根据切片方案和请求参数生成 SuperMap REST 动态调试图片 |
| `GET` / `HEAD` | `/spm_rest/iserver/services/map-debug/rest/maps/{scheme}/tilesets.json` | 返回空切片集 `[]`，模拟非切片服务 |
| `GET` / `HEAD` | `/spm_rest/iserver/manager/license.json` | 返回 SuperMap iServer 许可兼容信息 |
| `GET` / `HEAD` | `/spm_tile/iserver/services/map-debug/rest/maps/{scheme}` | 返回标记为缓存切片服务的 SuperMap 地图元数据 |
| `GET` / `HEAD` | `/spm_tile/iserver/services/map-debug/rest/maps/{scheme}/tilesets.json` | 根据 SQLite 方案返回 SuperMap 切片集元数据 |
| `GET` / `HEAD` | `/spm_tile/iserver/services/map-debug/rest/maps/{scheme}/tileImage.png?...` | 生成 SuperMap 切片服务调试图片 |
| `GET` / `HEAD` | `/spm_tile/iserver/manager/license.json` | 返回 SuperMap 切片服务许可兼容信息 |
| `GET` / `HEAD` | `/schemes` | 列出当前全部切片方案及矩阵层级 |
| `POST` | `/schemes` | 新增切片方案及完整矩阵层级 |
| `DELETE` | `/schemes/{scheme}` | 移除切片方案；移除默认方案时自动选择剩余方案作为默认 |
| `PUT` | `/schemes/{scheme}/default` | 将指定切片方案设置为唯一默认方案 |
| `OPTIONS` | 以上全部端点 | 返回跨域预检响应 |

`GET` 返回响应正文；`HEAD` 返回相同响应头但不返回正文。修改 `-base-path` 或 `GEO_DEBUG_BASE_PATH` 后，使用配置后的基础路径替换上述默认前缀。

服务说明页位于配置后的基础路径末尾 `/`。例如默认地址为 `/geo-debug-server/`；将基础路径配置为 `/base-url` 后，说明页地址为 `/base-url/`。基础路径之外的地址（包括不带末尾 `/` 的基础路径）会以 `302` 重定向到说明页；基础路径内未匹配服务端点的地址仍返回 `404`。

XYZ 直接通过路径生成瓦片，不提供 Capabilities；WMTS 和 WMS 提供可由客户端解析的标准 Capabilities；ArcGIS Tile MapServer 和 Dynamic MapServer 提供 JSON 元数据。

## 默认切片方案

仅当配置的 SQLite 文件不存在时，服务才会创建数据库并通过方案新增逻辑插入以下初始方案。只要文件已经存在，启动过程就不会建表、补方案、补层级或修改默认值；未来表结构变化将通过独立版本和迁移脚本处理。

| 标识 | 坐标系 | z0 矩阵 | 层级 | 瓦片大小 | 默认 |
| --- | --- | --- | --- | --- | --- |
| `WebMercatorQuad` | `EPSG:3857` | 1x1 | 0-22 | 256x256 | 否 |
| `WorldCRS84Quad` | `CRS:84` | 1x1 | 0-23 | 256x256 | 否 |
| `CGCS2000Quad` | `EPSG:4490` | 1x1 | 0-23 | 256x256 | 是 |

两个经纬度方案的 `z0` 分辨率为 `1.40625`，比例尺与 `WebMercatorQuad z0` 对齐；原先以 `0.703125` 开始的层级顺延为 `z1`，矩阵为 `2x1`。

切片方案保存在 `tile_schemes`，每一级参数保存在 `tile_matrix_levels`。数据库只保存层级分辨率，Capabilities 中的 `ScaleDenominator` 根据分辨率、方案坐标单位和请求 DPI 动态计算。`tile_schemes.y_coordinate_first` 用于指定 WMTS 元数据是否按 y/x 输出坐标；值为 `1` 时交换内部 x/y 顺序，值为 `0` 时保持 x/y。

服务会在进程内缓存查询成功的切片方案及层级信息，默认滑动 TTL 为 5 分钟。每次命中都会重新续期；超过 TTL 未访问的方案会自动从内存释放，下次请求重新读取 SQLite。并发缓存 miss 只会执行一次 SQLite 加载。直接修改数据库后，变更最迟在对应缓存停止访问并过期后可见。

### 切片方案管理

管理接口使用 JSON 且不包含应用内授权逻辑。新增请求必须提供从 `minZoom` 到 `maxZoom` 的全部矩阵层级，整个方案在同一 SQLite 事务内写入。例如：

```json
{
  "id": "LocalQuad",
  "name": "Local Quad",
  "crs": "EPSG:4326",
  "metersPerUnit": 111319.49079327358,
  "tileWidth": 256,
  "tileHeight": 256,
  "minZoom": 0,
  "maxZoom": 1,
  "originX": -180,
  "originY": 90,
  "minX": -180,
  "minY": -90,
  "maxX": 180,
  "maxY": 90,
  "yCoordinateFirst": true,
  "isDefault": false,
  "levels": [
    {"zoom": 0, "identifier": "0", "resolution": 1.40625, "matrixWidth": 1, "matrixHeight": 1},
    {"zoom": 1, "identifier": "1", "resolution": 0.703125, "matrixWidth": 2, "matrixHeight": 1}
  ]
}
```

方案 ID 大小写不敏感且不能包含路径分隔符。重复 ID 返回 `409`，无效方案返回 `400`，未知方案返回 `404`。新增、移除和设置默认成功后会立即清除方案内存缓存。

```sh
geo-debug-server --scheme-cache-ttl 30s
geo-debug-server --scheme-cache-ttl 0
```

## XYZ

默认 CGCS2000：

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
TILEMATRIXSET=CGCS2000Quad
```

`TILEMATRIX`、`TILEROW`、`TILECOL` 必须提供。例如：

```text
http://localhost:8080/geo-debug-server/wmts?TILEMATRIX=2&TILEROW=1&TILECOL=1
```

打开以下地址可以查看调试图层、数据库中的切片方案、各级矩阵信息和可复制的 `ResourceURL`：

```text
http://localhost:8080/geo-debug-server/wmts
```

WMTS 支持 1.0.0。该响应使用 WMTS 1.0.0 和 OWS 1.1 标准命名空间及结构，包含 `GetCapabilities`、`GetTile`、KVP/RESTFUL 编码声明、WGS84 范围、默认图层/样式/格式、REST 资源模板、服务元数据地址、可用矩阵集和完整瓦片矩阵。`WGS84BoundingBox` 固定使用经度、纬度顺序；`TopLeftCorner` 遵循矩阵集 CRS 的轴顺序，因此 `EPSG:4326` 为纬度、经度，而 `CRS:84` 为经度、纬度。服务商、联系人、关键词及 `GetFeatureInfo` 等与调试瓦片无关的信息已省略。

Capabilities 支持 `DPI` 扩展参数，参数名不区分大小写，必须为有限正数。缺省 DPI 按 WMTS 的 `0.28mm` 标准像素计算，即 `90.7142857142857`；指定 DPI 只会换算元数据中的 `ScaleDenominator`，不会改变瓦片分辨率、矩阵大小或范围：

```text
http://localhost:8080/geo-debug-server/wmts?REQUEST=GetCapabilities&DPI=96
http://localhost:8080/geo-debug-server/wmts/1.0.0/WMTSCapabilities.xml?DPI=96
```

## WMS

WMS 支持 1.3.0 和 1.1.1，缺省版本为 1.3.0。`GetMap` 支持两个版本的常见参数；`WIDTH`、`HEIGHT` 缺省为 256，`CRS/SRS` 缺省为 `EPSG:3857`，`BBOX` 缺省为对应版本和坐标系的全球范围。

```text
http://localhost:8080/geo-debug-server/wms?REQUEST=GetMap&LAYERS=debug&CRS=EPSG:3857&BBOX=0,0,1000,1000&WIDTH=512&HEIGHT=256
```

打开 `/geo-debug-server/wms` 可以查看标准 WMS 1.3.0 Capabilities；指定 `VERSION=1.1.1` 可以查看 WMS 1.1.1 Capabilities。响应包含 `GetCapabilities`、`GetMap`、默认图层 `debug`、默认样式 `default`、PNG 格式和支持的坐标系与范围。WMS 1.3.0 还声明最大宽高 4096 像素限制。服务商和联系人等与请求无关的信息已省略。

XYZ 是路径式调试切片接口，不定义协议版本，也不提供 Capabilities。

## ArcGIS Tile MapServer

ArcGIS REST 风格切片服务通过 `ags_tile` 路径提供。元数据请求必须指定 `f=json` 或 `f=pjson`；两者字段相同，`pjson` 使用缩进格式便于查看。默认方案和指定方案示例：

```text
http://localhost:8080/geo-debug-server/ags_tile?f=json
http://localhost:8080/geo-debug-server/ags_tile/WebMercatorQuad/?f=pjson
```

元数据包含空间参考、完整范围、原点、瓦片尺寸、96 DPI、LOD 分辨率与比例尺等 Tile MapServer 常用字段。瓦片路径为：

```text
http://localhost:8080/geo-debug-server/ags_tile/tile/{z}/{y}/{x}
http://localhost:8080/geo-debug-server/ags_tile/{scheme}/tile/{z}/{y}/{x}
```

## ArcGIS Dynamic MapServer

ArcGIS REST 风格动态地图服务通过 `ags_rest` 路径提供。元数据请求必须指定 `f=json` 或 `f=pjson`，默认使用当前默认切片方案，也可在路径中指定方案：

```text
http://localhost:8080/geo-debug-server/ags_rest?f=pjson
http://localhost:8080/geo-debug-server/ags_rest/WebMercatorQuad/?f=pjson
```

元数据包含空间参考、完整范围、图层、支持的图片格式和最大出图尺寸，并标记 `supportsDynamicLayers=true`、`singleFusedMapCache=false`。该动态服务不返回切片服务专用的 `tileInfo`。

`export` 支持直接返回 PNG，也支持返回包含图片地址、尺寸、范围和比例尺的 JSON/PJSON：

```text
http://localhost:8080/geo-debug-server/ags_rest/export?bbox=-180,-90,180,90&size=512,256&bboxSR=4490&imageSR=4490&dpi=96&format=png32&transparent=true&f=image
http://localhost:8080/geo-debug-server/ags_rest/export?bbox=-180,-90,180,90&size=512,256&f=pjson
http://localhost:8080/geo-debug-server/ags_rest/WebMercatorQuad/export?bbox=-20037508.342789244,-20037508.342789244,20037508.342789244,20037508.342789244&size=512,256&f=image
```

`bbox` 默认使用方案完整范围，`size` 默认 `256,256`，`bboxSR` 和 `imageSR` 默认使用方案坐标系，`dpi` 默认 96，`format` 默认 `png32`，`transparent` 默认 `true`，`f` 默认 `image`。支持的图片格式为 `png`、`png8`、`png24` 和 `png32`；单边尺寸限制为 8-4096 像素，总像素不超过 16,777,216。首版只展示请求参数，不进行 `bboxSR` 与 `imageSR` 之间的坐标转换。

## SuperMap REST Map

SuperMap REST 风格动态地图服务使用包含 `/iserver/services/map-debug/rest/maps/` 的固定路径结构，便于 iClient 按 URL 识别服务。地图资源名称使用 SQLite 中的切片方案 ID；服务直接读取方案参数，不进行坐标转换：

```text
http://localhost:8080/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad
http://localhost:8080/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/WebMercatorQuad.json
```

元数据中的 CRS、`viewBounds`、`bounds`、`center`、`origin`、瓦片尺寸、层级范围和分辨率均来自对应切片方案。`coordUnit` 根据方案的 `metersPerUnit` 区分米制和角度单位；CRS 仅解析常见 EPSG 标识用于填写 `epsgCode`，不保存或计算投影转换。未知方案返回 `404`。

同一地图资源下的 `tileImage.png` 统一生成动态调试图片。`width`、`height` 默认使用方案瓦片尺寸，图片中的 `origin` 和 `bounds` 来自方案；`scale`、`x`、`y`、`layersID`、`cacheEnabled` 和 `redirect` 仅展示请求值，不进行瓦片矩阵或坐标范围校验：

```text
http://localhost:8080/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad/tileImage.png?width=512&height=256&scale=0.000001&x=0&y=0&cacheEnabled=true&transparent=true
```

单边尺寸限制为 8-4096 像素，总像素不超过 16,777,216。`cacheEnabled=true` 不会改变服务端生成逻辑；成功图片仍遵循本服务统一的 HTTP 长期缓存规则。

动态服务没有缓存切片集，`spm_rest` 下每个方案的 `tilesets.json` 与真实 iServer 动态地图服务一致返回空数组：

```text
http://localhost:8080/geo-debug-server/spm_rest/iserver/services/map-debug/rest/maps/CGCS2000Quad/tilesets.json
```

iClient 加载时访问的许可检查地址位于相同的 `spm_rest/iserver` 路径下：

```text
http://localhost:8080/geo-debug-server/spm_rest/iserver/manager/license.json
```

## SuperMap Tile Map

缓存切片服务使用相同的 iServer 路径结构，以 `spm_tile` 和动态服务区分。地图元数据中的 `cacheEnabled` 为 `true`：

```text
http://localhost:8080/geo-debug-server/spm_tile/iserver/services/map-debug/rest/maps/CGCS2000Quad
```

`tilesets.json` 根据 SQLite 方案生成一项切片集，包含完整的分辨率、96 DPI 比例尺、原点、瓦片尺寸、方案范围、CRS、PNG 格式和 Compact 存储类型。无需额外的方案字段：

```text
http://localhost:8080/geo-debug-server/spm_tile/iserver/services/map-debug/rest/maps/CGCS2000Quad/tilesets.json
```

切片图片和许可兼容端点分别为：

```text
http://localhost:8080/geo-debug-server/spm_tile/iserver/services/map-debug/rest/maps/CGCS2000Quad/tileImage.png?width=256&height=256&scale=0.000001&x=0&y=0&cacheEnabled=true
http://localhost:8080/geo-debug-server/spm_tile/iserver/manager/license.json
```

## 调试参数

查询参数名不区分大小写。XYZ、WMTS 和 ArcGIS REST 瓦片仅显示 `z`、`x`、`y`，`time` 仅在显式传入时显示；ArcGIS Dynamic MapServer Export、SuperMap tileImage 和 WMS GetMap 还会按参数名排序显示其他参数：

```text
http://localhost:8080/geo-debug-server/xyz/3/4/2.png?time=step-1&source=test
```

所有图片接口还支持以下渲染参数。颜色值省略 `#`，支持 `RGB`、`RGBA`、`RRGGBB` 和 `RRGGBBAA` 格式；末尾的 Alpha 表示透明度，省略时为 `FF`：

| 参数 | 行为 |
| --- | --- |
| `transparent` | 默认为透明；设为 `false` 时填充背景 |
| `bgColor` | 背景色，缺失或格式无效时回退为 50% 透明黑色 `00000080` |
| `color` | 文字颜色，缺失或格式无效时沿用默认黄色 `FFFF00` |

例如：

```text
http://localhost:8080/geo-debug-server/xyz/3/4/2.png?transparent=false&bgColor=FFF8&color=003366CC
```

文字会先按像素宽度换行；内容超出高度时自动缩小字号并重新排版。所有响应允许任意来源跨域访问、任意请求头和任意响应头；跨域方法包含 `GET`、`HEAD`、`POST`、`PUT`、`DELETE` 和 `OPTIONS`。预检响应缓存 24 小时，不启用跨域凭据模式。

## 缓存行为

成功生成的 XYZ、WMTS、WMS、ArcGIS REST 和 SuperMap REST PNG 默认返回长期不可变缓存：

```text
Cache-Control: public, max-age=31536000, immutable
```

同一路径和查询参数对应的图片内容保持不变。需要绕过缓存时，请求中可以设置以下任一请求头：

```text
Cache-Control: no-cache
Cache-Control: no-store
Cache-Control: max-age=0
Pragma: no-cache
```

此时响应返回 `Cache-Control: no-store`。元数据和错误响应始终使用 `no-store`，不会长期缓存。

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
