package kernel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/event"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

const (
	// MAGIC_NUMBER matches the eBPF program
	MAGIC_NUMBER = 0xDEADBEEF
)

// EBPFConsumer reads events from the eBPF ring buffer and forwards them to the policy checker
type EBPFConsumer struct {
	config    *config.Config
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	wgLock    sync.Mutex
	rd        *ringbuf.Reader
	elfPath   string
	teeLogger *event.TeeLogger
}

// NewEBPFConsumer creates a new eBPF ring buffer consumer
func NewEBPFConsumer(cfg *config.Config) (*EBPFConsumer, error) {
	return NewEBPFConsumerWithLogger(cfg, nil)
}

func NewEBPFConsumerWithLogger(cfg *config.Config, teeLogger *event.TeeLogger) (*EBPFConsumer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &EBPFConsumer{
		config:    cfg,
		ctx:       ctx,
		cancel:    cancel,
		elfPath:   cfg.KernelEnforcement.RingBufferPath + "/pkg-gate.o",
		teeLogger: teeLogger,
	}, nil
}

// Start begins consuming events from the eBPF ring buffer
func (e *EBPFConsumer) Start() error {
	e.wgLock.Lock()
	if e.running {
		e.wgLock.Unlock()
		return nil
	}
	e.running = true
	e.wgLock.Unlock()

	log.Printf("[INFO] [kernel] [ebpf] consumer_started")

	// Remove memlock limit for eBPF
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("failed to remove memlock: %w", err)
	}

	// Load the eBPF collection from the ELF file
	spec, err := ebpf.LoadCollectionSpec(e.elfPath)
	if err != nil {
		return fmt.Errorf("failed to load eBPF collection spec: %w", err)
	}

	// Find the ring buffer map by name
	_, ok := spec.Maps["rb_map"]
	if !ok {
		return fmt.Errorf("ring buffer map 'rb_map' not found in eBPF collection")
	}

	// Load collection to get loaded maps
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("failed to load eBPF collection: %w", err)
	}

	// Access the loaded ring buffer map
	loadedMap := coll.Maps["rb_map"]
	if loadedMap == nil {
		coll.Close()
		return fmt.Errorf("ring buffer map 'rb_map' not found in loaded collection")
	}

	// Create ring buffer reader
	rd, err := ringbuf.NewReader(loadedMap)
	if err != nil {
		coll.Close()
		return fmt.Errorf("failed to create ring buffer: %w", err)
	}

	e.rd = rd
	e.elfPath = "" // Don't load again on stop

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Printf("[INFO] [kernel] [ebpf] shutdown_signal_received")
		e.cancel()
		e.wg.Wait()
		e.rd.Close()
	}()

	e.wg.Add(1)
	go e.consumeEvents()

	return nil
}

// Stop stops the consumer
func (e *EBPFConsumer) Stop() error {
	e.cancel()
	e.wgLock.Lock()
	e.running = false
	e.wgLock.Unlock()

	e.wg.Wait()
	if e.rd != nil {
		e.rd.Close()
	}

	log.Printf("[INFO] [kernel] [ebpf] consumer_stopped")

	return nil
}

// consumeEvents reads from the ring buffer and forwards events to the policy checker
func (e *EBPFConsumer) consumeEvents() {
	defer e.wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		// Read event from ring buffer
		record, err := e.rd.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return
			}
			e.teeLogger.Error("agent", "kernel", "kernel_enforcer", fmt.Sprintf("ringbuf_read_failed error=%v", err), map[string]interface{}{"error": err.Error()})
			continue
		}

		e.processEvent(record.RawSample)
	}
}

// processEvent handles a single eBPF event
func (e *EBPFConsumer) processEvent(data []byte) {
	// Decode using the struct from eBPF (must match exactly)
	// Layout: timestamp(8) pid(4) pgid(4) uid(8) gid(8) parent_pid(4) comm(16) cmdline(256) parent_comm(16) magic(4) = 328 bytes
	event := &redflagEvent{}
	// Use unsafe to copy raw bytes into the struct
	bytess := (*[328]byte)(unsafe.Pointer(event))
	copy(bytess[0:8], data[0:8])
	copy(bytess[8:12], data[8:12])
	copy(bytess[12:16], data[12:16])
	copy(bytess[16:24], data[16:24])
	copy(bytess[24:32], data[24:32])
	copy(bytess[32:36], data[32:36])
	copy(bytess[36:52], data[36:52])
	copy(bytess[52:308], data[52:308])
	copy(bytess[308:324], data[308:324])
	copy(bytess[324:328], data[324:328])

	// Validate magic number
	if event.Magic != MAGIC_NUMBER {
		e.teeLogger.Error("agent", "kernel", "kernel_enforcer", fmt.Sprintf("invalid_magic magic=0x%x expected=0x%x", event.Magic, MAGIC_NUMBER), map[string]interface{}{
			"magic":    fmt.Sprintf("0x%x", event.Magic),
			"expected": fmt.Sprintf("0x%x", MAGIC_NUMBER),
		})
		return
	}

	// Forward to rs-helper for policy decision
	decision, reason, err := e.checkPolicy(event)
	if err != nil {
		e.teeLogger.Error("agent", "kernel", "kernel_enforcer", fmt.Sprintf("policy_check_failed error=%v", err), map[string]interface{}{"error": err.Error()})
		return
	}

	if decision == "deny" {
		e.teeLogger.Error("agent", "kernel", "kernel_enforcer", fmt.Sprintf("execve_deny pid=%d comm=%s reason=%s", event.PID, event.Comm, reason), map[string]interface{}{
			"pid":    event.PID,
			"comm":   fmt.Sprintf("%s", event.Comm),
			"reason": reason,
		})
	} else {
		log.Printf("[INFO] [kernel] [ebpf] execve_allowed pid=%d comm=%s",
			event.PID, event.Comm)
	}
}

// redflagEvent matches the eBPF struct redflag_event layout
type redflagEvent struct {
	Timestamp  uint64
	PID        uint32
	PGID       uint32
	UID        uint64
	GID        uint64
	ParentPID  uint32
	Comm       [16]byte
	Cmdline    [256]byte
	ParentComm [16]byte
	Magic      uint32
}

// checkPolicy evaluates the policy for a package manager invocation
func (e *EBPFConsumer) checkPolicy(event *redflagEvent) (string, string, error) {
	// Decode strings for policy check
	comm := *(*string)(unsafe.Pointer(&event.Comm))
	cmdline := *(*string)(unsafe.Pointer(&event.Cmdline))
	parentComm := *(*string)(unsafe.Pointer(&event.ParentComm))

	log.Printf("[DEBUG] [kernel] [ebpf] policy_check comm=%s cmdline=%s parent=%s uid=%d",
		comm, cmdline, parentComm, event.UID)

	// Check if this is a package manager command
	isPackageManager := isPackageManager(comm)
	if !isPackageManager {
		log.Printf("[INFO] [kernel] [ebpf] not_package_manager comm=%s", comm)
		return "allow", "not_package_manager", nil
	}

	// TODO: Call rs-helper via Unix socket for policy decision
	// For now, return allow (agent should have rs-helper running)

	return "allow", "policy_evaluation_pending", nil
}

// isPackageManager checks if a command is a known package manager
func isPackageManager(comm string) bool {
	pkgManagers := []string{"apt", "apt-get", "apt-cache", "dnf", "yum", "rpm-ostree",
		"npm", "pnpm", "bun", "pip", "pip3", "uv",
		"docker", "crun", "containerd"}

	for _, pm := range pkgManagers {
		if comm == pm {
			return true
		}
	}
	return false
}

// Event represents a typed intercept event for API use
type Event struct {
	Timestamp  time.Time
	PID        uint32
	PGID       uint32
	Comm       string
	Cmdline    string
	UID        uint64
	GID        uint64
	ParentPID  uint32
	ParentComm string
}

// FromBinary creates an Event from binary data (deprecated, use direct struct access)
func FromBinary(data []byte) (*Event, error) {
	if len(data) < 328 {
		return nil, fmt.Errorf("data too short: %d", len(data))
	}

	event := &redflagEvent{}
	// Use unsafe to copy raw bytes into the struct
	bytess := (*[328]byte)(unsafe.Pointer(event))
	copy(bytess[0:8], data[0:8])
	copy(bytess[8:12], data[8:12])
	copy(bytess[12:16], data[12:16])
	copy(bytess[16:24], data[16:24])
	copy(bytess[24:32], data[24:32])
	copy(bytess[32:36], data[32:36])
	copy(bytess[36:52], data[36:52])
	copy(bytess[52:308], data[52:308])
	copy(bytess[308:324], data[308:324])
	copy(bytess[324:328], data[324:328])

	return &Event{
		Timestamp:  time.Unix(0, int64(event.Timestamp)),
		PID:        event.PID,
		PGID:       event.PGID,
		Comm:       string(bytes.TrimRight(event.Comm[:], "\x00")),
		Cmdline:    string(bytes.TrimRight(event.Cmdline[:], "\x00")),
		UID:        event.UID,
		GID:        event.GID,
		ParentPID:  event.ParentPID,
		ParentComm: string(bytes.TrimRight(event.ParentComm[:], "\x00")),
	}, nil
}
