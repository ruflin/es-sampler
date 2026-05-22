# syntax=docker/dockerfile:1.7
#
# Multi-stage build:
#  - builder: chainguard/go produces a static CGO_ENABLED=0 binary.
#  - runtime: chainguard/static (~2 MB, no shell, nonroot) executes it.
#
# Base images are pinned by digest so rebuilds are reproducible. Refresh with
# `docker pull` + `docker inspect --format='{{index .RepoDigests 0}}'`.

ARG BUILD_DATE=unknown
ARG SOURCE_COMMIT=unknown
ARG VERSION=dev

FROM cgr.dev/chainguard/go:latest@sha256:29c70d8bd05a956a07e880f8e00217172be765b36cdcd70ebe5c26a5ee9243c7 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION
ARG SOURCE_COMMIT
RUN CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${SOURCE_COMMIT}" \
      -o /out/es-sampler \
      .

FROM cgr.dev/chainguard/static:latest@sha256:77d8b8925dc27970ec2f48243f44c7a260d52c49cd778288e4ee97566e0cb75b

ARG BUILD_DATE
ARG SOURCE_COMMIT
ARG VERSION

LABEL org.opencontainers.image.title="es-sampler" \
      org.opencontainers.image.description="Sample documents from a source Elasticsearch cluster into a destination cluster." \
      org.opencontainers.image.source="https://github.com/ruflin/es-sampler" \
      org.opencontainers.image.revision="${SOURCE_COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /out/es-sampler /usr/local/bin/es-sampler

USER nonroot
ENTRYPOINT ["/usr/local/bin/es-sampler"]
