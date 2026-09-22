# syntax=docker/dockerfile:1

# Runtime-only image.
#
# The binary is compiled by CI (native Go cross-compile on the runner — no
# emulation) and staged into the build context per architecture as
# dockerctx/<arch>/immich-go, so this Dockerfile only packages the artifact
# plus runtime configuration. There is deliberately NO golang/toolchain stage:
# compiling inside a QEMU-emulated arm64 stage is what made image builds slow.

FROM debian:bookworm-slim

# FFmpeg shared libs, installed system-wide so the purego video loader
# (internal/video) can dlopen them by soname. Debian (glibc) base is required:
# the binary is glibc-linked, so Alpine/musl would fail. The apt cache mounts
# keep rebuilds fast (the arm64 layer is QEMU-emulated, so caching matters).
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg \
 && rm -rf /var/lib/apt/lists/*

# The loader looks for unversioned sonames (libavformat.so, ...) inside
# <exe dir>/libs; distro packages only ship versioned ones (libavformat.so.59
# ...), so create unversioned symlinks next to the binary. Their NEEDED deps
# resolve via the system ld.so cache. Note: a missing lib must NOT fail the
# build (the video backend degrades gracefully to the placeholder).
RUN set -e; \
    mkdir -p /usr/local/bin/libs; \
    triplet="$(dpkg-architecture -qDEB_HOST_MULTIARCH)"; \
    for l in libavformat libavcodec libavutil libswscale libswresample libavfilter libavdevice; do \
      f="$(ls /usr/lib/"$triplet"/"$l".so.* 2>/dev/null | head -n1 || true)"; \
      if [ -n "$f" ]; then ln -sf "$f" "/usr/local/bin/libs/$l.so"; fi; \
    done; \
    ls -l /usr/local/bin/libs

# Pre-built binary, selected per target architecture by BuildKit.
ARG TARGETARCH
COPY dockerctx/${TARGETARCH}/immich-go /usr/local/bin/immich-go
RUN chmod +x /usr/local/bin/immich-go

WORKDIR /data
ENV IMMICH_PORT=8081 \
    IMMICH_DB=/data/immich.db \
    IMMICH_RESOURCE=/data/resources
EXPOSE 8081
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/immich-go"]
