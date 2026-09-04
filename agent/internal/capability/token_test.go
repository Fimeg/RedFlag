package capability

import (
	"crypto/ed25519"
	"testing"
)

// Cross-language contract vector. The Rust executor (helper/src/main.rs) asserts
// these same strings for the same input. If either side drifts, both break.
func TestCanonicalVector(t *testing.T) {
	tok := &Token{
		Version: 1, TokenID: "tok-1", AgentID: "agent-123",
		PackageType: "npm", Operation: "install",
		Closure: []ClosureEntry{
			{Name: "left-pad", Version: "1.3.0", SHA256: "aaaa"},
			{Name: "is-odd", Version: "2.0.0", SHA256: "bbbb"},
		},
		ExpiresAt: 1700000000,
	}
	const wantHash = "49a181cd7b6df83a6dc83c7f647f2d224effe90907dd5dcdfdaf4c8697af595f"
	const wantMsg = "agent-123:tok-1:install:npm:" + wantHash + ":1700000000"
	if got := tok.ClosureHash(); got != wantHash {
		t.Fatalf("ClosureHash() = %q, want %q", got, wantHash)
	}
	if got := tok.CanonicalMessage(); got != wantMsg {
		t.Fatalf("CanonicalMessage() = %q, want %q", got, wantMsg)
	}
}

func TestClosureHashOrderIndependent(t *testing.T) {
	a := &Token{Closure: []ClosureEntry{{Name: "a", Version: "1", SHA256: "x"}, {Name: "b", Version: "2", SHA256: "y"}}}
	b := &Token{Closure: []ClosureEntry{{Name: "b", Version: "2", SHA256: "y"}, {Name: "a", Version: "1", SHA256: "x"}}}
	if a.ClosureHash() != b.ClosureHash() {
		t.Fatalf("closure hash depends on order: %q != %q", a.ClosureHash(), b.ClosureHash())
	}
}

func TestSignVerifyRoundtrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	tok := &Token{
		Version: 1, TokenID: "tok-2", AgentID: "agent-9",
		PackageType: "dnf", Operation: "upgrade",
		Closure:   []ClosureEntry{{Name: "openssl", Version: "3.2.1", SHA256: "deadbeef"}},
		ExpiresAt: 1700000000,
	}
	if _, err := tok.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if tok.KeyID != KeyIDFor(pub) {
		t.Fatalf("KeyID = %q, want %q", tok.KeyID, KeyIDFor(pub))
	}
	if err := tok.Verify(pub); err != nil {
		t.Fatalf("Verify after Sign failed: %v", err)
	}
	// Tamper detection: any change to the closure breaks verification.
	tok.Closure[0].Version = "3.2.2"
	if err := tok.Verify(pub); err == nil {
		t.Fatal("Verify accepted a tampered closure")
	}
}
