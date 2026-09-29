package inputresolver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

const maxBinaryInputBytes = 16 << 20
const maxCapturePacketBytes = 1 << 20
const maxCapturePackets = 512

// readBinaryLocked shares the manifest and beneath-root checks of ordinary
// input reads. Callers hold the workspace read lock throughout parsing.
func (w *Workspace) readBinaryLocked(ctx context.Context, name string) ([]byte, map[string]any, error) {
	if err := w.check(ctx); err != nil {
		return nil, nil, err
	}
	resource, err := w.resource(name)
	if err != nil {
		return nil, nil, err
	}
	if resource.SizeBytes > maxBinaryInputBytes {
		return nil, nil, fail("input_binary_limit", resource.ResourceId)
	}
	file, err := w.open(resource)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	var data bytes.Buffer
	block := make([]byte, 32<<10)
	for {
		if err := w.check(ctx); err != nil {
			return nil, nil, err
		}
		n, readErr := file.Read(block)
		if data.Len()+n > maxBinaryInputBytes {
			return nil, nil, fail("input_binary_limit", resource.ResourceId)
		}
		data.Write(block[:n])
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, nil, fail("input_read_failed", resource.ResourceId)
		}
	}
	digest := sha256.Sum256(data.Bytes())
	hash := hex.EncodeToString(digest[:])
	if uint64(data.Len()) != resource.SizeBytes || hash != resource.Sha256 {
		return nil, nil, fail("input_file_changed", resource.ResourceId)
	}
	return data.Bytes(), map[string]any{"path": resource.RelativePath, "resource_id": resource.ResourceId, "sha256": hash, "size_bytes": resource.SizeBytes}, nil
}

// ParsePacketCapture extracts packet headers only, without reassembly, payload
// disclosure, decryption, DNS resolution or any network access.
func (w *Workspace) ParsePacketCapture(ctx context.Context, name string) (map[string]any, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	data, result, err := w.readBinaryLocked(ctx, name)
	if err != nil {
		return nil, err
	}
	invalid := func() (map[string]any, error) {
		return nil, fail("input_capture_invalid", result["resource_id"].(string))
	}
	format, err := validateCapture(ctx, data)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fail("input_cancelled", result["resource_id"].(string))
		}
		return invalid()
	}
	var read func() ([]byte, gopacket.CaptureInfo, error)
	var link layers.LinkType
	if format == "pcapng" {
		reader, e := pcapgo.NewNgReader(bytes.NewReader(data), pcapgo.NgReaderOptions{WantMixedLinkType: true})
		if e != nil {
			return invalid()
		}
		read = reader.ReadPacketData
	} else {
		reader, e := pcapgo.NewReader(bytes.NewReader(data))
		if e != nil {
			return invalid()
		}
		read = reader.ReadPacketData
		link = reader.LinkType()
	}
	observations := make([]map[string]any, 0)
	protocols := map[string]int{}
	truncated := false
	for {
		if err := w.check(ctx); err != nil {
			return nil, err
		}
		packetData, ci, e := read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return invalid()
		}
		if len(observations) >= maxCapturePackets {
			truncated = true
			break
		}
		if format == "pcapng" {
			if len(ci.AncillaryData) == 0 {
				return invalid()
			}
			var ok bool
			link, ok = ci.AncillaryData[0].(layers.LinkType)
			if !ok {
				return invalid()
			}
		}
		packet := gopacket.NewPacket(packetData, link, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		if len(packet.Layers()) > 32 {
			return invalid()
		}
		item := map[string]any{"index": len(observations) + 1, "captured_bytes": ci.CaptureLength, "wire_bytes": ci.Length, "link_type": link.String()}
		names := []string{}
		for _, layer := range packet.Layers() {
			n := layer.LayerType().String()
			if n == "Payload" || n == "DecodeFailure" {
				continue
			}
			names = append(names, n)
			protocols[n]++
		}
		item["protocols"] = names
		if net := packet.NetworkLayer(); net != nil {
			item["source_ip"] = net.NetworkFlow().Src().String()
			item["destination_ip"] = net.NetworkFlow().Dst().String()
		}
		if transport := packet.TransportLayer(); transport != nil {
			item["source_port"] = transport.TransportFlow().Src().String()
			item["destination_port"] = transport.TransportFlow().Dst().String()
		}
		item["decode_error"] = packet.ErrorLayer() != nil
		observations = append(observations, item)
	}
	result["format"] = format
	result["packets"] = observations
	result["packet_count"] = len(observations)
	result["protocol_counts"] = protocols
	result["truncated"] = truncated
	result["limitations"] = []string{"header observations only; no stream reassembly or vulnerability verdict", "at most 512 packets; input at most 16 MiB"}
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	w.event("input.file.access", Event{ResourceID: result["resource_id"].(string), Path: result["path"].(string), Operation: "parse_packet_capture", BytesRead: int64(len(data))})
	return result, nil
}

// Validate lengths before pcapgo can allocate buffers based on attacker-owned
// snap lengths or packet sizes. Compressed captures are deliberately excluded.
func validateCapture(ctx context.Context, data []byte) (string, error) {
	bad := fmt.Errorf("invalid capture")
	if len(data) < 24 {
		return "", bad
	}
	var order binary.ByteOrder = binary.LittleEndian
	magic := binary.LittleEndian.Uint32(data)
	if magic != 0x0a0d0d0a {
		switch magic {
		case 0xa1b2c3d4, 0xa1b23c4d:
		case 0xd4c3b2a1, 0x4d3cb2a1:
			order = binary.BigEndian
		default:
			return "", bad
		}
		if order.Uint32(data[16:]) > maxCapturePacketBytes {
			return "", bad
		}
		for pos := 24; pos < len(data); {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if len(data)-pos < 16 {
				return "", bad
			}
			n := int(order.Uint32(data[pos+8:]))
			if n > maxCapturePacketBytes || n > len(data)-pos-16 {
				return "", bad
			}
			pos += 16 + n
		}
		return "pcap", nil
	}
	for pos := 0; pos < len(data); {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if len(data)-pos < 12 {
			return "", bad
		}
		if binary.LittleEndian.Uint32(data[pos:]) == 0x0a0d0d0a {
			switch binary.LittleEndian.Uint32(data[pos+8:]) {
			case 0x1a2b3c4d:
				order = binary.LittleEndian
			case 0x4d3c2b1a:
				order = binary.BigEndian
			default:
				return "", bad
			}
		}
		n := int(order.Uint32(data[pos+4:]))
		if n < 12 || n%4 != 0 || n > len(data)-pos || order.Uint32(data[pos+n-4:]) != uint32(n) {
			return "", bad
		}
		block := data[pos : pos+n]
		switch order.Uint32(block) {
		case 0x0a0d0d0a:
			if n < 28 {
				return "", bad
			}
		case 1:
			if n < 20 || order.Uint32(block[12:]) > maxCapturePacketBytes {
				return "", bad
			}
		case 6:
			if n < 32 {
				return "", bad
			}
			caplen := int(order.Uint32(block[20:]))
			if caplen > maxCapturePacketBytes || caplen > n-32 {
				return "", bad
			}
		case 3:
			if n < 16 || order.Uint32(block[8:]) > maxCapturePacketBytes {
				return "", bad
			}
		default:
			return "", bad // unsupported metadata blocks fail explicitly
		}
		pos += n
	}
	return "pcapng", nil
}
