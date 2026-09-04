package capability

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

const (
	// MutationProtocolVersion belongs to the manifest namespace, independently
	// of the current closure-based Token format. Backends opt into the envelope
	// path explicitly; pacman begins at the helper boundary.
	MutationProtocolVersion = 1

	// MaxAuthorizationLifetimeSeconds is the ceiling on expires_at - not_before.
	// Derived, not chosen: it is DefaultTokenTTL, the fleet minter's window
	// (server/internal/services/capability_minter.go). Standalone mint is
	// tighter still at 600s. Doctrine, not a knob — an authority that wants a
	// standing capability has to say so by minting again.
	MaxAuthorizationLifetimeSeconds = 3600

	manifestDomain      = "redflag.mutation-manifest"
	actionDomain        = "redflag.resolved-action"
	evidenceDomain      = "redflag.evidence"
	authorizationDomain = "redflag.mutation-authorization"
	receiptDomain       = "redflag.mutation-receipt"
)

// ResolvedAction carries exact backend-owned UTF-8 JSON bytes. The common
// protocol signs those bytes but does not reinterpret pacman, WUA, Winget,
// Docker, or self-update semantics into a fictional universal artifact.
type ResolvedAction struct {
	Kind     string `json:"kind"`
	Identity string `json:"identity"`
	Payload  string `json:"payload"`
}

// Evidence identifies provenance or policy evidence by digest. Execution
// location belongs in the resolved action payload, never in this trust class.
type Evidence struct {
	Kind   string `json:"kind"`
	Digest string `json:"digest"`
}

// MutationManifest is the immutable description an authority approves and
// an executor later receives unchanged.
//
// TargetID MUST be the locally provisioned RedFlag agent identity. The generic
// name is deliberate: a later protocol may define another target namespace.
type MutationManifest struct {
	ProtocolVersion int              `json:"protocol_version"`
	OperationID     string           `json:"operation_id"`
	TargetID        string           `json:"target_id"`
	Backend         string           `json:"backend"`
	Operation       string           `json:"operation"`
	ResolvedActions []ResolvedAction `json:"resolved_actions"`
	Evidence        []Evidence       `json:"evidence"`
}

// MutationAuthorization binds an authority decision to one manifest and body.
// Every field except Signature is inside CanonicalMessage, including KeyID.
type MutationAuthorization struct {
	ProtocolVersion int    `json:"protocol_version"`
	AuthorizationID string `json:"authorization_id"`
	ManifestHash    string `json:"manifest_hash"`
	AuthorityKind   string `json:"authority_kind"`
	AuthorityID     string `json:"authority_id"`
	TargetID        string `json:"target_id"`
	IssuedAt        int64  `json:"issued_at"`
	NotBefore       int64  `json:"not_before"`
	ExpiresAt       int64  `json:"expires_at"`
	Decision        string `json:"decision"`
	KeyID           string `json:"key_id"`
	Signature       string `json:"signature"`
}

// MutationEnvelope is the indivisible object handed across an authority or
// executor boundary. Verification always recomputes the manifest hash from the
// manifest carried beside its authorization.
type MutationEnvelope struct {
	Manifest      MutationManifest      `json:"manifest"`
	Authorization MutationAuthorization `json:"authorization"`
}

func writeLP(buf *bytes.Buffer, value []byte) {
	buf.WriteString(strconv.Itoa(len(value)))
	buf.WriteByte(':')
	buf.Write(value)
}

func canonicalRecord(domain string, values ...string) []byte {
	var buf bytes.Buffer
	writeLP(&buf, []byte(domain))
	for _, value := range values {
		writeLP(&buf, []byte(value))
	}
	return buf.Bytes()
}

func (a ResolvedAction) canonicalBytes() []byte {
	return canonicalRecord(actionDomain, a.Kind, a.Identity, a.Payload)
}

func (e Evidence) canonicalBytes() []byte {
	return canonicalRecord(evidenceDomain, e.Kind, e.Digest)
}

func sortedRecords[T any](values []T, encode func(T) []byte) [][]byte {
	records := make([][]byte, 0, len(values))
	for _, value := range values {
		records = append(records, encode(value))
	}
	sort.Slice(records, func(i, j int) bool { return bytes.Compare(records[i], records[j]) < 0 })
	return records
}

// CanonicalBytes is domain-separated and length-prefixed. Action and evidence
// ordering is irrelevant, while exact duplicates remain present and therefore
// change the hash. No set conversion is permitted here.
func (m MutationManifest) CanonicalBytes() []byte {
	var buf bytes.Buffer
	writeLP(&buf, []byte(manifestDomain))
	for _, value := range []string{
		strconv.Itoa(m.ProtocolVersion),
		m.OperationID,
		m.TargetID,
		m.Backend,
		m.Operation,
	} {
		writeLP(&buf, []byte(value))
	}

	actions := sortedRecords(m.ResolvedActions, func(a ResolvedAction) []byte { return a.canonicalBytes() })
	writeLP(&buf, []byte(strconv.Itoa(len(actions))))
	for _, action := range actions {
		writeLP(&buf, action)
	}

	evidence := sortedRecords(m.Evidence, func(e Evidence) []byte { return e.canonicalBytes() })
	writeLP(&buf, []byte(strconv.Itoa(len(evidence))))
	for _, item := range evidence {
		writeLP(&buf, item)
	}
	return buf.Bytes()
}

func (m MutationManifest) Hash() string {
	digest := sha256.Sum256(m.CanonicalBytes())
	return hex.EncodeToString(digest[:])
}

func (m MutationManifest) Validate() error {
	if m.ProtocolVersion != MutationProtocolVersion {
		return fmt.Errorf("mutation protocol: unsupported manifest version %d", m.ProtocolVersion)
	}
	for _, field := range [][2]string{
		{"operation_id", m.OperationID},
		{"target_id", m.TargetID},
		{"backend", m.Backend},
		{"operation", m.Operation},
	} {
		name, value := field[0], field[1]
		if value == "" {
			return fmt.Errorf("mutation protocol: manifest %s is empty", name)
		}
	}
	if len(m.ResolvedActions) == 0 {
		return fmt.Errorf("mutation protocol: manifest has no resolved actions")
	}
	for i, action := range m.ResolvedActions {
		if action.Kind == "" || action.Identity == "" || action.Payload == "" {
			return fmt.Errorf("mutation protocol: resolved action %d is incomplete", i)
		}
		if !json.Valid([]byte(action.Payload)) {
			return fmt.Errorf("mutation protocol: resolved action %d payload is not JSON", i)
		}
	}
	for i, evidence := range m.Evidence {
		if evidence.Kind == "" {
			return fmt.Errorf("mutation protocol: evidence %d kind is empty", i)
		}
		decoded, err := hex.DecodeString(evidence.Digest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("mutation protocol: evidence %d digest is not SHA-256 hex", i)
		}
	}
	return nil
}

func (a MutationAuthorization) CanonicalMessage() []byte {
	return canonicalRecord(
		authorizationDomain,
		strconv.Itoa(a.ProtocolVersion),
		a.AuthorizationID,
		a.ManifestHash,
		a.AuthorityKind,
		a.AuthorityID,
		a.TargetID,
		strconv.FormatInt(a.IssuedAt, 10),
		strconv.FormatInt(a.NotBefore, 10),
		strconv.FormatInt(a.ExpiresAt, 10),
		a.Decision,
		a.KeyID,
	)
}

// IsCanonicalUUIDv4 reports whether s is 8-4-4-4-12 lowercase hex with the
// version (4) and variant (8/9/a/b) nibbles set. Same discipline the standalone
// mint already applies to request_id, applied here before authorization_id can
// become an executor replay key: a newline-delimited replay ledger matched by
// exact line has no defence against an identifier that contains a newline.
func IsCanonicalUUIDv4(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			return false
		}
	}
	if s[14] != '4' {
		return false
	}
	switch s[19] {
	case '8', '9', 'a', 'b':
		return true
	}
	return false
}

func (a MutationAuthorization) validateShape() error {
	if a.ProtocolVersion != MutationProtocolVersion {
		return fmt.Errorf("mutation protocol: unsupported authorization version %d", a.ProtocolVersion)
	}
	for _, field := range [][2]string{
		{"authorization_id", a.AuthorizationID},
		{"authority_kind", a.AuthorityKind},
		{"authority_id", a.AuthorityID},
		{"target_id", a.TargetID},
		{"decision", a.Decision},
	} {
		name, value := field[0], field[1]
		if value == "" {
			return fmt.Errorf("mutation protocol: authorization %s is empty", name)
		}
	}
	if !IsCanonicalUUIDv4(a.AuthorizationID) {
		return fmt.Errorf("mutation protocol: authorization_id is not a canonical UUID v4")
	}
	if a.IssuedAt <= 0 || a.NotBefore < a.IssuedAt || a.ExpiresAt <= a.NotBefore {
		return fmt.Errorf("mutation protocol: invalid authorization time window")
	}
	if a.ExpiresAt-a.NotBefore > MaxAuthorizationLifetimeSeconds {
		return fmt.Errorf("mutation protocol: authorization lifetime %ds exceeds the %ds ceiling",
			a.ExpiresAt-a.NotBefore, MaxAuthorizationLifetimeSeconds)
	}
	return nil
}

func (a *MutationAuthorization) Sign(priv ed25519.PrivateKey, manifest MutationManifest) error {
	if len(priv) != ed25519.PrivateKeySize {
		return fmt.Errorf("mutation protocol: invalid private key size %d", len(priv))
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := a.validateShape(); err != nil {
		return err
	}
	if a.TargetID != manifest.TargetID {
		return fmt.Errorf("mutation protocol: authorization target does not match manifest target")
	}
	a.ManifestHash = manifest.Hash()
	a.KeyID = KeyIDFor(priv.Public().(ed25519.PublicKey))
	a.Signature = hex.EncodeToString(ed25519.Sign(priv, a.CanonicalMessage()))
	return nil
}

func (a MutationAuthorization) Verify(pub ed25519.PublicKey, manifest MutationManifest) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("mutation protocol: invalid public key size %d", len(pub))
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := a.validateShape(); err != nil {
		return err
	}
	if a.TargetID != manifest.TargetID {
		return fmt.Errorf("mutation protocol: authorization target does not match manifest target")
	}
	if a.ManifestHash != manifest.Hash() {
		return fmt.Errorf("mutation protocol: authorization manifest hash mismatch")
	}
	if a.KeyID != KeyIDFor(pub) {
		return fmt.Errorf("mutation protocol: authorization key id mismatch")
	}
	signature, err := hex.DecodeString(a.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("mutation protocol: malformed signature")
	}
	if !ed25519.Verify(pub, a.CanonicalMessage(), signature) {
		return fmt.Errorf("mutation protocol: signature verification failed")
	}
	return nil
}

// VerifyForExecutionAt adds the executor's decision and time checks to the
// cryptographic envelope verification.
func (a MutationAuthorization) VerifyForExecutionAt(pub ed25519.PublicKey, manifest MutationManifest, now int64) error {
	if err := a.Verify(pub, manifest); err != nil {
		return err
	}
	if a.Decision != "allow" {
		return fmt.Errorf("mutation protocol: authorization decision is %q", a.Decision)
	}
	if now < a.NotBefore || now > a.ExpiresAt {
		return fmt.Errorf("mutation protocol: authorization is outside its time window")
	}
	return nil
}

func (e MutationEnvelope) VerifyForExecutionAt(pub ed25519.PublicKey, now int64) error {
	return e.Authorization.VerifyForExecutionAt(pub, e.Manifest, now)
}

// MutationReceipt is the response half of the contract: what the privileged
// executor did with one envelope. It carries the audit join ARCH-002 names —
// operation ID, manifest hash, authorization ID — so a local receipt and a
// server history row can be joined without either guessing.
//
// It is not signed. The executor is not a second authority; this is a record
// produced inside the trust boundary that already ran the operation. Decision
// and Reason keep the PolicyResult taxonomy rather than inventing a new one.
type MutationReceipt struct {
	ProtocolVersion int    `json:"protocol_version"`
	OperationID     string `json:"operation_id"`
	ManifestHash    string `json:"manifest_hash"`
	AuthorizationID string `json:"authorization_id"`
	TargetID        string `json:"target_id"`
	Backend         string `json:"backend"`
	Operation       string `json:"operation"`
	Decision        string `json:"decision"` // executed | denied | failed
	Reason          string `json:"reason"`
	Executed        bool   `json:"executed"`
	VerifiedActions int    `json:"verified_actions"`
	ExitCode        int    `json:"exit_code"`
	Error           string `json:"error,omitempty"`
	Timestamp       int64  `json:"timestamp"`
}

// CanonicalBytes pins the receipt the same way the manifest is pinned, so a
// ledger can digest one without re-deriving field order from JSON. Every field
// is present even when empty — a refusal before parse still produces a receipt,
// and its emptiness is part of the record.
func (r MutationReceipt) CanonicalBytes() []byte {
	return canonicalRecord(
		receiptDomain,
		strconv.Itoa(r.ProtocolVersion),
		r.OperationID,
		r.ManifestHash,
		r.AuthorizationID,
		r.TargetID,
		r.Backend,
		r.Operation,
		r.Decision,
		r.Reason,
		strconv.FormatBool(r.Executed),
		strconv.Itoa(r.VerifiedActions),
		strconv.Itoa(r.ExitCode),
		r.Error,
		strconv.FormatInt(r.Timestamp, 10),
	)
}

func (r MutationReceipt) Digest() string {
	digest := sha256.Sum256(r.CanonicalBytes())
	return hex.EncodeToString(digest[:])
}

// MutationOutcome is what the executor did, separate from which envelope it
// did it to.
type MutationOutcome struct {
	Decision        string
	Reason          string
	Executed        bool
	VerifiedActions int
	ExitCode        int
	Detail          string
}

// ReceiptFor copies the audit join out of signed bytes rather than retyping it.
func (e MutationEnvelope) ReceiptFor(outcome MutationOutcome, now int64) MutationReceipt {
	return MutationReceipt{
		ProtocolVersion: MutationProtocolVersion,
		OperationID:     e.Manifest.OperationID,
		ManifestHash:    e.Manifest.Hash(),
		AuthorizationID: e.Authorization.AuthorizationID,
		TargetID:        e.Manifest.TargetID,
		Backend:         e.Manifest.Backend,
		Operation:       e.Manifest.Operation,
		Decision:        outcome.Decision,
		Reason:          outcome.Reason,
		Executed:        outcome.Executed,
		VerifiedActions: outcome.VerifiedActions,
		ExitCode:        outcome.ExitCode,
		Error:           outcome.Detail,
		Timestamp:       now,
	}
}
