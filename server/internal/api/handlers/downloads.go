package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/config"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services"
	serverVersion "github.com/Fimeg/RedFlag/server/internal/version"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// DownloadHandler handles agent binary downloads
type DownloadHandler struct {
	agentDir               string
	config                 *config.Config
	installTemplateService *services.InstallTemplateService
	packageQueries         *queries.PackageQueries
	signingService         *services.SigningService // ISSUE-002: For public key access
}

// NewDownloadHandler creates a new download handler
// ISSUE-002: Added signingService parameter for install script signature verification
func NewDownloadHandler(agentDir string, cfg *config.Config, packageQueries *queries.PackageQueries, signingService *services.SigningService) *DownloadHandler {
	return &DownloadHandler{
		agentDir:               agentDir,
		config:                 cfg,
		installTemplateService: services.NewInstallTemplateService(),
		packageQueries:         packageQueries,
		signingService:         signingService,
	}
}

// getServerURL determines the server URL with proper protocol detection.
// Delegates to resolveServerURL which checks the configured public URL first.
func (h *DownloadHandler) getServerURL(c *gin.Context) string {
	return resolveServerURL(c, h.config, "downloads")
}

// DownloadPackageArtifact downloads a package from upstream and serves it with SHA256
// This is used by the approval flow to compute expected hashes before agent installation
func (h *DownloadHandler) DownloadPackageArtifact(c *gin.Context) {
	ecosystem := c.Query("ecosystem")
	packageName := c.Param("package_name")
	version := c.Param("version")

	if ecosystem == "" || packageName == "" || version == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Missing required parameters: ecosystem, package_name, version",
		})
		return
	}

	// Map RedFlag package types to the single canonical registry the server can
	// fetch directly. OS package managers (dnf/apt) are intentionally absent: their
	// artifacts come from each agent's own GPG-signed repos, which the server
	// cannot reach. Those hashes are resolved and pinned agent-side at dry-run
	// (see ReportDependencies / installer.ResolveArtifactSHA256), so the server
	// never needs to serve them — answering here would be a placeholder lie.
	registryURLs := map[string]string{
		"npm":    "https://registry.npmjs.org",
		"pypi":   "https://pypi.org",
		"docker": "https://registry.hub.docker.com",
	}

	registryURL, ok := registryURLs[ecosystem]
	if !ok {
		log.Printf("[INFO] [server] [downloads] artifact_not_server_fetchable ecosystem=%s pkg=%s reason=agent_local_repo",
			ecosystem, packageName)
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": fmt.Sprintf("Artifacts for ecosystem %q are resolved agent-side from signed repo metadata, not served by the server", ecosystem),
		})
		return
	}

	// Construct download URL based on ecosystem
	var downloadURL string
	switch ecosystem {
	case "npm":
		// npm: https://registry.npmjs.org/<pkg>/-/<pkg>-<version>.tgz
		downloadURL = fmt.Sprintf("%s/%s/-/%s-%s.tgz", registryURL, packageName, packageName, version)
	case "pypi":
		// PyPI: https://files.pythonhosted.org/packages/<digest>/<prefix>/<filename>
		// We need to first fetch the package info to get the actual download URL
		infoURL := fmt.Sprintf("%s/pypi/%s/%s/json", registryURL, packageName, version)
		var body struct {
			URLs []struct {
				URL string `json:"url"`
			} `json:"urls"`
		}
		resp, err := http.Get(infoURL)
		if err != nil {
			log.Printf("[WARNING] [downloads] pypi_fetch_failed pkg=%s version=%s error=%v", packageName, version, err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch PyPI metadata"})
			return
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			log.Printf("[WARNING] [downloads] pypi_decode_failed pkg=%s error=%v", packageName, err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to decode PyPI response"})
			return
		}
		if len(body.URLs) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Package version not found"})
			return
		}
		downloadURL = body.URLs[0].URL
	case "docker":
		// Docker: redirect to Docker Hub
		redirURL := fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, packageName, version)
		resp, err := http.Get(redirURL)
		if err != nil {
			log.Printf("[WARNING] [downloads] docker_fetch_failed pkg=%s version=%s error=%v", packageName, version, err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch Docker Hub"})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusTemporaryRedirect && resp.StatusCode != http.StatusPermanentRedirect {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "Docker Hub redirect not supported"})
			return
		}
		// Parse redirect location
		location := resp.Header.Get("Location")
		downloadURL = location
	default:
		// Unreachable: the registry map above only holds ecosystems handled here.
		// Kept as a defensive fail-closed rather than fabricating a download URL.
		log.Printf("[ERROR] [server] [downloads] artifact_unhandled_ecosystem ecosystem=%s pkg=%s", ecosystem, packageName)
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": fmt.Sprintf("No artifact resolver for ecosystem %q", ecosystem),
		})
		return
	}

	// Download the artifact
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		log.Printf("[ERROR] [downloads] request_failed pkg=%s url=%s error=%v", packageName, downloadURL, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to create download request"})
		return
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] [downloads] download_failed pkg=%s url=%s error=%v", packageName, downloadURL, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to download package from upstream"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARNING] [downloads] download_status_failed pkg=%s status=%d", packageName, resp.StatusCode)
		c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("Failed to download package (status: %d)", resp.StatusCode)})
		return
	}

	// Read and return the artifact
	contentLength := resp.ContentLength
	if contentLength < 0 {
		contentLength = -1
	}

	c.Header("Content-Length", strconv.FormatInt(contentLength, 10))
	c.Header("Content-Type", resp.Header.Get("Content-Type"))

	// Stream the body
	if c.Request.Method == "HEAD" {
		c.Status(http.StatusOK)
		return
	}

	// Compute SHA256 while streaming
	sha := sha256.New()
	writer := io.MultiWriter(sha, c.Writer)
	io.Copy(writer, resp.Body)

	shaHex := hex.EncodeToString(sha.Sum(nil))
	c.Header("X-Content-SHA256", shaHex)
}

// DownloadAgent serves agent binaries for different platforms
func (h *DownloadHandler) DownloadAgent(c *gin.Context) {
	platform := c.Param("platform")
	version := c.Query("version") // Optional version parameter for signed binaries
	if version == "latest" {
		version = serverVersion.AgentVersion
	}

	// Validate platform to prevent directory traversal
	validPlatforms := map[string]bool{
		"linux-amd64":   true,
		"linux-arm64":   true,
		"windows-amd64": true,
		"windows-arm64": true,
	}

	if !validPlatforms[platform] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or unsupported platform"})
		return
	}

	var agentPath string
	var signedPackage *models.AgentUpdatePackage // ISSUE-002: Track for signature header

	// Try to serve signed package first if version is specified (F-E1-1 fix)
	if version != "" {
		parts := strings.SplitN(platform, "-", 2)
		if len(parts) == 2 {
			var err error
			signedPackage, err = h.packageQueries.GetSignedPackage(version, parts[0], parts[1])
			if err == nil && signedPackage != nil {
				// Path traversal guard for signed packages
				absSignedPath, absErr := filepath.Abs(signedPackage.BinaryPath)
				allowedBase, _ := filepath.Abs(h.config.BinaryStoragePath)
				if allowedBase == "" {
					allowedBase, _ = filepath.Abs("./binaries")
				}
				if absErr == nil && (strings.HasPrefix(absSignedPath, allowedBase+string(filepath.Separator)) || absSignedPath == allowedBase) {
					if _, statErr := os.Stat(absSignedPath); statErr == nil {
						agentPath = absSignedPath
						log.Printf("[INFO] [server] [downloads] serving_signed_package version=%s platform=%s", version, platform)
					}
				} else if absErr == nil {
					log.Printf("[ERROR] [server] [downloads] path_traversal_attempt_signed path=%s allowed=%s", absSignedPath, allowedBase)
				}
			}
		}
	}

	// No signed package found — serve nothing. Signing is mandatory.
	if agentPath == "" {
		log.Printf("[WARNING] [server] [downloads] no_signed_package version=%s platform=%s", version, platform)
		c.JSON(http.StatusNotFound, gin.H{
			"error":    "No signed binary available",
			"platform": platform,
			"version":  version,
		})
		return
	}

	// Check if file exists and is not empty
	info, err := os.Stat(agentPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":    "Agent binary not found",
			"platform": platform,
			"version":  version,
		})
		return
	}
	if info.Size() == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":    "Agent binary not found (empty file)",
			"platform": platform,
			"version":  version,
		})
		return
	}

	// Compute SHA256 checksum and set headers
	if checksum, err := computeFileSHA256(agentPath); err == nil {
		c.Header("X-Content-SHA256", checksum)
	}
	c.Header("X-Content-Length", strconv.FormatInt(info.Size(), 10))

	// ISSUE-002: Include signature header for signed packages
	if signedPackage.Signature != "" {
		c.Header("X-Content-Signature", signedPackage.Signature)
	}

	// Handle both GET and HEAD requests
	if c.Request.Method == "HEAD" {
		c.Status(http.StatusOK)
		return
	}

	c.File(agentPath)
}

// DownloadHelper serves the privileged capability-gate executor (redflag-helper).
// It is a first-class signed artifact: stored as a signed package under
// platform "helper-linux", listed in the release manifest, and verified by the
// installer against both the signed manifest and its per-binary Ed25519
// signature — the same cold-start trust the agent binary gets. The helper runs
// as root via systemd-run, so install-time tamper-evidence is mandatory.
func (h *DownloadHandler) DownloadHelper(c *gin.Context) {
	arch := c.Param("arch")
	version := c.Query("version")
	if version == "" || version == "latest" {
		version = serverVersion.AgentVersion
	}

	validArch := map[string]bool{"amd64": true, "arm64": true}
	if !validArch[arch] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or unsupported architecture"})
		return
	}

	signedPackage, err := h.packageQueries.GetSignedPackage(version, "helper-linux", arch)
	if err != nil || signedPackage == nil {
		log.Printf("[WARNING] [server] [downloads] no_signed_helper version=%s arch=%s", version, arch)
		c.JSON(http.StatusNotFound, gin.H{"error": "No signed helper available", "arch": arch, "version": version})
		return
	}

	// Path traversal guard — identical to the agent-binary path.
	absPath, absErr := filepath.Abs(signedPackage.BinaryPath)
	allowedBase, _ := filepath.Abs(h.config.BinaryStoragePath)
	if allowedBase == "" {
		allowedBase, _ = filepath.Abs("./binaries")
	}
	if absErr != nil || !(strings.HasPrefix(absPath, allowedBase+string(filepath.Separator)) || absPath == allowedBase) {
		log.Printf("[ERROR] [server] [downloads] path_traversal_attempt_helper path=%s allowed=%s", absPath, allowedBase)
		c.JSON(http.StatusNotFound, gin.H{"error": "Helper binary not found"})
		return
	}

	info, err := os.Stat(absPath)
	if err != nil || info.Size() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Helper binary not found", "arch": arch, "version": version})
		return
	}

	if checksum, csErr := computeFileSHA256(absPath); csErr == nil {
		c.Header("X-Content-SHA256", checksum)
	}
	c.Header("X-Content-Length", strconv.FormatInt(info.Size(), 10))
	if signedPackage.Signature != "" {
		c.Header("X-Content-Signature", signedPackage.Signature)
	}

	if c.Request.Method == "HEAD" {
		c.Status(http.StatusOK)
		return
	}
	c.File(absPath)
}

// DownloadDesktop serves the native Qt/QML local-machine operations console.
// The desktop binary is optional — if not built for a given platform/arch,
// returns 404 gracefully so the installer can skip it.
func (h *DownloadHandler) DownloadDesktop(c *gin.Context) {
	platform := c.Param("platform")
	arch := c.Param("arch")
	version := c.Query("version")
	if version == "" || version == "latest" {
		version = serverVersion.AgentVersion
	}

	validPlatform := map[string]bool{"linux": true, "windows": true}
	validArch := map[string]bool{"amd64": true, "arm64": true}
	if !validPlatform[platform] || !validArch[arch] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or unsupported platform/architecture"})
		return
	}

	pkgName := "desktop-" + platform
	signedPackage, err := h.packageQueries.GetSignedPackage(version, pkgName, arch)
	if err != nil || signedPackage == nil {
		// Desktop binary is optional — 404 lets the installer skip gracefully.
		c.JSON(http.StatusNotFound, gin.H{"error": "No desktop binary available", "arch": arch, "version": version})
		return
	}

	absPath, absErr := filepath.Abs(signedPackage.BinaryPath)
	allowedBase, _ := filepath.Abs(h.config.BinaryStoragePath)
	if allowedBase == "" {
		allowedBase, _ = filepath.Abs("./binaries")
	}
	if absErr != nil || !(strings.HasPrefix(absPath, allowedBase+string(filepath.Separator)) || absPath == allowedBase) {
		log.Printf("[ERROR] [server] [downloads] path_traversal_attempt_desktop path=%s allowed=%s", absPath, allowedBase)
		c.JSON(http.StatusNotFound, gin.H{"error": "Desktop binary not found"})
		return
	}

	info, err := os.Stat(absPath)
	if err != nil || info.Size() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Desktop binary not found", "arch": arch, "version": version})
		return
	}

	if checksum, csErr := computeFileSHA256(absPath); csErr == nil {
		c.Header("X-Content-SHA256", checksum)
	}
	c.Header("X-Content-Length", strconv.FormatInt(info.Size(), 10))
	if signedPackage.Signature != "" {
		c.Header("X-Content-Signature", signedPackage.Signature)
	}

	if c.Request.Method == "HEAD" {
		c.Status(http.StatusOK)
		return
	}
	c.File(absPath)
}

// DownloadManifest serves the signed release manifest — the cold-start trust
// root. It lists the expected SHA-256 of every released binary for the given
// version (defaults to latest) and is signed with the server's Ed25519 key.
// The signature is over the verbatim response body and travels in
// X-Content-Signature; the key fingerprint is in X-Key-Id. The installer
// verifies the signature, then checks the downloaded binary against the matching
// entry before executing it.
func (h *DownloadHandler) DownloadManifest(c *gin.Context) {
	version := c.Query("version")
	if version == "" || version == "latest" {
		version = serverVersion.AgentVersion
	}

	if h.signingService == nil || !h.signingService.IsEnabled() {
		log.Printf("[ERROR] [server] [downloads] manifest_unavailable reason=signing_disabled version=%s", version)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "release manifest unavailable: signing disabled"})
		return
	}

	manifest := h.buildReleaseManifest(version)
	if len(manifest.Artifacts) == 0 {
		log.Printf("[WARN] [server] [downloads] manifest_empty version=%s", version)
		c.JSON(http.StatusNotFound, gin.H{"error": "no signed binaries available for version", "version": version})
		return
	}

	// The signed bytes are the verbatim body. json.Marshal is deterministic here
	// (no map fields; Artifacts built in fixed platform order), so the installer
	// verifies the signature over exactly what it received.
	body, err := json.Marshal(manifest)
	if err != nil {
		log.Printf("[ERROR] [server] [downloads] manifest_marshal_failed version=%s error=%v", version, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build manifest"})
		return
	}

	sig, err := h.signingService.SignBytes(body)
	if err != nil {
		log.Printf("[ERROR] [server] [downloads] manifest_sign_failed version=%s error=%v", version, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign manifest"})
		return
	}

	c.Header("X-Content-Signature", sig)
	c.Header("X-Key-Id", manifest.KeyID)
	if c.Request.Method == "HEAD" {
		c.Status(http.StatusOK)
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}

// buildReleaseManifest assembles the manifest from the signed-package records.
// Components are pulled from the authoritative catalog (componentCatalog); artifacts
// are resolved from signed-package DB rows. Platforms are walked in a fixed order
// so the marshalled bytes are stable. A platform with no signed package (or no
// resolvable checksum) is omitted rather than fabricated — the manifest never lies
// about what it can attest.
func (h *DownloadHandler) buildReleaseManifest(version string) services.ReleaseManifest {
	manifest := services.ReleaseManifest{
		Version:     version,
		GeneratedAt: time.Now().UTC().Unix(),
		KeyID:       h.signingService.GetCurrentKeyID(),
		Components:  services.ComponentCatalog(),
		SupplyChain: services.EmbeddedPosture(),
	}

	platforms := []struct{ platform, arch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"windows", "amd64"},
		{"windows", "arm64"},
		{"helper-linux", "amd64"},
		{"helper-linux", "arm64"},
		{"desktop-linux", "amd64"},
	}

	for _, p := range platforms {
		pkg, err := h.packageQueries.GetSignedPackage(version, p.platform, p.arch)
		if err != nil || pkg == nil {
			continue
		}
		checksum := pkg.Checksum
		if checksum == "" && pkg.BinaryPath != "" {
			if cs, csErr := computeFileSHA256(pkg.BinaryPath); csErr == nil {
				checksum = cs
			}
		}
		if checksum == "" {
			log.Printf("[WARN] [server] [downloads] manifest_skip_no_checksum version=%s platform=%s-%s", version, p.platform, p.arch)
			continue
		}
		filename := "redflag-agent"
		switch {
		case strings.HasPrefix(p.platform, "helper"):
			filename = "redflag-helper"
		case strings.HasPrefix(p.platform, "desktop"):
			filename = "redflag-desktop"
		case p.platform == "windows":
			filename += ".exe"
		}
		manifest.Artifacts = append(manifest.Artifacts, services.ManifestArtifact{
			Platform:     p.platform,
			Architecture: p.arch,
			Filename:     filename,
			SHA256:       checksum,
			Size:         pkg.FileSize,
		})
	}
	return manifest
}

// DownloadUpdatePackage serves signed agent update packages
func (h *DownloadHandler) DownloadUpdatePackage(c *gin.Context) {
	packageID := c.Param("package_id")

	// Validate package ID format (UUID)
	if len(packageID) != 36 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid package ID format"})
		return
	}

	parsedPackageID, err := uuid.FromString(packageID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid package ID format"})
		return
	}

	// Fetch package from database
	pkg, err := h.packageQueries.GetSignedPackageByID(parsedPackageID)
	if err != nil {
		if err.Error() == "update package not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error":      "Package not found",
				"package_id": packageID,
			})
			return
		}

		log.Printf("[ERROR] [server] [downloads] package_fetch_failed package_id=%s error=%v", packageID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "Failed to retrieve package",
			"package_id": packageID,
		})
		return
	}

	// Check if binary_path is populated
	if pkg.BinaryPath == "" {
		log.Printf("[WARNING] [server] [downloads] package_binary_path_empty package_id=%s", packageID)
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":      "Package binary not yet available",
			"package_id": packageID,
		})
		return
	}

	// Resolve and sanitize binary path (defense in depth — prevent path traversal)
	absPath, err := filepath.Abs(pkg.BinaryPath)
	if err != nil {
		log.Printf("[ERROR] [server] [downloads] path_resolve_failed package_id=%s path=%s error=%v", packageID, pkg.BinaryPath, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid package path"})
		return
	}

	allowedDir, _ := filepath.Abs(h.config.BinaryStoragePath)
	if allowedDir == "" {
		allowedDir, _ = filepath.Abs("./binaries")
	}
	if !strings.HasPrefix(absPath, allowedDir+string(filepath.Separator)) && absPath != allowedDir {
		log.Printf("[ERROR] [server] [downloads] path_traversal_attempt package_id=%s resolved_path=%s allowed_dir=%s", packageID, absPath, allowedDir)
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	// Verify file exists on disk
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		log.Printf("[ERROR] [server] [downloads] package_file_not_found package_id=%s path=%s", packageID, absPath)
		c.JSON(http.StatusNotFound, gin.H{
			"error":      "Package file not found on disk",
			"package_id": packageID,
		})
		return
	}

	// Set appropriate headers
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(absPath)))
	c.Header("X-Package-Version", pkg.Version)
	c.Header("X-Package-Platform", pkg.Platform)
	c.Header("X-Package-Architecture", pkg.Architecture)

	if pkg.Signature != "" {
		c.Header("X-Package-Signature", pkg.Signature)
	}

	if pkg.Checksum != "" {
		c.Header("X-Package-Checksum", pkg.Checksum)
	}

	// Serve the file
	log.Printf("[INFO] [server] [downloads] package_download_served package_id=%s version=%s platform=%s", packageID, pkg.Version, pkg.Platform)
	c.File(absPath)
}

// InstallScript serves the installation script
func (h *DownloadHandler) InstallScript(c *gin.Context) {
	platform := c.Param("platform")

	// Validate platform
	validPlatforms := map[string]bool{
		"linux":   true,
		"windows": true,
	}

	if !validPlatforms[platform] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or unsupported platform"})
		return
	}

	serverURL := h.getServerURL(c)
	scriptContent := h.generateInstallScript(c, platform, serverURL)

	// Windows PowerShell 5.1's parser mishandles here-strings in LF-only
	// scripts — the embedded config template parse-errors with a cascade of
	// "Unexpected token ':'". Serve the Windows script with CRLF (correct for
	// a .ps1 anyway; parses in both 5.1 and 7). Linux stays LF for bash.
	if platform == "windows" {
		scriptContent = strings.ReplaceAll(scriptContent, "\r\n", "\n")
		scriptContent = strings.ReplaceAll(scriptContent, "\n", "\r\n")
	}

	// charset=utf-8 so PowerShell's irm decodes the body as UTF-8 instead of
	// Latin-1 (otherwise the ✓/⚠/— glyphs come through as mojibake).
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, scriptContent)
}

// parseAgentID extracts agent ID from header → path → query with security priority
func parseAgentID(c *gin.Context) string {
	// 1. Header → Secure (preferred)
	if agentID := c.GetHeader("X-Agent-ID"); agentID != "" {
		if _, err := uuid.FromString(agentID); err == nil {
			log.Printf("[DEBUG] Parsed agent ID from header: %s", agentID)
			return agentID
		}
		log.Printf("[DEBUG] Invalid UUID in header: %s", agentID)
	}

	// 2. Path parameter → Legacy compatible
	if agentID := c.Param("agent_id"); agentID != "" {
		if _, err := uuid.FromString(agentID); err == nil {
			log.Printf("[DEBUG] Parsed agent ID from path: %s", agentID)
			return agentID
		}
		log.Printf("[DEBUG] Invalid UUID in path: %s", agentID)
	}

	// 3. Query parameter → Fallback
	if agentID := c.Query("agent_id"); agentID != "" {
		if _, err := uuid.FromString(agentID); err == nil {
			log.Printf("[DEBUG] Parsed agent ID from query: %s", agentID)
			return agentID
		}
		log.Printf("[DEBUG] Invalid UUID in query: %s", agentID)
	}

	// Return placeholder for fresh installs
	log.Printf("[DEBUG] No valid agent ID found, using placeholder")
	return "<AGENT_ID>"
}

// HandleConfigDownload serves agent configuration templates with updated schema
// The install script injects the agent's actual credentials locally after download
func (h *DownloadHandler) HandleConfigDownload(c *gin.Context) {
	agentIDParam := c.Param("agent_id")

	// Validate UUID format
	parsedAgentID, err := uuid.FromString(agentIDParam)
	if err != nil {
		log.Printf("Invalid agent ID format for config download: %s, error: %v", agentIDParam, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid agent ID format",
		})
		return
	}

	// Log for security monitoring
	log.Printf("Config template download requested - agent_id: %s, remote_addr: %s",
		parsedAgentID.String(), c.ClientIP())

	// Get server URL for config
	serverURL := h.getServerURL(c)

	// Build config template with schema only (no sensitive credentials)
	// Credentials are preserved locally by the install script
	configTemplate := map[string]interface{}{
		"version":       5, // Current schema version (v5 as of 0.1.23+)
		"agent_version": "0.2.0.3",
		"server_url":    serverURL,

		// Placeholder credentials - will be replaced by install script
		"agent_id":           "00000000-0000-0000-0000-000000000000",
		"token":              "",
		"refresh_token":      "",
		"registration_token": "",
		"machine_id":         "",

		// Standard configuration with all subsystems
		"check_in_interval":     300,
		"rapid_polling_enabled": false,
		"rapid_polling_until":   "0001-01-01T00:00:00Z",

		"network": map[string]interface{}{
			"timeout":       30000000000,
			"retry_count":   3,
			"retry_delay":   5000000000,
			"max_idle_conn": 10,
		},

		"proxy": map[string]interface{}{
			"enabled": false,
		},

		"tls": map[string]interface{}{
			"enabled":              false,
			"insecure_skip_verify": false,
		},

		"logging": map[string]interface{}{
			"level":       "info",
			"max_size":    100,
			"max_backups": 3,
			"max_age":     28,
		},

		"subsystems": map[string]interface{}{
			"system": map[string]interface{}{
				"enabled": true,
				"timeout": 10000000000,
				"circuit_breaker": map[string]interface{}{
					"enabled":            true,
					"failure_threshold":  3,
					"failure_window":     600000000000,
					"open_duration":      1800000000000,
					"half_open_attempts": 2,
				},
			},
			"filesystem": map[string]interface{}{
				"enabled": true,
				"timeout": 10000000000,
				"circuit_breaker": map[string]interface{}{
					"enabled":            true,
					"failure_threshold":  3,
					"failure_window":     600000000000,
					"open_duration":      1800000000000,
					"half_open_attempts": 2,
				},
			},
			"network": map[string]interface{}{
				"enabled": true,
				"timeout": 30000000000,
				"circuit_breaker": map[string]interface{}{
					"enabled":            true,
					"failure_threshold":  3,
					"failure_window":     600000000000,
					"open_duration":      1800000000000,
					"half_open_attempts": 2,
				},
			},
			"processes": map[string]interface{}{
				"enabled": true,
				"timeout": 30000000000,
				"circuit_breaker": map[string]interface{}{
					"enabled":            true,
					"failure_threshold":  3,
					"failure_window":     600000000000,
					"open_duration":      1800000000000,
					"half_open_attempts": 2,
				},
			},
			"storage": map[string]interface{}{
				"enabled": true,
				"timeout": 10000000000,
				"circuit_breaker": map[string]interface{}{
					"enabled":            true,
					"failure_threshold":  3,
					"failure_window":     600000000000,
					"open_duration":      1800000000000,
					"half_open_attempts": 2,
				},
			},
		},

		"security": map[string]interface{}{
			"ed25519_verification": true,
			"nonce_validation":     true,
			"machine_id_binding":   true,
		},
	}

	// Return config template as JSON
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"redflag-config.json\""))
	c.JSON(http.StatusOK, configTemplate)
}

func (h *DownloadHandler) generateInstallScript(c *gin.Context, platform, baseURL string) string {
	// Parse agent ID with defense-in-depth priority
	agentIDParam := parseAgentID(c)

	// Extract registration token from header. Query-string tokens are rejected
	// (SEC-002): a token in the URL leaks to process lists, shell history, and
	// server access logs. The token value is never echoed back.
	registrationToken := c.GetHeader("X-Registration-Token")
	if registrationToken == "" {
		if c.Query("token") != "" {
			log.Printf("[WARNING] [server] [downloads] install_token_in_query_rejected remote=%s — token passed via URL, refusing (SEC-002)", c.ClientIP())
			return "# Error: passing the registration token in the URL is not supported (it leaks to logs and process lists).\n" +
				"# Use the header form instead:\n" +
				"#   curl -sfL -H \"X-Registration-Token: YOUR_TOKEN\" \"<server>/api/v1/install/<platform>\" | sudo bash\n"
		}
		return "# Error: registration token is required\n" +
			"# Pass it via header:\n" +
			"#   curl -sfL -H \"X-Registration-Token: YOUR_TOKEN\" \"<server>/api/v1/install/<platform>\" | sudo bash\n"
	}

	// Determine architecture: use ?arch= query param or default to amd64
	arch := c.DefaultQuery("arch", "amd64")
	validArchs := map[string]bool{"amd64": true, "arm64": true, "armv7": true}
	if !validArchs[arch] {
		arch = "amd64"
	}

	// Use template service to generate install scripts
	// Pass actual agent ID for upgrades, fallback placeholder for fresh installs
	// ISSUE-002: Include server public key for install-time signature verification
	script, err := h.installTemplateService.RenderInstallScriptFromBuild(
		agentIDParam,                    // Real agent ID or placeholder
		platform,                        // Platform (linux/windows)
		arch,                            // Architecture
		serverVersion.AgentVersion,      // Version
		baseURL,                         // Server base URL
		registrationToken,               // Registration token from query param
		h.signingService.GetPublicKey(), // Server public key for TOFU
	)
	if err != nil {
		return fmt.Sprintf("# Error generating install script: %v", err)
	}
	return script
}

// computeFileSHA256 returns the lowercase hex-encoded SHA256 hash of a file.
func computeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
