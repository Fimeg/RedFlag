package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/service"
	"github.com/Fimeg/RedFlag/agent/internal/version"
)

// CLIFlags holds all command-line flags
type CLI struct {
	Register       bool
	Scan           bool
	Status         bool
	LocalStatus    bool
	InitStandalone bool
	ListUpdates    bool
	Version        bool
	ServerURL      string
	Token          string
	ProxyHTTP      string
	ProxyHTTPS     string
	ProxyNoProxy   string
	LogLevel       string
	ConfigFile     string
	Tags           string
	Organization   string
	DisplayName    string
	InsecureTLS    bool
	ExportFormat   string
	InstallService bool
	RemoveService  bool
	StartService   bool
	StopService    bool
	ServiceStatus  bool
}

// ParseFlags parses all command-line flags and returns the CLI struct
func ParseFlags() *CLI {
	cli := &CLI{}

	// Define CLI flags
	flag.BoolVar(&cli.Register, "register", false, "Register agent with server")
	flag.BoolVar(&cli.Scan, "scan", false, "Scan for updates and display locally")
	flag.BoolVar(&cli.Status, "status", false, "Show agent status")
	flag.BoolVar(&cli.LocalStatus, "local-status", false, "Show live local agent status over local IPC")
	flag.BoolVar(&cli.InitStandalone, "init-standalone", false, "Create or print this host's standalone Agent identity")
	flag.BoolVar(&cli.ListUpdates, "list-updates", false, "List detailed update information")
	flag.BoolVar(&cli.Version, "version", false, "Show version information")
	flag.StringVar(&cli.ServerURL, "server", "", "Server URL")
	flag.StringVar(&cli.Token, "token", "", "Registration token for secure enrollment")
	flag.StringVar(&cli.ProxyHTTP, "proxy-http", "", "HTTP proxy URL")
	flag.StringVar(&cli.ProxyHTTPS, "proxy-https", "", "HTTPS proxy URL")
	flag.StringVar(&cli.ProxyNoProxy, "proxy-no", "", "Comma-separated hosts to bypass proxy")
	flag.StringVar(&cli.LogLevel, "log-level", "", "Log level (debug, info, warn, error)")
	flag.StringVar(&cli.ConfigFile, "config", "", "Configuration file path")
	flag.StringVar(&cli.Tags, "tags", "", "Comma-separated tags for agent")
	flag.StringVar(&cli.Organization, "organization", "", "Organization/group name")
	flag.StringVar(&cli.DisplayName, "name", "", "Display name for agent")
	flag.BoolVar(&cli.InsecureTLS, "insecure-tls", false, "Skip TLS certificate verification")
	flag.StringVar(&cli.ExportFormat, "export", "", "Export format: json, csv")

	// Windows service management commands
	flag.BoolVar(&cli.InstallService, "install-service", false, "Install as Windows service")
	flag.BoolVar(&cli.RemoveService, "remove-service", false, "Remove Windows service")
	flag.BoolVar(&cli.StartService, "start-service", false, "Start Windows service")
	flag.BoolVar(&cli.StopService, "stop-service", false, "Stop Windows service")
	flag.BoolVar(&cli.ServiceStatus, "service-status", false, "Show Windows service status")

	flag.Parse()

	return cli
}

// HandleVersionCommand handles the version display command
func HandleVersionCommand() {
	fmt.Printf("RedFlag Agent v%s\n", version.Version)
	fmt.Printf("Self-hosted update management platform\n")
	os.Exit(0)
}

// HandleWindowsServiceCommands handles Windows service management commands
func HandleWindowsServiceCommands(cli *CLI) bool {
	if runtime.GOOS != "windows" {
		return false
	}

	switch {
	case cli.InstallService:
		if err := service.InstallService(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to install service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("RedFlag service installed successfully")
		os.Exit(0)

	case cli.RemoveService:
		if err := service.RemoveService(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("RedFlag service removed successfully")
		os.Exit(0)

	case cli.StartService:
		if err := service.StartService(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("RedFlag service started successfully")
		os.Exit(0)

	case cli.StopService:
		if err := service.StopService(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("RedFlag service stopped successfully")
		os.Exit(0)

	case cli.ServiceStatus:
		if err := service.ServiceStatus(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get service status: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	return false
}

// ParseTags parses tags from comma-separated string
func ParseTags(tagsStr string) []string {
	if tagsStr == "" {
		return nil
	}

	tags := strings.Split(tagsStr, ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}
	return tags
}

// ToConfigFlags converts CLI flags to config.CLIFlags
func (cli *CLI) ToConfigFlags() *config.CLIFlags {
	return &config.CLIFlags{
		ServerURL:         cli.ServerURL,
		RegistrationToken: cli.Token,
		ProxyHTTP:         cli.ProxyHTTP,
		ProxyHTTPS:        cli.ProxyHTTPS,
		ProxyNoProxy:      cli.ProxyNoProxy,
		LogLevel:          cli.LogLevel,
		ConfigFile:        cli.ConfigFile,
		Tags:              ParseTags(cli.Tags),
		Organization:      cli.Organization,
		DisplayName:       cli.DisplayName,
		InsecureTLS:       cli.InsecureTLS,
	}
}
