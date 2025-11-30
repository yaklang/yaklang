package tcpmitm

import (
	"bytes"
)

// Protocol represents a detected protocol type.
type Protocol int

const (
	ProtocolUnknown Protocol = iota
	ProtocolHTTP
	ProtocolHTTPS
	ProtocolTLS
	ProtocolSSH
	ProtocolRedis
	ProtocolMySQL
	ProtocolPostgreSQL
	ProtocolMongoDB
	ProtocolSMTP
	ProtocolFTP
	ProtocolDNS
)

func (p Protocol) String() string {
	switch p {
	case ProtocolHTTP:
		return "HTTP"
	case ProtocolHTTPS:
		return "HTTPS"
	case ProtocolTLS:
		return "TLS"
	case ProtocolSSH:
		return "SSH"
	case ProtocolRedis:
		return "Redis"
	case ProtocolMySQL:
		return "MySQL"
	case ProtocolPostgreSQL:
		return "PostgreSQL"
	case ProtocolMongoDB:
		return "MongoDB"
	case ProtocolSMTP:
		return "SMTP"
	case ProtocolFTP:
		return "FTP"
	case ProtocolDNS:
		return "DNS"
	default:
		return "Unknown"
	}
}

// ProtocolDetector detects the protocol from initial bytes.
type ProtocolDetector struct{}

// NewProtocolDetector creates a new protocol detector.
func NewProtocolDetector() *ProtocolDetector {
	return &ProtocolDetector{}
}

// Detect attempts to identify the protocol from the first few bytes.
func (pd *ProtocolDetector) Detect(data []byte) Protocol {
	if len(data) == 0 {
		return ProtocolUnknown
	}

	// TLS/SSL: starts with 0x16 0x03 (TLS handshake)
	if len(data) >= 2 && data[0] == 0x16 && data[1] == 0x03 {
		return ProtocolTLS
	}

	// SSH: starts with "SSH-"
	if bytes.HasPrefix(data, []byte("SSH-")) {
		return ProtocolSSH
	}

	// HTTP methods
	httpMethods := [][]byte{
		[]byte("GET "),
		[]byte("POST "),
		[]byte("PUT "),
		[]byte("DELETE "),
		[]byte("HEAD "),
		[]byte("OPTIONS "),
		[]byte("PATCH "),
		[]byte("CONNECT "),
		[]byte("TRACE "),
	}
	for _, method := range httpMethods {
		if bytes.HasPrefix(data, method) {
			return ProtocolHTTP
		}
	}

	// HTTP response
	if bytes.HasPrefix(data, []byte("HTTP/")) {
		return ProtocolHTTP
	}

	// Redis RESP protocol
	// Commands start with * (array) or inline commands
	if len(data) >= 1 {
		switch data[0] {
		case '*', '+', '-', ':', '$':
			// Likely Redis RESP
			if isLikelyRedis(data) {
				return ProtocolRedis
			}
		}
	}

	// MySQL: greeting packet starts with protocol version
	if len(data) >= 5 && data[4] == 0x0a {
		return ProtocolMySQL
	}

	// PostgreSQL: startup message or SSL request
	if len(data) >= 8 {
		// SSL request: length(8) + code(80877103)
		if data[0] == 0 && data[1] == 0 && data[2] == 0 && data[3] == 8 &&
			data[4] == 0x04 && data[5] == 0xd2 && data[6] == 0x16 && data[7] == 0x2f {
			return ProtocolPostgreSQL
		}
	}

	// MongoDB: OP_MSG or OP_QUERY
	if len(data) >= 16 {
		// Check for MongoDB wire protocol opcodes
		opcode := int(data[12]) | int(data[13])<<8 | int(data[14])<<16 | int(data[15])<<24
		if opcode == 2013 || opcode == 2004 { // OP_MSG or OP_QUERY
			return ProtocolMongoDB
		}
	}

	// SMTP: greeting or commands
	smtpPrefixes := [][]byte{
		[]byte("220 "),      // Server greeting
		[]byte("EHLO "),     // Extended HELLO
		[]byte("HELO "),     // HELLO
		[]byte("MAIL FROM"), // Mail from
		[]byte("RCPT TO"),   // Recipient
	}
	for _, prefix := range smtpPrefixes {
		if bytes.HasPrefix(data, prefix) {
			return ProtocolSMTP
		}
	}

	// FTP: commands or responses
	ftpPrefixes := [][]byte{
		[]byte("220 "), // FTP server ready
		[]byte("USER "),
		[]byte("PASS "),
		[]byte("QUIT"),
		[]byte("TYPE "),
	}
	for _, prefix := range ftpPrefixes {
		if bytes.HasPrefix(data, prefix) {
			return ProtocolFTP
		}
	}

	return ProtocolUnknown
}

// isLikelyRedis performs additional checks for Redis RESP protocol.
func isLikelyRedis(data []byte) bool {
	if len(data) < 3 {
		return false
	}

	// Check if it looks like a valid RESP message
	switch data[0] {
	case '*': // Array
		// Should be followed by a number and CRLF
		for i := 1; i < len(data) && i < 10; i++ {
			if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
				return true
			}
			if data[i] < '0' || data[i] > '9' {
				if data[i] != '-' {
					return false
				}
			}
		}
	case '+', '-', ':': // Simple string, Error, Integer
		// Should contain CRLF
		return bytes.Contains(data, []byte("\r\n"))
	case '$': // Bulk string
		// Should be followed by length and CRLF
		for i := 1; i < len(data) && i < 10; i++ {
			if data[i] == '\r' {
				return true
			}
			if data[i] < '0' || data[i] > '9' {
				if data[i] != '-' {
					return false
				}
			}
		}
	}

	return false
}

// DetectFromFrame detects protocol from a frame.
func (pd *ProtocolDetector) DetectFromFrame(frame *Frame) Protocol {
	return pd.Detect(frame.GetRawBytes())
}

// IsEncrypted returns whether the detected protocol uses encryption.
func (pd *ProtocolDetector) IsEncrypted(protocol Protocol) bool {
	switch protocol {
	case ProtocolTLS, ProtocolHTTPS, ProtocolSSH:
		return true
	default:
		return false
	}
}
