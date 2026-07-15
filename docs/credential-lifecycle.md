# Credential Lifecycle

How the embeddable AnyCLI engine resolves, caches, and injects credentials.

## Ownership boundary

AnyCLI does not authenticate users, call a vault directly, read local credential files, or persist credentials. Its host supplies a `CredentialResolver` and may supply a `Cache`.

```text
host resolver -> Credential{Data, CacheUntil}
                    |
                    v
          AnyCLI cache by (tool, account)
                    |
                    v
       definition bindings -> env / arg / temp file
                    |
                    v
          external CLI or built-in service
```

The resolver owns where credentials come from. AnyCLI owns which declared fields are extracted, how they are injected, and when the cached projection becomes stale.

## Resolution

For each `ExecuteWith` call:

1. Load the embedded definition for the requested tool.
2. Derive `CacheKey(tool, account)`.
3. Use a fresh cache entry only when it contains every field required by the definition.
4. Otherwise call `CredentialResolver.Resolve(ctx, tool, account)`.
5. Extract only scalar fields named by the definition's `source.field` bindings.
6. Cache that projection until the resolver-supplied `CacheUntil`. A zero time disables reuse.

An empty account string selects the host's default account. Account semantics and ambiguity handling belong to the host.

## Injection

- `env`: adds the declared environment variable for the child CLI or built-in service invocation.
- `arg`: appends the declared flag and credential value to the tool arguments.
- `file`: writes an ephemeral file, redirects the tool through `config_env` or `config_flag`, and removes the file after execution.

Resolver-supplied credentials are always managed and ephemeral. AnyCLI never modifies a user's persistent config file.

## Failure and staleness

Built-in services report an explicit execution outcome. AnyCLI marks only that
`(tool, account)` cache entry stale when the provider explicitly rejects the
resolved credential. Local flag/JSON errors, scope or resource permission
failures, rate limits, transport failures, and provider 5xx responses do not
invalidate a credential that may still be valid.

CLI-backed tools cannot expose the same provider-aware signal, so a non-zero
child-process exit remains the conservative stale trigger for those tools. A
later call re-resolves the marked entry. The cache implementation stores state;
the engine owns freshness decisions.

The default cache is in-memory and process-local. A host that creates a fresh engine per command receives no cross-command reuse; a long-lived host may provide a longer-lived cache.

## Security properties

- Definitions are embedded in AnyCLI and cannot be supplied by a caller.
- Only fields referenced by credential bindings enter the cache.
- Credentials remain in memory except for explicitly declared ephemeral file injection.
- Tool manifests expose field names, never credential values.
- Missing or invalid execution contracts fail explicitly; there is no unauthenticated fallback mode.
