package browser

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/consts"
)

const (
	defaultExtensionBridgeHost     = "127.0.0.1"
	extensionBridgePath            = "/extension"
	extensionBridgePairingPath     = "/pairing"
	extensionBridgeProtocolVersion = 2
	extensionBridgeMaxMessageBytes = 16 << 20
	extensionBridgeChunkThreshold  = 512 << 10
	extensionBridgeChunkBytes      = 256 << 10
	extensionBridgeMaxTransfers    = 8
	extensionBridgeEventBuffer     = 128
)

var extensionBridgeEngineCapabilities = []string{
	"yakit.web_fuzzer.open",
	"yakit.poc.generate",
	"yakit.browser_request.prepare_analysis",
	"yakit.browser_authorization.task",
	"yakit.browser_authorization.open",
}

type ExtensionBridgeError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type ExtensionBridgeEnvelope struct {
	ID                string                            `json:"id,omitempty"`
	Type              string                            `json:"type"`
	Method            string                            `json:"method,omitempty"`
	Params            json.RawMessage                   `json:"params,omitempty"`
	Result            json.RawMessage                   `json:"result,omitempty"`
	Error             *ExtensionBridgeError             `json:"error,omitempty"`
	Token             string                            `json:"token,omitempty"`
	Client            string                            `json:"client,omitempty"`
	Version           string                            `json:"version,omitempty"`
	ProtocolVersion   int                               `json:"protocolVersion,omitempty"`
	Capabilities      []string                          `json:"capabilities,omitempty"`
	CapabilityCatalog *ExtensionBridgeCapabilityCatalog `json:"capabilityCatalog,omitempty"`
	SessionID         string                            `json:"sessionId,omitempty"`
	TaskID            string                            `json:"taskId,omitempty"`
	GrantID           string                            `json:"grantId,omitempty"`
	InstallationID    string                            `json:"installationId,omitempty"`
	EngineInstanceID  string                            `json:"engineInstanceId,omitempty"`
	ConnectionID      string                            `json:"connectionId,omitempty"`
	ResumeSessionID   string                            `json:"resumeSessionId,omitempty"`
	Resumed           bool                              `json:"resumed,omitempty"`
	Sequence          uint64                            `json:"sequence,omitempty"`
	Timestamp         int64                             `json:"timestamp,omitempty"`
	ReplyTimestamp    int64                             `json:"replyTimestamp,omitempty"`
	Challenge         string                            `json:"challenge,omitempty"`
	Signature         string                            `json:"signature,omitempty"`
	EngineIdentityID  string                            `json:"engineIdentityId,omitempty"`
	PublicKey         *ExtensionBridgeJWK               `json:"publicKey,omitempty"`
	TransferID        string                            `json:"transferId,omitempty"`
	Index             int                               `json:"index"`
	Total             int                               `json:"total"`
	Data              string                            `json:"data,omitempty"`
	OriginalBytes     int                               `json:"originalBytes,omitempty"`
}

type extensionBridgeChunkAssembly struct {
	createdAt     time.Time
	total         int
	originalBytes int
	parts         [][]byte
	received      int
}

type extensionBridgeClientRequest struct {
	connection *websocket.Conn
	cancel     context.CancelFunc
}

type extensionBridgePendingCall struct {
	connection *websocket.Conn
	response   chan ExtensionBridgeEnvelope
}

type ExtensionBridgeConnection struct {
	DeviceID          string                            `json:"deviceId"`
	InstallationID    string                            `json:"installationId"`
	Client            string                            `json:"client"`
	ClientVersion     string                            `json:"clientVersion"`
	Capabilities      []string                          `json:"capabilities"`
	CapabilityCatalog *ExtensionBridgeCapabilityCatalog `json:"capabilityCatalog,omitempty"`
	SessionID         string                            `json:"sessionId"`
	ConnectionID      string                            `json:"connectionId"`
	TaskID            string                            `json:"taskId,omitempty"`
	GrantID           string                            `json:"grantId,omitempty"`
	ConnectedAt       int64                             `json:"connectedAt"`
}

type extensionBridgeConnectedClient struct {
	connection *websocket.Conn
	status     ExtensionBridgeConnection
}

type ExtensionBridgeServer struct {
	tokenMu          sync.RWMutex
	token            string
	addr             string
	engineInstanceID string
	manager          *ExtensionBridgeManager

	server   *http.Server
	listener net.Listener
	closed   atomic.Bool
	sequence atomic.Uint64

	clientMu           sync.RWMutex
	client             *websocket.Conn
	clientName         string
	clientVersion      string
	clientCapabilities []string
	sessionID          string
	installationID     string
	connectionID       string
	taskID             string
	grantID            string
	lastInstallationID string
	lastSessionID      string
	connectedAt        time.Time
	writeMu            sync.Mutex
	managedClients     map[string]*extensionBridgeConnectedClient
	managedSessions    map[string]string

	catalogMu          sync.RWMutex
	capabilityCatalogs map[string]*ExtensionBridgeCapabilityCatalog

	pendingMu sync.Mutex
	pending   map[string]extensionBridgePendingCall
	events    chan ExtensionBridgeEnvelope

	clientRequestMu sync.Mutex
	clientRequests  map[string]extensionBridgeClientRequest
}

// newLegacyExtensionBridgeServer exists only for the protocol-v2 test suite.
// Production runtimes are owned by ExtensionBridgeManager and use protocol v3.
func newLegacyExtensionBridgeServer(port int, token string) (*ExtensionBridgeServer, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("extension bridge token is required")
	}
	return newExtensionBridgeServer(port, token, nil)
}

func newManagedExtensionBridgeServer(port int, manager *ExtensionBridgeManager) (*ExtensionBridgeServer, error) {
	if manager == nil {
		return nil, errors.New("extension bridge manager is required")
	}
	return newExtensionBridgeServer(port, "", manager)
}

func newExtensionBridgeServer(port int, token string, manager *ExtensionBridgeManager) (*ExtensionBridgeServer, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("extension bridge port out of range: %d", port)
	}
	engineInstanceID := ""
	if manager != nil {
		engineInstanceID = manager.engineInstanceID
	}
	if engineInstanceID == "" {
		var err error
		engineInstanceID, err = newExtensionBridgeID("engine")
		if err != nil {
			return nil, fmt.Errorf("create extension bridge engine identity: %w", err)
		}
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(defaultExtensionBridgeHost, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("listen for extension bridge: %w", err)
	}

	bridge := &ExtensionBridgeServer{
		token:              token,
		addr:               listener.Addr().String(),
		listener:           listener,
		engineInstanceID:   engineInstanceID,
		manager:            manager,
		pending:            make(map[string]extensionBridgePendingCall),
		events:             make(chan ExtensionBridgeEnvelope, extensionBridgeEventBuffer),
		clientRequests:     make(map[string]extensionBridgeClientRequest),
		managedClients:     make(map[string]*extensionBridgeConnectedClient),
		managedSessions:    make(map[string]string),
		capabilityCatalogs: make(map[string]*ExtensionBridgeCapabilityCatalog),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(extensionBridgePath, bridge.handleWebSocket)
	if manager != nil {
		mux.HandleFunc(extensionBridgePairingPath, bridge.handlePairingWebSocket)
	}
	bridge.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       45 * time.Second,
	}

	go func() {
		if serveErr := bridge.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			bridge.closed.Store(true)
		}
	}()
	return bridge, nil
}

func (s *ExtensionBridgeServer) URL() string {
	if s == nil {
		return ""
	}
	return "ws://" + s.addr + extensionBridgePath
}

func (s *ExtensionBridgeServer) Status() map[string]interface{} {
	if s == nil {
		return map[string]interface{}{"running": false, "connected": false}
	}
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	status := map[string]interface{}{"running": !s.closed.Load(), "url": s.URL()}
	if s.manager != nil {
		engineIdentityID, _ := s.manager.EngineIdentity()
		connections := s.connectionsLocked()
		status["connected"] = len(connections) > 0
		status["connections"] = connections
		status["protocolVersion"] = managedExtensionBridgeProtocolVersion
		status["engineIdentityId"] = engineIdentityID
		status["engineInstanceId"] = s.engineInstanceID
		status["engineCapabilities"] = append([]string(nil), extensionBridgeEngineCapabilities...)
		return status
	}
	status["connected"] = s.client != nil
	if s.client != nil {
		status["client"] = s.clientName
		status["clientVersion"] = s.clientVersion
		if s.manager == nil {
			status["protocolVersion"] = extensionBridgeProtocolVersion
		}
		status["capabilities"] = append([]string(nil), s.clientCapabilities...)
		status["engineCapabilities"] = append([]string(nil), extensionBridgeEngineCapabilities...)
		status["sessionId"] = s.sessionID
		status["engineInstanceId"] = s.engineInstanceID
		status["connectionId"] = s.connectionID
		status["installationId"] = s.installationID
		status["taskId"] = s.taskID
		status["grantId"] = s.grantID
		status["connectedAt"] = s.connectedAt.UnixMilli()
	}
	return status
}

func (s *ExtensionBridgeServer) Connections() []ExtensionBridgeConnection {
	if s == nil {
		return nil
	}
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	return s.connectionsLocked()
}

func (s *ExtensionBridgeServer) connectionsLocked() []ExtensionBridgeConnection {
	connections := make([]ExtensionBridgeConnection, 0, len(s.managedClients))
	for _, client := range s.managedClients {
		connection := client.status
		connection.Capabilities = append([]string(nil), connection.Capabilities...)
		connection.CapabilityCatalog = cloneExtensionBridgeCapabilityCatalog(connection.CapabilityCatalog)
		connections = append(connections, connection)
	}
	sort.Slice(connections, func(i, j int) bool {
		if connections[i].ConnectedAt == connections[j].ConnectedAt {
			return connections[i].DeviceID < connections[j].DeviceID
		}
		return connections[i].ConnectedAt < connections[j].ConnectedAt
	})
	return connections
}

func (s *ExtensionBridgeServer) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	if s == nil || s.closed.Load() {
		return nil, errors.New("extension bridge is not running")
	}
	if s.manager != nil {
		s.clientMu.RLock()
		var selected *extensionBridgeConnectedClient
		for _, client := range s.managedClients {
			if selected != nil {
				s.clientMu.RUnlock()
				return nil, errors.New("multiple browser extensions are connected; device id is required")
			}
			selected = client
		}
		s.clientMu.RUnlock()
		if selected == nil {
			return nil, errors.New("browser extension is not connected")
		}
		if selected.status.CapabilityCatalog == nil {
			return nil, errors.New("connected browser extension does not advertise a signed capability schema")
		}
		if err := selected.status.CapabilityCatalog.ValidateCapabilityParams(method, params); err != nil {
			return nil, err
		}
		return s.callConnection(ctx, selected.connection, method, params)
	}
	s.clientMu.RLock()
	client := s.client
	s.clientMu.RUnlock()
	if client == nil {
		return nil, errors.New("browser extension is not connected")
	}
	return s.callConnection(ctx, client, method, params)
}

func (s *ExtensionBridgeServer) CallDevice(ctx context.Context, deviceID, method string, params interface{}) (json.RawMessage, error) {
	if s == nil || s.closed.Load() {
		return nil, errors.New("extension bridge is not running")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, errors.New("browser extension device id is required")
	}
	s.clientMu.RLock()
	client := s.managedClients[deviceID]
	s.clientMu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("browser extension device %q is not connected", deviceID)
	}
	if client.status.CapabilityCatalog == nil {
		return nil, errors.New("connected browser extension does not advertise a signed capability schema")
	}
	if err := client.status.CapabilityCatalog.ValidateCapabilityParams(method, params); err != nil {
		return nil, err
	}
	return s.callConnection(ctx, client.connection, method, params)
}

func (s *ExtensionBridgeServer) callConnection(ctx context.Context, connection *websocket.Conn, method string, params interface{}) (json.RawMessage, error) {
	method = strings.TrimSpace(method)
	if method == "" {
		return nil, errors.New("extension bridge method is required")
	}
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal extension bridge params: %w", err)
	}
	id := fmt.Sprintf("yak-%d", s.sequence.Add(1))
	responseChannel := make(chan ExtensionBridgeEnvelope, 1)
	s.pendingMu.Lock()
	s.pending[id] = extensionBridgePendingCall{connection: connection, response: responseChannel}
	s.pendingMu.Unlock()
	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
	}()

	message := ExtensionBridgeEnvelope{ID: id, Type: "request", Method: method, Params: paramsRaw}
	write := func(message ExtensionBridgeEnvelope) error { return s.writeConnectionJSON(connection, message) }
	if err := write(message); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		_ = write(ExtensionBridgeEnvelope{ID: id, Type: "cancel"})
		return nil, fmt.Errorf("extension bridge call %q: %w", method, ctx.Err())
	case response := <-responseChannel:
		if response.Error != nil {
			return nil, fmt.Errorf("extension bridge call %q failed (%s): %s", method, response.Error.Code, response.Error.Message)
		}
		return response.Result, nil
	}
}

func (s *ExtensionBridgeServer) WaitEvent(ctx context.Context) (map[string]interface{}, error) {
	if s == nil || s.closed.Load() {
		return nil, errors.New("extension bridge is not running")
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for extension bridge event: %w", ctx.Err())
	case event := <-s.events:
		var params interface{}
		if len(event.Params) > 0 && string(event.Params) != "null" {
			if err := json.Unmarshal(event.Params, &params); err != nil {
				params = string(event.Params)
			}
		}
		return map[string]interface{}{
			"id":     event.ID,
			"method": event.Method,
			"params": params,
		}, nil
	}
}

func (s *ExtensionBridgeServer) Close() error {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.clientMu.Lock()
	clients := make([]*websocket.Conn, 0, len(s.managedClients)+1)
	if s.client != nil {
		clients = append(clients, s.client)
	}
	for _, managed := range s.managedClients {
		clients = append(clients, managed.connection)
	}
	s.client = nil
	s.managedClients = make(map[string]*extensionBridgeConnectedClient)
	s.clientMu.Unlock()
	for _, client := range clients {
		_ = client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bridge stopped"), time.Now().Add(time.Second))
		_ = client.Close()
	}
	s.failPendingCalls(nil, "extension bridge stopped")
	s.cancelClientRequests()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *ExtensionBridgeServer) disconnectInstallation(installationID string) {
	if s == nil || strings.TrimSpace(installationID) == "" {
		return
	}
	s.clientMu.Lock()
	clients := make([]*websocket.Conn, 0, 1)
	if s.manager != nil {
		delete(s.managedSessions, installationID)
		for _, managed := range s.managedClients {
			if managed.status.InstallationID == installationID {
				clients = append(clients, managed.connection)
			}
		}
	} else if s.installationID == installationID && s.client != nil {
		clients = append(clients, s.client)
	}
	s.clientMu.Unlock()
	for _, client := range clients {
		_ = client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "browser extension pairing revoked"), time.Now().Add(time.Second))
		_ = client.Close()
	}
}

func (s *ExtensionBridgeServer) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	upgrader := websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin: func(r *http.Request) bool {
			_, ok := NormalizeBrowserExtensionOrigin(r.Header.Get("Origin"))
			return ok
		},
	}
	connection, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}
	defer connection.Close()

	connection.SetReadLimit(2 << 20)
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	protocolVersion := extensionBridgeProtocolVersion
	engineIdentityID := ""
	var hello ExtensionBridgeEnvelope
	var device *ExtensionBridgeDevice
	if s.manager != nil {
		origin, _ := NormalizeBrowserExtensionOrigin(request.Header.Get("Origin"))
		managedHello, managedDevice, authErr := s.authenticateManagedConnection(connection, origin)
		if authErr != nil {
			s.rejectHandshake(connection, authErr.Error())
			return
		}
		hello, device = managedHello, managedDevice
		protocolVersion = managedExtensionBridgeProtocolVersion
		engineIdentityID, _ = s.manager.EngineIdentity()
	} else {
		if err := readStrictExtensionBridgeJSON(connection, &hello); err != nil || hello.Type != "hello" || hello.ProtocolVersion != extensionBridgeProtocolVersion || strings.TrimSpace(hello.InstallationID) == "" || !s.tokenMatches(hello.Token) {
			s.rejectHandshake(connection, "invalid extension bridge handshake")
			return
		}
	}
	_ = connection.SetReadDeadline(time.Time{})
	connection.SetReadLimit(16 << 20)

	connectionID, err := newExtensionBridgeID("connection")
	if err != nil {
		return
	}
	sessionID, err := newExtensionBridgeID("session")
	if err != nil {
		return
	}
	now := time.Now()
	deviceID := ""
	var previous *websocket.Conn
	resumed := false
	s.clientMu.Lock()
	if s.manager != nil && device != nil {
		deviceID = device.ID
		lastSessionID := s.managedSessions[hello.InstallationID]
		resumed = hello.ResumeSessionID != "" && hello.ResumeSessionID == lastSessionID
		if resumed {
			sessionID = lastSessionID
		}
		if existing := s.managedClients[deviceID]; existing != nil {
			previous = existing.connection
		}
		s.managedClients[deviceID] = &extensionBridgeConnectedClient{
			connection: connection,
			status: ExtensionBridgeConnection{
				DeviceID: deviceID, InstallationID: hello.InstallationID, Client: hello.Client,
				ClientVersion: hello.Version, Capabilities: append([]string(nil), hello.Capabilities...),
				CapabilityCatalog: hello.CapabilityCatalog,
				SessionID:         sessionID, ConnectionID: connectionID, TaskID: hello.TaskID,
				GrantID: hello.GrantID, ConnectedAt: now.UnixMilli(),
			},
		}
		s.managedSessions[hello.InstallationID] = sessionID
	} else {
		previous = s.client
		resumed = hello.ResumeSessionID != "" && hello.ResumeSessionID == s.lastSessionID && hello.InstallationID == s.lastInstallationID
		if resumed {
			sessionID = s.lastSessionID
		}
		s.client = connection
		s.clientName = hello.Client
		s.clientVersion = hello.Version
		s.clientCapabilities = append([]string(nil), hello.Capabilities...)
		s.sessionID = sessionID
		s.installationID = hello.InstallationID
		s.connectionID = connectionID
		s.taskID = hello.TaskID
		s.grantID = hello.GrantID
		s.lastInstallationID = hello.InstallationID
		s.lastSessionID = sessionID
		s.connectedAt = now
	}
	s.clientMu.Unlock()
	if previous != nil && previous != connection {
		_ = previous.Close()
	}
	if err := s.writeConnectionJSON(connection, ExtensionBridgeEnvelope{
		Type: "hello_ack", Version: consts.GetYakVersion(), ProtocolVersion: protocolVersion,
		Capabilities: append([]string(nil), extensionBridgeEngineCapabilities...), SessionID: sessionID,
		EngineInstanceID: s.engineInstanceID, EngineIdentityID: engineIdentityID, ConnectionID: connectionID, Resumed: resumed,
		TaskID: hello.TaskID, GrantID: hello.GrantID,
	}); err != nil {
		return
	}
	if s.manager != nil && device != nil {
		s.manager.markDeviceSeen(device.ID, hello.Version)
		s.manager.notifyStateChange("device_connected")
	}

	defer func() {
		disconnected := false
		s.clientMu.Lock()
		if s.manager != nil {
			if current := s.managedClients[deviceID]; current != nil && current.connection == connection {
				delete(s.managedClients, deviceID)
				disconnected = true
			}
		} else if s.client == connection {
			s.client = nil
			s.clientName = ""
			s.clientVersion = ""
			s.clientCapabilities = nil
			s.sessionID = ""
			s.installationID = ""
			s.connectionID = ""
			s.taskID = ""
			s.grantID = ""
			s.connectedAt = time.Time{}
			disconnected = true
		}
		s.clientMu.Unlock()
		s.cancelClientRequestsFor(connection)
		s.failPendingCalls(connection, "browser extension disconnected")
		if s.manager != nil && disconnected {
			s.manager.notifyStateChange("device_disconnected")
		}
	}()

	chunkAssemblies := make(map[string]*extensionBridgeChunkAssembly)
	for {
		var message ExtensionBridgeEnvelope
		if err := s.readConnectionJSON(connection, chunkAssemblies, &message); err != nil {
			return
		}
		s.clientMu.RLock()
		isCurrentClient := s.client == connection
		if s.manager != nil {
			current := s.managedClients[deviceID]
			isCurrentClient = current != nil && current.connection == connection
		}
		s.clientMu.RUnlock()
		if !isCurrentClient {
			return
		}
		switch message.Type {
		case "response":
			s.deliverResponse(connection, message)
		case "ping":
			_ = s.writeConnectionJSON(connection, ExtensionBridgeEnvelope{
				ID: message.ID, Type: "pong", Sequence: message.Sequence,
				Timestamp: message.Timestamp, ReplyTimestamp: time.Now().UnixMilli(),
			})
		case "event":
			if strings.TrimSpace(message.Method) != "" {
				s.enqueueEvent(message)
			}
		case "request":
			s.startClientRequest(connection, deviceID, message)
		case "cancel":
			s.cancelClientRequest(connection, message.ID)
		}
	}
}

func (s *ExtensionBridgeServer) deliverResponse(connection *websocket.Conn, message ExtensionBridgeEnvelope) {
	s.pendingMu.Lock()
	pending, exists := s.pending[message.ID]
	s.pendingMu.Unlock()
	if !exists || (pending.connection != nil && pending.connection != connection) {
		return
	}
	select {
	case pending.response <- message:
	default:
	}
}

func (s *ExtensionBridgeServer) rejectHandshake(connection *websocket.Conn, message string) {
	_ = connection.WriteJSON(ExtensionBridgeEnvelope{
		Type:  "response",
		Error: &ExtensionBridgeError{Code: "unauthorized", Message: message},
	})
	_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "unauthorized"), time.Now().Add(time.Second))
}

func (s *ExtensionBridgeServer) cacheCapabilityCatalog(
	catalog *ExtensionBridgeCapabilityCatalog,
) *ExtensionBridgeCapabilityCatalog {
	if catalog == nil {
		return nil
	}
	s.catalogMu.RLock()
	cached := s.capabilityCatalogs[catalog.Hash]
	s.catalogMu.RUnlock()
	if cached != nil {
		return cached
	}
	clone := cloneExtensionBridgeCapabilityCatalog(catalog)
	s.catalogMu.Lock()
	if cached = s.capabilityCatalogs[catalog.Hash]; cached == nil {
		if len(s.capabilityCatalogs) >= 32 {
			for hash := range s.capabilityCatalogs {
				delete(s.capabilityCatalogs, hash)
				break
			}
		}
		s.capabilityCatalogs[catalog.Hash] = clone
		cached = clone
	}
	s.catalogMu.Unlock()
	return cached
}

func (s *ExtensionBridgeServer) authenticateManagedConnection(connection *websocket.Conn, origin string) (ExtensionBridgeEnvelope, *ExtensionBridgeDevice, error) {
	var empty ExtensionBridgeEnvelope
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return empty, nil, err
	}
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes)
	engineIdentityID, publicKey := s.manager.EngineIdentity()
	timestamp := time.Now().UnixMilli()
	signature, err := s.manager.Sign(managedEngineChallengePayload(engineIdentityID, s.engineInstanceID, challenge, timestamp))
	if err != nil {
		return empty, nil, err
	}
	if err := connection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "challenge", ProtocolVersion: managedExtensionBridgeProtocolVersion,
		Challenge: challenge, Signature: signature, Timestamp: timestamp,
		EngineIdentityID: engineIdentityID, EngineInstanceID: s.engineInstanceID, PublicKey: &publicKey,
	}); err != nil {
		return empty, nil, err
	}
	var hello ExtensionBridgeEnvelope
	if err := readStrictExtensionBridgeJSON(connection, &hello); err != nil {
		return empty, nil, err
	}
	if hello.Type != "auth" || hello.ProtocolVersion != managedExtensionBridgeProtocolVersion || hello.Challenge != challenge || strings.TrimSpace(hello.InstallationID) == "" {
		return empty, nil, errors.New("invalid managed extension bridge handshake")
	}
	payload := managedClientAuthPayload(origin, engineIdentityID, s.engineInstanceID, challenge, hello)
	device, err := s.manager.authenticateDevice(hello.InstallationID, origin, payload, hello.Signature)
	if err != nil {
		return empty, nil, err
	}
	if err := validateExtensionBridgeCapabilityCatalog(hello.CapabilityCatalog, hello.Capabilities); err != nil {
		return empty, nil, err
	}
	hello.CapabilityCatalog = s.cacheCapabilityCatalog(hello.CapabilityCatalog)
	return hello, device, nil
}

type extensionBridgePairingEnvelope struct {
	Type             string              `json:"type"`
	ProtocolVersion  int                 `json:"protocolVersion,omitempty"`
	RequestID        string              `json:"requestId,omitempty"`
	InstallationID   string              `json:"installationId,omitempty"`
	Client           string              `json:"client,omitempty"`
	Version          string              `json:"version,omitempty"`
	Nonce            string              `json:"nonce,omitempty"`
	ServerNonce      string              `json:"serverNonce,omitempty"`
	PublicKey        *ExtensionBridgeJWK `json:"publicKey,omitempty"`
	EngineIdentityID string              `json:"engineIdentityId,omitempty"`
	Code             string              `json:"code,omitempty"`
	ExpiresAt        int64               `json:"expiresAt,omitempty"`
	DeviceID         string              `json:"deviceId,omitempty"`
	Message          string              `json:"message,omitempty"`
}

func (s *ExtensionBridgeServer) handlePairingWebSocket(writer http.ResponseWriter, request *http.Request) {
	if s.manager == nil {
		http.NotFound(writer, request)
		return
	}
	origin, ok := NormalizeBrowserExtensionOrigin(request.Header.Get("Origin"))
	if !ok {
		http.Error(writer, "browser extension origin required", http.StatusForbidden)
		return
	}
	upgrader := websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin: func(r *http.Request) bool {
			_, valid := NormalizeBrowserExtensionOrigin(r.Header.Get("Origin"))
			return valid
		},
	}
	connection, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	connection.SetReadLimit(32 << 10)
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	var message extensionBridgePairingEnvelope
	if err := readStrictExtensionBridgeJSON(connection, &message); err != nil || message.Type != "pair_request" || message.ProtocolVersion != managedExtensionBridgeProtocolVersion || message.PublicKey == nil {
		_ = connection.WriteJSON(extensionBridgePairingEnvelope{Type: "pair_error", Message: "invalid browser extension pairing request"})
		return
	}
	pending, err := s.manager.beginPairing(origin, extensionBridgePairingInput{
		InstallationID: message.InstallationID,
		Client:         message.Client,
		ClientVersion:  message.Version,
		Nonce:          message.Nonce,
		PublicKey:      *message.PublicKey,
	})
	if err != nil {
		_ = connection.WriteJSON(extensionBridgePairingEnvelope{Type: "pair_error", Message: err.Error()})
		return
	}
	engineIdentityID, enginePublicKey := s.manager.EngineIdentity()
	if err := connection.WriteJSON(extensionBridgePairingEnvelope{
		Type: "pair_pending", ProtocolVersion: managedExtensionBridgeProtocolVersion,
		RequestID: pending.request.ID, ServerNonce: pending.serverNonce, Code: pending.request.Code,
		ExpiresAt: pending.request.ExpiresAt, EngineIdentityID: engineIdentityID, PublicKey: &enginePublicKey,
	}); err != nil {
		s.manager.cancelPairing(pending.request.ID)
		return
	}
	_ = connection.SetReadDeadline(time.Time{})
	wait := time.Until(time.UnixMilli(pending.request.ExpiresAt))
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case decision := <-pending.decision:
		if decision.approved && decision.device != nil {
			_ = connection.WriteJSON(extensionBridgePairingEnvelope{
				Type: "pair_approved", RequestID: pending.request.ID, DeviceID: decision.device.ID,
				EngineIdentityID: engineIdentityID, PublicKey: &enginePublicKey,
			})
			return
		}
		_ = connection.WriteJSON(extensionBridgePairingEnvelope{Type: "pair_rejected", RequestID: pending.request.ID, Message: decision.message})
	case <-timer.C:
		s.manager.cancelPairing(pending.request.ID)
		_ = connection.WriteJSON(extensionBridgePairingEnvelope{Type: "pair_expired", RequestID: pending.request.ID, Message: "Pairing request expired"})
	case <-request.Context().Done():
		s.manager.cancelPairing(pending.request.ID)
	}
}

func (s *ExtensionBridgeServer) startClientRequest(connection *websocket.Conn, deviceID string, message ExtensionBridgeEnvelope) {
	if strings.TrimSpace(message.ID) == "" || strings.TrimSpace(message.Method) == "" {
		_ = s.writeConnectionJSON(connection, ExtensionBridgeEnvelope{ID: message.ID, Type: "response", Error: &ExtensionBridgeError{Code: "invalid_request", Message: "extension request requires id and method"}})
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.clientRequestMu.Lock()
	if active, exists := s.clientRequests[message.ID]; exists {
		if active.connection == connection {
			s.clientRequestMu.Unlock()
			cancel()
			_ = s.writeConnectionJSON(connection, ExtensionBridgeEnvelope{ID: message.ID, Type: "response", Error: &ExtensionBridgeError{Code: "duplicate_request_id", Message: "extension request id is already active"}})
			return
		}
		active.cancel()
	}
	s.clientRequests[message.ID] = extensionBridgeClientRequest{connection: connection, cancel: cancel}
	s.clientRequestMu.Unlock()
	go func() {
		defer func() {
			s.clientRequestMu.Lock()
			if active, exists := s.clientRequests[message.ID]; exists && active.connection == connection {
				delete(s.clientRequests, message.ID)
			}
			s.clientRequestMu.Unlock()
			cancel()
		}()
		var result interface{}
		var bridgeErr *ExtensionBridgeError
		if s.manager == nil && message.Method == "system.pairing.rotate" {
			result, bridgeErr = s.rotatePairingToken(message.Params)
		} else {
			result, bridgeErr = s.handleExtensionClientRequest(ctx, deviceID, message.Method, message.Params)
		}
		response := ExtensionBridgeEnvelope{ID: message.ID, Type: "response", Error: bridgeErr}
		if bridgeErr == nil {
			encoded, err := json.Marshal(result)
			if err != nil {
				response.Error = &ExtensionBridgeError{Code: "response_encode_failed", Message: err.Error()}
			} else {
				response.Result = encoded
			}
		}
		_ = s.writeConnectionJSON(connection, response)
	}()
}

func (s *ExtensionBridgeServer) cancelClientRequest(connection *websocket.Conn, id string) {
	s.clientRequestMu.Lock()
	request, exists := s.clientRequests[id]
	s.clientRequestMu.Unlock()
	if exists && request.connection == connection {
		request.cancel()
	}
}

func (s *ExtensionBridgeServer) cancelClientRequestsFor(connection *websocket.Conn) {
	s.clientRequestMu.Lock()
	requests := make([]context.CancelFunc, 0)
	for id, request := range s.clientRequests {
		if request.connection != connection {
			continue
		}
		delete(s.clientRequests, id)
		requests = append(requests, request.cancel)
	}
	s.clientRequestMu.Unlock()
	for _, cancel := range requests {
		cancel()
	}
}

func (s *ExtensionBridgeServer) cancelClientRequests() {
	s.clientRequestMu.Lock()
	requests := s.clientRequests
	s.clientRequests = make(map[string]extensionBridgeClientRequest)
	s.clientRequestMu.Unlock()
	for _, request := range requests {
		request.cancel()
	}
}

func (s *ExtensionBridgeServer) failPendingCalls(connection *websocket.Conn, message string) {
	s.pendingMu.Lock()
	pending := make([]extensionBridgePendingCall, 0)
	for _, call := range s.pending {
		if connection == nil || call.connection == connection {
			pending = append(pending, call)
		}
	}
	s.pendingMu.Unlock()
	for _, call := range pending {
		select {
		case call.response <- ExtensionBridgeEnvelope{Type: "response", Error: &ExtensionBridgeError{Code: "disconnected", Message: message}}:
		default:
		}
	}
}

func (s *ExtensionBridgeServer) enqueueEvent(event ExtensionBridgeEnvelope) {
	select {
	case s.events <- event:
		return
	default:
	}
	select {
	case <-s.events:
	default:
	}
	select {
	case s.events <- event:
	default:
	}
}

func (s *ExtensionBridgeServer) writeJSON(message ExtensionBridgeEnvelope) error {
	s.clientMu.RLock()
	client := s.client
	s.clientMu.RUnlock()
	if client == nil {
		return errors.New("browser extension is not connected")
	}
	return s.writeConnectionJSON(client, message)
}

func (s *ExtensionBridgeServer) readConnectionJSON(client *websocket.Conn, assemblies map[string]*extensionBridgeChunkAssembly, output *ExtensionBridgeEnvelope) error {
	for {
		_, payload, err := client.ReadMessage()
		if err != nil {
			return err
		}
		var message ExtensionBridgeEnvelope
		if err := decodeStrictExtensionBridgeJSON(payload, &message); err != nil {
			return fmt.Errorf("decode extension bridge message: %w", err)
		}
		if message.Type != "chunk" {
			*output = message
			return nil
		}
		now := time.Now()
		for id, assembly := range assemblies {
			if now.Sub(assembly.createdAt) > 30*time.Second {
				delete(assemblies, id)
			}
		}
		if message.TransferID == "" || len(message.TransferID) > 160 || message.Total < 1 || message.Total > extensionBridgeMaxMessageBytes/extensionBridgeChunkBytes || message.Index < 0 || message.Index >= message.Total || message.OriginalBytes < 1 || message.OriginalBytes > extensionBridgeMaxMessageBytes {
			return errors.New("invalid extension bridge chunk metadata")
		}
		part, err := base64.StdEncoding.DecodeString(message.Data)
		if err != nil || len(part) == 0 || len(part) > extensionBridgeChunkBytes || (message.Index < message.Total-1 && len(part) != extensionBridgeChunkBytes) {
			return errors.New("invalid extension bridge chunk data")
		}
		assembly := assemblies[message.TransferID]
		if assembly == nil {
			if len(assemblies) >= extensionBridgeMaxTransfers {
				return errors.New("too many concurrent extension bridge chunk transfers")
			}
			assembly = &extensionBridgeChunkAssembly{
				createdAt: now, total: message.Total, originalBytes: message.OriginalBytes,
				parts: make([][]byte, message.Total),
			}
			assemblies[message.TransferID] = assembly
		}
		if assembly.total != message.Total || assembly.originalBytes != message.OriginalBytes || assembly.parts[message.Index] != nil {
			delete(assemblies, message.TransferID)
			return errors.New("inconsistent extension bridge chunk transfer")
		}
		assembly.parts[message.Index] = part
		assembly.received++
		if assembly.received != assembly.total {
			continue
		}
		reassembled := make([]byte, assembly.originalBytes)
		offset := 0
		for _, item := range assembly.parts {
			if item == nil || offset+len(item) > len(reassembled) {
				delete(assemblies, message.TransferID)
				return errors.New("invalid extension bridge chunk assembly")
			}
			copy(reassembled[offset:], item)
			offset += len(item)
		}
		delete(assemblies, message.TransferID)
		if offset != len(reassembled) || !json.Valid(reassembled) {
			return errors.New("invalid reassembled extension bridge message")
		}
		if err := decodeStrictExtensionBridgeJSON(reassembled, output); err != nil {
			return fmt.Errorf("decode reassembled extension bridge message: %w", err)
		}
		if output.Type == "chunk" {
			return errors.New("nested extension bridge chunks are not allowed")
		}
		return nil
	}
}

func decodeStrictExtensionBridgeJSON(payload []byte, output interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("extension bridge message contains trailing data")
	}
	return nil
}

func readStrictExtensionBridgeJSON(connection *websocket.Conn, output interface{}) error {
	_, payload, err := connection.ReadMessage()
	if err != nil {
		return err
	}
	return decodeStrictExtensionBridgeJSON(payload, output)
}

func (s *ExtensionBridgeServer) writeConnectionJSON(client *websocket.Conn, message ExtensionBridgeEnvelope) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal extension bridge message: %w", err)
	}
	if len(payload) > extensionBridgeMaxMessageBytes {
		return fmt.Errorf("extension bridge message exceeds %d bytes", extensionBridgeMaxMessageBytes)
	}
	payloads := [][]byte{payload}
	if len(payload) > extensionBridgeChunkThreshold {
		transferID, err := newExtensionBridgeID("chunk")
		if err != nil {
			return fmt.Errorf("create extension bridge chunk identity: %w", err)
		}
		total := (len(payload) + extensionBridgeChunkBytes - 1) / extensionBridgeChunkBytes
		payloads = make([][]byte, 0, total)
		for index := 0; index < total; index++ {
			start := index * extensionBridgeChunkBytes
			end := start + extensionBridgeChunkBytes
			if end > len(payload) {
				end = len(payload)
			}
			chunk, marshalErr := json.Marshal(ExtensionBridgeEnvelope{
				Type: "chunk", TransferID: transferID, Index: index, Total: total,
				OriginalBytes: len(payload), Data: base64.StdEncoding.EncodeToString(payload[start:end]),
			})
			if marshalErr != nil {
				return fmt.Errorf("marshal extension bridge chunk: %w", marshalErr)
			}
			payloads = append(payloads, chunk)
		}
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for _, output := range payloads {
		if err := client.WriteMessage(websocket.TextMessage, output); err != nil {
			return fmt.Errorf("write extension bridge message: %w", err)
		}
	}
	return nil
}

func secureTokenEqual(left, right string) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func newExtensionBridgeID(prefix string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(random), nil
}

func (s *ExtensionBridgeServer) tokenMatches(value string) bool {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return secureTokenEqual(value, s.token)
}

func (s *ExtensionBridgeServer) rotatePairingToken(params json.RawMessage) (interface{}, *ExtensionBridgeError) {
	var input struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid pairing token payload"}
	}
	input.Token = strings.TrimSpace(input.Token)
	if len(input.Token) < 32 || len(input.Token) > 512 {
		return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "pairing token must contain 32 to 512 characters"}
	}
	s.tokenMu.Lock()
	s.token = input.Token
	s.tokenMu.Unlock()
	return map[string]interface{}{"rotated": true, "rotatedAt": time.Now().UnixMilli()}, nil
}

func ExtensionBridgeStatus() map[string]interface{} {
	if manager := ActiveExtensionBridgeManager(); manager != nil {
		snapshot := manager.Snapshot()
		encoded, _ := json.Marshal(snapshot)
		var status map[string]interface{}
		_ = json.Unmarshal(encoded, &status)
		return status
	}
	return map[string]interface{}{"running": false, "connected": false}
}

func CallExtensionBridge(method string, params interface{}, timeoutSeconds float64) (interface{}, error) {
	var server *ExtensionBridgeServer
	if manager := ActiveExtensionBridgeManager(); manager != nil {
		server = manager.currentServer()
	}
	if server == nil {
		return nil, errors.New("extension bridge is not running; start yak grpc with the browser extension bridge enabled")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds*float64(time.Second)))
	defer cancel()
	result, err := server.Call(ctx, method, params)
	if err != nil {
		return nil, err
	}
	var value interface{}
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(result, &value); err != nil {
		return string(result), nil
	}
	return value, nil
}

func WaitExtensionBridgeEvent(timeoutSeconds float64) (interface{}, error) {
	var server *ExtensionBridgeServer
	if manager := ActiveExtensionBridgeManager(); manager != nil {
		server = manager.currentServer()
	}
	if server == nil {
		return nil, errors.New("extension bridge is not running; start yak grpc with the browser extension bridge enabled")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds*float64(time.Second)))
	defer cancel()
	return server.WaitEvent(ctx)
}
