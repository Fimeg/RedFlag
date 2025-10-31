# Stage 1: Build server binary
FROM golang:1.23-alpine AS server-builder

WORKDIR /app
COPY aggregator-server/go.mod aggregator-server/go.sum ./
RUN go mod download

COPY aggregator-server/ .
RUN CGO_ENABLED=0 go build -o redflag-server cmd/server/main.go

# Stage 2: Build agent binaries for all platforms
FROM golang:1.23-alpine AS agent-builder

WORKDIR /build
# Copy agent source code
COPY aggregator-agent/ ./

# Build for Linux amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o binaries/linux-amd64/redflag-agent cmd/agent/main.go

# Build for Linux arm64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o binaries/linux-arm64/redflag-agent cmd/agent/main.go

# Build for Windows amd64
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o binaries/windows-amd64/redflag-agent.exe cmd/agent/main.go

# Build for Windows arm64
RUN CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o binaries/windows-arm64/redflag-agent.exe cmd/agent/main.go

# Stage 3: Final image with server and all agent binaries
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

# Copy server binary
COPY --from=server-builder /app/redflag-server .
COPY --from=server-builder /app/internal/database ./internal/database

# Copy all agent binaries
COPY --from=agent-builder /build/binaries ./binaries

EXPOSE 8080

CMD ["./redflag-server"]