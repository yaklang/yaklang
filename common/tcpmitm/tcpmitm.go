package tcpmitm

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"
)

// FrameHijackCallback is the callback function signature for frame hijacking.
// It receives the connection flow and the frame instance for inspection/modification.
type FrameHijackCallback func(flow *ConnectionFlow, frame *Frame)

// ConnHijackCallback is the callback function signature for connection hijacking.
// It receives the hijacked connection and an operator to control the connection.
type ConnHijackCallback func(conn net.Conn, operator *ConnOperator)

// TCPMitm is the main controller for TCP MITM operations.
// It loads connections from a channel and provides frame-level and connection-level hijacking.
type TCPMitm struct {
	ctx    context.Context
	cancel context.CancelFunc

	// Connection channel from TUN device
	connChan <-chan net.Conn

	// Callbacks
	frameHijackCallback FrameHijackCallback
	connHijackCallback  ConnHijackCallback

	// Configuration
	splitterConfig *SplitterConfig

	// Dialer for connecting to real servers
	dialer func(addr string) (net.Conn, error)

	// State
	mu      sync.RWMutex
	running bool
	wg      sync.WaitGroup
}

// LoadConnectionChannel creates a new TCPMitm instance from a connection channel.
// The channel should receive hijacked TCP connections (e.g., from TUN device).
func LoadConnectionChannel(connChan <-chan net.Conn, opts ...Option) (*TCPMitm, error) {
	if connChan == nil {
		return nil, utils.Error("connection channel cannot be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &TCPMitm{
		ctx:            ctx,
		cancel:         cancel,
		connChan:       connChan,
		splitterConfig: DefaultSplitterConfig(),
		dialer:         defaultDialer,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m, nil
}

// Option is a function that configures TCPMitm.
type Option func(*TCPMitm)

// WithContext sets a custom context for the TCPMitm instance.
func WithContext(ctx context.Context) Option {
	return func(m *TCPMitm) {
		m.ctx, m.cancel = context.WithCancel(ctx)
	}
}

// WithSplitterConfig sets a custom splitter configuration.
func WithSplitterConfig(config *SplitterConfig) Option {
	return func(m *TCPMitm) {
		if config != nil {
			m.splitterConfig = config
		}
	}
}

// WithDialer sets a custom dialer for connecting to real servers.
func WithDialer(dialer func(addr string) (net.Conn, error)) Option {
	return func(m *TCPMitm) {
		if dialer != nil {
			m.dialer = dialer
		}
	}
}

// WithTimeGapThreshold sets the time gap threshold for frame splitting.
func WithTimeGapThreshold(d time.Duration) Option {
	return func(m *TCPMitm) {
		m.splitterConfig.TimeGapThreshold = d
	}
}

// WithSplitStrategy sets the split strategy.
func WithSplitStrategy(strategy SplitStrategy) Option {
	return func(m *TCPMitm) {
		m.splitterConfig.Strategy = strategy
	}
}

// WithMaxBufferSize sets the maximum buffer size before forcing a frame split.
// Default is 8KB. When buffer exceeds this, a frame is automatically emitted.
func WithMaxBufferSize(size int) Option {
	return func(m *TCPMitm) {
		if size > 0 {
			m.splitterConfig.MaxBufferSize = size
			m.splitterConfig.MaxFrameSize = size
		}
	}
}

// WithReadBufferSize sets the I/O read buffer size.
func WithReadBufferSize(size int) Option {
	return func(m *TCPMitm) {
		if size > 0 {
			m.splitterConfig.ReadBufferSize = size
		}
	}
}

// WithProtocolAwareSplit enables protocol-aware frame splitting.
func WithProtocolAwareSplit(enable bool) Option {
	return func(m *TCPMitm) {
		m.splitterConfig.EnableProtocolAwareSplit = enable
	}
}

// SetHijackTCPFrame sets the callback for frame-level hijacking.
// This callback is invoked for each segmented data frame.
func (m *TCPMitm) SetHijackTCPFrame(callback FrameHijackCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frameHijackCallback = callback
}

// SetHijackTCPConn sets the callback for connection-level hijacking.
// This callback is invoked when a new connection is established.
func (m *TCPMitm) SetHijackTCPConn(callback ConnHijackCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connHijackCallback = callback
}

// Run starts the MITM processing loop.
// It will block until the context is cancelled or an error occurs.
func (m *TCPMitm) Run() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return utils.Error("TCPMitm is already running")
	}
	m.running = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	log.Infof("tcpmitm: starting MITM processing loop")

	for {
		select {
		case <-m.ctx.Done():
			log.Infof("tcpmitm: context cancelled, stopping")
			m.wg.Wait()
			return nil

		case conn, ok := <-m.connChan:
			if !ok {
				log.Infof("tcpmitm: connection channel closed, stopping")
				m.wg.Wait()
				return nil
			}

			m.wg.Add(1)
			go func(c net.Conn) {
				defer m.wg.Done()
				m.handleConnection(c)
			}(conn)
		}
	}
}

// Stop stops the MITM processing.
func (m *TCPMitm) Stop() {
	m.cancel()
}

// handleConnection processes a single hijacked connection.
func (m *TCPMitm) handleConnection(hijackedConn net.Conn) {
	defer hijackedConn.Close()

	// Extract flow information
	flow := m.extractFlowInfo(hijackedConn)
	if flow == nil {
		log.Warnf("tcpmitm: failed to extract flow info from connection")
		return
	}

	log.Infof("tcpmitm: handling connection %s", flow.String())

	// Create operator
	operator := NewConnOperator(hijackedConn, flow)

	// Invoke connection hijack callback first
	m.mu.RLock()
	connCallback := m.connHijackCallback
	m.mu.RUnlock()

	if connCallback != nil {
		connCallback(hijackedConn, operator)

		// Check if connection is being held externally
		if operator.IsHeld() {
			log.Debugf("tcpmitm: connection %s is held externally", flow.String())
			return
		}

		// Check if connection is closed
		if operator.IsClosed() {
			log.Debugf("tcpmitm: connection %s was closed by callback", flow.String())
			return
		}

		// Check if connection is attached to another
		if operator.IsAttached() {
			log.Debugf("tcpmitm: connection %s was attached to another connection", flow.String())
			return
		}
	}

	// If we have a frame callback, do frame-level processing
	m.mu.RLock()
	frameCallback := m.frameHijackCallback
	m.mu.RUnlock()

	if frameCallback != nil {
		m.handleWithFrameCallback(hijackedConn, flow, frameCallback)
	} else {
		// No frame callback, just transparent forward
		m.handleTransparent(hijackedConn, flow)
	}
}

// handleWithFrameCallback processes connection with frame-level hijacking.
func (m *TCPMitm) handleWithFrameCallback(hijackedConn net.Conn, flow *ConnectionFlow, callback FrameHijackCallback) {
	// Connect to real server
	serverAddr := flow.GetServerAddr()
	serverConn, err := m.dialer(serverAddr)
	if err != nil {
		log.Errorf("tcpmitm: failed to connect to server %s: %v", serverAddr, err)
		return
	}
	defer serverConn.Close()

	log.Debugf("tcpmitm: connected to real server %s", serverAddr)

	ctx, cancel := context.WithCancel(m.ctx)
	defer cancel()

	// Create connection context with frame queues
	connCtx := NewConnectionContext(ctx, flow)
	flow.SetConnectionContext(connCtx)
	defer connCtx.Close()

	var wg sync.WaitGroup

	// Client -> Server direction: reader -> queue -> callback -> writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		m.processDirectionWithQueue(ctx, hijackedConn, serverConn, flow, DirectionClientToServer, connCtx, callback)
	}()

	// Server -> Client direction: reader -> queue -> callback -> writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		m.processDirectionWithQueue(ctx, serverConn, hijackedConn, flow, DirectionServerToClient, connCtx, callback)
	}()

	wg.Wait()
}

// processDirection processes one direction of data flow with frame splitting.
// This is the legacy version without queue support.
func (m *TCPMitm) processDirection(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	flow *ConnectionFlow,
	direction FrameDirection,
	callback FrameHijackCallback,
) {
	splitter := NewStreamSplitter(reader, writer, direction, m.splitterConfig)
	splitter.Start()
	defer splitter.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-splitter.Frames():
			if !ok {
				return
			}

			// Invoke callback for inspection/modification
			callback(flow, frame)

			// Write the frame (handles drop, inject, modify)
			if err := splitter.WriteFrame(frame); err != nil {
				log.Debugf("tcpmitm: write frame error: %v", err)
				return
			}
		}
	}
}

// processDirectionWithQueue processes one direction of data flow with frame queuing.
// This version supports peek functionality by buffering frames in a queue before
// invoking the callback. Each connection's frames are processed sequentially within
// that connection, while multiple connections are processed concurrently.
func (m *TCPMitm) processDirectionWithQueue(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	flow *ConnectionFlow,
	direction FrameDirection,
	connCtx *ConnectionContext,
	callback FrameHijackCallback,
) {
	splitter := NewStreamSplitter(reader, writer, direction, m.splitterConfig)
	splitter.Start()
	defer splitter.Stop()

	queue := connCtx.GetQueue(direction)

	// Producer goroutine: reads frames and enqueues them
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		defer queue.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-splitter.Frames():
				if !ok {
					return
				}
				// Assign sequence number
				seq := connCtx.IncrementFrameCounter()
				frame.SetSequenceNum(seq)
				queue.Enqueue(frame)
			}
		}
	}()

	// Consumer: processes frames sequentially from the queue
	for {
		frame := queue.DequeueWait(ctx)
		if frame == nil {
			// Queue closed or context cancelled
			break
		}

		// Invoke callback for inspection/modification
		// At this point, the queue may have more frames buffered that can be peeked
		callback(flow, frame)

		// Write the frame (handles drop, inject, modify)
		if err := splitter.WriteFrame(frame); err != nil {
			log.Debugf("tcpmitm: write frame error: %v", err)
			break
		}
	}

	// Wait for producer to finish
	<-producerDone
}

// handleTransparent performs transparent forwarding without frame processing.
func (m *TCPMitm) handleTransparent(hijackedConn net.Conn, flow *ConnectionFlow) {
	// Connect to real server
	serverAddr := flow.GetServerAddr()
	serverConn, err := m.dialer(serverAddr)
	if err != nil {
		log.Errorf("tcpmitm: failed to connect to server %s: %v", serverAddr, err)
		return
	}
	defer serverConn.Close()

	log.Debugf("tcpmitm: transparent forwarding to %s", serverAddr)

	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Server
	go func() {
		defer wg.Done()
		io.Copy(serverConn, hijackedConn)
	}()

	// Server -> Client
	go func() {
		defer wg.Done()
		io.Copy(hijackedConn, serverConn)
	}()

	wg.Wait()
}

// extractFlowInfo extracts connection flow information from the hijacked connection.
func (m *TCPMitm) extractFlowInfo(conn net.Conn) *ConnectionFlow {
	// Try to get TransportEndpointID if it's a netstack TCPConn
	if tcpConn, ok := conn.(netstack.TCPConn); ok {
		id := tcpConn.ID()
		if id != nil {
			return m.flowFromEndpointID(conn, id)
		}
	}

	// Fallback: try to parse from net.Addr
	return m.flowFromAddr(conn)
}

// flowFromEndpointID creates ConnectionFlow from TransportEndpointID.
// Note: In hijacked connection context:
// - RemoteAddress/RemotePort is the CLIENT (connection initiator)
// - LocalAddress/LocalPort is the SERVER (original destination)
func (m *TCPMitm) flowFromEndpointID(conn net.Conn, id *stack.TransportEndpointID) *ConnectionFlow {
	return NewConnectionFlow(
		conn,
		id.RemoteAddress.String(), // Client IP
		int(id.RemotePort),        // Client Port
		id.LocalAddress.String(),  // Server IP
		int(id.LocalPort),         // Server Port
	)
}

// flowFromAddr creates ConnectionFlow from net.Addr (fallback).
func (m *TCPMitm) flowFromAddr(conn net.Conn) *ConnectionFlow {
	remoteAddr := conn.RemoteAddr()
	localAddr := conn.LocalAddr()

	if remoteAddr == nil || localAddr == nil {
		return nil
	}

	remoteTCP, ok := remoteAddr.(*net.TCPAddr)
	if !ok {
		return nil
	}
	localTCP, ok := localAddr.(*net.TCPAddr)
	if !ok {
		return nil
	}

	return NewConnectionFlow(
		conn,
		remoteTCP.IP.String(),
		remoteTCP.Port,
		localTCP.IP.String(),
		localTCP.Port,
	)
}

// defaultDialer is the default dialer that connects to the target server.
func defaultDialer(addr string) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, 30*time.Second)
}
