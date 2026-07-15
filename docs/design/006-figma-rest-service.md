# Figma REST Service

**Date:** 2026-07-15
**Status:** Accepted
**Scope:** PAT-authenticated Figma access for Helio, including the complete public REST surface and bounded MCP-like read ergonomics.

## Context

Helio stores a user-created Figma personal access token (PAT) after verifying it with `GET /v1/me`. The token gateway projects that credential as `access_token`. AnyCLI consumes the projection without implementing OAuth, persistence, refresh, or a second credential store.

Figma has no general agent-oriented native CLI. The `figma` tool is therefore a built-in service over Figma's public REST API. This is deliberately not an MCP transport.

## Authentication and credential lifecycle

`definitions/tools/figma.json` binds `access_token` to the process-local `FIGMA_ACCESS_TOKEN` environment value. API requests send it only in the official `X-Figma-Token` header. The PAT is never placed in a URL, command argument, response, downloaded-asset request, or persistent file.

Credential invalidation is provider-aware. AnyCLI marks the cached PAT stale only for HTTP 401 or an explicit invalid/expired-PAT response. It does not invalidate a valid PAT for:

- malformed local flags or JSON;
- missing file, team, project, seat, or plan permission;
- a scope or Enterprise-entitlement failure;
- rate limiting, transport failure, or a Figma 5xx.

This distinction matters because Figma commonly uses HTTP 403 for both token rejection and valid-token authorization failures.

## Operation catalog

`internal/tools/figma/operations.json` is generated from the official Figma OpenAPI repository at the pinned revision recorded in the file. The current snapshot contains 50 operations:

- 47 accept a personal access token;
- 3 require a plan access token or organization OAuth (`getActivityLogs`, `getDeveloperLogs`, and `getAiUsageDaily`).

The catalog records each operation's ID, method, versioned path, path/query parameters, required query parameters, OAuth-equivalent scopes, body requirement, and PAT compatibility. All named commands route through this catalog, so paths do not have a second handwritten source of truth.

Refresh the snapshot after reviewing an upstream spec change:

```text
ruby scripts/update-figma-operations.rb /path/to/figma-rest-api-spec/openapi/openapi.yaml figma/rest-api-spec@REVISION
go test ./internal/tools/figma/...
```

The catalog tests require unique operation IDs, matching path placeholders, valid required-query subsets, exactly 50 snapshot operations, exactly 47 PAT operations, and a named command for every PAT operation. Required query parameters are enforced by both named commands and `api call` before any request.

## Command surface

All commands are non-interactive and emit JSON.

### Discovery and generic access

```text
figma capabilities
figma api list
figma api describe OPERATION_ID
figma api call OPERATION_ID --param key=value [--body-json JSON | --body-file PATH]
figma api --method METHOD --path /v1/... [--query key=value] [--body-json JSON | --body-file PATH]
```

`api call` is the stable, catalog-validated form. The raw relative-path form is a forward-compatible escape hatch for a newly published Figma endpoint. It accepts only `/v1/` or `/v2/` paths on the fixed Figma API origin; arbitrary URLs, path traversal, inline query strings, and custom auth headers are rejected.

### Identity, projects, files, and images

```text
figma me
figma teams projects --team-id ID
figma projects meta --project-id ID
figma projects files --project-id ID [--branch-data]
figma files meta --file-key KEY
figma files get --file-key KEY [file read options]
figma files nodes --file-key KEY --ids IDS [file read options]
figma files versions --file-key KEY [pagination options]
figma images render --file-key KEY --ids IDS [all official render options]
figma images fills --file-key KEY
```

Team IDs remain explicit input because Figma does not expose a PAT endpoint that discovers a user's team IDs.

### Comments and reactions

```text
figma comments list --file-key KEY [--as-md]
figma comments post --file-key KEY --message TEXT [--comment-id ID] [--client-meta-json JSON]
figma comments delete --file-key KEY --comment-id ID
figma comments reactions list --file-key KEY --comment-id ID [--cursor CURSOR]
figma comments reactions add --file-key KEY --comment-id ID --emoji EMOJI
figma comments reactions delete --file-key KEY --comment-id ID --emoji EMOJI
```

### Libraries and variables

```text
figma libraries components {team|file|get} ...
figma libraries component-sets {team|file|get} ...
figma libraries styles {team|file|get} ...
figma variables local --file-key KEY
figma variables published --file-key KEY
figma variables update --file-key KEY --body-json JSON
```

Variable endpoints require an eligible Enterprise membership in addition to the PAT scopes. Library enumeration requires a known team or file; PAT REST cannot reproduce MCP's catalog of every subscribed or available community library.

### Dev resources, webhooks, analytics, payments, and oEmbed

```text
figma dev-resources {list|create|update|delete} ...
figma webhooks {list|create|get|update|delete|requests|team} ...
figma analytics {component-actions|component-usages|style-actions|style-usages|variable-actions|variable-usages} ...
figma payments list ...
figma oembed get --url URL ...
```

Complex, evolving write schemas (variables, dev resources, and webhooks) use syntax-validated `--body-json` or a bounded `--body-file` rather than duplicating the full upstream schema as flags. `api describe` reports parameter names, required query/body presence, and scopes; Figma remains the authority for parameter enums, conditional constraints, and request-body schemas.

## Agent-oriented context

Figma URLs from Design, legacy File, Prototype, and FigJam are accepted. Slides and Make URLs fail locally because the pinned PAT REST file contract does not expose them. A URL `node-id` is normalized to the REST node ID form, and `--ids` can override it.

```text
figma context metadata --url FIGMA_URL [--depth N] [--max-nodes N]
figma context design --url NODE_URL [--include-geometry] [--include-variables]
figma context figjam --url NODE_URL [--include-geometry] [--include-variables]
figma context screenshot --url NODE_URL [--format png] [--scale 1]
figma context variables --url FIGMA_URL
```

`context metadata` converts file/node JSON into a deterministic sparse tree and enforces a maximum output node count. `context design` fetches independent node, render, and optional Design-variable data concurrently; `context figjam` fetches nodes and renders but rejects Variables options that the upstream endpoint does not support. They return the exact REST payloads in one envelope and do not claim to reproduce Figma-hosted code generation or proprietary MCP context.

## Asset downloads

```text
figma assets download --url NODE_URL --output-dir DIR [--format png] [--scale 1] [--overwrite]
figma assets download-fills --url FIGMA_URL --output-dir DIR [--overwrite]
```

Downloads are bounded to 100 MiB per asset, accept only HTTPS asset URLs, use deterministic filesystem-safe names, run with bounded concurrency, refuse overwrite by default, and install through a temporary file. Signed asset requests never receive `X-Figma-Token`. Output is a JSON manifest containing IDs, filenames, byte counts, and content types; signed URLs are not repeated in the manifest.

## MCP parity boundary

An all-scope PAT grants every REST capability that the user's Figma plan, seat, and resource permissions allow. It does not turn private MCP or Plugin APIs into REST endpoints.

The service provides useful REST-backed outcomes for metadata, screenshots/renders, asset downloads, variables, known libraries, FigJam reads, and design context. It cannot provide literal parity for capabilities with no public PAT REST endpoint:

- general native canvas create/edit/delete (`use_figma`);
- creating files, generating designs, or generating FigJam diagrams;
- uploading assets into a Figma canvas;
- shader fill/effect discovery and import;
- Figma-hosted Code Connect mapping/suggestion tools;
- Figma Make resources and proprietary hosted context generation;
- global subscribed/available-library discovery;
- the MCP `whoami` plan-and-seat expansion.

General native-canvas authoring requires a Helio companion Figma plugin/bridge running inside Figma, or access to Figma's hosted MCP/partner surface. The official Code Connect CLI is a separate PAT-capable option for repository-to-component publishing, but it is not a canvas mutation API and is not silently substituted here.

`figma capabilities` exposes this boundary as JSON so an agent can reason about it without relying on prose.

## Error and output contract

- Only HTTP 2xx is success.
- Both documented Figma error envelopes (`err` and `message`) are understood.
- HTTP status and `Retry-After` are preserved without echoing response credentials.
- API redirects may retain the PAT only on the original origin; cross-origin redirects fail before a second request.
- Provider error text is PAT-redacted before it reaches stderr or credential classification.
- Parsed context/asset responses have a strict memory bound. Direct JSON passthrough is validated via a private temporary spool and bounded independently, so large files do not require an equivalent heap allocation.
- Local path, parameter, JSON, depth, scale, and format validation happens before a request.
- Empty successful responses become `{}`; non-JSON success bodies are rejected.
- Error bodies and request body files are bounded; large successful file documents remain available.

## Non-goals

- OAuth, plan-token, SCIM, or MCP authentication.
- A local Figma credential store.
- A Helio-side team/project/file allowlist that pretends to narrow PAT authority.
- Silent entitlement fallback when an Enterprise-only endpoint is unavailable.
- Claiming native-canvas write access that the public PAT REST API does not expose.
