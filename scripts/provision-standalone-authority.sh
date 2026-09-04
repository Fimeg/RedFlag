#!/bin/bash
# provision-standalone-authority.sh — set up the local mint authority on a
# STANDALONE host (no fleet server). Design: RAF/security/06-standalone-authority.md.
# Build tracking: docs/tasks/FEAT-003-standalone-local-authority.md.
#
# Run as root, after the base agent install (agent user, redflag-local group,
# helper binary, helper sudoers). The future native installers (.rpm/.deb/AUR)
# call this for standalone installs; fleet installs must NOT run it — fleet
# hosts have no local authority. The helper has a key-retirement primitive, but
# the full standalone-to-fleet transition is not implemented; the Agent refuses
# registration while local standalone identity is active.
#
# Idempotent: safe to re-run. The key init refuses to overwrite an existing
# authority by design (retire first).

set -euo pipefail

AGENT_USER="redflag-agent"
LOCAL_GROUP="redflag-local"
AGENT_BIN="/usr/local/bin/redflag-agent"
HELPER_BIN="/usr/local/bin/redflag-helper"
AGENT_CONFIG="/etc/redflag/agent/config.json"
AGENT_ID_FILE="/etc/redflag/agent_id"
JOURNAL_DIR="/var/lib/redflag/journal"
MINT_REQUEST_DIR="/var/lib/redflag/agent/mint"
TOKENS_DIR="/var/lib/redflag/agent/tokens"
MUTATION_REQUEST_DIR="/var/lib/redflag/agent/mutation-requests"
MUTATION_ENVELOPE_DIR="/var/lib/redflag/agent/mutation-envelopes"
MUTATION_RECEIPT_DIR="/var/lib/redflag/agent/mutation-receipts"
SUDOERS_FILE="/etc/sudoers.d/redflag-agent-mint"

log() { echo "[INFO] [provision] [standalone-authority] $*"; }
fail() { echo "[ERROR] [provision] [standalone-authority] $*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "must run as root"
[ -x "$AGENT_BIN" ] || fail "agent binary missing at $AGENT_BIN — run the base install first"
[ -x "$HELPER_BIN" ] || fail "helper binary missing at $HELPER_BIN — run the base install first"
[ -f "$AGENT_CONFIG" ] || fail "agent config missing at $AGENT_CONFIG — run the base install first"
id "$AGENT_USER" &>/dev/null || fail "agent user $AGENT_USER missing — run the base install first"
getent group "$LOCAL_GROUP" &>/dev/null || fail "group $LOCAL_GROUP missing — run the base install first"

# A standalone host still needs one stable identity for signed target binding.
# The Agent creates it once in config; this root provisioning step copies the
# same UUID into the helper's independent bind file.
STANDALONE_ID="$(runuser -u "$AGENT_USER" -- "$AGENT_BIN" --config "$AGENT_CONFIG" --init-standalone)" \
    || fail "standalone Agent identity initialization failed"
if [[ ! "$STANDALONE_ID" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]]; then
    fail "standalone Agent returned an invalid identity"
fi
printf '%s' "$STANDALONE_ID" > "$AGENT_ID_FILE"
chown root:root "$AGENT_ID_FILE"
chmod 0644 "$AGENT_ID_FILE"
log "standalone identity bound: $STANDALONE_ID"

# Journal dir: root-owned, setgid redflag-local, group-readable. Files the
# helper writes 0640 inherit the group via setgid, so the unprivileged agent
# (a group member) can serve journal entries over the local API while the dir
# stays root-writable only.
install -d -m 2750 -o root -g "$LOCAL_GROUP" "$JOURNAL_DIR"
log "journal dir ready: $JOURNAL_DIR (root:$LOCAL_GROUP 2750)"

# Mint request dir: agent writes requests, root (helper) reads them.
install -d -m 0750 -o "$AGENT_USER" -g "$LOCAL_GROUP" "$MINT_REQUEST_DIR"
log "mint request dir ready: $MINT_REQUEST_DIR"

install -d -m 0700 -o "$AGENT_USER" -g "$AGENT_USER" \
    "$MUTATION_REQUEST_DIR" "$MUTATION_ENVELOPE_DIR" "$MUTATION_RECEIPT_DIR"
log "mutation exchange dirs ready"

# Tokens dir should already exist from the base install; ensure it does.
[ -d "$TOKENS_DIR" ] || install -d -m 0700 -o "$AGENT_USER" -g "$AGENT_USER" "$TOKENS_DIR"

# Local authority keypair. --init-key writes the private seed 0600 root at
# /etc/redflag/authority_local.key and installs the public half into the
# helper keyring so the execute path trusts what mint signs. Refuses to
# overwrite an existing authority.
if [ -f /etc/redflag/authority_local.key ]; then
    log "local authority already provisioned — leaving key untouched"
else
    "$HELPER_BIN" mint --init-key
    log "local authority created"
fi

# Sudoers: the agent user may invoke exactly the mint command shape, mirroring
# the execute-path grant. Request and token paths are pinned to their dirs.
cat > "$SUDOERS_FILE" <<EOF
# RedFlag standalone authority — fixed helper protocols (FEAT-003 / ARCH-002).
# The agent may request authority; the gates, signatures, and root-owned key decide.
$AGENT_USER ALL=(root) NOPASSWD: /usr/bin/systemd-run --wait --property=ProtectSystem=no -- $HELPER_BIN mint --request-file $MINT_REQUEST_DIR/* --token-out $TOKENS_DIR/*
$AGENT_USER ALL=(root) NOPASSWD: /usr/bin/systemd-run --wait --property=ProtectSystem=no -- $HELPER_BIN mint-envelope --request-file $MUTATION_REQUEST_DIR/* --envelope-out $MUTATION_ENVELOPE_DIR/*
$AGENT_USER ALL=(root) NOPASSWD: /usr/bin/systemd-run --wait --property=ProtectSystem=no -- $HELPER_BIN execute-envelope --envelope-file $MUTATION_ENVELOPE_DIR/* --receipt-file $MUTATION_RECEIPT_DIR/*
EOF
chmod 0440 "$SUDOERS_FILE"
visudo -c -f "$SUDOERS_FILE" >/dev/null || fail "sudoers validation failed for $SUDOERS_FILE"
log "sudoers installed: $SUDOERS_FILE"

log "standalone authority provisioned — fleet join is not implemented; do not add fleet credentials beside this key"
