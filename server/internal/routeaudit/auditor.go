// Package routeaudit enforces ETHOS #2 (no unauthenticated endpoints) by
// auditing every registered route at startup and verifying each carries auth
// middleware matching its trust boundary. Routes missing auth that are not on
// an explicit public allowlist cause the server to refuse boot with a CRITICAL
// log and os.Exit(1).
//
// A Recorder wrapper intercepts route registrations at the time they're made,
// capturing the full flattened handler chain (inherited group middleware +
// route-specific handlers) for later validation. This avoids the unsafe
// reflection that was previously needed to walk Gin's internal radix trees.
package routeaudit

import (
	"log"
	"os"
	"path"
	"reflect"

	"github.com/gin-gonic/gin"
)

// PublicPathSet is the explicit allowlist of routes that are intentionally
// unauthenticated. Every route registered on the Gin engine must carry a
// recognised auth middleware (agent JWT, web JWT, or metrics token) unless it
// appears in this set. Adding a path here is a reviewable act.
var PublicPathSet = map[string]bool{
	// Health checks — no auth so load balancers and monitoring can reach them.
	"/health":        true,
	"/api/health":    true,
	"/api/v1/health": true,

	// Authentication endpoints — login, logout are public entry points.
	// /auth/verify carries WebAuthMiddleware and is intentionally NOT in this set.
	"/api/v1/auth/login":  true,
	"/api/v1/auth/logout": true,

	// System metadata — public so installers and agents can discover the server.
	"/api/v1/public-key":  true,
	"/api/v1/public-keys": true,
	"/api/v1/info":        true,

	// Agent setup — public so new agents can bootstrap.
	"/api/v1/setup/agent":     true,
	"/api/v1/setup/templates": true,
	"/api/v1/setup/validate":  true,

	// Agent registration and token renewal — public entry points (guarded
	// internally by registration tokens and machine binding respectively).
	"/api/v1/agents/register": true,
	"/api/v1/fleet-join":      true,
	"/api/v1/agents/renew":    true,

	// Downloads — public for bootstrapping. Artifact download is public so
	// the server can compute hashes at approval time without an agent JWT.
	"/api/v1/downloads/:platform": true,
	"/api/v1/install/:platform":   true,
	"/api/v1/manifest":            true,
	"/api/v1/helper/:arch":        true,
	"/api/v1/desktop/:platform/:arch": true,
	"/api/v1/downloads/artifact":  true,
}

// Auditor validates that every registered route carries auth middleware
// matching its trust boundary.
type Auditor struct {
	publicPaths map[string]bool
	authClasses map[uintptr]string // middleware code pointer -> boundary class
}

// NewAuditor returns an Auditor initialised with the public route allowlist.
// Auth middleware must be registered via RegisterAuth before AuditAndExit.
func NewAuditor() *Auditor {
	return &Auditor{
		publicPaths: PublicPathSet,
		authClasses: map[uintptr]string{},
	}
}

// RegisterAuth records h as a recognised auth middleware for the given trust
// boundary class ("web", "agent", "metrics", ...). Returns the Auditor for
// chaining.
//
// Classification is by code-pointer identity, not symbol name. Register the
// EXACT instance used at route registration: when the compiler inlines a
// middleware constructor, each inline site gets its own copy of the closure
// body, so two separate constructor calls are not guaranteed to share a code
// pointer. The wiring rule is one middleware instance per trust boundary,
// stored in a variable, used at every route, registered here.
//
// History: classification used to parse runtime.FuncForPC symbol names. The
// compiler inlined middleware.AuthMiddleware across packages, renaming its
// closure to "main.main.AuthMiddleware.func1" — the package-qualified match
// missed it and the server refused boot (122 container restarts). The bare
// substring match that replaced it would silently classify ANY symbol
// containing "AuthMiddleware" as an auth boundary — a false negative waiting
// for a colliding name. Pointer identity over shared instances has neither
// failure mode: copies of one func value always compare equal, and an
// unregistered wrapper or stray fresh constructor call fails the audit
// loudly at boot instead of passing silently.
func (a *Auditor) RegisterAuth(class string, h gin.HandlerFunc) *Auditor {
	a.authClasses[reflect.ValueOf(h).Pointer()] = class
	return a
}

// classifyHandler returns the auth boundary class of a handler, or an empty
// string if the handler is not a registered auth middleware.
func (a *Auditor) classifyHandler(h gin.HandlerFunc) string {
	return a.authClasses[reflect.ValueOf(h).Pointer()]
}

// routeRecord captures a single auditable route with its full handler chain.
type routeRecord struct {
	method   string
	path     string
	handlers []gin.HandlerFunc
}

// Recorder wraps gin.Engine to intercept route registrations for audit.
// Each route registration records the full flattened handler chain
// (inherited group middleware + route-specific handlers) for validation
// by Auditor.Validate. All routing methods proxy through to the real engine.
type Recorder struct {
	engine *gin.Engine
	routes []routeRecord
}

// RecorderGroup wraps gin.RouterGroup to intercept route registrations
// within a group context, recording the full handler chain including
// inherited parent-group middleware. All routing methods proxy through
// to the real router group.
type RecorderGroup struct {
	recorder *Recorder
	group    *gin.RouterGroup
	prefix   string
}

// NewRecorder creates a Recorder wrapping the given gin engine.
func NewRecorder(engine *gin.Engine) *Recorder {
	return &Recorder{engine: engine}
}

// --- Recorder methods (engine-level route registration) ---

// GET records and registers a GET route.
func (r *Recorder) GET(path string, handlers ...gin.HandlerFunc) {
	r.record("GET", path, handlers)
	r.engine.GET(path, handlers...)
}

// POST records and registers a POST route.
func (r *Recorder) POST(path string, handlers ...gin.HandlerFunc) {
	r.record("POST", path, handlers)
	r.engine.POST(path, handlers...)
}

// PUT records and registers a PUT route.
func (r *Recorder) PUT(path string, handlers ...gin.HandlerFunc) {
	r.record("PUT", path, handlers)
	r.engine.PUT(path, handlers...)
}

// DELETE records and registers a DELETE route.
func (r *Recorder) DELETE(path string, handlers ...gin.HandlerFunc) {
	r.record("DELETE", path, handlers)
	r.engine.DELETE(path, handlers...)
}

// PATCH records and registers a PATCH route.
func (r *Recorder) PATCH(path string, handlers ...gin.HandlerFunc) {
	r.record("PATCH", path, handlers)
	r.engine.PATCH(path, handlers...)
}

// HEAD records and registers a HEAD route.
func (r *Recorder) HEAD(path string, handlers ...gin.HandlerFunc) {
	r.record("HEAD", path, handlers)
	r.engine.HEAD(path, handlers...)
}

// OPTIONS records and registers an OPTIONS route.
func (r *Recorder) OPTIONS(path string, handlers ...gin.HandlerFunc) {
	r.record("OPTIONS", path, handlers)
	r.engine.OPTIONS(path, handlers...)
}

// Use adds middleware to the engine. Proxied through to gin; engine-handler
// state is read from the real engine at route-registration time.
func (r *Recorder) Use(middleware ...gin.HandlerFunc) {
	r.engine.Use(middleware...)
}

// Group creates a new recorder group. The returned RecorderGroup wraps the
// real gin.RouterGroup and records all routes registered on it.
func (r *Recorder) Group(relativePath string, handlers ...gin.HandlerFunc) *RecorderGroup {
	realGroup := r.engine.Group(relativePath, handlers...)
	prefix := path.Join("/", relativePath)
	return &RecorderGroup{
		recorder: r,
		group:    realGroup,
		prefix:   prefix,
	}
}

// record captures a route registered at engine level. The full handler chain
// includes any engine-level middleware (added via Use) plus route-specific
// handlers.
func (r *Recorder) record(method, routePath string, handlers []gin.HandlerFunc) {
	fullHandlers := make([]gin.HandlerFunc, 0, len(r.engine.Handlers)+len(handlers))
	fullHandlers = append(fullHandlers, r.engine.Handlers...)
	fullHandlers = append(fullHandlers, handlers...)
	r.routes = append(r.routes, routeRecord{
		method:   method,
		path:     routePath,
		handlers: fullHandlers,
	})
}

// --- RecorderGroup methods (group-level route registration) ---

// GET records and registers a GET route on this group.
func (g *RecorderGroup) GET(path string, handlers ...gin.HandlerFunc) {
	g.record("GET", path, handlers)
	g.group.GET(path, handlers...)
}

// POST records and registers a POST route on this group.
func (g *RecorderGroup) POST(path string, handlers ...gin.HandlerFunc) {
	g.record("POST", path, handlers)
	g.group.POST(path, handlers...)
}

// PUT records and registers a PUT route on this group.
func (g *RecorderGroup) PUT(path string, handlers ...gin.HandlerFunc) {
	g.record("PUT", path, handlers)
	g.group.PUT(path, handlers...)
}

// DELETE records and registers a DELETE route on this group.
func (g *RecorderGroup) DELETE(path string, handlers ...gin.HandlerFunc) {
	g.record("DELETE", path, handlers)
	g.group.DELETE(path, handlers...)
}

// PATCH records and registers a PATCH route on this group.
func (g *RecorderGroup) PATCH(path string, handlers ...gin.HandlerFunc) {
	g.record("PATCH", path, handlers)
	g.group.PATCH(path, handlers...)
}

// HEAD records and registers a HEAD route on this group.
func (g *RecorderGroup) HEAD(path string, handlers ...gin.HandlerFunc) {
	g.record("HEAD", path, handlers)
	g.group.HEAD(path, handlers...)
}

// OPTIONS records and registers an OPTIONS route on this group.
func (g *RecorderGroup) OPTIONS(path string, handlers ...gin.HandlerFunc) {
	g.record("OPTIONS", path, handlers)
	g.group.OPTIONS(path, handlers...)
}

// Use adds middleware to this group. Proxied through to gin; group-handler
// state is read from the real group at route-registration time.
func (g *RecorderGroup) Use(middleware ...gin.HandlerFunc) {
	g.group.Use(middleware...)
}

// Group creates a nested recorder group that records all routes registered
// on the subgroup.
func (g *RecorderGroup) Group(relativePath string, handlers ...gin.HandlerFunc) *RecorderGroup {
	realGroup := g.group.Group(relativePath, handlers...)
	prefix := path.Join(g.prefix, relativePath)
	return &RecorderGroup{
		recorder: g.recorder,
		group:    realGroup,
		prefix:   prefix,
	}
}

// record captures a route registered on this group. The full handler chain
// includes all inherited group middleware (from parent groups and this
// group's Use calls) plus route-specific handlers. The full path is
// reconstructed from the group prefix and the relative route path.
func (g *RecorderGroup) record(method, routePath string, handlers []gin.HandlerFunc) {
	fullPath := path.Join(g.prefix, routePath)
	fullHandlers := make([]gin.HandlerFunc, 0, len(g.group.Handlers)+len(handlers))
	fullHandlers = append(fullHandlers, g.group.Handlers...)
	fullHandlers = append(fullHandlers, handlers...)
	g.recorder.routes = append(g.recorder.routes, routeRecord{
		method:   method,
		path:     fullPath,
		handlers: fullHandlers,
	})
}

// Validate iterates every recorded route and checks that each carries auth
// middleware matching its trust boundary. Returns a list of paths that lack
// auth and are not on the public allowlist.
func (a *Auditor) Validate(recorder *Recorder) []string {
	var violations []string
	for _, route := range recorder.routes {
		var hasAuth bool
		for _, h := range route.handlers {
			if a.classifyHandler(h) != "" {
				hasAuth = true
				break
			}
		}
		if !hasAuth && !a.publicPaths[route.path] {
			violations = append(violations, route.path)
		}
	}
	return violations
}

// AuditAndExit calls Validate and, if any violations are found, logs them at
// CRITICAL level and exits with code 1. This is the single entry point called
// from main() — the server refuses to boot when a route is missing its auth.
func (a *Auditor) AuditAndExit(recorder *Recorder) {
	if len(a.authClasses) == 0 {
		log.Printf("[CRITICAL] [server] [route-audit] no_auth_middleware_registered — RegisterAuth must be called before AuditAndExit; refusing to boot")
		os.Exit(1)
	}
	violations := a.Validate(recorder)
	if len(violations) > 0 {
		for _, path := range violations {
			log.Printf("[CRITICAL] [server] [route-audit] route_missing_auth path=%s", path)
		}
		log.Printf("[CRITICAL] [server] [route-audit] audit_failed count=%d — server refusing to boot", len(violations))
		os.Exit(1)
	}
	log.Printf("[INFO] [server] [route-audit] audit_passed — all routes carry expected auth")
}
