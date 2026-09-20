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
	cli "github.com/yaklang/yaklang/common/urfavecli"
)

const decoderEnvironment = "YAK_SHARK_WORKER"
const decoderFrame = "\x1eYAKSHARK:"

type decodeRequest struct {
	Stream *streamDecodeRequest
	Number uint64
	Data   []byte
	Length int
	Link   layers.LinkType
}
type streamDecodeRequest struct {
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
}
type decodeResponse struct {
	Protocol string
	Number   uint64
	Fields   []decodeField
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
	if err := json.NewDecoder(io.LimitReader(input, 2<<20)).Decode(&req); err != nil {
		return err
	}
	if len(req.Data) > 262144 {
		return fmt.Errorf("packet exceeds decoder limit")
	}
	var d packetDetail
	if req.Stream != nil {
		d = decodeStream(*req.Stream)
	} else {
		raw := &capturedPacket{number: req.Number, data: req.Data, link: req.Link, ci: gopacket.CaptureInfo{Length: req.Length, CaptureLength: len(req.Data)}}
		d = detail(raw)
	}
	response := decodeResponse{Number: d.number, Protocol: d.protocol}
	for _, f := range d.fields {
		response.Fields = append(response.Fields, decodeField{Depth: f.depth, Text: f.text, Branch: f.branch, Start: f.start, End: f.end, HasRange: f.hasRange})
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "\n%s%s\n", decoderFrame, payload)
	return err
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 8<<20 {
		return 0, fmt.Errorf("decoder output exceeds 8 MiB")
	}
	return b.Buffer.Write(p)
}

// Only pinned packets use the VM. A bounded child process keeps both parser
// diagnostics and pathological input away from the capture/UI event loops.
func decodeIsolated(parent context.Context, raw *capturedPacket) (packetDetail, error) {
	output, err := runIsolated(parent, decodeRequest{Number: raw.number, Data: raw.data, Length: raw.ci.Length, Link: raw.link})
	if err != nil {
		return packetDetail{}, err
	}
	return decodeWorkerOutput(output, raw)
}

func decodeStreamIsolated(parent context.Context, stream *streamSnapshot) (packetDetail, error) {
	output, err := runIsolated(parent, decodeRequest{Stream: &streamDecodeRequest{ID: stream.id, Ports: stream.ports, Prefix: stream.prefix}})
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
	req, err := json.Marshal(request)
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
	d.fields = nil
	for _, f := range response.Fields {
		d.fields = append(d.fields, field{depth: f.Depth, text: safeText(f.Text), branch: f.Branch, start: f.Start, end: f.End, hasRange: f.HasRange && f.Start >= 0 && f.End > f.Start && f.End <= len(raw.data)})
	}
	return d, nil
}

// Apply the branch's existing TCP payload dispatcher to a contiguous stream
// prefix. Synthetic transport fields are never shown as captured facts.
func decodeStream(req streamDecodeRequest) (d packetDetail) {
	d.number = req.ID
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
