package browser

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	managedExtensionBridgeProtocolVersion = 3
	extensionBridgeIdentityVersion        = 1
	extensionPairingTTL                   = 2 * time.Minute
	extensionPairingMaxPending            = 8
	extensionPairingMaxPerMinute          = 5
)

type ExtensionBridgeJWK struct {
	KTY string `json:"kty"`
	CRV string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type ExtensionBridgeDevice struct {
	ID             string             `json:"id"`
	InstallationID string             `json:"installationId"`
	Name           string             `json:"name"`
	Client         string             `json:"client"`
	ClientVersion  string             `json:"clientVersion"`
	Origin         string             `json:"origin"`
	PublicKey      ExtensionBridgeJWK `json:"publicKey"`
	CreatedAt      int64              `json:"createdAt"`
	LastSeenAt     int64              `json:"lastSeenAt"`
}

type extensionBridgeIdentityState struct {
	Version          int                     `json:"version"`
	EngineIdentityID string                  `json:"engineIdentityId"`
	EnginePrivateKey string                  `json:"enginePrivateKey"`
	Devices          []ExtensionBridgeDevice `json:"devices"`
}

type ExtensionBridgeIdentityStore interface {
	Load() (*extensionBridgeIdentityState, error)
	Save(*extensionBridgeIdentityState) error
}

type ExtensionBridgeFileIdentityStore struct {
	mu   sync.Mutex
	path string
}

func NewExtensionBridgeFileIdentityStore(path string) *ExtensionBridgeFileIdentityStore {
	return &ExtensionBridgeFileIdentityStore{path: filepath.Clean(path)}
}

func (s *ExtensionBridgeFileIdentityStore) Load() (*extensionBridgeIdentityState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var state extensionBridgeIdentityState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode browser extension identity: %w", err)
	}
	return &state, nil
}

func (s *ExtensionBridgeFileIdentityStore) Save(state *extensionBridgeIdentityState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state == nil {
		return errors.New("browser extension identity is required")
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode browser extension identity: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create browser extension identity directory: %w", err)
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write browser extension identity: %w", err)
	}
	if err := os.Rename(temporary, s.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace browser extension identity: %w", err)
	}
	return nil
}

type ExtensionBridgePairingRequest struct {
	ID             string `json:"id"`
	InstallationID string `json:"installationId"`
	ExtensionID    string `json:"extensionId"`
	Client         string `json:"client"`
	ClientVersion  string `json:"clientVersion"`
	Origin         string `json:"origin"`
	Code           string `json:"code"`
	CreatedAt      int64  `json:"createdAt"`
	ExpiresAt      int64  `json:"expiresAt"`
}

type extensionBridgePairingInput struct {
	InstallationID string             `json:"installationId"`
	Client         string             `json:"client"`
	ClientVersion  string             `json:"clientVersion"`
	Nonce          string             `json:"nonce"`
	PublicKey      ExtensionBridgeJWK `json:"publicKey"`
}

type extensionBridgePairingDecision struct {
	approved bool
	message  string
	device   *ExtensionBridgeDevice
}

type extensionBridgePendingPairing struct {
	request     ExtensionBridgePairingRequest
	input       extensionBridgePairingInput
	serverNonce string
	decision    chan extensionBridgePairingDecision
}

type ExtensionBridgeManagerSnapshot struct {
	Revision         uint64                          `json:"revision"`
	Running          bool                            `json:"running"`
	Connected        bool                            `json:"connected"`
	URL              string                          `json:"url,omitempty"`
	LastError        string                          `json:"lastError,omitempty"`
	ProtocolVersion  int                             `json:"protocolVersion"`
	EngineIdentityID string                          `json:"engineIdentityId"`
	EngineInstanceID string                          `json:"engineInstanceId"`
	PairingOpenUntil int64                           `json:"pairingOpenUntil,omitempty"`
	Pending          []ExtensionBridgePairingRequest `json:"pending"`
	Devices          []ExtensionBridgeDevice         `json:"devices"`
	Connections      []ExtensionBridgeConnection     `json:"connections"`
}

type ExtensionBridgeManager struct {
	mu                      sync.RWMutex
	authorizationMu         sync.Mutex
	authorization           map[string]ExtensionAuthorizationWorkspace
	authorizationTombstones map[string]extensionAuthorizationWorkspaceTombstone
	engineInstanceID        string
	store                   ExtensionBridgeIdentityStore
	identity                extensionBridgeIdentityState
	privateKey              *ecdsa.PrivateKey
	server                  *ExtensionBridgeServer
	lastError               string
	pairingOpenUntil        time.Time
	pending                 map[string]*extensionBridgePendingPairing
	pairingAttempts         map[string][]time.Time
	revision                uint64
	onChange                func(uint64, string)
}

func NewExtensionBridgeManager(store ExtensionBridgeIdentityStore, onChange func(uint64, string)) (*ExtensionBridgeManager, error) {
	if store == nil {
		return nil, errors.New("browser extension identity store is required")
	}
	state, key, err := loadOrCreateExtensionBridgeIdentity(store)
	if err != nil {
		return nil, err
	}
	engineInstanceID, err := newExtensionBridgeID("engine")
	if err != nil {
		return nil, fmt.Errorf("create browser extension engine instance: %w", err)
	}
	return &ExtensionBridgeManager{
		store:                   store,
		identity:                *state,
		privateKey:              key,
		engineInstanceID:        engineInstanceID,
		authorization:           make(map[string]ExtensionAuthorizationWorkspace),
		authorizationTombstones: make(map[string]extensionAuthorizationWorkspaceTombstone),
		pending:                 make(map[string]*extensionBridgePendingPairing),
		pairingAttempts:         make(map[string][]time.Time),
		onChange:                onChange,
	}, nil
}

func loadOrCreateExtensionBridgeIdentity(store ExtensionBridgeIdentityStore) (*extensionBridgeIdentityState, *ecdsa.PrivateKey, error) {
	state, err := store.Load()
	if err == nil {
		if state.Version != extensionBridgeIdentityVersion || strings.TrimSpace(state.EngineIdentityID) == "" {
			return nil, nil, errors.New("unsupported browser extension identity format")
		}
		encoded, decodeErr := base64.RawURLEncoding.DecodeString(state.EnginePrivateKey)
		if decodeErr != nil {
			return nil, nil, fmt.Errorf("decode browser extension private key: %w", decodeErr)
		}
		key, parseErr := x509.ParseECPrivateKey(encoded)
		if parseErr != nil || key.Curve != elliptic.P256() {
			return nil, nil, errors.New("browser extension private key is invalid")
		}
		return state, key, nil
	}
	if !os.IsNotExist(err) {
		return nil, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate browser extension identity: %w", err)
	}
	encoded, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode browser extension identity: %w", err)
	}
	identityID, err := newExtensionBridgeID("engine-identity")
	if err != nil {
		return nil, nil, err
	}
	state = &extensionBridgeIdentityState{
		Version:          extensionBridgeIdentityVersion,
		EngineIdentityID: identityID,
		EnginePrivateKey: base64.RawURLEncoding.EncodeToString(encoded),
		Devices:          make([]ExtensionBridgeDevice, 0),
	}
	if err := store.Save(state); err != nil {
		return nil, nil, err
	}
	return state, key, nil
}

func (m *ExtensionBridgeManager) Start(port int) error {
	server, err := newManagedExtensionBridgeServer(port, m)
	m.mu.Lock()
	previous := m.server
	if err != nil {
		m.lastError = err.Error()
		m.bumpRevisionLocked("bridge_start_failed")
		m.mu.Unlock()
		return err
	}
	m.server = server
	m.lastError = ""
	m.bumpRevisionLocked("bridge_started")
	m.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	return nil
}

func (m *ExtensionBridgeManager) Close() error {
	m.mu.Lock()
	server := m.server
	m.server = nil
	m.bumpRevisionLocked("bridge_stopped")
	m.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Close()
}

func (m *ExtensionBridgeManager) OpenPairingWindow(ttl time.Duration) time.Time {
	if ttl <= 0 || ttl > 10*time.Minute {
		ttl = extensionPairingTTL
	}
	m.mu.Lock()
	m.pairingOpenUntil = time.Now().Add(ttl)
	m.bumpRevisionLocked("pairing_window_opened")
	until := m.pairingOpenUntil
	m.mu.Unlock()
	return until
}

func (m *ExtensionBridgeManager) Snapshot() ExtensionBridgeManagerSnapshot {
	m.mu.Lock()
	m.cleanupExpiredPairingsLocked(time.Now())
	server := m.server
	snapshot := ExtensionBridgeManagerSnapshot{
		Revision:         m.revision,
		Running:          server != nil && !server.closed.Load(),
		LastError:        m.lastError,
		ProtocolVersion:  managedExtensionBridgeProtocolVersion,
		EngineIdentityID: m.identity.EngineIdentityID,
		EngineInstanceID: m.engineInstanceID,
		Pending:          make([]ExtensionBridgePairingRequest, 0, len(m.pending)),
		Devices:          append([]ExtensionBridgeDevice(nil), m.identity.Devices...),
		Connections:      make([]ExtensionBridgeConnection, 0),
	}
	if !m.pairingOpenUntil.IsZero() && time.Now().Before(m.pairingOpenUntil) {
		snapshot.PairingOpenUntil = m.pairingOpenUntil.UnixMilli()
	}
	for _, pending := range m.pending {
		snapshot.Pending = append(snapshot.Pending, pending.request)
	}
	m.mu.Unlock()
	if server != nil {
		snapshot.URL = server.URL()
		snapshot.Connections = server.Connections()
		snapshot.Connected = len(snapshot.Connections) > 0
	}
	sort.Slice(snapshot.Pending, func(i, j int) bool { return snapshot.Pending[i].CreatedAt < snapshot.Pending[j].CreatedAt })
	sort.Slice(snapshot.Devices, func(i, j int) bool { return snapshot.Devices[i].CreatedAt < snapshot.Devices[j].CreatedAt })
	return snapshot
}

func (m *ExtensionBridgeManager) EngineIdentity() (string, ExtensionBridgeJWK) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.identity.EngineIdentityID, publicKeyToJWK(&m.privateKey.PublicKey)
}

func (m *ExtensionBridgeManager) currentServer() *ExtensionBridgeServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.server
}

func (m *ExtensionBridgeManager) CallDevice(ctx context.Context, deviceID, method string, params interface{}) (json.RawMessage, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, errors.New("browser extension device id is required")
	}
	m.mu.RLock()
	server := m.server
	paired := false
	for _, device := range m.identity.Devices {
		if device.ID == deviceID {
			paired = true
			break
		}
	}
	m.mu.RUnlock()
	if !paired {
		return nil, errors.New("paired browser extension device not found")
	}
	if server == nil || server.closed.Load() {
		return nil, errors.New("extension bridge is not running")
	}
	return server.CallDevice(ctx, deviceID, method, params)
}

func (m *ExtensionBridgeManager) Sign(payload string) (string, error) {
	m.mu.RLock()
	key := m.privateKey
	m.mu.RUnlock()
	hash := sha256.Sum256([]byte(payload))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", err
	}
	return encodeECDSASignature(r, s), nil
}

func (m *ExtensionBridgeManager) beginPairing(origin string, input extensionBridgePairingInput) (*extensionBridgePendingPairing, error) {
	now := time.Now()
	input.InstallationID = strings.TrimSpace(input.InstallationID)
	input.Client = strings.TrimSpace(input.Client)
	input.ClientVersion = strings.TrimSpace(input.ClientVersion)
	if input.InstallationID == "" || len(input.InstallationID) > 160 || input.Client == "" || len(input.Client) > 120 {
		return nil, errors.New("invalid browser extension pairing identity")
	}
	if _, err := parseExtensionBridgePublicKey(input.PublicKey); err != nil {
		return nil, err
	}
	clientNonce, err := base64.RawURLEncoding.DecodeString(input.Nonce)
	if err != nil || len(clientNonce) < 16 || len(clientNonce) > 64 {
		return nil, errors.New("invalid browser extension pairing nonce")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredPairingsLocked(now)
	if len(m.pending) >= extensionPairingMaxPending {
		return nil, errors.New("too many browser extension pairing requests")
	}
	attempts := m.pairingAttempts[origin][:0]
	for _, attempt := range m.pairingAttempts[origin] {
		if now.Sub(attempt) < time.Minute {
			attempts = append(attempts, attempt)
		}
	}
	if len(attempts) >= extensionPairingMaxPerMinute {
		m.pairingAttempts[origin] = attempts
		return nil, errors.New("browser extension pairing rate limit exceeded")
	}
	m.pairingAttempts[origin] = append(attempts, now)
	requestID, err := newExtensionBridgeID("pairing")
	if err != nil {
		return nil, err
	}
	serverNonceBytes := make([]byte, 32)
	if _, err := rand.Read(serverNonceBytes); err != nil {
		return nil, err
	}
	serverNonce := base64.RawURLEncoding.EncodeToString(serverNonceBytes)
	extensionID := strings.TrimPrefix(strings.TrimPrefix(origin, "chrome-extension://"), "moz-extension://")
	request := ExtensionBridgePairingRequest{
		ID: requestID, InstallationID: input.InstallationID, ExtensionID: extensionID,
		Client: input.Client, ClientVersion: input.ClientVersion, Origin: origin,
		Code:      pairingVerificationCode(m.identity.EngineIdentityID, requestID, origin, input, serverNonce),
		CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(extensionPairingTTL).UnixMilli(),
	}
	pending := &extensionBridgePendingPairing{
		request: request, input: input, serverNonce: serverNonce,
		decision: make(chan extensionBridgePairingDecision, 1),
	}
	m.pending[requestID] = pending
	m.bumpRevisionLocked("pairing_requested")
	return pending, nil
}

func (m *ExtensionBridgeManager) cancelPairing(requestID string) {
	m.mu.Lock()
	if _, ok := m.pending[requestID]; ok {
		delete(m.pending, requestID)
		m.bumpRevisionLocked("pairing_cancelled")
	}
	m.mu.Unlock()
}

func (m *ExtensionBridgeManager) ApprovePairing(requestID, name, replaceDeviceID string) (*ExtensionBridgeDevice, error) {
	m.mu.Lock()
	m.cleanupExpiredPairingsLocked(time.Now())
	pending := m.pending[requestID]
	if pending == nil {
		m.mu.Unlock()
		return nil, errors.New("browser extension pairing request not found or expired")
	}
	replaceDeviceID = strings.TrimSpace(replaceDeviceID)
	replacementIndex := -1
	if replaceDeviceID != "" {
		for index := range m.identity.Devices {
			candidate := m.identity.Devices[index]
			if candidate.ID != replaceDeviceID {
				continue
			}
			if candidate.Origin != pending.request.Origin || candidate.Client != pending.input.Client {
				m.mu.Unlock()
				return nil, errors.New("replacement browser identity does not match the pairing request")
			}
			replacementIndex = index
			break
		}
		if replacementIndex < 0 {
			m.mu.Unlock()
			return nil, errors.New("replacement browser identity was not found")
		}
	} else {
		for index := range m.identity.Devices {
			if m.identity.Devices[index].InstallationID == pending.input.InstallationID {
				replacementIndex = index
				break
			}
		}
	}

	deviceID := ""
	createdAt := int64(0)
	previousInstallationIDs := make(map[string]struct{})
	if replacementIndex >= 0 {
		replacement := m.identity.Devices[replacementIndex]
		deviceID = replacement.ID
		createdAt = replacement.CreatedAt
		previousInstallationIDs[replacement.InstallationID] = struct{}{}
		if strings.TrimSpace(replacement.Name) != "" {
			name = replacement.Name
		}
	} else {
		var err error
		deviceID, err = newExtensionBridgeID("browser")
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
	}
	if strings.TrimSpace(name) == "" {
		name = "Browser Extension"
	}
	now := time.Now().UnixMilli()
	if createdAt == 0 {
		createdAt = now
	}
	device := ExtensionBridgeDevice{
		ID: deviceID, InstallationID: pending.input.InstallationID, Name: strings.TrimSpace(name),
		Client: pending.input.Client, ClientVersion: pending.input.ClientVersion,
		Origin: pending.request.Origin, PublicKey: pending.input.PublicKey, CreatedAt: createdAt, LastSeenAt: now,
	}
	updated := m.identity
	updated.Devices = make([]ExtensionBridgeDevice, 0, len(m.identity.Devices)+1)
	replaced := false
	for index, candidate := range m.identity.Devices {
		if index == replacementIndex {
			updated.Devices = append(updated.Devices, device)
			replaced = true
			continue
		}
		if candidate.InstallationID == pending.input.InstallationID {
			previousInstallationIDs[candidate.InstallationID] = struct{}{}
			continue
		}
		updated.Devices = append(updated.Devices, candidate)
	}
	if !replaced {
		updated.Devices = append(updated.Devices, device)
	}
	if err := m.store.Save(&updated); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.identity = updated
	delete(m.pending, requestID)
	server := m.server
	m.bumpRevisionLocked("pairing_approved")
	m.mu.Unlock()
	if server != nil {
		for installationID := range previousInstallationIDs {
			server.disconnectInstallation(installationID)
		}
	}
	select {
	case pending.decision <- extensionBridgePairingDecision{approved: true, device: &device}:
	default:
	}
	return &device, nil
}

func (m *ExtensionBridgeManager) RejectPairing(requestID, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pending := m.pending[requestID]
	if pending == nil {
		return errors.New("browser extension pairing request not found or expired")
	}
	delete(m.pending, requestID)
	m.bumpRevisionLocked("pairing_rejected")
	if strings.TrimSpace(message) == "" {
		message = "Pairing request rejected in Yakit"
	}
	select {
	case pending.decision <- extensionBridgePairingDecision{message: message}:
	default:
	}
	return nil
}

func (m *ExtensionBridgeManager) RenameDevice(deviceID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return errors.New("browser extension device name must contain 1 to 80 characters")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	updated := m.identity
	updated.Devices = append([]ExtensionBridgeDevice(nil), m.identity.Devices...)
	found := false
	for index := range updated.Devices {
		if updated.Devices[index].ID == deviceID {
			updated.Devices[index].Name = name
			found = true
			break
		}
	}
	if !found {
		return errors.New("paired browser extension device not found")
	}
	if err := m.store.Save(&updated); err != nil {
		return err
	}
	m.identity = updated
	m.bumpRevisionLocked("device_renamed")
	return nil
}

func (m *ExtensionBridgeManager) RevokeDevice(deviceID string) error {
	m.mu.Lock()
	updated := m.identity
	updated.Devices = make([]ExtensionBridgeDevice, 0, len(m.identity.Devices))
	found := false
	revokedInstallationID := ""
	for _, device := range m.identity.Devices {
		if device.ID == deviceID {
			found = true
			revokedInstallationID = device.InstallationID
			continue
		}
		updated.Devices = append(updated.Devices, device)
	}
	if !found {
		m.mu.Unlock()
		return errors.New("paired browser extension device not found")
	}
	if err := m.store.Save(&updated); err != nil {
		m.mu.Unlock()
		return err
	}
	m.identity = updated
	server := m.server
	m.bumpRevisionLocked("device_revoked")
	m.mu.Unlock()
	if server != nil {
		server.disconnectInstallation(revokedInstallationID)
	}
	return nil
}

func (m *ExtensionBridgeManager) authenticateDevice(installationID, origin, payload, signature string) (*ExtensionBridgeDevice, error) {
	m.mu.RLock()
	var device *ExtensionBridgeDevice
	for index := range m.identity.Devices {
		candidate := &m.identity.Devices[index]
		if candidate.InstallationID == installationID && candidate.Origin == origin {
			copy := *candidate
			device = &copy
			break
		}
	}
	m.mu.RUnlock()
	if device == nil {
		return nil, errors.New("browser extension installation is not paired")
	}
	publicKey, err := parseExtensionBridgePublicKey(device.PublicKey)
	if err != nil {
		return nil, err
	}
	if !verifyECDSASignature(publicKey, payload, signature) {
		return nil, errors.New("browser extension signature is invalid")
	}
	return device, nil
}

func (m *ExtensionBridgeManager) markDeviceSeen(deviceID, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UnixMilli()
	updated := m.identity
	updated.Devices = append([]ExtensionBridgeDevice(nil), m.identity.Devices...)
	changed := false
	for index := range updated.Devices {
		if updated.Devices[index].ID == deviceID && now-updated.Devices[index].LastSeenAt > int64(time.Minute/time.Millisecond) {
			updated.Devices[index].LastSeenAt = now
			updated.Devices[index].ClientVersion = version
			changed = true
			break
		}
	}
	if !changed || m.store.Save(&updated) != nil {
		return
	}
	m.identity = updated
	m.bumpRevisionLocked("device_connected")
}

func (m *ExtensionBridgeManager) cleanupExpiredPairingsLocked(now time.Time) {
	changed := false
	for id, pending := range m.pending {
		if now.UnixMilli() < pending.request.ExpiresAt {
			continue
		}
		delete(m.pending, id)
		changed = true
		select {
		case pending.decision <- extensionBridgePairingDecision{message: "Pairing request expired"}:
		default:
		}
	}
	if changed {
		m.bumpRevisionLocked("pairing_expired")
	}
}

func (m *ExtensionBridgeManager) bumpRevisionLocked(event string) {
	m.revision++
	if m.onChange != nil {
		revision := m.revision
		go m.onChange(revision, event)
	}
}

func (m *ExtensionBridgeManager) notifyStateChange(event string) {
	m.mu.Lock()
	m.bumpRevisionLocked(event)
	m.mu.Unlock()
}

func publicKeyToJWK(key *ecdsa.PublicKey) ExtensionBridgeJWK {
	return ExtensionBridgeJWK{
		KTY: "EC", CRV: "P-256",
		X: base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
		Y: base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
	}
}

func parseExtensionBridgePublicKey(jwk ExtensionBridgeJWK) (*ecdsa.PublicKey, error) {
	if jwk.KTY != "EC" || jwk.CRV != "P-256" {
		return nil, errors.New("browser extension public key must use ECDSA P-256")
	}
	xBytes, xErr := base64.RawURLEncoding.DecodeString(jwk.X)
	yBytes, yErr := base64.RawURLEncoding.DecodeString(jwk.Y)
	if xErr != nil || yErr != nil || len(xBytes) != 32 || len(yBytes) != 32 {
		return nil, errors.New("browser extension public key is invalid")
	}
	x, y := new(big.Int).SetBytes(xBytes), new(big.Int).SetBytes(yBytes)
	if !elliptic.P256().IsOnCurve(x, y) {
		return nil, errors.New("browser extension public key is not on P-256")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

func encodeECDSASignature(r, s *big.Int) string {
	encoded := make([]byte, 64)
	r.FillBytes(encoded[:32])
	s.FillBytes(encoded[32:])
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func verifyECDSASignature(key *ecdsa.PublicKey, payload, signature string) bool {
	encoded, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || len(encoded) != 64 {
		return false
	}
	hash := sha256.Sum256([]byte(payload))
	return ecdsa.Verify(key, hash[:], new(big.Int).SetBytes(encoded[:32]), new(big.Int).SetBytes(encoded[32:]))
}

func pairingVerificationCode(engineIdentityID, requestID, origin string, input extensionBridgePairingInput, serverNonce string) string {
	parts := []string{
		"yak-browser-pairing-v1", engineIdentityID, requestID, origin, input.InstallationID,
		input.Nonce, serverNonce, input.PublicKey.KTY, input.PublicKey.CRV, input.PublicKey.X, input.PublicKey.Y,
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return fmt.Sprintf("%06d", binary.BigEndian.Uint64(hash[:8])%1_000_000)
}

func managedEngineChallengePayload(engineIdentityID, engineInstanceID, challenge string, timestamp int64) string {
	return strings.Join([]string{
		"yak-browser-bridge-v3", "engine-challenge", engineIdentityID, engineInstanceID,
		challenge, fmt.Sprintf("%d", timestamp),
	}, "\n")
}

func managedClientAuthPayload(origin, engineIdentityID, engineInstanceID, challenge string, hello ExtensionBridgeEnvelope) string {
	capabilities := append([]string(nil), hello.Capabilities...)
	sort.Strings(capabilities)
	capabilityCatalogVersion := ""
	capabilityCatalogHash := ""
	if hello.CapabilityCatalog != nil {
		capabilityCatalogVersion = strconv.Itoa(hello.CapabilityCatalog.Version)
		capabilityCatalogHash = hello.CapabilityCatalog.Hash
	}
	return strings.Join([]string{
		"yak-browser-bridge-v3", "client-auth", origin, engineIdentityID, engineInstanceID,
		challenge, hello.InstallationID, hello.Client, hello.Version, strings.Join(capabilities, ","),
		capabilityCatalogVersion, capabilityCatalogHash,
		hello.TaskID, hello.GrantID, hello.ResumeSessionID,
	}, "\n")
}

var activeExtensionBridgeManager struct {
	sync.RWMutex
	manager *ExtensionBridgeManager
}

func SetActiveExtensionBridgeManager(manager *ExtensionBridgeManager) {
	activeExtensionBridgeManager.Lock()
	activeExtensionBridgeManager.manager = manager
	activeExtensionBridgeManager.Unlock()
}

func ActiveExtensionBridgeManager() *ExtensionBridgeManager {
	activeExtensionBridgeManager.RLock()
	defer activeExtensionBridgeManager.RUnlock()
	return activeExtensionBridgeManager.manager
}
