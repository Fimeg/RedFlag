package kernel

import (
	"context"
	"log"

	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/event"
)

type Enforcer interface {
	Start(ctx context.Context) error
	Stop() error
	GetPackageType() string
}

type EnforcerFactory func(*config.Config, *event.TeeLogger) (Enforcer, error)

var enforcers = map[string]EnforcerFactory{}

func RegisterEnforcer(pkgType string, factory EnforcerFactory) {
	enforcers[pkgType] = factory
}

func NewEnforcerWithLogger(cfg *config.Config, teeLogger *event.TeeLogger) (Enforcer, error) {
	if !cfg.KernelEnforcement.Enabled {
		return &noopEnforcer{}, nil
	}

	// Determine package type based on platform
	pkgType := "linux"
	if cfg.OS.Type == "windows" {
		pkgType = "windows"
	}

	if factory, ok := enforcers[pkgType]; ok {
		return factory(cfg, teeLogger)
	}

	log.Printf("[WARNING] [kernel] unknown_package_type=%s", pkgType)
	return &noopEnforcer{}, nil
}

type noopEnforcer struct{}

func (n *noopEnforcer) Start(ctx context.Context) error { return nil }
func (n *noopEnforcer) Stop() error                     { return nil }
func (n *noopEnforcer) GetPackageType() string          { return "noop" }

type EBPFEnforcer struct {
	consumer *EBPFConsumer
}

func NewEBPFEnforcer(cfg *config.Config) (*EBPFEnforcer, error) {
	return NewEBPFEnforcerWithLogger(cfg, nil)
}

func NewEBPFEnforcerWithLogger(cfg *config.Config, teeLogger *event.TeeLogger) (*EBPFEnforcer, error) {
	consumer, err := NewEBPFConsumerWithLogger(cfg, teeLogger)
	if err != nil {
		return nil, err
	}
	return &EBPFEnforcer{consumer: consumer}, nil
}

func (e *EBPFEnforcer) Start(ctx context.Context) error {
	return e.consumer.Start()
}

func (e *EBPFEnforcer) Stop() error {
	return e.consumer.Stop()
}

func (e *EBPFEnforcer) GetPackageType() string { return "linux" }

func init() {
	RegisterEnforcer("linux", func(cfg *config.Config, teeLogger *event.TeeLogger) (Enforcer, error) {
		consumer, err := NewEBPFConsumerWithLogger(cfg, teeLogger)
		if err != nil {
			return nil, err
		}
		return &EBPFEnforcer{consumer: consumer}, nil
	})
}
