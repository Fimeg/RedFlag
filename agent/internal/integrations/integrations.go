// Package integrations detects third-party software the agent can observe on its
// own host (Sunshine, etc.) and folds a compact report into the agent's regular
// system-info payload under metadata["integrations"].
//
// Architecture: observe-only. Detection inspects the local host and reports what
// it finds. It never starts, stops, configures, or otherwise commands the software
// it observes — that boundary is what keeps a remote-desktop integration from
// becoming a remote-control backdoor. The dashboard renders what the agent reports;
// it does not reach into the host.
package integrations

// Detect returns the integrations block for metadata["integrations"]. primaryIP is
// the agent's primary address, used to construct local management URLs (e.g. the
// Sunshine web UI). An empty map means nothing was detected — callers may omit it.
func Detect(primaryIP string) map[string]interface{} {
	out := make(map[string]interface{})

	if s := detectSunshine(primaryIP); s != nil {
		out["sunshine"] = s
	}

	return out
}
