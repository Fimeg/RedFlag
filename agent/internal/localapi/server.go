package localapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/cache"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/models"
	"github.com/Fimeg/RedFlag/agent/internal/system"
	"github.com/Fimeg/RedFlag/agent/internal/version"
)

const (
	DefaultUnixGroupName    = "redflag-local"
	DefaultWindowsGroupName = "RedFlagLocal"
	DefaultWindowsPipeName  = `\\.\pipe\RedFlagAgentLocal`
)

// ErrScanInFlight is returned by a TriggerScan callback while a previously
// triggered scan is still running. The handler maps it to 409 Conflict.
var ErrScanInFlight = errors.New("localapi: scan already in flight")

// Approval callback sentinels. The loop wiring translates handler/supplychain
// errors into these so this package stays decoupled from the approval stack.
var (
	// ErrApprovalConflict → 409: gate refused, fleet mode, duplicate, or an
	// approval already in flight. The wrapped message carries the specifics.
	ErrApprovalConflict = errors.New("localapi: approval conflict")
	// ErrApprovalUnavailable → 503: no local authority on this host.
	ErrApprovalUnavailable = errors.New("localapi: approval unavailable")
)

// Options configures the local read-only API. Group, socket, and pipe defaults
// are platform-specific and enforced by the listener implementation.
type Options struct {
	Config                *config.Config
	GroupName             string
	UnixSocketPath        string
	WindowsPipeName       string
	LoadCache             func() (*cache.LocalCache, error)
	RequestLog            func(format string, args ...interface{})
	ListenerOverride      net.Listener
	DesktopProvider       DesktopStatusProvider
	SystemProvider        func() (*system.SystemInfo, error)
	ProcessProvider       func(limit int) ([]system.TopProcess, error)
	MonitorProvider       func() (*system.ResourceSnapshot, error)
	ProcessesProvider     func() (*system.FullProcessSnapshot, error)
	ProcessDetailProvider func(pid int, caps system.ProcessCaps) (*system.FullProcess, error)
	SoftwareProvider      func() (*system.SoftwareSnapshot, error)
	PackageDetailProvider func(packageType, identity string) (*system.PackageDetail, error)
	ConnectionsProvider   func() (*system.ConnectionSnapshot, error)
	ServicesProvider      func() (*system.ServiceSnapshot, error)
	DockerProvider        func() (*DockerResponse, error)
	EventsProvider        func() ([]*models.SystemEvent, error)
	// TriggerScan enqueues a package-update scan through the agent's existing
	// scanner primitives (FEAT-002 write path). Authorization is the OS-local
	// group boundary on the socket/pipe — anyone who can connect may trigger.
	// Nil disables the endpoint (503). Return ErrScanInFlight to signal 409.
	TriggerScan func(source string) error
	// ApproveUpdate runs the standalone approval flow (FEAT-003): gates →
	// mint → execute. Synchronous; the response carries the full verdict.
	// Nil disables the endpoint (503). Wrap errors in ErrApprovalConflict /
	// ErrApprovalUnavailable to control the HTTP status.
	ApproveUpdate func(body []byte) (interface{}, error)
	// OnDesktopHealth receives each Desktop self-report (POST /v1/desktop) so the
	// agent can track app liveness/version — on Linux Desktop is autostart-
	// launched and this is the only signal. Nil means reports are logged only.
	OnDesktopHealth func(version string, windowOpen bool)
}

// Server owns the local API listener and HTTP server.
type Server struct {
	httpServer *http.Server
	listener   net.Listener
	address    string
	logf       func(format string, args ...interface{})
	monitor    *system.ResourceMonitor
}

// Start creates a platform-native local listener and serves the read-only local
// API. It returns an error before serving if the OS-local ACL cannot be applied.
func Start(opts Options) (*Server, error) {
	if opts.Config == nil {
		return nil, errors.New("localapi: config is required")
	}
	if opts.LoadCache == nil {
		opts.LoadCache = cache.Load
	}
	if opts.RequestLog == nil {
		opts.RequestLog = log.Printf
	}

	var monitor *system.ResourceMonitor
	if opts.MonitorProvider == nil {
		monitor = system.NewResourceMonitor(time.Second, 300)
		monitor.Start()
		opts.MonitorProvider = monitor.Snapshot
	}
	handler := newHandler(opts)
	httpServer := &http.Server{Handler: handler}

	listener := opts.ListenerOverride
	address := ""
	var err error
	if listener == nil {
		listener, address, err = listen(opts)
		if err != nil {
			if monitor != nil {
				monitor.Stop()
			}
			return nil, err
		}
	} else {
		address = listener.Addr().String()
	}

	srv := &Server{
		httpServer: httpServer,
		listener:   listener,
		address:    address,
		logf:       opts.RequestLog,
		monitor:    monitor,
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			opts.RequestLog("[ERROR] [agent] [localapi] serve_failed address=%s error=%v", address, err)
		}
	}()

	opts.RequestLog("[INFO] [agent] [localapi] started address=%s", address)
	return srv, nil
}

// Stop shuts down the local API listener.
func (s *Server) Stop() {
	if s == nil || s.httpServer == nil {
		return
	}
	if err := s.httpServer.Close(); err != nil && s.logf != nil {
		s.logf("[WARNING] [agent] [localapi] stop_failed address=%s error=%v", s.address, err)
	}
	if s.monitor != nil {
		s.monitor.Stop()
	}
}

type handler struct {
	cfg             *config.Config
	loadCache       func() (*cache.LocalCache, error)
	desktop         DesktopStatusProvider
	triggerScan     func(source string) error
	approveUpdate   func(body []byte) (interface{}, error)
	onDesktopHealth func(version string, windowOpen bool)
	systemInfo      func() (*system.SystemInfo, error)
	topProcesses    func(limit int) ([]system.TopProcess, error)
	monitorSnapshot func() (*system.ResourceSnapshot, error)
	processes       func() (*system.FullProcessSnapshot, error)
	processDetail   func(pid int, caps system.ProcessCaps) (*system.FullProcess, error)
	software        func() (*system.SoftwareSnapshot, error)
	packageDetail   func(packageType, identity string) (*system.PackageDetail, error)
	connections     func() (*system.ConnectionSnapshot, error)
	services        func() (*system.ServiceSnapshot, error)
	docker          func() (*DockerResponse, error)
	events          func() ([]*models.SystemEvent, error)
}

// DesktopStatusProvider allows the desktop manager to report its status.
type DesktopStatusProvider interface {
	Status() (running bool, pid int)
}

type IdentityResponse struct {
	AgentID         string   `json:"agent_id"`
	ServerURL       string   `json:"server_url"`
	Hostname        string   `json:"hostname,omitempty"`
	OSType          string   `json:"os_type,omitempty"`
	DisplayName     string   `json:"display_name,omitempty"`
	Organization    string   `json:"organization,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	AgentVersion    string   `json:"agent_version"`
	ConfigVersion   string   `json:"config_version,omitempty"`
	CheckInInterval int      `json:"check_in_interval"`
	Registered      bool     `json:"registered"`
}

type StatusResponse struct {
	AgentStatus string                        `json:"agent_status"`
	LastCheckIn time.Time                     `json:"last_check_in,omitempty"`
	LastUpdated time.Time                     `json:"last_updated,omitempty"`
	LastScan    time.Time                     `json:"last_scan_time,omitempty"`
	UpdateCount int                           `json:"update_count"`
	Summary     cache.UpdateSummary           `json:"summary"`
	Scanners    map[string]cache.ScannerState `json:"scanners,omitempty"`
	Registered  bool                          `json:"registered"`
	Desktop     *DesktopStatus                `json:"desktop,omitempty"`
}

type DesktopStatus struct {
	Running bool `json:"running"`
	PID     int  `json:"pid,omitempty"`
	Enabled bool `json:"enabled"`
}

// DesktopHealthRequest is sent by the desktop app to report its health.
type DesktopHealthRequest struct {
	Version    string `json:"version"`
	Uptime     int64  `json:"uptime_seconds"`
	WindowOpen bool   `json:"window_open"`
}

type ScanResponse struct {
	LastScanTime time.Time                     `json:"last_scan_time,omitempty"`
	LastUpdated  time.Time                     `json:"last_updated,omitempty"`
	UpdateCount  int                           `json:"update_count"`
	Summary      cache.UpdateSummary           `json:"summary"`
	Scanners     map[string]cache.ScannerState `json:"scanners,omitempty"`
	Updates      []client.UpdateReportItem     `json:"updates"`
}

type TokenResponse struct {
	Capabilities cache.CapabilityTokenState `json:"capabilities"`
}

type SystemResponse struct {
	System       *system.SystemInfo  `json:"system"`
	TopProcesses []system.TopProcess `json:"top_processes"`
	CollectedAt  time.Time           `json:"collected_at"`
}

type DockerResponse struct {
	Available   bool                           `json:"available"`
	Version     string                         `json:"version,omitempty"`
	Containers  []client.DockerReportContainer `json:"containers"`
	Stacks      []client.DockerReportStack     `json:"stacks"`
	Count       int                            `json:"count"`
	Running     int                            `json:"running"`
	Unhealthy   int                            `json:"unhealthy"`
	CollectedAt time.Time                      `json:"collected_at"`
}

type EventsResponse struct {
	Events      []*models.SystemEvent `json:"events"`
	Count       int                   `json:"count"`
	CollectedAt time.Time             `json:"collected_at"`
}

type SecurityResponse struct {
	CommandSigningEnabled bool                       `json:"command_signing_enabled"`
	CommandEnforcement    string                     `json:"command_enforcement"`
	TLSVerification       bool                       `json:"tls_verification"`
	SecurityLogging       bool                       `json:"security_logging"`
	KernelEnforcement     bool                       `json:"kernel_enforcement"`
	KernelFailClosed      bool                       `json:"kernel_fail_closed"`
	DegradedMode          bool                       `json:"degraded_mode"`
	Registered            bool                       `json:"registered"`
	CriticalUpdates       int                        `json:"critical_updates"`
	HighUpdates           int                        `json:"high_updates"`
	Capabilities          cache.CapabilityTokenState `json:"capabilities"`
	CollectedAt           time.Time                  `json:"collected_at"`
}

func newHandler(opts Options) http.Handler {
	h := &handler{
		cfg:             opts.Config,
		loadCache:       opts.LoadCache,
		desktop:         opts.DesktopProvider,
		triggerScan:     opts.TriggerScan,
		approveUpdate:   opts.ApproveUpdate,
		onDesktopHealth: opts.OnDesktopHealth,
		systemInfo:      opts.SystemProvider,
		topProcesses:    opts.ProcessProvider,
		monitorSnapshot: opts.MonitorProvider,
		processes:       opts.ProcessesProvider,
		processDetail:   opts.ProcessDetailProvider,
		software:        opts.SoftwareProvider,
		packageDetail:   opts.PackageDetailProvider,
		connections:     opts.ConnectionsProvider,
		services:        opts.ServicesProvider,
		docker:          opts.DockerProvider,
		events:          opts.EventsProvider,
	}
	if h.systemInfo == nil {
		h.systemInfo = func() (*system.SystemInfo, error) {
			return system.GetSystemInfo(version.Version)
		}
	}
	if h.topProcesses == nil {
		h.topProcesses = system.GetTopProcesses
	}
	if h.processes == nil {
		h.processes = system.GetFullProcessSnapshot
	}
	if h.processDetail == nil {
		h.processDetail = system.GetProcessDetail
	}
	if h.software == nil {
		h.software = system.GetSoftwareSnapshot
	}
	if h.packageDetail == nil {
		h.packageDetail = system.GetPackageDetail
	}
	if h.connections == nil {
		h.connections = system.GetConnectionsSnapshot
	}
	if h.services == nil {
		h.services = system.GetServicesSnapshot
	}
	if h.loadCache == nil {
		h.loadCache = cache.Load
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/identity", h.identity)
	mux.HandleFunc("/v1/status", h.status)
	mux.HandleFunc("/v1/scans/latest", h.scansLatest)
	mux.HandleFunc("/v1/packages", h.scansLatest)
	mux.HandleFunc("/v1/system", h.system)
	mux.HandleFunc("/v1/monitor", h.monitor)
	mux.HandleFunc("/v1/processes", h.processList)
	mux.HandleFunc("/v1/processes/", h.processByPID)
	mux.HandleFunc("/v1/software", h.softwareList)
	mux.HandleFunc("/v1/software/detail", h.softwareDetail)
	mux.HandleFunc("/v1/connections", h.connectionList)
	mux.HandleFunc("/v1/services", h.serviceList)
	mux.HandleFunc("/v1/containers", h.containerList)
	mux.HandleFunc("/v1/events", h.eventList)
	mux.HandleFunc("/v1/security", h.security)
	mux.HandleFunc("/v1/tokens/active", h.tokensActive)
	mux.HandleFunc("/v1/desktop", h.desktopHealth)
	mux.HandleFunc("/v1/actions/trigger-scan", h.triggerScanAction)
	mux.HandleFunc("/v1/actions/approve-update", h.approveUpdateAction)
	return mux
}

func (h *handler) containerList(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	if h.docker == nil {
		writeJSON(w, DockerResponse{Available: false, CollectedAt: time.Now().UTC()})
		return
	}
	snapshot, err := h.docker()
	if err != nil {
		log.Printf("[WARNING] [agent] [localapi] container_inventory_failed error=%v", err)
		http.Error(w, "container inventory unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, snapshot)
}

func (h *handler) eventList(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	if h.events == nil {
		writeJSON(w, EventsResponse{CollectedAt: time.Now().UTC()})
		return
	}
	events, err := h.events()
	if err != nil {
		log.Printf("[WARNING] [agent] [localapi] event_history_failed error=%v", err)
		http.Error(w, "local event history unavailable", http.StatusServiceUnavailable)
		return
	}
	sort.Slice(events, func(i, j int) bool { return events[i].CreatedAt.After(events[j].CreatedAt) })
	writeJSON(w, EventsResponse{Events: events, Count: len(events), CollectedAt: time.Now().UTC()})
}

func (h *handler) security(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	localCache, ok := h.load(w)
	if !ok {
		return
	}
	response := SecurityResponse{
		CommandSigningEnabled: h.cfg.CommandSigning.Enabled,
		CommandEnforcement:    h.cfg.CommandSigning.EnforcementMode,
		TLSVerification:       !h.cfg.TLS.InsecureSkipVerify,
		SecurityLogging:       h.cfg.SecurityLogging.Enabled,
		KernelEnforcement:     h.cfg.KernelEnforcement.Enabled,
		KernelFailClosed:      h.cfg.KernelEnforcement.FailClosed,
		DegradedMode:          h.cfg.DegradedMode,
		Registered:            h.cfg.IsRegistered(),
		CriticalUpdates:       localCache.Summary.BySeverity["critical"],
		HighUpdates:           localCache.Summary.BySeverity["high"] + localCache.Summary.BySeverity["important"],
		Capabilities:          localCache.Capabilities,
		CollectedAt:           time.Now().UTC(),
	}
	writeJSON(w, response)
}

func (h *handler) connectionList(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	snapshot, err := h.connections()
	if err != nil {
		log.Printf("[WARNING] [agent] [localapi] connection_inventory_failed error=%v", err)
		http.Error(w, "connection inventory unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, snapshot)
}

func (h *handler) serviceList(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	snapshot, err := h.services()
	if err != nil {
		log.Printf("[WARNING] [agent] [localapi] service_inventory_failed error=%v", err)
		http.Error(w, "service inventory unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, snapshot)
}

func (h *handler) monitor(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	if h.monitorSnapshot == nil {
		http.Error(w, "resource monitor unavailable", http.StatusServiceUnavailable)
		return
	}
	snapshot, err := h.monitorSnapshot()
	if err != nil {
		log.Printf("[WARNING] [agent] [localapi] resource_monitor_failed error=%v", err)
		http.Error(w, "resource monitor unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, snapshot)
}

func (h *handler) processList(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	snapshot, err := h.processes()
	if err != nil {
		log.Printf("[WARNING] [agent] [localapi] process_inventory_failed error=%v", err)
		http.Error(w, "process inventory unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, snapshot)
}

func (h *handler) processByPID(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	rawPID := strings.TrimPrefix(r.URL.Path, "/v1/processes/")
	pid, err := strconv.Atoi(rawPID)
	if err != nil || pid <= 0 || strings.Contains(rawPID, "/") {
		http.Error(w, "invalid process id", http.StatusBadRequest)
		return
	}
	caps := processCaps(h.cfg.ProcessExplorer)
	process, err := h.processDetail(pid, caps)
	if err != nil {
		log.Printf("[INFO] [agent] [localapi] process_detail_unavailable pid=%d error=%v", pid, err)
		http.Error(w, "process unavailable", http.StatusNotFound)
		return
	}
	writeJSON(w, process)
}

func (h *handler) softwareList(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	snapshot, err := h.software()
	if err != nil {
		log.Printf("[WARNING] [agent] [localapi] software_inventory_failed error=%v", err)
		http.Error(w, "software inventory unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, snapshot)
}

func (h *handler) softwareDetail(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	packageType := strings.TrimSpace(r.URL.Query().Get("manager"))
	identity := strings.TrimSpace(r.URL.Query().Get("identity"))
	if packageType == "" || identity == "" {
		http.Error(w, "manager and identity are required", http.StatusBadRequest)
		return
	}
	detail, err := h.packageDetail(packageType, identity)
	if err != nil {
		log.Printf("[INFO] [agent] [localapi] software_detail_unavailable manager=%s identity=%s error=%v", packageType, identity, err)
		http.Error(w, "software detail unavailable", http.StatusNotFound)
		return
	}
	writeJSON(w, detail)
}

func processCaps(cfg config.ProcessExplorerConfig) system.ProcessCaps {
	capOr := func(value, fallback int) int {
		if value > 0 {
			return value
		}
		return fallback
	}
	return system.ProcessCaps{
		MaxOpenFiles:      capOr(cfg.MaxOpenFiles, 2000),
		MaxSockets:        capOr(cfg.MaxSockets, 500),
		MaxPipes:          capOr(cfg.MaxPipes, 500),
		MaxMemoryMap:      capOr(cfg.MaxMemoryMap, 2000),
		MaxNamespaces:     capOr(cfg.MaxNamespaces, 50),
		MaxEnvKeys:        capOr(cfg.MaxEnvKeys, 200),
		MaxListeningPorts: capOr(cfg.MaxListeningPorts, 100),
	}
}

func (h *handler) system(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	info, err := h.systemInfo()
	if err != nil {
		log.Printf("[ERROR] [agent] [localapi] system_info_failed error=%v", err)
		http.Error(w, "system health unavailable", http.StatusServiceUnavailable)
		return
	}
	processes, err := h.topProcesses(8)
	if err != nil {
		log.Printf("[WARNING] [agent] [localapi] top_processes_failed error=%v", err)
		processes = nil
	}
	writeJSON(w, SystemResponse{
		System:       info,
		TopProcesses: processes,
		CollectedAt:  time.Now().UTC(),
	})
}

func (h *handler) identity(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("[WARNING] [agent] [localapi] hostname_failed error=%v", err)
	}

	resp := IdentityResponse{
		AgentID:         h.cfg.AgentID.String(),
		ServerURL:       h.cfg.ServerURL,
		Hostname:        hostname,
		OSType:          h.cfg.OSType,
		DisplayName:     h.cfg.DisplayName,
		Organization:    h.cfg.Organization,
		Tags:            sortedCopy(h.cfg.Tags),
		AgentVersion:    version.Version,
		ConfigVersion:   h.cfg.Version,
		CheckInInterval: h.cfg.CheckInInterval,
		Registered:      h.cfg.IsRegistered(),
	}
	writeJSON(w, resp)
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	localCache, ok := h.load(w)
	if !ok {
		return
	}

	resp := StatusResponse{
		AgentStatus: localCache.AgentStatus,
		LastCheckIn: localCache.LastCheckIn,
		LastUpdated: localCache.LastUpdated,
		LastScan:    localCache.LastScanTime,
		UpdateCount: localCache.UpdateCount,
		Summary:     localCache.Summary,
		Scanners:    localCache.Scanners,
		Registered:  h.cfg.IsRegistered(),
	}

	if h.desktop != nil {
		running, pid := h.desktop.Status()
		resp.Desktop = &DesktopStatus{
			Running: running,
			PID:     pid,
			Enabled: h.cfg.Desktop.Enabled,
		}
	}

	writeJSON(w, resp)
}

func (h *handler) scansLatest(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	localCache, ok := h.load(w)
	if !ok {
		return
	}
	writeJSON(w, ScanResponse{
		LastScanTime: localCache.LastScanTime,
		LastUpdated:  localCache.LastUpdated,
		UpdateCount:  localCache.UpdateCount,
		Summary:      localCache.Summary,
		Scanners:     localCache.Scanners,
		Updates:      localCache.Updates,
	})
}

func (h *handler) tokensActive(w http.ResponseWriter, r *http.Request) {
	if !requireGet(w, r) {
		return
	}
	localCache, ok := h.load(w)
	if !ok {
		return
	}
	writeJSON(w, TokenResponse{Capabilities: localCache.Capabilities})
}

// desktopHealth handles POST /v1/desktop — the desktop app reports its health.
func (h *handler) desktopHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DesktopHealthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Log the desktop health report.
	log.Printf("[INFO] [agent] [localapi] desktop_health version=%s uptime=%ds window_open=%v",
		req.Version, req.Uptime, req.WindowOpen)

	if h.onDesktopHealth != nil {
		h.onDesktopHealth(req.Version, req.WindowOpen)
	}

	// Respond with agent status so the desktop app can display it.
	desktopStatus := DesktopStatus{Enabled: h.cfg.Desktop.Enabled}
	if h.desktop != nil {
		running, pid := h.desktop.Status()
		desktopStatus.Running = running
		desktopStatus.PID = pid
	}

	writeJSON(w, map[string]interface{}{
		"agent_version": version.Version,
		"desktop":       desktopStatus,
	})
}

// triggerScanAction handles POST /v1/actions/trigger-scan — the first local
// write endpoint (FEAT-002). The OS-local group ACL on the socket/pipe is the
// authorization boundary. The scan runs through the agent's existing scanner
// primitives; this never bypasses server command-signing or package
// authorization paths because it cannot install anything.
func (h *handler) triggerScanAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.triggerScan == nil {
		log.Printf("[WARNING] [agent] [localapi] scan_trigger_unavailable")
		http.Error(w, "scan trigger unavailable", http.StatusServiceUnavailable)
		return
	}

	err := h.triggerScan("localapi")
	if errors.Is(err, ErrScanInFlight) {
		log.Printf("[INFO] [agent] [localapi] scan_trigger_rejected reason=in_flight")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeJSONBody(w, map[string]interface{}{"accepted": false, "error": "scan already in flight"})
		return
	}
	if err != nil {
		log.Printf("[ERROR] [agent] [localapi] scan_trigger_failed error=%v", err)
		http.Error(w, "scan trigger failed", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] [agent] [localapi] scan_trigger_accepted source=localapi")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSONBody(w, map[string]interface{}{"accepted": true})
}

// approveUpdateAction handles POST /v1/actions/approve-update — the standalone
// approval flow (FEAT-003). Authorization is the OS-local group ACL on the
// socket/pipe; the real judgment lives in the gates and the root-owned mint
// key. Synchronous: the connection is held until verdict + install complete.
func (h *handler) approveUpdateAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.approveUpdate == nil {
		log.Printf("[WARNING] [agent] [localapi] approve_unavailable")
		http.Error(w, "local approval unavailable", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "request read failed", http.StatusBadRequest)
		return
	}

	result, err := h.approveUpdate(body)
	switch {
	case err == nil:
		log.Printf("[INFO] [agent] [localapi] approve_completed")
		writeJSON(w, result)
	case errors.Is(err, ErrApprovalConflict):
		log.Printf("[SECURITY] [agent] [localapi] approve_conflict error=%v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeJSONBody(w, map[string]interface{}{"error": err.Error()})
	case errors.Is(err, ErrApprovalUnavailable):
		log.Printf("[WARNING] [agent] [localapi] approve_unavailable error=%v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		writeJSONBody(w, map[string]interface{}{"error": err.Error()})
	default:
		log.Printf("[ERROR] [agent] [localapi] approve_failed error=%v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSONBody(w, map[string]interface{}{"error": err.Error()})
	}
}

func (h *handler) load(w http.ResponseWriter) (*cache.LocalCache, bool) {
	localCache, err := h.loadCache()
	if err != nil {
		log.Printf("[ERROR] [agent] [localapi] cache_load_failed error=%v", err)
		http.Error(w, "local state unavailable", http.StatusServiceUnavailable)
		return nil, false
	}
	return localCache, true
}

func requireGet(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	writeJSONBody(w, value)
}

// writeJSONBody encodes without touching headers — for callers that already
// wrote a non-200 status code.
func writeJSONBody(w http.ResponseWriter, value interface{}) {
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("[WARNING] [agent] [localapi] response_encode_failed error=%v", err)
	}
}

func sortedCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}

func groupName(opts Options) string {
	if opts.GroupName != "" {
		return opts.GroupName
	}
	return defaultGroupName()
}

func unixSocketPath(opts Options) string {
	if opts.UnixSocketPath != "" {
		return opts.UnixSocketPath
	}
	return defaultUnixSocketPath()
}

func windowsPipeName(opts Options) string {
	if opts.WindowsPipeName != "" {
		return opts.WindowsPipeName
	}
	return DefaultWindowsPipeName
}

func formatGroupMissing(name string, err error) error {
	return fmt.Errorf("localapi: local access group %q unavailable: %w", name, err)
}
