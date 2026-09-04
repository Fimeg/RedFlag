package middleware

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// MachineBindingMiddleware validates machine ID matches database record
// This prevents agent impersonation via config file copying to different machines
func MachineBindingMiddleware(agentQueries *queries.AgentQueries, minAgentVersion string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if not authenticated (handled by auth middleware)
		agentIDVal, exists := c.Get("agent_id")
		if !exists {
			c.Next()
			return
		}

		agentID, ok := agentIDVal.(uuid.UUID)
		if !ok {
			log.Printf("[MachineBinding] Invalid agent_id type in context")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid agent ID"})
			c.Abort()
			return
		}

		// Get agent from database
		agent, err := agentQueries.GetAgentByID(agentID)
		if err != nil {
			log.Printf("[MachineBinding] Agent %s not found: %v", agentID, err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "agent not found"})
			c.Abort()
			return
		}

		// Check if agent is reporting an update completion
		reportedVersion := c.GetHeader("X-Agent-Version")
		updateNonce := c.GetHeader("X-Update-Nonce")

		if agent.IsUpdating && updateNonce != "" {
			// Validate the nonce first (proves server authorized this update)
			if agent.PublicKeyFingerprint == nil {
				log.Printf("[SECURITY] Agent %s has no public key fingerprint for nonce validation", agentID)
				c.JSON(http.StatusForbidden, gin.H{"error": "server public key not configured"})
				c.Abort()
				return
			}
			if err := validateUpdateNonceMiddleware(updateNonce, *agent.PublicKeyFingerprint); err != nil {
				log.Printf("[SECURITY] Invalid update nonce for agent %s: %v", agentID, err)
				c.JSON(http.StatusForbidden, gin.H{"error": "invalid update nonce"})
				c.Abort()
				return
			}

			// Check for downgrade attempt (security boundary)
			if !isVersionUpgrade(reportedVersion, agent.CurrentVersion) {
				log.Printf("[SECURITY] Downgrade attempt detected: agent %s %s → %s",
					agentID, agent.CurrentVersion, reportedVersion)
				c.JSON(http.StatusForbidden, gin.H{"error": "downgrade not allowed"})
				c.Abort()
				return
			}

			// Valid upgrade - complete it in database
			go func() {
				if err := agentQueries.CompleteAgentUpdate(agentID.String(), reportedVersion); err != nil {
					log.Printf("[ERROR] Failed to complete agent update: %v", err)
				} else {
					log.Printf("[system] Agent %s updated: %s → %s", agentID, agent.CurrentVersion, reportedVersion)
				}
			}()

			// Allow this request through
			c.Next()
			return
		}

		// Check minimum version (hard cutoff for legacy de-support)
		if agent.CurrentVersion != "" && minAgentVersion != "" {
			if !utils.IsNewerOrEqualVersion(agent.CurrentVersion, minAgentVersion) {
				// Allow old agents to check in if they have pending update commands
				// This prevents deadlock where agent can't check in to receive the update
				if c.Request.Method == "GET" && strings.HasSuffix(c.Request.URL.Path, "/commands") {
					// Check if agent has pending update command
					hasPendingUpdate, err := agentQueries.HasPendingUpdateCommand(agentID.String())
					if err != nil {
						log.Printf("[MachineBinding] Error checking pending updates for agent %s: %v", agentID, err)
					}

					if hasPendingUpdate {
						log.Printf("[MachineBinding] Allowing old agent %s (%s) to check in for update delivery (v%s < v%s)",
							agent.Hostname, agentID, agent.CurrentVersion, minAgentVersion)
						c.Next()
						return
					}
				}

				log.Printf("[MachineBinding] Agent %s version %s below minimum %s - rejecting",
					agent.Hostname, agent.CurrentVersion, minAgentVersion)
				c.JSON(http.StatusUpgradeRequired, gin.H{
					"error":           "agent version too old - upgrade required for security",
					"current_version": agent.CurrentVersion,
					"minimum_version": minAgentVersion,
					"upgrade_instructions": "Please upgrade to the latest agent version and re-register",
				})
				c.Abort()
				return
			}
		}

		// Extract X-Machine-ID header
		reportedMachineID := c.GetHeader("X-Machine-ID")
		if reportedMachineID == "" {
			log.Printf("[MachineBinding] Agent %s (%s) missing X-Machine-ID header",
				agent.Hostname, agentID)
			c.JSON(http.StatusForbidden, gin.H{
				"error": "missing machine ID header - agent version too old or tampered",
				"hint":  "Please upgrade to the latest agent version (v0.1.22+)",
			})
			c.Abort()
			return
		}

		// Validate machine ID matches database
		if agent.MachineID == nil {
			log.Printf("[MachineBinding] Agent %s (%s) has no machine_id in database - legacy agent",
				agent.Hostname, agentID)
			c.JSON(http.StatusForbidden, gin.H{
				"error": "agent not bound to machine - re-registration required",
				"hint":  "This agent was registered before v0.1.22. Please re-register with a new registration token.",
			})
			c.Abort()
			return
		}

		if *agent.MachineID != reportedMachineID {
			log.Printf("[WARNING] [server] [auth] machine_id_mismatch agent=%s (%s) db=%s reported=%s",
				agent.Hostname, agentID, *agent.MachineID, reportedMachineID)
			c.JSON(http.StatusForbidden, gin.H{
				"error":       "machine ID mismatch - config file copied to different machine",
				"hint":        "Agent configuration is bound to the original machine. Please register this machine with a new registration token.",
				"security_note": "This prevents agent impersonation attacks",
			})
			c.Abort()
			return
		}

		// Machine ID validated - allow request
		log.Printf("[INFO] [server] [auth] machine_id_validated agent=%s (%s) id=%s",
			agent.Hostname, agentID, reportedMachineID[:16]+"...")
		c.Next()
	}
}

func validateUpdateNonceMiddleware(nonceB64, serverPublicKey string) error {
	// Decode base64 nonce
	data, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return fmt.Errorf("invalid base64: %w", err)
	}

	// Parse JSON
	var nonce struct {
		AgentID       string `json:"agent_id"`
		TargetVersion string `json:"target_version"`
		Timestamp     int64  `json:"timestamp"`
		Signature     string `json:"signature"`
	}
	if err := json.Unmarshal(data, &nonce); err != nil {
		return fmt.Errorf("invalid format: %w", err)
	}

	// Check freshness
	if time.Now().UTC().Unix()-nonce.Timestamp > 600 { // 10 minutes
		return fmt.Errorf("nonce expired (age: %d seconds)", time.Now().UTC().Unix()-nonce.Timestamp)
	}

	// Verify signature
	signature, err := base64.StdEncoding.DecodeString(nonce.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Parse server's public key
	pubKeyBytes, err := hex.DecodeString(serverPublicKey)
	if err != nil {
		return fmt.Errorf("invalid server public key: %w", err)
	}

	// Remove signature for verification
	originalSig := nonce.Signature
	nonce.Signature = ""
	verifyData, err := json.Marshal(nonce)
	if err != nil {
		return fmt.Errorf("marshal verify data: %w", err)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), verifyData, signature) {
		return fmt.Errorf("signature verification failed")
	}

	// Restore signature (not needed but good practice)
	nonce.Signature = originalSig
	return nil
}

// isVersionUpgrade returns true when `new` is strictly newer than `current`.
// Uses utils.IsNewerVersion, which is the project's semver-aware comparator (handles
// arbitrary part counts including 4-part versions like 0.2.0.1). The prior
// hand-rolled implementation panicked on <3 parts and silently dropped parts beyond
// index 2, false-rejecting legitimate upgrades.
func isVersionUpgrade(new, current string) bool {
	// Reject empty inputs explicitly — nothing is an upgrade from missing data.
	if new == "" || current == "" {
		return false
	}
	// Also tolerate a leading "v" by stripping it before comparison; the rest of
	// the codebase sometimes carries it on tag-derived strings.
	return utils.IsNewerVersion(strings.TrimPrefix(new, "v"), strings.TrimPrefix(current, "v"))
}
