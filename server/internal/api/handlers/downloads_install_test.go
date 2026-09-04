package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Fimeg/RedFlag/server/internal/config"
	"github.com/Fimeg/RedFlag/server/internal/services"
	serverVersion "github.com/Fimeg/RedFlag/server/internal/version"
	"github.com/gin-gonic/gin"
)

// TestInstallScriptRejectsQueryToken locks in SEC-002: a registration token
// passed as a URL query parameter must be refused (it leaks to shell history,
// process lists, and access logs) and must never be echoed back.
func TestInstallScriptRejectsQueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	signingService, err := services.NewSigningService(hex.EncodeToString(privateKey))
	if err != nil {
		t.Fatalf("new signing service: %v", err)
	}

	handler := NewDownloadHandler("/tmp/redflag-test", &config.Config{}, nil, signingService)
	router := gin.New()
	router.GET("/api/v1/install/:platform", handler.InstallScript)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/install/linux?token=leaked-secret-token", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if strings.Contains(body, "leaked-secret-token") {
		t.Fatalf("rejected install script echoes the token back:\n%s", body)
	}
	if !strings.Contains(body, "X-Registration-Token") {
		t.Fatalf("rejection message does not point at the header form:\n%s", body)
	}
	if strings.Contains(body, "#!/") || strings.Contains(body, "Registering agent") {
		t.Fatalf("query-token request still rendered a real install script")
	}
}

func TestWindowsInstallScriptUsesResolvedVersionAndRuntimeServerURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	signingService, err := services.NewSigningService(hex.EncodeToString(privateKey))
	if err != nil {
		t.Fatalf("new signing service: %v", err)
	}

	handler := NewDownloadHandler("/tmp/redflag-test", &config.Config{}, nil, signingService)
	router := gin.New()
	router.GET("/api/v1/install/:platform", handler.InstallScript)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/install/windows", nil)
	req.Header.Set("X-Registration-Token", "test-token")
	req.Host = "redflag.example:31336"
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	body := resp.Body.String()
	if strings.Contains(body, "vlatest") || strings.Contains(body, "version=latest") {
		t.Fatalf("installer still renders latest sentinel:\n%s", body)
	}
	if !strings.Contains(body, `$Version = "`+serverVersion.AgentVersion+`"`) {
		t.Fatalf("installer does not set resolved version %q", serverVersion.AgentVersion)
	}
	if !strings.Contains(body, `$BinaryURL = "$ServerUrl/api/v1/downloads/windows-${ArchTag}?version=`+serverVersion.AgentVersion+`"`) {
		t.Fatalf("binary URL is not built from runtime ServerUrl with resolved version")
	}
	if !strings.Contains(body, `$ManifestUrl = "$ServerUrl/api/v1/manifest?version=`+serverVersion.AgentVersion+`"`) {
		t.Fatalf("manifest URL is not built from runtime ServerUrl with resolved version")
	}
	if strings.Contains(body, `Invoke-WebRequest -Uri $ManifestUrl -UseBasicParsing -PassThru`) {
		t.Fatalf("manifest fetch uses PowerShell 5.1-incompatible PassThru without OutFile")
	}
	if strings.Contains(body, `[Convert]::FromHexString`) {
		t.Fatalf("installer uses PowerShell 5.1-incompatible FromHexString")
	}
	if strings.Contains(body, "Expected: Trust on First Use (TOFU) verification failed") {
		t.Fatalf("installer still reports verifier compatibility failures as TOFU tampering")
	}
	if !strings.Contains(body, "function Convert-HexToBytes") {
		t.Fatalf("installer does not include PowerShell 5.1-compatible hex decoder")
	}
	if !strings.Contains(body, "function Test-Ed25519Signature") {
		t.Fatalf("installer does not include Ed25519 verifier helper")
	}
	if !strings.Contains(body, "binary signature not checked (manifest hash pin already enforced)") {
		t.Fatalf("installer does not distinguish missing Ed25519 verifier from tampering")
	}
	if strings.Contains(body, `Administrators:(OI)(CI)F`) {
		t.Fatalf("installer uses localized/early ACL grant that can block registration")
	}
	if !strings.Contains(body, "function Set-AgentConfigPermissions") {
		t.Fatalf("installer does not include config ACL helper")
	}
	if !strings.Contains(body, "function Repair-AgentConfigAccessIfNeeded") {
		t.Fatalf("installer does not include preflight config ACL repair helper")
	}
	if !strings.Contains(body, `*S-1-5-32-544:F`) {
		t.Fatalf("installer does not grant config access using BUILTIN Administrators SID")
	}
	repairIndex := strings.Index(body, `Repair-AgentConfigAccessIfNeeded -ConfigFilePath $ConfigPath`)
	firstConfigReadIndex := strings.Index(body, `Get-Content $ConfigPath | ConvertFrom-Json`)
	if repairIndex == -1 || firstConfigReadIndex == -1 || repairIndex > firstConfigReadIndex {
		t.Fatalf("installer must repair unreadable existing config before detection reads it")
	}
	registerIndex := strings.Index(body, `[INFO] [installer] [register] Registering agent with server`)
	aclIndex := strings.Index(body, `Set-AgentConfigPermissions -ConfigFilePath $ConfigPath`)
	if registerIndex == -1 || aclIndex == -1 || aclIndex < registerIndex {
		t.Fatalf("installer must apply restrictive config ACL after registration block")
	}
	if !strings.Contains(body, `$ServerUrl          = $ServerUrl.TrimEnd("/")`) {
		t.Fatalf("installer does not normalize trailing slash on ServerUrl")
	}
	if !strings.Contains(body, "Manifest response: $ManifestErrorBody") {
		t.Fatalf("installer does not report manifest failure response body")
	}
}
