package tcpmitm

import (
	"io"
	"net"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

// ConnOperator provides operations on a hijacked TCP connection.
// It allows the user to control how the hijacked connection is handled.
type ConnOperator struct {
	mu sync.RWMutex

	// The hijacked connection (from TUN device)
	hijackedConn net.Conn

	// Connection flow information
	flow *ConnectionFlow

	// State flags
	held       bool
	closed     bool
	attached   bool
	attachedTo net.Conn
}

// NewConnOperator creates a new ConnOperator for the given hijacked connection.
func NewConnOperator(conn net.Conn, flow *ConnectionFlow) *ConnOperator {
	return &ConnOperator{
		hijackedConn: conn,
		flow:         flow,
		held:         false,
		closed:       false,
		attached:     false,
	}
}

// CloseHijackedConn forcefully closes the hijacked connection.
// This will terminate the connection immediately.
func (op *ConnOperator) CloseHijackedConn() error {
	op.mu.Lock()
	defer op.mu.Unlock()

	if op.closed {
		return nil
	}

	op.closed = true
	if op.hijackedConn != nil {
		log.Infof("tcpmitm: closing hijacked connection %s", op.flow.String())
		return op.hijackedConn.Close()
	}
	return nil
}

// AttachConnection bridges the hijacked connection with a new remote connection.
// Data will be transparently forwarded between the two connections.
// This is useful for redirecting traffic to a different destination.
func (op *ConnOperator) AttachConnection(remoteConn net.Conn) {
	op.mu.Lock()
	if op.closed || op.held || op.attached {
		op.mu.Unlock()
		return
	}
	op.attached = true
	op.attachedTo = remoteConn
	op.mu.Unlock()

	log.Infof("tcpmitm: attaching connection %s to %s", op.flow.String(), remoteConn.RemoteAddr().String())

	// Start bidirectional forwarding
	go op.bridgeConnections(op.hijackedConn, remoteConn)
}

// Hold indicates that the connection is being handled elsewhere.
// No further automatic processing will be done on this connection.
// Use this when you want to take full control of the connection.
func (op *ConnOperator) Hold() {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.held = true
	log.Infof("tcpmitm: holding connection %s for external handling", op.flow.String())
}

// GetHijackedConn returns the hijacked connection for manual handling.
// Note: After calling this, you are responsible for closing the connection.
func (op *ConnOperator) GetHijackedConn() net.Conn {
	return op.hijackedConn
}

// GetFlow returns the connection flow information.
func (op *ConnOperator) GetFlow() *ConnectionFlow {
	return op.flow
}

// IsHeld returns whether the connection is being held for external handling.
func (op *ConnOperator) IsHeld() bool {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.held
}

// IsClosed returns whether the connection has been closed.
func (op *ConnOperator) IsClosed() bool {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.closed
}

// IsAttached returns whether the connection has been attached to another connection.
func (op *ConnOperator) IsAttached() bool {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.attached
}

// bridgeConnections creates a bidirectional bridge between two connections.
func (op *ConnOperator) bridgeConnections(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// conn1 -> conn2
	go func() {
		defer wg.Done()
		_, err := io.Copy(conn2, conn1)
		if err != nil && err != io.EOF {
			log.Debugf("tcpmitm: bridge conn1->conn2 error: %v", err)
		}
	}()

	// conn2 -> conn1
	go func() {
		defer wg.Done()
		_, err := io.Copy(conn1, conn2)
		if err != nil && err != io.EOF {
			log.Debugf("tcpmitm: bridge conn2->conn1 error: %v", err)
		}
	}()

	wg.Wait()

	// Close both connections when done
	conn1.Close()
	conn2.Close()

	log.Debugf("tcpmitm: bridge completed for %s", op.flow.String())
}
