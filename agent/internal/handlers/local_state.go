package handlers

import (
	"log"

	"github.com/Fimeg/RedFlag/agent/internal/cache"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/orchestrator"
)

func persistLocalState(cfg *config.Config, update func(*cache.LocalCache)) {
	localCache, err := cache.Load()
	if err != nil {
		log.Printf("[WARNING] [agent] [local_state] load_failed error=%v", err)
		localCache = &cache.LocalCache{}
	}

	applyLocalIdentity(localCache, cfg)
	if update != nil {
		update(localCache)
	}

	if err := localCache.Save(); err != nil {
		log.Printf("[WARNING] [agent] [local_state] save_failed error=%v", err)
	}
}

func applyLocalIdentity(localCache *cache.LocalCache, cfg *config.Config) {
	if localCache == nil || cfg == nil || !cfg.IsRegistered() {
		return
	}
	localCache.SetAgentInfo(cfg.AgentID, cfg.ServerURL)
}

func recordLocalFullScan(cfg *config.Config, updates []client.UpdateReportItem, results []orchestrator.ScanResult) {
	persistLocalState(cfg, func(localCache *cache.LocalCache) {
		localCache.SetAgentStatus("online")
		localCache.UpdateScanResults(updates)
		for _, result := range results {
			localCache.RecordScannerResult(result.ScannerName, result.Status, result.Updates, result.Error, result.Duration, false)
		}
	})
}

func recordLocalScanResult(cfg *config.Config, result orchestrator.ScanResult, affectsUpdateList bool) {
	persistLocalState(cfg, func(localCache *cache.LocalCache) {
		localCache.SetAgentStatus("online")
		localCache.RecordScannerResult(result.ScannerName, result.Status, result.Updates, result.Error, result.Duration, affectsUpdateList)
	})
}

func recordLocalScanResults(cfg *config.Config, results []orchestrator.ScanResult, affectsUpdateList bool) {
	persistLocalState(cfg, func(localCache *cache.LocalCache) {
		localCache.SetAgentStatus("online")
		for _, result := range results {
			localCache.RecordScannerResult(result.ScannerName, result.Status, result.Updates, result.Error, result.Duration, affectsUpdateList)
		}
	})
}
