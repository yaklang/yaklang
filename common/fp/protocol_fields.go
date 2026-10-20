package fp

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

const (
	maxProtocolResponseBytes = 1 << 20
	maxProtocolMessages      = 256
)

// ParseProtocolFields inspects application response bytes, without Ethernet,
// IP or TCP headers. A UDP input must be exactly one datagram. Port is only a
// hint; protocol admission and framing are validated by the bin-parser session.
//
// Each protocol key contains a []any of message maps with fields, profile,
// status and completeness. Header-only observations retain their actual status;
// unknown, malformed and incomplete messages never become successful fields.
// No extra network requests are made. The returned map is always initialized
// and owns its data, so callers may reuse raw after return.
func ParseProtocolFields(raw []byte, transport TransportProto, port int) map[string]any {
	return parseProtocolFields(context.Background(), nil, [][]byte{raw}, transport, port, false)
}

func parseProtocolFields(ctx context.Context, request []byte, responses [][]byte, transport TransportProto, port int, eof bool) (fields map[string]any) {
	fields = make(map[string]any)
	// Enrichment must not turn a parser failure into a failed service scan.
	defer func() {
		if err := recover(); err != nil {
			log.Debugf("service response protocol parser: %v", err)
		}
	}()
	if transport != TCP && transport != UDP || port < 0 || port > 65535 || ctx.Err() != nil {
		return fields
	}
	total := len(request)
	for _, response := range responses {
		if len(response) > maxProtocolResponseBytes-total {
			return fields
		}
		total += len(response)
	}
	if total <= len(request) || total > maxProtocolResponseBytes {
		return fields
	}
	options := []pcaputil.ProtocolSessionOption{
		pcaputil.WithSessionTransport(string(transport)),
		pcaputil.WithSessionPorts(0, uint16(port)),
	}
	if transport == TCP {
		options = append(options, pcaputil.WithSessionClientDirection(0))
	}
	session, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.ParserBudget{
		MaxFrameBytes: maxProtocolResponseBytes, MaxMessageBytes: maxProtocolResponseBytes,
		MaxBufferedBytes: 4 << 20, MaxRecursionDepth: 32, MaxCollectionElements: 1024,
	}, options...)
	if err != nil {
		return fields
	}
	// A scan timeout or data-size limit is not a peer FIN. Only observed EOF
	// permits the framer to finish a close-delimited response body.
	defer session.Close("servicescan")
	for remaining := request; len(remaining) > 0 && ctx.Err() == nil; {
		n := len(remaining)
		if transport == TCP {
			n = min(n, 4096)
		}
		session.Feed(0, time.Time{}, remaining[:n])
		remaining = remaining[n:]
	}
	count := 0
	collect := func(events []*pcaputil.ProtocolEvent) {
		for _, event := range events {
			if count >= maxProtocolMessages || ctx.Err() != nil {
				return
			}
			if event.Direction != 1 || event.Protocol == "" {
				continue
			}
			value, err := event.GetFields()
			if err != nil || len(value) == 0 {
				continue
			}
			message := map[string]any{
				"fields": value, "profile": event.Profile,
				"status": event.Status, "completeness": event.Completeness,
			}
			if event.Metadata != nil {
				message["metadata"] = event.Metadata
			}
			messages, _ := fields[event.Protocol].([]any)
			fields[event.Protocol] = append(messages, message)
			count++
		}
	}
	for _, response := range responses {
		for len(response) > 0 && count < maxProtocolMessages && ctx.Err() == nil {
			n := len(response)
			if transport == TCP {
				n = min(n, 4096)
			}
			collect(session.Feed(1, time.Time{}, response[:n]).Events)
			response = response[n:]
		}
	}
	reason := "servicescan"
	if eof && transport == TCP {
		reason = string(pcaputil.TrafficFlowCloseReason_FIN)
	}
	collect(session.Close(reason))
	if len(fields) == 0 && len(request) > 0 && ctx.Err() == nil {
		// A scan probe can be insufficient to establish a negotiated session.
		// Independently recognizable replies can still expose bounded fields.
		return parseProtocolFields(ctx, nil, responses, transport, port, eof)
	}
	return fields
}

func (m *MatchResult) mergeProtocolFields(fields map[string]any) {
	if m.ProtocolFields == nil {
		m.ProtocolFields = make(map[string]any)
	}
	for protocol, value := range fields {
		messages, ok := value.([]any)
		if !ok {
			continue
		}
		previous, _ := m.ProtocolFields[protocol].([]any)
		// Do not borrow a source result's slice backing array.
		merged := make([]any, 0, len(previous)+len(messages))
		merged = append(merged, previous...)
		m.ProtocolFields[protocol] = append(merged, messages...)
	}
}

// StableReader concatenates its reads. Retain UDP message boundaries separately
// and record EOF so a quiet/limited TCP prefix is never treated as a peer FIN.
type protocolResponseConn struct {
	net.Conn
	udp       bool
	readBytes atomic.Int64
	eof       atomic.Bool
	mu        sync.Mutex
	datagrams [][]byte
	retained  int
}

func (c *protocolResponseConn) Read(buffer []byte) (int, error) {
	n, err := c.Conn.Read(buffer)
	if n > 0 {
		c.readBytes.Add(int64(n))
		if c.udp {
			c.mu.Lock()
			if len(c.datagrams) < maxProtocolMessages && n <= maxProtocolResponseBytes-c.retained {
				c.datagrams = append(c.datagrams, append([]byte(nil), buffer[:n]...))
				c.retained += n
			}
			c.mu.Unlock()
		}
	}
	if err == io.EOF {
		c.eof.Store(true)
	}
	return n, err
}

func (c *protocolResponseConn) parse(ctx context.Context, request, banner []byte, transport TransportProto, port int) map[string]any {
	if !c.udp {
		eof := c.eof.Load() && c.readBytes.Load() == int64(len(banner))
		return parseProtocolFields(ctx, request, [][]byte{banner}, transport, port, eof)
	}
	c.mu.Lock()
	datagrams := append([][]byte(nil), c.datagrams...)
	c.mu.Unlock()
	// Read can be one message ahead when StableReader reaches its byte limit.
	// Only enrich from datagrams that were actually delivered in the banner.
	size, end := 0, 0
	for end < len(datagrams) && len(datagrams[end]) <= len(banner)-size {
		size += len(datagrams[end])
		end++
	}
	return parseProtocolFields(ctx, request, datagrams[:end], transport, port, false)
}
