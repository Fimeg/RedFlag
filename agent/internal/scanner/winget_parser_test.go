package scanner

// winget_parser_test.go — Pre-fix tests for winget output parsing.
// [SHARED] — no build tag, compiles on all platforms.
//
// F-C1-2 MEDIUM: Text fallback parser splits on whitespace,
//   breaks on package names with spaces.
// F-C1-8 LOW: No winget parsing tests existed before this file.
//
// Run: cd agent && go test ./internal/scanner/... -v -run TestWinget

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 2.1 — Text parser handles spaces in package names (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestWingetTextParserHandlesSpacesInPackageNames(t *testing.T) {
	// F-C1-2 MEDIUM: Text parser splits on all whitespace.
	// Package names with spaces are truncated to the first word.
	// After fix: use column-position parsing based on header separator line.
	scanner := NewWingetScanner()

	mockOutput := "Name                               Id                          Version  Available\n" +
		"------------------------------------------------------------------------------------\n" +
		"Microsoft Visual Studio Code       Microsoft.VisualStudioCode  1.85.0   1.86.0\n" +
		"Git                                Git.Git                     2.43.0   2.44.0\n"

	updates, err := scanner.parseWingetTextOutput(mockOutput)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(updates) == 0 {
		t.Fatal("[ERROR] [agent] [scanner] no updates parsed from text output")
	}

	// The first package should have the full name
	firstName := updates[0].PackageName
	if firstName != "Microsoft Visual Studio Code" {
		t.Errorf("[ERROR] [agent] [scanner] expected full name \"Microsoft Visual Studio Code\", got %q.\n"+
			"F-C1-2: text parser truncates names at first space.", firstName)
	}
}

// ---------------------------------------------------------------------------
// Test 2.2 — Documents text parser truncation (F-C1-2)
//
// Category: PASS-NOW (documents the bug)
// ---------------------------------------------------------------------------

func TestWingetTextParserCurrentlyBreaksOnSpaces(t *testing.T) {
	// POST-FIX (F-C1-2): Text parser now uses column-position parsing.
	// Full package names with spaces are preserved.
	scanner := NewWingetScanner()

	mockOutput := "Name                               Id                          Version  Available\n" +
		"------------------------------------------------------------------------------------\n" +
		"Microsoft Visual Studio Code       Microsoft.VisualStudioCode  1.85.0   1.86.0\n"

	updates, err := scanner.parseWingetTextOutput(mockOutput)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(updates) == 0 {
		t.Fatal("[ERROR] [agent] [scanner] no updates parsed")
	}

	if updates[0].PackageName != "Microsoft Visual Studio Code" {
		t.Errorf("[ERROR] [agent] [scanner] expected full name, got %q", updates[0].PackageName)
	}

	t.Log("[INFO] [agent] [scanner] F-C1-2 FIXED: full package name preserved")
}

// ---------------------------------------------------------------------------
// Test 2.3 — JSON parser handles spaces correctly
//
// Category: PASS-NOW (JSON path works)
// ---------------------------------------------------------------------------

func TestWingetJsonParserHandlesSpacesInPackageNames(t *testing.T) {
	// F-C1-2: JSON parser handles spaces correctly.
	// The text fallback is the broken path.
	scanner := NewWingetScanner()

	// Mock JSON output matching WingetPackage struct
	mockJSON := `[
		{"Name":"Microsoft Visual Studio Code","Id":"Microsoft.VisualStudioCode","Version":"1.85.0","Available":"1.86.0","Source":"winget"},
		{"Name":"Git","Id":"Git.Git","Version":"2.43.0","Available":"2.44.0","Source":"winget"}
	]`

	var packages []WingetPackage
	if err := json.Unmarshal([]byte(mockJSON), &packages); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if len(packages) < 1 {
		t.Fatal("[ERROR] [agent] [scanner] no packages parsed from JSON")
	}

	// JSON parser should get full name
	item := scanner.parseWingetPackage(packages[0])
	if item.PackageName != "Microsoft Visual Studio Code" {
		t.Errorf("[ERROR] [agent] [scanner] expected full name, got %q", item.PackageName)
	}

	t.Log("[INFO] [agent] [scanner] F-C1-2: JSON parser handles spaces correctly")
}

// ---------------------------------------------------------------------------
// Test 2.4 — Both parsers return consistent structure
//
// Category: PASS-NOW
// ---------------------------------------------------------------------------

func TestWingetParserReturnsConsistentStructure(t *testing.T) {
	// F-C1-2: Both parsers must return the same struct type.
	scanner := NewWingetScanner()

	// JSON path
	pkg := WingetPackage{
		Name:      "Git",
		ID:        "Git.Git",
		Version:   "2.43.0",
		Available: "2.44.0",
		Source:    "winget",
	}
	jsonResult := scanner.parseWingetPackage(pkg)

	// Text path (simple name without spaces)
	textOutput := "Name  Id       Version  Available\n" +
		"----------------------------------\n" +
		"Git   Git.Git  2.43.0   2.44.0\n"
	textResults, err := scanner.parseWingetTextOutput(textOutput)
	if err != nil {
		t.Fatalf("text parse error: %v", err)
	}

	if len(textResults) == 0 {
		t.Fatal("[ERROR] [agent] [scanner] no results from text parser")
	}

	// Both should have the same package type
	if jsonResult.PackageType != textResults[0].PackageType {
		t.Errorf("[ERROR] [agent] [scanner] package type mismatch: json=%q text=%q",
			jsonResult.PackageType, textResults[0].PackageType)
	}

	t.Log("[INFO] [agent] [scanner] F-C1-2: both parsers return consistent structure")
}
