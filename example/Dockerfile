# Hardened image for the example service. Build from the repo root: make docker
# Base images are pinned by digest; Dependabot bumps them.
FROM cgr.dev/chainguard/go:latest@sha256:97b081e091e2c7df54b30f468969ea2e62d7d1a64fff89346facb564da979300 AS builder

WORKDIR /workdir

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY pkg ./pkg
COPY example ./example
ARG APP_VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=${APP_VERSION}" -o /app ./example

# --- Runtime: no shell, no package manager, only the static binary ---
FROM cgr.dev/chainguard/static:latest@sha256:fc42af33e643f20d08baf03cddcc1c02ba57052dfb02edb0d79f3fc47bcea8d9

COPY --from=builder /app /app

# Numeric so Kubernetes runAsNonRoot can verify it.
USER 65532:65532

ENV APP_ADDR=0.0.0.0:8080

EXPOSE 8080

# No shell or curl here to run a probe; probe /readyz from the orchestrator.
HEALTHCHECK NONE

ENTRYPOINT ["/app"]
CMD ["serve"]
