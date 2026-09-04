# Docker Scanner

**Docker image scanning for container-based agents.**

---

## Component Details

| Property | Value |
|----------|-------|
| Method | Read `/var/run/docker.sock` via Docker SDK |
| Platform | Linux (Docker agents) |
| Execution time | ~5 seconds per scan |
| Output format | `[]client.UpdateReportItem` with full metadata in `Metadata` map |
| Failure modes | Docker daemon unavailable, socket permission denied, registry rate-limited |

---

## Implementation

**File:** `agent/internal/orchestrator/docker_scanner.go`

The Docker scanner connects directly to the local Docker daemon via the Docker SDK, lists all containers, inspects each image, then queries the remote registry (Docker Hub or custom) to compare digests.

```go
// agent/internal/orchestrator/docker_scanner.go
func (s *DockerScanner) Scan() ([]client.UpdateReportItem, error) {
    containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
    // ... inspect each image, compare local vs remote digest ...
    items = append(items, client.UpdateReportItem{
        PackageType:      "docker_image",
        PackageName:      imageName,
        CurrentVersion:   localShortDigest,
        AvailableVersion: remoteShortDigest,
        Severity:         severity,
        RepositorySource: baseImage,
        Metadata: map[string]interface{}{
            "has_update":       hasUpdate,
            "image_id":         localShortDigest,
            "latest_image_id":  remoteShortDigest,
            // ... container info, labels, etc.
        },
    })
    return items, nil
}
```

### Registry Client

The `RegistryClient` within `docker_scanner.go` handles:
- Docker Hub token authentication (`auth.docker.io`)
- Registry API v2 manifest fetch via `Docker-Content-Digest` header
- 5-minute TTL cache to avoid rate limits
- Custom registry support (gcr.io, etc.) via domain detection in `parseImageName()`

---

## Integration Points

### 1. Orchestrator Registration

Registered in `agent/internal/agent/loop.go` as a direct `orchestrator.Scanner` implementation:

```go
dockerScanner, _ := orchestrator.NewDockerScanner()
scanOrchestrator.RegisterScanner("docker", dockerScanner, dockerCB, ...)
```

### 2. Command Dispatch

The `HandleScanDocker` handler in `agent/internal/handlers/scan.go` runs the scan once through the orchestrator and reads results from `result.Updates[]` — no double-scanning.

### 3. Server-Side Storage

Scan results are reported via `client.ReportDockerImages()` to `POST /api/v1/agents/:id/docker-images`. The server stores them in `docker_images` table via `server/internal/database/queries/docker.go` (`CreateDockerEventsBatch`, `GetDockerImages`, etc.).

---

## Data Flow

```
Agent Poll → Server creates "scan_docker" command
    ↓
Agent receives command → orchestrator.ScanSingle("docker")
    ↓
DockerScanner.Scan() connects to local Docker daemon
    ↓
List containers, inspect images, check registry digests
    ↓
Return UpdateReportItems with full metadata
    ↓
Handler reports results to server API endpoint
    ↓
Server stores in docker_images table
```

---

## Footer: Assumptions & Connections

**Assumption:** Docker daemon is available on the agent host (`/var/run/docker.sock`). Registry access requires outbound internet (or mirror configuration).

**Connection:** Scanner registration (`agent/internal/agent/loop.go`) wires Docker into the orchestrator alongside APT, DNF, Windows, and Winget scanners — all now implement `orchestrator.Scanner` directly (no wrapper layer).

**Connection:** Registry client (`orchestrator/docker_scanner.go`) uses ETHOS `[TAG] [system] [component]` logging for registry failures.

**Connection:** Server-side `docker_images` table (`server/internal/database/queries/docker.go`) stores reported scan results for dashboard display.

---

*Last reviewed: 2026-05-27*
