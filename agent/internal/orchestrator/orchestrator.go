package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/circuitbreaker"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/event"
)

// Scanner represents a generic update scanner
type Scanner interface {
	// IsAvailable checks if the scanner is available on this system
	IsAvailable() bool

	// Scan performs the actual scanning and returns update items
	Scan() ([]client.UpdateReportItem, error)

	// Name returns the scanner name for logging
	Name() string
}

// InventoryScanner scans for installed software, container images, and other
// inventory data. Distinct from Scanner which scans for available updates.
type InventoryScanner interface {
	IsAvailable() bool
	ScanInventory() ([]client.InventoryItem, error)
	Name() string
}

// InventoryScannerConfig holds configuration for a single inventory scanner.
type InventoryScannerConfig struct {
	Scanner        InventoryScanner
	CircuitBreaker *circuitbreaker.CircuitBreaker
	Timeout        time.Duration
	Enabled        bool
}

// InventoryScanResult holds the result of an inventory scanner execution.
type InventoryScanResult struct {
	ScannerName string
	Items       []client.InventoryItem
	Error       error
	Duration    time.Duration
	Status      string // "success", "failed", "disabled", "unavailable", "skipped"
}

// ScannerConfig holds configuration for a single scanner
type ScannerConfig struct {
	Scanner        Scanner
	CircuitBreaker *circuitbreaker.CircuitBreaker
	Timeout        time.Duration
	Enabled        bool
}

// ScanResult holds the result of a scanner execution
type ScanResult struct {
	ScannerName string
	Updates     []client.UpdateReportItem
	Error       error
	Duration    time.Duration
	Status      string // "success", "failed", "disabled", "unavailable", "skipped"
}

// Orchestrator manages and coordinates multiple scanners
type Orchestrator struct {
	scanners          map[string]*ScannerConfig
	inventoryScanners map[string]*InventoryScannerConfig
	logger            *event.TeeLogger
	mu                sync.RWMutex
}

// NewOrchestratorWithEvents creates a new scanner orchestrator with event buffering
func NewOrchestratorWithEvents(logger *event.TeeLogger) *Orchestrator {
	return &Orchestrator{
		scanners:          make(map[string]*ScannerConfig),
		inventoryScanners: make(map[string]*InventoryScannerConfig),
		logger:            logger,
	}
}

// RegisterScanner adds a scanner to the orchestrator
func (o *Orchestrator) RegisterScanner(name string, scanner Scanner, cb *circuitbreaker.CircuitBreaker, timeout time.Duration, enabled bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.scanners[name] = &ScannerConfig{
		Scanner:        scanner,
		CircuitBreaker: cb,
		Timeout:        timeout,
		Enabled:        enabled,
	}
}

// RegisterInventoryScanner adds an inventory scanner to the orchestrator.
func (o *Orchestrator) RegisterInventoryScanner(name string, scanner InventoryScanner, cb *circuitbreaker.CircuitBreaker, timeout time.Duration, enabled bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.inventoryScanners[name] = &InventoryScannerConfig{
		Scanner:        scanner,
		CircuitBreaker: cb,
		Timeout:        timeout,
		Enabled:        enabled,
	}
}

// ScanInventorySingle executes a single inventory scanner by name.
func (o *Orchestrator) ScanInventorySingle(ctx context.Context, scannerName string) (InventoryScanResult, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	cfg, exists := o.inventoryScanners[scannerName]
	if !exists {
		return InventoryScanResult{
			ScannerName: scannerName,
			Status:      "failed",
			Error:       fmt.Errorf("inventory scanner not found: %s", scannerName),
		}, fmt.Errorf("inventory scanner not found: %s", scannerName)
	}

	return o.executeInventoryScan(ctx, scannerName, cfg), nil
}

// executeInventoryScan runs a single inventory scanner with circuit breaker and timeout protection.
func (o *Orchestrator) executeInventoryScan(ctx context.Context, name string, cfg *InventoryScannerConfig) InventoryScanResult {
	result := InventoryScanResult{
		ScannerName: name,
		Status:      "failed",
	}

	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()

	if !cfg.Enabled {
		result.Status = "disabled"
		log.Printf("[%s] Inventory scanner disabled via configuration", name)
		return result
	}

	if !cfg.Scanner.IsAvailable() {
		result.Status = "unavailable"
		log.Printf("[%s] Inventory scanner not available on this system", name)
		return result
	}

	log.Printf("[%s] Starting inventory scan...", name)

	var items []client.InventoryItem

	err := cfg.CircuitBreaker.Call(func() error {
		timeoutCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()

		type scanResult struct {
			items []client.InventoryItem
			err   error
		}
		scanChan := make(chan scanResult, 1)

		go func() {
			i, e := cfg.Scanner.ScanInventory()
			scanChan <- scanResult{items: i, err: e}
		}()

		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("inventory scan timeout after %v", cfg.Timeout)
		case res := <-scanChan:
			if res.err != nil {
				return res.err
			}
			items = res.items
			return nil
		}
	})

	if err != nil {
		result.Error = err
		result.Status = "failed"
		log.Printf("[%s] Inventory scan failed: %v", name, err)
		return result
	}

	result.Items = items
	result.Status = "success"
	log.Printf("[%s] Inventory scan completed: found %d items (took %v)", name, len(items), result.Duration)

	return result
}

// ScanAll executes all registered scanners in parallel
func (o *Orchestrator) ScanAll(ctx context.Context) ([]ScanResult, []client.UpdateReportItem) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var wg sync.WaitGroup
	resultsChan := make(chan ScanResult, len(o.scanners))

	// Launch goroutine for each scanner
	for name, scannerConfig := range o.scanners {
		wg.Add(1)
		go func(name string, cfg *ScannerConfig) {
			defer wg.Done()
			result := o.executeScan(ctx, name, cfg)
			resultsChan <- result
		}(name, scannerConfig)
	}

	// Wait for all scanners to complete
	wg.Wait()
	close(resultsChan)

	// Collect results
	var results []ScanResult
	var allUpdates []client.UpdateReportItem

	for result := range resultsChan {
		results = append(results, result)
		if result.Error == nil && len(result.Updates) > 0 {
			allUpdates = append(allUpdates, result.Updates...)
		}
	}

	return results, allUpdates
}

// ScanSingle executes a single scanner by name
func (o *Orchestrator) ScanSingle(ctx context.Context, scannerName string) (ScanResult, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	cfg, exists := o.scanners[scannerName]
	if !exists {
		return ScanResult{
			ScannerName: scannerName,
			Status:      "failed",
			Error:       fmt.Errorf("scanner not found: %s", scannerName),
		}, fmt.Errorf("scanner not found: %s", scannerName)
	}

	return o.executeScan(ctx, scannerName, cfg), nil
}

// executeScan runs a single scanner with circuit breaker and timeout protection
func (o *Orchestrator) executeScan(ctx context.Context, name string, cfg *ScannerConfig) ScanResult {
	result := ScanResult{
		ScannerName: name,
		Status:      "failed",
	}

	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()

	// Check if enabled
	if !cfg.Enabled {
		result.Status = "disabled"
		o.logger.Info("agent", "orchestrator", "scanner", "scanner disabled", map[string]interface{}{
			"scanner_name": name,
			"status":       "disabled",
			"reason":       "configuration",
		})
		return result
	}

	// Check if available
	if !cfg.Scanner.IsAvailable() {
		result.Status = "unavailable"
		o.logger.Info("agent", "orchestrator", "scanner", "scanner unavailable", map[string]interface{}{
			"scanner_name": name,
			"status":       "unavailable",
			"reason":       "system_incompatible",
		})
		return result
	}

	// Execute with circuit breaker and timeout
	o.logger.Info("agent", "orchestrator", "scanner", "starting scan", map[string]interface{}{
		"scanner_name": name,
	})

	var updates []client.UpdateReportItem

	err := cfg.CircuitBreaker.Call(func() error {
		// Create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()

		// Channel for scan result
		type scanResult struct {
			updates []client.UpdateReportItem
			err     error
		}
		scanChan := make(chan scanResult, 1)

		// Run scan in goroutine
		go func() {
			u, e := cfg.Scanner.Scan()
			scanChan <- scanResult{updates: u, err: e}
		}()

		// Wait for scan or timeout
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("scan timeout after %v", cfg.Timeout)
		case res := <-scanChan:
			if res.err != nil {
				return res.err
			}
			updates = res.updates
			return nil
		}
	})

	if err != nil {
		result.Error = err
		result.Status = "failed"
		o.logger.Error("agent", "orchestrator", "scanner", "scan failed", map[string]interface{}{
			"scanner_name":  name,
			"error_type":    "scan_failed",
			"error_details": err.Error(),
			"duration_ms":   result.Duration.Milliseconds(),
		})
		return result
	}

	result.Updates = updates
	result.Status = "success"
	o.logger.Info("agent", "orchestrator", "scanner", "scan completed", map[string]interface{}{
		"scanner_name":  name,
		"updates_found": len(updates),
		"duration_ms":   result.Duration.Milliseconds(),
		"status":        "success",
	})

	return result
}

// GetScannerNames returns a list of all registered scanner names
func (o *Orchestrator) GetScannerNames() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	names := make([]string, 0, len(o.scanners))
	for name := range o.scanners {
		names = append(names, name)
	}
	return names
}

// FormatScanSummary creates a human-readable summary of scan results
func FormatScanSummary(results []ScanResult) (stdout string, stderr string, exitCode int) {
	var successResults []string
	var errorMessages []string
	totalUpdates := 0

	for _, result := range results {
		switch result.Status {
		case "success":
			msg := fmt.Sprintf("%s: Found %d updates (%.2fs)",
				result.ScannerName, len(result.Updates), result.Duration.Seconds())
			successResults = append(successResults, msg)
			totalUpdates += len(result.Updates)

		case "failed":
			msg := fmt.Sprintf("%s: %v", result.ScannerName, result.Error)
			errorMessages = append(errorMessages, msg)

		case "disabled":
			successResults = append(successResults, fmt.Sprintf("%s: Disabled", result.ScannerName))

		case "unavailable":
			successResults = append(successResults, fmt.Sprintf("%s: Not available", result.ScannerName))
		}
	}

	// Build stdout
	if len(successResults) > 0 {
		stdout = "Scan Results:\n"
		for _, msg := range successResults {
			stdout += fmt.Sprintf("  - %s\n", msg)
		}
		stdout += fmt.Sprintf("\nTotal Updates Found: %d\n", totalUpdates)
	}

	// Build stderr
	if len(errorMessages) > 0 {
		stderr = "Scan Errors:\n"
		for _, msg := range errorMessages {
			stderr += fmt.Sprintf("  - %s\n", msg)
		}
	}

	// Determine exit code
	if len(errorMessages) > 0 {
		exitCode = 1
	} else {
		exitCode = 0
	}

	return stdout, stderr, exitCode
}
