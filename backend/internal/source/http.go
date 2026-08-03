package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxResponseBytes = 8 << 20 // 8 MiB guard against hostile or huge responses

// client is shared by the HTTP-based providers. The timeout matters: an import
// runs inside a request, so a hanging tracker must not hold the handler open.
var client = &http.Client{Timeout: 20 * time.Second}

// fetch runs one JSON request against a tracker and decodes it into out.
// method is GET unless a body is supplied.
func fetch(ctx context.Context, kind Kind, method, endpoint, authHeader string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")
	// GitHub rejects requests without one, and it makes the traffic identifiable
	// in every tracker's audit log.
	req.Header.Set("User-Agent", "estimeet")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", kind, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("read %s response: %w", kind, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &Error{Kind: kind, Status: res.StatusCode, Detail: summarize(raw)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s response: %w", kind, err)
	}
	return nil
}

// summarize keeps a short, single-line hint from an error body without echoing
// anything long enough to contain a credential.
func summarize(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", "")
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}
