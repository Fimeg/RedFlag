package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// ProcessHandler handles process scan endpoints.
type ProcessHandler struct {
	processQueries *queries.ProcessQueries
	agentQueries   *queries.AgentQueries
	commandQueries *queries.CommandQueries
	signingService *services.SigningService
}

// NewProcessHandler creates a new process handler.
func NewProcessHandler(
	pq *queries.ProcessQueries,
	aq *queries.AgentQueries,
	cq *queries.CommandQueries,
	ss *services.SigningService,
) *ProcessHandler {
	return &ProcessHandler{
		processQueries: pq,
		agentQueries:   aq,
		commandQueries: cq,
		signingService: ss,
	}
}

// ReportProcessScan handles POST /api/v1/agents/:id/process-scan
// Agent reports a full process scan result.
func (h *ProcessHandler) ReportProcessScan(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	var req models.ProcessScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if req.AgentID != agentID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Agent ID mismatch"})
		return
	}

	ctx := c.Request.Context()
	now := time.Now().UTC()

	// Insert snapshot header
	snapID := uuid.Must(uuid.NewV4())
	snap := models.ProcessSnapshot{
		ID:             snapID,
		AgentID:        agentID,
		CommandID:      req.CommandID,
		ProcessCount:   req.Snapshot.ProcessCount,
		ScannedAt:      req.Snapshot.ScannedAt,
		ScanDurationMs: int(req.Snapshot.DurationMs),
		CreatedAt:      now,
	}
	if err := h.processQueries.InsertSnapshot(ctx, snap); err != nil {
		log.Printf("[ERROR] [server] [processes] insert_snapshot agent=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save snapshot"})
		return
	}

	// Convert and insert processes
	procs := make([]models.Process, 0, len(req.Snapshot.Processes))
	for _, p := range req.Snapshot.Processes {
		procID := uuid.Must(uuid.NewV4())
		procs = append(procs, models.Process{
			ID:               procID,
			SnapshotID:       snapID,
			AgentID:          agentID,
			PID:              p.PID,
			Name:             p.Name,
			Path:             p.Path,
			Cmdline:          p.Cmdline,
			Cwd:              p.Cwd,
			State:            p.State,
			UID:              int(p.UID),
			GID:              int(p.GID),
			EUID:             int(p.EUID),
			EGID:             int(p.EGID),
			User:             p.User,
			Group:            p.Group,
			TTY:              p.TTY,
			TTYName:          p.TTYName,
			CPUSecondsUser:   p.CPUSecondsUser,
			CPUSecondsSystem: p.CPUSecondsSystem,
			CPUPercent:       p.CPUPercent,
			RSSBytes:         int64(p.RSSBytes),
			VMSBytes:         int64(p.VMSBytes),
			MemPercent:       p.MemPercent,
			Threads:          p.Threads,
			Nice:             p.Nice,
			StartTimeSeconds: int64(p.StartTimeSeconds),
			ParentPID:        p.ParentPID,
			ProcessGroupID:   p.ProcessGroupID,
			ElevationStatus:  p.ElevationStatus,
			OnDisk:           p.OnDisk,
			DiskBytesRead:    int64(p.DiskBytesRead),
			DiskBytesWritten: int64(p.DiskBytesWritten),
			CreatedAt:        now,
		})
	}

	if err := h.processQueries.InsertProcesses(ctx, procs); err != nil {
		log.Printf("[ERROR] [server] [processes] insert_processes agent=%s error=%v", agentID, err)
		// Clean up the snapshot header so the UI doesn't show an empty snapshot as latest.
		_ = h.processQueries.DeleteSnapshot(ctx, snapID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save processes"})
		return
	}

	// Insert related data for each process
	var relatedEntries []models.ProcessRelated
	for i, p := range req.Snapshot.Processes {
		if i >= len(procs) {
			break
		}
		procID := procs[i].ID
		now := time.Now().UTC()

		for _, relType := range []struct {
			name string
			data interface{}
		}{
			{"open_file", p.OpenFiles},
			{"socket", p.OpenSockets},
			{"pipe", p.OpenPipes},
			{"environment", p.EnvironmentVars},
			{"memory_map", p.MemoryMap},
			{"namespace", p.Namespaces},
			{"listening_port", p.ListeningPorts},
		} {
			if relType.data == nil {
				continue
			}
			// Marshal once, check for empty/null, then store as JSONB.
			dataJSON, err := json.Marshal(relType.data)
			if err != nil {
				continue
			}
			if len(dataJSON) <= 2 || string(dataJSON) == "null" {
				continue // "[]", "{}", or "null"
			}
			// Unmarshal into interface{} to get a generic Go value (slice or map),
			// which JSONB (map[string]interface{}) can hold as a value.
			var jb models.JSONB
			if err := json.Unmarshal(dataJSON, &jb); err != nil {
				// Slice data won't unmarshal into a map — wrap it.
				jb = models.JSONB{"_items": json.RawMessage(dataJSON)}
			}

			relatedEntries = append(relatedEntries, models.ProcessRelated{
				ID:           uuid.Must(uuid.NewV4()),
				ProcessID:    procID,
				RelationType: relType.name,
				Data:         jb,
				CreatedAt:    now,
			})
		}
	}

	if len(relatedEntries) > 0 {
		if err := h.processQueries.InsertRelatedData(ctx, relatedEntries); err != nil {
			log.Printf("[ERROR] [server] [processes] insert_related agent=%s error=%v", agentID, err)
			// Non-fatal: process data is saved, related data failed
		}
	}

	// Cleanup old snapshots (keep last 10)
	deleted, err := h.processQueries.CleanupOldSnapshots(ctx, agentID, 10)
	if err != nil {
		log.Printf("[WARN] [server] [processes] cleanup_old agent=%s error=%v", agentID, err)
	} else if deleted > 0 {
		log.Printf("[INFO] [server] [processes] cleanup_old agent=%s deleted=%d", agentID, deleted)
	}

	log.Printf("[INFO] [server] [processes] scan_stored agent=%s snapshot=%s processes=%d duration=%dms",
		agentID, snapID, len(procs), snap.ScanDurationMs)

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"snapshot_id": snapID,
		"processes":   len(procs),
	})
}

// GetLatestProcessSnapshot handles GET /api/v1/agents/:id/processes
// Dashboard reads the latest snapshot.
func (h *ProcessHandler) GetLatestProcessSnapshot(c *gin.Context) {
	agentIDStr := c.Param("id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	ctx := c.Request.Context()

	snap, err := h.processQueries.GetLatestSnapshot(ctx, agentID)
	if err != nil {
		log.Printf("[ERROR] [server] [processes] get_snapshot agent=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve snapshot"})
		return
	}
	if snap == nil {
		c.JSON(http.StatusOK, gin.H{"snapshot": nil, "processes": []interface{}{}})
		return
	}

	// Parse query params for filtering
	filter := queries.ProcessFilter{
		Name:    c.Query("name"),
		User:    c.Query("user"),
		State:   c.Query("state"),
		SortBy:  c.DefaultQuery("sort_by", "cpu"),
		SortDir: c.DefaultQuery("sort_dir", "desc"),
		Limit:   500, // cap at 500 for UI
	}
	if v := c.Query("limit"); v != "" {
		fmt.Sscanf(v, "%d", &filter.Limit)
	}
	if v := c.Query("offset"); v != "" {
		fmt.Sscanf(v, "%d", &filter.Offset)
	}

	procs, total, err := h.processQueries.GetProcessesBySnapshot(ctx, snap.ID, filter)
	if err != nil {
		log.Printf("[ERROR] [server] [processes] get_processes agent=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve processes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshot":  snap,
		"processes": procs,
		"total":     total,
	})
}

// GetProcessDetail handles GET /api/v1/agents/:id/processes/:processId
// Dashboard reads a single process with its related data.
func (h *ProcessHandler) GetProcessDetail(c *gin.Context) {
	processIDStr := c.Param("processId")
	processID, err := uuid.FromString(processIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid process ID"})
		return
	}

	ctx := c.Request.Context()

	proc, err := h.processQueries.GetProcessByID(ctx, processID)
	if err != nil {
		log.Printf("[ERROR] [server] [processes] get_detail error=%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve process"})
		return
	}
	if proc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Process not found"})
		return
	}

	// Get all related data
	related, err := h.processQueries.GetProcessRelated(ctx, processID, "")
	if err != nil {
		log.Printf("[WARN] [server] [processes] get_related error=%v", err)
	}

	// Group by type
	resp := models.ProcessDetailResponse{Process: *proc}
	for _, r := range related {
		switch r.RelationType {
		case "open_file":
			resp.OpenFiles = append(resp.OpenFiles, r)
		case "socket":
			resp.OpenSockets = append(resp.OpenSockets, r)
		case "pipe":
			resp.OpenPipes = append(resp.OpenPipes, r)
		case "environment":
			resp.Environment = append(resp.Environment, r)
		case "memory_map":
			resp.MemoryMap = append(resp.MemoryMap, r)
		case "namespace":
			resp.Namespaces = append(resp.Namespaces, r)
		case "listening_port":
			resp.ListeningPorts = append(resp.ListeningPorts, r)
		}
	}

	c.JSON(http.StatusOK, resp)
}

// TriggerProcessScan handles POST /api/v1/agents/:id/processes/scan
// Dashboard triggers an on-demand process scan.
func (h *ProcessHandler) TriggerProcessScan(c *gin.Context) {
	agentIDStr := c.Param("id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	// Verify agent exists
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil || agent == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Check for existing pending scan_processes command (dedup)
	// Use GetPendingCommands (small bounded set) instead of GetCommandsByAgentID (all history).
	pendingCmds, err := h.commandQueries.GetPendingCommands(agentID)
	if err == nil {
		for _, cmd := range pendingCmds {
			if cmd.CommandType == "scan_processes" {
				c.JSON(http.StatusOK, gin.H{
					"message":    "Scan already in progress",
					"command_id": cmd.ID.String(),
				})
				return
			}
		}
	}

	// Create signed command
	cmdID := uuid.Must(uuid.NewV4())
	cmd := &models.AgentCommand{
		ID:          cmdID,
		AgentID:     agentID,
		CommandType: "scan_processes",
		Status:      "pending",
		Source:      "manual",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if h.signingService == nil || !h.signingService.IsEnabled() {
		log.Printf("[ERROR] [server] [processes] signing_unavailable agent=%s", agentID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Signing service unavailable"})
		return
	}

	signature, err := h.signingService.SignCommand(cmd)
	if err != nil {
		log.Printf("[ERROR] [server] [processes] sign_failed agent=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sign command"})
		return
	}
	cmd.Signature = signature

	if err := h.commandQueries.CreateCommand(cmd); err != nil {
		log.Printf("[ERROR] [server] [processes] create_command agent=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create command"})
		return
	}

	log.Printf("[INFO] [server] [processes] scan_triggered agent=%s command=%s", agentID, cmdID)

	c.JSON(http.StatusOK, gin.H{
		"message":    "Process scan triggered",
		"command_id": cmdID,
	})
}
