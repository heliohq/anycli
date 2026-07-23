package mailchimp

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// defaultMetadataURL is the production OAuth metadata endpoint. For OAuth
// tokens it is the only documented source of the account's data-center prefix
// (dc), which the Marketing API base URL is built from.
const defaultMetadataURL = "https://login.mailchimp.com/oauth2/metadata"

// apiKeyDCPattern matches the data-center suffix of a Mailchimp API key
// (e.g. "...-us6"). OAuth access tokens carry no such suffix and fall through
// to the metadata lookup.
var apiKeyDCPattern = regexp.MustCompile(`-([a-z]{2,4}[0-9]+)$`)

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required flag, a bad enum value, or invalid JSON. It maps to exit
// code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Mailchimp non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError resolves through it (401 rejections).
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// requester issues authenticated Marketing API calls for one command
// invocation. It resolves the API base (from the api-key suffix or the OAuth
// metadata endpoint) once and caches it.
type requester struct {
	s     *Service
	token string
	base  string
}

// resolveBase returns the Marketing API base URL, resolving it once. Order:
// an explicit test BaseURL override; the data-center suffix of an API-key
// token; otherwise the OAuth metadata endpoint (one extra round-trip).
func (r *requester) resolveBase(ctx context.Context) (string, error) {
	if r.base != "" {
		return r.base, nil
	}
	if r.s.BaseURL != "" {
		r.base = strings.TrimRight(r.s.BaseURL, "/")
		return r.base, nil
	}
	if m := apiKeyDCPattern.FindStringSubmatch(r.token); m != nil {
		r.base = "https://" + m[1] + ".api.mailchimp.com/3.0"
		return r.base, nil
	}
	base, err := r.fetchMetadataBase(ctx)
	if err != nil {
		return "", err
	}
	r.base = base
	return base, nil
}

// fetchMetadataBase looks up the account's data center via the OAuth metadata
// endpoint. The endpoint is documented with the `OAuth` auth scheme (not
// Bearer). The response's api_endpoint is preferred when present; the official
// guide documents only dc, so we fall back to constructing the base from dc.
func (r *requester) fetchMetadataBase(ctx context.Context) (string, error) {
	metaURL := r.s.MetadataURL
	if metaURL == "" {
		metaURL = defaultMetadataURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metaURL, nil)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("mailchimp: build metadata request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "OAuth "+r.token)
	req.Header.Set("Accept", "application/json")

	resp, err := r.client().Do(req)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("mailchimp: GET metadata: %v", err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("mailchimp: read metadata response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", newAPIError(resp.StatusCode, body)
	}
	var meta struct {
		DC          string `json:"dc"`
		APIEndpoint string `json:"api_endpoint"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", &apiError{msg: fmt.Sprintf("mailchimp: decode metadata: %v", err), err: err}
	}
	if meta.APIEndpoint != "" {
		return strings.TrimRight(meta.APIEndpoint, "/") + "/3.0", nil
	}
	if meta.DC != "" {
		return "https://" + meta.DC + ".api.mailchimp.com/3.0", nil
	}
	return "", &apiError{msg: "mailchimp: metadata response carried no dc or api_endpoint"}
}

// do performs one Marketing API request with Bearer auth and returns the raw
// response body. A 204/empty body returns an empty slice. A non-2xx surfaces
// Mailchimp's problem-detail as an apiError carrying the HTTP status; 401 also
// marks the credential rejected.
func (r *requester) do(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, error) {
	base, err := r.resolveBase(ctx)
	if err != nil {
		return nil, err
	}
	var reqBody io.Reader
	if payload != nil {
		b, mErr := json.Marshal(payload)
		if mErr != nil {
			return nil, &apiError{msg: fmt.Sprintf("mailchimp: encode request: %v", mErr), err: mErr}
		}
		reqBody = bytes.NewReader(b)
	}
	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mailchimp: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mailchimp: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mailchimp: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(resp.StatusCode, body)
	}
	return body, nil
}

func (r *requester) client() *http.Client {
	if r.s.HC != nil {
		return r.s.HC
	}
	return http.DefaultClient
}

// newAPIError builds an apiError from a non-2xx Marketing API response. A 401
// appends a reconnect hint and is marked credential-rejected so the token
// gateway can invalidate the stored credential.
func newAPIError(status int, body []byte) *apiError {
	msg := fmt.Sprintf("mailchimp API error (HTTP %d): %s", status, problemDetail(body))
	if status == http.StatusUnauthorized {
		msg += " — reconnect Mailchimp to refresh the credential"
		return &apiError{msg: msg, status: status, err: execution.RejectCredential(errors.New(msg))}
	}
	return &apiError{msg: msg, status: status}
}

// problemDetail extracts Mailchimp's RFC-7807 problem-detail title/detail from
// an error body, falling back to the raw body.
func problemDetail(body []byte) string {
	var p struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &p); err == nil && (p.Title != "" || p.Detail != "") {
		switch {
		case p.Title != "" && p.Detail != "":
			return p.Title + ": " + p.Detail
		case p.Title != "":
			return p.Title
		default:
			return p.Detail
		}
	}
	return strings.TrimSpace(string(body))
}

// subscriberHash is the MD5 of the lowercase email address — Mailchimp's stable
// member id within a list. Computed client-side (deterministic across API
// versions).
func subscriberHash(email string) string {
	sum := md5.Sum([]byte(strings.ToLower(email)))
	return hex.EncodeToString(sum[:])
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitValue marshals a client-side value (204 receipts) and writes it to
// stdout (+ newline).
func (s *Service) emitValue(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("mailchimp: encode output: %v", err), err: err}
	}
	return s.emit(b)
}

// actionReceipt is the small JSON receipt emitted for action endpoints that
// return 204 No Content (send/test/schedule/unschedule/archive/delete/tag), so
// an agent always receives structured output.
func actionReceipt(action, id string) map[string]any {
	return map[string]any{"ok": true, "action": action, "id": id}
}

// registerListFlags wires the shared pagination + field-projection flags onto a
// list command.
func registerListFlags(cmd *cobra.Command) {
	cmd.Flags().Int("count", 0, "max records to return")
	cmd.Flags().Int("offset", 0, "records to skip (pagination)")
	cmd.Flags().String("fields", "", "comma-separated fields projection (passthrough)")
}

// listQuery builds the query values from the shared list flags, omitting
// zero/empty values so Mailchimp applies its own defaults.
func listQuery(cmd *cobra.Command) url.Values {
	q := url.Values{}
	if c, _ := cmd.Flags().GetInt("count"); c > 0 {
		q.Set("count", strconv.Itoa(c))
	}
	if o, _ := cmd.Flags().GetInt("offset"); o > 0 {
		q.Set("offset", strconv.Itoa(o))
	}
	if f, _ := cmd.Flags().GetString("fields"); f != "" {
		q.Set("fields", f)
	}
	return q
}

// memberSelector resolves the subscriber hash for a member command from exactly
// one of --email (hashed client-side) or --hash (passthrough).
func memberSelector(cmd *cobra.Command, verb string) (string, error) {
	email, _ := cmd.Flags().GetString("email")
	hash, _ := cmd.Flags().GetString("hash")
	switch {
	case email != "" && hash != "":
		return "", &usageError{msg: verb + " requires exactly one of --email or --hash, not both"}
	case email != "":
		return subscriberHash(email), nil
	case hash != "":
		return hash, nil
	default:
		return "", &usageError{msg: verb + " requires one of --email or --hash"}
	}
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: a bare
// group shows help, an unknown subcommand fails with exit 2.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
