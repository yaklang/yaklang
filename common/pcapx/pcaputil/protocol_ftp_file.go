package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"path/filepath"
)

const MaxFTPFileBytes = 64 << 20

// WithProtocolFTPFileAssembly selects bounded observed RETR/STOR file
// assembly. A file is exposed only after a matching 226/250 completion reply.
func WithProtocolFTPFileAssembly(maximum int) CaptureOption {
	return func(c *CaptureConfig) error {
		if maximum < 1 || maximum > MaxFTPFileBytes {
			return fmt.Errorf("FTP file assembly requires 1..67108864 bytes")
		}
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.FTPFileBytesLimit = maximum
		return nil
	}
}

// WithSessionFTPFileAssembly selects the same finite FTP file profile.
func WithSessionFTPFileAssembly(maximum int) ProtocolSessionOption {
	return func(s *captureSession) error {
		if maximum < 1 || maximum > MaxFTPFileBytes {
			return fmt.Errorf("FTP file assembly requires 1..67108864 bytes")
		}
		s.f.a.config.FTPFileBytesLimit = maximum
		return nil
	}
}

type ftpFileTransfer struct {
	id, control                   uint64
	command, filename, direction  string
	body                          []byte
	nextOffset                    uint64
	packetRefs                    []PacketReference
	parents                       []uint64
	sources                       []map[string]any
	source, destination, byteKind string
	protected                     bool
}

func (f *binFlow) startFTPFileTransferLocked(e *ProtocolEvent, command, filename string) {
	a := f.a
	if a.config.FTPFileBytesLimit == 0 || command != "RETR" && command != "STOR" {
		return
	}
	if a.ftpFiles == nil {
		a.ftpFiles = make(map[uint64]*ftpFileTransfer)
	}
	if len(a.ftpFiles) >= min(256, sessionCollectionLimit(a.budget.MaxCollectionElements)) {
		return
	}
	a.abortFTPFileLocked(e.ID)
	direction := "download"
	if command == "STOR" {
		direction = "upload"
	}
	a.ftpFiles[e.ID] = &ftpFileTransfer{
		id: e.ID, control: f.id, command: command, filename: filename,
		direction: direction, protected: f.ftp.dataProtected,
	}
}

func (a *binParser) appendFTPFileChunk(e *ProtocolEvent, binding *ftpDataBinding) {
	if binding == nil || a.config.FTPFileBytesLimit == 0 || len(e.Raw) == 0 {
		return
	}
	a.ftpMu.Lock()
	defer a.ftpMu.Unlock()
	state := a.ftpFiles[binding.transfer]
	if state == nil {
		return
	}
	if len(state.sources) >= a.budget.MaxCollectionElements ||
		len(state.packetRefs) >= a.budget.MaxCollectionElements ||
		len(state.parents) >= a.budget.MaxCollectionElements {
		a.abortFTPFileLocked(binding.transfer)
		return
	}
	offset, ok := ftpFileUint64(e.Session["Stream Offset"])
	if !ok || offset != state.nextOffset {
		a.abortFTPFileLocked(binding.transfer)
		return
	}
	if len(e.Raw) > a.config.FTPFileBytesLimit-len(state.body) || a.ftpFileBytes+int64(len(e.Raw)) > int64(a.config.MaxBufferedBytes/2) {
		a.abortFTPFileLocked(binding.transfer)
		return
	}
	fileOffset := len(state.body)
	state.body = append(state.body, e.Raw...)
	state.nextOffset += uint64(len(e.Raw))
	if state.source == "" {
		state.source, state.destination = e.Source, e.Destination
	}
	if state.byteKind == "" || e.SourceBytes.Kind == "decrypted" {
		state.byteKind = e.SourceBytes.Kind
	}
	for _, ref := range e.SourceBytes.PacketRefs {
		if !packetReferenceContains(state.packetRefs, ref) {
			state.packetRefs = append(state.packetRefs, ref)
		}
	}
	numbers := make([]uint64, 0, len(e.SourceBytes.PacketRefs))
	for _, ref := range e.SourceBytes.PacketRefs {
		numbers = append(numbers, ref.Number)
	}
	state.sources = append(state.sources, map[string]any{
		"Event ID":       e.ID,
		"File Offset":    fileOffset,
		"Length":         len(e.Raw),
		"Packet Numbers": numbers,
	})
	if e.ID != 0 && !containsProtocolObjectUint64(state.parents, e.ID) {
		state.parents = append(state.parents, e.ID)
	}
	a.ftpFileBytes += int64(len(e.Raw))
}

func (a *binParser) finishFTPFileLocked(transfer uint64, e *ProtocolEvent, code int) {
	if transfer == 0 || e == nil {
		return
	}
	state := a.ftpFiles[transfer]
	if state == nil {
		return
	}
	if code != 226 && code != 250 || len(state.body) == 0 || state.nextOffset != uint64(len(state.body)) {
		a.abortFTPFileLocked(transfer)
		return
	}
	filename := state.filename
	if filename == "" {
		filename = "ftp-object"
	}
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	parent := append([]uint64(nil), state.parents...)
	if state.id != 0 && !containsProtocolObjectUint64(parent, state.id) {
		parent = append(parent, state.id)
	}
	sum := sha256.Sum256(state.body)
	source := ByteSource{
		Kind:       state.byteKind,
		SHA256:     hex.EncodeToString(sum[:]),
		ParentPDUs: parent,
		PacketRefs: append([]PacketReference(nil), state.packetRefs...),
	}
	if source.Kind == "" {
		source.Kind = "reassembled"
	}
	if len(source.PacketRefs) > 0 {
		completed := source.PacketRefs[len(source.PacketRefs)-1]
		source.CompletedIn = &completed
	}
	body := bytes.Clone(state.body)
	e.Session["File Bytes"] = body
	e.Session["Filename"] = filename
	e.Session["File Byte Count"] = len(body)
	e.Session["File SHA256"] = source.SHA256
	e.Session["File Transfer"] = state.direction
	e.Session["File Request ID"] = state.id
	e.Session["File Byte Source"] = source
	e.Session["FTP Command"] = state.command
	e.Session["FTP Completion Code"] = code
	e.Session["FTP File Content Type"] = contentType
	e.Session["FTP File Source"] = state.source
	e.Session["FTP File Destination"] = state.destination
	e.Session["FTP File Protected"] = state.protected
	e.Session["FTP File Sources"] = cloneSessionValue(state.sources)
	a.abortFTPFileLocked(transfer)
}

func (a *binParser) abortFTPFileLocked(transfer uint64) {
	state := a.ftpFiles[transfer]
	if state == nil {
		return
	}
	a.ftpFileBytes -= int64(len(state.body))
	delete(a.ftpFiles, transfer)
}

func (a *binParser) abortFTPFilesForControl(control uint64) {
	for transfer, state := range a.ftpFiles {
		if state.control == control {
			a.abortFTPFileLocked(transfer)
		}
	}
}

func containsProtocolObjectUint64(values []uint64, want uint64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func ftpFileUint64(value any) (uint64, bool) {
	switch value := value.(type) {
	case uint64:
		return value, true
	case uint32:
		return uint64(value), true
	case uint:
		return uint64(value), true
	case int:
		return uint64(value), value >= 0
	case int64:
		return uint64(value), value >= 0
	default:
		return 0, false
	}
}
