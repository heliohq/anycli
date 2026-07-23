#!/usr/bin/env bash
# Rotate HELIO_E2E_API_KEY (design 008 D1): exchange the current e2e
# assistant key for a fresh 48h key via the runtime self-renewal endpoint,
# verify the new key actually works, then write it back to the repository
# secret. Fails loudly on every step — a silent rotation failure means a
# dead key within 24h.
set -euo pipefail

: "${HELIO_E2E_API_KEY:?HELIO_E2E_API_KEY is required}"
: "${HELIO_E2E_API_BASE:?HELIO_E2E_API_BASE is required}"
: "${GH_TOKEN:?GH_TOKEN (PAT with secrets:write) is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

base="${HELIO_E2E_API_BASE%/}"

# 1. Who am I? The key's subject is the e2e assistant's AI user.
me=$(curl -fsS -H "Authorization: Bearer ${HELIO_E2E_API_KEY}" "${base}/user/me")
ai_user_id=$(jq -re '.data.id' <<<"$me")
echo "rotating key for AI user ${ai_user_id}"

# 2. Self-renew: mints a fresh 48h key; the old key expires naturally.
refreshed=$(curl -fsS -X POST -H "Authorization: Bearer ${HELIO_E2E_API_KEY}" \
  "${base}/users/ai/${ai_user_id}/api-key-refresh")
new_key=$(jq -re '.data.secret' <<<"$refreshed")

# 3. Verify the fresh key BEFORE persisting it. Never overwrite a working
#    secret with a broken one.
curl -fsS -H "Authorization: Bearer ${new_key}" "${base}/user/me" > /dev/null
echo "fresh key verified"

# 4. Persist.
gh secret set HELIO_E2E_API_KEY --repo "${GITHUB_REPOSITORY}" --body "${new_key}"
echo "HELIO_E2E_API_KEY rotated"
