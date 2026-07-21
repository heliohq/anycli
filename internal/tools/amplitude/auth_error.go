package amplitude

import (
	"fmt"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// euRetryHint is the self-correction guidance appended to a default-region
// (US) 401. An Amplitude project lives in exactly one data-residency silo, and
// its keys are unknown to the other silo's host, so a valid EU-project key
// called against the default US host returns 401 — indistinguishable at the
// transport layer from a genuinely dead key.
const euRetryHint = "; if this is an EU data-residency project, retry with --region eu before reconnecting"

// classifyCredentialError applies the region-aware 401 rule (DESIGN §3.4).
//
//   - Non-401: returned unchanged (transport/API/rate-limit failures never
//     invalidate a credential).
//   - 401 with an EXPLICITLY chosen region (--region us|eu passed): the region
//     is asserted, so a 401 is unambiguous evidence the credential is dead —
//     mark it rejected so the token gateway invalidates it.
//   - 401 with the DEFAULT (unasserted US) region: region-ambiguous — it may be
//     a live EU-project key hitting the wrong silo. Do NOT reject; instead
//     append the --region eu retry hint so the assistant self-corrects rather
//     than looping on a false "reconnect" verdict.
func classifyCredentialError(regionExplicit bool, status int, err error) error {
	if status != http.StatusUnauthorized {
		return err
	}
	if regionExplicit {
		return execution.RejectCredential(err)
	}
	return fmt.Errorf("%w%s", err, euRetryHint)
}
