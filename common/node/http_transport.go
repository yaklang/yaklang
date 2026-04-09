package node

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

const (
	bootstrapEndpointPath = "/v1/nodes/bootstrap"
	heartbeatEndpointFmt  = "/v1/node-sessions/%s/heartbeats"
	shutdownEndpointFmt   = "/v1/node-sessions/%s/shutdown"
)

// SessionTransport defines how a node acquires and renews a platform session.
type SessionTransport interface {
	Bootstrap(context.Context, BootstrapRequest) (SessionState, error)
	Heartbeat(context.Context, SessionState, HeartbeatRequest) error
	Shutdown(context.Context, SessionState, ShutdownRequest) error
}

// BootstrapRequest is sent once to create a short-lived node session.
type BootstrapRequest struct {
	EnrollmentToken          string            `json:"enrollment_token"`
	NodeID                   string            `json:"node_id"`
	NodeType                 string            `json:"node_type"`
	Version                  string            `json:"version"`
	Labels                   map[string]string `json:"labels"`
	CapabilityKeys           []string          `json:"capability_keys"`
	HeartbeatIntervalSeconds uint32            `json:"heartbeat_interval_seconds"`
}

// SessionState is the session material returned by the platform.
type SessionState struct {
	SessionID          string
	SessionToken       string
	NATSURL            string
	CommandSubject     string
	EventSubjectPrefix string
	ExpiresAt          time.Time
}

// HeartbeatRequest keeps the node session alive and reports runtime state.
type HeartbeatRequest struct {
	LifecycleState           string                   `json:"lifecycle_state"`
	Version                  string                   `json:"version"`
	RunningJobs              uint32                   `json:"running_jobs"`
	MaxRunningJobs           uint32                   `json:"max_running_jobs"`
	CapabilityKeys           []string                 `json:"capability_keys"`
	Labels                   map[string]string        `json:"labels"`
	ObservedAt               time.Time                `json:"observed_at"`
	HeartbeatIntervalSeconds uint32                   `json:"heartbeat_interval_seconds"`
	ActiveAttempts           []ActiveAttemptHeartbeat `json:"active_attempts"`
}

// ShutdownRequest marks a node session inactive immediately on graceful exit.
type ShutdownRequest struct {
	ObservedAt time.Time `json:"observed_at"`
}

// HTTPTransportConfig configures the platform HTTP transport.
type HTTPTransportConfig struct {
	BaseURL string
	Client  *http.Client
}

type httpTransport struct {
	baseURL string
	client  *http.Client
}

type HTTPStatusError struct {
	StatusCode int
	Message    string
	Body       string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "transport status=0"
	}
	if e.Message != "" {
		return fmt.Sprintf("transport status=%d error=%s", e.StatusCode, e.Message)
	}
	if e.Body != "" {
		return fmt.Sprintf("transport status=%d body=%s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("transport status=%d", e.StatusCode)
}

func IsSessionInactiveTransportError(err error) bool {
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	if statusErr.StatusCode != http.StatusConflict {
		return false
	}

	message := strings.ToLower(strings.TrimSpace(statusErr.Message))
	body := strings.ToLower(strings.TrimSpace(statusErr.Body))
	return strings.Contains(message, "node session is not active") ||
		strings.Contains(body, "node session is not active")
}

// NewHTTPTransport creates the default HTTP session transport.
func NewHTTPTransport(cfg HTTPTransportConfig) (SessionTransport, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("http transport base_url is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse transport base_url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("http transport base_url must include scheme and host")
	}

	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: DefaultRequestTimeout}
	}
	return &httpTransport{baseURL: baseURL, client: client}, nil
}

func (t *httpTransport) Bootstrap(
	ctx context.Context,
	request BootstrapRequest,
) (SessionState, error) {
	var response struct {
		NodeSessionID      string    `json:"node_session_id"`
		SessionToken       string    `json:"session_token"`
		NATSURL            string    `json:"nats_url"`
		CommandSubject     string    `json:"command_subject"`
		EventSubjectPrefix string    `json:"event_subject_prefix"`
		ExpiresAt          time.Time `json:"expires_at"`
	}

	if err := t.postJSON(ctx, bootstrapEndpointPath, "", request, &response); err != nil {
		return SessionState{}, err
	}
	return SessionState{
		SessionID:          response.NodeSessionID,
		SessionToken:       response.SessionToken,
		NATSURL:            response.NATSURL,
		CommandSubject:     response.CommandSubject,
		EventSubjectPrefix: response.EventSubjectPrefix,
		ExpiresAt:          response.ExpiresAt,
	}, nil
}

func (t *httpTransport) Heartbeat(
	ctx context.Context,
	session SessionState,
	request HeartbeatRequest,
) error {
	endpoint := fmt.Sprintf(heartbeatEndpointFmt, url.PathEscape(session.SessionID))
	return t.postJSON(ctx, endpoint, session.SessionToken, request, nil)
}

func (t *httpTransport) Shutdown(
	ctx context.Context,
	session SessionState,
	request ShutdownRequest,
) error {
	endpoint := fmt.Sprintf(shutdownEndpointFmt, url.PathEscape(session.SessionID))
	return t.postJSON(ctx, endpoint, session.SessionToken, request, nil)
}

func (t *httpTransport) postJSON(
	ctx context.Context,
	path string,
	bearerToken string,
	requestBody any,
	responseBody any,
) error {
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal transport request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		t.baseURL+path,
		bytes.NewReader(raw),
	)
	if err != nil {
		return fmt.Errorf("build transport request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	response, err := t.client.Do(request)
	if err != nil {
		return fmt.Errorf("send transport request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		return readHTTPError(response)
	}
	if responseBody == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(responseBody); err != nil {
		return fmt.Errorf("decode transport response: %w", err)
	}
	return nil
}

func readHTTPError(response *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return fmt.Errorf("transport status=%d read_body=%v", response.StatusCode, err)
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return &HTTPStatusError{StatusCode: response.StatusCode}
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error) != "" {
		return &HTTPStatusError{
			StatusCode: response.StatusCode,
			Message:    strings.TrimSpace(payload.Error),
			Body:       trimmed,
		}
	}
	return &HTTPStatusError{
		StatusCode: response.StatusCode,
		Body:       trimmed,
	}
}
