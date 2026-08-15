# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# TARGETOS/TARGETARCH are injected by Buildx (defaults below keep a plain
# `docker build` working too). CGO_ENABLED=0 -> no cgo; the resulting binary
# is glibc-linked (the modernc.org/sqlite stack needs libc) and runs on any
# standard glibc Linux. Use a glibc runtime base below so it exec's cleanly.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/immich-go .

# ---- runtime stage ----
# Debian (glibc) base: the binary is glibc-linked, so Alpine/musl would fail
# with "no such file or directory". ffmpeg (libav*/libsw*) is installed
# system-wide so the purego video loader (internal/video) can dlopen them by
# soname from /usr/lib — in-container video thumbnail/transcode works OOTB.
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg \
    && rm -rf /var/lib/apt/lists/*
# The purego video loader (internal/video) looks for unversioned sonames
# (libavformat.so, ...) inside <exe dir>/libs. Distro runtime packages only
# ship versioned ones (libavformat.so.59 ...), so create unversioned symlinks
# next to the binary. Their NEEDED deps resolve via the system ld.so cache.
RUN mkdir -p /usr/local/bin/libs && \
    triplet=$(dpkg-architecture -qDEB_HOST_MULTIARCH 2>/dev/null || echo x86_64-linux-gnu); \
    for l in libavformat libavcodec libavutil libswscale libswresample libavfilter libavdevice; do \
      f=$(ls /usr/lib/$triplet/$l.so.* 2>/dev/null | head -1); \
      [ -n "$f" ] && ln -sf "$f" /usr/local/bin/libs/$l.so; \
    done && \
    ls -l /usr/local/bin/libs
WORKDIR /data
COPY --from=build /out/immich-go /usr/local/bin/immich-go
ENV IMMICH_PORT=8081 \
    IMMICH_DB=/data/immich.db \
    IMMICH_RESOURCE=/data/resources
EXPOSE 8081
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/immich-go"]

# To build a multi-arch image (e.g. linux/amd64 + linux/arm64), enable QEMU
# in the workflow and set `platforms: linux/amd64,linux/arm64` in the
# build-push step. ARM is emulated and slower, so it is off by default to
# keep CI minutes low.
