package upstream

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// defaultSyncWorkers is the concurrency cap used when
// REDFLAG_UPSTREAM_SYNC_CONCURRENCY is unset, zero, or invalid.
const defaultSyncWorkers = 5

// Syncer walks tracked_software at a configurable interval, dispatches each
// row to the appropriate ReleaseSource adapter, writes the result back, and
// emits a drift event when latest_version moves.
//
// tick() runs due rows concurrently up to syncWorkers goroutines (bounded by
// a semaphore). Set REDFLAG_UPSTREAM_SYNC_CONCURRENCY to override the default.
type Syncer struct {
	queries      *queries.UpstreamQueries
	registry     *Registry
	aliasQueries *queries.RepologyQueries
	aliasCache   *RepologyCache
	interval     time.Duration
	staleness    time.Duration
	batch        int
	syncWorkers  int
	shutdown     chan struct{}
	stopOnce     sync.Once
}

func NewSyncer(q *queries.UpstreamQueries, r *Registry, interval, staleness time.Duration, batch int, aliasQ *queries.RepologyQueries) *Syncer {
	if interval <= 0 {
		interval = time.Hour
	}
	if staleness <= 0 {
		staleness = 6 * time.Hour
	}
	if batch <= 0 {
		batch = 50
	}
	workers := defaultSyncWorkers
	if raw := os.Getenv("REDFLAG_UPSTREAM_SYNC_CONCURRENCY"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			workers = n
		} else {
			log.Printf("[WARN] [upstream] [syncer] invalid REDFLAG_UPSTREAM_SYNC_CONCURRENCY=%q, using default %d", raw, defaultSyncWorkers)
		}
	}
	var ac *RepologyCache
	if aliasQ != nil {
		ac = NewRepologyCache(aliasQ)
	}
	return &Syncer{
		queries:      q,
		registry:     r,
		aliasQueries: aliasQ,
		aliasCache:   ac,
		interval:     interval,
		staleness:    staleness,
		batch:        batch,
		syncWorkers:  workers,
		shutdown:     make(chan struct{}),
	}
}

func (s *Syncer) Start(ctx context.Context) {
	go s.loop(ctx)
}

func (s *Syncer) Stop() {
	s.stopOnce.Do(func() { close(s.shutdown) })
}

func (s *Syncer) loop(ctx context.Context) {
	log.Printf("[INFO] [upstream] [syncer] started interval=%s staleness=%s batch=%d workers=%d sources=%v",
		s.interval, s.staleness, s.batch, s.syncWorkers, s.registry.Names())

	// Run once immediately so a freshly-added row gets synced without
	// waiting a full interval. Then settle into the cadence.
	s.tick(ctx)
	s.tickAliases(ctx)

	t := time.NewTicker(s.interval)
	aliasTicker := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	defer aliasTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("[INFO] [upstream] [syncer] context cancelled, stopping")
			return
		case <-s.shutdown:
			log.Printf("[INFO] [upstream] [syncer] shutdown received, stopping")
			return
		case <-t.C:
			s.tick(ctx)
		case <-aliasTicker.C:
			s.tickAliases(ctx)
		}
	}
}

// tickAliases refreshes stale Repology alias cache entries.
func (s *Syncer) tickAliases(ctx context.Context) {
	if s.aliasQueries == nil || s.aliasCache == nil {
		return
	}
	rows, err := s.queries.List()
	if err != nil {
		log.Printf("[ERROR] [upstream] [syncer] tick_aliases list failed: %v", err)
		return
	}
	stale, err := s.aliasQueries.GetStaleSlugs(s.aliasCache.ttl, 0)
	if err != nil {
		log.Printf("[ERROR] [upstream] [syncer] tick_aliases get_stale_failed: %v", err)
		return
	}
	staleSet := make(map[string]bool, len(stale))
	for _, slug := range stale {
		staleSet[slug] = true
	}
	for _, row := range rows {
		if row.RepologySlug == nil || *row.RepologySlug == "" {
			continue
		}
		if !staleSet[*row.RepologySlug] {
			continue
		}
		if err := s.aliasCache.Refresh(ctx, *row.RepologySlug); err != nil {
			log.Printf("[WARN] [upstream] [syncer] tick_aliases refresh failed slug=%s: %v", *row.RepologySlug, err)
		}
	}
}

func (s *Syncer) tick(ctx context.Context) {
	rows, err := s.queries.DueForSync(s.staleness, s.batch)
	if err != nil {
		log.Printf("[ERROR] [upstream] [syncer] due_for_sync failed: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	log.Printf("[INFO] [upstream] [syncer] processing %d due rows workers=%d", len(rows), s.syncWorkers)

	// Bounded-concurrency fan-out: a buffered semaphore channel limits the
	// number of concurrent syncOne goroutines to s.syncWorkers. The WaitGroup
	// ensures tick() does not return until all launched goroutines finish, so
	// the next tick never overlaps with an in-flight batch.
	sem := make(chan struct{}, s.syncWorkers)
	var wg sync.WaitGroup
	for _, row := range rows {
		row := row // capture loop variable
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			_ = s.syncOne(ctx, row)
		}()
	}
	wg.Wait()
}

// SyncOne performs an on-demand sync for a single row (called by the
// admin "sync now" endpoint). Returns the error so the handler can surface
// it to the operator instead of only the DB.
func (s *Syncer) SyncOne(ctx context.Context, id uuid.UUID) error {
	row, err := s.queries.GetByID(id)
	if err != nil {
		return err
	}
	return s.syncOne(ctx, *row)
}

func (s *Syncer) syncOne(ctx context.Context, row models.TrackedSoftware) error {
	source, ok := s.registry.Get(row.Source)
	if !ok {
		msg := "unknown source: " + row.Source
		log.Printf("[WARN] [upstream] [syncer] %s for %s/%s", msg, row.Name, row.SourceRef)
		_ = s.queries.ApplySyncError(row.ID, msg)
		return fmt.Errorf("%s", msg)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var release *Release
	var err error
	if row.TrackPrereleases {
		if prereleaseSource, ok := source.(PrereleaseAwareSource); ok {
			release, err = prereleaseSource.FetchRelease(fetchCtx, row.SourceRef, true)
		} else {
			release, err = source.Fetch(fetchCtx, row.SourceRef)
		}
	} else {
		release, err = source.Fetch(fetchCtx, row.SourceRef)
	}
	if err != nil {
		log.Printf("[WARN] [upstream] [syncer] fetch failed source=%s ref=%s err=%v", row.Source, row.SourceRef, err)
		if dbErr := s.queries.ApplySyncError(row.ID, err.Error()); dbErr != nil {
			log.Printf("[ERROR] [upstream] [syncer] could not record fetch error: %v", dbErr)
		}
		return err
	}

	priorLatest := ""
	if row.LatestVersion != nil {
		priorLatest = *row.LatestVersion
	}

	if err := s.queries.ApplySyncResult(row.ID, release.Version, release.PublishedAt, release.EOLAt); err != nil {
		log.Printf("[ERROR] [upstream] [syncer] apply_sync_result failed: %v", err)
		return err
	}

	// Emit drift event when latest_version actually moved. The cheap heuristic
	// for severity: major if the first dotted segment changed, else minor.
	// "eol" overrides if endoflife.date now says the deployed version's branch
	// is past EOL.
	if priorLatest != "" && CompareVersions(priorLatest, release.Version) != 0 {
		severity := ClassifyDrift(priorLatest, release.Version)
		from, to := priorLatest, release.Version
		var note *string
		if release.EOLAt != nil && row.CurrentVersion != nil && release.EOLAt.Before(time.Now().UTC()) {
			severity = "eol"
			n := "deployed branch past upstream EOL"
			note = &n
		}
		if err := s.queries.InsertDriftEvent(row.ID, severity, &from, &to, note); err != nil {
			log.Printf("[ERROR] [upstream] [syncer] insert_drift_event failed: %v", err)
		} else {
			log.Printf("[INFO] [upstream] [syncer] drift name=%s severity=%s %s -> %s", row.Name, severity, from, release.Version)
		}
	}
	return nil
}
