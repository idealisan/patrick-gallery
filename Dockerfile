# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# TARGETOS/TARGETARCH are injected by Buildx (defaults below keep a plain
# `docker build` working too). CGO_ENABLED=0 -> fully static binary.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/immich-go .

# ---- runtime stage ----
FROM alpine:3.20
WORKDIR /data
COPY --from=build /out/immich-go /usr/local/bin/immich-go
ENV IMMICH_PORT=8081 \
    IMMICH_DB=/data/immich.db \
    IMMICH_RESOURCE=/data/resources
EXPOSE 8081
VOLUME ["/data"]
ENTRYPOINT ["immich-go"]

# To build a multi-arch image (e.g. linux/amd64 + linux/arm64), enable QEMU
# in the workflow and set `platforms: linux/amd64,linux/arm64` in the
# build-push step. ARM is emulated and slower, so it is off by default to
# keep CI minutes low.
