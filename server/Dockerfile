# Stage 0: Build the web dashboard for embedding into the server binary
FROM node:20-alpine AS web-builder

WORKDIR /web

COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts

COPY web/ ./
RUN npx vite build

# Stage 0b: Supply-chain self-attestation. Runs the SAME scripts/dep-scan.sh the
# CI/release pipeline runs, so an operator's own docker-compose build gates its
# dependency supply chain and produces the attested posture embedded into the
# server below. A real un-accepted vulnerability fails the build here; an offline
# build (advisory DBs unreachable) degrades to an honest unattested posture
# rather than blocking an air-gapped operator.
FROM golang:1.26-bookworm AS posture-builder

WORKDIR /src

# Rust toolchain from the canonical official Rust image, not apt and not a
# curl|sh rustup.rs installer: bookworm's packaged cargo (~1.65) is too old to
# build a current cargo-audit (needs the 2024 edition), and an old scanner
# misses new advisories. Copying the toolchain out of rust:1-bookworm -- the same
# base the desktop-builder stage below uses -- gives a pinned, reproducibly
# distributed rustc with no pipe-to-shell install inside the stage that produces
# the attestation. Stronger provenance than a TLS-TOFU rustup.rs fetch, and
# consistent with the other Rust stages. (CI-003)
COPY --from=rust:1-bookworm /usr/local/rustup /usr/local/rustup
COPY --from=rust:1-bookworm /usr/local/cargo /usr/local/cargo
ENV RUSTUP_HOME=/usr/local/rustup
ENV CARGO_HOME=/usr/local/cargo
ENV PATH="/go/bin:/usr/local/cargo/bin:${PATH}"

# Node + python3 for the gate scripts (Debian's node is enough for npm audit; no
# front-end build happens in this stage). curl is gone -- only the removed
# rustup installer used it.
RUN apt-get update && apt-get install -y --no-install-recommends \
        nodejs npm python3 pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*

# Scanners installed in their own layer so they cache until this line changes.
RUN go install golang.org/x/vuln/cmd/govulncheck@latest \
    && cargo install cargo-audit --locked

COPY server/ ./server/
COPY agent/ ./agent/
COPY web/ ./web/
COPY helper/ ./helper/
COPY scripts/ ./scripts/
COPY .govulncheck-allow ./

# exit 1 = real block (fail the build); exit 2 = infra/offline (posture written
# unattested, continue). Guarantee a posture file exists regardless.
RUN rc=0; bash scripts/dep-scan.sh --posture-out /posture-build.json || rc=$?; \
    if [ "$rc" = "1" ]; then echo "[ERROR] dep-scan blocked the build"; exit 1; fi; \
    test -f /posture-build.json || \
      printf '%s' '{"attested":false,"generated_at":0,"substrate":{},"scans":[],"exceptions":[]}' > /posture-build.json

# Stage 1: Build server binary
FROM golang:1.26-alpine AS server-builder

ARG BUILD_VERSION=dev

WORKDIR /app

# Copy go.mod and go.sum
COPY server/go.mod server/go.sum ./
RUN go mod download

# Copy server contents to /app
COPY server/ ./

# Embed the dashboard build — server/internal/webui picks this up via go:embed
COPY --from=web-builder /web/dist ./internal/webui/dist

# Embed the attested supply-chain posture over the committed attested:false stub
# (services/posture.go picks this up via go:embed).
COPY --from=posture-builder /posture-build.json ./internal/services/posture-build.json

# Build server with version injection
RUN echo "Building server version: $BUILD_VERSION" && \
    CGO_ENABLED=0 go build \
        -ldflags "-X github.com/Fimeg/RedFlag/server/internal/version.AgentVersion=$BUILD_VERSION" \
        -o redflag-server ./cmd/server/

# Stage 2: Build agent binaries for all platforms
FROM golang:1.26-alpine AS agent-builder

ARG BUILD_VERSION=dev

WORKDIR /build

# Copy agent source code
COPY agent/ ./

# Build for Linux amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
        -ldflags "-X github.com/Fimeg/RedFlag/agent/internal/version.Version=$BUILD_VERSION" \
        -o binaries/linux-amd64/redflag-agent ./cmd/agent

# Build for Linux arm64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
        -ldflags "-X github.com/Fimeg/RedFlag/agent/internal/version.Version=$BUILD_VERSION" \
        -o binaries/linux-arm64/redflag-agent ./cmd/agent

# Build for macOS amd64
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
        -ldflags "-X github.com/Fimeg/RedFlag/agent/internal/version.Version=$BUILD_VERSION" \
        -o binaries/darwin-amd64/redflag-agent ./cmd/agent

# Build for macOS arm64
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
        -ldflags "-X github.com/Fimeg/RedFlag/agent/internal/version.Version=$BUILD_VERSION" \
        -o binaries/darwin-arm64/redflag-agent ./cmd/agent

# Build for Windows amd64
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
        -ldflags "-X github.com/Fimeg/RedFlag/agent/internal/version.Version=$BUILD_VERSION" \
        -o binaries/windows-amd64/redflag-agent.exe ./cmd/agent

# Build for Windows arm64
RUN CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build \
        -ldflags "-X github.com/Fimeg/RedFlag/agent/internal/version.Version=$BUILD_VERSION" \
        -o binaries/windows-arm64/redflag-agent.exe ./cmd/agent

# Stage 2b: Build the Rust capability-gate executor (redflag-helper)
FROM rust:1-alpine AS helper-builder

# The Rust crates carry 3-part Cargo versions and a release identity has four,
# so build.rs takes it from here. "dev" is the bare-docker-build default and is
# not a release, so it is left uninjected and the binary names the crate.
ARG BUILD_VERSION=dev

WORKDIR /helper

# musl-dev provides the C runtime cargo links against on alpine.
RUN apk --no-cache add musl-dev

# Copy helper crate and build a static release binary. ed25519-dalek/sha2/serde
# are pure Rust, so the default x86_64-unknown-linux-musl target builds clean.
COPY helper/ ./
RUN if [ "$BUILD_VERSION" != "dev" ]; then export REDFLAG_RELEASE_VERSION="$BUILD_VERSION"; fi && \
    cargo build --release && \
    mkdir -p /out/helper-linux-amd64 && \
    cp target/release/redflag-helper /out/helper-linux-amd64/redflag-helper

# Cross-build for arm64 (INSTALL-003: Pixel 3 / SBC fleet). The crate is pure
# Rust and the musl target ships self-contained crt objects, so rust-lld links
# it without an aarch64 C cross-toolchain. Static musl runs on glibc distros
# (Arch Linux ARM) the same as the amd64 helper does everywhere else.
RUN if [ "$BUILD_VERSION" != "dev" ]; then export REDFLAG_RELEASE_VERSION="$BUILD_VERSION"; fi && \
    rustup target add aarch64-unknown-linux-musl && \
    CARGO_TARGET_AARCH64_UNKNOWN_LINUX_MUSL_LINKER=rust-lld \
    cargo build --release --target aarch64-unknown-linux-musl && \
    mkdir -p /out/helper-linux-arm64 && \
    cp target/aarch64-unknown-linux-musl/release/redflag-helper /out/helper-linux-arm64/redflag-helper

# Stage 2c: Build the native Qt/QML Desktop.
FROM rust:1-bookworm AS desktop-builder

ARG BUILD_VERSION=dev

RUN apt-get update && apt-get install -y --no-install-recommends \
    qt6-base-dev \
    qt6-declarative-dev \
    qt6-declarative-dev-tools \
    libgl1-mesa-dev \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# The QML module, bridge, and visual assets are embedded by CXX-Qt.
COPY desktop/ desktop/

RUN cd desktop && \
    if [ "$BUILD_VERSION" != "dev" ]; then export REDFLAG_RELEASE_VERSION="$BUILD_VERSION"; fi && \
    cargo build --release --locked && \
    mkdir -p /out && \
    cp target/release/redflag-desktop /out/redflag-desktop

# Stage 2d: Fetch the prebuilt, signed Windows Desktop from the newest
# release and verify it against the release manifest hash. The Windows desktop
# needs a matching native Qt and MSVC SDK, which has no business in a
# from-source server build. A native release runner will build it once; this
# stage fetches that signed result. Optional component: anything short of a
# hash mismatch degrades to "no Windows Desktop". See scripts/fetch-desktop-windows.sh.
FROM alpine:3.21 AS desktop-windows-fetcher

ARG BUILD_VERSION=dev
ARG DESKTOP_RELEASE_REPO_API=https://forge.caseytunturi.com/api/v1/repos/Fimeg/RedFlag
ENV DESKTOP_RELEASE_REPO_API=${DESKTOP_RELEASE_REPO_API}

RUN apk --no-cache add curl jq unzip

COPY scripts/fetch-desktop-windows.sh /usr/local/bin/fetch-desktop-windows.sh

# BUILD_VERSION sits in this layer's cache key so bumping the version re-fetches
# rather than reusing a stale Desktop from an earlier build.
RUN echo "fetch Windows Desktop for build $BUILD_VERSION" && \
    sh /usr/local/bin/fetch-desktop-windows.sh amd64 /out

# Stage 3: Final image with server and all agent binaries
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata bash
WORKDIR /app

# Copy server binary
COPY --from=server-builder /app/redflag-server .
COPY --from=server-builder /app/internal/database ./internal/database

# Copy all agent binaries
COPY --from=agent-builder /build/binaries ./binaries

# Copy the signed-at-startup capability-gate executor alongside the agent
# binaries so BuildOrchestrator.SignExistingBinary can find it at
# binaries/helper-linux-<arch>/redflag-helper.
COPY --from=helper-builder /out/helper-linux-amd64 ./binaries/helper-linux-amd64
COPY --from=helper-builder /out/helper-linux-arm64 ./binaries/helper-linux-arm64

# Copy native RedFlag Desktop.
COPY --from=desktop-builder /out/redflag-desktop ./binaries/linux-amd64/redflag-desktop

# Stage in Windows Desktop IF the newest release carried one (fetched +
# hash-verified by the desktop-windows-fetcher stage). When absent — no release
# yet, offline build, or a release with no desktop — the server simply serves no
# Windows Desktop and the installer 404-skips it. The native Windows compile never
# happens here; only the verified artifact lands.
COPY --from=desktop-windows-fetcher /out /opt/winfetch
RUN if [ -f /opt/winfetch/redflag-desktop.exe ]; then \
        mkdir -p ./binaries/windows-amd64 && \
        mv /opt/winfetch/redflag-desktop.exe ./binaries/windows-amd64/redflag-desktop.exe && \
        echo "[INFO] [build] [desktop] staged Windows Desktop into image"; \
    else \
        echo "[INFO] [build] [desktop] no Windows Desktop staged (none in latest release)"; \
    fi; \
    rm -rf /opt/winfetch

# Copy and setup entrypoint script
COPY server/docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

EXPOSE 8080

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./redflag-server"]
