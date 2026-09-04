package capability

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type mutationProtocolFixture struct {
	Manifest                       MutationManifest      `json:"manifest"`
	Authorization                  MutationAuthorization `json:"authorization"`
	Receipt                        MutationReceipt       `json:"receipt"`
	TestSeed                       string                `json:"test_seed"`
	ExpectedManifestCanonicalHex   string                `json:"expected_manifest_canonical_hex"`
	ExpectedManifestHash           string                `json:"expected_manifest_hash"`
	ExpectedAuthorizationCanonical string                `json:"expected_authorization_canonical_hex"`
	ExpectedKeyID                  string                `json:"expected_key_id"`
	ExpectedSignature              string                `json:"expected_signature"`
	ExpectedReceiptCanonicalHex    string                `json:"expected_receipt_canonical_hex"`
	ExpectedReceiptDigest          string                `json:"expected_receipt_digest"`
}

func loadMutationProtocolFixture(t *testing.T) mutationProtocolFixture {
	t.Helper()
	raw, err := os.ReadFile("../../../protocol/testdata/mutation-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture mutationProtocolFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func signedMutationProtocolFixture(t *testing.T) (MutationManifest, MutationAuthorization, ed25519.PublicKey) {
	t.Helper()
	fixture := loadMutationProtocolFixture(t)
	seed, err := hex.DecodeString(fixture.TestSeed)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	authorization := fixture.Authorization
	if err := authorization.Sign(privateKey, fixture.Manifest); err != nil {
		t.Fatal(err)
	}
	return fixture.Manifest, authorization, privateKey.Public().(ed25519.PublicKey)
}

func cloneMutationManifest(t *testing.T, manifest MutationManifest) MutationManifest {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var clone MutationManifest
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func TestMutationProtocolGoldenVector(t *testing.T) {
	fixture := loadMutationProtocolFixture(t)
	manifest, authorization, publicKey := signedMutationProtocolFixture(t)

	if fixture.ExpectedManifestHash == "" {
		t.Fatalf(
			"fill fixture: manifest_canonical=%s\nmanifest_hash=%s\nauthorization_canonical=%s\nkey_id=%s\nsignature=%s\nreceipt_canonical=%s\nreceipt_digest=%s",
			hex.EncodeToString(manifest.CanonicalBytes()),
			manifest.Hash(),
			hex.EncodeToString(authorization.CanonicalMessage()),
			authorization.KeyID,
			authorization.Signature,
			hex.EncodeToString(fixture.Receipt.CanonicalBytes()),
			fixture.Receipt.Digest(),
		)
	}
	if got := hex.EncodeToString(manifest.CanonicalBytes()); got != fixture.ExpectedManifestCanonicalHex {
		t.Fatalf("manifest canonical bytes = %q, want %q", got, fixture.ExpectedManifestCanonicalHex)
	}
	if got := manifest.Hash(); got != fixture.ExpectedManifestHash {
		t.Fatalf("manifest hash = %q, want %q", got, fixture.ExpectedManifestHash)
	}
	if got := hex.EncodeToString(authorization.CanonicalMessage()); got != fixture.ExpectedAuthorizationCanonical {
		t.Fatalf("authorization canonical bytes = %q, want %q", got, fixture.ExpectedAuthorizationCanonical)
	}
	if authorization.KeyID != fixture.ExpectedKeyID {
		t.Fatalf("key id = %q, want %q", authorization.KeyID, fixture.ExpectedKeyID)
	}
	if authorization.Signature != fixture.ExpectedSignature {
		t.Fatalf("signature = %q, want %q", authorization.Signature, fixture.ExpectedSignature)
	}
	if got := hex.EncodeToString(fixture.Receipt.CanonicalBytes()); got != fixture.ExpectedReceiptCanonicalHex {
		t.Fatalf("receipt canonical bytes = %q, want %q", got, fixture.ExpectedReceiptCanonicalHex)
	}
	if got := fixture.Receipt.Digest(); got != fixture.ExpectedReceiptDigest {
		t.Fatalf("receipt digest = %q, want %q", got, fixture.ExpectedReceiptDigest)
	}
	envelope := MutationEnvelope{Manifest: manifest, Authorization: authorization}
	if err := envelope.VerifyForExecutionAt(publicKey, 1_700_000_100); err != nil {
		t.Fatalf("golden authorization did not verify: %v", err)
	}
}

func TestMutationProtocolOrderingAndDuplicateSemantics(t *testing.T) {
	manifest, authorization, publicKey := signedMutationProtocolFixture(t)

	reordered := cloneMutationManifest(t, manifest)
	reordered.ResolvedActions[0], reordered.ResolvedActions[1] = reordered.ResolvedActions[1], reordered.ResolvedActions[0]
	reordered.Evidence[0], reordered.Evidence[1] = reordered.Evidence[1], reordered.Evidence[0]
	if reordered.Hash() != manifest.Hash() {
		t.Fatal("manifest hash changed when action/evidence order changed")
	}
	if err := authorization.Verify(publicKey, reordered); err != nil {
		t.Fatalf("authorization rejected reordered manifest: %v", err)
	}

	duplicate := cloneMutationManifest(t, manifest)
	duplicate.ResolvedActions = append(duplicate.ResolvedActions, duplicate.ResolvedActions[0])
	if duplicate.Hash() == manifest.Hash() {
		t.Fatal("exact duplicate action was silently de-duplicated")
	}
	if err := authorization.Verify(publicKey, duplicate); err == nil {
		t.Fatal("authorization accepted a duplicate resolved action")
	}
}

func TestMutationProtocolExecutorAffectingTamperFails(t *testing.T) {
	manifest, authorization, publicKey := signedMutationProtocolFixture(t)

	tests := map[string]func(*MutationManifest){
		"provenance": func(m *MutationManifest) { m.Evidence[0].Digest = strings.Repeat("c", 64) },
		"execution location": func(m *MutationManifest) {
			m.ResolvedActions[0].Payload = strings.Replace(m.ResolvedActions[0].Payload, "/var/cache/redflag", "/tmp", 1)
		},
		"target":          func(m *MutationManifest) { m.TargetID = "6f1e2d3c-4b5a-4998-8877-665544332211" },
		"backend":         func(m *MutationManifest) { m.Backend = "wua" },
		"resolved action": func(m *MutationManifest) { m.ResolvedActions[0].Identity = "zsh@6.0-1" },
	}
	for name, tamper := range tests {
		t.Run(name, func(t *testing.T) {
			changed := cloneMutationManifest(t, manifest)
			tamper(&changed)
			if err := authorization.Verify(publicKey, changed); err == nil {
				t.Fatal("authorization accepted tampered manifest")
			}
		})
	}

	changedAuthorization := authorization
	changedAuthorization.IssuedAt++
	if err := changedAuthorization.Verify(publicKey, manifest); err == nil {
		t.Fatal("authorization accepted tampered authorization metadata")
	}
}

func TestMutationProtocolUnknownVersionsFailClosed(t *testing.T) {
	manifest, authorization, publicKey := signedMutationProtocolFixture(t)
	manifest.ProtocolVersion++
	if err := manifest.Validate(); err == nil {
		t.Fatal("unknown manifest version passed validation")
	}

	manifest.ProtocolVersion = MutationProtocolVersion
	authorization.ProtocolVersion++
	if err := authorization.Verify(publicKey, manifest); err == nil {
		t.Fatal("unknown authorization version passed verification")
	}
}

// The target fields carry the RedFlag agent identity. Both are signed and the
// verifier requires them equal, so an executor that binds either one to the
// host it read for itself has bound the whole envelope.
func TestMutationProtocolTargetBindsManifestAndAuthorization(t *testing.T) {
	manifest, authorization, publicKey := signedMutationProtocolFixture(t)
	if manifest.TargetID != authorization.TargetID {
		t.Fatal("golden fixture disagrees with itself about the target")
	}

	split := authorization
	split.TargetID = "6f1e2d3c-4b5a-4998-8877-665544332211"
	if err := split.Verify(publicKey, manifest); err == nil {
		t.Fatal("authorization for another target verified against this manifest")
	}
}

func TestMutationAuthorizationIDIsCanonicalUUIDv4(t *testing.T) {
	manifest, authorization, publicKey := signedMutationProtocolFixture(t)

	// A replay ledger matched line-by-line has no defence against an embedded
	// newline; the shape check is what makes the identifier safe to record.
	for _, bad := range []string{
		"",
		"not-a-uuid",
		"550e8400-e29b-41d4-a716-44665544001",
		"550e8400-e29b-11d4-a716-446655440011",
		"550e8400-e29b-41d4-c716-446655440011",
		"550E8400-E29B-41D4-A716-446655440011",
		"550e8400-e29b-41d4-a716-4466554400\n1",
	} {
		if IsCanonicalUUIDv4(bad) {
			t.Fatalf("accepted %q as a canonical UUID v4", bad)
		}
		changed := authorization
		changed.AuthorizationID = bad
		if err := changed.Verify(publicKey, manifest); err == nil {
			t.Fatalf("authorization with id %q verified", bad)
		}
	}

	if !IsCanonicalUUIDv4(authorization.AuthorizationID) {
		t.Fatalf("golden authorization_id %q is not a canonical UUID v4", authorization.AuthorizationID)
	}
}

func TestMutationAuthorizationLifetimeCeiling(t *testing.T) {
	manifest, authorization, publicKey := signedMutationProtocolFixture(t)

	seed, err := hex.DecodeString(loadMutationProtocolFixture(t).TestSeed)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)

	atCeiling := authorization
	atCeiling.ExpiresAt = atCeiling.NotBefore + MaxAuthorizationLifetimeSeconds
	if err := atCeiling.Sign(privateKey, manifest); err != nil {
		t.Fatalf("an authorization exactly at the ceiling must sign: %v", err)
	}
	if err := atCeiling.Verify(publicKey, manifest); err != nil {
		t.Fatalf("an authorization exactly at the ceiling must verify: %v", err)
	}

	overCeiling := authorization
	overCeiling.ExpiresAt = overCeiling.NotBefore + MaxAuthorizationLifetimeSeconds + 1
	if err := overCeiling.Sign(privateKey, manifest); err == nil {
		t.Fatal("minted an authorization past the lifetime ceiling")
	}
	if err := overCeiling.Verify(publicKey, manifest); err == nil {
		t.Fatal("verified an authorization past the lifetime ceiling")
	}
}

// Evidence carries digests. Operator reason prose and policy text stay in the
// authority's journal, and the shape check is what keeps them out.
func TestMutationEvidenceCarriesDigestsNotProse(t *testing.T) {
	manifest, _, _ := signedMutationProtocolFixture(t)

	for _, bad := range []string{"", "operator accepted the CVE risk", strings.Repeat("a", 63), strings.Repeat("z", 64)} {
		changed := cloneMutationManifest(t, manifest)
		changed.Evidence[0].Digest = bad
		if err := changed.Validate(); err == nil {
			t.Fatalf("manifest validated with evidence digest %q", bad)
		}
	}
}

func TestMutationReceiptCarriesAuditJoin(t *testing.T) {
	manifest, authorization, _ := signedMutationProtocolFixture(t)
	envelope := MutationEnvelope{Manifest: manifest, Authorization: authorization}

	receipt := envelope.ReceiptFor(MutationOutcome{
		Decision: "denied",
		Reason:   "backend_not_migrated",
		ExitCode: 18,
		Detail:   "backend=pacman",
	}, 1_700_000_100)
	if receipt.OperationID != manifest.OperationID ||
		receipt.ManifestHash != manifest.Hash() ||
		receipt.AuthorizationID != authorization.AuthorizationID ||
		receipt.TargetID != manifest.TargetID {
		t.Fatal("receipt lost the operation/manifest/authorization/target join")
	}
	if receipt.Backend != manifest.Backend || receipt.Operation != manifest.Operation {
		t.Fatal("receipt lost the backend/operation it answers")
	}

	// Every recorded field is in the canonical bytes, including the ones a
	// lossy report would drop first.
	for name, mutate := range map[string]func(*MutationReceipt){
		"decision":         func(r *MutationReceipt) { r.Decision = "executed" },
		"reason":           func(r *MutationReceipt) { r.Reason = "operation_completed" },
		"executed":         func(r *MutationReceipt) { r.Executed = true },
		"verified actions": func(r *MutationReceipt) { r.VerifiedActions = 1 },
		"exit code":        func(r *MutationReceipt) { r.ExitCode = 0 },
		"error":            func(r *MutationReceipt) { r.Error = "" },
		"timestamp":        func(r *MutationReceipt) { r.Timestamp++ },
	} {
		t.Run(name, func(t *testing.T) {
			changed := receipt
			mutate(&changed)
			if changed.Digest() == receipt.Digest() {
				t.Fatal("receipt digest ignored a recorded field")
			}
		})
	}
}
