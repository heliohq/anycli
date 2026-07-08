# Reading a Notion Page's Body (page read + block children)

**Date:** 2026-07-07
**Status:** Proposed
**Scope:** Close the Notion tool's one day-one capability gap — an AI agent can read a page's metadata and append to it, but cannot read the page's existing **body**. Add two read paths, markdown-first: `page read <id>` (`GET /pages/{id}/markdown`, the token-efficient primary) and `block children <id>` (`GET /blocks/{id}/children`, the full-fidelity fallback). No other Notion surface changes.

## 1. Background

### Current state

The `notion` built-in service (`internal/tools/notion/notion.go`) exposes five subcommands: `page create` (`POST /pages`), `page get` (`GET /pages/{id}`), `page append` (`PATCH /blocks/{id}/children`), `search` (`POST /search`), `db query` (`POST /databases/{id}/query`). Every command makes one HTTP call and emits the provider's JSON verbatim. The service pins `Notion-Version: 2022-06-28` globally (`notion.go:23`).

### Problem

`page get` calls `GET /pages/{id}`, which by the Notion spec returns only the page object — title, timestamps, parent, `has_children`, properties. **The body is not in it.** A page's body lives in its child block subtree and is never fetched by any existing command. So an agent handed a normal Notion page link can confirm metadata and append to the page, but reads back an empty body — a silent, successful-looking `2xx` with no content. Databases read fine only because `db query` returns each row's values inline in `properties`; a normal page's prose lives in blocks, which no command retrieves.

This is a capability gap, present from day one — not a regression, permission problem, or gated code path.

### Reading a page body: two paths

- **Markdown** — `GET /v1/pages/{id}/markdown` returns the whole page rendered as enhanced Markdown in one response. Notion's own hosted MCP server chose this for agents because it is *"significantly more token-efficient than block JSON."* Requires `Notion-Version: 2026-03-11`. GA; available to internal integration tokens (our `NOTION_TOKEN`) with the `read_content` capability. Truncates only past ~20,000 blocks, surfacing `truncated: true` + up to 100 `unknown_block_ids` to re-fetch.
- **Block JSON** — `GET /v1/blocks/{id}/children` returns the first level of child blocks as a paginated array (`has_more` / `next_cursor`, ≤100 per page). Full fidelity, works on the current `2022-06-28` pin, but token-heavy and requires the caller to paginate and recurse into nested blocks. This is what most third-party Notion CLIs (4ier, litencatt, Coastal) expose.

### Goals

1. Add `page read <id>` (markdown) as the primary body read.
2. Add `block children <id>` (block JSON) as the full-fidelity fallback.
3. Let a single command use a Notion-Version other than the global pin, without disturbing the existing five commands.
4. Keep every command a thin verbatim passthrough, per the established service convention.

### Non-goals

Deferred to later specs: a generic `api <method> <path>` escape hatch; multi-block / mixed-type `page append`; `db query` sorts + cursor; markdown **write** (`PATCH /pages/{id}/markdown`); individual block retrieve/update/delete; the `2025-09-03` data-sources model; comments; users.

## 2. Decisions

### D1 — two read commands; `page read` under `page`, `block children` under a new `block` group

```
notion page get   <id>   # metadata (unchanged)
notion page read  <id>   # GET /pages/{id}/markdown        (Notion-Version 2026-03-11)
notion block children <id>   # GET /blocks/{id}/children   (Notion-Version 2022-06-28)
```

`page read` sits beside `page get` — `get` returns metadata, `read` returns body. `block children` goes under a new `block` command group, matching the near-universal third-party naming and reserving a clean `block` namespace for any future block get/update/delete. Wiring in `newRoot` (`notion.go:71`): add `s.newPageReadCmd(token)` to the `page` group; add `block := &cobra.Command{Use: "block", Short: "Blocks"}` with `s.newBlockChildrenCmd(token)`, and register it via `root.AddCommand(page, s.newSearchCmd(token), db, block)`.

**Flags**

| Command | Flag | Default | Effect |
|---|---|---|---|
| `page read` | `--include-transcript` | `false` | when set, append `?include_transcript=true` (meeting-note pages return full transcripts) |
| `block children` | `--page-size` | `100` | `?page_size=N` (Notion caps at 100) |
| `block children` | `--start-cursor` | `""` | when non-empty, append `?start_cursor=…` for the next page |

Query strings are built with `url.Values` and appended to the path, matching how the service already assembles paths inline.

### D2 — per-command Notion-Version via `callWithVersion`; `call` delegates

The core of `call` is extracted into `callWithVersion(ctx, token, method, path, payload, version string)`, which sets `req.Header.Set("Notion-Version", version)`. `call` becomes a thin wrapper preserving today's behavior:

```go
const markdownVersion = "2026-03-11" // page read; the markdown endpoints require it

func (s *Service) call(ctx context.Context, token, method, path string, payload any) ([]byte, error) {
    return s.callWithVersion(ctx, token, method, path, payload, notionVersion)
}
```

The five existing commands keep calling `call` — **zero change, still `2022-06-28`**. Only `page read` calls `callWithVersion(..., markdownVersion)`. This is deliberate: bumping the *global* version to `2026-03-11` would break `db query` and `page get` for databases (under `2025-09-03`+ the `POST /databases/{id}/query` path is gone and `GET /databases/{id}` changes shape). Notion's own upgrade guide prescribes per-request versioning for exactly this gradual-migration case.

### D3 — output stays a verbatim passthrough

Both new commands `emit(body)` the provider JSON unchanged, like every other command.

- `page read` emits the whole `{object:"page_markdown", id, markdown, truncated, unknown_block_ids}` object. It does **not** extract just the `markdown` string: `truncated` / `unknown_block_ids` are the agent's signal to re-fetch subtrees, and hiding them both misleads the agent and breaks the emit-verbatim convention.
- `block children` emits the whole list response; `has_more` / `next_cursor` stay in the payload so the agent paginates by re-invoking with `--start-cursor` — consistent with how the service already leaves pagination to the caller (`db query`, `search`).

### D4 — shared 403/404 access hint

In the non-2xx branch of `callWithVersion`, a `403` or `404` appends one actionable clause to the surfaced error, mirroring `google.go`'s `scopeHint` idiom:

```
notion API error (HTTP 404): object_not_found: ... (check the ID and that the integration has been granted access to this resource)
```

This lives in the shared call path, so it benefits every command, and most helps the read commands — where a bare `object_not_found` / `restricted_resource` would otherwise leave the agent unsure whether the page is missing or merely unshared. It is additive: existing error tests match on the provider's error code with `strings.Contains`, which the appended clause does not disturb.

### D5 — `page get` nudges to `page read` when the body exists

The original failure mode (§1) was not just the missing read command but the *silent* one: `page get` returns a 2xx metadata-only page object, so an agent concludes the page is empty. Adding `page read` alone leaves that trap armed for any agent that reaches for `page get` first. So `page get` now:

- keeps stdout a verbatim passthrough (unchanged JSON, unchanged exit 0), and
- when the response carries `has_children: true`, prints one line to **stderr**: `note: this page has content blocks not included here; use "notion page read <page-id>" to read the body`.

Its `Short` help also states it fetches metadata + properties and points to `page read` for the body. This is the narrowest amendment to "the existing commands are unchanged" that actually disarms the silent failure; the passthrough convention (D3) holds because the hint rides stderr, never stdout.

## 3. Test plan (tests first)

New cases in `internal/tools/notion/notion_test.go`, reusing the existing `newServer` / `run` httptest template:

- `TestPageRead_Happy` — asserts `GET /pages/{id}/markdown`, **`Notion-Version: 2026-03-11`** (proves per-command override), `--include-transcript` → `include_transcript=true` in the query, and the `page_markdown` body echoed to stdout verbatim.
- `TestPageRead_APIError` — `404` → exit 1, stderr carries `object_not_found` **and** the access-hint clause.
- `TestBlockChildren_Happy` — asserts `GET /blocks/{id}/children`, `Notion-Version: 2022-06-28`, `--page-size` / `--start-cursor` → query, body echoed verbatim.
- `TestBlockChildren_APIError` — `403` → exit 1, stderr carries `restricted_resource` and the hint.
- Version regression — assert an existing command (`db query` or `page get`) still sends `2022-06-28` while `page read` sends `2026-03-11`, guarding the override from leaking.
- `TestPageGet_HasChildrenNudge` — `has_children: true` → exit 0, stdout verbatim, stderr carries the `page read` nudge; `TestPageGet_Happy` asserts stderr stays empty without children (D5).
- Verbatim passthrough is asserted with exact stdout equality (`response + "\n"`) on both new commands — `truncated` / `unknown_block_ids` / `has_more` / `next_cursor` must survive; substring checks would let a reshaped body through.
- Hint symmetry — the access hint is asserted present on a 403 (`page append`) and a 404, and absent on a 401 (`search`) and a 400 (`page create`).
- `TestUnknownSubcommand_Fails` — an unknown subcommand under a group exits 1 with an `unknown command` error and sends no request.

`assertAuth` currently hardcodes `Notion-Version == "2022-06-28"`; it is split so the expected version is a parameter (or `page read` gets its own assertion), keeping existing callers green.

## 4. What changes / what does not

### Changes

- `internal/tools/notion/notion.go` — `markdownVersion` const; `callWithVersion` (extracted) + `call` wrapper; 403/404 hint in the non-2xx branch; `newPageReadCmd`; `newBlockChildrenCmd`; a `block` group and `page read` wired into `newRoot`; the `page get` stderr nudge + help text (D5); runnable `NoArgs` command groups so an unknown subcommand exits 1 instead of printing help with exit 0 (cobra skips `Args` validation on non-runnable commands — the other services' groups share this quirk and are a follow-up).
- `internal/tools/notion/notion_test.go` — the cases above; `assertAuth` parameterized on version.

### Unchanged

- The five existing commands' requests, stdout, and `2022-06-28` behavior (`page get` additionally gains the stderr nudge of D5).
- The service template (injectable base URL / `HC`, cobra-per-`Execute`, env-only credentials, verbatim stdout) from design 003.
- The definition (`definitions/tools/notion.json`), credential binding, engine, and middleware — no new binding or capability is needed; the same `NOTION_TOKEN` carries `read_content`.

## References

- `docs/design/003-multi-account-and-helio-toolset.md` — the Notion tool and the built-in service conventions.
- Notion API: [Retrieve a page as markdown](https://developers.notion.com/reference/retrieve-page-markdown) · [Working with markdown content](https://developers.notion.com/guides/data-apis/working-with-markdown-content) · [Retrieve block children](https://developers.notion.com/reference/get-block-children) · [Upgrade guide 2025-09-03](https://developers.notion.com/docs/upgrade-guide-2025-09-03).
