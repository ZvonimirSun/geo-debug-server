# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags="-s -w \
    -X github.com/iszy/geo-debug-server/internal/buildinfo.Version=${VERSION} \
    -X github.com/iszy/geo-debug-server/internal/buildinfo.Commit=${COMMIT} \
    -X github.com/iszy/geo-debug-server/internal/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/geo-debug-server ./cmd/geo-debug-server

FROM alpine:3.22

RUN addgroup -S app && \
    adduser -S -G app app && \
    mkdir -p /data && \
    chown app:app /data

COPY --from=build /out/geo-debug-server /usr/local/bin/geo-debug-server

USER app

ENV GEO_DEBUG_LISTEN=:8080 \
    GEO_DEBUG_DATABASE=/data/geo-debug.db

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/geo-debug-server"]
