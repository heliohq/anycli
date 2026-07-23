package metaads

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxErrorBodyBytes = 8 << 10

// get performs an authenticated GET against a Graph node/edge and returns the
// raw JSON body. query holds the field selection and paging parameters.
func (s *Service) get(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	return s.call(ctx, token, http.MethodGet, path, query, nil)
}

// post performs an authenticated POST against a Graph node/edge, sending the
// write parameters as an application/x-www-form-urlencoded body (the Graph
// API's native write encoding).
func (s *Service) post(ctx context.Context, token, path string, form url.Values) ([]byte, error) {
	return s.call(ctx, token, http.MethodPost, path, nil, form)
}

func (s *Service) call(ctx context.Context, token, method, path string, query, form url.Values) ([]byte, error) {
	requestURL := s.apiBase() + s.graphPath(path)
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("meta-ads: build request: %w", err)
	}
	// Send the token via the Authorization header (not ?access_token=) so it
	// never lands in a URL or server access log.
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta-ads: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("meta-ads: read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newAPIError(resp.StatusCode, respBody, token)
	}
	return respBody, nil
}

func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
