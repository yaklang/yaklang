package tcpmitm

import (
	"fmt"
	"net"
)

// ConnectionFlow represents the flow information of a TCP connection.
// It contains the client and server endpoint information.
type ConnectionFlow struct {
	clientIP   string
	clientPort int
	serverIP   string
	serverPort int

	// raw connection for internal use
	conn net.Conn
}

// NewConnectionFlow creates a new ConnectionFlow from the given connection.
// For hijacked connections from TUN device:
// - RemoteAddress/RemotePort represents the client (the one initiating the connection)
// - LocalAddress/LocalPort represents the server (the original destination)
func NewConnectionFlow(conn net.Conn, clientIP string, clientPort int, serverIP string, serverPort int) *ConnectionFlow {
	return &ConnectionFlow{
		clientIP:   clientIP,
		clientPort: clientPort,
		serverIP:   serverIP,
		serverPort: serverPort,
		conn:       conn,
	}
}

// GetClientIP returns the client IP address.
func (cf *ConnectionFlow) GetClientIP() string {
	return cf.clientIP
}

// GetClientPort returns the client port number.
func (cf *ConnectionFlow) GetClientPort() int {
	return cf.clientPort
}

// GetServerIP returns the server IP address (original destination).
func (cf *ConnectionFlow) GetServerIP() string {
	return cf.serverIP
}

// GetServerPort returns the server port number (original destination).
func (cf *ConnectionFlow) GetServerPort() int {
	return cf.serverPort
}

// GetServerAddr returns the server address in "ip:port" format.
func (cf *ConnectionFlow) GetServerAddr() string {
	return fmt.Sprintf("%s:%d", cf.serverIP, cf.serverPort)
}

// GetClientAddr returns the client address in "ip:port" format.
func (cf *ConnectionFlow) GetClientAddr() string {
	return fmt.Sprintf("%s:%d", cf.clientIP, cf.clientPort)
}

// String returns a string representation of the connection flow.
func (cf *ConnectionFlow) String() string {
	return fmt.Sprintf("%s -> %s", cf.GetClientAddr(), cf.GetServerAddr())
}
