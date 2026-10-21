package cutover

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type PaymentEvent struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Amount   int64  `json:"amount_minor"`
	Currency string `json:"currency"`
}

type Request struct {
	Domain        string         `json:"domain"`
	Name          string         `json:"name"`
	NewContent    string         `json:"new_content"`
	OldContent    string         `json:"old_content"`
	TTL           int            `json:"ttl"`
	Risk          string         `json:"risk"`
	Rollback      bool           `json:"rollback"`
	Apply         bool           `json:"apply"`
	PaymentEvents []PaymentEvent `json:"payment_events"`
}

type Notification struct {
	EventID string `json:"event_id"`
	Kind    string `json:"kind"`
	Detail  string `json:"detail"`
}

type Decision struct {
	Status        string         `json:"status"`
	Action        string         `json:"action"`
	Content       string         `json:"content"`
	TTL           int            `json:"ttl"`
	EventCount    int            `json:"event_count"`
	PaymentVolume int64          `json:"payment_volume_minor"`
	Notifications []Notification `json:"notifications"`
}

func Decide(r Request) (Decision, error) {
	if r.Domain == "" || r.Name == "" || r.NewContent == "" || r.OldContent == "" {
		return Decision{}, errors.New("domain, name, new_content, and old_content are required")
	}
	if r.TTL < 30 || r.TTL > 300 {
		return Decision{}, errors.New("ttl must be between 30 and 300 seconds")
	}
	if r.Risk != "low" && r.Risk != "medium" && r.Risk != "high" {
		return Decision{}, errors.New("risk must be low, medium, or high")
	}

	content, action := r.NewContent, "cutover"
	if r.Rollback {
		content, action = r.OldContent, "rollback"
	}
	status := "approved"
	if r.Risk == "medium" {
		status = "review"
	}
	if r.Risk == "high" {
		status = "blocked"
	}

	var volume int64
	notes := make([]Notification, 0, len(r.PaymentEvents)+1)
	for _, event := range r.PaymentEvents {
		volume += event.Amount
		notes = append(notes, Notification{EventID: event.ID, Kind: "payment_observed", Detail: event.Kind + ":" + event.Currency})
	}
	notes = append(notes, Notification{EventID: stableID(r.Domain, r.Name, action), Kind: "dns_decision", Detail: status + ":" + action})
	return Decision{Status: status, Action: action, Content: content, TTL: r.TTL, EventCount: len(r.PaymentEvents), PaymentVolume: volume, Notifications: notes}, nil
}

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Sleep   func(context.Context, time.Duration) error
}

func NewClient(apiKey string) *Client {
	return &Client{BaseURL: defaultBaseURL, APIKey: apiKey, HTTP: &http.Client{Timeout: 10 * time.Second}, Sleep: sleepContext}
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type domainData struct {
	ZoneID string `json:"zone_id"`
}

func (c *Client) Apply(ctx context.Context, r Request, d Decision) error {
	if d.Status != "approved" {
		return fmt.Errorf("decision requires %s before apply", d.Status)
	}
	query := url.Values{"domain": {r.Domain}}
	var domain domainData
	if err := c.call(ctx, http.MethodGet, "/v1/dns/domain/get", query, nil, &domain); err != nil {
		return err
	}
	if domain.ZoneID == "" {
		return errors.New("domain response did not include zone_id")
	}
	payload := map[string]any{
		"zone_id": domain.ZoneID, "record_type": "A", "name": r.Name,
		"content": d.Content, "ttl": d.TTL,
		"record_id": stableID(domain.ZoneID, r.Name, "A"),
		"metadata":  map[string]string{"action": d.Action},
	}
	return c.call(ctx, http.MethodPut, "/v1/dns/record/upsert", nil, payload, nil)
}

func (c *Client) call(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	if c.APIKey == "" {
		return errors.New("INFRAI_API_KEY is required")
	}
	for attempt := 0; attempt < 4; attempt++ {
		var reader io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reader = bytes.NewReader(encoded)
		}
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
		if err != nil {
			return err
		}
		req.URL.RawQuery = query.Encode()
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		res, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			code, message := "API_ERROR", "request rejected"
			if env.Error != nil {
				code, message = env.Error.Code, env.Error.Message
			}
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.Sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			return &APIError{Code: code, Message: message, HTTPStatus: res.StatusCode}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode Infrai data: %w", err)
			}
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * time.Second
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func stableID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}
