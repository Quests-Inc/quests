# ADR-0001. Architecture: a modular monolith by contexts (backend) and FSD (frontend)

- Status: accepted
- Date: 2026-10-11
- Related: #9 (splitting big files and packages), #10 (API types), #11 (frontend), #16 (the overlay in Rust)

## Context

The code had grown by technical layers rather than by subject area (measurements at the date of the decision):

**Backend (Go, ≈24k lines without tests).**
- `store` — 8.3k lines and **195 methods** on a single `*Store`: data, invariants (`guardAuthor`, `guardExec`, `guardTemplateSteps`) and SQL together.
- `httpapi` — 8.4k lines in one package (41 files, up to 1345 lines), 309 calls to `s.Store.*`.
- `domain` — 520 lines (quest and step); notes, templates, questlines and attachments live as structs inside `store`.
- Needless links: `store → auth`, `llmassist → cli`, `metrics → store`, `events → hooks`; `schedule` contains 16 direct SQL queries bypassing `store`.
- One entity is smeared across layers: “template” is in 12 files of `store`, 7 of `httpapi`, 3 of `schedule`, 2 of `domain`.

**Frontend (Svelte + TypeScript, ≈27k lines).**
- Laid out by kind of file: `blocks`, `modals`, `ui`, `js`; giant pages (`GraphPage` 3643 lines, `NotesPage` 2555, `App` 2042, 85 functions).
- A single `api.ts` of 909 lines; 30 components import it directly; there are no boundaries between entities.

## Decision

### 1. Backend: a modular monolith by bounded contexts

One process and one database, but the code is divided into contexts with their own language, data and public interface. We take the **strategic** part of DDD (contexts, a ubiquitous language, dependency rules) and a **minimum of the tactical** part: domain types with invariants where they exist (quest statuses, the `wait_previous` chain, emit rules, command permissions, workspace isolation). Aggregates, a repository per entity and domain events for the sake of ceremony are not introduced: the domain is mostly CRUD, a scheduler and a graph.

| Context | What is inside |
|---|---|
| `journal` | quests, steps, questlines, categories, tags, the change history |
| `knowledge` | notes, attachments, search, references (`kind=ID`), note freshness checks |
| `automation` | templates and emit, step checks, runners and their jobs, the scheduler, command secrets |
| `identity` | accounts, sessions, tokens, MFA, workspaces, roles, invitations |
| `platform` | the database and RLS (`wsctx`), WebDAV, ClamAV, `secretbox`, `timeutil`, metrics, events, health, config |

The target layout (created as code is moved; no empty layers “in reserve”):

```
go/internal/
  platform/…            infrastructure and shared primitives
  identity/  journal/  knowledge/  automation/
      <ctx>.go …        root package: domain types, the service (scenarios), port interfaces
      pg/               the ports implemented on PostgreSQL (what is `store` now)
      http/             the context's handlers; they register routes in the shared mux
  httpapi/              the HTTP composition root: server, middleware (auth, workspace, CORS, headers), SPA, health/metrics
go/cmd/*                wiring the dependencies (composition root)
```

**The shared kernel.** Several contexts must apply the same small rules: a runner label, an icon file name, “who may write a command”. They live in `internal/kernel` (which depends only on `identity` and `platform`); the contexts `journal`, `knowledge` and `automation` import it. By meaning the leaf `internal/refs` (parsing `kind=ID` references) belongs there too.

**Amendment (#20): secrets belong to `automation`.** A secret exists only as the environment of a command a runner executes, is inherited along the chain step → quest → template → questline and reads those entities' tables directly, so it belongs to `automation`, not to `knowledge` as the first edition had it.

**Dependency rules** (checked in CI, see “Checking the boundaries”):

1. `platform` imports nothing from the contexts or from `httpapi`.
2. A context imports `platform` and the **root packages** of other contexts from a whitelist; the `pg` and `http` packages of other contexts — never.
3. Allowed links between contexts: `journal → identity`, `knowledge → identity`, `automation → identity, journal, knowledge`. The reverse is forbidden; `identity` depends on nothing.
4. `<ctx>/http` does not import `<ctx>/pg`: they are tied together by ports in `cmd/*`.
5. Everything that crosses contexts (search over everything, resolving `kind=ID` references, the graph) lives in a separate “reading” layer over the contexts' narrow interfaces, not inside another context's `store`.
6. Workspace isolation (RLS, `wsctx`) stays cross-cutting and belongs to `platform`.

**Amendment (#21): the scheduler is a coordinating package.** `internal/schedule` (template materialization, expiry, maintenance, purging) belongs to no single context: it decides *when* to do what and calls the contexts (`journal`, `automation`, `knowledge`). So it stays a top-level package allowed to depend on them, but it **does not touch the database directly** — only through the contexts' stores (`automation.Store.EmitRollFor/InsertEmitRoll/ListEnabledTemplateRows…`, `journal.Store.QuestIDForPeriod/ActiveQuestIDsPastDeadline`). The templates' rules (what “it is time” means) live in `schedule`, the data in `automation`.

Working components outside the API package layout (the bot, MCP, STT, and the workstation binaries `quests-overlay` / `quests-runner`) are not part of this scheme: they talk to the API only over HTTP (`docs/architecture.md`).

### 2. Frontend: Feature-Sliced Design

Layers `app → pages → widgets → features → entities → shared`, imports only downwards, every slice with a public API in `index.ts`.

| Layer | For Quests |
|---|---|
| `app` | the shell, tabs and route state, the theme, the session, global styles |
| `pages` | journal, notes, attachments, calendar, history, map, runners |
| `widgets` | the header, the quest sidebar, the activity calendar, the map canvas |
| `features` | edit-quest, upload-attachment, run-template, search, assistant… |
| `entities` | quest, questline, note, attachment, template, tag, runner, workspace (model + queries + atomic UI) |
| `shared` | `api` (the client and generated types), `ui`, `lib`, `styles`, `config` |

FSD entities match the backend's contexts and entities by name; the types generated from Go (`tygo`) live in `shared/api`, and their use lives in `entities/<x>/api`.

### 3. A ubiquitous language

Names of packages, slices and types in code are English (as now); Russian terms stay in the interface, the documentation and the [glossary](../glossary.md), which both layers refer to.

### 4. Checking the boundaries

- Go: the import rules are `depguard` in `go/.golangci.yml` (section `linters.settings.depguard`), run in CI together with the rest of `golangci-lint`; test files are excluded from the rules. Violations that existed at the date of the decision were listed as exceptions and removed by sub-tasks; the rules admit no new ones. No exceptions are left: `metrics → store` and `events → hooks` were removed in #18, `store → auth` in #19, `llmassist → cli` in #23 (the shared API client moved to `platform/apiclient`), direct SQL from `schedule` in #21.
- Frontend (the result of a spike, #25): its own check, `frontend/scripts/check-fsd.mjs` (`bun run check:fsd`, part of `verify`). It parses `.ts` and the scripts of `.svelte` through the TypeScript compiler and checks: imports only downwards across layers, slices of one layer do not know each other (cross-imports of entities go through `@x`), a slice is entered only through `index.ts`. It is tested on fixtures (`tests/unit/check-fsd.test.mjs`) and now guards the real layers.
  - `steiger` (as in `grafana/oncall`) reads the scripts of `.svelte` but **does not see imports whose target is a `.svelte` file itself** (bypassing a public API and importing a higher component go unnoticed) and is noisy with its structural rules; it may be added later for the structural checks.
  - `dependency-cruiser` 18 does not extract dependencies from components with Svelte 5 and `lang="ts"` (0 imports), so it does not fit.

## Rollout order

No big bang; each step is a separate PR with green CI and an unchanged JSON shape (`Test*JSONShape`).

1. Boundary rules and the checking tool (Go and frontend), the glossary.
2. The backend by contexts, from the most isolated: `platform` → `identity` → `knowledge` → `automation` → `journal`; then `httpapi` shrinks to a composition root, big files are cut along the way.
3. The frontend: the layer skeleton and `shared` → `entities` → `features`/`widgets` → `pages`; before cutting `GraphPage`, `NotesPage` and `App`, component tests of the critical scenarios (#11).

The sub-tasks were created as sub-issues of #9.

## Outcome (2026-10-12)

The migration is done and the gates are in CI (`docs/architecture.md`, “Gates against regressions”). One thing differs from the target layout: a context's SQL store was **not** split into a separate `pg/` package. Each context keeps its domain types and its PostgreSQL store together in the root package (`journal`, `knowledge`, `automation`, `identity`) next to `http/`; the ports between contexts are the root packages' public methods, and rule 4 holds trivially. The shared parts that fit no single context are the cross-cutting slices `assistant`, `graphdraft` and `wsarchive`, the remainder `store` (cross-context reads and the backup log) and `schedule`.

## Consequences

**Pros.** A change to an entity touches one context and one slice; the boundaries are checked by a machine, not by review; `Store` stops being the point everything passes through; modules can be tested through narrow interfaces.

**Cons and risks.** A large mechanical move with a risk of regressions (insurance: tests, JSON shapes, a manual run in the browser); during the migration two layouts coexist in the code; the scheduler and the graph cross contexts, and interfaces have to be chosen carefully for them; FSD for Svelte needs its own boundary check.

## Alternatives considered

- **Split `httpapi` and `Store` into packages without a context model.** Cheap, but it leaves entities smeared and gives no dependency rules; after a while everything glues back together.
- **Full tactical DDD** (aggregates, repositories, domain events, factories). An expensive ceremony for a CRUD domain with few real invariants.
- **Separate services.** There is neither the load nor the teams that would justify it; the price is the network, transactions and deployment.
- **Leave it as it is.** Files and packages keep growing, and the question “where does rule X live” stays with review.
