// redflag-helper — capability-token executor (keystone).
// Windows target: stub binary (self-upgrade not yet ported — handled by installer).
#![cfg_attr(windows, allow(dead_code, unused_variables))]
//
// Reads one Ed25519-signed capability token from stdin, verifies it against a
// locally pinned trusted keyring, verifies every artifact hash it can reach on
// disk, guards against replay, then performs exactly one package operation over
// the signed dependency closure. No shell, no inherited environment, fail-closed
// on every error path. The signing authority lives off this host; this process
// only verifies and executes.
//
// Contract: see RAF/security/05-supply-chain-gate.md. The canonical signed message and
// closure hash here MUST stay byte-identical to the Go signer/verifier.

use std::collections::BTreeSet;
#[cfg(unix)]
use std::ffi::CString;
use std::fs;
use std::io::Read;
#[cfg(unix)]
use std::os::fd::{AsRawFd, FromRawFd};
#[cfg(unix)]
use std::os::unix::ffi::OsStrExt;
#[cfg(unix)]
use std::os::unix::fs::{MetadataExt, OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::{SystemTime, UNIX_EPOCH};

use ed25519_dalek::{Signature, Signer, SigningKey, Verifier, VerifyingKey};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use subtle::ConstantTimeEq;

#[allow(dead_code)]
mod mutation_protocol;
use mutation_protocol::{
    key_id_for, MutationAuthorization, MutationEnvelope, MutationManifest, MutationOutcome,
    MutationReceipt, MUTATION_PROTOCOL_VERSION,
};

#[cfg(windows)]
fn main() {
    eprintln!(
        "redflag-helper: Windows self-upgrade not yet implemented. \
         Desktop and agent updates on Windows are handled by the installer."
    );
    std::process::exit(0);
}

const SUPPORTED_TOKEN_VERSION: u32 = 1;

// Exit codes double as deny taxonomy. 0 = the one operation ran and exited 0.
const EXIT_OK: i32 = 0;
const EXIT_BAD_TOKEN: i32 = 10;
const EXIT_VERSION: i32 = 11;
const EXIT_TIME_WINDOW: i32 = 12;
const EXIT_AGENT_MISMATCH: i32 = 13;
const EXIT_KEY_NOT_FOUND: i32 = 14;
const EXIT_SIGNATURE: i32 = 15;
const EXIT_ARTIFACT: i32 = 16;
const EXIT_REPLAY: i32 = 17;
const EXIT_UNSUPPORTED_OP: i32 = 18;
const EXIT_EXEC_FAILED: i32 = 19;
const EXIT_INTERNAL: i32 = 20;
const EXIT_INTEGRITY: i32 = 21; // agent-binary hash mismatch (watchdog mode)
                                // Mint-mode (standalone authority) deny taxonomy.
const EXIT_MINT_GATE: i32 = 22; // gate verdict refuses (vuln/unverified without reason)
const EXIT_MINT_STALE: i32 = 23; // gate evidence outside the freshness window
const EXIT_MINT_KEY: i32 = 24; // mint key missing/unsafe permissions
const EXIT_MINT_DUPLICATE: i32 = 25; // request_id already minted
const EXIT_TRUST_PATH: i32 = 26; // trusted file/dir fails owner/perm/symlink validation (SEC-021)
const EXIT_AUTHORIZATION_DENIED: i32 = 27; // envelope carries a non-allow authority decision

// Default on-host locations. All overridable by env so packaging/tests can relocate.
const DEFAULT_KEYRING_DIR: &str = "/etc/redflag/trusted-keys";
const DEFAULT_STATE_FILE: &str = "/var/lib/redflag/helper/consumed-tokens";
#[cfg(unix)]
const DEFAULT_MUTATION_REPLAY_DIR: &str = "/var/lib/redflag/helper/consumed-authorizations";
#[cfg(unix)]
const DEFAULT_MUTATION_STAGING_DIR: &str = "/var/lib/redflag/helper/mutations";
#[cfg(unix)]
const DEFAULT_AGENT_MUTATION_REQUEST_DIR: &str = "/var/lib/redflag/agent/mutation-requests";
#[cfg(unix)]
const DEFAULT_AGENT_MUTATION_ENVELOPE_DIR: &str = "/var/lib/redflag/agent/mutation-envelopes";
#[cfg(unix)]
const DEFAULT_AGENT_MUTATION_RECEIPT_DIR: &str = "/var/lib/redflag/agent/mutation-receipts";
#[cfg(unix)]
const DEFAULT_AGENT_MINT_REQUEST_DIR: &str = "/var/lib/redflag/agent/mint";
#[cfg(unix)]
const DEFAULT_AGENT_TOKEN_DIR: &str = "/var/lib/redflag/agent/tokens";
#[cfg(unix)]
const DEFAULT_AGENT_RESULT_DIR: &str = "/var/lib/redflag/agent/results";
#[cfg(unix)]
const MAX_AGENT_EXCHANGE_BYTES: u64 = 8 * 1024 * 1024;
#[cfg(unix)]
const PACMAN_BINARY: &str = "/usr/bin/pacman";
#[cfg(unix)]
const VERCMP_BINARY: &str = "/usr/bin/vercmp";
#[cfg(unix)]
const PACMAN_KEY_BINARY: &str = "/usr/bin/pacman-key";
const AGENT_ID_FILES: &[&str] = &["/etc/redflag/agent_id", "/var/lib/redflag/agent_id"];

// Agent self-upgrade (package_type "agent-self"). The agent drops the downloaded
// binary at AGENT_UPGRADE_SOURCE — an agent-writable path on the real filesystem,
// not PrivateTmp, which this transient unit's own mount namespace cannot see. The
// helper copies it into HELPER_STAGING (root-only) before hashing and installing,
// so the agent cannot swap the bytes between verification and install. AGENT_BINARY
// is where the running agent lives and is always a helper constant, never taken
// from the token.
const DEFAULT_AGENT_BINARY: &str = "/usr/local/bin/redflag-agent";
const DEFAULT_AGENT_UPGRADE_SOURCE: &str = "/var/lib/redflag/agent/pending-upgrade.bin";
const DEFAULT_HELPER_STAGING: &str = "/var/lib/redflag/helper/upgrade-staging.bin";
const DEFAULT_HELPER_BINARY: &str = "/usr/local/bin/redflag-helper";
const AGENT_SELF_PACKAGE_TYPE: &str = "agent-self";
// Helper self-upgrade: the agent stages the new helper binary at this path,
// then invokes the old helper with a helper-self token. The old helper verifies
// the hash against the token's closure entry and replaces itself.
const HELPER_SELF_PACKAGE_TYPE: &str = "helper-self";
// DEFAULT_HELPER_SELF_SOURCE is a cross-language contract: the Go agent stages the
// new helper binary here before invoking the old helper with a helper-self token.
// The Go counterpart is helperSelfStagingPath in agent/internal/supplychain/binary_update.go.
// Both must be updated together if this path changes.
const DEFAULT_HELPER_SELF_SOURCE: &str = "/var/lib/redflag/agent/pending-helper.bin";
// Desktop self-upgrade (package_type "desktop-self"). The desktop binary is a
// system binary that lives beside the agent binary, so its install is a
// privileged swap like the agent's — it now runs through the helper instead of
// in the agent process (GATE-004), reusing the same hash-verify + replay guard.
const DESKTOP_SELF_PACKAGE_TYPE: &str = "desktop-self";
// DEFAULT_DESKTOP_SELF_SOURCE is a cross-language contract: the Go agent stages
// the new desktop binary here before invoking the helper with a desktop-self
// token. The Go counterpart is desktopSelfStagingPath in
// agent/internal/supplychain/binary_update.go. Both must change together.
const DEFAULT_DESKTOP_SELF_SOURCE: &str = "/var/lib/redflag/agent/pending-desktop.bin";
// DEFAULT_DESKTOP_BINARY is the install target — beside the agent binary, never
// taken from the token. Overridable via REDFLAG_DESKTOP_BINARY.
const DEFAULT_DESKTOP_BINARY: &str = "/usr/local/bin/redflag-desktop";

#[derive(Debug, Deserialize, Serialize, Clone)]
struct ClosureEntry {
    name: String,
    version: String,
    sha256: String,
    #[serde(default)]
    source: String, // "mirror" | "registry"
    #[serde(default)]
    artifact_path: Option<String>,
}

#[derive(Debug, Deserialize)]
struct CapabilityToken {
    version: u32,
    token_id: String,
    agent_id: String,
    key_id: String,
    package_type: String,
    operation: String,
    closure: Vec<ClosureEntry>,
    #[allow(dead_code)]
    issued_at: i64,
    not_before: i64,
    expires_at: i64,
    signature: String, // hex ed25519
}

#[derive(Debug, Serialize)]
struct PolicyResult {
    token_id: String,
    agent_id: String,
    package_type: String,
    operation: String,
    decision: String, // "executed" | "denied" | "failed"
    reason: String,
    executed: bool,
    verified_artifacts: usize,
    exit_code: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
    timestamp: i64,
}

// A deny/fail with its taxonomy code. Carries enough to emit a structured result.
#[derive(Debug)]
struct Denial {
    code: i32,
    reason: &'static str,
    detail: String,
}

impl Denial {
    fn new(code: i32, reason: &'static str, detail: impl Into<String>) -> Self {
        Denial {
            code,
            reason,
            detail: detail.into(),
        }
    }
}

fn now_unix() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

// ETHOS structured logging: [TAG] [system] [component] message key=value.
fn log_security(msg: &str) {
    eprintln!("[SECURITY] [helper] [executor] {}", msg);
}
fn log_info(msg: &str) {
    eprintln!("[INFO] [helper] [executor] {}", msg);
}
fn log_error(msg: &str) {
    eprintln!("[ERROR] [helper] [executor] {}", msg);
}

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

fn read_token_from_stdin() -> Result<CapabilityToken, Denial> {
    let mut buf = String::new();
    std::io::stdin()
        .read_to_string(&mut buf)
        .map_err(|e| Denial::new(EXIT_BAD_TOKEN, "stdin_read_failed", e.to_string()))?;
    let token: CapabilityToken = serde_json::from_str(&buf)
        .map_err(|e| Denial::new(EXIT_BAD_TOKEN, "token_parse_failed", e.to_string()))?;
    Ok(token)
}

fn read_token_from_file(path: &str) -> Result<CapabilityToken, Denial> {
    #[cfg(unix)]
    let buf = read_agent_exchange_file(path, DEFAULT_AGENT_TOKEN_DIR)?;
    #[cfg(not(unix))]
    let buf = fs::read_to_string(path).map_err(|e| {
        Denial::new(
            EXIT_BAD_TOKEN,
            "token_file_read_failed",
            format!("{}: {}", path, e),
        )
    })?;
    let token: CapabilityToken = serde_json::from_str(&buf)
        .map_err(|e| Denial::new(EXIT_BAD_TOKEN, "token_parse_failed", e.to_string()))?;
    Ok(token)
}

// This host's identity, read independently of the token so a forged agent_id
// cannot bind a token to a host it was not minted for.
fn local_agent_id() -> Result<String, Denial> {
    if let Ok(v) = std::env::var("REDFLAG_AGENT_ID") {
        let v = v.trim().to_string();
        if !v.is_empty() {
            return Ok(v);
        }
    }
    for path in AGENT_ID_FILES {
        let p = Path::new(path);
        if fs::symlink_metadata(p).is_err() {
            continue; // not provisioned at this location
        }
        // SEC-021: an agent_id file that exists but fails trust validation is a
        // denial, never a fall-through — a writable agent_id lets a compromised
        // agent rebind tokens minted for another host.
        validate_trusted_path(p)?;
        if let Ok(contents) = fs::read_to_string(p) {
            let v = contents.trim().to_string();
            if !v.is_empty() {
                return Ok(v);
            }
        }
    }
    Err(Denial::new(
        EXIT_AGENT_MISMATCH,
        "local_agent_id_unavailable",
        "no REDFLAG_AGENT_ID env and no provisioned agent_id file",
    ))
}

// Canonical UUID v4: 8-4-4-4-12 lowercase hex with version (4) and variant
// (8/9/a/b) nibbles. Used to constrain request_id so the journal dedup
// needle is always an exact-match target, never a substring of another ID.
fn is_valid_uuid_v4(s: &str) -> bool {
    mutation_protocol::is_canonical_uuid_v4(s)
}

// SEC-021: the helper defends its own trust inputs instead of relying on the
// installer having set permissions correctly. A trusted file or directory must
// be owned by the required uid, must not be writable by group or other, and
// must not be a symlink. Fail-closed: any violation is a denial, not a skip.
fn validate_trusted_path_as(path: &Path, required_uid: u32) -> Result<(), Denial> {
    let meta = fs::symlink_metadata(path).map_err(|e| {
        Denial::new(
            EXIT_TRUST_PATH,
            "trusted_path_unreadable",
            format!("{}: {}", path.display(), e),
        )
    })?;
    if meta.file_type().is_symlink() {
        return Err(Denial::new(
            EXIT_TRUST_PATH,
            "trusted_path_symlink",
            format!("{} — symlinked trust inputs are refused", path.display()),
        ));
    }
    #[cfg(unix)]
    {
        if meta.uid() != required_uid {
            return Err(Denial::new(
                EXIT_TRUST_PATH,
                "trusted_path_wrong_owner",
                format!(
                    "{} uid={} required={}",
                    path.display(),
                    meta.uid(),
                    required_uid
                ),
            ));
        }
        if meta.mode() & 0o022 != 0 {
            return Err(Denial::new(
                EXIT_TRUST_PATH,
                "trusted_path_writable",
                format!(
                    "{} mode={:o} — group/other write on a trust input",
                    path.display(),
                    meta.mode() & 0o7777
                ),
            ));
        }
    }
    Ok(())
}

// Production entry: trust inputs must be root-owned. Checks the path itself
// and its immediate parent so neither a swapped file nor a swapped containing
// directory passes.
fn validate_trusted_path(path: &Path) -> Result<(), Denial> {
    validate_trusted_path_as(path, 0)?;
    if let Some(parent) = path.parent() {
        validate_trusted_path_as(parent, 0)?;
    }
    Ok(())
}

// Load *.pub hex files from the keyring dir, indexed by computed key_id.
fn load_keyring(dir: &Path) -> Result<Vec<(String, VerifyingKey)>, Denial> {
    // SEC-021: refuse a keyring whose directory or files an unprivileged user
    // could have swapped or appended to — a writable keyring lets a compromised
    // agent inject its own key and mint self-signed tokens.
    validate_trusted_path(dir)?;
    let entries = fs::read_dir(dir).map_err(|e| {
        Denial::new(
            EXIT_KEY_NOT_FOUND,
            "keyring_unreadable",
            format!("{}: {}", dir.display(), e),
        )
    })?;
    let mut keys = Vec::new();
    for entry in entries.flatten() {
        let path = entry.path();
        if path.extension().and_then(|e| e.to_str()) != Some("pub") {
            continue;
        }
        validate_trusted_path_as(&path, 0)?;
        let raw = match fs::read_to_string(&path) {
            Ok(r) => r,
            Err(e) => {
                log_error(&format!(
                    "keyring_file_skipped path={} error={}",
                    path.display(),
                    e
                ));
                continue;
            }
        };
        let bytes = match hex::decode(raw.trim()) {
            Ok(b) => b,
            Err(e) => {
                log_error(&format!(
                    "keyring_file_bad_hex path={} error={}",
                    path.display(),
                    e
                ));
                continue;
            }
        };
        let arr: [u8; 32] = match bytes.as_slice().try_into() {
            Ok(a) => a,
            Err(_) => {
                log_error(&format!(
                    "keyring_file_bad_len path={} len={}",
                    path.display(),
                    bytes.len()
                ));
                continue;
            }
        };
        match VerifyingKey::from_bytes(&arr) {
            Ok(vk) => keys.push((key_id_for(&arr), vk)),
            Err(e) => log_error(&format!(
                "keyring_file_bad_key path={} error={}",
                path.display(),
                e
            )),
        }
    }
    if keys.is_empty() {
        return Err(Denial::new(
            EXIT_KEY_NOT_FOUND,
            "keyring_empty",
            format!("no usable *.pub keys in {}", dir.display()),
        ));
    }
    Ok(keys)
}

// closure_hash = hex(sha256( "\n".join( sorted("{name}@{version}#{sha256}") ) ))
// Sorted independently of array order so ordering cannot change the digest.
fn closure_hash(closure: &[ClosureEntry]) -> String {
    let mut lines: BTreeSet<String> = BTreeSet::new();
    for e in closure {
        lines.insert(format!("{}@{}#{}", e.name, e.version, e.sha256));
    }
    let joined = lines.into_iter().collect::<Vec<_>>().join("\n");
    hex::encode(Sha256::digest(joined.as_bytes()))
}

// signed_message = "{agent_id}:{token_id}:{operation}:{package_type}:{closure_hash}:{expires_at}"
fn signed_message(token: &CapabilityToken, closure_hash: &str) -> String {
    format!(
        "{}:{}:{}:{}:{}:{}",
        token.agent_id,
        token.token_id,
        token.operation,
        token.package_type,
        closure_hash,
        token.expires_at
    )
}

fn verify_signature(
    token: &CapabilityToken,
    keyring: &[(String, VerifyingKey)],
) -> Result<(), Denial> {
    let vk = keyring
        .iter()
        .find(|(id, _)| id == &token.key_id)
        .map(|(_, vk)| vk)
        .ok_or_else(|| {
            Denial::new(
                EXIT_KEY_NOT_FOUND,
                "key_id_not_trusted",
                format!("key_id={}", token.key_id),
            )
        })?;

    let sig_bytes = hex::decode(token.signature.trim())
        .map_err(|e| Denial::new(EXIT_SIGNATURE, "signature_bad_hex", e.to_string()))?;
    let sig = Signature::from_slice(&sig_bytes)
        .map_err(|e| Denial::new(EXIT_SIGNATURE, "signature_malformed", e.to_string()))?;

    let ch = closure_hash(&token.closure);
    let msg = signed_message(token, &ch);

    vk.verify(msg.as_bytes(), &sig)
        .map_err(|e| Denial::new(EXIT_SIGNATURE, "signature_invalid", e.to_string()))
}

fn compute_file_sha256(path: &Path) -> std::io::Result<String> {
    let mut file = fs::File::open(path)?;
    let mut hasher = Sha256::new();
    let mut buffer = [0u8; 8192];
    loop {
        let n = file.read(&mut buffer)?;
        if n == 0 {
            break;
        }
        hasher.update(&buffer[..n]);
    }
    Ok(hex::encode(hasher.finalize()))
}

// Verify every artifact the executor can actually reach on disk.
// mirror source: artifact_path is required and the file MUST exist and match.
// Constant-time equality for hex hash strings. A short-circuiting == or
// eq_ignore_ascii_case on the artifact-integrity gate would leak the pinned hash
// byte-by-byte through timing; this process runs as root via systemd-run and its
// wall-clock is observable, so the load-bearing compare must not be a timing
// oracle. Length is public (sha256 hex is always 64 chars), so the early length
// check leaks only that a malformed input was offered, never hash bytes. `subtle`
// is already in the dep tree via curve25519-dalek; depending on it explicitly keeps
// that a guarantee rather than a transitive accident.
fn ct_eq_hex(a: &str, b: &str) -> bool {
    let a = a.trim().to_lowercase();
    let b = b.trim().to_lowercase();
    a.len() == b.len() && bool::from(a.as_bytes().ct_eq(b.as_bytes()))
}

// registry source: if a local artifact_path is present, verify it; otherwise the
// server already attested the hash (covered by the signature) and enforcement is
// the mirror's job at fetch time. Returns count of hashes verified on disk.
fn verify_artifacts(token: &CapabilityToken) -> Result<usize, Denial> {
    let mut verified = 0usize;
    for e in &token.closure {
        let is_mirror = e.source == "mirror";
        let local_path = e.artifact_path.as_ref().filter(|p| !p.is_empty());

        match local_path {
            Some(p) => {
                let path = Path::new(p);
                if !path.is_file() {
                    if is_mirror {
                        return Err(Denial::new(
                            EXIT_ARTIFACT,
                            "mirror_artifact_missing",
                            format!("{}@{} path={}", e.name, e.version, p),
                        ));
                    }
                    log_info(&format!(
                        "registry_artifact_not_local name={} version={} deferred_to_mirror",
                        e.name, e.version
                    ));
                    continue;
                }
                let actual = compute_file_sha256(path).map_err(|err| {
                    Denial::new(
                        EXIT_ARTIFACT,
                        "artifact_hash_read_failed",
                        format!("{}: {}", p, err),
                    )
                })?;
                if !ct_eq_hex(&actual, &e.sha256) {
                    return Err(Denial::new(
                        EXIT_ARTIFACT,
                        "artifact_hash_mismatch",
                        format!(
                            "{}@{} expected={} actual={}",
                            e.name, e.version, e.sha256, actual
                        ),
                    ));
                }
                verified += 1;
            }
            None => {
                if is_mirror {
                    return Err(Denial::new(
                        EXIT_ARTIFACT,
                        "mirror_artifact_no_path",
                        format!(
                            "{}@{} source=mirror but no artifact_path",
                            e.name, e.version
                        ),
                    ));
                }
                log_info(&format!(
                    "registry_artifact_no_path name={} version={} deferred_to_mirror",
                    e.name, e.version
                ));
            }
        }
    }
    Ok(verified)
}

// Replay guard. token_id is recorded BEFORE execution so a token can never run
// twice even across a crash. A record-write failure is fail-closed (deny).
fn replay_check_and_record(token_id: &str, state_path: &Path) -> Result<(), Denial> {
    if let Some(parent) = state_path.parent() {
        fs::create_dir_all(parent).map_err(|e| {
            Denial::new(
                EXIT_INTERNAL,
                "state_dir_create_failed",
                format!("{}: {}", parent.display(), e),
            )
        })?;
        // SEC-021: a writable replay-guard dir lets a compromised agent clear
        // consumed-token records and replay. Validate after ensure-exists so a
        // pre-planted attacker-owned dir is refused, not adopted.
        validate_trusted_path_as(parent, 0)?;
    }
    if fs::symlink_metadata(state_path).is_ok() {
        validate_trusted_path_as(state_path, 0)?;
    }
    if let Ok(contents) = fs::read_to_string(state_path) {
        if contents.lines().any(|l| l.trim() == token_id) {
            return Err(Denial::new(
                EXIT_REPLAY,
                "token_already_consumed",
                format!("token_id={}", token_id),
            ));
        }
    }
    let mut existing = fs::read_to_string(state_path).unwrap_or_default();
    existing.push_str(token_id);
    existing.push('\n');
    fs::write(state_path, existing).map_err(|e| {
        Denial::new(
            EXIT_INTERNAL,
            "state_write_failed",
            format!("{}: {}", state_path.display(), e),
        )
    })?;
    Ok(())
}

// Build the argv plan for the one authorized operation. Returns a list of
// (program, args) invocations — single-element for line managers, per-artifact
// for image/winget managers. Unsupported (type, operation) pairs are denied
// rather than faked. Forward-only: only install/upgrade exist.
fn build_plan(token: &CapabilityToken) -> Result<Vec<(String, Vec<String>)>, Denial> {
    match token.operation.as_str() {
        "install" | "upgrade" => {}
        other => {
            return Err(Denial::new(
                EXIT_UNSUPPORTED_OP,
                "operation_not_allowed",
                format!("operation={} (forward-only: install|upgrade)", other),
            ))
        }
    }

    let c = &token.closure;
    let plan = match token.package_type.as_str() {
        "apt" => {
            let mut args = vec![
                "install".into(),
                "-y".into(),
                "--no-install-recommends".into(),
            ];
            // End-of-options marker: everything after is positional, so a
            // crafted package name starting with '-' cannot inject apt flags.
            args.push("--".into());
            for e in c {
                args.push(format!("{}={}", e.name, e.version));
            }
            vec![("apt-get".to_string(), args)]
        }
        "dnf" => {
            let mut args = vec!["install".into(), "-y".into()];
            // End-of-options marker: a crafted name-version string starting
            // with '-' cannot inject dnf flags (e.g. --skip-broken).
            args.push("--".into());
            for e in c {
                args.push(format!("{}-{}", e.name, e.version));
            }
            vec![("dnf".to_string(), args)]
        }
        "npm" => {
            let mut args = vec!["install".into()];
            for e in c {
                args.push(format!("{}@{}", e.name, e.version));
            }
            vec![("npm".to_string(), args)]
        }
        "bun" => {
            let mut args = vec!["add".into()];
            for e in c {
                args.push(format!("{}@{}", e.name, e.version));
            }
            vec![("bun".to_string(), args)]
        }
        "pip" => {
            let mut args = vec!["install".into()];
            for e in c {
                args.push(format!("{}=={}", e.name, e.version));
            }
            vec![("pip".to_string(), args)]
        }
        "docker" => c
            .iter()
            .map(|e| {
                (
                    "docker".to_string(),
                    vec!["pull".to_string(), format!("{}:{}", e.name, e.version)],
                )
            })
            .collect(),
        "winget" => c
            .iter()
            .map(|e| {
                (
                    "winget".to_string(),
                    vec![
                        "install".into(),
                        "--id".into(),
                        e.name.clone(),
                        "--version".into(),
                        e.version.clone(),
                        "--exact".into(),
                        "--accept-package-agreements".into(),
                        "--accept-source-agreements".into(),
                    ],
                )
            })
            .collect(),
        other => {
            return Err(Denial::new(
                EXIT_UNSUPPORTED_OP,
                "package_type_not_supported",
                format!("package_type={}", other),
            ))
        }
    };
    Ok(plan)
}

// Execute the plan with no shell and a stripped environment. Any non-zero step
// aborts the rest and fails closed.
fn execute_plan(plan: &[(String, Vec<String>)]) -> Result<(), Denial> {
    for (program, args) in plan {
        log_info(&format!("exec program={} argc={}", program, args.len()));
        let status = Command::new(program)
            .args(args)
            .env_clear()
            .env(
                "PATH",
                "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            )
            .status()
            .map_err(|e| {
                Denial::new(
                    EXIT_EXEC_FAILED,
                    "exec_spawn_failed",
                    format!("{}: {}", program, e),
                )
            })?;
        if !status.success() {
            return Err(Denial::new(
                EXIT_EXEC_FAILED,
                "exec_nonzero_exit",
                format!("{} exit={:?}", program, status.code()),
            ));
        }
    }
    Ok(())
}

fn emit_result<T: Serialize>(result: &T) {
    match serde_json::to_string(result) {
        Ok(s) => println!("{}", s),
        Err(e) => log_error(&format!("result_serialize_failed error={}", e)),
    }
}

fn emit_result_to_file<T: Serialize>(result: &T, path: &str) {
    let json = match serde_json::to_string(result) {
        Ok(s) => s,
        Err(e) => {
            log_error(&format!("result_serialize_failed error={}", e));
            return;
        }
    };
    // Write 0644 so the unprivileged agent can read the result back.
    // The result directory is 0700 agent-owned, which already blocks other
    // local users. The file itself must be world-readable because the helper
    // runs as root (via systemd-run) and the agent needs to read it.
    let mut o = std::fs::OpenOptions::new();
    o.write(true).create_new(true);
    #[cfg(unix)]
    {
        o.mode(0o644).custom_flags(libc::O_NOFOLLOW);
    }
    match o.open(path) {
        Ok(mut f) => {
            if let Err(e) = std::io::Write::write_all(&mut f, json.as_bytes()) {
                log_error(&format!("result_write_failed path={} error={}", path, e));
            }
        }
        Err(e) => {
            log_error(&format!("result_open_failed path={} error={}", path, e));
        }
    }
}

#[cfg(unix)]
fn validate_agent_exchange_path(path: &str, expected_dir: &str) -> Result<(), Denial> {
    let candidate = Path::new(path);
    if !candidate.is_absolute() || candidate.parent() != Some(Path::new(expected_dir)) {
        return Err(Denial::new(
            EXIT_TRUST_PATH,
            "agent_exchange_path_outside_protocol",
            format!("path={} expected_dir={}", path, expected_dir),
        ));
    }
    let name = candidate
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("");
    if name.is_empty()
        || name.len() > 160
        || name.bytes().any(|byte| byte.is_ascii_control())
        || name.contains('/')
        || name.contains('\\')
    {
        return Err(Denial::new(
            EXIT_TRUST_PATH,
            "agent_exchange_filename_invalid",
            format!("path={}", path),
        ));
    }
    let parent = fs::symlink_metadata(expected_dir).map_err(|error| {
        Denial::new(
            EXIT_TRUST_PATH,
            "agent_exchange_dir_unavailable",
            format!("{}: {}", expected_dir, error),
        )
    })?;
    if !parent.file_type().is_dir() || parent.file_type().is_symlink() {
        return Err(Denial::new(
            EXIT_TRUST_PATH,
            "agent_exchange_dir_invalid",
            expected_dir,
        ));
    }
    Ok(())
}

#[cfg(unix)]
fn open_directory_without_symlinks(path: &Path) -> Result<fs::File, Denial> {
    if !path.is_absolute() {
        return Err(Denial::new(
            EXIT_TRUST_PATH,
            "agent_exchange_dir_not_absolute",
            path.display().to_string(),
        ));
    }
    let mut current = fs::OpenOptions::new()
        .read(true)
        .custom_flags(libc::O_DIRECTORY | libc::O_NOFOLLOW | libc::O_CLOEXEC)
        .open("/")
        .map_err(|error| {
            Denial::new(
                EXIT_TRUST_PATH,
                "agent_exchange_root_open_failed",
                error.to_string(),
            )
        })?;
    for component in path.components() {
        let std::path::Component::Normal(name) = component else {
            continue;
        };
        let name = CString::new(name.as_bytes()).map_err(|_| {
            Denial::new(
                EXIT_TRUST_PATH,
                "agent_exchange_dir_component_invalid",
                path.display().to_string(),
            )
        })?;
        let descriptor = unsafe {
            libc::openat(
                current.as_raw_fd(),
                name.as_ptr(),
                libc::O_RDONLY | libc::O_DIRECTORY | libc::O_NOFOLLOW | libc::O_CLOEXEC,
            )
        };
        if descriptor < 0 {
            return Err(Denial::new(
                EXIT_TRUST_PATH,
                "agent_exchange_dir_open_failed",
                format!("{}: {}", path.display(), std::io::Error::last_os_error()),
            ));
        }
        current = unsafe { fs::File::from_raw_fd(descriptor) };
    }
    Ok(current)
}

#[cfg(unix)]
fn open_agent_exchange_file(
    path: &str,
    expected_dir: &str,
    flags: libc::c_int,
    mode: libc::mode_t,
) -> Result<fs::File, Denial> {
    validate_agent_exchange_path(path, expected_dir)?;
    let directory = open_directory_without_symlinks(Path::new(expected_dir))?;
    let name = Path::new(path)
        .file_name()
        .ok_or_else(|| Denial::new(EXIT_TRUST_PATH, "agent_exchange_filename_invalid", path))?;
    let name = CString::new(name.as_bytes())
        .map_err(|_| Denial::new(EXIT_TRUST_PATH, "agent_exchange_filename_invalid", path))?;
    let descriptor = unsafe {
        libc::openat(
            directory.as_raw_fd(),
            name.as_ptr(),
            flags | libc::O_NOFOLLOW | libc::O_CLOEXEC,
            mode as libc::c_uint,
        )
    };
    if descriptor < 0 {
        return Err(Denial::new(
            EXIT_TRUST_PATH,
            "agent_exchange_open_failed",
            format!("{}: {}", path, std::io::Error::last_os_error()),
        ));
    }
    Ok(unsafe { fs::File::from_raw_fd(descriptor) })
}

#[cfg(unix)]
fn read_agent_exchange_file(path: &str, expected_dir: &str) -> Result<String, Denial> {
    let mut file = open_agent_exchange_file(path, expected_dir, libc::O_RDONLY, 0)?;
    let metadata = file.metadata().map_err(|error| {
        Denial::new(
            EXIT_TRUST_PATH,
            "agent_exchange_metadata_failed",
            format!("{}: {}", path, error),
        )
    })?;
    if !metadata.is_file() {
        return Err(Denial::new(
            EXIT_TRUST_PATH,
            "agent_exchange_not_regular",
            path,
        ));
    }
    if metadata.len() > MAX_AGENT_EXCHANGE_BYTES {
        return Err(Denial::new(
            EXIT_BAD_TOKEN,
            "agent_exchange_too_large",
            format!(
                "{} bytes={} limit={}",
                path,
                metadata.len(),
                MAX_AGENT_EXCHANGE_BYTES
            ),
        ));
    }
    let mut buffer = String::new();
    file.read_to_string(&mut buffer).map_err(|error| {
        Denial::new(
            EXIT_BAD_TOKEN,
            "agent_exchange_read_failed",
            format!("{}: {}", path, error),
        )
    })?;
    Ok(buffer)
}

#[cfg(unix)]
fn write_agent_exchange_json<T: Serialize>(
    result: &T,
    path: &str,
    expected_dir: &str,
) -> Result<(), Denial> {
    let json = serde_json::to_vec(result).map_err(|error| {
        Denial::new(EXIT_INTERNAL, "result_serialize_failed", error.to_string())
    })?;
    let mut file = open_agent_exchange_file(
        path,
        expected_dir,
        libc::O_WRONLY | libc::O_CREAT | libc::O_EXCL,
        0o644,
    )?;
    std::io::Write::write_all(&mut file, &json).map_err(|error| {
        Denial::new(
            EXIT_INTERNAL,
            "result_write_failed",
            format!("{}: {}", path, error),
        )
    })?;
    file.sync_all().map_err(|error| {
        Denial::new(
            EXIT_INTERNAL,
            "result_sync_failed",
            format!("{}: {}", path, error),
        )
    })
}

// stage_and_verify_binary copies a source binary into a root-only staging file
// and verifies its SHA-256 against the expected hash. Returns the staging path.
// Run BEFORE the replay slot is consumed so a tampered binary does not burn the
// token. Generic: used for both agent and helper binaries.
fn stage_and_verify_binary(
    source: &str,
    staging: &str,
    expected_hash: &str,
) -> Result<String, Denial> {
    let expected = expected_hash.trim().to_lowercase();
    if expected.is_empty() {
        return Err(Denial::new(
            EXIT_ARTIFACT,
            "stage_empty_hash",
            "expected sha256 is empty",
        ));
    }

    if let Some(parent) = Path::new(staging).parent() {
        fs::create_dir_all(parent).map_err(|e| {
            Denial::new(
                EXIT_INTERNAL,
                "stage_dir_create",
                format!("{}: {}", parent.display(), e),
            )
        })?;
    }
    // Copy first, then hash the copy — never hash a path we will later re-read,
    // or the agent could swap the bytes in between.
    fs::copy(source, staging).map_err(|e| {
        Denial::new(
            EXIT_ARTIFACT,
            "stage_copy_failed",
            format!("{} -> {}: {}", source, staging, e),
        )
    })?;

    let actual = match compute_file_sha256(Path::new(staging)) {
        Ok(h) => h.to_lowercase(),
        Err(e) => {
            let _ = fs::remove_file(staging);
            return Err(Denial::new(
                EXIT_ARTIFACT,
                "stage_hash_failed",
                format!("{}: {}", staging, e),
            ));
        }
    };
    if !ct_eq_hex(&actual, &expected) {
        let _ = fs::remove_file(staging);
        return Err(Denial::new(
            EXIT_ARTIFACT,
            "stage_hash_mismatch",
            format!("source={} expected={} actual={}", source, expected, actual),
        ));
    }
    Ok(staging.to_string())
}

// atomic_replace_binary swaps `install` for the contents of `staged` without ever
// opening the live binary for writing. Copying directly onto a path that is being
// executed fails with ETXTBSY ("text file busy") — the kernel forbids a truncating
// write to a running executable. Instead we write a sibling temp in the same
// directory (same filesystem, so the rename stays atomic), fix its mode, and
// rename() it over the target. rename relinks the directory entry: the running
// process keeps its old inode, the next exec picks up the new one. Both temp and
// target live in the root-owned install dir, so no agent-writable intermediate is
// introduced and the verify->install integrity guarantee is preserved.
fn atomic_replace_binary(staged: &str, install: &str, op: &'static str) -> Result<(), Denial> {
    let tmp = format!("{}.new", install);
    fs::copy(staged, &tmp).map_err(|e| {
        Denial::new(
            EXIT_EXEC_FAILED,
            op,
            format!("{} -> {}: {}", staged, tmp, e),
        )
    })?;
    #[cfg(unix)]
    if let Err(e) = fs::set_permissions(&tmp, fs::Permissions::from_mode(0o755)) {
        let _ = fs::remove_file(&tmp);
        return Err(Denial::new(
            EXIT_EXEC_FAILED,
            op,
            format!("chmod {}: {}", tmp, e),
        ));
    }
    if let Err(e) = fs::rename(&tmp, install) {
        let _ = fs::remove_file(&tmp);
        return Err(Denial::new(
            EXIT_EXEC_FAILED,
            op,
            format!("{} -> {}: {}", tmp, install, e),
        ));
    }
    Ok(())
}

// install_staged_agent_binary backs up the live agent binary, installs the
// verified staged copy in its place, fixes the mode, and restarts the agent. The
// install path is a helper constant, never taken from the token. Runs only after
// the replay slot is consumed (these side effects are irreversible).
fn install_staged_agent_binary(staged: &str) -> Result<(), Denial> {
    let install = env_or("REDFLAG_AGENT_BINARY", DEFAULT_AGENT_BINARY);
    let backup = format!("{}.bak", install);

    if Path::new(&install).exists() {
        fs::copy(&install, &backup).map_err(|e| {
            Denial::new(
                EXIT_EXEC_FAILED,
                "agent_self_backup",
                format!("{} -> {}: {}", install, backup, e),
            )
        })?;
    }
    atomic_replace_binary(staged, &install, "agent_self_install")?;
    let _ = fs::remove_file(staged);

    // Restore CAP_SYS_PTRACE on the new binary. rename() creates a new inode
    // which strips file capabilities. Without this, the agent loses its ability
    // to read /proc/[pid]/environ for display/process discovery after every upgrade.
    let setcap_status = Command::new("setcap")
        .args(["cap_sys_ptrace=eip", &install])
        .env_clear()
        .env(
            "PATH",
            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        )
        .status();
    match setcap_status {
        Ok(s) if s.success() => {
            log_security(&format!("cap_sys_ptrace_restored path={}", install));
        }
        Ok(s) => {
            // Non-fatal: agent works without it, just loses screenshot/process discovery.
            log_security(&format!(
                "cap_sys_ptrace_restore_failed path={} exit={}",
                install, s
            ));
        }
        Err(e) => {
            log_security(&format!(
                "cap_sys_ptrace_restore_error path={} err={}",
                install, e
            ));
        }
    }

    // Reconcile the unit drop-in before the restart so the restarted agent
    // comes up with the capability grant in place.
    reconcile_agent_unit_dropin();

    // Enqueue the restart with --no-block and return. The agent process is the
    // consumer blocked on this helper's stdout pipe; a synchronous restart would
    // SIGTERM it before we emit our result (broken pipe). --no-block lets this
    // helper finish and report first, then systemd performs the restart. Boot
    // verification is server-side (the new binary reports its version on check-in).
    let status = Command::new("systemctl")
        .args(["restart", "--no-block", "redflag-agent"])
        .env_clear()
        .env(
            "PATH",
            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        )
        .status()
        .map_err(|e| Denial::new(EXIT_EXEC_FAILED, "agent_self_restart_spawn", e.to_string()))?;
    if !status.success() {
        return Err(Denial::new(
            EXIT_EXEC_FAILED,
            "agent_self_restart_failed",
            format!("systemctl restart redflag-agent exit={:?}", status.code()),
        ));
    }
    Ok(())
}

// reconcile_agent_unit_dropin delivers the CAP_SYS_PTRACE grant to fleet units
// that predate the AmbientCapabilities line in the installer template. Self-
// upgrade swaps the binary but never rewrites the installer-owned service file,
// so without this the capability only reaches hosts that re-run the install
// script. A drop-in is additive and idempotent — it never clobbers the unit.
//
// AmbientCapabilities only, never CapabilityBoundingSet: restricting the
// bounding set would strip CAP_SETUID/CAP_SETGID from setuid binaries (sudo)
// inside the unit and break package discovery and the helper invocation path.
//
// Non-fatal on failure: the agent runs without the cap (loses screenshot and
// process discovery), and a failed upgrade is worse than a missing capability.
fn reconcile_agent_unit_dropin() {
    const DROPIN_DIR: &str = "/etc/systemd/system/redflag-agent.service.d";
    const DROPIN_PATH: &str = "/etc/systemd/system/redflag-agent.service.d/10-capabilities.conf";
    const DROPIN_CONTENT: &str = "# Managed by redflag-helper (agent self-upgrade). Do not edit.\n\
        # Grants /proc/<pid>/environ read for display/process discovery on units\n\
        # installed before the template carried this line.\n\
        [Service]\n\
        AmbientCapabilities=CAP_SYS_PTRACE\n";

    // Already reconciled — skip the write and the daemon-reload.
    if fs::read_to_string(DROPIN_PATH)
        .map(|c| c == DROPIN_CONTENT)
        .unwrap_or(false)
    {
        return;
    }

    if let Err(e) = fs::create_dir_all(DROPIN_DIR) {
        log_security(&format!(
            "unit_dropin_dir_failed path={} err={}",
            DROPIN_DIR, e
        ));
        return;
    }
    if let Err(e) = fs::write(DROPIN_PATH, DROPIN_CONTENT) {
        log_security(&format!(
            "unit_dropin_write_failed path={} err={}",
            DROPIN_PATH, e
        ));
        return;
    }

    // The drop-in only takes effect after a reload; the --no-block restart
    // that follows self-upgrade then picks it up.
    let status = Command::new("systemctl")
        .args(["daemon-reload"])
        .env_clear()
        .env(
            "PATH",
            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        )
        .status();
    match status {
        Ok(s) if s.success() => {
            log_security(&format!("unit_dropin_reconciled path={}", DROPIN_PATH));
        }
        Ok(s) => {
            log_security(&format!("unit_dropin_daemon_reload_failed exit={}", s));
        }
        Err(e) => {
            log_security(&format!("unit_dropin_daemon_reload_error err={}", e));
        }
    }
}

// install_helper_binary replaces the running helper binary with a verified
// staged copy. This is a self-update: the current process is the helper, and
// we're replacing our own binary on disk. The new binary takes effect on the
// next invocation (the current process finishes normally). No restart needed —
// the helper is a one-shot executor, not a long-running service.
fn install_helper_binary(staged: &str) -> Result<(), Denial> {
    let install = env_or("REDFLAG_HELPER_BINARY", DEFAULT_HELPER_BINARY);
    let backup = format!("{}.bak", install);

    if Path::new(&install).exists() {
        fs::copy(&install, &backup).map_err(|e| {
            Denial::new(
                EXIT_EXEC_FAILED,
                "helper_self_backup",
                format!("{} -> {}: {}", install, backup, e),
            )
        })?;
    }
    atomic_replace_binary(staged, &install, "helper_self_install")?;
    let _ = fs::remove_file(staged);
    log_security(&format!("helper_self_updated path={}", install));
    Ok(())
}

// install_desktop_binary backs up the live desktop binary and installs the
// verified staged copy beside the agent binary. The install path is a helper
// constant, never taken from the token. Runs only after the replay slot is
// consumed (the side effect is irreversible). The desktop app is a one-shot the
// agent re-launches after a successful install, so no setcap/service restart is
// performed here.
fn install_desktop_binary(staged: &str) -> Result<(), Denial> {
    let install = env_or("REDFLAG_DESKTOP_BINARY", DEFAULT_DESKTOP_BINARY);
    let backup = format!("{}.bak", install);

    if Path::new(&install).exists() {
        fs::copy(&install, &backup).map_err(|e| {
            Denial::new(
                EXIT_EXEC_FAILED,
                "desktop_self_backup",
                format!("{} -> {}: {}", install, backup, e),
            )
        })?;
    }
    atomic_replace_binary(staged, &install, "desktop_self_install")?;
    let _ = fs::remove_file(staged);
    log_security(&format!("desktop_self_updated path={}", install));

    // A desktop-self token onto a pre-tray host delivers a binary that nothing
    // launches — heal the session-start provisioning the token install can't
    // carry (UPDATE-002 gap 2), same reconciliation posture as the agent unit
    // drop-in.
    reconcile_desktop_autostart(&install);
    Ok(())
}

// reconcile_desktop_autostart creates the XDG autostart entry when it is
// missing, so hosts that received the tray via desktop-self token (rather than
// the installer) launch it at session login. Create-if-absent only: an existing
// file is never rewritten, whether installer-owned or operator-edited — and
// deliberate removal is how an operator opts a session out today, which the
// INSTALL-004 opt-out will formalize by gating the desktop-self mint
// server-side. Group membership is not healed here: the helper has no session
// user to bind, and the tray's own socket-permission diagnostic tells the user
// the exact usermod to run. Non-fatal on failure — the binary install already
// succeeded and is journaled.
fn reconcile_desktop_autostart(binary_path: &str) {
    const AUTOSTART_DIR: &str = "/etc/xdg/autostart";
    const AUTOSTART_PATH: &str = "/etc/xdg/autostart/redflag-desktop.desktop";

    if Path::new(AUTOSTART_PATH).exists() {
        return;
    }

    let content = format!(
        "[Desktop Entry]\n\
         Type=Application\n\
         Name=RedFlag\n\
         Comment=RedFlag system tray (local agent dashboard)\n\
         Exec={}\n\
         Terminal=false\n\
         X-GNOME-Autostart-enabled=true\n",
        binary_path
    );

    if let Err(e) = fs::create_dir_all(AUTOSTART_DIR) {
        log_security(&format!(
            "desktop_autostart_dir_failed path={} err={}",
            AUTOSTART_DIR, e
        ));
        return;
    }
    if let Err(e) = fs::write(AUTOSTART_PATH, content) {
        log_security(&format!(
            "desktop_autostart_write_failed path={} err={}",
            AUTOSTART_PATH, e
        ));
        return;
    }
    // World-readable like every other /etc/xdg/autostart entry.
    if let Err(e) = fs::set_permissions(AUTOSTART_PATH, fs::Permissions::from_mode(0o644)) {
        log_security(&format!(
            "desktop_autostart_chmod_failed path={} err={}",
            AUTOSTART_PATH, e
        ));
        return;
    }
    log_security(&format!(
        "desktop_autostart_reconciled path={}",
        AUTOSTART_PATH
    ));
}

fn run(
    token_file: Option<&str>,
    helper_file: Option<&str>,
) -> Result<PolicyResult, Box<(Option<CapabilityToken>, Denial)>> {
    let token = match token_file {
        Some(path) => read_token_from_file(path).map_err(|d| Box::new((None, d))),
        None => read_token_from_stdin().map_err(|d| Box::new((None, d))),
    }?;

    if token.version != SUPPORTED_TOKEN_VERSION {
        let d = Denial::new(
            EXIT_VERSION,
            "unsupported_token_version",
            format!(
                "version={} supported={}",
                token.version, SUPPORTED_TOKEN_VERSION
            ),
        );
        return Err(Box::new((Some(token), d)));
    }

    // Validity window.
    let now = now_unix();
    if now < token.not_before {
        let d = Denial::new(
            EXIT_TIME_WINDOW,
            "token_not_yet_valid",
            format!("now={} not_before={}", now, token.not_before),
        );
        return Err(Box::new((Some(token), d)));
    }
    if now > token.expires_at {
        let d = Denial::new(
            EXIT_TIME_WINDOW,
            "token_expired",
            format!("now={} expires_at={}", now, token.expires_at),
        );
        return Err(Box::new((Some(token), d)));
    }

    // Bind-check against this host's independently-read identity.
    let local = match local_agent_id() {
        Ok(v) => v,
        Err(d) => return Err(Box::new((Some(token), d))),
    };
    if local != token.agent_id {
        let d = Denial::new(
            EXIT_AGENT_MISMATCH,
            "agent_id_mismatch",
            format!("token_agent_id={} host_agent_id={}", token.agent_id, local),
        );
        return Err(Box::new((Some(token), d)));
    }

    // Verify signature against the pinned keyring (verify keys, not servers).
    let keyring_dir = PathBuf::from(env_or("REDFLAG_HELPER_KEYRING", DEFAULT_KEYRING_DIR));
    let keyring = match load_keyring(&keyring_dir) {
        Ok(k) => k,
        Err(d) => return Err(Box::new((Some(token), d))),
    };
    if let Err(d) = verify_signature(&token, &keyring) {
        return Err(Box::new((Some(token), d)));
    }

    // Agent self-upgrade is a privileged binary swap, not a package install. The
    // signature is verified above; here we verify the new binary's hash and replace
    // the agent binary, bypassing the package-manager plan path entirely.
    //
    // When the closure contains 2 entries (agent + helper), the helper self-updates
    // first, then installs the new agent binary. This keeps the helper and agent
    // in sync across updates.
    if token.package_type == AGENT_SELF_PACKAGE_TYPE {
        if token.operation != "upgrade" {
            return Err(Box::new((
                Some(token),
                Denial::new(
                    EXIT_UNSUPPORTED_OP,
                    "operation_not_allowed",
                    "agent-self requires operation=upgrade",
                ),
            )));
        }

        // Separate the closure into agent and helper entries.
        let mut agent_entry: Option<&ClosureEntry> = None;
        let mut helper_entry: Option<&ClosureEntry> = None;
        for e in &token.closure {
            match e.name.as_str() {
                "redflag-agent" => agent_entry = Some(e),
                "redflag-helper" => helper_entry = Some(e),
                _ => {}
            }
        }
        let agent_entry = match agent_entry {
            Some(e) => e,
            None => {
                return Err(Box::new((
                    Some(token),
                    Denial::new(
                        EXIT_BAD_TOKEN,
                        "agent_self_no_agent_entry",
                        "closure missing redflag-agent entry",
                    ),
                )))
            }
        };

        // Stage + hash-verify the agent binary before consuming the replay slot.
        let staged_agent = match stage_and_verify_binary(
            &env_or("REDFLAG_AGENT_UPGRADE_SOURCE", DEFAULT_AGENT_UPGRADE_SOURCE),
            &env_or("REDFLAG_HELPER_STAGING", DEFAULT_HELPER_STAGING),
            &agent_entry.sha256,
        ) {
            Ok(p) => p,
            Err(d) => return Err(Box::new((Some(token), d))),
        };

        // If the closure has a helper entry and --helper-file was provided,
        // stage and verify the helper binary for self-update.
        let staged_helper: Option<String> =
            if let (Some(h_entry), Some(h_path)) = (helper_entry, helper_file) {
                let staging = env_or("REDFLAG_HELPER_STAGING", DEFAULT_HELPER_STAGING) + ".helper";
                match stage_and_verify_binary(h_path, &staging, &h_entry.sha256) {
                    Ok(p) => Some(p),
                    Err(d) => return Err(Box::new((Some(token), d))),
                }
            } else {
                None
            };

        // Consume the replay slot now that both binaries are verified.
        let state_path = PathBuf::from(env_or("REDFLAG_HELPER_STATE", DEFAULT_STATE_FILE));
        if let Err(d) = replay_check_and_record(&token.token_id, &state_path) {
            return Err(Box::new((Some(token), d)));
        }

        log_security(&format!(
            "authorized agent_self_upgrade token_id={} agent_id={} has_helper_update={}",
            token.token_id,
            token.agent_id,
            staged_helper.is_some()
        ));

        // Self-update the helper first (if we have one), then install the agent.
        // Order matters: if the helper self-update fails, we abort before touching
        // the agent binary.
        if let Some(ref helper_staged) = staged_helper {
            if let Err(d) = install_helper_binary(helper_staged) {
                let _ = fs::remove_file(&staged_agent);
                return Err(Box::new((Some(token), d)));
            }
        }
        if let Err(d) = install_staged_agent_binary(&staged_agent) {
            return Err(Box::new((Some(token), d)));
        }

        let verified_count = if staged_helper.is_some() { 2 } else { 1 };
        return Ok(PolicyResult {
            token_id: token.token_id.clone(),
            agent_id: token.agent_id.clone(),
            package_type: token.package_type.clone(),
            operation: token.operation.clone(),
            decision: "executed".to_string(),
            reason: if staged_helper.is_some() {
                "agent_and_helper_upgraded"
            } else {
                "agent_self_upgraded"
            }
            .to_string(),
            executed: true,
            verified_artifacts: verified_count,
            exit_code: EXIT_OK,
            error: None,
            timestamp: now_unix(),
        });
    }

    // Helper self-upgrade: the agent staged the new helper binary and invokes the
    // old helper with a helper-self token. This branch verifies the hash, consumes
    // the replay slot, and atomically replaces the helper binary on disk. No restart
    // is needed — the helper is a one-shot executor; the next invocation runs the
    // new binary.
    if token.package_type == HELPER_SELF_PACKAGE_TYPE {
        if token.operation != "upgrade" {
            return Err(Box::new((
                Some(token),
                Denial::new(
                    EXIT_UNSUPPORTED_OP,
                    "operation_not_allowed",
                    "helper-self requires operation=upgrade",
                ),
            )));
        }

        let sha256 = {
            let entry = token.closure.iter().find(|e| e.name == "redflag-helper");
            match entry {
                Some(e) => e.sha256.clone(),
                None => {
                    return Err(Box::new((
                        Some(token),
                        Denial::new(
                            EXIT_BAD_TOKEN,
                            "helper_self_no_entry",
                            "closure missing redflag-helper entry",
                        ),
                    )))
                }
            }
        };

        let source = env_or("REDFLAG_HELPER_SELF_SOURCE", DEFAULT_HELPER_SELF_SOURCE);
        let staging = env_or("REDFLAG_HELPER_STAGING", DEFAULT_HELPER_STAGING) + ".self";

        let staged = match stage_and_verify_binary(&source, &staging, &sha256) {
            Ok(p) => p,
            Err(d) => return Err(Box::new((Some(token), d))),
        };

        let state_path = PathBuf::from(env_or("REDFLAG_HELPER_STATE", DEFAULT_STATE_FILE));
        if let Err(d) = replay_check_and_record(&token.token_id, &state_path) {
            let _ = fs::remove_file(&staged);
            return Err(Box::new((Some(token), d)));
        }

        log_security(&format!(
            "authorized helper_self_upgrade token_id={} agent_id={}",
            token.token_id, token.agent_id
        ));

        if let Err(d) = install_helper_binary(&staged) {
            let _ = fs::remove_file(&staged);
            return Err(Box::new((Some(token), d)));
        }

        return Ok(PolicyResult {
            token_id: token.token_id.clone(),
            agent_id: token.agent_id.clone(),
            package_type: token.package_type.clone(),
            operation: token.operation.clone(),
            decision: "executed".to_string(),
            reason: "helper_self_upgraded".to_string(),
            executed: true,
            verified_artifacts: 1,
            exit_code: EXIT_OK,
            error: None,
            timestamp: now_unix(),
        });
    }

    // Desktop self-upgrade (GATE-004): the agent stages the new desktop binary and
    // invokes the helper with a desktop-self token instead of installing it in the
    // agent process. Same shape as helper-self: verify the hash against the closure
    // entry, consume the replay slot (record-before-install), then atomically swap
    // the desktop binary. The token, closure, and signed message are identical to
    // the old in-process path — only the executor moved.
    if token.package_type == DESKTOP_SELF_PACKAGE_TYPE {
        if token.operation != "upgrade" {
            return Err(Box::new((
                Some(token),
                Denial::new(
                    EXIT_UNSUPPORTED_OP,
                    "operation_not_allowed",
                    "desktop-self requires operation=upgrade",
                ),
            )));
        }

        let sha256 = {
            let entry = token.closure.iter().find(|e| e.name == "redflag-desktop");
            match entry {
                Some(e) => e.sha256.clone(),
                None => {
                    return Err(Box::new((
                        Some(token),
                        Denial::new(
                            EXIT_BAD_TOKEN,
                            "desktop_self_no_entry",
                            "closure missing redflag-desktop entry",
                        ),
                    )))
                }
            }
        };

        let source = env_or("REDFLAG_DESKTOP_SELF_SOURCE", DEFAULT_DESKTOP_SELF_SOURCE);
        let staging = env_or("REDFLAG_HELPER_STAGING", DEFAULT_HELPER_STAGING) + ".desktop";

        let staged = match stage_and_verify_binary(&source, &staging, &sha256) {
            Ok(p) => p,
            Err(d) => return Err(Box::new((Some(token), d))),
        };

        let state_path = PathBuf::from(env_or("REDFLAG_HELPER_STATE", DEFAULT_STATE_FILE));
        if let Err(d) = replay_check_and_record(&token.token_id, &state_path) {
            let _ = fs::remove_file(&staged);
            return Err(Box::new((Some(token), d)));
        }

        log_security(&format!(
            "authorized desktop_self_upgrade token_id={} agent_id={}",
            token.token_id, token.agent_id
        ));

        if let Err(d) = install_desktop_binary(&staged) {
            let _ = fs::remove_file(&staged);
            return Err(Box::new((Some(token), d)));
        }

        return Ok(PolicyResult {
            token_id: token.token_id.clone(),
            agent_id: token.agent_id.clone(),
            package_type: token.package_type.clone(),
            operation: token.operation.clone(),
            decision: "executed".to_string(),
            reason: "desktop_self_upgraded".to_string(),
            executed: true,
            verified_artifacts: 1,
            exit_code: EXIT_OK,
            error: None,
            timestamp: now_unix(),
        });
    }

    // Verify artifact hashes the executor can reach.
    let verified = match verify_artifacts(&token) {
        Ok(v) => v,
        Err(d) => return Err(Box::new((Some(token), d))),
    };

    // Build the one operation before consuming the replay slot, so an
    // unsupported op does not burn the token_id.
    let plan = match build_plan(&token) {
        Ok(p) => p,
        Err(d) => return Err(Box::new((Some(token), d))),
    };

    // Replay guard records the token_id before execution.
    let state_path = PathBuf::from(env_or("REDFLAG_HELPER_STATE", DEFAULT_STATE_FILE));
    if let Err(d) = replay_check_and_record(&token.token_id, &state_path) {
        return Err(Box::new((Some(token), d)));
    }

    log_security(&format!(
        "authorized token_id={} agent_id={} package_type={} operation={} closure_size={} verified_artifacts={}",
        token.token_id, token.agent_id, token.package_type, token.operation, token.closure.len(), verified
    ));

    if let Err(d) = execute_plan(&plan) {
        return Err(Box::new((Some(token), d)));
    }

    Ok(PolicyResult {
        token_id: token.token_id.clone(),
        agent_id: token.agent_id.clone(),
        package_type: token.package_type.clone(),
        operation: token.operation.clone(),
        decision: "executed".to_string(),
        reason: "operation_completed".to_string(),
        executed: true,
        verified_artifacts: verified,
        exit_code: EXIT_OK,
        error: None,
        timestamp: now_unix(),
    })
}

// ============================================================================
// Mint mode — standalone local authority (RAF/security/06-standalone-authority.md)
//
// On a host with no fleet server, this privileged invocation mints the
// capability token after the unprivileged agent has run the gates (closure
// dry-run resolve, OSV best-effort, age/soak). The key remains root-owned and
// unreadable by the Agent, while the fixed helper protocol constrains what the
// Agent may ask root to sign. This does not reproduce off-host authority against
// a compromised Agent: the helper validates evidence shape, not the human.
//
// The freshness window is HARD-CODED (no doctrinal knobs): evidence older
// than 15 minutes means the agent re-resolves and re-checks. Vulnerable or
// OSV-unverified closures mint only with an explicit operator override
// reason, and every decision is journaled before the token is emitted.
// ============================================================================

const MINT_EVIDENCE_MAX_AGE_SECS: i64 = 900; // 15 min — fixed by design, not configurable
const MINT_CLOCK_SKEW_SECS: i64 = 60;
const MINT_TOKEN_TTL_SECS: i64 = 600; // matches the 10-minute command validity window
const DEFAULT_MINT_KEY: &str = "/etc/redflag/authority_local.key";
const DEFAULT_MINT_JOURNAL: &str = "/var/lib/redflag/journal/mint.log";
// The standalone local API exposes legacy capability approval only for APT and
// DNF. Keep the privileged minter equally narrow; pacman has its envelope path.
const MINTABLE_PACKAGE_TYPES: &[&str] = &["apt", "dnf"];

#[derive(Debug, Deserialize)]
struct GateEvidence {
    resolved_at: i64,
    #[serde(default)]
    osv_checked_at: i64,
    osv_status: String, // "clear" | "vulnerable" | "unreachable" | "unsupported"
    #[serde(default)]
    osv_vuln_count: u32,
    age_gate: String,  // "pass" | "overridden" | "not_applicable"
    soak_gate: String, // "pass" | "overridden" | "not_applicable"
    operator: String,
    #[serde(default)]
    override_reason: String,
}

#[derive(Debug, Deserialize)]
struct MintRequest {
    version: u32,
    request_id: String,
    agent_id: String,
    package_type: String,
    operation: String,
    closure: Vec<ClosureEntry>,
    gate_evidence: GateEvidence,
}

#[cfg(unix)]
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct EnvelopeMintRequest {
    version: u32,
    request_id: String,
    manifest: MutationManifest,
    gate_evidence: GateEvidence,
}

// Serialized wire token — must match the CapabilityToken the execute path and
// the Go consumer parse. Field order is irrelevant (JSON), the signature is not.
#[derive(Debug, Serialize)]
struct MintedToken {
    version: u32,
    token_id: String,
    agent_id: String,
    key_id: String,
    package_type: String,
    operation: String,
    closure: Vec<ClosureEntry>,
    issued_at: i64,
    not_before: i64,
    expires_at: i64,
    signature: String,
}

struct MintPaths {
    key_path: PathBuf,
    journal_path: PathBuf,
    required_uid: u32,
}

impl MintPaths {
    fn from_env() -> Self {
        MintPaths {
            key_path: PathBuf::from(env_or("REDFLAG_MINT_KEY", DEFAULT_MINT_KEY)),
            journal_path: PathBuf::from(env_or("REDFLAG_MINT_JOURNAL", DEFAULT_MINT_JOURNAL)),
            required_uid: 0,
        }
    }
}

fn log_mint(msg: &str) {
    eprintln!("[SECURITY] [helper] [mint] {}", msg);
}

fn random_bytes(n: usize) -> Result<Vec<u8>, Denial> {
    let mut buf = vec![0u8; n];
    let mut f = fs::File::open("/dev/urandom")
        .map_err(|e| Denial::new(EXIT_INTERNAL, "urandom_open_failed", e.to_string()))?;
    f.read_exact(&mut buf)
        .map_err(|e| Denial::new(EXIT_INTERNAL, "urandom_read_failed", e.to_string()))?;
    Ok(buf)
}

fn new_uuid_v4() -> Result<String, Denial> {
    let mut b = random_bytes(16)?;
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    let h = hex::encode(&b);
    Ok(format!(
        "{}-{}-{}-{}-{}",
        &h[0..8],
        &h[8..12],
        &h[12..16],
        &h[16..20],
        &h[20..32]
    ))
}

// Load the root-owned mint key (hex-encoded 32-byte Ed25519 seed). Refuses a
// key readable by group/other — a loose key is treated as no key at all.
fn load_mint_key(path: &Path, required_uid: u32) -> Result<SigningKey, Denial> {
    validate_trusted_path_as(path, required_uid)
        .map_err(|denial| Denial::new(EXIT_MINT_KEY, "mint_key_trust_failed", denial.detail))?;
    if let Some(parent) = path.parent() {
        validate_trusted_path_as(parent, required_uid).map_err(|denial| {
            Denial::new(EXIT_MINT_KEY, "mint_key_parent_trust_failed", denial.detail)
        })?;
    }
    let meta = fs::symlink_metadata(path).map_err(|e| {
        Denial::new(
            EXIT_MINT_KEY,
            "mint_key_unavailable",
            format!("{}: {}", path.display(), e),
        )
    })?;
    #[cfg(unix)]
    {
        let mode = meta.permissions().mode();
        if mode & 0o077 != 0 {
            return Err(Denial::new(
                EXIT_MINT_KEY,
                "mint_key_permissions_unsafe",
                format!(
                    "{} mode={:o} — must not be group/other accessible",
                    path.display(),
                    mode & 0o777
                ),
            ));
        }
    }
    let mut options = fs::OpenOptions::new();
    options.read(true);
    #[cfg(unix)]
    options.custom_flags(libc::O_NOFOLLOW);
    let mut key_file = options.open(path).map_err(|e| {
        Denial::new(
            EXIT_MINT_KEY,
            "mint_key_read_failed",
            format!("{}: {}", path.display(), e),
        )
    })?;
    let mut raw = String::new();
    key_file.read_to_string(&mut raw).map_err(|e| {
        Denial::new(
            EXIT_MINT_KEY,
            "mint_key_read_failed",
            format!("{}: {}", path.display(), e),
        )
    })?;
    let bytes = hex::decode(raw.trim())
        .map_err(|e| Denial::new(EXIT_MINT_KEY, "mint_key_bad_hex", e.to_string()))?;
    let seed: [u8; 32] = bytes.as_slice().try_into().map_err(|_| {
        Denial::new(
            EXIT_MINT_KEY,
            "mint_key_bad_len",
            format!("len={}", bytes.len()),
        )
    })?;
    Ok(SigningKey::from_bytes(&seed))
}

fn validate_gate_evidence(ev: &GateEvidence, now: i64) -> Result<(), Denial> {
    if ev.operator.trim().is_empty() {
        return Err(Denial::new(EXIT_MINT_GATE, "mint_operator_missing", ""));
    }

    let age = now - ev.resolved_at;
    if !(-MINT_CLOCK_SKEW_SECS..=MINT_EVIDENCE_MAX_AGE_SECS).contains(&age) {
        return Err(Denial::new(
            EXIT_MINT_STALE,
            "mint_evidence_stale",
            format!(
                "resolved_at={} now={} window_secs={}",
                ev.resolved_at, now, MINT_EVIDENCE_MAX_AGE_SECS
            ),
        ));
    }
    if ev.osv_status == "clear" {
        let osv_age = now - ev.osv_checked_at;
        if !(-MINT_CLOCK_SKEW_SECS..=MINT_EVIDENCE_MAX_AGE_SECS).contains(&osv_age) {
            return Err(Denial::new(
                EXIT_MINT_STALE,
                "mint_osv_evidence_stale",
                format!("osv_checked_at={} now={}", ev.osv_checked_at, now),
            ));
        }
    }

    let has_reason = !ev.override_reason.trim().is_empty();
    match ev.osv_status.as_str() {
        "clear" => {}
        "vulnerable" | "unreachable" | "unsupported" => {
            if !has_reason {
                return Err(Denial::new(
                    EXIT_MINT_GATE,
                    "mint_gate_requires_override_reason",
                    format!(
                        "osv_status={} vuln_count={}",
                        ev.osv_status, ev.osv_vuln_count
                    ),
                ));
            }
        }
        other => {
            return Err(Denial::new(
                EXIT_MINT_GATE,
                "mint_osv_status_unknown",
                format!("osv_status={}", other),
            ));
        }
    }
    for (gate, verdict) in [
        ("age_gate", ev.age_gate.as_str()),
        ("soak_gate", ev.soak_gate.as_str()),
    ] {
        match verdict {
            "pass" | "not_applicable" => {}
            "overridden" => {
                if !has_reason {
                    return Err(Denial::new(
                        EXIT_MINT_GATE,
                        "mint_gate_requires_override_reason",
                        format!("{}=overridden", gate),
                    ));
                }
            }
            other => {
                return Err(Denial::new(
                    EXIT_MINT_GATE,
                    "mint_gate_verdict_unknown",
                    format!("{}={}", gate, other),
                ));
            }
        }
    }
    Ok(())
}

fn validate_mint_request(req: &MintRequest, now: i64) -> Result<(), Denial> {
    if req.version != SUPPORTED_TOKEN_VERSION {
        return Err(Denial::new(
            EXIT_VERSION,
            "mint_unsupported_version",
            format!("version={}", req.version),
        ));
    }
    match req.operation.as_str() {
        "install" | "upgrade" => {}
        other => {
            return Err(Denial::new(
                EXIT_UNSUPPORTED_OP,
                "mint_operation_not_allowed",
                format!("operation={} (forward-only: install|upgrade)", other),
            ))
        }
    }
    if !MINTABLE_PACKAGE_TYPES.contains(&req.package_type.as_str()) {
        return Err(Denial::new(
            EXIT_UNSUPPORTED_OP,
            "mint_package_type_not_supported",
            format!("package_type={}", req.package_type),
        ));
    }
    // request_id must be a canonical UUID v4 — this prevents the journal
    // substring-containment dedup check from matching across different IDs
    // (e.g. "req-1" matching inside "req-10") and blocks characters that
    // could interfere with the JSON needle pattern.
    if !is_valid_uuid_v4(&req.request_id) {
        return Err(Denial::new(
            EXIT_BAD_TOKEN,
            "mint_request_id_not_uuid_v4",
            "request_id must be a canonical UUID v4 (8-4-4-4-12 lowercase hex)",
        ));
    }

    // Bind to this host's independently-read identity, same as execute mode.
    let local = local_agent_id()?;
    if local != req.agent_id {
        return Err(Denial::new(
            EXIT_AGENT_MISMATCH,
            "mint_agent_id_mismatch",
            format!("request_agent_id={} host_agent_id={}", req.agent_id, local),
        ));
    }

    // Closure shape: fail-closed on anything the execute path could not verify.
    if req.closure.is_empty() {
        return Err(Denial::new(EXIT_ARTIFACT, "mint_closure_empty", ""));
    }
    for e in &req.closure {
        if e.name.trim().is_empty() || e.version.trim().is_empty() {
            return Err(Denial::new(
                EXIT_ARTIFACT,
                "mint_closure_entry_incomplete",
                format!("name={:?}", e.name),
            ));
        }
        let sha = e.sha256.trim();
        if sha.len() != 64 || !sha.chars().all(|c| c.is_ascii_hexdigit()) {
            return Err(Denial::new(
                EXIT_ARTIFACT,
                "mint_closure_hash_invalid",
                format!("{}@{} sha256={:?}", e.name, e.version, e.sha256),
            ));
        }
    }

    validate_gate_evidence(&req.gate_evidence, now)
}

// Duplicate request_id guard: without it an agent retry would mint two live
// tokens for one approval — two executable authorizations. Journal lines carry
// request_id=<id>, so the journal doubles as the dedupe index.
fn mint_journal_has_request(journal: &Path, request_id: &str) -> bool {
    let needle = format!("\"request_id\":\"{}\"", request_id);
    match fs::read_to_string(journal) {
        Ok(contents) => contents.lines().any(|l| l.contains(&needle)),
        Err(_) => false,
    }
}

// Claim before signing. The journal remains the durable audit, while a
// create-new file provides the atomic one-request/one-authorization decision
// that a read-then-append journal check cannot provide under concurrency.
fn claim_mint_request(paths: &MintPaths, request_id: &str) -> Result<(), Denial> {
    if mint_journal_has_request(&paths.journal_path, request_id) {
        return Err(Denial::new(
            EXIT_MINT_DUPLICATE,
            "mint_request_already_minted",
            format!("request_id={}", request_id),
        ));
    }
    let parent = paths.journal_path.parent().ok_or_else(|| {
        Denial::new(
            EXIT_TRUST_PATH,
            "mint_claim_parent_missing",
            paths.journal_path.display().to_string(),
        )
    })?;
    validate_trusted_path_as(parent, paths.required_uid)?;
    let claims = parent.join("mint-claims");
    match fs::symlink_metadata(&claims) {
        Ok(_) => {}
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            if let Err(error) = fs::create_dir(&claims) {
                if error.kind() != std::io::ErrorKind::AlreadyExists {
                    return Err(Denial::new(
                        EXIT_INTERNAL,
                        "mint_claim_dir_create_failed",
                        format!("{}: {}", claims.display(), error),
                    ));
                }
            }
            #[cfg(unix)]
            fs::set_permissions(&claims, fs::Permissions::from_mode(0o700)).map_err(|error| {
                Denial::new(
                    EXIT_INTERNAL,
                    "mint_claim_dir_chmod_failed",
                    format!("{}: {}", claims.display(), error),
                )
            })?;
        }
        Err(error) => {
            return Err(Denial::new(
                EXIT_INTERNAL,
                "mint_claim_dir_unavailable",
                format!("{}: {}", claims.display(), error),
            ))
        }
    }
    validate_trusted_path_as(&claims, paths.required_uid)?;
    let claim = claims.join(request_id);
    let mut options = fs::OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    options
        .mode(0o600)
        .custom_flags(libc::O_NOFOLLOW | libc::O_CLOEXEC);
    match options.open(&claim) {
        Ok(file) => file.sync_all().map_err(|error| {
            Denial::new(
                EXIT_INTERNAL,
                "mint_claim_sync_failed",
                format!("{}: {}", claim.display(), error),
            )
        }),
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => Err(Denial::new(
            EXIT_MINT_DUPLICATE,
            "mint_request_already_minted",
            format!("request_id={}", request_id),
        )),
        Err(error) => Err(Denial::new(
            EXIT_INTERNAL,
            "mint_claim_create_failed",
            format!("{}: {}", claim.display(), error),
        )),
    }
}

// Journal before token emission: audit precedes authority. Append-only 0640 —
// install provisioning makes the journal dir setgid root:redflag-local, so the
// group inherits read access without the key dir ever loosening.
fn mint_journal_append(journal: &Path, entry: &serde_json::Value) -> Result<(), Denial> {
    if let Some(parent) = journal.parent() {
        fs::create_dir_all(parent).map_err(|e| {
            Denial::new(
                EXIT_INTERNAL,
                "mint_journal_dir_failed",
                format!("{}: {}", parent.display(), e),
            )
        })?;
    }
    let line = format!("{}\n", entry);
    let mut j = fs::OpenOptions::new();
    j.append(true).create(true);
    #[cfg(unix)]
    {
        j.mode(0o640);
    }
    let mut f = j.open(journal).map_err(|e| {
        Denial::new(
            EXIT_INTERNAL,
            "mint_journal_open_failed",
            format!("{}: {}", journal.display(), e),
        )
    })?;
    std::io::Write::write_all(&mut f, line.as_bytes()).map_err(|e| {
        Denial::new(
            EXIT_INTERNAL,
            "mint_journal_write_failed",
            format!("{}: {}", journal.display(), e),
        )
    })
}

fn run_mint(req: &MintRequest, paths: &MintPaths, now: i64) -> Result<MintedToken, Denial> {
    validate_mint_request(req, now)?;
    claim_mint_request(paths, &req.request_id)?;

    let signing_key = load_mint_key(&paths.key_path, paths.required_uid)?;
    let verifying_key = signing_key.verifying_key();
    let key_id = key_id_for(verifying_key.as_bytes());

    let token_id = new_uuid_v4()?;
    let ch = closure_hash(&req.closure);
    let mut token = MintedToken {
        version: SUPPORTED_TOKEN_VERSION,
        token_id: token_id.clone(),
        agent_id: req.agent_id.clone(),
        key_id: key_id.clone(),
        package_type: req.package_type.clone(),
        operation: req.operation.clone(),
        closure: req.closure.clone(),
        issued_at: now,
        not_before: now - MINT_CLOCK_SKEW_SECS,
        expires_at: now + MINT_TOKEN_TTL_SECS,
        signature: String::new(),
    };
    let msg = format!(
        "{}:{}:{}:{}:{}:{}",
        token.agent_id, token.token_id, token.operation, token.package_type, ch, token.expires_at
    );
    token.signature = hex::encode(signing_key.sign(msg.as_bytes()).to_bytes());

    let ev = &req.gate_evidence;
    mint_journal_append(
        &paths.journal_path,
        &serde_json::json!({
            "ts": now,
            "event": "mint",
            "request_id": req.request_id,
            "token_id": token_id,
            "agent_id": req.agent_id,
            "package_type": req.package_type,
            "operation": req.operation,
            "closure_size": req.closure.len(),
            "closure_hash": ch,
            "osv_status": ev.osv_status,
            "osv_vuln_count": ev.osv_vuln_count,
            "age_gate": ev.age_gate,
            "soak_gate": ev.soak_gate,
            "operator": ev.operator,
            "override_reason": ev.override_reason,
            "key_id": key_id,
            "expires_at": token.expires_at,
        }),
    )?;

    log_mint(&format!(
        "minted token_id={} request_id={} package_type={} operation={} closure_size={} osv_status={} operator={}",
        token_id, req.request_id, req.package_type, req.operation, req.closure.len(), ev.osv_status, ev.operator
    ));
    Ok(token)
}

#[cfg(unix)]
fn validate_envelope_mint_request(
    req: &EnvelopeMintRequest,
    now: i64,
) -> Result<Vec<PacmanResolvedAction>, Denial> {
    if req.version != MUTATION_PROTOCOL_VERSION {
        return Err(Denial::new(
            EXIT_VERSION,
            "mint_envelope_unsupported_version",
            format!("version={}", req.version),
        ));
    }
    if !is_valid_uuid_v4(&req.request_id) {
        return Err(Denial::new(
            EXIT_BAD_TOKEN,
            "mint_request_id_not_uuid_v4",
            "request_id must be a canonical UUID v4 (8-4-4-4-12 lowercase hex)",
        ));
    }
    req.manifest
        .validate()
        .map_err(|error| Denial::new(EXIT_BAD_TOKEN, "mint_envelope_manifest_invalid", error))?;
    let local = local_agent_id()?;
    if local != req.manifest.target_id {
        return Err(Denial::new(
            EXIT_AGENT_MISMATCH,
            "mint_envelope_target_mismatch",
            format!(
                "manifest_target_id={} host_agent_id={}",
                req.manifest.target_id, local
            ),
        ));
    }
    let actions = parse_pacman_actions(&req.manifest)?;
    for action in &actions {
        if action.signature_path.is_none() || action.signature_sha256.is_none() {
            return Err(Denial::new(
                EXIT_ARTIFACT,
                "mint_envelope_signature_required",
                format!("identity={}@{}", action.name, action.version),
            ));
        }
    }
    validate_gate_evidence(&req.gate_evidence, now)?;
    Ok(actions)
}

#[cfg(unix)]
fn run_envelope_mint(
    req: &EnvelopeMintRequest,
    paths: &MintPaths,
    now: i64,
) -> Result<MutationEnvelope, Denial> {
    let actions = validate_envelope_mint_request(req, now)?;
    claim_mint_request(paths, &req.request_id)?;

    let signing_key = load_mint_key(&paths.key_path, paths.required_uid)?;
    let verifying_key = signing_key.verifying_key();
    let key_id = key_id_for(verifying_key.as_bytes());
    let authorization_id = new_uuid_v4()?;

    // The local helper is the authority, so it proves the exact Agent-provided
    // bytes against the Arch keyring before it signs their manifest. Execution
    // repeats every check after a fresh root-owned stage.
    let (operation_dir, staged) = stage_pacman_actions_as(
        &actions,
        Path::new(DEFAULT_MUTATION_STAGING_DIR),
        &authorization_id,
        0,
    )?;
    let mut requested_relation = None;
    let staged_result = staged.iter().try_for_each(|action| {
        verify_pacman_archive_identity(action)?;
        verify_pacman_archive_signature(action)?;
        let relation = enforce_forward_only_pacman_action(action)?;
        if action.requested {
            requested_relation = Some(relation);
        }
        Ok(())
    });
    let _ = fs::remove_dir_all(&operation_dir);
    staged_result?;
    require_requested_pacman_transition(&req.manifest.operation, requested_relation)?;

    let manifest_hash = req.manifest.hash();
    let mut authorization = MutationAuthorization {
        protocol_version: MUTATION_PROTOCOL_VERSION,
        authorization_id: authorization_id.clone(),
        manifest_hash: manifest_hash.clone(),
        authority_kind: "standalone".to_string(),
        authority_id: format!("local:{}", key_id),
        target_id: req.manifest.target_id.clone(),
        issued_at: now,
        not_before: now,
        expires_at: now + MINT_TOKEN_TTL_SECS,
        decision: "allow".to_string(),
        key_id: key_id.clone(),
        signature: String::new(),
    };
    authorization.signature = hex::encode(
        signing_key
            .sign(&authorization.canonical_message())
            .to_bytes(),
    );
    let envelope = MutationEnvelope {
        manifest: req.manifest.clone(),
        authorization,
    };
    envelope
        .verify_for_execution_at(&verifying_key, now)
        .map_err(|error| {
            Denial::new(
                EXIT_INTERNAL,
                "mint_envelope_self_verification_failed",
                error,
            )
        })?;

    let ev = &req.gate_evidence;
    mint_journal_append(
        &paths.journal_path,
        &serde_json::json!({
            "ts": now,
            "event": "mint_envelope",
            "request_id": req.request_id,
            "operation_id": req.manifest.operation_id,
            "authorization_id": authorization_id,
            "manifest_hash": manifest_hash,
            "agent_id": req.manifest.target_id,
            "backend": req.manifest.backend,
            "operation": req.manifest.operation,
            "action_count": req.manifest.resolved_actions.len(),
            "osv_status": ev.osv_status,
            "osv_vuln_count": ev.osv_vuln_count,
            "age_gate": ev.age_gate,
            "soak_gate": ev.soak_gate,
            "operator": ev.operator,
            "override_reason": ev.override_reason,
            "key_id": key_id,
            "expires_at": envelope.authorization.expires_at,
        }),
    )?;
    log_mint(&format!(
        "minted envelope authorization_id={} request_id={} backend={} operation={} actions={} osv_status={} operator={}",
        envelope.authorization.authorization_id,
        req.request_id,
        req.manifest.backend,
        req.manifest.operation,
        req.manifest.resolved_actions.len(),
        ev.osv_status,
        ev.operator
    ));
    Ok(envelope)
}

#[cfg(unix)]
fn run_mint_envelope_cli(args: &[String]) -> i32 {
    if args.len() != 4 || args[0] != "--request-file" || args[2] != "--envelope-out" {
        log_error("mint-envelope usage: redflag-helper mint-envelope --request-file <path> --envelope-out <path>");
        return EXIT_BAD_TOKEN;
    }
    let request_file = &args[1];
    let envelope_out = &args[3];
    let req: EnvelopeMintRequest =
        match read_agent_exchange_file(request_file, DEFAULT_AGENT_MUTATION_REQUEST_DIR).and_then(
            |buffer| {
                serde_json::from_str(&buffer).map_err(|error| {
                    Denial::new(
                        EXIT_BAD_TOKEN,
                        "mint_envelope_request_parse_failed",
                        error.to_string(),
                    )
                })
            },
        ) {
            Ok(request) => request,
            Err(denial) => {
                log_mint(&format!(
                    "denied reason={} detail={} exit={}",
                    denial.reason, denial.detail, denial.code
                ));
                return denial.code;
            }
        };
    let expected_name = format!("{}.json", req.request_id);
    let expected_request = Path::new(DEFAULT_AGENT_MUTATION_REQUEST_DIR).join(&expected_name);
    let expected_envelope = Path::new(DEFAULT_AGENT_MUTATION_ENVELOPE_DIR).join(&expected_name);
    if Path::new(request_file) != expected_request || Path::new(envelope_out) != expected_envelope {
        log_mint("denied reason=mint_envelope_exchange_join_mismatch exit=26");
        return EXIT_TRUST_PATH;
    }
    if let Err(denial) =
        validate_agent_exchange_path(envelope_out, DEFAULT_AGENT_MUTATION_ENVELOPE_DIR)
    {
        log_mint(&format!(
            "denied reason={} detail={} exit={}",
            denial.reason, denial.detail, denial.code
        ));
        return denial.code;
    }

    match run_envelope_mint(&req, &MintPaths::from_env(), now_unix()) {
        Ok(envelope) => {
            if let Err(denial) = write_agent_exchange_json(
                &envelope,
                envelope_out,
                DEFAULT_AGENT_MUTATION_ENVELOPE_DIR,
            ) {
                log_error(&format!(
                    "mint_envelope_write_failed reason={} detail={}",
                    denial.reason, denial.detail
                ));
                return denial.code;
            }
            EXIT_OK
        }
        Err(denial) => {
            log_mint(&format!(
                "denied reason={} detail={} exit={}",
                denial.reason, denial.detail, denial.code
            ));
            denial.code
        }
    }
}

// mint --init-key: generate the local authority keypair. Private seed (hex)
// 0600 at the key path; public half installed into the keyring dir as
// authority_local.pub so the execute path trusts what mint signs.
fn run_mint_init_key(paths: &MintPaths) -> Result<(), Denial> {
    if paths.key_path.exists() {
        return Err(Denial::new(
            EXIT_MINT_KEY,
            "mint_key_already_exists",
            format!(
                "{} — retire it first; refusing to overwrite an authority",
                paths.key_path.display()
            ),
        ));
    }
    let seed_bytes = random_bytes(32)?;
    let seed: [u8; 32] = seed_bytes.as_slice().try_into().unwrap();
    let signing_key = SigningKey::from_bytes(&seed);
    let verifying_key = signing_key.verifying_key();
    let key_id = key_id_for(verifying_key.as_bytes());

    if let Some(parent) = paths.key_path.parent() {
        fs::create_dir_all(parent).map_err(|e| {
            Denial::new(
                EXIT_INTERNAL,
                "mint_key_dir_failed",
                format!("{}: {}", parent.display(), e),
            )
        })?;
    }
    let mut k = fs::OpenOptions::new();
    k.write(true).create_new(true);
    #[cfg(unix)]
    {
        k.mode(0o600);
    }
    let mut f = k.open(&paths.key_path).map_err(|e| {
        Denial::new(
            EXIT_MINT_KEY,
            "mint_key_create_failed",
            format!("{}: {}", paths.key_path.display(), e),
        )
    })?;
    std::io::Write::write_all(&mut f, hex::encode(seed).as_bytes())
        .map_err(|e| Denial::new(EXIT_MINT_KEY, "mint_key_write_failed", e.to_string()))?;

    let keyring_dir = PathBuf::from(env_or("REDFLAG_HELPER_KEYRING", DEFAULT_KEYRING_DIR));
    fs::create_dir_all(&keyring_dir).map_err(|e| {
        Denial::new(
            EXIT_INTERNAL,
            "keyring_dir_failed",
            format!("{}: {}", keyring_dir.display(), e),
        )
    })?;
    let pub_path = keyring_dir.join("authority_local.pub");
    fs::write(&pub_path, hex::encode(verifying_key.as_bytes())).map_err(|e| {
        Denial::new(
            EXIT_INTERNAL,
            "keyring_pub_write_failed",
            format!("{}: {}", pub_path.display(), e),
        )
    })?;

    mint_journal_append(
        &paths.journal_path,
        &serde_json::json!({ "ts": now_unix(), "event": "authority_created", "key_id": key_id }),
    )?;
    log_mint(&format!(
        "authority_created key_id={} pub={}",
        key_id,
        pub_path.display()
    ));
    println!(
        "{}",
        serde_json::json!({ "key_id": key_id, "public_key_file": pub_path.to_string_lossy() })
    );
    Ok(())
}

// mint --retire-key: destroy the local authority (fleet join). Overwrite the
// seed before unlink so the hex isn't left on disk, remove the public half
// from the keyring, journal the retirement.
fn run_mint_retire_key(paths: &MintPaths) -> Result<(), Denial> {
    let meta = fs::metadata(&paths.key_path).map_err(|e| {
        Denial::new(
            EXIT_MINT_KEY,
            "mint_key_unavailable",
            format!("{}: {}", paths.key_path.display(), e),
        )
    })?;
    let zeros = vec![b'0'; meta.len() as usize];
    fs::write(&paths.key_path, &zeros)
        .map_err(|e| Denial::new(EXIT_INTERNAL, "mint_key_scrub_failed", e.to_string()))?;
    fs::remove_file(&paths.key_path)
        .map_err(|e| Denial::new(EXIT_INTERNAL, "mint_key_remove_failed", e.to_string()))?;

    let keyring_dir = PathBuf::from(env_or("REDFLAG_HELPER_KEYRING", DEFAULT_KEYRING_DIR));
    let pub_path = keyring_dir.join("authority_local.pub");
    if pub_path.exists() {
        if let Err(e) = fs::remove_file(&pub_path) {
            log_error(&format!(
                "keyring_pub_remove_failed path={} error={}",
                pub_path.display(),
                e
            ));
        }
    }

    mint_journal_append(
        &paths.journal_path,
        &serde_json::json!({ "ts": now_unix(), "event": "authority_retired" }),
    )?;
    log_mint("authority_retired");
    Ok(())
}

// CLI: redflag-helper mint --request-file <p> --token-out <p>
//      redflag-helper mint --init-key
//      redflag-helper mint --retire-key
fn run_mint_cli(args: &[String]) -> i32 {
    let paths = MintPaths::from_env();

    match args.first().map(|s| s.as_str()) {
        Some("--init-key") if args.len() == 1 => {
            return match run_mint_init_key(&paths) {
                Ok(()) => EXIT_OK,
                Err(d) => {
                    log_mint(&format!(
                        "denied reason={} detail={} exit={}",
                        d.reason, d.detail, d.code
                    ));
                    d.code
                }
            }
        }
        Some("--retire-key") if args.len() == 1 => {
            return match run_mint_retire_key(&paths) {
                Ok(()) => EXIT_OK,
                Err(d) => {
                    log_mint(&format!(
                        "denied reason={} detail={} exit={}",
                        d.reason, d.detail, d.code
                    ));
                    d.code
                }
            }
        }
        _ => {}
    }

    if args.len() != 4 || args[0] != "--request-file" || args[2] != "--token-out" {
        log_error("mint usage: redflag-helper mint --request-file <path> --token-out <path> | --init-key | --retire-key");
        return EXIT_BAD_TOKEN;
    }
    let request_file = &args[1];
    let token_out = &args[3];

    #[cfg(unix)]
    let request = read_agent_exchange_file(request_file, DEFAULT_AGENT_MINT_REQUEST_DIR);
    #[cfg(not(unix))]
    let request = fs::read_to_string(request_file).map_err(|e| {
        Denial::new(
            EXIT_BAD_TOKEN,
            "mint_request_read_failed",
            format!("{}: {}", request_file, e),
        )
    });
    let req: MintRequest = match request.and_then(|buf| {
        serde_json::from_str(&buf)
            .map_err(|e| Denial::new(EXIT_BAD_TOKEN, "mint_request_parse_failed", e.to_string()))
    }) {
        Ok(r) => r,
        Err(d) => {
            log_mint(&format!(
                "denied reason={} detail={} exit={}",
                d.reason, d.detail, d.code
            ));
            return d.code;
        }
    };

    #[cfg(unix)]
    {
        let expected_name = format!("{}.json", req.request_id);
        let expected_request = Path::new(DEFAULT_AGENT_MINT_REQUEST_DIR).join(&expected_name);
        let expected_token = Path::new(DEFAULT_AGENT_TOKEN_DIR).join(&expected_name);
        if Path::new(request_file) != expected_request || Path::new(token_out) != expected_token {
            log_mint("denied reason=mint_exchange_join_mismatch exit=26");
            return EXIT_TRUST_PATH;
        }
    }

    match run_mint(&req, &paths, now_unix()) {
        Ok(token) => {
            #[cfg(unix)]
            {
                if let Err(denial) =
                    write_agent_exchange_json(&token, token_out, DEFAULT_AGENT_TOKEN_DIR)
                {
                    log_error(&format!(
                        "mint_token_write_failed reason={} detail={}",
                        denial.reason, denial.detail
                    ));
                    return denial.code;
                }
            }
            #[cfg(not(unix))]
            emit_result_to_file(&token, token_out);
            EXIT_OK
        }
        Err(d) => {
            log_mint(&format!(
                "denied request_id={} reason={} detail={} exit={}",
                req.request_id, d.reason, d.detail, d.code
            ));
            d.code
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn entry(name: &str, version: &str, sha: &str) -> ClosureEntry {
        ClosureEntry {
            name: name.to_string(),
            version: version.to_string(),
            sha256: sha.to_string(),
            source: String::new(),
            artifact_path: None,
        }
    }

    // ---- SEC-021 trust-path validation ----
    // Tests run unprivileged, so they exercise validate_trusted_path_as with
    // the test user's own uid (obtained from a file the test just created);
    // production calls pin required_uid to 0.

    fn trust_fixture(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "redflag-trust-test-{}-{}",
            name,
            std::process::id()
        ));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        fs::set_permissions(&dir, fs::Permissions::from_mode(0o700)).unwrap();
        dir
    }

    fn own_uid(dir: &Path) -> u32 {
        fs::symlink_metadata(dir).unwrap().uid()
    }

    #[test]
    fn trusted_path_accepts_owned_unwritable() {
        let dir = trust_fixture("ok");
        let file = dir.join("agent_id");
        fs::write(&file, "abc").unwrap();
        fs::set_permissions(&file, fs::Permissions::from_mode(0o644)).unwrap();
        let uid = own_uid(&dir);

        assert!(validate_trusted_path_as(&file, uid).is_ok());
        assert!(validate_trusted_path_as(&dir, uid).is_ok());
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn trusted_path_rejects_group_or_other_writable() {
        let dir = trust_fixture("writable");
        let file = dir.join("key.pub");
        fs::write(&file, "aa").unwrap();
        let uid = own_uid(&dir);

        for mode in [0o664u32, 0o646, 0o666] {
            fs::set_permissions(&file, fs::Permissions::from_mode(mode)).unwrap();
            let denial = validate_trusted_path_as(&file, uid).unwrap_err();
            assert_eq!(
                denial.code, EXIT_TRUST_PATH,
                "mode {:o} must be refused",
                mode
            );
            assert_eq!(denial.reason, "trusted_path_writable");
        }
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn trusted_path_rejects_wrong_owner() {
        let dir = trust_fixture("owner");
        let file = dir.join("agent_id");
        fs::write(&file, "abc").unwrap();
        fs::set_permissions(&file, fs::Permissions::from_mode(0o644)).unwrap();
        let uid = own_uid(&dir);

        // Demand a uid this unprivileged test cannot own (root unless running as root).
        let other = if uid == 0 { 12345 } else { 0 };
        let denial = validate_trusted_path_as(&file, other).unwrap_err();
        assert_eq!(denial.code, EXIT_TRUST_PATH);
        assert_eq!(denial.reason, "trusted_path_wrong_owner");
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn trusted_path_rejects_symlink() {
        let dir = trust_fixture("symlink");
        let target = dir.join("real");
        let link = dir.join("link.pub");
        fs::write(&target, "aa").unwrap();
        std::os::unix::fs::symlink(&target, &link).unwrap();
        let uid = own_uid(&dir);

        let denial = validate_trusted_path_as(&link, uid).unwrap_err();
        assert_eq!(denial.code, EXIT_TRUST_PATH);
        assert_eq!(denial.reason, "trusted_path_symlink");
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn trusted_path_rejects_missing() {
        let dir = trust_fixture("missing");
        let uid = own_uid(&dir);
        let denial = validate_trusted_path_as(&dir.join("nope"), uid).unwrap_err();
        assert_eq!(denial.code, EXIT_TRUST_PATH);
        assert_eq!(denial.reason, "trusted_path_unreadable");
        let _ = fs::remove_dir_all(&dir);
    }

    // Cross-language contract vector. These exact strings are produced by the Go
    // capability package for the same input; if either side changes, this breaks.
    #[test]
    fn canonical_matches_go() {
        let token = CapabilityToken {
            version: 1,
            token_id: "tok-1".into(),
            agent_id: "agent-123".into(),
            key_id: String::new(),
            package_type: "npm".into(),
            operation: "install".into(),
            closure: vec![
                entry("left-pad", "1.3.0", "aaaa"),
                entry("is-odd", "2.0.0", "bbbb"),
            ],
            issued_at: 0,
            not_before: 0,
            expires_at: 1700000000,
            signature: String::new(),
        };
        let ch = closure_hash(&token.closure);
        assert_eq!(
            ch,
            "49a181cd7b6df83a6dc83c7f647f2d224effe90907dd5dcdfdaf4c8697af595f"
        );
        assert_eq!(
            signed_message(&token, &ch),
            "agent-123:tok-1:install:npm:49a181cd7b6df83a6dc83c7f647f2d224effe90907dd5dcdfdaf4c8697af595f:1700000000"
        );
    }

    // Closure ordering must not change the digest.
    #[test]
    fn closure_hash_order_independent() {
        let a = vec![entry("a", "1", "x"), entry("b", "2", "y")];
        let b = vec![entry("b", "2", "y"), entry("a", "1", "x")];
        assert_eq!(closure_hash(&a), closure_hash(&b));
    }

    // ---- mint mode ----

    const TEST_SHA: &str = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

    struct MintFixture {
        dir: PathBuf,
        paths: MintPaths,
        verifying_key: VerifyingKey,
    }

    impl Drop for MintFixture {
        fn drop(&mut self) {
            let _ = fs::remove_dir_all(&self.dir);
        }
    }

    fn mint_fixture(name: &str) -> MintFixture {
        let dir =
            std::env::temp_dir().join(format!("redflag-mint-test-{}-{}", name, std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();

        let seed = [7u8; 32];
        let signing_key = SigningKey::from_bytes(&seed);
        let key_path = dir.join("authority_local.key");
        {
            let mut f = fs::OpenOptions::new()
                .write(true)
                .create_new(true)
                .mode(0o600)
                .open(&key_path)
                .unwrap();
            std::io::Write::write_all(&mut f, hex::encode(seed).as_bytes()).unwrap();
        }
        // Host identity for the agent_id bind check.
        std::env::set_var("REDFLAG_AGENT_ID", "agent-123");

        MintFixture {
            paths: MintPaths {
                key_path,
                journal_path: dir.join("mint.log"),
                required_uid: fs::symlink_metadata(&dir).unwrap().uid(),
            },
            verifying_key: signing_key.verifying_key(),
            dir,
        }
    }

    fn good_request(request_id: &str, now: i64) -> MintRequest {
        MintRequest {
            version: 1,
            request_id: request_id.to_string(),
            agent_id: "agent-123".to_string(),
            package_type: "dnf".to_string(),
            operation: "install".to_string(),
            closure: vec![ClosureEntry {
                name: "hyprutils".into(),
                version: "0.2.4-1.fc43".into(),
                sha256: TEST_SHA.into(),
                source: "registry".into(),
                artifact_path: None,
            }],
            gate_evidence: GateEvidence {
                resolved_at: now - 60,
                osv_checked_at: now - 60,
                osv_status: "clear".into(),
                osv_vuln_count: 0,
                age_gate: "pass".into(),
                soak_gate: "pass".into(),
                operator: "casey".into(),
                override_reason: String::new(),
            },
        }
    }

    // Valid UUID v4 strings for test request_ids.
    const UUID_1: &str = "550e8400-e29b-41d4-a716-446655440000";
    const UUID_2: &str = "550e8400-e29b-41d4-a716-446655440001";
    const UUID_3: &str = "550e8400-e29b-41d4-a716-446655440002";
    const UUID_4: &str = "550e8400-e29b-41d4-a716-446655440003";
    const UUID_5: &str = "550e8400-e29b-41d4-a716-446655440004";
    const UUID_6: &str = "550e8400-e29b-41d4-a716-446655440005";
    const UUID_7: &str = "550e8400-e29b-41d4-a716-446655440006";

    // The full loop: mint a token, then verify it with the exact same functions
    // the execute path uses. If this passes, a minted token is executable.
    #[test]
    fn mint_round_trips_through_execute_verification() {
        let fx = mint_fixture("roundtrip");
        let now = now_unix();
        let token =
            run_mint(&good_request(UUID_1, now), &fx.paths, now).expect("mint should succeed");

        assert_eq!(token.version, SUPPORTED_TOKEN_VERSION);
        assert_eq!(token.key_id, key_id_for(fx.verifying_key.as_bytes()));
        assert!(token.expires_at > now && token.not_before < now);

        let ch = closure_hash(&token.closure);
        let msg = format!(
            "{}:{}:{}:{}:{}:{}",
            token.agent_id,
            token.token_id,
            token.operation,
            token.package_type,
            ch,
            token.expires_at
        );
        let sig = Signature::from_slice(&hex::decode(&token.signature).unwrap()).unwrap();
        fx.verifying_key
            .verify(msg.as_bytes(), &sig)
            .expect("execute-path verification must accept");

        // And the wire shape parses as the execute path's CapabilityToken.
        let json = serde_json::to_string(&token).unwrap();
        let parsed: CapabilityToken = serde_json::from_str(&json).expect("wire-compatible");
        assert_eq!(parsed.token_id, token.token_id);

        // Journaled before emission.
        let journal = fs::read_to_string(&fx.paths.journal_path).unwrap();
        assert!(journal.contains(&format!("\"request_id\":\"{}\"", UUID_1)));
        assert!(journal.contains(&token.token_id));
    }

    #[test]
    fn mint_rejects_stale_evidence() {
        let fx = mint_fixture("stale");
        let now = now_unix();
        let mut req = good_request(UUID_2, now);
        req.gate_evidence.resolved_at = now - MINT_EVIDENCE_MAX_AGE_SECS - 1;
        let d = run_mint(&req, &fx.paths, now).unwrap_err();
        assert_eq!(d.code, EXIT_MINT_STALE);
    }

    #[test]
    fn mint_rejects_future_dated_evidence() {
        let fx = mint_fixture("future");
        let now = now_unix();
        let mut req = good_request(UUID_3, now);
        req.gate_evidence.resolved_at = now + MINT_CLOCK_SKEW_SECS + 30;
        let d = run_mint(&req, &fx.paths, now).unwrap_err();
        assert_eq!(d.code, EXIT_MINT_STALE);
    }

    #[test]
    fn mint_vulnerable_requires_override_reason() {
        let fx = mint_fixture("vuln");
        let now = now_unix();
        let mut req = good_request(UUID_4, now);
        req.gate_evidence.osv_status = "vulnerable".into();
        req.gate_evidence.osv_vuln_count = 2;
        let d = run_mint(&req, &fx.paths, now).unwrap_err();
        assert_eq!(d.code, EXIT_MINT_GATE);

        req.gate_evidence.override_reason = "CVE-2026-0001 not reachable in our deployment".into();
        let token = run_mint(&req, &fx.paths, now).expect("override with reason mints");
        let journal = fs::read_to_string(&fx.paths.journal_path).unwrap();
        assert!(journal.contains("not reachable in our deployment"));
        assert!(journal.contains(&token.token_id));
    }

    #[test]
    fn mint_duplicate_request_id_denied() {
        let fx = mint_fixture("dup");
        let now = now_unix();
        run_mint(&good_request(UUID_5, now), &fx.paths, now).expect("first mint");
        let d = run_mint(&good_request(UUID_5, now), &fx.paths, now).unwrap_err();
        assert_eq!(d.code, EXIT_MINT_DUPLICATE);
        assert!(fx
            .paths
            .journal_path
            .parent()
            .unwrap()
            .join("mint-claims")
            .join(UUID_5)
            .is_file());
    }

    #[test]
    fn mint_loose_key_permissions_denied() {
        let fx = mint_fixture("perms");
        let now = now_unix();
        fs::set_permissions(&fx.paths.key_path, fs::Permissions::from_mode(0o644)).unwrap();
        let d = run_mint(&good_request(UUID_6, now), &fx.paths, now).unwrap_err();
        assert_eq!(d.code, EXIT_MINT_KEY);
    }

    #[test]
    fn mint_rejects_agent_self_and_bad_hashes() {
        let fx = mint_fixture("shape");
        let now = now_unix();

        let mut req = good_request(UUID_7, now);
        req.package_type = AGENT_SELF_PACKAGE_TYPE.to_string();
        assert_eq!(
            run_mint(&req, &fx.paths, now).unwrap_err().code,
            EXIT_UNSUPPORTED_OP
        );

        let mut req = good_request(UUID_7, now);
        req.closure[0].sha256 = "deadbeef".into();
        assert_eq!(
            run_mint(&req, &fx.paths, now).unwrap_err().code,
            EXIT_ARTIFACT
        );

        let mut req = good_request(UUID_7, now);
        req.operation = "remove".into();
        assert_eq!(
            run_mint(&req, &fx.paths, now).unwrap_err().code,
            EXIT_UNSUPPORTED_OP
        );
    }

    #[test]
    fn mint_rejects_package_types_outside_local_api() {
        let fx = mint_fixture("package-types");
        let now = now_unix();
        for package_type in ["npm", "bun", "pip", "docker", "winget", "pacman"] {
            let mut req = good_request(UUID_7, now);
            req.package_type = package_type.to_string();
            let denial = run_mint(&req, &fx.paths, now)
                .expect_err("legacy standalone mint must match the local API allowlist");
            assert_eq!(denial.code, EXIT_UNSUPPORTED_OP, "{package_type}");
        }
    }
    // ---- verify-envelope (ARCH-002 cut 2 compatibility path) ----
    // Verification-only still refuses everything. These pin the refusal order,
    // and that inspection never consumes the executor's replay claim.

    const TEST_TARGET: &str = "agent-123";
    const ENVELOPE_NOW: i64 = 1_700_000_100;

    fn envelope_json(
        target: &str,
        authorization_id: &str,
        version: u32,
        expires_at: i64,
    ) -> String {
        format!(
            r#"{{"manifest":{{"protocol_version":{version},"operation_id":"550e8400-e29b-41d4-a716-446655440010","target_id":"{target}","backend":"pacman","operation":"install","resolved_actions":[{{"kind":"package","identity":"acl@2.3.2-1","payload":"{{\"repository\":\"core\"}}"}}],"evidence":[]}},"authorization":{{"protocol_version":{version},"authorization_id":"{authorization_id}","manifest_hash":"","authority_kind":"fleet-server","authority_id":"redflag-prod","target_id":"{target}","issued_at":1700000000,"not_before":1700000001,"expires_at":{expires_at},"decision":"allow","key_id":"00000000000000000000000000000000","signature":""}}}}"#
        )
    }

    fn write_envelope(name: &str, body: &str) -> PathBuf {
        let dir = trust_fixture(name);
        let path = dir.join("envelope.json");
        fs::write(&path, body).unwrap();
        path
    }

    const GOOD_AUTHORIZATION_ID: &str = "550e8400-e29b-41d4-a716-446655440011";

    #[test]
    fn envelope_parse_failure_denies_without_identity() {
        let path = write_envelope("env-parse", "{ not json");
        let (envelope, denial) = run_envelope(path.to_str().unwrap(), ENVELOPE_NOW);
        assert!(envelope.is_none());
        assert_eq!(denial.code, EXIT_BAD_TOKEN);
        assert_eq!(denial.reason, "envelope_parse_failed");
    }

    #[test]
    fn envelope_unknown_version_fails_closed() {
        let path = write_envelope(
            "env-version",
            &envelope_json(TEST_TARGET, GOOD_AUTHORIZATION_ID, 2, 1_700_000_600),
        );
        let denial = run_envelope(path.to_str().unwrap(), ENVELOPE_NOW).1;
        assert_eq!(denial.code, EXIT_VERSION);
        assert_eq!(denial.reason, "unsupported_envelope_version");
    }

    #[test]
    fn envelope_authorization_id_shape_is_refused() {
        let path = write_envelope(
            "env-uuid",
            &envelope_json(TEST_TARGET, "req-1", 1, 1_700_000_600),
        );
        let denial = run_envelope(path.to_str().unwrap(), ENVELOPE_NOW).1;
        assert_eq!(denial.code, EXIT_BAD_TOKEN);
        assert_eq!(denial.reason, "authorization_id_not_uuid_v4");
    }

    #[test]
    fn envelope_target_must_match_host_identity() {
        std::env::set_var("REDFLAG_AGENT_ID", TEST_TARGET);
        let path = write_envelope(
            "env-target",
            &envelope_json("some-other-body", GOOD_AUTHORIZATION_ID, 1, 1_700_000_600),
        );
        let denial = run_envelope(path.to_str().unwrap(), ENVELOPE_NOW).1;
        assert_eq!(denial.code, EXIT_AGENT_MISMATCH);
        assert_eq!(denial.reason, "envelope_target_mismatch");
    }

    #[test]
    fn envelope_lifetime_ceiling_is_enforced() {
        std::env::set_var("REDFLAG_AGENT_ID", TEST_TARGET);
        let over = 1_700_000_001 + mutation_protocol::MAX_AUTHORIZATION_LIFETIME_SECS + 1;
        let path = write_envelope(
            "env-lifetime",
            &envelope_json(TEST_TARGET, GOOD_AUTHORIZATION_ID, 1, over),
        );
        let denial = run_envelope(path.to_str().unwrap(), ENVELOPE_NOW).1;
        assert_eq!(denial.code, EXIT_TIME_WINDOW);
        assert_eq!(denial.reason, "authorization_lifetime_exceeded");
    }

    #[test]
    fn envelope_timestamp_arithmetic_fails_closed() {
        std::env::set_var("REDFLAG_AGENT_ID", TEST_TARGET);
        let body = envelope_json(TEST_TARGET, GOOD_AUTHORIZATION_ID, 1, i64::MAX).replace(
            "\"not_before\":1700000001",
            &format!("\"not_before\":{}", i64::MIN),
        );
        let path = write_envelope("env-time-overflow", &body);
        let denial = run_envelope(path.to_str().unwrap(), ENVELOPE_NOW).1;
        assert_eq!(denial.code, EXIT_TIME_WINDOW);
        assert_eq!(denial.reason, "authorization_time_window_invalid");
    }

    #[test]
    fn envelope_backend_payload_must_be_a_json_object() {
        let manifest = MutationManifest {
            protocol_version: 1,
            operation_id: "op".into(),
            target_id: TEST_TARGET.into(),
            backend: "pacman".into(),
            operation: "install".into(),
            resolved_actions: vec![mutation_protocol::ResolvedAction {
                kind: "package".into(),
                identity: "acl@2.3.2-1".into(),
                payload: "[\"core\"]".into(),
            }],
            evidence: vec![],
        };
        let denial = validate_backend_payload_shape(&manifest).unwrap_err();
        assert_eq!(denial.code, EXIT_BAD_TOKEN);
        assert_eq!(denial.reason, "backend_payload_not_object");
    }

    // A well-formed envelope walks the real verification pipeline as far as the
    // trusted keyring, and inspection never burns an execution replay slot.
    #[test]
    fn envelope_reaches_the_keyring_and_never_records_replay() {
        std::env::set_var("REDFLAG_AGENT_ID", TEST_TARGET);
        let dir = trust_fixture("env-keyring");
        let state = dir.join("consumed-tokens");
        std::env::set_var("REDFLAG_HELPER_KEYRING", dir.join("trusted-keys"));
        std::env::set_var("REDFLAG_HELPER_STATE", &state);

        let path = write_envelope(
            "env-pipeline",
            &envelope_json(TEST_TARGET, GOOD_AUTHORIZATION_ID, 1, 1_700_000_600),
        );
        let denial = run_envelope(path.to_str().unwrap(), ENVELOPE_NOW).1;

        // Unprivileged tests cannot own a root keyring, so the keyring step is
        // where a valid envelope stops — which is the ordering being asserted.
        assert!(
            denial.code == EXIT_TRUST_PATH || denial.code == EXIT_KEY_NOT_FOUND,
            "expected to reach the keyring, got {} ({})",
            denial.code,
            denial.reason
        );
        assert!(!state.exists(), "envelope path wrote a replay record");

        std::env::remove_var("REDFLAG_HELPER_KEYRING");
        std::env::remove_var("REDFLAG_HELPER_STATE");
        let _ = fs::remove_dir_all(&dir);
    }
}

// ============================================================================
// mutation-envelope verification and pacman execution (ARCH-002)
//
// Common verification remains independent of backend execution. The legacy
// verify-envelope command proves and refuses without consuming replay state;
// execute-envelope admits only pacman and produces a joined receipt.
//
// Pacman execution copies signed cache paths into root-owned custody, verifies
// the signed hash and archive identity, atomically consumes authorization_id,
// and invokes one fixed pacman -U argv. Other backends still fail closed.
// ============================================================================

fn read_envelope_from_file(path: &str) -> Result<MutationEnvelope, Denial> {
    #[cfg(unix)]
    if Path::new(path).parent() == Some(Path::new(DEFAULT_AGENT_MUTATION_ENVELOPE_DIR)) {
        let raw = read_agent_exchange_file(path, DEFAULT_AGENT_MUTATION_ENVELOPE_DIR)?;
        return serde_json::from_str(&raw)
            .map_err(|e| Denial::new(EXIT_BAD_TOKEN, "envelope_parse_failed", e.to_string()));
    }
    let mut options = fs::OpenOptions::new();
    options.read(true);
    #[cfg(unix)]
    options.custom_flags(libc::O_NOFOLLOW);
    let mut file = options.open(path).map_err(|e| {
        Denial::new(
            EXIT_BAD_TOKEN,
            "envelope_file_open_failed",
            format!("{}: {}", path, e),
        )
    })?;
    let mut raw = String::new();
    file.read_to_string(&mut raw).map_err(|e| {
        Denial::new(
            EXIT_BAD_TOKEN,
            "envelope_file_read_failed",
            format!("{}: {}", path, e),
        )
    })?;
    serde_json::from_str(&raw)
        .map_err(|e| Denial::new(EXIT_BAD_TOKEN, "envelope_parse_failed", e.to_string()))
}

// Enough shape to fail closed without inventing backend semantics: the common
// layer never reads inside a payload, but it refuses one that could not be a
// backend action in the first place.
fn validate_backend_payload_shape(manifest: &MutationManifest) -> Result<(), Denial> {
    for (index, action) in manifest.resolved_actions.iter().enumerate() {
        match serde_json::from_str::<serde_json::Value>(&action.payload) {
            Ok(serde_json::Value::Object(_)) => {}
            Ok(_) => {
                return Err(Denial::new(
                    EXIT_BAD_TOKEN,
                    "backend_payload_not_object",
                    format!("resolved action {} identity={}", index, action.identity),
                ))
            }
            Err(e) => {
                return Err(Denial::new(
                    EXIT_BAD_TOKEN,
                    "backend_payload_not_json",
                    format!("resolved action {}: {}", index, e),
                ))
            }
        }
    }
    Ok(())
}

#[cfg(unix)]
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct PacmanExecutionLocation {
    kind: String,
    value: String,
}

#[cfg(unix)]
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct PacmanActionPayload {
    artifact_sha256: String,
    execution_location: PacmanExecutionLocation,
    repository: String,
    #[serde(default)]
    requested: bool,
    #[serde(default)]
    signature_sha256: Option<String>,
    #[serde(default)]
    signature_location: Option<PacmanExecutionLocation>,
}

#[cfg(unix)]
#[derive(Debug)]
struct PacmanResolvedAction {
    name: String,
    version: String,
    requested: bool,
    artifact_sha256: String,
    source_path: PathBuf,
    signature_sha256: Option<String>,
    signature_path: Option<PathBuf>,
}

#[cfg(unix)]
#[derive(Debug)]
struct PacmanStagedAction {
    name: String,
    version: String,
    requested: bool,
    path: PathBuf,
    signature_path: Option<PathBuf>,
}

#[cfg(unix)]
fn plain_pacman_name(value: &str) -> bool {
    !value.is_empty()
        && !value.starts_with('-')
        && value.bytes().all(|byte| {
            byte.is_ascii_alphanumeric() || matches!(byte, b'+' | b'-' | b'.' | b'_' | b'@')
        })
}

#[cfg(unix)]
fn plain_pacman_version(value: &str) -> bool {
    !value.is_empty()
        && !value.starts_with('-')
        && value.bytes().all(|byte| {
            byte.is_ascii_alphanumeric() || matches!(byte, b'+' | b'-' | b'.' | b'_' | b':' | b'~')
        })
}

#[cfg(unix)]
fn plain_pacman_repository(value: &str) -> bool {
    !value.is_empty()
        && !value.starts_with('-')
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'.' | b'_'))
}

#[cfg(unix)]
fn parse_pacman_actions(manifest: &MutationManifest) -> Result<Vec<PacmanResolvedAction>, Denial> {
    if manifest.backend != "pacman" {
        return Err(Denial::new(
            EXIT_UNSUPPORTED_OP,
            "backend_not_migrated",
            format!(
                "backend={} has no executable path through the envelope yet",
                manifest.backend
            ),
        ));
    }
    if !matches!(manifest.operation.as_str(), "install" | "upgrade") {
        return Err(Denial::new(
            EXIT_UNSUPPORTED_OP,
            "operation_not_allowed",
            format!(
                "pacman operation={} (forward-only: install|upgrade)",
                manifest.operation
            ),
        ));
    }

    let mut identities = BTreeSet::new();
    let mut resolved = Vec::with_capacity(manifest.resolved_actions.len());
    let mut requested_count = 0usize;
    for (index, action) in manifest.resolved_actions.iter().enumerate() {
        if action.kind != "package" {
            return Err(Denial::new(
                EXIT_BAD_TOKEN,
                "pacman_action_kind_invalid",
                format!("resolved action {} kind={}", index, action.kind),
            ));
        }
        if !identities.insert(action.identity.clone()) {
            return Err(Denial::new(
                EXIT_BAD_TOKEN,
                "pacman_duplicate_identity",
                format!("identity={}", action.identity),
            ));
        }
        let (name, version) = action.identity.rsplit_once('@').ok_or_else(|| {
            Denial::new(
                EXIT_BAD_TOKEN,
                "pacman_identity_invalid",
                format!("identity={} expected=name@version", action.identity),
            )
        })?;
        if !plain_pacman_name(name) || !plain_pacman_version(version) {
            return Err(Denial::new(
                EXIT_BAD_TOKEN,
                "pacman_identity_invalid",
                format!("identity={}", action.identity),
            ));
        }

        let payload: PacmanActionPayload = serde_json::from_str(&action.payload).map_err(|e| {
            Denial::new(
                EXIT_BAD_TOKEN,
                "pacman_payload_invalid",
                format!("identity={} error={}", action.identity, e),
            )
        })?;
        if payload.execution_location.kind != "cache" {
            return Err(Denial::new(
                EXIT_BAD_TOKEN,
                "pacman_execution_location_invalid",
                format!(
                    "identity={} kind={} expected=cache",
                    action.identity, payload.execution_location.kind
                ),
            ));
        }
        if !plain_pacman_repository(&payload.repository) {
            return Err(Denial::new(
                EXIT_BAD_TOKEN,
                "pacman_repository_invalid",
                format!(
                    "identity={} repository={}",
                    action.identity, payload.repository
                ),
            ));
        }
        if payload.artifact_sha256.len() != 64
            || !payload
                .artifact_sha256
                .bytes()
                .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
        {
            return Err(Denial::new(
                EXIT_ARTIFACT,
                "pacman_artifact_hash_invalid",
                format!("identity={}", action.identity),
            ));
        }
        let (signature_sha256, signature_path) =
            match (payload.signature_sha256, payload.signature_location) {
                (None, None) => (None, None),
                (Some(hash), Some(location)) => {
                    if location.kind != "cache" {
                        return Err(Denial::new(
                            EXIT_BAD_TOKEN,
                            "pacman_signature_location_invalid",
                            format!(
                                "identity={} kind={} expected=cache",
                                action.identity, location.kind
                            ),
                        ));
                    }
                    if hash.len() != 64
                        || !hash
                            .bytes()
                            .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
                    {
                        return Err(Denial::new(
                            EXIT_ARTIFACT,
                            "pacman_signature_hash_invalid",
                            format!("identity={}", action.identity),
                        ));
                    }
                    let path = PathBuf::from(location.value);
                    if !path.is_absolute() || !path.is_file() {
                        return Err(Denial::new(
                            EXIT_ARTIFACT,
                            "pacman_signature_unavailable",
                            format!("identity={} path={}", action.identity, path.display()),
                        ));
                    }
                    (Some(hash), Some(path))
                }
                _ => {
                    return Err(Denial::new(
                        EXIT_BAD_TOKEN,
                        "pacman_signature_incomplete",
                        format!("identity={}", action.identity),
                    ));
                }
            };
        let source_path = PathBuf::from(&payload.execution_location.value);
        if !source_path.is_absolute() || !source_path.is_file() {
            return Err(Denial::new(
                EXIT_ARTIFACT,
                "pacman_artifact_unavailable",
                format!(
                    "identity={} path={}",
                    action.identity,
                    source_path.display()
                ),
            ));
        }
        let file_name = source_path
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("");
        if !file_name.contains(".pkg.tar.") || file_name.bytes().any(|byte| byte.is_ascii_control())
        {
            return Err(Denial::new(
                EXIT_ARTIFACT,
                "pacman_artifact_name_invalid",
                format!(
                    "identity={} path={}",
                    action.identity,
                    source_path.display()
                ),
            ));
        }
        if let Some(path) = &signature_path {
            let expected = format!("{}.sig", source_path.display());
            if path.to_string_lossy() != expected {
                return Err(Denial::new(
                    EXIT_ARTIFACT,
                    "pacman_signature_name_invalid",
                    format!(
                        "identity={} path={} expected={}",
                        action.identity,
                        path.display(),
                        expected
                    ),
                ));
            }
        }

        if payload.requested {
            requested_count += 1;
        }
        resolved.push(PacmanResolvedAction {
            name: name.to_string(),
            version: version.to_string(),
            requested: payload.requested,
            artifact_sha256: payload.artifact_sha256,
            source_path,
            signature_sha256,
            signature_path,
        });
    }
    if requested_count != 1 {
        return Err(Denial::new(
            EXIT_BAD_TOKEN,
            "pacman_requested_root_invalid",
            format!("requested_roots={} expected=1", requested_count),
        ));
    }
    Ok(resolved)
}

#[cfg(unix)]
fn prepare_mutation_staging_dir(
    root: &Path,
    authorization_id: &str,
    required_uid: u32,
) -> Result<PathBuf, Denial> {
    match fs::symlink_metadata(root) {
        Ok(_) => validate_trusted_path_as(root, required_uid)?,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            fs::create_dir_all(root).map_err(|e| {
                Denial::new(
                    EXIT_INTERNAL,
                    "mutation_staging_root_create_failed",
                    format!("{}: {}", root.display(), e),
                )
            })?;
        }
        Err(e) => {
            return Err(Denial::new(
                EXIT_INTERNAL,
                "mutation_staging_root_unreadable",
                format!("{}: {}", root.display(), e),
            ))
        }
    }
    fs::set_permissions(root, fs::Permissions::from_mode(0o700)).map_err(|e| {
        Denial::new(
            EXIT_INTERNAL,
            "mutation_staging_root_chmod_failed",
            format!("{}: {}", root.display(), e),
        )
    })?;
    validate_trusted_path_as(root, required_uid)?;
    if let Some(parent) = root.parent() {
        validate_trusted_path_as(parent, required_uid)?;
    }

    let operation_dir = root.join(authorization_id);
    if fs::symlink_metadata(&operation_dir).is_ok() {
        validate_trusted_path_as(&operation_dir, required_uid)?;
        fs::remove_dir_all(&operation_dir).map_err(|e| {
            Denial::new(
                EXIT_INTERNAL,
                "mutation_staging_reset_failed",
                format!("{}: {}", operation_dir.display(), e),
            )
        })?;
    }
    fs::create_dir(&operation_dir).map_err(|e| {
        Denial::new(
            EXIT_INTERNAL,
            "mutation_staging_create_failed",
            format!("{}: {}", operation_dir.display(), e),
        )
    })?;
    fs::set_permissions(&operation_dir, fs::Permissions::from_mode(0o700)).map_err(|e| {
        Denial::new(
            EXIT_INTERNAL,
            "mutation_staging_chmod_failed",
            format!("{}: {}", operation_dir.display(), e),
        )
    })?;
    validate_trusted_path_as(&operation_dir, required_uid)?;
    Ok(operation_dir)
}

#[cfg(unix)]
fn stage_pacman_actions_as(
    actions: &[PacmanResolvedAction],
    staging_root: &Path,
    authorization_id: &str,
    required_uid: u32,
) -> Result<(PathBuf, Vec<PacmanStagedAction>), Denial> {
    let operation_dir = prepare_mutation_staging_dir(staging_root, authorization_id, required_uid)?;
    let mut staged = Vec::with_capacity(actions.len());
    for (index, action) in actions.iter().enumerate() {
        let source_name = action
            .source_path
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("package.pkg.tar.zst");
        let destination = operation_dir.join(format!("{:04}-{}", index, source_name));
        let mut source_options = fs::OpenOptions::new();
        source_options.read(true).custom_flags(libc::O_NOFOLLOW);
        let mut source = match source_options.open(&action.source_path) {
            Ok(file) => file,
            Err(e) => {
                let _ = fs::remove_dir_all(&operation_dir);
                return Err(Denial::new(
                    EXIT_ARTIFACT,
                    "pacman_artifact_source_open_failed",
                    format!("{}: {}", action.source_path.display(), e),
                ));
            }
        };
        match source.metadata() {
            Ok(metadata) if metadata.is_file() => {}
            Ok(_) => {
                let _ = fs::remove_dir_all(&operation_dir);
                return Err(Denial::new(
                    EXIT_ARTIFACT,
                    "pacman_artifact_source_not_regular",
                    action.source_path.display().to_string(),
                ));
            }
            Err(e) => {
                let _ = fs::remove_dir_all(&operation_dir);
                return Err(Denial::new(
                    EXIT_ARTIFACT,
                    "pacman_artifact_source_metadata_failed",
                    format!("{}: {}", action.source_path.display(), e),
                ));
            }
        }
        let mut destination_options = fs::OpenOptions::new();
        destination_options.write(true).create_new(true).mode(0o600);
        let mut destination_file = match destination_options.open(&destination) {
            Ok(file) => file,
            Err(e) => {
                let _ = fs::remove_dir_all(&operation_dir);
                return Err(Denial::new(
                    EXIT_ARTIFACT,
                    "pacman_artifact_stage_create_failed",
                    format!("{}: {}", destination.display(), e),
                ));
            }
        };
        if let Err(e) = std::io::copy(&mut source, &mut destination_file) {
            let _ = fs::remove_dir_all(&operation_dir);
            return Err(Denial::new(
                EXIT_ARTIFACT,
                "pacman_artifact_stage_failed",
                format!(
                    "{} -> {}: {}",
                    action.source_path.display(),
                    destination.display(),
                    e
                ),
            ));
        }
        if let Err(e) = destination_file.sync_all() {
            let _ = fs::remove_dir_all(&operation_dir);
            return Err(Denial::new(
                EXIT_INTERNAL,
                "pacman_artifact_stage_sync_failed",
                format!("{}: {}", destination.display(), e),
            ));
        }
        drop(destination_file);
        if let Err(d) = validate_trusted_path_as(&destination, required_uid) {
            let _ = fs::remove_dir_all(&operation_dir);
            return Err(d);
        }
        let actual = match compute_file_sha256(&destination) {
            Ok(hash) => hash,
            Err(e) => {
                let _ = fs::remove_dir_all(&operation_dir);
                return Err(Denial::new(
                    EXIT_ARTIFACT,
                    "pacman_artifact_hash_read_failed",
                    format!("{}: {}", destination.display(), e),
                ));
            }
        };
        if !ct_eq_hex(&actual, &action.artifact_sha256) {
            let _ = fs::remove_dir_all(&operation_dir);
            return Err(Denial::new(
                EXIT_ARTIFACT,
                "pacman_artifact_hash_mismatch",
                format!(
                    "identity={} expected={} actual={}",
                    action.name, action.artifact_sha256, actual
                ),
            ));
        }
        let staged_signature = match (&action.signature_path, &action.signature_sha256) {
            (Some(source_path), Some(expected_hash)) => {
                let destination_path = PathBuf::from(format!("{}.sig", destination.display()));
                let mut source_options = fs::OpenOptions::new();
                source_options.read(true).custom_flags(libc::O_NOFOLLOW);
                let mut signature_source = match source_options.open(source_path) {
                    Ok(file) => file,
                    Err(e) => {
                        let _ = fs::remove_dir_all(&operation_dir);
                        return Err(Denial::new(
                            EXIT_ARTIFACT,
                            "pacman_signature_source_open_failed",
                            format!("{}: {}", source_path.display(), e),
                        ));
                    }
                };
                if !signature_source
                    .metadata()
                    .map(|metadata| metadata.is_file())
                    .unwrap_or(false)
                {
                    let _ = fs::remove_dir_all(&operation_dir);
                    return Err(Denial::new(
                        EXIT_ARTIFACT,
                        "pacman_signature_source_not_regular",
                        source_path.display().to_string(),
                    ));
                }
                let mut destination_options = fs::OpenOptions::new();
                destination_options.write(true).create_new(true).mode(0o600);
                let mut signature_destination = match destination_options.open(&destination_path) {
                    Ok(file) => file,
                    Err(e) => {
                        let _ = fs::remove_dir_all(&operation_dir);
                        return Err(Denial::new(
                            EXIT_ARTIFACT,
                            "pacman_signature_stage_create_failed",
                            format!("{}: {}", destination_path.display(), e),
                        ));
                    }
                };
                if let Err(e) = std::io::copy(&mut signature_source, &mut signature_destination) {
                    let _ = fs::remove_dir_all(&operation_dir);
                    return Err(Denial::new(
                        EXIT_ARTIFACT,
                        "pacman_signature_stage_failed",
                        format!(
                            "{} -> {}: {}",
                            source_path.display(),
                            destination_path.display(),
                            e
                        ),
                    ));
                }
                if let Err(e) = signature_destination.sync_all() {
                    let _ = fs::remove_dir_all(&operation_dir);
                    return Err(Denial::new(
                        EXIT_INTERNAL,
                        "pacman_signature_stage_sync_failed",
                        format!("{}: {}", destination_path.display(), e),
                    ));
                }
                drop(signature_destination);
                if let Err(d) = validate_trusted_path_as(&destination_path, required_uid) {
                    let _ = fs::remove_dir_all(&operation_dir);
                    return Err(d);
                }
                let actual_hash = match compute_file_sha256(&destination_path) {
                    Ok(hash) => hash,
                    Err(e) => {
                        let _ = fs::remove_dir_all(&operation_dir);
                        return Err(Denial::new(
                            EXIT_ARTIFACT,
                            "pacman_signature_hash_read_failed",
                            format!("{}: {}", destination_path.display(), e),
                        ));
                    }
                };
                if !ct_eq_hex(&actual_hash, expected_hash) {
                    let _ = fs::remove_dir_all(&operation_dir);
                    return Err(Denial::new(
                        EXIT_ARTIFACT,
                        "pacman_signature_hash_mismatch",
                        format!(
                            "identity={} expected={} actual={}",
                            action.name, expected_hash, actual_hash
                        ),
                    ));
                }
                Some(destination_path)
            }
            (None, None) => None,
            _ => {
                let _ = fs::remove_dir_all(&operation_dir);
                return Err(Denial::new(
                    EXIT_BAD_TOKEN,
                    "pacman_signature_incomplete",
                    format!("identity={}", action.name),
                ));
            }
        };
        staged.push(PacmanStagedAction {
            name: action.name.clone(),
            version: action.version.clone(),
            requested: action.requested,
            path: destination,
            signature_path: staged_signature,
        });
    }
    Ok((operation_dir, staged))
}

#[cfg(unix)]
fn validate_pacman_archive_identity_output(
    action: &PacmanStagedAction,
    output: &[u8],
) -> Result<(), Denial> {
    let found = String::from_utf8_lossy(output);
    let mut fields = found.split_ascii_whitespace();
    let found_name = fields.next().unwrap_or("");
    let found_version = fields.next().unwrap_or("");
    if found_name != action.name || found_version != action.version || fields.next().is_some() {
        return Err(Denial::new(
            EXIT_ARTIFACT,
            "pacman_archive_identity_mismatch",
            format!(
                "expected={}@{} found={} path={}",
                action.name,
                action.version,
                found.trim(),
                action.path.display()
            ),
        ));
    }
    Ok(())
}

#[cfg(unix)]
fn verify_pacman_archive_identity(action: &PacmanStagedAction) -> Result<(), Denial> {
    let output = Command::new(PACMAN_BINARY)
        .args(["-Qp", "--"])
        .arg(&action.path)
        .env_clear()
        .env(
            "PATH",
            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        )
        .env("LC_ALL", "C")
        .output()
        .map_err(|e| {
            Denial::new(
                EXIT_EXEC_FAILED,
                "pacman_archive_inspection_spawn_failed",
                e.to_string(),
            )
        })?;
    if !output.status.success() {
        return Err(Denial::new(
            EXIT_ARTIFACT,
            "pacman_archive_inspection_failed",
            format!(
                "path={} exit={:?}",
                action.path.display(),
                output.status.code()
            ),
        ));
    }
    validate_pacman_archive_identity_output(action, &output.stdout)
}

#[cfg(unix)]
fn verify_pacman_archive_signature(action: &PacmanStagedAction) -> Result<(), Denial> {
    let Some(signature_path) = &action.signature_path else {
        return Err(Denial::new(
            EXIT_ARTIFACT,
            "pacman_signature_required",
            format!("identity={}@{}", action.name, action.version),
        ));
    };
    let output = Command::new(PACMAN_KEY_BINARY)
        .args(["--verify"])
        .arg(signature_path)
        .arg(&action.path)
        .env_clear()
        .env(
            "PATH",
            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        )
        .env("LC_ALL", "C")
        .output()
        .map_err(|e| {
            Denial::new(
                EXIT_EXEC_FAILED,
                "pacman_signature_verification_spawn_failed",
                e.to_string(),
            )
        })?;
    if !output.status.success() {
        return Err(Denial::new(
            EXIT_ARTIFACT,
            "pacman_signature_verification_failed",
            format!(
                "identity={}@{} exit={:?}",
                action.name,
                action.version,
                output.status.code()
            ),
        ));
    }
    Ok(())
}

#[cfg(unix)]
fn parse_pacman_local_version(name: &str, output: &[u8]) -> Result<String, Denial> {
    let found = String::from_utf8_lossy(output);
    let mut fields = found.split_ascii_whitespace();
    let found_name = fields.next().unwrap_or("");
    let found_version = fields.next().unwrap_or("");
    if found_name != name || found_version.is_empty() || fields.next().is_some() {
        return Err(Denial::new(
            EXIT_INTERNAL,
            "pacman_local_identity_invalid",
            format!("expected={} found={}", name, found.trim()),
        ));
    }
    Ok(found_version.to_string())
}

#[cfg(unix)]
fn current_pacman_version(name: &str) -> Result<Option<String>, Denial> {
    let output = Command::new(PACMAN_BINARY)
        .args(["-Q", "--", name])
        .env_clear()
        .env(
            "PATH",
            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        )
        .env("LC_ALL", "C")
        .output()
        .map_err(|e| {
            Denial::new(
                EXIT_EXEC_FAILED,
                "pacman_local_query_spawn_failed",
                e.to_string(),
            )
        })?;
    if output.status.success() {
        return parse_pacman_local_version(name, &output.stdout).map(Some);
    }
    if output.status.code() == Some(1) && output.stdout.is_empty() {
        return Ok(None);
    }
    Err(Denial::new(
        EXIT_EXEC_FAILED,
        "pacman_local_query_failed",
        format!("name={} exit={:?}", name, output.status.code()),
    ))
}

#[cfg(unix)]
fn compare_pacman_versions(candidate: &str, current: &str) -> Result<i32, Denial> {
    let output = Command::new(VERCMP_BINARY)
        .args([candidate, current])
        .env_clear()
        .env("LC_ALL", "C")
        .output()
        .map_err(|e| {
            Denial::new(
                EXIT_EXEC_FAILED,
                "pacman_vercmp_spawn_failed",
                e.to_string(),
            )
        })?;
    if !output.status.success() {
        return Err(Denial::new(
            EXIT_EXEC_FAILED,
            "pacman_vercmp_failed",
            format!("exit={:?}", output.status.code()),
        ));
    }
    parse_pacman_vercmp_output(&output.stdout)
}

#[cfg(unix)]
fn parse_pacman_vercmp_output(output: &[u8]) -> Result<i32, Denial> {
    String::from_utf8_lossy(output)
        .trim()
        .parse::<i32>()
        .map_err(|_| {
            Denial::new(
                EXIT_INTERNAL,
                "pacman_vercmp_output_invalid",
                "non-integer output",
            )
        })
}

#[cfg(unix)]
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum PacmanVersionRelation {
    Absent,
    Equal,
    Newer,
}

#[cfg(unix)]
fn enforce_forward_only_pacman_action(
    action: &PacmanStagedAction,
) -> Result<PacmanVersionRelation, Denial> {
    let Some(current) = current_pacman_version(&action.name)? else {
        return Ok(PacmanVersionRelation::Absent);
    };
    match compare_pacman_versions(&action.version, &current)? {
        value if value > 0 => Ok(PacmanVersionRelation::Newer),
        0 => Ok(PacmanVersionRelation::Equal),
        _ => Err(Denial::new(
            EXIT_AUTHORIZATION_DENIED,
            "pacman_downgrade_refused",
            format!(
                "identity={} current={} target={}",
                action.name, current, action.version
            ),
        )),
    }
}

#[cfg(unix)]
fn require_requested_pacman_transition(
    operation: &str,
    relation: Option<PacmanVersionRelation>,
) -> Result<(), Denial> {
    match (operation, relation) {
        ("upgrade", Some(PacmanVersionRelation::Newer)) => Ok(()),
        ("install", Some(PacmanVersionRelation::Absent)) => Ok(()),
        ("upgrade", _) => Err(Denial::new(
            EXIT_AUTHORIZATION_DENIED,
            "pacman_upgrade_not_forward",
            "requested root is absent, equal, or older than the authorized upgrade target",
        )),
        ("install", _) => Err(Denial::new(
            EXIT_AUTHORIZATION_DENIED,
            "pacman_install_not_new",
            "requested root is already installed",
        )),
        _ => Err(Denial::new(
            EXIT_UNSUPPORTED_OP,
            "operation_not_allowed",
            format!("pacman operation={}", operation),
        )),
    }
}

#[cfg(unix)]
fn claim_mutation_authorization_as(
    authorization_id: &str,
    replay_dir: &Path,
    required_uid: u32,
) -> Result<(), Denial> {
    if !mutation_protocol::is_canonical_uuid_v4(authorization_id) {
        return Err(Denial::new(
            EXIT_BAD_TOKEN,
            "authorization_id_not_uuid_v4",
            "authorization_id cannot key the replay ledger",
        ));
    }
    match fs::symlink_metadata(replay_dir) {
        Ok(_) => validate_trusted_path_as(replay_dir, required_uid)?,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            fs::create_dir_all(replay_dir).map_err(|e| {
                Denial::new(
                    EXIT_INTERNAL,
                    "mutation_replay_dir_create_failed",
                    format!("{}: {}", replay_dir.display(), e),
                )
            })?;
        }
        Err(e) => {
            return Err(Denial::new(
                EXIT_INTERNAL,
                "mutation_replay_dir_unreadable",
                format!("{}: {}", replay_dir.display(), e),
            ))
        }
    }
    fs::set_permissions(replay_dir, fs::Permissions::from_mode(0o700)).map_err(|e| {
        Denial::new(
            EXIT_INTERNAL,
            "mutation_replay_dir_chmod_failed",
            format!("{}: {}", replay_dir.display(), e),
        )
    })?;
    validate_trusted_path_as(replay_dir, required_uid)?;
    if let Some(parent) = replay_dir.parent() {
        validate_trusted_path_as(parent, required_uid)?;
    }

    let claim = replay_dir.join(authorization_id);
    let mut options = fs::OpenOptions::new();
    options.write(true).create_new(true).mode(0o600);
    match options.open(&claim) {
        Ok(_) => {
            validate_trusted_path_as(&claim, required_uid)?;
            Ok(())
        }
        Err(e) if e.kind() == std::io::ErrorKind::AlreadyExists => Err(Denial::new(
            EXIT_REPLAY,
            "authorization_already_consumed",
            format!("authorization_id={}", authorization_id),
        )),
        Err(e) => Err(Denial::new(
            EXIT_INTERNAL,
            "mutation_replay_claim_failed",
            format!("{}: {}", claim.display(), e),
        )),
    }
}

#[cfg(unix)]
fn execute_pacman_actions(actions: &[PacmanStagedAction]) -> Result<(), Denial> {
    let mut command = Command::new(PACMAN_BINARY);
    command.args(["-U", "--noconfirm", "--"]);
    for action in actions {
        command.arg(&action.path);
    }
    let status = command
        .env_clear()
        .env(
            "PATH",
            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        )
        .env("LC_ALL", "C")
        .status()
        .map_err(|e| Denial::new(EXIT_EXEC_FAILED, "pacman_exec_spawn_failed", e.to_string()))?;
    if !status.success() {
        return Err(Denial::new(
            EXIT_EXEC_FAILED,
            "pacman_exec_nonzero_exit",
            format!("{} exit={:?}", PACMAN_BINARY, status.code()),
        ));
    }
    Ok(())
}

#[cfg(all(test, unix))]
mod pacman_envelope_tests {
    use super::*;

    const TEST_AUTHORIZATION_ID: &str = "550e8400-e29b-41d4-a716-446655440011";

    fn fixture_dir(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "redflag-pacman-envelope-test-{}-{}",
            name,
            std::process::id()
        ));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        fs::set_permissions(&dir, fs::Permissions::from_mode(0o700)).unwrap();
        dir
    }

    fn manifest_for(path: &Path, sha256: &str, identity: &str) -> MutationManifest {
        MutationManifest {
            protocol_version: mutation_protocol::MUTATION_PROTOCOL_VERSION,
            operation_id: "550e8400-e29b-41d4-a716-446655440010".into(),
            target_id: "agent-123".into(),
            backend: "pacman".into(),
            operation: "upgrade".into(),
            resolved_actions: vec![mutation_protocol::ResolvedAction {
                kind: "package".into(),
                identity: identity.into(),
                payload: serde_json::json!({
                    "artifact_sha256": sha256,
                    "execution_location": {
                        "kind": "cache",
                        "value": path,
                    },
                    "repository": "core-testing",
                    "requested": true,
                })
                .to_string(),
            }],
            evidence: vec![],
        }
    }

    #[test]
    fn pacman_payload_pins_cache_hash_and_epoch_identity() {
        let root = fixture_dir("pins");
        let source = root.join("acl-2.3.2-1-x86_64.pkg.tar.zst");
        fs::write(&source, b"signed package bytes").unwrap();
        let expected = compute_file_sha256(&source).unwrap();
        let actions = parse_pacman_actions(&manifest_for(&source, &expected, "acl@1:2.3.2-1"))
            .expect("signed pacman payload should parse");

        let staging = root.join("staging");
        let uid = fs::symlink_metadata(&root).unwrap().uid();
        let (operation_dir, staged) =
            stage_pacman_actions_as(&actions, &staging, TEST_AUTHORIZATION_ID, uid)
                .expect("artifact should stage under executor custody");

        assert_eq!(staged.len(), 1);
        assert_eq!(staged[0].name, "acl");
        assert_eq!(staged[0].version, "1:2.3.2-1");
        assert_eq!(compute_file_sha256(&staged[0].path).unwrap(), expected);
        assert!(staged[0].path.starts_with(&operation_dir));
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn pacman_staging_refuses_bytes_outside_signed_hash() {
        let root = fixture_dir("tamper");
        let source = root.join("acl-2.3.2-1-x86_64.pkg.tar.zst");
        fs::write(&source, b"different package bytes").unwrap();
        let actions = parse_pacman_actions(&manifest_for(&source, &"a".repeat(64), "acl@2.3.2-1"))
            .expect("payload shape should parse before custody verifies bytes");

        let staging = root.join("staging");
        let uid = fs::symlink_metadata(&root).unwrap().uid();
        let denial = stage_pacman_actions_as(&actions, &staging, TEST_AUTHORIZATION_ID, uid)
            .expect_err("tampered package must fail closed");

        assert_eq!(denial.code, EXIT_ARTIFACT);
        assert_eq!(denial.reason, "pacman_artifact_hash_mismatch");
        assert!(!staging.join(TEST_AUTHORIZATION_ID).exists());
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn pacman_staging_refuses_symlink_sources() {
        let root = fixture_dir("symlink");
        let target = root.join("target.pkg.tar.zst");
        let source = root.join("acl-2.3.2-1-x86_64.pkg.tar.zst");
        fs::write(&target, b"signed package bytes").unwrap();
        std::os::unix::fs::symlink(&target, &source).unwrap();
        let expected = compute_file_sha256(&target).unwrap();
        let actions = parse_pacman_actions(&manifest_for(&source, &expected, "acl@2.3.2-1"))
            .expect("payload shape may name an existing path before custody opens it");

        let staging = root.join("staging");
        let uid = fs::symlink_metadata(&root).unwrap().uid();
        let denial = stage_pacman_actions_as(&actions, &staging, TEST_AUTHORIZATION_ID, uid)
            .expect_err("custody must not follow an agent-controlled symlink");

        assert_eq!(denial.code, EXIT_ARTIFACT);
        assert_eq!(denial.reason, "pacman_artifact_source_open_failed");
        assert!(!staging.join(TEST_AUTHORIZATION_ID).exists());
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn standalone_envelope_mint_requires_a_detached_signature() {
        std::env::set_var("REDFLAG_AGENT_ID", "agent-123");
        let root = fixture_dir("mint-signature");
        let source = root.join("acl-2.3.2-1-x86_64.pkg.tar.zst");
        fs::write(&source, b"package bytes").unwrap();
        let request = EnvelopeMintRequest {
            version: MUTATION_PROTOCOL_VERSION,
            request_id: "550e8400-e29b-41d4-a716-446655440012".into(),
            manifest: manifest_for(
                &source,
                &compute_file_sha256(&source).unwrap(),
                "acl@2.3.2-1",
            ),
            gate_evidence: GateEvidence {
                resolved_at: 1_700_000_000,
                osv_checked_at: 0,
                osv_status: "unsupported".into(),
                osv_vuln_count: 0,
                age_gate: "not_applicable".into(),
                soak_gate: "not_applicable".into(),
                operator: "local-operator".into(),
                override_reason: "Arch advisory mapping unavailable".into(),
            },
        };
        let denial = validate_envelope_mint_request(&request, 1_700_000_001)
            .expect_err("standalone authority must not sign an unsigned archive");
        assert_eq!(denial.reason, "mint_envelope_signature_required");
        std::env::remove_var("REDFLAG_AGENT_ID");
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn mutation_authorization_claim_is_atomic_and_one_shot() {
        let root = fixture_dir("replay");
        let replay = root.join("consumed-authorizations");
        let uid = fs::symlink_metadata(&root).unwrap().uid();

        claim_mutation_authorization_as(TEST_AUTHORIZATION_ID, &replay, uid)
            .expect("first claim should own the authorization");
        let denial = claim_mutation_authorization_as(TEST_AUTHORIZATION_ID, &replay, uid)
            .expect_err("second claim must be replay");

        assert_eq!(denial.code, EXIT_REPLAY);
        assert_eq!(denial.reason, "authorization_already_consumed");
        assert!(replay.join(TEST_AUTHORIZATION_ID).is_file());
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn pacman_archive_identity_output_is_exact() {
        let action = PacmanStagedAction {
            name: "acl".into(),
            version: "1:2.3.2-1".into(),
            requested: true,
            path: PathBuf::from("/root/custody/acl.pkg.tar.zst"),
            signature_path: None,
        };

        validate_pacman_archive_identity_output(&action, b"acl 1:2.3.2-1\n")
            .expect("pacman query identity should match the signed action");
        let denial = validate_pacman_archive_identity_output(&action, b"acl 2.3.2-1\n")
            .expect_err("different archive version must fail closed");
        assert_eq!(denial.reason, "pacman_archive_identity_mismatch");
    }

    #[test]
    fn pacman_executor_requires_a_detached_signature() {
        let action = PacmanStagedAction {
            name: "acl".into(),
            version: "2.3.2-1".into(),
            requested: true,
            path: PathBuf::from("/root/custody/acl.pkg.tar.zst"),
            signature_path: None,
        };
        let denial = verify_pacman_archive_signature(&action)
            .expect_err("execution must not rely on the authority having required a signature");
        assert_eq!(denial.code, EXIT_ARTIFACT);
        assert_eq!(denial.reason, "pacman_signature_required");
    }

    #[test]
    fn pacman_local_identity_output_is_exact() {
        assert_eq!(
            parse_pacman_local_version("linux", b"linux 1:6.19.14-1\n").unwrap(),
            "1:6.19.14-1"
        );
        let denial = parse_pacman_local_version("linux", b"other 1:6.19.14-1\n")
            .expect_err("a different installed package must fail closed");
        assert_eq!(denial.reason, "pacman_local_identity_invalid");
    }

    #[test]
    fn pacman_vercmp_output_is_an_integer() {
        assert_eq!(parse_pacman_vercmp_output(b"1\n").unwrap(), 1);
        assert_eq!(parse_pacman_vercmp_output(b"0\n").unwrap(), 0);
        assert_eq!(parse_pacman_vercmp_output(b"-1\n").unwrap(), -1);
        let denial = parse_pacman_vercmp_output(b"older\n")
            .expect_err("non-integer vercmp output must fail closed");
        assert_eq!(denial.reason, "pacman_vercmp_output_invalid");
    }

    #[test]
    fn pacman_operation_requires_the_requested_root_transition() {
        require_requested_pacman_transition("upgrade", Some(PacmanVersionRelation::Newer))
            .expect("a strictly newer requested root is an upgrade");
        assert_eq!(
            require_requested_pacman_transition("upgrade", Some(PacmanVersionRelation::Equal),)
                .expect_err("an equal requested root must not masquerade as an upgrade")
                .reason,
            "pacman_upgrade_not_forward"
        );
        require_requested_pacman_transition("install", Some(PacmanVersionRelation::Absent))
            .expect("an absent requested root is an install");
        assert_eq!(
            require_requested_pacman_transition("install", Some(PacmanVersionRelation::Newer),)
                .expect_err("an installed root must not masquerade as a new install")
                .reason,
            "pacman_install_not_new"
        );
    }
}

// Ordered so each refusal gets its own exit code instead of one opaque
// verification failure. Execution is a separate step after this returns.
fn verify_envelope(
    envelope_file: &str,
    now: i64,
) -> Result<MutationEnvelope, Box<(Option<MutationEnvelope>, Denial)>> {
    let envelope = read_envelope_from_file(envelope_file).map_err(|d| Box::new((None, d)))?;

    // Evaluate the denial before moving the envelope into the return — every
    // detail string reads from it.
    macro_rules! deny {
        ($d:expr) => {{
            let denial = $d;
            return Err(Box::new((Some(envelope), denial)));
        }};
    }

    if envelope.manifest.protocol_version != mutation_protocol::MUTATION_PROTOCOL_VERSION
        || envelope.authorization.protocol_version != mutation_protocol::MUTATION_PROTOCOL_VERSION
    {
        deny!(Denial::new(
            EXIT_VERSION,
            "unsupported_envelope_version",
            format!(
                "manifest={} authorization={} supported={}",
                envelope.manifest.protocol_version,
                envelope.authorization.protocol_version,
                mutation_protocol::MUTATION_PROTOCOL_VERSION
            ),
        ));
    }

    if let Err(e) = envelope.manifest.validate() {
        deny!(Denial::new(EXIT_BAD_TOKEN, "manifest_invalid", e));
    }

    if !mutation_protocol::is_canonical_uuid_v4(&envelope.authorization.authorization_id) {
        deny!(Denial::new(
            EXIT_BAD_TOKEN,
            "authorization_id_not_uuid_v4",
            "authorization_id must be a canonical UUID v4 before it can key a replay ledger",
        ));
    }

    // target_id is the RedFlag agent identity. Both copies are signed and the
    // verifier requires them equal; both are compared anyway, because the cost
    // is a string compare and the failure would be silent.
    let local = match local_agent_id() {
        Ok(v) => v,
        Err(d) => deny!(d),
    };
    let targets = [
        ("manifest", envelope.manifest.target_id.clone()),
        ("authorization", envelope.authorization.target_id.clone()),
    ];
    for (field, value) in targets {
        if value != local {
            deny!(Denial::new(
                EXIT_AGENT_MISMATCH,
                "envelope_target_mismatch",
                format!("{}_target_id={} host_agent_id={}", field, value, local),
            ));
        }
    }

    if now < envelope.authorization.not_before {
        deny!(Denial::new(
            EXIT_TIME_WINDOW,
            "envelope_not_yet_valid",
            format!(
                "now={} not_before={}",
                now, envelope.authorization.not_before
            ),
        ));
    }
    if now > envelope.authorization.expires_at {
        deny!(Denial::new(
            EXIT_TIME_WINDOW,
            "envelope_expired",
            format!(
                "now={} expires_at={}",
                now, envelope.authorization.expires_at
            ),
        ));
    }
    let lifetime = match envelope
        .authorization
        .expires_at
        .checked_sub(envelope.authorization.not_before)
    {
        Some(value) if value > 0 => value,
        _ => deny!(Denial::new(
            EXIT_TIME_WINDOW,
            "authorization_time_window_invalid",
            format!(
                "not_before={} expires_at={}",
                envelope.authorization.not_before, envelope.authorization.expires_at
            ),
        )),
    };
    if lifetime > mutation_protocol::MAX_AUTHORIZATION_LIFETIME_SECS {
        deny!(Denial::new(
            EXIT_TIME_WINDOW,
            "authorization_lifetime_exceeded",
            format!(
                "lifetime={}s ceiling={}s",
                lifetime,
                mutation_protocol::MAX_AUTHORIZATION_LIFETIME_SECS
            ),
        ));
    }

    if envelope.authorization.decision != "allow" {
        deny!(Denial::new(
            EXIT_AUTHORIZATION_DENIED,
            "authorization_decision_not_allow",
            format!("decision={}", envelope.authorization.decision),
        ));
    }

    let keyring_dir = PathBuf::from(env_or("REDFLAG_HELPER_KEYRING", DEFAULT_KEYRING_DIR));
    let keyring = match load_keyring(&keyring_dir) {
        Ok(k) => k,
        Err(d) => deny!(d),
    };
    let verifying_key = match keyring
        .iter()
        .find(|(id, _)| id == &envelope.authorization.key_id)
        .map(|(_, vk)| *vk)
    {
        Some(vk) => vk,
        None => deny!(Denial::new(
            EXIT_KEY_NOT_FOUND,
            "key_id_not_trusted",
            format!("key_id={}", envelope.authorization.key_id),
        )),
    };

    if let Err(e) =
        envelope
            .authorization
            .verify_for_execution_at(&verifying_key, &envelope.manifest, now)
    {
        deny!(Denial::new(
            EXIT_SIGNATURE,
            "envelope_verification_failed",
            e
        ));
    }

    if let Err(d) = validate_backend_payload_shape(&envelope.manifest) {
        deny!(d);
    }

    let backend = envelope.manifest.backend.clone();
    log_security(&format!(
        "envelope_verified operation_id={} manifest_hash={} authorization_id={} backend={} target_id={}",
        envelope.manifest.operation_id,
        envelope.manifest.hash(),
        envelope.authorization.authorization_id,
        backend,
        envelope.manifest.target_id
    ));
    Ok(envelope)
}

// Verification-only compatibility path. It proves the envelope without
// consuming replay state or executing a backend.
fn run_envelope(envelope_file: &str, now: i64) -> (Option<MutationEnvelope>, Denial) {
    match verify_envelope(envelope_file, now) {
        Ok(envelope) => {
            let backend = envelope.manifest.backend.clone();
            (
                Some(envelope),
                Denial::new(
                    EXIT_UNSUPPORTED_OP,
                    "backend_not_migrated",
                    format!(
                        "backend={} has no executable path through verify-envelope",
                        backend
                    ),
                ),
            )
        }
        Err(result) => *result,
    }
}

fn run_verify_envelope_cli(args: &[String]) -> i32 {
    let mut envelope_file: Option<String> = None;
    let mut receipt_file: Option<String> = None;
    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--envelope-file" => {
                envelope_file = args.get(i + 1).cloned();
                i += 2;
            }
            "--receipt-file" => {
                receipt_file = args.get(i + 1).cloned();
                i += 2;
            }
            other => {
                log_error(&format!("verify-envelope unknown argument {}", other));
                return EXIT_BAD_TOKEN;
            }
        }
    }
    let envelope_file = match envelope_file {
        Some(p) => p,
        None => {
            log_error("verify-envelope usage: redflag-helper verify-envelope --envelope-file <path> [--receipt-file <path>]");
            return EXIT_BAD_TOKEN;
        }
    };
    let now = now_unix();
    let (envelope, denial) = run_envelope(&envelope_file, now);
    let decision = if denial.code == EXIT_EXEC_FAILED {
        "failed"
    } else {
        "denied"
    };
    log_security(&format!(
        "{} reason={} detail={} exit={}",
        decision, denial.reason, denial.detail, denial.code
    ));

    let receipt = match envelope {
        Some(e) => e.receipt_for(
            MutationOutcome {
                decision: decision.to_string(),
                reason: denial.reason.to_string(),
                executed: false,
                verified_actions: 0,
                exit_code: denial.code,
                detail: denial.detail.clone(),
            },
            now,
        ),
        None => MutationReceipt {
            protocol_version: mutation_protocol::MUTATION_PROTOCOL_VERSION,
            operation_id: String::new(),
            manifest_hash: String::new(),
            authorization_id: String::new(),
            target_id: String::new(),
            backend: String::new(),
            operation: String::new(),
            decision: decision.to_string(),
            reason: denial.reason.to_string(),
            executed: false,
            verified_actions: 0,
            exit_code: denial.code,
            error: denial.detail.clone(),
            timestamp: now,
        },
    };
    if let Some(rp) = receipt_file {
        emit_result_to_file(&receipt, &rp);
    }
    emit_result(&receipt);
    denial.code
}

#[cfg(unix)]
fn empty_mutation_receipt(denial: &Denial, decision: &str, now: i64) -> MutationReceipt {
    MutationReceipt {
        protocol_version: mutation_protocol::MUTATION_PROTOCOL_VERSION,
        operation_id: String::new(),
        manifest_hash: String::new(),
        authorization_id: String::new(),
        target_id: String::new(),
        backend: String::new(),
        operation: String::new(),
        decision: decision.to_string(),
        reason: denial.reason.to_string(),
        executed: false,
        verified_actions: 0,
        exit_code: denial.code,
        error: denial.detail.clone(),
        timestamp: now,
    }
}

#[cfg(unix)]
fn execute_verified_envelope(envelope: &MutationEnvelope) -> Result<u32, (Denial, u32)> {
    let actions = parse_pacman_actions(&envelope.manifest).map_err(|denial| (denial, 0))?;
    let (operation_dir, staged) = stage_pacman_actions_as(
        &actions,
        Path::new(DEFAULT_MUTATION_STAGING_DIR),
        &envelope.authorization.authorization_id,
        0,
    )
    .map_err(|denial| (denial, 0))?;

    let mut requested_relation = None;
    for (index, action) in staged.iter().enumerate() {
        if let Err(denial) = verify_pacman_archive_identity(action) {
            let _ = fs::remove_dir_all(&operation_dir);
            return Err((denial, index as u32));
        }
        if let Err(denial) = verify_pacman_archive_signature(action) {
            let _ = fs::remove_dir_all(&operation_dir);
            return Err((denial, index as u32));
        }
        match enforce_forward_only_pacman_action(action) {
            Ok(relation) => {
                if action.requested {
                    requested_relation = Some(relation);
                }
            }
            Err(denial) => {
                let _ = fs::remove_dir_all(&operation_dir);
                return Err((denial, index as u32));
            }
        }
    }
    if let Err(denial) =
        require_requested_pacman_transition(&envelope.manifest.operation, requested_relation)
    {
        let _ = fs::remove_dir_all(&operation_dir);
        return Err((denial, staged.len() as u32));
    }

    if let Err(denial) = claim_mutation_authorization_as(
        &envelope.authorization.authorization_id,
        Path::new(DEFAULT_MUTATION_REPLAY_DIR),
        0,
    ) {
        let _ = fs::remove_dir_all(&operation_dir);
        return Err((denial, staged.len() as u32));
    }

    log_security(&format!(
        "authorized mutation operation_id={} authorization_id={} target_id={} backend=pacman operation={} actions={}",
        envelope.manifest.operation_id,
        envelope.authorization.authorization_id,
        envelope.manifest.target_id,
        envelope.manifest.operation,
        staged.len()
    ));
    let result = execute_pacman_actions(&staged);
    let _ = fs::remove_dir_all(&operation_dir);
    result.map_err(|denial| (denial, staged.len() as u32))?;
    Ok(staged.len() as u32)
}

#[cfg(unix)]
fn run_execute_envelope_cli(args: &[String]) -> i32 {
    let (envelope_file, receipt_file) = if args.len() == 2 && args[0] == "--envelope-file" {
        (args[1].clone(), None)
    } else if args.len() == 4 && args[0] == "--envelope-file" && args[2] == "--receipt-file" {
        (args[1].clone(), Some(args[3].clone()))
    } else {
        log_error("execute-envelope usage: redflag-helper execute-envelope --envelope-file <path> [--receipt-file <path>]");
        return EXIT_BAD_TOKEN;
    };
    if let Err(denial) =
        validate_agent_exchange_path(&envelope_file, DEFAULT_AGENT_MUTATION_ENVELOPE_DIR)
    {
        log_security(&format!(
            "denied reason={} detail={} exit={}",
            denial.reason, denial.detail, denial.code
        ));
        return denial.code;
    }
    if let Some(path) = &receipt_file {
        if let Err(denial) = validate_agent_exchange_path(path, DEFAULT_AGENT_MUTATION_RECEIPT_DIR)
        {
            log_security(&format!(
                "denied reason={} detail={} exit={}",
                denial.reason, denial.detail, denial.code
            ));
            return denial.code;
        }
        if Path::new(path).file_name() != Path::new(&envelope_file).file_name() {
            log_security("denied reason=mutation_exchange_join_mismatch exit=26");
            return EXIT_TRUST_PATH;
        }
    }

    let now = now_unix();
    let (receipt, exit_code) = match verify_envelope(&envelope_file, now) {
        Ok(envelope) => match execute_verified_envelope(&envelope) {
            Ok(verified_actions) => (
                envelope.receipt_for(
                    MutationOutcome {
                        decision: "executed".to_string(),
                        reason: "operation_completed".to_string(),
                        executed: true,
                        verified_actions,
                        exit_code: EXIT_OK,
                        detail: String::new(),
                    },
                    now,
                ),
                EXIT_OK,
            ),
            Err((denial, verified_actions)) => {
                let decision = if denial.code == EXIT_EXEC_FAILED {
                    "failed"
                } else {
                    "denied"
                };
                log_security(&format!(
                    "{} reason={} detail={} exit={}",
                    decision, denial.reason, denial.detail, denial.code
                ));
                (
                    envelope.receipt_for(
                        MutationOutcome {
                            decision: decision.to_string(),
                            reason: denial.reason.to_string(),
                            executed: false,
                            verified_actions,
                            exit_code: denial.code,
                            detail: denial.detail.clone(),
                        },
                        now,
                    ),
                    denial.code,
                )
            }
        },
        Err(result) => {
            let (envelope, denial) = *result;
            let decision = if denial.code == EXIT_EXEC_FAILED {
                "failed"
            } else {
                "denied"
            };
            log_security(&format!(
                "{} reason={} detail={} exit={}",
                decision, denial.reason, denial.detail, denial.code
            ));
            let receipt = match envelope {
                Some(envelope) => envelope.receipt_for(
                    MutationOutcome {
                        decision: decision.to_string(),
                        reason: denial.reason.to_string(),
                        executed: false,
                        verified_actions: 0,
                        exit_code: denial.code,
                        detail: denial.detail.clone(),
                    },
                    now,
                ),
                None => empty_mutation_receipt(&denial, decision, now),
            };
            (receipt, denial.code)
        }
    };

    if let Some(path) = receipt_file {
        if let Err(denial) =
            write_agent_exchange_json(&receipt, &path, DEFAULT_AGENT_MUTATION_RECEIPT_DIR)
        {
            log_error(&format!(
                "mutation_receipt_write_failed reason={} detail={}",
                denial.reason, denial.detail
            ));
            return denial.code;
        }
    }
    emit_result(&receipt);
    exit_code
}

#[derive(Debug, Serialize)]
struct IntegrityResult {
    target: String,
    expected: String,
    actual: String,
    matched: bool,
    timestamp: i64,
}

// verify-binary — runtime watchdog half of the two-binary model.
//
// Hashes the agent binary on disk and compares it to the expected SHA-256. This
// is network-less by design (same guarantee as the token executor): it emits a
// structured verdict to stdout and signals via exit code. The agent reports the
// hash to the server on check-in and reacts to a mismatch.
//
// SKETCH — the hash/compare/verdict is complete and safe to ship. The
// kill-agent-on-mismatch + final phone-home is intentionally NOT wired here:
// phoning home would break this process's network-less guarantee, so it belongs
// agent-side. The kill itself (SIGKILL to a passed --agent-pid) is privileged
// and local and could live here behind an --enforce flag. Settle that path
// against the network-less invariant before wiring it.
//
// Usage: redflag-helper verify-binary <target_path> <expected_sha256>
fn run_verify_binary(args: &[String]) -> i32 {
    let (target, expected) = match (args.first(), args.get(1)) {
        (Some(t), Some(e)) => (t.clone(), e.trim().to_lowercase()),
        _ => {
            log_error(
                "verify-binary usage: redflag-helper verify-binary <target_path> <expected_sha256>",
            );
            return EXIT_BAD_TOKEN;
        }
    };

    let actual = match compute_file_sha256(Path::new(&target)) {
        Ok(h) => h.to_lowercase(),
        Err(e) => {
            log_error(&format!(
                "integrity_hash_failed target={} error={}",
                target, e
            ));
            return EXIT_INTEGRITY;
        }
    };
    let matched = ct_eq_hex(&actual, &expected);

    let result = IntegrityResult {
        target: target.clone(),
        expected: expected.clone(),
        actual: actual.clone(),
        matched,
        timestamp: now_unix(),
    };
    match serde_json::to_string(&result) {
        Ok(s) => println!("{}", s),
        Err(e) => log_error(&format!("integrity_result_serialize_failed error={}", e)),
    }

    if matched {
        log_security(&format!("integrity_ok target={} sha256={}", target, actual));
        EXIT_OK
    } else {
        // TODO(watchdog): on mismatch, kill the agent (SIGKILL to --agent-pid,
        // privileged + local) and have the agent make one final phone-home before
        // going silent. Phone-home stays agent-side to keep this executor
        // network-less. Not wired yet — verdict only.
        log_security(&format!(
            "integrity_mismatch target={} expected={} actual={}",
            target, expected, actual
        ));
        EXIT_INTEGRITY
    }
}

#[cfg(unix)]
fn main() {
    let args: Vec<String> = std::env::args().collect();

    // The signed release manifest lists version_cmd "--version" for this
    // component, so the binary has to answer it. Positional, like every other
    // arm here, and ahead of any path that touches privilege or state.
    if matches!(args.get(1).map(|s| s.as_str()), Some("--version") | Some("-V")) {
        println!("RedFlag helper v{}", env!("REDFLAG_VERSION"));
        return;
    }

    // Subcommand dispatch. "verify-binary" is the runtime watchdog mode.
    if args.get(1).map(|s| s.as_str()) == Some("verify-binary") {
        std::process::exit(run_verify_binary(&args[2..]));
    }

    // "mint" is the standalone local authority (no fleet server on this host).
    // See RAF/security/06-standalone-authority.md.
    if args.get(1).map(|s| s.as_str()) == Some("mint") {
        std::process::exit(run_mint_cli(&args[2..]));
    }

    // Standalone mutation authority: signs a fully resolved pacman manifest
    // only after the helper has verified the package identities and signatures.
    if args.get(1).map(|s| s.as_str()) == Some("mint-envelope") {
        std::process::exit(run_mint_envelope_cli(&args[2..]));
    }

    // Verification-only compatibility path: proves and refuses without
    // consuming authorization replay state.
    if args.get(1).map(|s| s.as_str()) == Some("verify-envelope") {
        std::process::exit(run_verify_envelope_cli(&args[2..]));
    }

    // The mutation-envelope executor. Pacman is the first migrated backend;
    // every other backend still fails closed after common verification.
    if args.get(1).map(|s| s.as_str()) == Some("execute-envelope") {
        std::process::exit(run_execute_envelope_cli(&args[2..]));
    }

    // The stdin form remains available for direct root invocation. File transport
    // is the Agent protocol: exact argument shapes, joined token/result names, and
    // one fixed helper-self staging path. Sudoers wildcards are not a parser.
    let (token_file, result_file, helper_file) = match args.as_slice() {
        [_program] => (None, None, None),
        [_program, token_flag, token, result_flag, result]
            if token_flag == "--token-file" && result_flag == "--result-file" =>
        {
            (Some(token.as_str()), Some(result.as_str()), None)
        }
        [_program, token_flag, token, result_flag, result, helper_flag, helper]
            if token_flag == "--token-file"
                && result_flag == "--result-file"
                && helper_flag == "--helper-file"
                && helper == DEFAULT_HELPER_SELF_SOURCE =>
        {
            (
                Some(token.as_str()),
                Some(result.as_str()),
                Some(helper.as_str()),
            )
        }
        _ => {
            log_error("usage: redflag-helper [--token-file <path> --result-file <path> [--helper-file /var/lib/redflag/agent/pending-helper.bin]]");
            std::process::exit(EXIT_BAD_TOKEN);
        }
    };

    if let (Some(token), Some(result)) = (token_file, result_file) {
        if Path::new(token).file_name() != Path::new(result).file_name() {
            log_security("denied reason=capability_exchange_join_mismatch exit=26");
            std::process::exit(EXIT_TRUST_PATH);
        }
    }

    let result = match run(token_file, helper_file) {
        Ok(r) => r,
        Err(boxed) => {
            let (token, denial) = *boxed;
            let decision = if denial.code == EXIT_EXEC_FAILED {
                "failed"
            } else {
                "denied"
            };
            log_security(&format!(
                "{} reason={} detail={} exit={}",
                decision, denial.reason, denial.detail, denial.code
            ));
            let r = PolicyResult {
                token_id: token
                    .as_ref()
                    .map(|t| t.token_id.clone())
                    .unwrap_or_default(),
                agent_id: token
                    .as_ref()
                    .map(|t| t.agent_id.clone())
                    .unwrap_or_default(),
                package_type: token
                    .as_ref()
                    .map(|t| t.package_type.clone())
                    .unwrap_or_default(),
                operation: token
                    .as_ref()
                    .map(|t| t.operation.clone())
                    .unwrap_or_default(),
                decision: decision.to_string(),
                reason: denial.reason.to_string(),
                executed: false,
                verified_artifacts: 0,
                exit_code: denial.code,
                error: Some(denial.detail),
                timestamp: now_unix(),
            };
            if let Some(rp) = result_file {
                #[cfg(unix)]
                if let Err(write_denial) =
                    write_agent_exchange_json(&r, rp, DEFAULT_AGENT_RESULT_DIR)
                {
                    log_error(&format!(
                        "result_write_failed reason={} detail={}",
                        write_denial.reason, write_denial.detail
                    ));
                    std::process::exit(write_denial.code);
                }
                #[cfg(not(unix))]
                emit_result_to_file(&r, rp);
            }
            emit_result(&r);
            std::process::exit(denial.code);
        }
    };

    log_security(&format!(
        "executed token_id={} package_type={}",
        result.token_id, result.package_type
    ));
    if let Some(rp) = result_file {
        #[cfg(unix)]
        if let Err(denial) = write_agent_exchange_json(&result, rp, DEFAULT_AGENT_RESULT_DIR) {
            log_error(&format!(
                "result_write_failed reason={} detail={}",
                denial.reason, denial.detail
            ));
            std::process::exit(denial.code);
        }
        #[cfg(not(unix))]
        emit_result_to_file(&result, rp);
    }
    emit_result(&result);
    std::process::exit(EXIT_OK);
}
