package operator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// SagaRun is one saga of a release's journal as the API lists it.
type SagaRun struct {
	ID      string `json:"id"`
	At      string `json:"at"`
	Outcome string `json:"outcome"`
}

// SagaEvent is a journal event as the API replays it (the fields a diagnosis reads).
type SagaEvent struct {
	Seq     int64  `json:"seq"`
	Key     string `json:"key,omitempty"`
	Step    string `json:"step,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// LogsDoc is what pack logs --json says: the lines of each key by instance, and the keys whose target
// gives none.
type LogsDoc struct {
	Logs   map[string]map[string][]string `json:"logs"`
	NoLogs []string                       `json:"no_logs,omitempty"`
}

// Sagas is the release's sagas, the newest first.
func (c *Client) Sagas(ctx context.Context, release string) ([]SagaRun, error) {
	var out []SagaRun
	return out, c.Get(ctx, "/api/releases/"+relEsc(release)+"/sagas", &out)
}

// SagaEvents is the saga's journal, replayed from the API's SSE stream and read whole.
func (c *Client) SagaEvents(ctx context.Context, release, saga string) ([]SagaEvent, error) {
	path := "/api/releases/" + relEsc(release) + "/sagas/" + url.PathEscape(saga) + "/events"
	req, err := http.NewRequestWithContext(ctx, "GET", c.Base+path, nil)
	if err != nil {
		return nil, err
	}
	if len(c.User) > 0 {
		req.Header.Set("X-Pack-User", c.User)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &Failure{Code: "error", Message: fmt.Sprintf("%s: %s", resp.Status, bytes.TrimSpace(raw))}
	}
	var events []SagaEvent
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		var ev SagaEvent
		if err := json.Unmarshal(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:"))), &ev); err == nil {
			events = append(events, ev)
		}
	}
	return events, sc.Err()
}

// Logs is the tail of the key's logs as the API's pack logs --json says them (tail lines of each instance).
func (c *Client) Logs(ctx context.Context, release, key string, tail int) (*LogsDoc, error) {
	q := url.Values{}
	if len(key) > 0 {
		q.Set("key", key)
	}
	if tail > 0 {
		q.Set("tail", fmt.Sprint(tail))
	}
	raw, err := c.Raw(ctx, release, "logs", q.Encode())
	if err != nil {
		return nil, err
	}
	var doc LogsDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("logs: %w", err)
	}
	return &doc, nil
}
