// Package hass talks to Home Assistant: REST for one-off calls and the
// WebSocket API for live state.
package hass

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUnauthorized means Home Assistant did not accept the access token.
var ErrUnauthorized = errors.New("Home Assistant did not accept the access token")

// State is one entity as Home Assistant reports it.
type State struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
}

// Domain is the part of the entity ID before the dot.
func (s State) Domain() string {
	d, _, _ := strings.Cut(s.EntityID, ".")
	return d
}

// Name is the friendly name, or the entity ID when there is none.
func (s State) Name() string {
	if n, ok := s.Attributes["friendly_name"].(string); ok && n != "" {
		return n
	}
	return s.EntityID
}

// Str returns a string attribute, or "".
func (s State) Str(key string) string {
	v, _ := s.Attributes[key].(string)
	return v
}

// Num returns a numeric attribute and whether it was there.
func (s State) Num(key string) (float64, bool) {
	v, ok := s.Attributes[key].(float64)
	return v, ok
}

// Info is the little we need from /api/config.
type Info struct {
	LocationName string `json:"location_name"`
	Version      string `json:"version"`
}

// Client makes REST calls to one Home Assistant instance.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client for the instance at base, which should already have
// been through NormalizeURL.
func New(base, token string) *Client {
	return &Client{base: base, token: token, http: &http.Client{Timeout: 20 * time.Second}}
}

// NormalizeURL tidies what a person types into the address field:
// "homeassistant.local:8123/" becomes "http://homeassistant.local:8123".
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("enter the address of your Home Assistant")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("that address doesn't look right")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("the address has to start with http:// or https://")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

// WebSocketURL turns the instance address into its WebSocket endpoint.
func WebSocketURL(base string) string {
	switch {
	case strings.HasPrefix(base, "https://"):
		return "wss://" + strings.TrimPrefix(base, "https://") + "/api/websocket"
	default:
		return "ws://" + strings.TrimPrefix(base, "http://") + "/api/websocket"
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error repeats the method and full address; the cause is enough
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("couldn't reach Home Assistant: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("Home Assistant answered %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Info checks the address and token and returns the instance name and version.
func (c *Client) Info(ctx context.Context) (Info, error) {
	var info Info
	err := c.do(ctx, http.MethodGet, "/api/config", nil, &info)
	return info, err
}

// CallService runs a service such as light.turn_on.
func (c *Client) CallService(ctx context.Context, domain, service string, data map[string]any) error {
	return c.do(ctx, http.MethodPost, "/api/services/"+domain+"/"+service, data, nil)
}

// History returns the numeric readings of one entity since the given time.
func (c *Client) History(ctx context.Context, entityID string, since time.Time) ([]Point, error) {
	q := url.Values{}
	q.Set("filter_entity_id", entityID)
	q.Set("end_time", time.Now().UTC().Format(time.RFC3339))
	path := "/api/history/period/" + since.UTC().Format("2006-01-02T15:04:05Z") +
		"?" + q.Encode() + "&minimal_response&no_attributes"
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}
	return ParseHistory(raw)
}
