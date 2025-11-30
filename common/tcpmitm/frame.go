package tcpmitm

import (
	"bytes"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// FrameDirection indicates the direction of data flow.
type FrameDirection int

const (
	// DirectionClientToServer indicates data flowing from client to server.
	DirectionClientToServer FrameDirection = iota
	// DirectionServerToClient indicates data flowing from server to client.
	DirectionServerToClient
)

func (d FrameDirection) String() string {
	switch d {
	case DirectionClientToServer:
		return "client->server"
	case DirectionServerToClient:
		return "server->client"
	default:
		return "unknown"
	}
}

// Frame represents a chunk of TCP data that has been segmented.
// It provides methods to inspect, modify, and control the data flow.
type Frame struct {
	mu sync.RWMutex

	// raw data bytes
	rawBytes []byte

	// direction of the frame
	direction FrameDirection

	// timestamp when this frame was captured
	timestamp time.Time

	// flags for frame control
	dropped     bool
	forwarded   bool
	modified    bool
	injectQueue [][]byte

	// injector function for immediate injection
	injector func([]byte) error

	// bin-parser cached result
	parsedNode     *base.Node
	parsedProtocol Protocol
	parseError     error
	parsed         bool

	// unknown protocol flag
	unknownProtocol bool
}

// NewFrame creates a new Frame with the given data and direction.
func NewFrame(data []byte, direction FrameDirection, injector func([]byte) error) *Frame {
	// Make a copy of data to avoid external modification
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	return &Frame{
		rawBytes:        dataCopy,
		direction:       direction,
		timestamp:       time.Now(),
		dropped:         false,
		forwarded:       false,
		modified:        false,
		injectQueue:     nil,
		injector:        injector,
		parsed:          false,
		unknownProtocol: false,
	}
}

// GetRawBytes returns a copy of the raw data bytes.
func (f *Frame) GetRawBytes() []byte {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make([]byte, len(f.rawBytes))
	copy(result, f.rawBytes)
	return result
}

// SetRawBytes modifies the frame's data.
// This marks the frame as modified and invalidates cached parse results.
func (f *Frame) SetRawBytes(modified []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.rawBytes = make([]byte, len(modified))
	copy(f.rawBytes, modified)
	f.modified = true

	// Invalidate cached parse results
	f.parsed = false
	f.parsedNode = nil
	f.parseError = nil
}

// Drop marks the frame to be dropped (not forwarded).
// A dropped frame will not be sent to its destination.
func (f *Frame) Drop() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.dropped = true
}

// Forward explicitly marks the frame to be forwarded.
// This is the default behavior if neither Drop nor Forward is called.
func (f *Frame) Forward() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.forwarded = true
}

// Inject queues data to be sent immediately before this frame is forwarded.
// Multiple calls to Inject will queue multiple payloads in order.
func (f *Frame) Inject(data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	f.injectQueue = append(f.injectQueue, dataCopy)
}

// InjectImmediate sends data immediately to the target without waiting.
// This bypasses the normal frame processing queue.
func (f *Frame) InjectImmediate(data []byte) error {
	if f.injector != nil {
		return f.injector(data)
	}
	return nil
}

// GetDirection returns the direction of this frame.
func (f *Frame) GetDirection() FrameDirection {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.direction
}

// GetTimestamp returns when this frame was captured.
func (f *Frame) GetTimestamp() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.timestamp
}

// IsDropped returns whether this frame is marked for dropping.
func (f *Frame) IsDropped() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.dropped
}

// IsModified returns whether this frame has been modified.
func (f *Frame) IsModified() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.modified
}

// Size returns the current size of the frame data.
func (f *Frame) Size() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.rawBytes)
}

// IsUnknownProtocol returns true if the frame's protocol could not be identified.
func (f *Frame) IsUnknownProtocol() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.unknownProtocol
}

// SetUnknownProtocol marks this frame as having an unknown protocol.
func (f *Frame) SetUnknownProtocol(unknown bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unknownProtocol = unknown
}

// GetDetectedProtocol returns the detected protocol type.
func (f *Frame) GetDetectedProtocol() Protocol {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.parsedProtocol
}

// GetBinParserNode attempts to parse the frame data using bin-parser.
// The result is cached - subsequent calls return the cached result.
// Returns the parsed node and any error encountered during parsing.
func (f *Frame) GetBinParserNode() (*base.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Return cached result if already parsed
	if f.parsed {
		return f.parsedNode, f.parseError
	}

	f.parsed = true

	// Detect protocol first
	detector := NewProtocolDetector()
	protocol := detector.Detect(f.rawBytes)
	f.parsedProtocol = protocol

	// Map protocol to bin-parser rule
	ruleName := protocolToBinParserRule(protocol)
	if ruleName == "" {
		f.unknownProtocol = true
		f.parseError = nil
		return nil, nil
	}

	// Parse using bin-parser
	reader := bytes.NewReader(f.rawBytes)
	node, err := parser.ParseBinary(reader, ruleName)
	if err != nil {
		f.parseError = err
		f.unknownProtocol = true
		return nil, err
	}

	f.parsedNode = node
	f.parseError = nil
	return node, nil
}

// GetBinParserNodeWithRule parses the frame data using a specific bin-parser rule.
// The result is NOT cached to allow re-parsing with different rules.
func (f *Frame) GetBinParserNodeWithRule(rule string, keys ...string) (*base.Node, error) {
	f.mu.RLock()
	data := make([]byte, len(f.rawBytes))
	copy(data, f.rawBytes)
	f.mu.RUnlock()

	reader := bytes.NewReader(data)
	return parser.ParseBinary(reader, rule, keys...)
}

// protocolToBinParserRule maps detected protocol to bin-parser rule name.
func protocolToBinParserRule(protocol Protocol) string {
	switch protocol {
	case ProtocolHTTP:
		return "application-layer.http"
	case ProtocolTLS, ProtocolHTTPS:
		return "application-layer.tls"
	case ProtocolDNS:
		return "application-layer.dns"
	case ProtocolSSH:
		// SSH is encrypted, no bin-parser rule available
		return ""
	case ProtocolRedis:
		// Redis RESP protocol - no specific rule yet
		return ""
	case ProtocolMySQL:
		// MySQL protocol - no specific rule yet
		return ""
	case ProtocolPostgreSQL:
		// PostgreSQL protocol - no specific rule yet
		return ""
	case ProtocolMongoDB:
		// MongoDB protocol - no specific rule yet
		return ""
	case ProtocolSMTP:
		// SMTP protocol - no specific rule yet
		return ""
	case ProtocolFTP:
		// FTP protocol - no specific rule yet
		return ""
	default:
		return ""
	}
}

// getInjectQueue returns the queued injection data (internal use).
func (f *Frame) getInjectQueue() [][]byte {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.injectQueue
}

// shouldForward returns whether this frame should be forwarded (internal use).
func (f *Frame) shouldForward() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return !f.dropped
}

// getFinalBytes returns the bytes to be forwarded (internal use).
func (f *Frame) getFinalBytes() []byte {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.rawBytes
}
