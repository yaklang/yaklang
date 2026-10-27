package sharkcli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	cli "github.com/yaklang/yaklang/common/urfavecli"
)

const decoderEnvironment = "YAK_SHARK_WORKER"
const decoderFrame = "\x1eYAKSHARK:"

const decoderInputLimit = 2 << 20
const decoderOutputLimit = 8 << 20

type decodeRequest struct {
	ByteFields         []workerByteField    `json:",omitempty"`
	TransportView      *packetTransportView `json:",omitempty"`
	TransportHeader    []byte               `json:",omitempty"`
	ConnectionStack    []string             `json:",omitempty"`
	ConnectionProtocol string               `json:",omitempty"`
	ConnectionEvidence string               `json:",omitempty"`
	OpaqueTransport    bool                 `json:",omitempty"`
	Events             []*pcaputil.ProtocolEvent
	Stream             *streamDecodeRequest
	Number             uint64
	Data               []byte
	Length             int
	Link               layers.LinkType
}
type streamDecodeRequest struct {
	Events []*pcaputil.ProtocolEvent
	ID     uint64
	Ports  [2]uint16
	Prefix [2][]byte
}
type decodeField struct {
	Depth      int
	Text       string
	Branch     bool
	Start, End int
	HasRange   bool
	Source     int
}
type decodeResponse struct {
	Protocol string
	Number   uint64
	Fields   []decodeField
	Sources  []detailSource
}

var WorkerCommand = &cli.Command{
	Name: "shark-worker", Hidden: true,
	Action: func(*cli.Context) error {
		if os.Getenv(decoderEnvironment) != "1" {
			return fmt.Errorf("shark-worker is an internal command")
		}
		// The parent consumes stdout as a framed protocol. No dissector can write to
		// the terminal, including VM diagnostics that bypass the logging package.
		log.SetOutput(io.Discard)
		return runDecodeWorker(os.Stdin, os.Stdout)
	},
}

func runDecodeWorker(input io.Reader, output io.Writer) error {
	var req decodeRequest
	decoder := json.NewDecoder(io.LimitReader(input, decoderInputLimit))
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if len(req.Data) > 262144 {
		return fmt.Errorf("packet exceeds decoder limit")
	}
	events := req.Events
	if req.Stream != nil {
		events = req.Stream.Events
	}
	if err := restoreWorkerByteFields(events, req.ByteFields); err != nil {
		return err
	}
	var d packetDetail
	if req.Stream != nil {
		d = decodeStream(*req.Stream)
	} else {
		raw := &capturedPacket{events: req.Events, number: req.Number, data: req.Data, link: req.Link, application: req.ConnectionProtocol, evidence: req.ConnectionEvidence, connectionStack: req.ConnectionStack, opaqueTransport: req.OpaqueTransport, ci: gopacket.CaptureInfo{Length: req.Length, CaptureLength: len(req.Data)}}
		if req.TransportView != nil {
			v := req.TransportView
			if len(req.TransportHeader) != v.HeaderLength || v.Protocol == "tcp" && (v.HeaderLength < 20 || v.HeaderLength > 60 || int(req.TransportHeader[12]>>4)*4 != v.HeaderLength) || v.Protocol == "udp" && v.HeaderLength != 8 || v.Protocol != "tcp" && v.Protocol != "udp" {
				return fmt.Errorf("invalid observed transport header in decoder request")
			}
			v.Header = req.TransportHeader // JSON decoding owns this logical byte source.
			raw.transportView = v
		}
		d = detail(raw)
	}
	response := decodeResponse{Number: d.number, Protocol: d.protocol, Sources: d.sources}
	for _, f := range d.fields {
		response.Fields = append(response.Fields, decodeField{Depth: f.depth, Text: f.text, Branch: f.branch, Start: f.start, End: f.end, HasRange: f.hasRange, Source: f.source})
	}
	payload, err := marshalDecodeResponse(response)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "\n%s%s\n", decoderFrame, payload)
	return err
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > decoderOutputLimit {
		return 0, fmt.Errorf("decoder output exceeds 8 MiB")
	}
	return b.Buffer.Write(p)
}

// Only pinned packets use the VM. A bounded child process keeps both parser
// diagnostics and pathological input away from the capture/UI event loops.
func decodeIsolated(parent context.Context, raw *capturedPacket) (packetDetail, error) {
	output, err := runIsolated(parent, packetDecodeRequest(raw))
	if err != nil {
		return packetDetail{}, err
	}
	return decodeWorkerOutput(output, raw)
}

func packetDecodeRequest(raw *capturedPacket) decodeRequest {
	req := decodeRequest{Events: raw.protocolEvents(), Number: raw.number, Data: raw.data, Length: raw.ci.Length, Link: raw.link, ConnectionStack: raw.connectionStack, ConnectionProtocol: raw.application, ConnectionEvidence: raw.evidence, OpaqueTransport: raw.opaqueTransport, TransportView: raw.transportView}
	if raw.transportView != nil {
		req.TransportHeader = raw.transportView.Header
	}
	return req
}

func decodeStreamIsolated(parent context.Context, stream *streamSnapshot) (packetDetail, error) {
	if stream.events == nil {
		stream.events = []*pcaputil.ProtocolEvent{}
	}
	output, err := runIsolated(parent, decodeRequest{Stream: &streamDecodeRequest{ID: stream.id, Ports: stream.ports, Events: stream.events, Prefix: stream.prefix}})
	if err != nil {
		return packetDetail{}, err
	}
	return decodeWorkerOutput(output, &capturedPacket{number: stream.id, link: layers.LinkTypeEthernet})
}

func runIsolated(parent context.Context, request decodeRequest) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	req, err := marshalDecodeRequest(request)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, executable, "shark-worker")
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, decoderEnvironment+"=") && !strings.HasPrefix(env, "YAKLANGDEBUG=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, decoderEnvironment+"=1")
	cmd.Stdin = bytes.NewReader(req)
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("deep decoder exited: %w", err)
	}
	return output.Bytes(), nil
}

func marshalDecodeRequest(request decodeRequest) ([]byte, error) {
	// JSON escapes and repeated maps are charged as transmitted bytes. Work
	// on shallow copies: history snapshots still belong to their callers.
	request.ByteFields = nil // Parent facts are regenerated from actual Go types.
	var events []*pcaputil.ProtocolEvent
	if request.Stream != nil {
		stream := *request.Stream
		request.Stream = &stream
		events, stream.Events = stream.Events, nil
	} else {
		events, request.Events = request.Events, nil
	}
	base, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	remaining := decoderInputLimit - len(base) - 64
	var bounded []*pcaputil.ProtocolEvent
	if events != nil {
		bounded = make([]*pcaputil.ProtocolEvent, len(events))
	}
	markerCosts := make([]int, len(events))
	for i, e := range events {
		if e == nil {
			markerCosts[i] = 4
			remaining -= 5
			continue
		}
		marker := &pcaputil.ProtocolEvent{ID: e.ID, FlowID: e.FlowID, TCPFlowID: e.TCPFlowID, ResponseTo: e.ResponseTo, Protocol: e.Protocol[:min(len(e.Protocol), 128)], Status: "limited", Completeness: "limited", Summary: "Detail unavailable: IPC byte/type projection limit", Error: "Detail unavailable: IPC byte/type projection limit", ExpertCode: string(pcaputil.ErrResourceExceeded), Domain: pcaputil.CaptureDomain{Section: e.Domain.Section, Interface: e.Domain.Interface}}
		wire, err := json.Marshal(marker)
		if err != nil {
			return nil, err
		}
		bounded[i], markerCosts[i] = marker, len(wire)
		remaining -= len(wire) + 1
	}
	if remaining < 0 {
		return nil, fmt.Errorf("decoder request metadata exceeds IPC input limit")
	}
	for i, e := range events {
		if e == nil {
			continue
		}
		wire, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		types, typeErr := eventByteFields(e, i)
		if typeErr != nil || len(request.ByteFields)+len(types) > workerByteFieldLimit {
			continue // The explicit IPC-limited marker remains, never untyped success.
		}
		typeCost := 0
		for _, descriptor := range types {
			encoded, err := json.Marshal(descriptor)
			if err != nil {
				return nil, err
			}
			typeCost += len(encoded) + 1
		}
		delta := len(wire) - markerCosts[i] + typeCost
		if delta <= remaining {
			bounded[i] = e
			request.ByteFields = append(request.ByteFields, types...)
			remaining -= delta
		}
	}
	if request.Stream != nil {
		request.Stream.Events = bounded
	} else {
		request.Events = bounded
	}
	wire, err := json.Marshal(request)
	if err == nil && len(wire) > decoderInputLimit {
		return nil, fmt.Errorf("decoder request exceeds IPC input limit")
	}
	return wire, err
}

func marshalDecodeResponse(response decodeResponse) ([]byte, error) {
	fields := response.Fields
	response.Fields = nil
	base, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	marker := decodeField{Text: "Detail truncated: IPC encoded byte budget"}
	markerWire, err := json.Marshal(marker)
	if err != nil {
		return nil, err
	}
	remaining := decoderOutputLimit - len(decoderFrame) - 3 - len(base) - len(markerWire) - 64
	for _, field := range fields {
		wire, err := json.Marshal(field)
		if err != nil {
			return nil, err
		}
		if len(wire)+1 > remaining {
			response.Fields = append(response.Fields, marker)
			break
		}
		remaining -= len(wire) + 1
		response.Fields = append(response.Fields, field)
	}
	wire, err := json.Marshal(response)
	if err == nil && len(wire)+len(decoderFrame)+3 > decoderOutputLimit {
		return nil, fmt.Errorf("decoder response metadata exceeds IPC output limit")
	}
	return wire, err
}

func decodeWorkerOutput(output []byte, raw *capturedPacket) (packetDetail, error) {
	at := bytes.LastIndex(output, []byte(decoderFrame))
	if at < 0 {
		return packetDetail{}, fmt.Errorf("deep decoder returned no result")
	}
	var response decodeResponse
	if err := json.Unmarshal(bytes.TrimSpace(output[at+len(decoderFrame):]), &response); err != nil {
		return packetDetail{}, err
	}
	if response.Number != raw.number {
		return packetDetail{}, fmt.Errorf("deep decoder returned another packet")
	}
	d := quickDetail(raw)
	d.protocol = response.Protocol
	// Native packet identity is frozen before IPC. The child receives public
	// field snapshots, not the private authentication/ALPN proof; its raw UDP
	// fallback must not overwrite that observed HTTP3 identity on a diagnostic.
	if raw.presentation.protocol == "HTTP/3" && (raw.presentation.scope == "authenticated application bytes" || raw.presentation.scope == "connection protocol violation observed") {
		d.protocol = raw.presentation.protocol
	}
	d.fields = nil
	d.sources = response.Sources
	for i := range d.sources {
		d.sources[i].Name = safeText(d.sources[i].Name)
	}
	for _, f := range response.Fields {
		length := len(raw.data)
		validSource := f.Source == 0
		if f.Source > 0 && f.Source <= len(d.sources) {
			length, validSource = len(d.sources[f.Source-1].Data), true
		}
		d.fields = append(d.fields, field{depth: f.Depth, text: safeText(f.Text), branch: f.Branch, start: f.Start, end: f.End, source: f.Source, hasRange: f.HasRange && validSource && f.Start >= 0 && f.End > f.Start && f.End <= length})
	}
	return d, nil
}

// Apply the branch's existing TCP payload dispatcher to a contiguous stream
// prefix. Synthetic transport fields are never shown as captured facts.
func decodeStream(req streamDecodeRequest) (d packetDetail) {
	d.number = req.ID
	if req.Events != nil {
		d.fields, d.sources = protocolEventDetail(req.Events)
		d.protocol = eventProtocols(req.Events)
		return
	}
	defer func() {
		if v := recover(); v != nil {
			d.fields = append(d.fields, field{text: fmt.Sprintf("Stream dissector incomplete: %v", v)})
		}
	}()
	for direction, data := range req.Prefix {
		if len(data) == 0 {
			continue
		}
		if len(data) > streamPrefixLimit {
			data = data[:streamPrefixLimit]
		}
		label := "A → B"
		if direction == 1 {
			label = "B → A"
		}
		d.fields = append(d.fields, field{branch: true, text: label + " · contiguous prefix (up to 16 KiB)"})
		header := make([]byte, 20, 20+len(data))
		binary.BigEndian.PutUint16(header[:2], req.Ports[direction])
		binary.BigEndian.PutUint16(header[2:4], req.Ports[1-direction])
		header[12], header[13] = 0x50, 0x18
		verified := sniffStreamPayload(data, req.Ports[direction], req.Ports[1-direction])
		rules := map[string][2]string{
			"HTTP": {"application-layer.http", "HTTP"}, "TLS": {"application-layer.tls", "Transport Layer Security"},
			"HTTP2": {"application-layer.http2", "HTTP2Preface"}, "SSH": {"application-layer.ssh", "SSH"},
			"Redis": {"application-layer.redis", "RESP"}, "MQTT": {"application-layer.mqtt", "MQTT"},
			"SMB2": {"application-layer.smb2", "SMB2"}, "AMQP": {"amqp", "AMQP"}, "VNC": {"vnc", "VNC"},
			"DNS": {"application-layer.dns", "DNS"},
		}
		var node *base.Node
		var err error
		if rule, ok := rules[verified]; ok {
			input := data
			if verified == "DNS" {
				input = data[2:]
			}
			if verified == "SMB2" && !bytes.HasPrefix(input, []byte{0xfe, 'S', 'M', 'B'}) {
				input = input[4:]
			}
			node, err = parser.ParseBinary(bytes.NewReader(input), rule[0], rule[1])
		} else {
			node, err = parser.ParseBinary(bytes.NewReader(append(header, data...)), "transmission_control_protocol", "TCP")
		}
		if err == nil {
			var value *base.NodeValue
			value, err = node.Result()
			if err == nil && value != nil {
				if app := findApplication(value); app != nil {
					candidate := applicationNodes[app.Name]
					// Some YAML rules accept structurally valid random bytes. Require
					// a signature or matching port before advertising their identity.
					identified := ""
					if verified != "" && verified == candidate {
						identified = verified
					} else if candidate != "TLS" && candidate != "HTTP" && candidate != "SSH" && candidate == portProtocol("tcp", req.Ports[0], req.Ports[1]) {
						identified = candidate
					}
					if identified != "" {
						d.protocol = identified
						flattenFields(app, 0, &d.fields)
						continue
					}
				}
			}
		}
		message := "No verified application rule for this prefix; inspect stream text/hex."
		if err != nil {
			message = "Application decode incomplete (split/truncated/unknown data); inspect stream text/hex."
		}
		d.fields = append(d.fields, field{text: message})
	}
	return d
}
func findApplication(n *base.NodeValue) *base.NodeValue {
	if !n.IsValue() && applicationNodes[n.Name] != "" {
		return n
	}
	for _, child := range n.Children() {
		if v := findApplication(child); v != nil {
			return v
		}
	}
	return nil
}
