package services_test

// ethos_logging_test.go — Pre-fix tests for fmt.Printf in services.
// D-2: security_settings_service.go uses fmt.Printf for audit log warning.

import (
	"os"
	"strings"
	"testing"
)

func TestServicesUseFmtPrintfForLogging(t *testing.T) {
	content, err := os.ReadFile("security_settings_service.go")
	if err != nil {
		t.Fatalf("failed to read security_settings_service.go: %v", err)
	}

	count := strings.Count(string(content), "fmt.Printf")
	if count > 0 {
		t.Errorf("[ERROR] [server] [services] D-2 NOT FIXED: %d fmt.Printf in security_settings_service.go", count)
	}

	t.Log("[INFO] [server] [services] D-2 FIXED: no fmt.Printf in security_settings_service.go")
}

func TestServicesUseStructuredLogging(t *testing.T) {
	content, err := os.ReadFile("security_settings_service.go")
	if err != nil {
		t.Fatalf("failed to read security_settings_service.go: %v", err)
	}

	count := strings.Count(string(content), "fmt.Printf")
	if count > 0 {
		t.Errorf("[ERROR] [server] [services] %d fmt.Printf calls in security_settings_service.go.\n"+
			"D-2: use log.Printf with ETHOS format.", count)
	}
}
