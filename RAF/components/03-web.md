# Web Component

**React dashboard, embedded into the server binary — the operator's single pane of glass.**

---

## Stack

React 18 + TypeScript 5 + Vite + Tailwind 3, react-router 6, react-hot-toast. No state framework beyond a small store (`lib/store.ts`); server is the source of truth, the UI polls.

**Build embedding:** the production bundle is staged into `server/internal/webui/dist` before the server compiles — that directory is gitignored, so a bare `go build` embeds an *empty* UI. The release pipeline stages it; local dev runs Vite separately. See [deployment/01-docker-stack](../deployment/01-docker-stack.md).

**Aesthetic:** hand-crafted 90's Novell look. This is deliberate and load-bearing for the project's identity — no modern flat-design rewrites.

---

## Structure

```
web/src/
├── pages/          # Route-level views
│   ├── Dashboard, Agents, Updates, PackageDetail
│   ├── Docker, History, LiveOperations
│   ├── SecuritySettings, Settings, settings/, RateLimiting
│   └── Setup, Login, TokenManagement
├── components/
│   ├── primitives/         # FilterBar, SearchInput, FilterDropdown, FilterPill, FilterCountButton,
│   │                       # SortableTable, StateBadge (StatusBadge/SeverityBadge), CommandCard,
│   │                       # CommandStatusBadge, Modal, PageState, Pagination, StatCard,
│   │                       # ScreenshotCard, MetricItem, ProcessTable
│   ├── security/           # Security health panels
│   ├── DependencyClosureTree, VulnerabilityList
│   ├── ProcessesTab, ProcessDetailModal
│   └── AgentHealth, HistoryTimeline, AttentionPanel, ...
├── hooks/          # Stateful composition over primitives
│   ├── useFilterUrl        # URL-synced filter state (useFilterUrl.ts)
│   ├── useQueryParser      # key:value query string parsing (useQueryParser.ts)
│   ├── useMultimodalFilter # Composed filter: search box + filter pills + URL (useMultimodalFilter.ts)
│   ├── useDebounce         # Generic debounce (useDebounce.ts)
│   └── useColumnSort       # Reusable column sort state (useColumnSort.tsx)
├── lib/
│   ├── api.ts              # API client (web-auth boundary)
│   ├── polling.ts          # POLL.* constants — all intervals centralized
│   ├── store.ts, queryParser.ts, vulnerabilities.ts
│   └── client-logger.ts    # Client errors ship to the server log (ETHOS #1)
├── desktop/        # Tauri tray-app variant (vite.desktop.config.ts)
└── types/          # Shared TS types mirroring server models
```

---

## Conventions

- **One way to render state.** Status and severity render through `StatusBadge` / `SeverityBadge` — never ad-hoc colored spans. Tables that sort use `SortableTable`.
- **One way to filter.** Filter state syncs to URL via `useFilterUrl`; free-text search uses a local `useState` + `useDebounce` pair (instant feedback in the input, debounced value for API calls). Compose both into a `FilterBar`. No ad-hoc `useState` chains for filter state.
- **Polling intervals** come from `POLL.*` in `lib/polling.ts` — no hardcoded milliseconds in components.
- **Render the divergence, not the union** (framework §11.8): when agent-reported and server-expected state differ, the UI shows the difference, it does not paper over it.
- All routes sit behind `WebAuthMiddleware` (admin routes additionally behind `AdminRoleMiddleware`) — see [security/01-trust-boundaries](../security/01-trust-boundaries.md).

---

## Honest Gaps

- No automated web tests ([testing/01-test-pyramid](../testing/01-test-pyramid.md))
- Mobile layout usable, not optimized
- Several UI coverage gaps tracked as `UI-*` tasks (not architecture — task tier)

---

*Last reviewed: 2026-06-14*
