package dataforseo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DataForSEO in-body status codes. The API returns HTTP 200 for almost
// everything and rides real errors in the body's status_code fields.
const (
	statusOK          = 20000 // "Ok."
	statusTaskCreated = 20100 // "Task Created." (task queue mode; success)
	statusNotAuth     = 40100 // not authorized → credential rejected
	statusPayment     = 40200 // payment required / out of funds
	statusFunds       = 40210 // insufficient funds
)

// usageError is a parameter / usage error: it maps to exit code 2 and kind
// "usage". Cobra's own parse errors are treated the same way in Execute.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an HTTP non-2xx, an in-body error status
// code, or a transport failure. It maps to exit code 1 and kind "api". It wraps
// the underlying cause so errors.As for a credential rejection still resolves
// through it.
type apiError struct {
	msg string
	err error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// apiResult is the unwrapped, agent-facing view of a DataForSEO response:
// the metered top-level cost plus tasks[0].result.
type apiResult struct {
	Cost   float64         `json:"cost"`
	Result json.RawMessage `json:"result"`
}

// basicAuth builds the Authorization header value from the stored login:password
// pair. A value without a colon is a malformed credential (the connect form
// stored something that is not a pair) and is reported as a credential
// rejection so Helio surfaces stale-credential feedback.
func basicAuth(credential string) (string, error) {
	if !strings.Contains(credential, ":") {
		base := errors.New(EnvCredentials + " must be a login:password pair (colon-separated); reconnect DataForSEO with valid API credentials")
		return "", &apiError{msg: base.Error(), err: execution.RejectCredential(base)}
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(credential)), nil
}

// callRaw performs one request and returns the HTTP status and raw body.
// POST wraps the task object in a single-element array (the DataForSEO generic
// POST shape; Live endpoints accept exactly one task); GET sends no body.
func (s *Service) callRaw(ctx context.Context, credential, method, path string, task map[string]any) (int, []byte, error) {
	auth, err := basicAuth(credential)
	if err != nil {
		return 0, nil, err
	}
	var reqBody io.Reader
	if method == http.MethodPost {
		payload := []map[string]any{}
		if task != nil {
			payload = append(payload, task)
		}
		b, mErr := json.Marshal(payload)
		if mErr != nil {
			return 0, nil, &apiError{msg: fmt.Sprintf("dataforseo: encode request: %v", mErr), err: mErr}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL()+path, reqBody)
	if err != nil {
		return 0, nil, &apiError{msg: fmt.Sprintf("dataforseo: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, nil, &apiError{msg: fmt.Sprintf("dataforseo: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, &apiError{msg: fmt.Sprintf("dataforseo: read response: %v", err), err: err}
	}
	return resp.StatusCode, body, nil
}

// envelope is the DataForSEO response wrapper. Only the fields this service
// needs are decoded.
type envelope struct {
	StatusCode    int    `json:"status_code"`
	StatusMessage string `json:"status_message"`
	Cost          float64
	Tasks         []struct {
		StatusCode    int             `json:"status_code"`
		StatusMessage string          `json:"status_message"`
		Result        json.RawMessage `json:"result"`
	} `json:"tasks"`
}

func (e *envelope) UnmarshalJSON(b []byte) error {
	type alias envelope
	var raw struct {
		alias
		Cost json.Number `json:"cost"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*e = envelope(raw.alias)
	e.Cost, _ = raw.Cost.Float64()
	return nil
}

// callAPI performs a request and maps HTTP status + the DataForSEO in-body
// status codes to either an unwrapped {cost, result} or a classified error.
func (s *Service) callAPI(ctx context.Context, credential, method, path string, task map[string]any) (*apiResult, error) {
	status, body, err := s.callRaw(ctx, credential, method, path, task)
	if err != nil {
		return nil, err
	}
	// HTTP-level errors: 401 is a credential rejection; other non-2xx is a plain
	// API error (402 billing, 404 unknown endpoint, 500 server).
	if status == http.StatusUnauthorized {
		base := fmt.Errorf("dataforseo API error (HTTP 401): %s", firstNonEmpty(bodyMessage(body), "unauthorized"))
		return nil, &apiError{msg: base.Error(), err: execution.RejectCredential(base)}
	}
	if status < 200 || status > 299 {
		return nil, &apiError{msg: fmt.Sprintf("dataforseo API error (HTTP %d): %s", status, firstNonEmpty(bodyMessage(body), string(body)))}
	}

	var env envelope
	if uErr := json.Unmarshal(body, &env); uErr != nil {
		return nil, &apiError{msg: fmt.Sprintf("dataforseo: decode response: %v", uErr), err: uErr}
	}
	// Top-level status: a non-success code means the whole call was rejected.
	if !isSuccess(env.StatusCode) {
		return nil, classifyBodyError(env.StatusCode, env.StatusMessage)
	}
	// Task-level status: Live endpoints carry one task; a per-task error (e.g.
	// 40501 invalid field) rides under a top-level 20000.
	if len(env.Tasks) > 0 && !isSuccess(env.Tasks[0].StatusCode) {
		return nil, classifyBodyError(env.Tasks[0].StatusCode, env.Tasks[0].StatusMessage)
	}
	res := &apiResult{Cost: env.Cost}
	if len(env.Tasks) > 0 {
		res.Result = env.Tasks[0].Result
	}
	return res, nil
}

// isSuccess reports whether a DataForSEO status_code is a success code.
func isSuccess(code int) bool {
	return code == statusOK || code == statusTaskCreated
}

// classifyBodyError turns an in-body error status code into the right typed
// error: 40100 → credential rejection; 40200/40210 → explicit balance guidance;
// anything else → a plain API error carrying the code and message.
func classifyBodyError(code int, message string) error {
	switch code {
	case statusNotAuth:
		base := fmt.Errorf("dataforseo API error (status_code %d): %s", code, firstNonEmpty(message, "not authorized"))
		return &apiError{msg: base.Error(), err: execution.RejectCredential(base)}
	case statusPayment, statusFunds:
		return &apiError{msg: fmt.Sprintf(
			"dataforseo: insufficient DataForSEO balance (status_code %d): %s — top up at https://app.dataforseo.com",
			code, firstNonEmpty(message, "payment required"))}
	default:
		return &apiError{msg: fmt.Sprintf("dataforseo API error (status_code %d): %s", code, firstNonEmpty(message, "request failed"))}
	}
}

// bodyMessage extracts a top-level status_message from an error body, if any.
func bodyMessage(body []byte) string {
	var e struct {
		StatusMessage string `json:"status_message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		return e.StatusMessage
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// emit writes the {cost, result} envelope to stdout (+ newline).
func (s *Service) emit(res *apiResult) error {
	out := map[string]any{"cost": res.Cost, "result": rawOrNull(res.Result)}
	b, err := json.Marshal(out)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("dataforseo: encode output: %v", err), err: err}
	}
	if _, err := s.stdout().Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// rawOrNull returns the raw JSON message, or a JSON null when it is empty, so
// json.Marshal renders `null` rather than an empty string.
func rawOrNull(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return raw
}

// do runs a request and emits the unwrapped {cost, result}.
func (s *Service) do(ctx context.Context, credential, method, path string, task map[string]any) error {
	res, err := s.callAPI(ctx, credential, method, path, task)
	if err != nil {
		return err
	}
	return s.emit(res)
}

// --- shared flag helpers -----------------------------------------------------

// taskParams holds the location/language pair most endpoints accept. location
// may be a location name or a numeric location_code; language is a language
// code (e.g. en).
type taskParams struct {
	location string
	language string
}

// registerLocationLang wires --location / --language onto a command with the
// design defaults (United States, en).
func registerLocationLang(cmd *cobra.Command, p *taskParams) {
	cmd.Flags().StringVar(&p.location, "location", "United States", "location name or numeric location_code")
	cmd.Flags().StringVar(&p.language, "language", "en", "language code, e.g. en")
}

// registerLanguageOnly wires --language onto a command whose endpoint takes no
// location (e.g. search intent).
func registerLanguageOnly(cmd *cobra.Command, p *taskParams) {
	cmd.Flags().StringVar(&p.language, "language", "en", "language code, e.g. en")
}

// apply writes the location/language params into a task object. A numeric
// location becomes location_code; anything else becomes location_name.
func (p taskParams) apply(task map[string]any) {
	if p.location != "" {
		if code, err := strconv.Atoi(p.location); err == nil {
			task["location_code"] = code
		} else {
			task["location_name"] = p.location
		}
	}
	if p.language != "" {
		task["language_code"] = p.language
	}
}

// splitKeywords parses a comma-separated --keywords value into a trimmed,
// empty-free slice.
func splitKeywords(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
