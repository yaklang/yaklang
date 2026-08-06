//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	browserTransformAdapterProtocolVersion = "1"
	browserTransformAdapterMaxPacketBytes  = browserTransformMaxBodyBytes + 1024*1024
	browserTransformAdapterMaxPayloadBytes = 28 * 1024 * 1024
	browserTransformAdapterMaxConcurrency  = 16
)

type browserTransformAdapterCall struct {
	Version       int    `json:"version"`
	Direction     string `json:"direction"`
	HTTPS         bool   `json:"https"`
	PacketBase64  string `json:"packetBase64"`
	RequestBase64 string `json:"requestBase64,omitempty"`
}

type browserTransformAdapterResult struct {
	Version      int     `json:"version"`
	Sequence     uint64  `json:"sequence"`
	ProfileID    string  `json:"profileId"`
	Direction    string  `json:"direction"`
	PacketBase64 string  `json:"packetBase64"`
	Applied      bool    `json:"applied"`
	BypassReason string  `json:"bypassReason,omitempty"`
	DurationMs   float64 `json:"durationMs"`
}

type browserTransformAdapterHTTPError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type browserTransformExternalAdapter struct {
	runtime  *browserTransformRuntime
	listener net.Listener
	server   *http.Server
	token    string
	host     string
	port     int
	endpoint string
	timeout  time.Duration

	running      atomic.Bool
	sequence     atomic.Uint64
	requestCount atomic.Uint64
	bypassCount  atomic.Uint64
	failureCount atomic.Uint64
	lastUsedAt   atomic.Int64
	startedAt    time.Time
	concurrency  chan struct{}

	errorMu   sync.RWMutex
	lastError string
}

func normalizeBrowserTransformAdapterHost(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" || strings.EqualFold(host, "localhost") {
		return "127.0.0.1", nil
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("browser transform adapter must listen on a loopback address")
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.String(), nil
	}
	return ip.String(), nil
}

func browserTransformAdapterTimeout(milliseconds int64) time.Duration {
	if milliseconds <= 0 {
		return 10 * time.Second
	}
	timeout := time.Duration(milliseconds) * time.Millisecond
	if timeout < 2*time.Second {
		return 2 * time.Second
	}
	if timeout > 60*time.Second {
		return 60 * time.Second
	}
	return timeout
}

func newBrowserTransformAdapterToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("create browser transform adapter token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func newBrowserTransformExternalAdapter(
	ctx context.Context,
	caller browserTransformCaller,
	deviceID string,
	profileID string,
	host string,
	port int,
	timeout time.Duration,
) (*browserTransformExternalAdapter, error) {
	if port < 0 || port > 65535 {
		return nil, errors.New("browser transform adapter port is out of range")
	}
	normalizedHost, err := normalizeBrowserTransformAdapterHost(host)
	if err != nil {
		return nil, err
	}
	runtime, err := prepareBrowserTransform(ctx, caller, deviceID, profileID, timeout)
	if err != nil {
		return nil, err
	}
	if runtime == nil {
		return nil, errors.New("browser transform adapter requires a paired browser and a transform profile")
	}
	token, err := newBrowserTransformAdapterToken()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(normalizedHost, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("listen for browser transform adapter: %w", err)
	}
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, errors.New("browser transform adapter did not receive a TCP listener")
	}
	adapter := &browserTransformExternalAdapter{
		runtime:     runtime,
		listener:    listener,
		token:       token,
		host:        normalizedHost,
		port:        tcpAddress.Port,
		timeout:     timeout,
		startedAt:   time.Now(),
		concurrency: make(chan struct{}, browserTransformAdapterMaxConcurrency),
	}
	adapter.endpoint = "http://" + net.JoinHostPort(adapter.host, strconv.Itoa(adapter.port))
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", adapter.handleHealth)
	mux.HandleFunc("/v1/transform", adapter.handleTransform)
	adapter.server = &http.Server{
		Handler:           adapter.secureHeaders(mux),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       timeout + 5*time.Second,
		WriteTimeout:      timeout + 5*time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}
	adapter.running.Store(true)
	go func() {
		serveErr := adapter.server.Serve(listener)
		adapter.running.Store(false)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			adapter.recordFailure(serveErr)
		}
	}()
	return adapter, nil
}

func (a *browserTransformExternalAdapter) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(writer, request)
	})
}

func browserTransformAdapterMessage(err error) string {
	message := "browser transform adapter failed"
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	message = strings.NewReplacer("\r", " ", "\n", " ").Replace(message)
	if len(message) > 1024 {
		message = message[:1024]
	}
	return message
}

func (a *browserTransformExternalAdapter) recordFailure(err error) {
	a.failureCount.Add(1)
	a.errorMu.Lock()
	a.lastError = browserTransformAdapterMessage(err)
	a.errorMu.Unlock()
}

func (a *browserTransformExternalAdapter) clearFailure() {
	a.errorMu.Lock()
	a.lastError = ""
	a.errorMu.Unlock()
}

func (a *browserTransformExternalAdapter) errorMessage() string {
	a.errorMu.RLock()
	defer a.errorMu.RUnlock()
	return a.lastError
}

func (a *browserTransformExternalAdapter) authorized(request *http.Request) bool {
	parts := strings.Fields(request.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) != len(a.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(a.token)) == 1
}

func writeBrowserTransformAdapterJSON(writer http.ResponseWriter, status int, value interface{}) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeBrowserTransformAdapterError(writer http.ResponseWriter, status int, code string, err error) {
	value := browserTransformAdapterHTTPError{}
	value.Error.Code = code
	value.Error.Message = browserTransformAdapterMessage(err)
	writeBrowserTransformAdapterJSON(writer, status, value)
}

func (a *browserTransformExternalAdapter) authorizeHTTP(writer http.ResponseWriter, request *http.Request) bool {
	if request.Header.Get("Origin") != "" {
		writeBrowserTransformAdapterError(writer, http.StatusForbidden, "browser_origin_rejected", errors.New("browser-originated adapter calls are not allowed"))
		return false
	}
	if !a.authorized(request) {
		writer.Header().Set("WWW-Authenticate", "Bearer realm=\"Yak Browser Transform Adapter\"")
		writeBrowserTransformAdapterError(writer, http.StatusUnauthorized, "unauthorized", errors.New("a valid bearer token is required"))
		return false
	}
	return true
}

func (a *browserTransformExternalAdapter) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeBrowserTransformAdapterError(writer, http.StatusMethodNotAllowed, "method_not_allowed", errors.New("health accepts GET only"))
		return
	}
	if !a.authorizeHTTP(writer, request) {
		return
	}
	writeBrowserTransformAdapterJSON(writer, http.StatusOK, map[string]interface{}{
		"version":         1,
		"running":         a.running.Load(),
		"profileId":       a.runtime.profileID,
		"profileName":     a.runtime.profileName,
		"requestEnabled":  a.runtime.requestEnabled,
		"responseEnabled": a.runtime.responseEnabled,
		"requestCount":    a.requestCount.Load(),
		"bypassCount":     a.bypassCount.Load(),
		"failureCount":    a.failureCount.Load(),
	})
}

func decodeBrowserTransformAdapterPacket(raw string) ([]byte, error) {
	if raw == "" {
		return nil, errors.New("packetBase64 is required")
	}
	if base64.StdEncoding.DecodedLen(len(raw)) > browserTransformAdapterMaxPacketBytes {
		return nil, errors.New("adapter packet exceeds 9 MiB")
	}
	packet, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("adapter packet is not valid standard Base64")
	}
	if len(packet) == 0 || len(packet) > browserTransformAdapterMaxPacketBytes {
		return nil, errors.New("adapter packet is empty or exceeds 9 MiB")
	}
	return packet, nil
}

func browserTransformWildcardMatch(pattern string, value string) bool {
	pattern = strings.ToLower(pattern)
	value = strings.ToLower(value)
	patternIndex, valueIndex := 0, 0
	starIndex, retryIndex := -1, 0
	for valueIndex < len(value) {
		if patternIndex < len(pattern) && pattern[patternIndex] == value[valueIndex] {
			patternIndex++
			valueIndex++
			continue
		}
		if patternIndex < len(pattern) && pattern[patternIndex] == '*' {
			starIndex = patternIndex
			patternIndex++
			retryIndex = valueIndex
			continue
		}
		if starIndex >= 0 {
			patternIndex = starIndex + 1
			retryIndex++
			valueIndex = retryIndex
			continue
		}
		return false
	}
	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}

func browserTransformAdapterRouteMatches(runtime *browserTransformRuntime, requestPacket []byte, isHTTPS bool) (bool, error) {
	if runtime == nil {
		return false, errors.New("browser transform adapter profile is unavailable")
	}
	method, _, _ := lowhttp.GetHTTPPacketFirstLine(requestPacket)
	if strings.TrimSpace(method) == "" {
		return false, errors.New("adapter request packet has no HTTP method")
	}
	if len(runtime.methods) > 0 {
		matchedMethod := false
		for _, allowed := range runtime.methods {
			if strings.EqualFold(strings.TrimSpace(allowed), method) {
				matchedMethod = true
				break
			}
		}
		if !matchedMethod {
			return false, nil
		}
	}
	requestURL, err := lowhttp.ExtractURLFromHTTPRequestRaw(requestPacket, isHTTPS)
	if err != nil {
		return false, fmt.Errorf("resolve adapter request URL: %w", err)
	}
	if requestURL == nil {
		return false, errors.New("resolve adapter request URL: empty URL")
	}
	pattern := strings.TrimSpace(runtime.urlPattern)
	if pattern == "" || pattern == "*" {
		pattern = "*"
	}
	lowerPattern := strings.ToLower(pattern)
	explicitOrigin := strings.HasPrefix(lowerPattern, "http://") ||
		strings.HasPrefix(lowerPattern, "https://") || strings.HasPrefix(lowerPattern, "*://")
	if runtime.origin != "" && !explicitOrigin {
		profileOrigin, parseErr := url.Parse(runtime.origin)
		if parseErr != nil || profileOrigin == nil || profileOrigin.Scheme == "" || profileOrigin.Hostname() == "" {
			return false, errors.New("browser transform adapter profile has an invalid origin")
		}
		if !strings.EqualFold(requestURL.Scheme, profileOrigin.Scheme) ||
			!strings.EqualFold(requestURL.Hostname(), profileOrigin.Hostname()) ||
			browserTransformEffectivePort(requestURL) != browserTransformEffectivePort(profileOrigin) {
			return false, nil
		}
	}
	if browserTransformWildcardMatch(pattern, requestURL.String()) ||
		browserTransformWildcardMatch(pattern, requestURL.EscapedPath()) {
		return true, nil
	}
	return false, nil
}

func (a *browserTransformExternalAdapter) handleTransform(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeBrowserTransformAdapterError(writer, http.StatusMethodNotAllowed, "method_not_allowed", errors.New("transform accepts POST only"))
		return
	}
	if !a.authorizeHTTP(writer, request) {
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeBrowserTransformAdapterError(writer, http.StatusUnsupportedMediaType, "unsupported_media_type", errors.New("Content-Type must be application/json"))
		return
	}
	if request.ContentLength > browserTransformAdapterMaxPayloadBytes {
		err := errors.New("adapter payload exceeds 28 MiB")
		a.recordFailure(err)
		writeBrowserTransformAdapterError(writer, http.StatusRequestEntityTooLarge, "payload_too_large", err)
		return
	}
	select {
	case a.concurrency <- struct{}{}:
		defer func() { <-a.concurrency }()
	default:
		a.recordFailure(errors.New("adapter concurrency limit reached"))
		writeBrowserTransformAdapterError(writer, http.StatusTooManyRequests, "adapter_busy", errors.New("adapter concurrency limit reached"))
		return
	}

	request.Body = http.MaxBytesReader(writer, request.Body, browserTransformAdapterMaxPayloadBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var call browserTransformAdapterCall
	if err := decoder.Decode(&call); err != nil {
		a.recordFailure(err)
		status, code := http.StatusBadRequest, "invalid_payload"
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			status, code = http.StatusRequestEntityTooLarge, "payload_too_large"
		}
		writeBrowserTransformAdapterError(writer, status, code, err)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("adapter payload contains multiple JSON values")
		}
		a.recordFailure(err)
		writeBrowserTransformAdapterError(writer, http.StatusBadRequest, "invalid_payload", err)
		return
	}
	if call.Version != 1 {
		err := errors.New("adapter protocol version must be 1")
		a.recordFailure(err)
		writeBrowserTransformAdapterError(writer, http.StatusBadRequest, "unsupported_version", err)
		return
	}
	call.Direction = strings.ToLower(strings.TrimSpace(call.Direction))
	if call.Direction != "request" && call.Direction != "response" {
		err := errors.New("direction must be request or response")
		a.recordFailure(err)
		writeBrowserTransformAdapterError(writer, http.StatusBadRequest, "invalid_direction", err)
		return
	}
	if (call.Direction == "request" && !a.runtime.requestEnabled) ||
		(call.Direction == "response" && !a.runtime.responseEnabled) {
		err := fmt.Errorf("profile direction %s is disabled", call.Direction)
		a.recordFailure(err)
		writeBrowserTransformAdapterError(writer, http.StatusConflict, "direction_disabled", err)
		return
	}
	packet, err := decodeBrowserTransformAdapterPacket(call.PacketBase64)
	if err != nil {
		a.recordFailure(err)
		writeBrowserTransformAdapterError(writer, http.StatusBadRequest, "invalid_packet", err)
		return
	}
	requestPacket := packet
	if call.Direction == "response" {
		if call.RequestBase64 == "" {
			err = errors.New("requestBase64 is required for response transforms")
		} else {
			requestPacket, err = decodeBrowserTransformAdapterPacket(call.RequestBase64)
		}
		if err != nil {
			a.recordFailure(err)
			writeBrowserTransformAdapterError(writer, http.StatusBadRequest, "invalid_request_packet", err)
			return
		}
	}
	matched, err := browserTransformAdapterRouteMatches(a.runtime, requestPacket, call.HTTPS)
	if err != nil {
		a.recordFailure(err)
		writeBrowserTransformAdapterError(writer, http.StatusBadRequest, "invalid_request_packet", err)
		return
	}
	if !matched {
		sequence := a.sequence.Add(1)
		a.bypassCount.Add(1)
		a.lastUsedAt.Store(time.Now().UnixMilli())
		writeBrowserTransformAdapterJSON(writer, http.StatusOK, browserTransformAdapterResult{
			Version:      1,
			Sequence:     sequence,
			ProfileID:    a.runtime.profileID,
			Direction:    call.Direction,
			PacketBase64: call.PacketBase64,
			Applied:      false,
			BypassReason: "route_mismatch",
		})
		return
	}

	sequence := a.sequence.Add(1)
	a.requestCount.Add(1)
	a.lastUsedAt.Store(time.Now().UnixMilli())
	started := time.Now()
	callContext, cancel := context.WithTimeout(request.Context(), a.timeout)
	defer cancel()
	output, err := a.runtime.transformPacket(callContext, call.Direction, packet, requestPacket, call.HTTPS)
	if err != nil {
		a.recordFailure(err)
		writeBrowserTransformAdapterError(writer, http.StatusBadGateway, "transform_failed", err)
		return
	}
	a.clearFailure()
	writeBrowserTransformAdapterJSON(writer, http.StatusOK, browserTransformAdapterResult{
		Version:      1,
		Sequence:     sequence,
		ProfileID:    a.runtime.profileID,
		Direction:    call.Direction,
		PacketBase64: base64.StdEncoding.EncodeToString(output),
		Applied:      true,
		DurationMs:   float64(time.Since(started).Microseconds()) / 1000,
	})
}

func (a *browserTransformExternalAdapter) status(includeToken bool) *ypb.BrowserTransformAdapterStatus {
	if a == nil {
		return &ypb.BrowserTransformAdapterStatus{ProtocolVersion: browserTransformAdapterProtocolVersion}
	}
	token := ""
	if includeToken {
		token = a.token
	}
	return &ypb.BrowserTransformAdapterStatus{
		Running:             a.running.Load(),
		Endpoint:            a.endpoint,
		Token:               token,
		DeviceId:            a.runtime.deviceID,
		ProfileId:           a.runtime.profileID,
		ProfileName:         a.runtime.profileName,
		RequestEnabled:      a.runtime.requestEnabled,
		ResponseEnabled:     a.runtime.responseEnabled,
		StartedAt:           a.startedAt.UnixMilli(),
		RequestCount:        a.requestCount.Load(),
		FailureCount:        a.failureCount.Load(),
		BypassCount:         a.bypassCount.Load(),
		LastUsedAt:          a.lastUsedAt.Load(),
		LastError:           a.errorMessage(),
		Port:                int32(a.port),
		Host:                a.host,
		TimeoutMilliseconds: a.timeout.Milliseconds(),
		ProtocolVersion:     browserTransformAdapterProtocolVersion,
		Methods:             append([]string(nil), a.runtime.methods...),
		UrlPattern:          a.runtime.urlPattern,
		Origin:              a.runtime.origin,
	}
}

func (a *browserTransformExternalAdapter) Close(ctx context.Context) error {
	if a == nil || a.server == nil {
		return nil
	}
	a.running.Store(false)
	if err := a.server.Shutdown(ctx); err != nil {
		closeErr := a.server.Close()
		if closeErr != nil {
			return fmt.Errorf("shutdown browser transform adapter: %v; close: %w", err, closeErr)
		}
		return err
	}
	return nil
}

func (s *Server) StartBrowserTransformAdapter(
	ctx context.Context,
	req *ypb.BrowserTransformAdapterStartRequest,
) (*ypb.BrowserTransformAdapterStatus, error) {
	if s == nil || s.browserBridge == nil {
		return nil, errors.New("browser transform adapter is unavailable: extension bridge is not running")
	}
	if req == nil {
		return nil, errors.New("browser transform adapter request is required")
	}
	host, err := normalizeBrowserTransformAdapterHost(req.GetHost())
	if err != nil {
		return nil, err
	}
	port := int(req.GetPort())
	if port < 0 || port > 65535 {
		return nil, errors.New("browser transform adapter port is out of range")
	}
	timeout := browserTransformAdapterTimeout(req.GetTimeoutMilliseconds())

	s.browserTransformAdapterMu.Lock()
	current := s.browserTransformAdapter
	if current != nil && current.running.Load() &&
		current.runtime.deviceID == strings.TrimSpace(req.GetDeviceId()) &&
		current.runtime.profileID == strings.TrimSpace(req.GetProfileId()) &&
		current.host == host && (port == 0 || current.port == port) && current.timeout == timeout {
		status := current.status(true)
		s.browserTransformAdapterMu.Unlock()
		return status, nil
	}
	if current != nil && current.running.Load() && port != 0 && current.host == host && current.port == port {
		s.browserTransformAdapterMu.Unlock()
		return nil, errors.New("requested port belongs to the active adapter; stop it first or use automatic port selection")
	}
	adapter, err := newBrowserTransformExternalAdapter(
		ctx,
		s.browserBridge,
		req.GetDeviceId(),
		req.GetProfileId(),
		host,
		port,
		timeout,
	)
	if err != nil {
		s.browserTransformAdapterMu.Unlock()
		return nil, err
	}
	s.browserTransformAdapter = adapter
	s.browserTransformAdapterMu.Unlock()

	if current != nil {
		closeContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = current.Close(closeContext)
		cancel()
	}
	return adapter.status(true), nil
}

func (s *Server) GetBrowserTransformAdapterStatus(
	_ context.Context,
	_ *ypb.Empty,
) (*ypb.BrowserTransformAdapterStatus, error) {
	if s == nil {
		return (*browserTransformExternalAdapter)(nil).status(false), nil
	}
	s.browserTransformAdapterMu.Lock()
	adapter := s.browserTransformAdapter
	status := adapter.status(true)
	s.browserTransformAdapterMu.Unlock()
	return status, nil
}

func (s *Server) StopBrowserTransformAdapter(
	_ context.Context,
	_ *ypb.Empty,
) (*ypb.BrowserTransformAdapterStatus, error) {
	if s == nil {
		return (*browserTransformExternalAdapter)(nil).status(false), nil
	}
	if err := s.closeBrowserTransformAdapter(); err != nil {
		return nil, err
	}
	return (*browserTransformExternalAdapter)(nil).status(false), nil
}

func (s *Server) closeBrowserTransformAdapter() error {
	if s == nil {
		return nil
	}
	s.browserTransformAdapterMu.Lock()
	adapter := s.browserTransformAdapter
	s.browserTransformAdapter = nil
	s.browserTransformAdapterMu.Unlock()
	if adapter == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return adapter.Close(ctx)
}
