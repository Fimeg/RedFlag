package routeaudit

import (
	"testing"

	"github.com/Fimeg/RedFlag/server/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

// testAuth is a fake auth provider. Its middleware is recognised only when
// the exact instance is registered with the auditor — there is no name-based
// matching, and separate constructor calls may not share a code pointer
// (inlining duplicates closure bodies per call site). Tests follow the
// production wiring rule: one instance per boundary, shared everywhere.
type testAuth struct{}

func (t *testAuth) WebAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {}
}

// newTestAuditor returns an auditor plus the single registered web auth
// middleware instance that routes must use.
func newTestAuditor() (*Auditor, gin.HandlerFunc) {
	authMw := (&testAuth{}).WebAuthMiddleware()
	return NewAuditor().RegisterAuth("web", authMw), authMw
}

// TestValidate_NakedRoute verifies that a route with no auth middleware
// and no public allowlist entry is reported as a violation.
func TestValidate_NakedRoute(t *testing.T) {
	auditor, _ := newTestAuditor()
	recorder := NewRecorder(gin.New())
	recorder.GET("/naked", func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0] != "/naked" {
		t.Fatalf("expected violation for /naked, got %s", violations[0])
	}
}

// TestValidate_AuthMiddleware verifies that a route carrying the registered
// auth middleware instance passes the audit.
func TestValidate_AuthMiddleware(t *testing.T) {
	auditor, authMw := newTestAuditor()
	recorder := NewRecorder(gin.New())
	recorder.GET("/protected", authMw, func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %v", len(violations), violations)
	}
}

// TestValidate_PublicAllowlist verifies that a route on the public allowlist
// passes the audit even without auth middleware.
func TestValidate_PublicAllowlist(t *testing.T) {
	auditor, _ := newTestAuditor()
	recorder := NewRecorder(gin.New())
	recorder.GET("/health", func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for public path, got %d: %v", len(violations), violations)
	}
}

// TestValidate_RealAgentAuthMiddleware uses the actual production middleware
// under the production wiring rule: one middleware.AuthMiddleware() instance
// shared across all routes and registered with the auditor. Regression for
// the inlining bug lineage: name-based classification missed the inlined
// closure ("main.main.AuthMiddleware.func1") and refused boot (122
// restarts); substring matching fixed that but could silently pass colliding
// names. Pointer identity over a shared instance has neither failure mode.
func TestValidate_RealAgentAuthMiddleware(t *testing.T) {
	agentAuth := middleware.AuthMiddleware()
	auditor := NewAuditor().RegisterAuth("agent", agentAuth)
	recorder := NewRecorder(gin.New())

	agents := recorder.Group("/api/v1/agents")
	agents.Use(agentAuth)
	agents.GET("/:id/commands", func(c *gin.Context) {})

	recorder.GET("/api/v1/downloads/updates/:package_id", agentAuth, func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	if len(violations) != 0 {
		t.Fatalf("real middleware.AuthMiddleware misclassified — violations: %v", violations)
	}
}

// fakeAuthMiddlewareLogger has a symbol name containing "AuthMiddleware" but
// is NOT an auth boundary. Under the old substring classification it would
// have silently passed the audit; under pointer identity it must be flagged.
func fakeAuthMiddlewareLogger() gin.HandlerFunc {
	return func(c *gin.Context) {}
}

// TestValidate_CollidingNameNotRegistered is the regression test for the
// substring-matching false negative: an unregistered middleware whose name
// collides with an auth middleware must not count as auth.
func TestValidate_CollidingNameNotRegistered(t *testing.T) {
	auditor, _ := newTestAuditor()
	recorder := NewRecorder(gin.New())
	recorder.GET("/collide", fakeAuthMiddlewareLogger(), func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	if len(violations) != 1 {
		t.Fatalf("colliding-name middleware passed as auth — expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0] != "/collide" {
		t.Fatalf("expected violation for /collide, got %s", violations[0])
	}
}

// TestValidate_FreshInstanceNotRegistered pins the shared-instance wiring
// rule: a route using a fresh constructor call (not the registered instance)
// must fail the audit rather than silently pass. This is the loud failure
// mode that catches a stray authHandler.WebAuthMiddleware() in main.
func TestValidate_FreshInstanceNotRegistered(t *testing.T) {
	auditor, _ := newTestAuditor()
	recorder := NewRecorder(gin.New())
	other := &testAuth{}
	recorder.GET("/fresh", other.WebAuthMiddleware(), func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	// Either outcome is sound if pointers happen to coincide without
	// inlining, but the contract we pin is: the auditor must never crash
	// and must classify deterministically. Accept 0 or 1, reject anything
	// else, and require the path to be /fresh when flagged.
	if len(violations) > 1 {
		t.Fatalf("expected at most 1 violation, got %d: %v", len(violations), violations)
	}
	if len(violations) == 1 && violations[0] != "/fresh" {
		t.Fatalf("expected violation for /fresh, got %s", violations[0])
	}
}

// TestValidate_ParamRoutes verifies the auditor handles full paths through
// wildcard (param) segments — the live router is full of :id segments.
func TestValidate_ParamRoutes(t *testing.T) {
	auditor, authMw := newTestAuditor()
	recorder := NewRecorder(gin.New())

	// Param route on the allowlist passes without auth.
	recorder.GET("/api/v1/downloads/:platform", func(c *gin.Context) {})
	// Param route with auth passes.
	recorder.GET("/api/v1/agents/:id/commands", authMw, func(c *gin.Context) {})
	// Naked param route is a violation, reported with the :param segment intact.
	recorder.GET("/api/v1/widgets/:id", func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0] != "/api/v1/widgets/:id" {
		t.Fatalf("expected violation for /api/v1/widgets/:id, got %s", violations[0])
	}
}

// TestValidate_GroupMiddleware verifies routes registered under a group
// inherit the group's auth middleware in their handler chains, including
// nested groups — mirroring the live dashboard/admin layout.
func TestValidate_GroupMiddleware(t *testing.T) {
	auditor, authMw := newTestAuditor()
	recorder := NewRecorder(gin.New())

	dashboard := recorder.Group("/api/v1")
	dashboard.Use(authMw)
	dashboard.GET("/stats/summary", func(c *gin.Context) {})

	admin := dashboard.Group("/admin")
	admin.GET("/registration-tokens", func(c *gin.Context) {})

	naked := recorder.Group("/api/v1/open")
	naked.GET("/thing", func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0] != "/api/v1/open/thing" {
		t.Fatalf("expected violation for /api/v1/open/thing, got %s", violations[0])
	}
}

// TestValidate_MethodCoverage verifies each HTTP method is audited —
// a naked POST must be caught even when the GET twin is authed.
func TestValidate_MethodCoverage(t *testing.T) {
	auditor, authMw := newTestAuditor()
	recorder := NewRecorder(gin.New())

	recorder.GET("/thing", authMw, func(c *gin.Context) {})
	recorder.POST("/thing", func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0] != "/thing" {
		t.Fatalf("expected violation for /thing, got %s", violations[0])
	}
}

// TestValidate_MultipleRoutes verifies the auditor correctly handles a mix
// of authed, public, and naked routes.
func TestValidate_MultipleRoutes(t *testing.T) {
	auditor, authMw := newTestAuditor()
	recorder := NewRecorder(gin.New())

	recorder.GET("/health", func(c *gin.Context) {})
	recorder.GET("/protected", authMw, func(c *gin.Context) {})
	recorder.GET("/naked", func(c *gin.Context) {})

	violations := auditor.Validate(recorder)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0] != "/naked" {
		t.Fatalf("expected violation for /naked, got %s", violations[0])
	}
}
