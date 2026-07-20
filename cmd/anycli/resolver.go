package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	anycli "github.com/heliohq/anycli"
)

// credEnvPrefix marks the environment variables the dev harness reads as
// credential fields: ANYCLI_CRED_ACCESS_TOKEN becomes Data["access_token"].
const credEnvPrefix = "ANYCLI_CRED_"

// envResolver is the harness's static CredentialResolver: every credential
// field comes from the caller's environment, no server round-trips. This is
// a tool-definition development aid only — production embedding (heliox)
// resolves through the integration token gateway instead.
type envResolver struct {
	environ []string
}

func newEnvResolver(environ []string) envResolver {
	return envResolver{environ: environ}
}

func (r envResolver) Resolve(_ context.Context, tool anycli.Tool, _ string) (*anycli.Credential, error) {
	data := map[string]string{}
	for _, kv := range r.environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || v == "" || !strings.HasPrefix(k, credEnvPrefix) {
			continue
		}
		field := strings.ToLower(strings.TrimPrefix(k, credEnvPrefix))
		if field == "" {
			continue
		}
		data[field] = v
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no %s* environment variables set for tool %q (e.g. %sACCESS_TOKEN)", credEnvPrefix, tool, credEnvPrefix)
	}
	return &anycli.Credential{Data: data, CacheUntil: time.Now().Add(time.Hour)}, nil
}
