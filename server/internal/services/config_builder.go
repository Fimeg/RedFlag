package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// AgentTemplate defines a template for different agent types
type AgentTemplate struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	BaseConfig  map[string]interface{} `json:"base_config"`
	Secrets     []string               `json:"required_secrets"`
	Validation  ValidationRules       `json:"validation"`
}

// ValidationRules defines validation rules for configuration
type ValidationRules struct {
	RequiredFields []string               `json:"required_fields"`
	AllowedValues  map[string][]string    `json:"allowed_values"`
	Patterns       map[string]string      `json:"patterns"`
	Constraints    map[string]interface{} `json:"constraints"`
}

// PublicKeyResponse represents the server's public key response
type PublicKeyResponse struct {
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	Algorithm   string `json:"algorithm"`
	KeySize     int    `json:"key_size"`
}

// ConfigBuilder handles dynamic agent configuration generation
type ConfigBuilder struct {
	serverURL        string
	templates        map[string]AgentTemplate
	httpClient       *http.Client
	publicKeyCache   map[string]string
	scannerConfigQ   *queries.ScannerConfigQueries
}

// NewConfigBuilder creates a new configuration builder
func NewConfigBuilder(serverURL string, db *sqlx.DB) *ConfigBuilder {
	return &ConfigBuilder{
		serverURL: serverURL,
		templates: getAgentTemplates(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		publicKeyCache: make(map[string]string),
		scannerConfigQ: queries.NewScannerConfigQueries(db),
	}
}

// AgentSetupRequest represents a request to set up a new agent
type AgentSetupRequest struct {
	ServerURL     string                 `json:"server_url" binding:"required"`
	Environment   string                 `json:"environment" binding:"required"`
	AgentType     string                 `json:"agent_type" binding:"required,oneof=linux-server windows-workstation docker-host"`
	Organization  string                 `json:"organization" binding:"required"`
	CustomSettings map[string]interface{} `json:"custom_settings,omitempty"`
	DeploymentID  string                 `json:"deployment_id,omitempty"`
	AgentID       string                 `json:"agent_id,omitempty"` // Optional: existing agent ID for upgrades
}

// BuildAgentConfig builds a complete agent configuration
func (cb *ConfigBuilder) BuildAgentConfig(req AgentSetupRequest) (*AgentConfiguration, error) {
	// Validate request
	if err := cb.validateRequest(req); err != nil {
		return nil, err
	}

	// Determine agent ID - use existing if provided and valid, otherwise generate new
	agentID := cb.determineAgentID(req.AgentID)

	// Fetch server public key
	serverPublicKey, err := cb.fetchServerPublicKey(req.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch server public key: %w", err)
	}

	// Generate registration token
	registrationToken, err := cb.generateRegistrationToken(agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate registration token: %w", err)
	}

	// Get template
	template, exists := cb.templates[req.AgentType]
	if !exists {
		return nil, fmt.Errorf("unknown agent type: %s", req.AgentType)
	}

	// Build base configuration
	config := cb.buildFromTemplate(template, req.CustomSettings)

	// Override scanner timeouts from database (user-configurable)
	cb.overrideScannerTimeoutsFromDB(config)

	// Inject deployment-specific values
	cb.injectDeploymentValues(config, req, agentID, registrationToken, serverPublicKey)

	// Apply environment-specific defaults
	cb.applyEnvironmentDefaults(config, req.Environment)

	// Validate final configuration
	if err := cb.validateConfiguration(config, template); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Separate sensitive and non-sensitive data
	publicConfig, secrets := cb.separateSecrets(config)

	// Create Docker secrets if needed
	var secretsCreated bool
	var secretsPath string
	if len(secrets) > 0 {
		secretsManager := NewSecretsManager()

		// Generate encryption key if not set
		if secretsManager.GetEncryptionKey() == "" {
			key, err := secretsManager.GenerateEncryptionKey()
			if err != nil {
				return nil, fmt.Errorf("failed to generate encryption key: %w", err)
			}
			secretsManager.SetEncryptionKey(key)
		}

		// Create Docker secrets
		if err := secretsManager.CreateDockerSecrets(secrets); err != nil {
			return nil, fmt.Errorf("failed to create Docker secrets: %w", err)
		}

		secretsCreated = true
		secretsPath = secretsManager.GetSecretsPath()
	}

	// Determine platform from agent type
	platform := "linux-amd64" // Default
	if req.AgentType == "windows-workstation" {
		platform = "windows-amd64"
	}

	return &AgentConfiguration{
		AgentID:        agentID,
		PublicConfig:   publicConfig,
		Secrets:        secrets,
		Template:       req.AgentType,
		Environment:    req.Environment,
		ServerURL:      req.ServerURL,
		Organization:   req.Organization,
		Platform:       platform,
		ConfigVersion:  "5",         // Config schema version
		AgentVersion:   "0.1.23.6",  // Agent binary version
		BuildTime:      time.Now().UTC(),
		SecretsCreated: secretsCreated,
		SecretsPath:    secretsPath,
	}, nil
}

// AgentConfiguration represents a complete agent configuration
type AgentConfiguration struct {
	AgentID       string                 `json:"agent_id"`
	PublicConfig  map[string]interface{} `json:"public_config"`
	Secrets       map[string]string      `json:"secrets"`
	Template      string                 `json:"template"`
	Environment   string                 `json:"environment"`
	ServerURL     string                 `json:"server_url"`
	Organization  string                 `json:"organization"`
	Platform      string                 `json:"platform"`
	ConfigVersion string                 `json:"config_version"`  // Config schema version (e.g., "5")
	AgentVersion  string                 `json:"agent_version"`   // Agent binary version (e.g., "0.1.23.6")
	BuildTime     time.Time              `json:"build_time"`
	SecretsCreated bool                  `json:"secrets_created"`
	SecretsPath   string                 `json:"secrets_path,omitempty"`
}

// validateRequest validates the setup request
func (cb *ConfigBuilder) validateRequest(req AgentSetupRequest) error {
	if req.ServerURL == "" {
		return fmt.Errorf("server_url is required")
	}

	if req.Environment == "" {
		return fmt.Errorf("environment is required")
	}

	if req.AgentType == "" {
		return fmt.Errorf("agent_type is required")
	}

	if req.Organization == "" {
		return fmt.Errorf("organization is required")
	}

	// Check if agent type exists
	if _, exists := cb.templates[req.AgentType]; !exists {
		return fmt.Errorf("unknown agent type: %s", req.AgentType)
	}

	return nil
}

// fetchServerPublicKey fetches the server's public key with caching
func (cb *ConfigBuilder) fetchServerPublicKey(serverURL string) (string, error) {
	// Check cache first
	if cached, exists := cb.publicKeyCache[serverURL]; exists {
		return cached, nil
	}

	// Fetch from server
	resp, err := cb.httpClient.Get(serverURL + "/api/v1/public-key")
	if err != nil {
		return "", fmt.Errorf("failed to fetch public key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var keyResp PublicKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&keyResp); err != nil {
		return "", fmt.Errorf("failed to decode public key response: %w", err)
	}

	// Cache the key
	cb.publicKeyCache[serverURL] = keyResp.PublicKey

	return keyResp.PublicKey, nil
}

// generateRegistrationToken generates a secure registration token
func (cb *ConfigBuilder) generateRegistrationToken(agentID string) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Combine agent ID with random bytes for uniqueness
	data := append([]byte(agentID), bytes...)
	token := hex.EncodeToString(data)

	// Ensure token doesn't exceed reasonable length
	if len(token) > 128 {
		token = token[:128]
	}

	return token, nil
}

// buildFromTemplate builds configuration from template
func (cb *ConfigBuilder) buildFromTemplate(template AgentTemplate, customSettings map[string]interface{}) map[string]interface{} {
	config := make(map[string]interface{})

	// Deep copy base configuration
	for k, v := range template.BaseConfig {
		config[k] = cb.deepCopy(v)
	}

	// Apply custom settings
	if customSettings != nil {
		cb.mergeSettings(config, customSettings)
	}

	return config
}

// injectDeploymentValues injects deployment-specific values into configuration
func (cb *ConfigBuilder) injectDeploymentValues(config map[string]interface{}, req AgentSetupRequest, agentID, registrationToken, serverPublicKey string) {
	config["version"] = "5"              // Config schema version (for migration system)
	config["agent_version"] = "0.1.23.6" // Agent binary version (MUST match the binary being served)
	config["server_url"] = req.ServerURL
	config["agent_id"] = agentID
	config["registration_token"] = registrationToken
	config["server_public_key"] = serverPublicKey
	config["organization"] = req.Organization
	config["environment"] = req.Environment
	config["agent_type"] = req.AgentType

	if req.DeploymentID != "" {
		config["deployment_id"] = req.DeploymentID
	}
}

// determineAgentID checks if an existing agent ID is provided and valid, otherwise generates new
func (cb *ConfigBuilder) determineAgentID(providedAgentID string) string {
	if providedAgentID != "" {
		// Validate it's a proper UUID
		if _, err := uuid.FromString(providedAgentID); err == nil {
			return providedAgentID
		}
	}
	// Generate new UUID if none provided or invalid
	return uuid.Must(uuid.NewV4()).String()
}

// applyEnvironmentDefaults applies environment-specific configuration defaults
func (cb *ConfigBuilder) applyEnvironmentDefaults(config map[string]interface{}, environment string) {
	environmentDefaults := map[string]interface{}{
		"development": map[string]interface{}{
			"logging": map[string]interface{}{
				"level":       "debug",
				"max_size":    50,
				"max_backups": 2,
				"max_age":     7,
			},
			"check_in_interval": 60, // More frequent polling in development
		},
		"staging": map[string]interface{}{
			"logging": map[string]interface{}{
				"level":       "info",
				"max_size":    100,
				"max_backups": 3,
				"max_age":     14,
			},
			"check_in_interval": 180,
		},
		"production": map[string]interface{}{
			"logging": map[string]interface{}{
				"level":       "warn",
				"max_size":    200,
				"max_backups": 5,
				"max_age":     30,
			},
			"check_in_interval": 300, // 5 minutes for production
		},
		"testing": map[string]interface{}{
			"logging": map[string]interface{}{
				"level":       "debug",
				"max_size":    10,
				"max_backups": 1,
				"max_age":     1,
			},
			"check_in_interval": 30, // Very frequent for testing
		},
	}

	if defaults, exists := environmentDefaults[environment]; exists {
		if defaultsMap, ok := defaults.(map[string]interface{}); ok {
			cb.mergeSettings(config, defaultsMap)
		}
	}
}

// validateConfiguration validates the final configuration
func (cb *ConfigBuilder) validateConfiguration(config map[string]interface{}, template AgentTemplate) error {
	// Check required fields
	for _, field := range template.Validation.RequiredFields {
		if _, exists := config[field]; !exists {
			return fmt.Errorf("required field missing: %s", field)
		}
	}

	// Validate allowed values
	for field, allowedValues := range template.Validation.AllowedValues {
		if value, exists := config[field]; exists {
			if strValue, ok := value.(string); ok {
				if !cb.containsString(allowedValues, strValue) {
					return fmt.Errorf("invalid value for %s: %s (allowed: %v)", field, strValue, allowedValues)
				}
			}
		}
	}

	// Validate constraints
	for field, constraint := range template.Validation.Constraints {
		if value, exists := config[field]; exists {
			if err := cb.validateConstraint(field, value, constraint); err != nil {
				return err
			}
		}
	}

	return nil
}

// separateSecrets separates sensitive data from public configuration
func (cb *ConfigBuilder) separateSecrets(config map[string]interface{}) (map[string]interface{}, map[string]string) {
	publicConfig := make(map[string]interface{})
	secrets := make(map[string]string)

	// Copy all values to public config initially
	for k, v := range config {
		publicConfig[k] = cb.deepCopy(v)
	}

	// Extract known sensitive fields
	sensitiveFields := []string{
		"registration_token",
		"server_public_key",
	}

	for _, field := range sensitiveFields {
		if value, exists := publicConfig[field]; exists {
			if strValue, ok := value.(string); ok {
				secrets[field] = strValue
				delete(publicConfig, field)
			}
		}
	}

	// Extract nested sensitive fields
	if proxy, exists := publicConfig["proxy"].(map[string]interface{}); exists {
		if username, exists := proxy["username"].(string); exists && username != "" {
			secrets["proxy_username"] = username
			delete(proxy, "username")
		}
		if password, exists := proxy["password"].(string); exists && password != "" {
			secrets["proxy_password"] = password
			delete(proxy, "password")
		}
	}

	if tls, exists := publicConfig["tls"].(map[string]interface{}); exists {
		if certFile, exists := tls["cert_file"].(string); exists && certFile != "" {
			secrets["tls_cert"] = certFile
			delete(tls, "cert_file")
		}
		if keyFile, exists := tls["key_file"].(string); exists && keyFile != "" {
			secrets["tls_key"] = keyFile
			delete(tls, "key_file")
		}
		if caFile, exists := tls["ca_file"].(string); exists && caFile != "" {
			secrets["tls_ca"] = caFile
			delete(tls, "ca_file")
		}
	}

	return publicConfig, secrets
}

// Helper functions

func (cb *ConfigBuilder) deepCopy(value interface{}) interface{} {
	if m, ok := value.(map[string]interface{}); ok {
		result := make(map[string]interface{})
		for k, v := range m {
			result[k] = cb.deepCopy(v)
		}
		return result
	}
	if s, ok := value.([]interface{}); ok {
		result := make([]interface{}, len(s))
		for i, v := range s {
			result[i] = cb.deepCopy(v)
		}
		return result
	}
	return value
}

func (cb *ConfigBuilder) mergeSettings(target map[string]interface{}, source map[string]interface{}) {
	for key, value := range source {
		if existing, exists := target[key]; exists {
			if existingMap, ok := existing.(map[string]interface{}); ok {
				if sourceMap, ok := value.(map[string]interface{}); ok {
					cb.mergeSettings(existingMap, sourceMap)
					continue
				}
			}
		}
		target[key] = cb.deepCopy(value)
	}
}

func (cb *ConfigBuilder) containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetTemplates returns the available agent templates
func (cb *ConfigBuilder) GetTemplates() map[string]AgentTemplate {
	return getAgentTemplates()
}

// GetTemplate returns a specific agent template
func (cb *ConfigBuilder) GetTemplate(agentType string) (AgentTemplate, bool) {
	template, exists := getAgentTemplates()[agentType]
	return template, exists
}

func (cb *ConfigBuilder) validateConstraint(field string, value interface{}, constraint interface{}) error {
	constraints, ok := constraint.(map[string]interface{})
	if !ok {
		return nil
	}

	if numValue, ok := value.(float64); ok {
		if min, exists := constraints["min"].(float64); exists && numValue < min {
			return fmt.Errorf("value for %s is below minimum: %f < %f", field, numValue, min)
		}
		if max, exists := constraints["max"].(float64); exists && numValue > max {
			return fmt.Errorf("value for %s is above maximum: %f > %f", field, numValue, max)
		}
	}

	return nil
}

// getAgentTemplates returns the available agent templates
// overrideScannerTimeoutsFromDB overrides scanner timeouts with values from database
// This allows users to configure scanner timeouts via the web UI
func (cb *ConfigBuilder) overrideScannerTimeoutsFromDB(config map[string]interface{}) {
	if cb.scannerConfigQ == nil {
		// No database connection, use defaults
		return
	}

	// Get subsystems section
	subsystems, exists := config["subsystems"].(map[string]interface{})
	if !exists {
		return
	}

	// List of scanners that can have configurable timeouts
	scannerNames := []string{"apt", "dnf", "docker", "windows", "winget", "system", "storage"}

	for _, scannerName := range scannerNames {
		scannerConfig, exists := subsystems[scannerName].(map[string]interface{})
		if !exists {
			continue
		}

		// Get timeout from database
		timeout := cb.scannerConfigQ.GetScannerTimeoutWithDefault(scannerName, 30*time.Minute)
		scannerConfig["timeout"] = int(timeout.Nanoseconds())
	}
}

func getAgentTemplates() map[string]AgentTemplate {
	return map[string]AgentTemplate{
		"linux-server": {
			Name:        "Linux Server Agent",
			Description: "Optimized for Linux server deployments with package management",
			BaseConfig: map[string]interface{}{
				"check_in_interval": 300,
				"network": map[string]interface{}{
					"timeout":       30000000000,
					"retry_count":   3,
					"retry_delay":   5000000000,
					"max_idle_conn": 10,
				},
				"proxy": map[string]interface{}{
					"enabled": false,
				},
				"tls": map[string]interface{}{
					"insecure_skip_verify": false,
				},
				"logging": map[string]interface{}{
					"level":       "info",
					"max_size":    100,
					"max_backups": 3,
					"max_age":     28,
				},
				"subsystems": map[string]interface{}{
					"apt": map[string]interface{}{
						"enabled": true,
						"timeout": 30000000000,
						"circuit_breaker": map[string]interface{}{
							"enabled":           true,
							"failure_threshold": 3,
							"failure_window":    600000000000,
							"open_duration":     1800000000000,
							"half_open_attempts": 2,
						},
					},
					"dnf": map[string]interface{}{
						"enabled": true,
						"timeout": 1800000000000, // 30 minutes - configurable via server settings
						"circuit_breaker": map[string]interface{}{
							"enabled":           true,
							"failure_threshold": 3,
							"failure_window":    600000000000,
							"open_duration":     1800000000000,
							"half_open_attempts": 2,
						},
					},
					"docker": map[string]interface{}{
						"enabled": true,
						"timeout": 60000000000,
						"circuit_breaker": map[string]interface{}{
							"enabled":           true,
							"failure_threshold": 3,
							"failure_window":    600000000000,
							"open_duration":     1800000000000,
							"half_open_attempts": 2,
						},
					},
					"windows": map[string]interface{}{
						"enabled": false,
					},
					"winget": map[string]interface{}{
						"enabled": false,
					},
					"storage": map[string]interface{}{
						"enabled": true,
						"timeout": 10000000000,
						"circuit_breaker": map[string]interface{}{
							"enabled":           true,
							"failure_threshold": 3,
							"failure_window":    600000000000,
							"open_duration":     1800000000000,
							"half_open_attempts": 2,
						},
					},
				},
			},
			Secrets: []string{"registration_token", "server_public_key"},
			Validation: ValidationRules{
				RequiredFields: []string{"server_url", "organization"},
				AllowedValues: map[string][]string{
					"environment": {"development", "staging", "production", "testing"},
				},
				Patterns: map[string]string{
					"server_url": "^https?://.+",
				},
				Constraints: map[string]interface{}{
					"check_in_interval": map[string]interface{}{"min": 30, "max": 3600},
				},
			},
		},
		"windows-workstation": {
			Name:        "Windows Workstation Agent",
			Description: "Optimized for Windows workstation deployments",
			BaseConfig: map[string]interface{}{
				"check_in_interval": 300,
				"network": map[string]interface{}{
					"timeout":       30000000000,
					"retry_count":   3,
					"retry_delay":   5000000000,
					"max_idle_conn": 10,
				},
				"proxy": map[string]interface{}{
					"enabled": false,
				},
				"tls": map[string]interface{}{
					"insecure_skip_verify": false,
				},
				"logging": map[string]interface{}{
					"level":       "info",
					"max_size":    100,
					"max_backups": 3,
					"max_age":     28,
				},
				"subsystems": map[string]interface{}{
					"apt": map[string]interface{}{
						"enabled": false,
					},
					"dnf": map[string]interface{}{
						"enabled": false,
					},
					"docker": map[string]interface{}{
						"enabled": false,
					},
					"windows": map[string]interface{}{
						"enabled": true,
						"timeout": 600000000000,
						"circuit_breaker": map[string]interface{}{
							"enabled":           true,
							"failure_threshold": 2,
							"failure_window":    900000000000,
							"open_duration":     3600000000000,
							"half_open_attempts": 3,
						},
					},
					"winget": map[string]interface{}{
						"enabled": true,
						"timeout": 120000000000,
						"circuit_breaker": map[string]interface{}{
							"enabled":           true,
							"failure_threshold": 3,
							"failure_window":    600000000000,
							"open_duration":     1800000000000,
							"half_open_attempts": 2,
						},
					},
					"storage": map[string]interface{}{
						"enabled": false,
					},
				},
			},
			Secrets: []string{"registration_token", "server_public_key"},
			Validation: ValidationRules{
				RequiredFields: []string{"server_url", "organization"},
				AllowedValues: map[string][]string{
					"environment": {"development", "staging", "production", "testing"},
				},
				Patterns: map[string]string{
					"server_url": "^https?://.+",
				},
				Constraints: map[string]interface{}{
					"check_in_interval": map[string]interface{}{"min": 30, "max": 3600},
				},
			},
		},
		"docker-host": {
			Name:        "Docker Host Agent",
			Description: "Optimized for Docker host deployments",
			BaseConfig: map[string]interface{}{
				"check_in_interval": 300,
				"network": map[string]interface{}{
					"timeout":       30000000000,
					"retry_count":   3,
					"retry_delay":   5000000000,
					"max_idle_conn": 10,
				},
				"proxy": map[string]interface{}{
					"enabled": false,
				},
				"tls": map[string]interface{}{
					"insecure_skip_verify": false,
				},
				"logging": map[string]interface{}{
					"level":       "info",
					"max_size":    100,
					"max_backups": 3,
					"max_age":     28,
				},
				"subsystems": map[string]interface{}{
					"apt": map[string]interface{}{
						"enabled": false,
					},
					"dnf": map[string]interface{}{
						"enabled": false,
					},
					"docker": map[string]interface{}{
						"enabled": true,
						"timeout": 60000000000,
						"circuit_breaker": map[string]interface{}{
							"enabled":           true,
							"failure_threshold": 3,
							"failure_window":    600000000000,
							"open_duration":     1800000000000,
							"half_open_attempts": 2,
						},
					},
					"windows": map[string]interface{}{
						"enabled": false,
					},
					"winget": map[string]interface{}{
						"enabled": false,
					},
					"storage": map[string]interface{}{
						"enabled": false,
					},
				},
			},
			Secrets: []string{"registration_token", "server_public_key"},
			Validation: ValidationRules{
				RequiredFields: []string{"server_url", "organization"},
				AllowedValues: map[string][]string{
					"environment": {"development", "staging", "production", "testing"},
				},
				Patterns: map[string]string{
					"server_url": "^https?://.+",
				},
				Constraints: map[string]interface{}{
					"check_in_interval": map[string]interface{}{"min": 30, "max": 3600},
				},
			},
		},
	}
}
