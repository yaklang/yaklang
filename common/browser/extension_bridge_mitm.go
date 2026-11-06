package browser

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/minimartian"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

const extensionMITMLeaseTTL = 90 * time.Second
const extensionMITMMaxInstances = 64
const extensionMITMMaxConnections = 128

type extensionMITMRuntime struct {
	ctx      context.Context
	cancel   context.CancelFunc
	host     string
	port     int
	auth     bool
	incoming chan *minimartian.WrapperedConn
	leases   map[string]*extensionMITMLease
}

type extensionMITMLease struct {
	closed   bool
	listener net.Listener
	identity httpctx.BrowserInstance
	expires  time.Time
	conns    map[net.Conn]struct{}
}

type extensionMITMIngress struct {
	Available         bool   `json:"available"`
	Host              string `json:"host,omitempty"`
	Port              int    `json:"port,omitempty"`
	ExpiresAt         int64  `json:"expiresAt,omitempty"`
	RequiresProxyAuth bool   `json:"requiresProxyAuth,omitempty"`
}

// RegisterMITMIngress makes an already-listening MITM available to paired
// browsers. All instance listeners feed the SAME proxy, hooks and project DB.
// The returned cleanup and ctx cancellation both reclaim listeners and sockets.
func (m *ExtensionBridgeManager) RegisterMITMIngress(ctx context.Context, address string, auth bool, incoming chan *minimartian.WrapperedConn) func() {
	host, rawPort, err := net.SplitHostPort(address)
	port, _ := strconv.Atoi(rawPort)
	if m == nil || err != nil || port < 1 || port > 65535 || incoming == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	runtime := &extensionMITMRuntime{ctx: ctx, cancel: cancel, host: host, port: port, auth: auth,
		incoming: incoming, leases: make(map[string]*extensionMITMLease)}
	m.mitmMu.Lock()
	if m.mitmRuntimes == nil {
		m.mitmRuntimes = make(map[string]*extensionMITMRuntime)
	}
	if previous := m.mitmRuntimes[address]; previous != nil {
		m.closeMITMRuntimeLocked(previous)
	}
	m.mitmRuntimes[address] = runtime
	m.mitmMu.Unlock()
	cleanup := func() {
		m.mitmMu.Lock()
		defer m.mitmMu.Unlock()
		if m.mitmRuntimes[address] == runtime {
			delete(m.mitmRuntimes, address)
		}
		m.closeMITMRuntimeLocked(runtime)
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		defer cleanup()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				m.expireMITMLeases(runtime, now)
			}
		}
	}()
	return cleanup
}

func (m *ExtensionBridgeManager) closeMITMLeaseLocked(lease *extensionMITMLease) {
	lease.closed = true
	_ = lease.listener.Close()
	for conn := range lease.conns {
		_ = conn.Close()
	}
	clear(lease.conns)
}

func (m *ExtensionBridgeManager) closeMITMRuntimeLocked(runtime *extensionMITMRuntime) {
	runtime.cancel()
	for key, lease := range runtime.leases {
		m.closeMITMLeaseLocked(lease)
		delete(runtime.leases, key)
	}
}

func (m *ExtensionBridgeManager) closeMITMIngresses() {
	m.mitmMu.Lock()
	defer m.mitmMu.Unlock()
	for key, runtime := range m.mitmRuntimes {
		m.closeMITMRuntimeLocked(runtime)
		delete(m.mitmRuntimes, key)
	}
}

func (m *ExtensionBridgeManager) expireMITMLeases(runtime *extensionMITMRuntime, now time.Time) {
	m.mitmMu.Lock()
	defer m.mitmMu.Unlock()
	for key, lease := range runtime.leases {
		if !now.Before(lease.expires) {
			m.closeMITMLeaseLocked(lease)
			delete(runtime.leases, key)
		}
	}
}

func (m *ExtensionBridgeManager) releaseMITMInstallation(installationID string) {
	m.mitmMu.Lock()
	defer m.mitmMu.Unlock()
	for _, runtime := range m.mitmRuntimes {
		for key, lease := range runtime.leases {
			if lease.identity.InstallationID == installationID {
				m.closeMITMLeaseLocked(lease)
				delete(runtime.leases, key)
			}
		}
	}
}

func matchingMITMHost(requested, bound string) bool {
	loopback := func(host string) bool {
		ip := net.ParseIP(strings.Trim(host, "[]"))
		return strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback()
	}
	if strings.EqualFold(requested, bound) || loopback(requested) && loopback(bound) {
		return true
	}
	ip := net.ParseIP(bound)
	return loopback(requested) && (bound == "" || ip != nil && ip.IsUnspecified())
}

func (s *ExtensionBridgeServer) acquireMITMIngress(deviceID string, params json.RawMessage) (interface{}, *ExtensionBridgeError) {
	var input struct {
		ProxyHost string `json:"proxyHost"`
		ProxyPort int    `json:"proxyPort"`
	}
	if err := decodeStrictExtensionBridgeJSON(params, &input); err != nil || input.ProxyPort < 1 || input.ProxyPort > 65535 || len(input.ProxyHost) < 1 || len(input.ProxyHost) > 255 {
		return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid MITM proxy endpoint"}
	}
	s.clientMu.RLock()
	client := s.managedClients[deviceID]
	var identity httpctx.BrowserInstance
	if client != nil {
		identity = client.status.BrowserInstance()
	}
	s.clientMu.RUnlock()
	if s.manager == nil || identity.InstanceID == "" {
		return extensionMITMIngress{}, nil
	}
	// Serialize against revocation: no in-flight request may allocate a new
	// listener after RevokeDevice has removed and reclaimed this installation.
	s.manager.mu.RLock()
	defer s.manager.mu.RUnlock()
	for _, device := range s.manager.identity.Devices {
		if device.ID == deviceID && device.InstallationID == identity.InstallationID {
			return s.manager.acquireInstanceMITMIngress(input.ProxyHost, input.ProxyPort, identity)
		}
	}
	return extensionMITMIngress{}, nil
}

func (m *ExtensionBridgeManager) acquireInstanceMITMIngress(host string, port int, identity httpctx.BrowserInstance) (extensionMITMIngress, *ExtensionBridgeError) {
	m.mitmMu.Lock()
	defer m.mitmMu.Unlock()
	for _, runtime := range m.mitmRuntimes {
		if runtime.ctx.Err() != nil || runtime.port != port || !matchingMITMHost(host, runtime.host) {
			continue
		}
		key := identity.InstallationID + "/" + identity.InstanceID
		lease := runtime.leases[key]
		if lease != nil && (lease.identity != identity || !time.Now().Before(lease.expires)) {
			m.closeMITMLeaseLocked(lease)
			delete(runtime.leases, key)
			lease = nil
		}
		if lease == nil {
			if len(runtime.leases) >= extensionMITMMaxInstances {
				return extensionMITMIngress{}, &ExtensionBridgeError{Code: "server_busy", Message: "MITM instance ingress limit reached"}
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				return extensionMITMIngress{}, &ExtensionBridgeError{Code: "ingress_unavailable", Message: "cannot allocate browser MITM ingress"}
			}
			lease = &extensionMITMLease{listener: listener, identity: identity, conns: make(map[net.Conn]struct{})}
			runtime.leases[key] = lease
			go m.acceptMITMIngress(runtime, lease)
		}
		lease.expires = time.Now().Add(extensionMITMLeaseTTL)
		return extensionMITMIngress{Available: true, Host: "127.0.0.1", Port: lease.listener.Addr().(*net.TCPAddr).Port,
			ExpiresAt: lease.expires.UnixMilli(), RequiresProxyAuth: runtime.auth}, nil
	}
	return extensionMITMIngress{}, nil
}

type extensionMITMConn struct {
	net.Conn
	once sync.Once
	done func()
}

func (c *extensionMITMConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.done)
	return err
}

func (m *ExtensionBridgeManager) acceptMITMIngress(runtime *extensionMITMRuntime, lease *extensionMITMLease) {
	for {
		conn, err := lease.listener.Accept()
		if err != nil {
			return
		}
		m.mitmMu.Lock()
		if lease.closed || runtime.ctx.Err() != nil || !time.Now().Before(lease.expires) || len(lease.conns) >= extensionMITMMaxConnections {
			m.mitmMu.Unlock()
			_ = conn.Close()
			continue
		}
		lease.conns[conn] = struct{}{}
		m.mitmMu.Unlock()
		tracked := &extensionMITMConn{Conn: conn, done: func() {
			m.mitmMu.Lock()
			delete(lease.conns, conn)
			m.mitmMu.Unlock()
		}}
		wrapped := minimartian.NewWrapperedConnEx(tracked, false,
			map[string]any{httpctx.BrowserInstanceContextKey: lease.identity}, true)
		select {
		case runtime.incoming <- wrapped:
		case <-runtime.ctx.Done():
			_ = tracked.Close()
			return
		case <-time.After(5 * time.Second):
			_ = tracked.Close()
		}
	}
}
