# Notion 2026-03-11 Migration and File Upload Support

**Date:** 2026-07-18
**Status:** Proposed
**Scope:** Move the built-in Notion service to the current Notion API shape (`Notion-Version: 2026-03-11`) and add first-class File Upload support so local private files, especially invoice PDFs received as email attachments, can be uploaded to Notion-managed storage and attached to page `files` properties. AnyCLI is not published yet, so this design intentionally does **not** preserve the old `2022-06-28` database model or old command compatibility.

## 1. Background

### Current source state

The Notion service already has a split-version implementation:

- `internal/tools/notion/notion.go` defines `notionVersion = "2022-06-28"` and `markdownVersion = "2026-03-11"`.
- `call()` still routes through the old default version; individual newer commands call `callWithVersion(..., markdownVersion)`.
- `search`, `fetch`, `db query`, `data-source update`, `view create/update`, page markdown reads/writes, move, and duplicate already use the `2026-03-11` header on the relevant calls.
- `page patchPageProps` still calls `s.call(...)`, so page property/icon/cover updates still use `2022-06-28`.
- The root command exposes `fetch`, `search`, `page`, `db`, `data-source`, `view`, `comment`, `user`, and `task`; it has no `api` escape hatch and no `file` upload/attach group.
- `--file` is a global helper for reading markdown text into flags. It is not binary upload support.

That means the codebase is halfway through a current-API migration, but still keeps a compatibility seam that makes the invoice use case fail: the tool can write an external file URL into a property, but it cannot upload a local PDF and refer to it as a Notion `file_upload` object.

### Product problem

Invoice sync needs to write PDFs from email attachments into a Notion database row. Some invoices, such as ngrok invoices, exist only as private email attachments with no stable public URL. Notion `files` properties accept either:

- `external` file objects: public HTTPS URLs Notion can read later; or
- `file_upload` file objects: Notion-managed files uploaded through the File Upload API.

The current tool only supports the first pattern indirectly. For private local attachments, it must either leave the property empty or store an email link, which is not the same artifact and can break for other agents, humans, or future account contexts.

### Official Notion API changes that matter

#### 2025-09-03: database container vs. data source

Notion split the old database model into:

- `database`: the container; and
- `data_source`: the schema/queryable source inside the container.

Required consequences:

- Querying rows is `POST /v1/data_sources/{data_source_id}/query`, not `POST /v1/databases/{database_id}/query`.
- Creating a page under a table uses a `data_source_id` parent.
- Schema/property operations are data-source operations.
- Search filters are `page|data_source`, not `page|database`.
- A database container can return `data_sources[]`; callers use those IDs for row operations.

#### 2026-03-11: cleanup of older fields

The direct `2026-03-11` upgrade requires:

- `after` in append-block-children becomes `position: {type: after_block|start|end, ...}`.
- `archived` becomes `in_trash` in request and response handling.
- `transcription` block type becomes `meeting_notes`.

AnyCLI should use the new names everywhere. Since the tool is unpublished, there is no value in carrying old aliases in the implementation.

#### File Upload API

Notion now exposes File Upload APIs:

- `POST /v1/file_uploads` creates a `file_upload` object. Modes include `single_part`, `multi_part`, and `external_url`.
- `POST /v1/file_uploads/{file_upload_id}/send` sends file bytes as `multipart/form-data` with a required `file` part; multi-part uploads also send `part_number`.
- `POST /v1/file_uploads/{file_upload_id}/complete` finalizes a `multi_part` upload.
- `GET /v1/file_uploads/{file_upload_id}` retrieves lifecycle state.
- `GET /v1/file_uploads` lists uploads for the current bot connection.
- For small direct uploads, Notion documents files up to 20 MB as the single-part path.
- After status becomes `uploaded`, the ID is attached through a file object:

```json
{
  "type": "file_upload",
  "file_upload": { "id": "43833259-72ae-404e-8441-b6577f3159b4" }
}
```

Notion also supports `external_url` imports, but the invoice case is local/private bytes. The first milestone should support direct local upload; `external_url` can be included because it is cheap and shares the same object lifecycle, but it is not a substitute for local bytes.

## 2. Goals

1. Make the Notion service consistently current-version: all Notion requests use `Notion-Version: 2026-03-11`.
2. Remove the default `2022-06-28` compatibility path from the service.
3. Make `data_source` the only row/query/schema parent concept in the command surface.
4. Add a `notion api` escape hatch, similar in spirit to `gh api`, for raw Notion REST calls and protocol debugging.
5. Add `notion file` commands for the normal local upload and attach workflow.
6. Let page create/update properties express both external and uploaded files.
7. Provide an invoice-sync-ready flow: local email attachment path -> Notion file upload -> page `files` property.
8. Keep the built-in-service convention: non-interactive flags, JSON-safe output, explicit exit codes, no prompts.

## 3. Non-goals

- Preserving `2022-06-28` behavior.
- Supporting `database_id` as an implicit row parent.
- Implementing every Notion File Upload mode in the first slice if direct local upload is complete. Multi-part can land after single-part if the command shape reserves it cleanly.
- Creating public temporary URLs for private files. That is the workaround this design removes.

## 4. Decisions

### D1 - use one Notion version constant

Replace the split constants with one current version:

```go
const notionVersion = "2026-03-11"
```

Then either remove `callWithVersion` or keep it private for tests/future exceptional endpoints, but every production command should call through a path that sends `2026-03-11`. The important invariant is that there is no semantic split where page properties or user/comment calls accidentally run old headers.

Required source changes:

- `internal/tools/notion/notion.go`: remove `markdownVersion` as a conceptual second model; register the new `file` group.
- `internal/tools/notion/client.go`: make `call()` send `2026-03-11`; add a binary/multipart call path for file uploads.
- Any existing `callWithVersion(..., markdownVersion)` calls can become `call(...)` unless keeping the explicit form reads clearer during the migration.

### D2 - rename the command surface around data sources

Keep `db create` only for database container creation. Move row query into the `data-source` group:

```text
notion db create --parent ... --title ... --properties '{...}'
notion data-source query <data-source-id> [--filter ...] [--sorts ...] [--page-size ...] [--start-cursor ...] [--all]
notion data-source update <data-source-id> [--properties ...] [--name ...] [--in-trash]
```

`notion db query` should be removed, not retained as a compatibility alias. Any old test or doc that teaches `db query` should change to `data-source query`. If we want a short alias later, it must still accept a data source ID and should not reintroduce database-row ambiguity.

Fetch/search behavior:

```text
notion search --query ... [--type page|data_source]
notion fetch <id> [--type page|database|data_source]
```

`fetch <database-id>` remains valid because the database container is still a real object and is the discovery path for `data_sources[]`. Row operations must never accept the database container ID.

### D3 - replace old field names everywhere

Use only the new names in code, tests, docs, and errors:

| Old | New |
|---|---|
| `archived` | `in_trash` |
| append `after` | `position` object |
| `transcription` | `meeting_notes` |
| search type `database` | `data_source` |
| row parent `database_id` | `data_source_id` |

Concrete command additions:

- `data-source update --in-trash true|false` maps to the `in_trash` request field.
- `page update --in-trash true|false` if trashing/restoring pages is in this tool's scope; otherwise keep page trash out of scope but ensure response decoding never expects `archived`.
- `page move` and parent resolution should continue rejecting database containers as row parents and should phrase the message around `data_source_id` only.

### D4 - add a `notion api` escape hatch

Add a top-level raw request command:

```text
notion api <METHOD> <PATH> [--body JSON|--body-file path] [--header name:value]
notion api <METHOD> <PATH> --form-file field=path [--form field=value] [--header name:value]
```

Rules:

- `PATH` is a Notion API path relative to `/v1`, such as `/file_uploads` or `/file_uploads/{id}/send`. A full `https://api.notion.com/v1/...` URL may be accepted, but the command should normalize it to the same base.
- The command always injects `Authorization: Bearer <NOTION_TOKEN>` and `Notion-Version: 2026-03-11`; callers cannot override either with `--header`.
- JSON requests use `--body` or `--body-file` and set `Content-Type: application/json`.
- Multipart requests use `--form-file` / `--form` and set `multipart/form-data` with the generated boundary.
- Responses are emitted verbatim JSON/body with the same API error classification as the rest of the service.
- No interactive prompt, no token printing, no shelling out to curl.

Why this belongs in the design:

- Notion has a wide surface and keeps adding endpoints. Without `notion api`, every rare lifecycle endpoint becomes a first-class command just so agents can debug or unblock one flow.
- The File Upload protocol has low-level states (`pending`, `uploaded`, `expired`, `failed`) and modes (`single_part`, `multi_part`, `external_url`). A raw API command handles recovery, multipart experiments, and external-url import without polluting the normal CLI shape.
- AnyCLI is embedded; the host already owns credential injection. A raw API command gives the same safe credential path as resource commands while keeping tokens out of user-authored shell snippets.

Examples:

```bash
# Create a raw File Upload object
notion api POST /file_uploads --body '{"mode":"single_part","filename":"invoice.pdf","content_type":"application/pdf"}'

# Send local bytes to an existing File Upload object
notion api POST /file_uploads/<file-upload-id>/send --form-file file=./invoice.pdf

# Inspect a stuck upload
notion api GET /file_uploads/<file-upload-id>

# Query a data source before a dedicated wrapper exists
notion api POST /data_sources/<data-source-id>/query --body '{"page_size":10}'
```

### D5 - keep the file command group small

Add a new root resource group named around the user-facing noun, not the internal API object:

```text
notion file upload <path> [--name ...] [--content-type ...] [--json]
notion file attach <page-id> --property <name> (--upload-id <id>|--external-url <url>) [--name ...]
```

`file upload` is the normal command agents and humans should reach for. It creates a `single_part` File Upload, sends the local file, and emits the uploaded `file_upload` object or the ID. `file attach` writes that uploaded file into a page `files` property.

Do **not** add `file upload-create`, `file upload-send`, `file upload-complete`, `file upload-get`, or `file upload-list` as first-class commands in this slice. Those are protocol lifecycle operations, not user tasks. They should be handled through `notion api` until a repeated real workflow proves that one deserves a dedicated wrapper.

### D6 - implement direct local upload with multipart/form-data

Add a Notion-specific binary request helper rather than trying to force binary through JSON `call()`:

```go
func (s *Service) callMultipart(ctx context.Context, token, path string, fields map[string]string, fileField, fileName, contentType string, data []byte) ([]byte, error)
```

Implementation notes:

- Use `mime/multipart.Writer`; do not hand-build MIME boundaries for Notion unless tests require exact bytes.
- Let `multipart.Writer` set the `Content-Type` boundary.
- Include `Authorization` and `Notion-Version: 2026-03-11`.
- For the internal send step used by `file upload`, the form field name is `file`. Multi-part `part_number` is left to `notion api` until a dedicated large-file wrapper exists.
- Infer content type from `--content-type`, then filename extension, then `application/octet-stream`.
- Infer upload filename from `--name`, then `filepath.Base(path)`.
- `--file` is currently global markdown text input; do not use it for binary file upload. `notion file upload` takes the local file path positionally. If raw multipart is needed, `notion api --form-file file=path` is the escape hatch. This keeps global `--file` as markdown-only and avoids a command where `--file` means binary in one place and text everywhere else.

### D7 - attach uploaded files through page properties

The page property path should accept raw Notion property JSON, but agents need a compact scalar shape for the common file case.

Keep `--properties` as raw JSON. Add optional sugar only if it does not compete with raw JSON:

```text
notion page update <page-id> --properties '{
  "Invoice": {
    "files": [
      {"name":"ngrok-invoice.pdf","type":"file_upload","file_upload":{"id":"..."}}
    ]
  }
}'
```

If adding sugar, make it explicit:

```text
notion file attach <page-id> --property Invoice --upload-id <file-upload-id> [--name invoice.pdf]
notion file attach <page-id> --property Invoice --external-url https://... [--name invoice.pdf]
```

Recommended first implementation: add `file attach`. It gives invoice sync a safe single-property mutation without forcing every caller to assemble Notion property JSON correctly. Under the hood it sends:

```json
{
  "properties": {
    "Invoice": {
      "files": [
        {
          "name": "ngrok-invoice.pdf",
          "type": "file_upload",
          "file_upload": { "id": "..." }
        }
      ]
    }
  }
}
```

For replacement semantics, default to replacing the property value. Appending to an existing files property requires a read-modify-write and can be a later explicit `--append` flag; do not silently read/merge unless the command name says so.

### D8 - keep external files, but do not treat them as uploads

External URL file objects remain valid and should still be accepted in page properties, icons, and covers. But code and docs must keep the concepts separate:

- `external`: Notion links to a public HTTPS URL and does not own the bytes.
- `file_upload`: Notion owns uploaded bytes and the file can be reused across pages/blocks/properties.
- `external_url` File Upload mode: Notion imports a public URL into Notion-managed storage asynchronously. This is useful but not a solution for private email attachments.

The invoice-sync happy path must be local direct upload, not an external URL import.

## 5. Proposed command contract

### Direct upload

```bash
notion file upload ./invoice.pdf --name ngrok-invoice.pdf --content-type application/pdf --json
```

Returns the File Upload object. Without `--json`, stdout may emit only the uploaded ID for easy shell chaining:

```text
43833259-72ae-404e-8441-b6577f3159b4
```

### Attach to a files property

```bash
notion file attach <page-id> \
  --property Invoice \
  --upload-id 43833259-72ae-404e-8441-b6577f3159b4 \
  --name ngrok-invoice.pdf
```

### Full invoice flow

```bash
UPLOAD_ID=$(notion file upload ./ngrok-invoice.pdf --name ngrok-invoice.pdf)
notion file attach <invoice-row-page-id> --property Invoice --upload-id "$UPLOAD_ID" --name ngrok-invoice.pdf
```

For an invoice sync program using AnyCLI as a library, the same flow is two `Engine.Execute` calls. No public URL is created.

## 6. Implementation plan

### Slice 1 - version cleanup

- Change `notionVersion` to `2026-03-11`.
- Remove `markdownVersion` or alias it to `notionVersion` temporarily during the diff.
- Convert all production requests to `call()` unless an explicit version is needed for readability.
- Update `patchPageProps` to use the new version.
- Add tests asserting page property patch sends `2026-03-11`.

### Slice 2 - data source command cleanup

- Move `newDBQueryCmd` to `data-source query`.
- Remove `db query` from root registration.
- Keep `db create` as database-container creation with `initial_data_source.properties`.
- Add `data-source update --in-trash`.
- Update help text, tests, and docs so database IDs are discovery-only for row operations.

### Slice 3 - field rename audit

Run repository checks and fix all hits outside historical design docs:

```bash
git grep -n 'archived\|after\|transcription\|database_id\|db query\|2022-06-28' -- internal definitions docs README.md WHY_ANY_CLI.md
```

Expected acceptable leftovers:

- historical design docs may mention old behavior as context;
- tests may mention old terms only in migration assertions, if any.

Everything in active code, command help, and current docs should use the new terms.

### Slice 4 - raw API escape hatch

Add `internal/tools/notion/api.go` with `newAPICmd`.

Support:

- `notion api <METHOD> <PATH>`
- `--body` and `--body-file` for JSON/raw request bodies
- `--form field=value` and `--form-file field=path` for multipart form requests
- repeatable `--header name:value` for non-auth, non-version headers

Validation:

- reject attempts to override `Authorization` or `Notion-Version`
- reject mixing JSON body flags with multipart flags
- reject local file read errors before any request
- normalize `/v1` paths so callers do not accidentally hit a double `/v1/v1`

Tests should cover JSON, multipart, extra headers, auth/version override rejection, API errors, and file read failures.

### Slice 5 - file upload service

Add `internal/tools/notion/file.go` or `file_upload.go` with only the high-level wrappers:

- `newFileUploadCmd` for `file upload`
- `newFileAttachCmd` for `file attach`
- multipart helper(s) shared with `notion api` where sensible

Wire it in `newRoot`:

```go
file := newGroupCmd("file", "Manage Notion files and uploads")
file.AddCommand(...)
root.AddCommand(..., file)
```

### Slice 6 - file attach

Add `newFileAttachCmd` under `file`.

Flags:

- `--property` required.
- exactly one of `--upload-id` or `--external-url` required.
- `--name` optional; default basename for external URLs or empty for upload IDs if Notion accepts it. Prefer requiring `--name` for `--upload-id` if live/API tests show files property display names need it.
- optional `--json` already global.

### Slice 7 - invoice sync caller update

Where the invoice sync currently tries to write a URL or leaves the property empty:

1. Save the email attachment PDF to a local temp file.
2. Call `notion file upload <path> --name <invoice filename> --content-type application/pdf --json`.
3. Extract `id` from the returned File Upload object.
4. Call `notion file attach <row-page-id> --property <files property> --upload-id <id> --name <invoice filename>`.
5. Delete the local temp file.

Do not upload the file to a public temporary host.

## 7. Tests

### Version and data model tests

- Every Notion command test should assert `Notion-Version: 2026-03-11`.
- `page update` properties-only path sends `2026-03-11`.
- `search --type data_source` succeeds; `--type database` is a usage error.
- `data-source query <id>` sends `POST /data_sources/{id}/query`.
- `db query` is an unknown command.
- `db create --properties` wraps into `initial_data_source.properties`.
- `data-source update --in-trash true` sends `{"in_trash":true}`.

### Raw API tests

Use the existing `httptest` harness pattern.

- `api GET /users/me` sends the injected auth and `Notion-Version: 2026-03-11`.
- `api POST /file_uploads --body ...` sends JSON with `Content-Type: application/json`.
- `api POST /file_uploads/{id}/send --form-file file=./invoice.pdf` sends `multipart/form-data` with the file part.
- `api --header accept:application/json` forwards safe custom headers.
- Attempts to override `Authorization` or `Notion-Version` are usage exit 2.
- Mixing `--body` and `--form-file` is usage exit 2.
- Missing body files or form files are usage exit 2 before any request.
- Non-2xx responses use the shared Notion API error renderer.

### File upload tests

- `file upload <path>` performs create then send, emits the upload id in default mode, emits structured JSON under `--json`, and stops after create if send fails.
- Missing path, unreadable file, bad MIME, and files larger than the supported single-part limit return usage exit 2 before any request.

### File attach tests

- `file attach --upload-id` sends a files property containing `type: file_upload`.
- `file attach --external-url` sends a files property containing `type: external`.
- Supplying both or neither of `--upload-id` / `--external-url` is usage exit 2.
- A property patch failure exits 1 and does not pretend the upload failed; the upload and attach phases have separate errors.

### Integration smoke

With a real Notion test workspace:

1. Create or fetch a test database and data source.
2. Create a row page.
3. Upload a small local PDF.
4. Attach it to the row's files property.
5. Fetch the row and confirm the files property contains a `file_upload` object.
6. Download/open the file from Notion UI manually once to validate it is not an external URL placeholder.

## 8. Failure modes and guardrails

| Failure | Behavior |
|---|---|
| File path missing or unreadable | usage exit 2; no Notion request |
| Create succeeds, send fails | emit/stderr mentions the created upload ID so the caller can inspect or retry; exit 1 |
| Upload is pending/expired/failed when attaching | Notion returns API error; surface status/message verbatim |
| Caller passes database ID where data source ID is required | fail fast with guidance to `fetch <database-id>` and use `data_sources[]` |
| File exceeds single-part limit | usage error in `file upload` if size > 20 MB until a high-level multi-part wrapper exists; raw multi-part experiments can use `notion api` |
| External URL is private/non-SSL/missing content headers | `external_url` import returns Notion validation/failed status; do not retry as direct upload unless caller has local bytes |
| Existing files property has values | `file attach` replaces by default; later `--append` must do explicit read-modify-write |

## 9. Open questions

1. Should `file upload` default stdout be the ID only, or the full JSON object? ID-only is easier for agents to chain; full JSON is more consistent with provider passthrough. Recommendation: ID-only default, full body with `--json`, matching page create's ID-list default.
2. Should `file attach` support `--append` in the first implementation? Recommendation: no; replacement is deterministic and simple. Add append after a row-property read path is reliable.
3. Should a dedicated high-level multi-part command exist now? Recommendation: no. Route it through `notion api` first; add a wrapper only after real repeated use.
4. Should `db query` be removed or retained as an alias? Recommendation: remove it because AnyCLI is unpublished and keeping it preserves the wrong concept.

## 10. References

- Current source: `internal/tools/notion/notion.go`, `client.go`, `db.go`, `fetch_search.go`, `page.go`, `page_move.go`, `view.go`.
- Existing design: `docs/design/004-notion-read-page-body.md`.
- Notion: [Upgrading to 2025-09-03](https://developers.notion.com/docs/upgrade-guide-2025-09-03).
- Notion: [Upgrading to 2026-03-11](https://developers.notion.com/docs/upgrade-guide-2026-03-11).
- Notion: [Uploading small files](https://developers.notion.com/guides/data-apis/uploading-small-files).
- Notion: [File Upload object](https://developers.notion.com/reference/file-upload).
- Notion: [Create a file upload](https://developers.notion.com/reference/create-a-file-upload), [Send a file upload](https://developers.notion.com/reference/send-a-file-upload), [Complete a file upload](https://developers.notion.com/reference/complete-a-file-upload), [Retrieve a file upload](https://developers.notion.com/reference/retrieve-a-file-upload), [List file uploads](https://developers.notion.com/reference/list-file-uploads).
- Notion: [File object](https://developers.notion.com/reference/file-object).
