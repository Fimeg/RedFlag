package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// SetupHandler handles server configuration
type SetupHandler struct {
	configPath string
}

func NewSetupHandler(configPath string) *SetupHandler {
	return &SetupHandler{
		configPath: configPath,
	}
}

// ShowSetupPage displays the web setup interface
func (h *SetupHandler) ShowSetupPage(c *gin.Context) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>RedFlag - Server Configuration</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); min-height: 100vh; }
        .container { max-width: 800px; margin: 0 auto; padding: 40px 20px; }
        .card { background: white; border-radius: 12px; box-shadow: 0 10px 30px rgba(0,0,0,0.2); overflow: hidden; }
        .header { background: linear-gradient(135deg, #4f46e5 0%, #7c3aed 100%); color: white; padding: 30px; text-align: center; }
        .content { padding: 40px; }
        h1 { margin: 0; font-size: 2.5rem; font-weight: 700; }
        .subtitle { margin: 10px 0 0 0; opacity: 0.9; font-size: 1.1rem; }
        .form-section { margin: 30px 0; }
        .form-section h3 { color: #4f46e5; margin-bottom: 15px; font-size: 1.2rem; }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 5px; font-weight: 500; color: #374151; }
        input, select { width: 100%; padding: 12px; border: 2px solid #e5e7eb; border-radius: 6px; font-size: 1rem; transition: border-color 0.3s; }
        input:focus, select:focus { outline: none; border-color: #4f46e5; box-shadow: 0 0 0 3px rgba(79, 70, 229, 0.1); }
        input[type="password"] { font-family: monospace; }
        .button { background: linear-gradient(135deg, #4f46e5 0%, #7c3aed 100%); color: white; border: none; padding: 14px 28px; border-radius: 6px; font-size: 1rem; font-weight: 600; cursor: pointer; transition: transform 0.2s; }
        .button:hover { transform: translateY(-1px); }
        .button:active { transform: translateY(0); }
        .progress { background: #f3f4f6; border-radius: 6px; height: 8px; overflow: hidden; margin: 20px 0; }
        .progress-bar { background: linear-gradient(90deg, #4f46e5, #7c3aed); height: 100%; width: 0%; transition: width 0.3s; }
        .status { text-align: center; padding: 20px; display: none; }
        .error { background: #fef2f2; color: #dc2626; padding: 15px; border-radius: 6px; margin: 20px 0; border: 1px solid #fecaca; }
        .success { background: #f0fdf4; color: #16a34a; padding: 15px; border-radius: 6px; margin: 20px 0; border: 1px solid #bbf7d0; }
        .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
        @media (max-width: 768px) { .grid { grid-template-columns: 1fr; } }
    </style>
</head>
<body>
    <div class="container">
        <div class="card">
            <div class="header">
                <h1>🚀 RedFlag Server Setup</h1>
                <p class="subtitle">Configure your update management server</p>
            </div>
            <div class="content">
                <form id="setupForm">
                    <div class="form-section">
                        <h3>🔐 Admin Account</h3>
                        <div class="grid">
                            <div class="form-group">
                                <label for="adminUser">Admin Username</label>
                                <input type="text" id="adminUser" name="adminUser" value="admin" required>
                            </div>
                            <div class="form-group">
                                <label for="adminPassword">Admin Password</label>
                                <input type="password" id="adminPassword" name="adminPassword" required>
                            </div>
                        </div>
                    </div>

                    <div class="form-section">
                        <h3>💾 Database Configuration</h3>
                        <div class="grid">
                            <div class="form-group">
                                <label for="dbHost">Database Host</label>
                                <input type="text" id="dbHost" name="dbHost" value="postgres" required>
                            </div>
                            <div class="form-group">
                                <label for="dbPort">Database Port</label>
                                <input type="number" id="dbPort" name="dbPort" value="5432" required>
                            </div>
                            <div class="form-group">
                                <label for="dbName">Database Name</label>
                                <input type="text" id="dbName" name="dbName" value="redflag" required>
                            </div>
                            <div class="form-group">
                                <label for="dbUser">Database User</label>
                                <input type="text" id="dbUser" name="dbUser" value="redflag" required>
                            </div>
                            <div class="form-group">
                                <label for="dbPassword">Database Password</label>
                                <input type="password" id="dbPassword" name="dbPassword" value="redflag" required>
                            </div>
                        </div>
                    </div>

                    <div class="form-section">
                        <h3>🌐 Server Configuration</h3>
                        <div class="grid">
                            <div class="form-group">
                                <label for="serverHost">Server Host</label>
                                <input type="text" id="serverHost" name="serverHost" value="0.0.0.0" required>
                            </div>
                            <div class="form-group">
                                <label for="serverPort">Server Port</label>
                                <input type="number" id="serverPort" name="serverPort" value="8080" required>
                            </div>
                        </div>
                        <div class="form-group">
                            <label for="maxSeats">Maximum Agent Seats</label>
                            <input type="number" id="maxSeats" name="maxSeats" value="50" min="1" max="1000">
                        </div>
                    </div>

                    <div class="progress" id="progress" style="display: none;">
                        <div class="progress-bar" id="progressBar"></div>
                    </div>

                    <div id="status" class="status"></div>

                    <button type="submit" class="button">Configure Server</button>
                </form>
            </div>
        </div>
    </div>

    <script>
        document.getElementById('setupForm').addEventListener('submit', async function(e) {
            e.preventDefault();

            const formData = new FormData(e.target);
            const data = Object.fromEntries(formData.entries());

            const progress = document.getElementById('progress');
            const progressBar = document.getElementById('progressBar');
            const status = document.getElementById('status');
            const submitButton = e.target.querySelector('button[type="submit"]');

            // Show progress and disable button
            progress.style.display = 'block';
            submitButton.disabled = true;
            submitButton.textContent = 'Configuring...';

            try {
                const response = await fetch('/api/v1/setup', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify(data)
                });

                const result = await response.json();

                if (response.ok) {
                    // Success
                    progressBar.style.width = '100%';
                    status.innerHTML = '<div class="success">✅ ' + result.message + '</div>';
                    submitButton.textContent = 'Configuration Complete';

                    // Redirect to admin interface after delay
                    setTimeout(() => {
                        window.location.href = '/admin';
                    }, 3000);
                } else {
                    // Error
                    status.innerHTML = '<div class="error">❌ ' + result.error + '</div>';
                    submitButton.disabled = false;
                    submitButton.textContent = 'Configure Server';
                }
            } catch (error) {
                status.innerHTML = '<div class="error">❌ Network error: ' + error.message + '</div>';
                submitButton.disabled = false;
                submitButton.textContent = 'Configure Server';
            }
        });
    </script>
</body>
</html>`
	c.Data(200, "text/html; charset=utf-8", []byte(html))
}

// ConfigureServer handles the configuration submission
func (h *SetupHandler) ConfigureServer(c *gin.Context) {
	var req struct {
		AdminUser    string `json:"adminUser"`
		AdminPass    string `json:"adminPassword"`
		DBHost       string `json:"dbHost"`
		DBPort       string `json:"dbPort"`
		DBName       string `json:"dbName"`
		DBUser       string `json:"dbUser"`
		DBPassword   string `json:"dbPassword"`
		ServerHost   string `json:"serverHost"`
		ServerPort   string `json:"serverPort"`
		MaxSeats     string `json:"maxSeats"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Validate inputs
	if req.AdminUser == "" || req.AdminPass == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Admin username and password are required"})
		return
	}

	if req.DBHost == "" || req.DBPort == "" || req.DBName == "" || req.DBUser == "" || req.DBPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "All database fields are required"})
		return
	}

	// Parse numeric values
	dbPort, err := strconv.Atoi(req.DBPort)
	if err != nil || dbPort <= 0 || dbPort > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid database port"})
		return
	}

	serverPort, err := strconv.Atoi(req.ServerPort)
	if err != nil || serverPort <= 0 || serverPort > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server port"})
		return
	}

	maxSeats, err := strconv.Atoi(req.MaxSeats)
	if err != nil || maxSeats <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid maximum agent seats"})
		return
	}

	// Create configuration content
	envContent := fmt.Sprintf(`# RedFlag Server Configuration
# Generated by web setup

# Server Configuration
REDFLAG_SERVER_HOST=%s
REDFLAG_SERVER_PORT=%d
REDFLAG_TLS_ENABLED=false
# REDFLAG_TLS_CERT_FILE=
# REDFLAG_TLS_KEY_FILE=

# Database Configuration
REDFLAG_DB_HOST=%s
REDFLAG_DB_PORT=%d
REDFLAG_DB_NAME=%s
REDFLAG_DB_USER=%s
REDFLAG_DB_PASSWORD=%s

# Admin Configuration
REDFLAG_ADMIN_USER=%s
REDFLAG_ADMIN_PASSWORD=%s
REDFLAG_JWT_SECRET=%s

# Agent Registration
REDFLAG_TOKEN_EXPIRY=24h
REDFLAG_MAX_TOKENS=100
REDFLAG_MAX_SEATS=%d

# Legacy Configuration (for backwards compatibility)
SERVER_PORT=%d
DATABASE_URL=postgres://%s:%s@%s:%d/%s?sslmode=disable
JWT_SECRET=%s
CHECK_IN_INTERVAL=300
OFFLINE_THRESHOLD=600
TIMEZONE=UTC
LATEST_AGENT_VERSION=0.1.16`,
		req.ServerHost, serverPort,
		req.DBHost, dbPort, req.DBName, req.DBUser, req.DBPassword,
		req.AdminUser, req.AdminPass, deriveJWTSecret(req.AdminUser, req.AdminPass),
		maxSeats,
		serverPort, req.DBUser, req.DBPassword, req.DBHost, dbPort, req.DBName, deriveJWTSecret(req.AdminUser, req.AdminPass))

	// Write configuration to persistent location
	configDir := "/app/config"
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Printf("Failed to create config directory: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create config directory: %v", err)})
		return
	}

	envPath := filepath.Join(configDir, ".env")
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		fmt.Printf("Failed to save configuration: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save configuration: %v", err)})
		return
	}

	// Trigger graceful server restart after configuration
	go func() {
		time.Sleep(2 * time.Second) // Give response time to reach client

		// Get the current executable path
		execPath, err := os.Executable()
		if err != nil {
			fmt.Printf("Failed to get executable path: %v\n", err)
			return
		}

		// Restart the server with the same executable
		cmd := exec.Command(execPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		// Start the new process
		if err := cmd.Start(); err != nil {
			fmt.Printf("Failed to start new server process: %v\n", err)
			return
		}

		// Exit the current process gracefully
		fmt.Printf("Server restarting... PID: %d\n", cmd.Process.Pid)
		os.Exit(0)
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "Configuration saved successfully! Server will restart automatically.",
		"configPath": envPath,
		"restart": true,
	})
}

// deriveJWTSecret generates a JWT secret from admin credentials
func deriveJWTSecret(username, password string) string {
	hash := sha256.Sum256([]byte(username + password + "redflag-jwt-2024"))
	return hex.EncodeToString(hash[:])
}