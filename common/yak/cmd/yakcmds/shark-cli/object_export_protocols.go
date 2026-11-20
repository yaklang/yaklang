package sharkcli

import (
	"bytes"
	"fmt"
	"mime"
	"path/filepath"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func observedGRPCObjects(e *pcaputil.ProtocolEvent) []*objectObservation {
	if e == nil || e.Protocol != "grpc" || e.Session == nil || e.Status == "malformed" {
		return nil
	}
	messages, ok := e.Session["GRPC Messages"].([]map[string]any)
	if !ok || len(messages) == 0 {
		return nil
	}
	streamID, _ := objectUint64(e.Session["Stream ID"])
	uri, authority, method := e.ApplicationObjectContext()
	contentType, _ := e.Session["GRPC Content Type"].(string)
	if contentType == "" {
		contentType = "application/grpc"
	}
	refs := e.ApplicationMessagePacketRefs()
	if len(refs) == 0 {
		snapshot := e.DisplayFieldValues()
		refs = append([]pcaputil.PacketReference(nil), snapshot.Source.PacketRefs...)
	}
	parents := append([]uint64(nil), e.SourceBytes.ParentPDUs...)
	if e.ID != 0 && !containsUint64(parents, e.ID) {
		parents = append(parents, e.ID)
	}
	direction := "client_to_server"
	if e.Direction != 0 {
		direction = "server_to_client"
	}
	var out []*objectObservation
	for position, message := range messages {
		payload, _ := message["Decoded Payload"].([]byte)
		compressed := message["Compressed"] == true
		if payload == nil {
			payload, _ = message["Payload"].([]byte)
		}
		payload = bytes.Clone(payload)
		messageID, _ := objectUint64(message["Index"])
		if messageID == 0 {
			messageID = uint64(position + 1)
		}
		declared := int64(len(payload))
		byteKind := "decoded"
		if compressed {
			byteKind = "decompressed"
		} else if e.SourceBytes.Kind == "decrypted" {
			byteKind = "decrypted"
		}
		name := fmt.Sprintf("grpc-s%d-m%d", streamID, messageID)
		out = append(out, &objectObservation{
			id: e.ID, protocol: e.Protocol, objectType: "message", direction: direction,
			source: e.Source, destination: e.Destination, streamID: streamID, messageID: messageID,
			authority: authority, method: method, uri: uri,
			contentType: contentType, suggestedFilename: safeObjectName(name, contentType),
			allowEmpty: true, compressed: compressed, body: bytes.NewReader(payload), declared: &declared,
			refs: refs, bytes: assembledObjectByteSource(payload, byteKind, parents, refs),
		})
	}
	return out
}

func observedWebSocketObjects(e *pcaputil.ProtocolEvent) []*objectObservation {
	if e == nil || e.Protocol != "websocket" || e.Session == nil || e.Status == "malformed" {
		return nil
	}
	opcode, _ := e.Session["Opcode Name"].(string)
	if opcode != "text" && opcode != "binary" {
		return nil
	}
	payload, ok := e.Session["Application"].([]byte)
	if !ok {
		return nil
	}
	payload = bytes.Clone(payload)
	contentType := "application/octet-stream"
	if opcode == "text" {
		contentType = "text/plain; charset=utf-8"
	}
	compressed, _ := e.Session["Compressed"].(bool)
	refs := e.ApplicationMessagePacketRefs()
	if len(refs) == 0 {
		snapshot := e.DisplayFieldValues()
		refs = append([]pcaputil.PacketReference(nil), snapshot.Source.PacketRefs...)
	}
	direction := "client_to_server"
	if e.Direction != 0 {
		direction = "server_to_client"
	}
	declared := int64(len(payload))
	byteKind := "decoded"
	if compressed {
		byteKind = "decompressed"
	}
	return []*objectObservation{{
		id: e.ID, protocol: e.Protocol, objectType: "message", direction: direction,
		source: e.Source, destination: e.Destination, opcode: opcode,
		contentType: contentType, suggestedFilename: safeObjectName("websocket-"+opcode, contentType),
		allowEmpty: true, compressed: compressed, body: bytes.NewReader(payload), declared: &declared,
		refs: refs, bytes: assembledObjectByteSource(payload, byteKind, []uint64{e.ID}, refs),
	}}
}

func observedFTPObject(e *pcaputil.ProtocolEvent) *objectObservation {
	if e == nil || e.Protocol != "ftp" || e.Session == nil || e.Status == "malformed" {
		return nil
	}
	payload, ok := e.Session["File Bytes"].([]byte)
	if !ok {
		return nil
	}
	payload = bytes.Clone(payload)
	filename, _ := e.Session["Filename"].(string)
	if filename == "" {
		filename = "ftp-object"
	}
	contentType, _ := e.Session["FTP File Content Type"].(string)
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(filename))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	direction, _ := e.Session["File Transfer"].(string)
	if direction != "upload" && direction != "download" {
		return nil
	}
	source, _ := e.Session["FTP File Source"].(string)
	destination, _ := e.Session["FTP File Destination"].(string)
	if source == "" || destination == "" {
		return nil
	}
	transferID, _ := objectUint64(e.Session["File Request ID"])
	byteSource, _ := e.Session["File Byte Source"].(pcaputil.ByteSource)
	refs := append([]pcaputil.PacketReference(nil), byteSource.PacketRefs...)
	declared := int64(len(payload))
	return &objectObservation{
		id: e.ID, protocol: e.Protocol, objectType: "file", direction: direction,
		source: source, destination: destination,
		contentType: contentType, suggestedFilename: safeObjectName(filename, contentType),
		transferID: transferID, allowEmpty: true, body: bytes.NewReader(payload), declared: &declared,
		refs: refs, bytes: byteSource,
	}
}

func observedTFTPObject(e *pcaputil.ProtocolEvent) *objectObservation {
	if e == nil || e.Protocol != "tftp" || e.Session == nil || e.Status == "malformed" {
		return nil
	}
	payload, ok := e.Session["File Bytes"].([]byte)
	if !ok {
		return nil
	}
	payload = bytes.Clone(payload)
	filename, _ := e.Session["Filename"].(string)
	if filename == "" {
		filename = "tftp-object"
	}
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	direction := e.ApplicationObjectDirection()
	if direction != "upload" && direction != "download" {
		direction = "download"
	}
	transferID, _ := objectUint64(e.Session["File Request ID"])
	refs := tftpObjectPacketRefs(e)
	parents := tftpObjectParentPDUs(e)
	declared := int64(len(payload))
	return &objectObservation{
		id: e.ID, protocol: e.Protocol, objectType: "file", direction: direction,
		// The completion event is the final ACK, so file bytes travel opposite it.
		source: e.Destination, destination: e.Source,
		contentType: contentType, suggestedFilename: safeObjectName(filename, contentType),
		transferID: transferID, allowEmpty: true, body: bytes.NewReader(payload), declared: &declared,
		refs: refs, bytes: assembledObjectByteSource(payload, "reassembled", parents, refs),
	}
}

func tftpObjectPacketRefs(e *pcaputil.ProtocolEvent) []pcaputil.PacketReference {
	var refs []pcaputil.PacketReference
	sources, _ := e.Session["File Sources"].([]map[string]any)
	for _, source := range sources {
		numbers, _ := source["Packet Numbers"].([]uint64)
		for _, number := range numbers {
			ref := pcaputil.PacketReference{Number: number, Domain: e.Domain}
			if !containsPacketReference(refs, ref) {
				refs = append(refs, ref)
			}
		}
	}
	if len(refs) == 0 {
		refs = append(refs, e.SourceBytes.PacketRefs...)
	}
	return refs
}

func tftpObjectParentPDUs(e *pcaputil.ProtocolEvent) []uint64 {
	var parents []uint64
	add := func(id uint64) {
		if id != 0 && !containsUint64(parents, id) {
			parents = append(parents, id)
		}
	}
	if id, ok := objectUint64(e.Session["File Request ID"]); ok {
		add(id)
	}
	if id, ok := objectUint64(e.Session["File Negotiation ID"]); ok {
		add(id)
	}
	sources, _ := e.Session["File Sources"].([]map[string]any)
	for _, source := range sources {
		if id, ok := objectUint64(source["Event ID"]); ok {
			add(id)
		}
	}
	return parents
}
