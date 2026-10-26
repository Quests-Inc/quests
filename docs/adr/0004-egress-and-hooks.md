# ADR-0004. Outgoing HTTP of scripts and hooks 2.0

- Status: **accepted**, implemented in 0.8 (quest=485)
- Date: 2026-10-26
- Related: ADR-0003 (script graphs), ADR-0002 (execution context), `docs/security.md`, `docs/automation.md`, note=310 and note=311 (the analysis and the design discussion)

## Context

1. The server runs no user code (ADR-0002/0003), so a script that wants data from a public API needed a runner with `curl` and
   `jq`. The only server node that went out was `sink.http` (hook webhooks). Its guard (`rejectPrivateHost`) resolved the name once
   and then let `http.DefaultClient` resolve it again and follow redirects unchecked: DNS rebinding and a redirect to an internal
   address got through; there were no limits on port, size or number.
2. Hooks fired on ten quest events and were tied to one quest or to the whole workspace. A note, a step, a variable or a file
   could not wake a script, and a hook that writes a variable of the thing it watches had no protection from itself.

The threat model is not "the owner of the workspace is the administrator". The workspace owner may be someone else's account on
the administrator's server, and the server sits in the administrator's local network.

## Decision

### 1. One place that goes out: `platform/egress`

Everything a script sends from the server (`sink.http`, `http.get`) goes through one client built from the **administrator's
policy**: environment variables of the server (`QUESTS_EGRESS_*`), never the database.

| Variable | Meaning | Default |
|---|---|---|
| `QUESTS_EGRESS_MODE` | `off`, `public` (any public address), `allowlist` (only `QUESTS_EGRESS_ALLOW`) | `off` |
| `QUESTS_EGRESS_ALLOW` | `api.example.org`, `*.example.org` (a subdomain, not the bare domain) | empty |
| `QUESTS_EGRESS_ALLOW_NETS` | explicit exceptions to the ban on internal addresses (CIDR or address) | empty |
| `QUESTS_EGRESS_PORTS` | allowed ports | `80,443` |
| `QUESTS_EGRESS_MAX_BODY` / `_TIMEOUT` | answer size after unpacking (bytes) / seconds | 1 MiB / 10 |
| `QUESTS_EGRESS_RATE` / `_CONCURRENCY` | requests per hour per workspace / at once | 120 / 4 |
| `QUESTS_EGRESS_PROXY` | the administrator's one proxy (a machine that reaches the outside only through one); needs `allowlist`, because a proxy resolves names where the server cannot look | none |

On top of the mode, a workspace needs a **grant** (`egress_grant`, written only by the admin CLI as the database owner; the API
role can read it and cannot write it). Without it a graph with a node that goes out cannot be saved there and does not run.

How the address is judged: at the **connection** (`net.Dialer.Control`, after the name was resolved, for every address the dial
tries), so a name that changes its answer and a redirect meet the same check. Refused: loopback, RFC 1918, CGNAT, link-local
(including the metadata address), `0/8`, multicast, `240/4`, IPv6 `::1`, `fc00::/7`, `fe80::/10`, Teredo, documentation ranges;
an IPv4 address wrapped in IPv6 (mapped, NAT64, 6to4) is judged as the IPv4 it carries. Before any connection: only `http(s)`,
no user info, a normalised host (lower case, no trailing dot, plain ASCII, no name that ends in a number: `2130706433`,
`0x7f.1`), the port in the list, the host allowed by the mode. At most three redirects, each judged again. The answer is cut at
the limit **after** decompression (a small gzip bomb stops), cookies and the environment's proxy are not used, one connection per
request. Every attempt, refused ones included, goes to `egress_log` (30 days) and to `quests-server egress log`.

Admin CLI: `quests-server egress status | enable <ws> | disable <ws> | log [<ws> [n]]`.

### 2. Nodes

- `http.get` ("HTTP: запрос"): a GET of `https://<host><path>?<query>`. The **host is a parameter the script's owner fixes**; what
  arrives on the `path` and `query` inputs fills only the path (one escaped piece, `{path}` in the path or appended) and the query
  values, so text from outside cannot name another host. Optional `auth {header, secret, prefix}` puts a secret of the source's
  entity into one header (not `Host`, `Cookie`…); the secret reaches no output or log. Outputs `body` (JSON), `text`, `status`,
  and `failed {kind: denied | timeout | status | too_large | bad_content | error, message}`: a failure is data. It is a server
  node, so a graph with it needs no runner; embedded scripts (`script=N{…}`) may use it (without `auth`: they have no
  `secrets`).
- `json.get` ("JSON: достать"): one value out of a document by a dotted path (`current.temperature_2m`, `items.0.name`) as a
  type; a missing path or another type leaves `value` without a token and says why on `bad`.
- A node that goes out is marked `network` in the catalog; saving a graph (and a check on it) with such a node is refused where the
  mode or the grant is off. The evaluator never does I/O itself: a run is given a `Fetcher` by the host, and none means none.

### 3. Hooks 2.0

- **Area.** `scope_kind` is `workspace`, `quest`, `step`, `note` or `note_tree` (a note and everything under it), with
  `scope_id`. `quest_id` stays one release in step with a quest area and in the API.
- **Events** have one catalog (`platform/hooks/catalog.go`): quest, step, note (`note_created/updated/deleted`), variable
  (`values_changed`: owner, name, type, and the value for a number or a yes/no, never for a text), file
  (`attachment_added`, `attachment_revision`). The interface, the MCP `hook` tool and the ports of `source.event` read it.
- **`source.event`** gives a hook script the event with typed ports by `family` (`quest`, `step`, `note`, `value` with an
  `owner`, `attachment`, `any`), including a `ref` and `secrets` of the entity. A graph that reads only the event runs as the
  workspace.
- **Limits.** Per hook: `cooldown_sec` (a pause per event kind and entity; the first event runs at once, the later ones wait and
  the newest runs when the pause is over; at least 30 s for `note_updated`, 5 s for `step_progress`, 2 s for
  `values_changed`), `max_per_hour` (over it the hook is **switched off** with `disabled_reason`), `include_private` (events of
  private notes are skipped unless asked for).
- **Loops.** A run descends from a chain of hooks (`origin`, "hook:3>hook:7"); what it writes (variables, through `sink.value`)
  carries the chain in the event, and a hook is not woken by an event its own chain made, nor by a chain of three. The chain is
  kept in the run's key, so it survives a Code node's wait. Other sinks do not carry it yet: the pause and the hourly cap cover
  them.

## Not done / later

- A query for a list of entities (`Запрос: квесты`), list types, a timer node, `POST` in `http.get`.
- Hook areas by questline or tag; events for runners going offline, failed checks, step failures.
- The origin chain for sinks other than `sink.value`.
