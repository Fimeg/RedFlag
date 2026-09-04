//! Mutation manifest and authorization contract.
//!
//! Current closure-token execution remains unchanged. Pacman is the first
//! envelope executor; the bytes remain backend-neutral across Server, Agent,
//! and helper.

use ed25519_dalek::{Signature, Verifier, VerifyingKey};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

pub const MUTATION_PROTOCOL_VERSION: u32 = 1;

/// Ceiling on `expires_at - not_before`. Derived, not chosen: it is the fleet
/// minter's DefaultTokenTTL. Standalone mint is tighter still at 600s.
pub const MAX_AUTHORIZATION_LIFETIME_SECS: i64 = 3600;

const MANIFEST_DOMAIN: &str = "redflag.mutation-manifest";
const ACTION_DOMAIN: &str = "redflag.resolved-action";
const EVIDENCE_DOMAIN: &str = "redflag.evidence";
const AUTHORIZATION_DOMAIN: &str = "redflag.mutation-authorization";
const RECEIPT_DOMAIN: &str = "redflag.mutation-receipt";

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
pub struct ResolvedAction {
    pub kind: String,
    pub identity: String,
    pub payload: String,
}

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
pub struct Evidence {
    pub kind: String,
    pub digest: String,
}

/// `target_id` MUST be the locally provisioned RedFlag agent identity. The
/// generic name is deliberate: a later protocol may define another namespace.
#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
pub struct MutationManifest {
    pub protocol_version: u32,
    pub operation_id: String,
    pub target_id: String,
    pub backend: String,
    pub operation: String,
    pub resolved_actions: Vec<ResolvedAction>,
    pub evidence: Vec<Evidence>,
}

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
pub struct MutationAuthorization {
    pub protocol_version: u32,
    pub authorization_id: String,
    pub manifest_hash: String,
    pub authority_kind: String,
    pub authority_id: String,
    pub target_id: String,
    pub issued_at: i64,
    pub not_before: i64,
    pub expires_at: i64,
    pub decision: String,
    pub key_id: String,
    pub signature: String,
}

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
pub struct MutationEnvelope {
    pub manifest: MutationManifest,
    pub authorization: MutationAuthorization,
}

fn write_lp(out: &mut Vec<u8>, value: &[u8]) {
    out.extend_from_slice(value.len().to_string().as_bytes());
    out.push(b':');
    out.extend_from_slice(value);
}

fn canonical_record(domain: &str, values: &[&str]) -> Vec<u8> {
    let mut out = Vec::new();
    write_lp(&mut out, domain.as_bytes());
    for value in values {
        write_lp(&mut out, value.as_bytes());
    }
    out
}

impl ResolvedAction {
    fn canonical_bytes(&self) -> Vec<u8> {
        canonical_record(ACTION_DOMAIN, &[&self.kind, &self.identity, &self.payload])
    }
}

impl Evidence {
    fn canonical_bytes(&self) -> Vec<u8> {
        canonical_record(EVIDENCE_DOMAIN, &[&self.kind, &self.digest])
    }
}

impl MutationManifest {
    pub fn canonical_bytes(&self) -> Vec<u8> {
        let version = self.protocol_version.to_string();
        let mut out = Vec::new();
        write_lp(&mut out, MANIFEST_DOMAIN.as_bytes());
        for value in [
            version.as_str(),
            self.operation_id.as_str(),
            self.target_id.as_str(),
            self.backend.as_str(),
            self.operation.as_str(),
        ] {
            write_lp(&mut out, value.as_bytes());
        }

        let mut actions: Vec<Vec<u8>> = self
            .resolved_actions
            .iter()
            .map(ResolvedAction::canonical_bytes)
            .collect();
        actions.sort();
        write_lp(&mut out, actions.len().to_string().as_bytes());
        for action in actions {
            write_lp(&mut out, &action);
        }

        let mut evidence: Vec<Vec<u8>> = self
            .evidence
            .iter()
            .map(Evidence::canonical_bytes)
            .collect();
        evidence.sort();
        write_lp(&mut out, evidence.len().to_string().as_bytes());
        for item in evidence {
            write_lp(&mut out, &item);
        }
        out
    }

    pub fn hash(&self) -> String {
        hex::encode(Sha256::digest(self.canonical_bytes()))
    }

    pub fn validate(&self) -> Result<(), String> {
        if self.protocol_version != MUTATION_PROTOCOL_VERSION {
            return Err(format!(
                "mutation protocol: unsupported manifest version {}",
                self.protocol_version
            ));
        }
        for (name, value) in [
            ("operation_id", self.operation_id.as_str()),
            ("target_id", self.target_id.as_str()),
            ("backend", self.backend.as_str()),
            ("operation", self.operation.as_str()),
        ] {
            if value.is_empty() {
                return Err(format!("mutation protocol: manifest {name} is empty"));
            }
        }
        if self.resolved_actions.is_empty() {
            return Err("mutation protocol: manifest has no resolved actions".into());
        }
        for (index, action) in self.resolved_actions.iter().enumerate() {
            if action.kind.is_empty() || action.identity.is_empty() || action.payload.is_empty() {
                return Err(format!(
                    "mutation protocol: resolved action {index} is incomplete"
                ));
            }
            if serde_json::from_str::<serde_json::Value>(&action.payload).is_err() {
                return Err(format!(
                    "mutation protocol: resolved action {index} payload is not JSON"
                ));
            }
        }
        for (index, evidence) in self.evidence.iter().enumerate() {
            if evidence.kind.is_empty() {
                return Err(format!("mutation protocol: evidence {index} kind is empty"));
            }
            let digest = hex::decode(&evidence.digest).map_err(|_| {
                format!("mutation protocol: evidence {index} digest is not SHA-256 hex")
            })?;
            if digest.len() != 32 {
                return Err(format!(
                    "mutation protocol: evidence {index} digest is not SHA-256 hex"
                ));
            }
        }
        Ok(())
    }
}

impl MutationAuthorization {
    pub fn canonical_message(&self) -> Vec<u8> {
        let version = self.protocol_version.to_string();
        let issued_at = self.issued_at.to_string();
        let not_before = self.not_before.to_string();
        let expires_at = self.expires_at.to_string();
        canonical_record(
            AUTHORIZATION_DOMAIN,
            &[
                &version,
                &self.authorization_id,
                &self.manifest_hash,
                &self.authority_kind,
                &self.authority_id,
                &self.target_id,
                &issued_at,
                &not_before,
                &expires_at,
                &self.decision,
                &self.key_id,
            ],
        )
    }

    pub fn verify(
        &self,
        public_key: &VerifyingKey,
        manifest: &MutationManifest,
    ) -> Result<(), String> {
        manifest.validate()?;
        self.validate_shape()?;
        if self.target_id != manifest.target_id {
            return Err(
                "mutation protocol: authorization target does not match manifest target".into(),
            );
        }
        if self.manifest_hash != manifest.hash() {
            return Err("mutation protocol: authorization manifest hash mismatch".into());
        }
        let expected_key_id = key_id_for(public_key.as_bytes());
        if self.key_id != expected_key_id {
            return Err("mutation protocol: authorization key id mismatch".into());
        }
        let signature_bytes = hex::decode(&self.signature)
            .map_err(|_| "mutation protocol: malformed signature".to_string())?;
        let signature = Signature::from_slice(&signature_bytes)
            .map_err(|_| "mutation protocol: malformed signature".to_string())?;
        public_key
            .verify(&self.canonical_message(), &signature)
            .map_err(|_| "mutation protocol: signature verification failed".to_string())
    }

    pub fn verify_for_execution_at(
        &self,
        public_key: &VerifyingKey,
        manifest: &MutationManifest,
        now: i64,
    ) -> Result<(), String> {
        self.verify(public_key, manifest)?;
        if self.decision != "allow" {
            return Err(format!(
                "mutation protocol: authorization decision is {:?}",
                self.decision
            ));
        }
        if now < self.not_before || now > self.expires_at {
            return Err("mutation protocol: authorization is outside its time window".into());
        }
        Ok(())
    }

    fn validate_shape(&self) -> Result<(), String> {
        if self.protocol_version != MUTATION_PROTOCOL_VERSION {
            return Err(format!(
                "mutation protocol: unsupported authorization version {}",
                self.protocol_version
            ));
        }
        for (name, value) in [
            ("authorization_id", self.authorization_id.as_str()),
            ("authority_kind", self.authority_kind.as_str()),
            ("authority_id", self.authority_id.as_str()),
            ("target_id", self.target_id.as_str()),
            ("decision", self.decision.as_str()),
        ] {
            if value.is_empty() {
                return Err(format!("mutation protocol: authorization {name} is empty"));
            }
        }
        if !is_canonical_uuid_v4(&self.authorization_id) {
            return Err("mutation protocol: authorization_id is not a canonical UUID v4".into());
        }
        if self.issued_at <= 0
            || self.not_before < self.issued_at
            || self.expires_at <= self.not_before
        {
            return Err("mutation protocol: invalid authorization time window".into());
        }
        if self.expires_at - self.not_before > MAX_AUTHORIZATION_LIFETIME_SECS {
            return Err(format!(
                "mutation protocol: authorization lifetime {}s exceeds the {}s ceiling",
                self.expires_at - self.not_before,
                MAX_AUTHORIZATION_LIFETIME_SECS
            ));
        }
        Ok(())
    }
}

impl MutationEnvelope {
    pub fn verify_for_execution_at(
        &self,
        public_key: &VerifyingKey,
        now: i64,
    ) -> Result<(), String> {
        self.authorization
            .verify_for_execution_at(public_key, &self.manifest, now)
    }

    /// Copies the audit join out of signed bytes rather than retyping it.
    pub fn receipt_for(&self, outcome: MutationOutcome, now: i64) -> MutationReceipt {
        MutationReceipt {
            protocol_version: MUTATION_PROTOCOL_VERSION,
            operation_id: self.manifest.operation_id.clone(),
            manifest_hash: self.manifest.hash(),
            authorization_id: self.authorization.authorization_id.clone(),
            target_id: self.manifest.target_id.clone(),
            backend: self.manifest.backend.clone(),
            operation: self.manifest.operation.clone(),
            decision: outcome.decision,
            reason: outcome.reason,
            executed: outcome.executed,
            verified_actions: outcome.verified_actions,
            exit_code: outcome.exit_code,
            error: outcome.detail,
            timestamp: now,
        }
    }
}

/// What the executor did, separate from which envelope it did it to.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MutationOutcome {
    pub decision: String,
    pub reason: String,
    pub executed: bool,
    pub verified_actions: u32,
    pub exit_code: i32,
    pub detail: String,
}

/// The response half of the contract: what the privileged executor did with one
/// envelope. Carries the operation/manifest/authorization join so a local
/// receipt and a server history row join without either guessing.
///
/// Not signed. The executor is not a second authority.
#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
pub struct MutationReceipt {
    pub protocol_version: u32,
    pub operation_id: String,
    pub manifest_hash: String,
    pub authorization_id: String,
    pub target_id: String,
    pub backend: String,
    pub operation: String,
    pub decision: String,
    pub reason: String,
    pub executed: bool,
    pub verified_actions: u32,
    pub exit_code: i32,
    #[serde(default)]
    pub error: String,
    pub timestamp: i64,
}

impl MutationReceipt {
    pub fn canonical_bytes(&self) -> Vec<u8> {
        let version = self.protocol_version.to_string();
        let executed = if self.executed { "true" } else { "false" };
        let verified_actions = self.verified_actions.to_string();
        let exit_code = self.exit_code.to_string();
        let timestamp = self.timestamp.to_string();
        canonical_record(
            RECEIPT_DOMAIN,
            &[
                &version,
                &self.operation_id,
                &self.manifest_hash,
                &self.authorization_id,
                &self.target_id,
                &self.backend,
                &self.operation,
                &self.decision,
                &self.reason,
                executed,
                &verified_actions,
                &exit_code,
                &self.error,
                &timestamp,
            ],
        )
    }

    pub fn digest(&self) -> String {
        hex::encode(Sha256::digest(self.canonical_bytes()))
    }
}

pub fn key_id_for(public_key: &[u8; 32]) -> String {
    let digest = Sha256::digest(public_key);
    hex::encode(&digest[..16])
}

/// 8-4-4-4-12 lowercase hex with the version (4) and variant (8/9/a/b) nibbles
/// set. Same discipline the standalone mint applies to request_id, applied here
/// before authorization_id can become an executor replay key: a newline-delimited
/// ledger matched by exact line has no defence against an embedded newline.
pub fn is_canonical_uuid_v4(s: &str) -> bool {
    let bytes = s.as_bytes();
    if bytes.len() != 36 {
        return false;
    }
    for (i, &b) in bytes.iter().enumerate() {
        let is_separator = matches!(i, 8 | 13 | 18 | 23);
        if is_separator {
            if b != b'-' {
                return false;
            }
        } else if !b.is_ascii_hexdigit() || b.is_ascii_uppercase() {
            return false;
        }
    }
    bytes[14] == b'4' && matches!(bytes[19], b'8' | b'9' | b'a' | b'b')
}

#[cfg(test)]
mod tests {
    use ed25519_dalek::{Signer, SigningKey};
    use serde::Deserialize;

    use super::*;

    #[derive(Deserialize)]
    struct GoldenFixture {
        manifest: MutationManifest,
        authorization: MutationAuthorization,
        receipt: MutationReceipt,
        test_seed: String,
        expected_manifest_canonical_hex: String,
        expected_manifest_hash: String,
        expected_authorization_canonical_hex: String,
        expected_key_id: String,
        expected_signature: String,
        expected_receipt_canonical_hex: String,
        expected_receipt_digest: String,
    }

    fn fixture() -> GoldenFixture {
        serde_json::from_str(include_str!("../../protocol/testdata/mutation-golden.json"))
            .expect("shared mutation fixture must parse")
    }

    fn signed_fixture() -> (GoldenFixture, MutationAuthorization, VerifyingKey) {
        let fixture = fixture();
        let seed: [u8; 32] = hex::decode(&fixture.test_seed)
            .expect("seed hex")
            .try_into()
            .expect("32-byte seed");
        let signing_key = SigningKey::from_bytes(&seed);
        let verifying_key = signing_key.verifying_key();
        let mut authorization = fixture.authorization.clone();
        authorization.manifest_hash = fixture.manifest.hash();
        authorization.key_id = key_id_for(verifying_key.as_bytes());
        authorization.signature = hex::encode(
            signing_key
                .sign(&authorization.canonical_message())
                .to_bytes(),
        );
        (fixture, authorization, verifying_key)
    }

    #[test]
    fn golden_vector_matches_go() {
        let (fixture, authorization, verifying_key) = signed_fixture();
        if fixture.expected_manifest_hash.is_empty() {
            panic!(
                "fill fixture: manifest_canonical={}\nmanifest_hash={}\nauthorization_canonical={}\nkey_id={}\nsignature={}",
                hex::encode(fixture.manifest.canonical_bytes()),
                fixture.manifest.hash(),
                hex::encode(authorization.canonical_message()),
                authorization.key_id,
                authorization.signature,
            );
        }
        assert_eq!(
            hex::encode(fixture.manifest.canonical_bytes()),
            fixture.expected_manifest_canonical_hex
        );
        assert_eq!(fixture.manifest.hash(), fixture.expected_manifest_hash);
        assert_eq!(
            hex::encode(authorization.canonical_message()),
            fixture.expected_authorization_canonical_hex
        );
        assert_eq!(authorization.key_id, fixture.expected_key_id);
        assert_eq!(authorization.signature, fixture.expected_signature);
        assert_eq!(
            hex::encode(fixture.receipt.canonical_bytes()),
            fixture.expected_receipt_canonical_hex
        );
        assert_eq!(fixture.receipt.digest(), fixture.expected_receipt_digest);
        MutationEnvelope {
            manifest: fixture.manifest.clone(),
            authorization,
        }
        .verify_for_execution_at(&verifying_key, 1_700_000_100)
        .expect("golden authorization must verify");
    }

    #[test]
    fn ordering_is_irrelevant_but_duplicates_are_not_erased() {
        let (fixture, authorization, verifying_key) = signed_fixture();
        let mut reordered = fixture.manifest.clone();
        reordered.resolved_actions.reverse();
        reordered.evidence.reverse();
        assert_eq!(reordered.hash(), fixture.manifest.hash());
        authorization
            .verify(&verifying_key, &reordered)
            .expect("ordering must not change authority");

        let mut duplicate = fixture.manifest.clone();
        duplicate
            .resolved_actions
            .push(duplicate.resolved_actions[0].clone());
        assert_ne!(duplicate.hash(), fixture.manifest.hash());
        assert!(authorization.verify(&verifying_key, &duplicate).is_err());
    }

    #[test]
    fn executor_affecting_tamper_is_refused() {
        let (fixture, authorization, verifying_key) = signed_fixture();
        let mut changed = fixture.manifest.clone();
        changed.evidence[0].digest = "cc".repeat(32);
        assert!(authorization.verify(&verifying_key, &changed).is_err());

        let mut changed = fixture.manifest.clone();
        changed.resolved_actions[0].payload = changed.resolved_actions[0]
            .payload
            .replace("/var/cache/redflag", "/tmp");
        assert!(authorization.verify(&verifying_key, &changed).is_err());

        let mut changed = fixture.manifest.clone();
        changed.target_id = "body-other".into();
        assert!(authorization.verify(&verifying_key, &changed).is_err());

        let mut changed = fixture.manifest.clone();
        changed.backend = "wua".into();
        assert!(authorization.verify(&verifying_key, &changed).is_err());

        let mut changed = fixture.manifest.clone();
        changed.resolved_actions[0].identity = "zsh@6.0-1".into();
        assert!(authorization.verify(&verifying_key, &changed).is_err());

        let mut changed = authorization.clone();
        changed.issued_at += 1;
        assert!(changed.verify(&verifying_key, &fixture.manifest).is_err());
    }

    #[test]
    fn unknown_formats_and_non_executable_authority_fail_closed() {
        let (fixture, authorization, verifying_key) = signed_fixture();
        let mut changed = fixture.manifest.clone();
        changed.protocol_version += 1;
        assert!(changed.validate().is_err());

        let mut changed = authorization.clone();
        changed.protocol_version += 1;
        assert!(changed.verify(&verifying_key, &fixture.manifest).is_err());

        let mut denied = authorization.clone();
        denied.decision = "deny".into();
        let seed: [u8; 32] = hex::decode(&fixture.test_seed)
            .expect("seed hex")
            .try_into()
            .expect("32-byte seed");
        denied.signature = hex::encode(
            SigningKey::from_bytes(&seed)
                .sign(&denied.canonical_message())
                .to_bytes(),
        );
        assert!(denied
            .verify_for_execution_at(&verifying_key, &fixture.manifest, 1_700_000_100)
            .is_err());
        assert!(authorization
            .verify_for_execution_at(
                &verifying_key,
                &fixture.manifest,
                authorization.expires_at + 1
            )
            .is_err());
    }
    #[test]
    fn authorization_id_must_be_canonical_uuid_v4() {
        let (fixture, authorization, verifying_key) = signed_fixture();
        for bad in [
            "",
            "not-a-uuid",
            "550e8400-e29b-41d4-a716-44665544001",
            "550e8400-e29b-11d4-a716-446655440011",
            "550e8400-e29b-41d4-c716-446655440011",
            "550E8400-E29B-41D4-A716-446655440011",
            "550e8400-e29b-41d4-a716-4466554400\n1",
        ] {
            assert!(!is_canonical_uuid_v4(bad), "accepted {bad:?}");
            let mut changed = authorization.clone();
            changed.authorization_id = bad.into();
            assert!(changed.verify(&verifying_key, &fixture.manifest).is_err());
        }
        assert!(is_canonical_uuid_v4(&authorization.authorization_id));
    }

    #[test]
    fn authorization_lifetime_ceiling_is_enforced() {
        let (fixture, authorization, verifying_key) = signed_fixture();
        let seed: [u8; 32] = hex::decode(&fixture.test_seed)
            .expect("seed hex")
            .try_into()
            .expect("32-byte seed");
        let signing_key = SigningKey::from_bytes(&seed);

        let mut at_ceiling = authorization.clone();
        at_ceiling.expires_at = at_ceiling.not_before + MAX_AUTHORIZATION_LIFETIME_SECS;
        at_ceiling.signature =
            hex::encode(signing_key.sign(&at_ceiling.canonical_message()).to_bytes());
        at_ceiling
            .verify(&verifying_key, &fixture.manifest)
            .expect("an authorization exactly at the ceiling must verify");

        let mut over = authorization.clone();
        over.expires_at = over.not_before + MAX_AUTHORIZATION_LIFETIME_SECS + 1;
        over.signature = hex::encode(signing_key.sign(&over.canonical_message()).to_bytes());
        assert!(over.verify(&verifying_key, &fixture.manifest).is_err());
    }

    // Evidence carries digests. Operator reason prose stays in the journal, and
    // the shape check is what keeps it out.
    #[test]
    fn evidence_carries_digests_not_prose() {
        let fixture = fixture();
        for bad in [
            "",
            "operator accepted the CVE risk",
            &"a".repeat(63),
            &"z".repeat(64),
        ] {
            let mut changed = fixture.manifest.clone();
            changed.evidence[0].digest = bad.into();
            assert!(changed.validate().is_err(), "validated digest {bad:?}");
        }
    }

    #[test]
    fn target_binds_manifest_and_authorization() {
        let (fixture, authorization, verifying_key) = signed_fixture();
        assert_eq!(fixture.manifest.target_id, authorization.target_id);

        let mut split = authorization.clone();
        split.target_id = "6f1e2d3c-4b5a-4998-8877-665544332211".into();
        assert!(split.verify(&verifying_key, &fixture.manifest).is_err());
    }

    #[test]
    fn receipt_carries_the_audit_join() {
        let (fixture, authorization, _) = signed_fixture();
        let envelope = MutationEnvelope {
            manifest: fixture.manifest.clone(),
            authorization: authorization.clone(),
        };
        let receipt = envelope.receipt_for(
            MutationOutcome {
                decision: "denied".into(),
                reason: "backend_not_migrated".into(),
                executed: false,
                verified_actions: 0,
                exit_code: 18,
                detail: "backend=pacman".into(),
            },
            1_700_000_100,
        );

        assert_eq!(receipt.operation_id, fixture.manifest.operation_id);
        assert_eq!(receipt.manifest_hash, fixture.manifest.hash());
        assert_eq!(receipt.authorization_id, authorization.authorization_id);
        assert_eq!(receipt.target_id, fixture.manifest.target_id);
        assert_eq!(receipt.backend, fixture.manifest.backend);

        // The fields a lossy report drops first are still in the digest.
        for mutate in [
            (|r: &mut MutationReceipt| r.decision = "executed".into()) as fn(&mut MutationReceipt),
            |r: &mut MutationReceipt| r.reason = "operation_completed".into(),
            |r: &mut MutationReceipt| r.executed = true,
            |r: &mut MutationReceipt| r.verified_actions = 1,
            |r: &mut MutationReceipt| r.exit_code = 0,
            |r: &mut MutationReceipt| r.error = String::new(),
            |r: &mut MutationReceipt| r.timestamp += 1,
        ] {
            let mut changed = receipt.clone();
            mutate(&mut changed);
            assert_ne!(changed.digest(), receipt.digest());
        }
    }
}
