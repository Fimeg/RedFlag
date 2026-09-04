package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile_DoesNotOverrideExistingEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redflag.env")
	content := "REDFLAG_TEST_A=from_file\nREDFLAG_TEST_B=from_file\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	os.Unsetenv("REDFLAG_TEST_A")
	t.Setenv("REDFLAG_TEST_B", "already_set")

	if err := loadEnvFile(path); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}

	if got := os.Getenv("REDFLAG_TEST_A"); got != "from_file" {
		t.Errorf("REDFLAG_TEST_A = %q, want %q (should be set from file)", got, "from_file")
	}
	if got := os.Getenv("REDFLAG_TEST_B"); got != "already_set" {
		t.Errorf("REDFLAG_TEST_B = %q, want %q (existing env var must win)", got, "already_set")
	}
}

func TestLoadEnvFile_SkipsCommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redflag.env")
	content := "# a comment\n\nREDFLAG_TEST_C=\"quoted\"\n  \nREDFLAG_TEST_D='single'\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	os.Unsetenv("REDFLAG_TEST_C")
	os.Unsetenv("REDFLAG_TEST_D")

	if err := loadEnvFile(path); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}

	if got := os.Getenv("REDFLAG_TEST_C"); got != "quoted" {
		t.Errorf("REDFLAG_TEST_C = %q, want %q", got, "quoted")
	}
	if got := os.Getenv("REDFLAG_TEST_D"); got != "single" {
		t.Errorf("REDFLAG_TEST_D = %q, want %q", got, "single")
	}
}

func TestLoadNativeConfigFile_NoopWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REDFLAG_CONFIG_FILE", filepath.Join(dir, "does-not-exist.env"))
	os.Unsetenv("REDFLAG_TEST_SHOULD_NOT_APPEAR")

	loadNativeConfigFile() // must not panic or error when the file is missing

	if _, ok := os.LookupEnv("REDFLAG_TEST_SHOULD_NOT_APPEAR"); ok {
		t.Error("expected no env var to be set when config file is absent")
	}
}
