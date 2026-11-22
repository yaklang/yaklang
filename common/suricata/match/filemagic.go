package match

import (
	"bytes"
	"encoding/binary"
	"io"
	"mime"
	"mime/multipart"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

// DetectFileMagic returns a libmagic-compatible file type description string
// for the leading bytes of a file buffer without requiring cgo/libmagic.
func DetectFileMagic(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	// 7-Zip archive: 37 7A BC AF 27 1C
	if len(data) >= 6 && bytes.HasPrefix(data, []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}) {
		return "7-zip archive data"
	}
	// ZIP archive: PK\x03\x04, PK\x05\x06 (empty), PK\x07\x08 (spanned)
	if len(data) >= 4 && data[0] == 'P' && data[1] == 'K' &&
		((data[2] == 0x03 && data[3] == 0x04) ||
			(data[2] == 0x05 && data[3] == 0x06) ||
			(data[2] == 0x07 && data[3] == 0x08)) {
		return "Zip archive data"
	}
	// POSIX / GNU tar archive (ustar magic at offset 257 or valid 512-byte tar header checksum)
	if isPOSIXTar(data) {
		return "POSIX tar archive"
	}
	// GZIP
	if len(data) >= 3 && data[0] == 0x1f && data[1] == 0x8b && data[2] == 0x08 {
		return "gzip compressed data"
	}
	// BZIP2
	if len(data) >= 3 && data[0] == 'B' && data[1] == 'Z' && data[2] == 'h' {
		return "bzip2 compressed data"
	}
	// XZ
	if len(data) >= 6 && bytes.HasPrefix(data, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}) {
		return "XZ compressed data"
	}
	// Zstandard
	if len(data) >= 4 && bytes.HasPrefix(data, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		return "Zstandard compressed data"
	}
	// RAR
	if len(data) >= 6 && bytes.HasPrefix(data, []byte{'R', 'a', 'r', '!', 0x1a, 0x07}) {
		return "RAR archive data"
	}
	// Microsoft Cabinet
	if len(data) >= 4 && bytes.HasPrefix(data, []byte("MSCF")) {
		return "Microsoft Cabinet archive data"
	}
	// PDF
	if len(data) >= 5 && bytes.HasPrefix(data, []byte("%PDF-")) {
		return "PDF document"
	}
	// ELF
	if len(data) >= 4 && bytes.HasPrefix(data, []byte{0x7f, 'E', 'L', 'F'}) {
		return "ELF"
	}
	// PE / MS-DOS executable
	if len(data) >= 2 && data[0] == 'M' && data[1] == 'Z' {
		if len(data) >= 64 {
			peOff := int(binary.LittleEndian.Uint32(data[60:64]))
			if peOff > 0 && peOff+4 <= len(data) && bytes.Equal(data[peOff:peOff+4], []byte{'P', 'E', 0x00, 0x00}) {
				return "PE32 executable"
			}
		}
		return "MS-DOS executable"
	}
	// Mach-O
	if len(data) >= 4 {
		switch binary.BigEndian.Uint32(data[:4]) {
		case 0xfeedface, 0xcefaedfe, 0xfeedfacf, 0xcffaedfe:
			return "Mach-O"
		case 0xcafebabe:
			return "compiled Java class data"
		}
	}
	// OLE2 / MS Office compound document
	if len(data) >= 8 && bytes.HasPrefix(data, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}) {
		return "Composite Document File V2 Document"
	}
	// SQLite 3
	if len(data) >= 16 && bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
		return "SQLite 3.x database"
	}
	// Images
	if len(data) >= 8 && bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		return "PNG image data"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "JPEG image data"
	}
	if len(data) >= 6 && (bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))) {
		return "GIF image data"
	}
	if len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return "RIFF (little-endian) data, Web/P image"
	}
	// Text / markup
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if bytes.HasPrefix(trimmed, []byte("<?xml")) {
		return "XML document"
	}
	if len(trimmed) >= 5 {
		low := strings.ToLower(string(trimmed[:min(len(trimmed), 16)]))
		if strings.HasPrefix(low, "<!doctype html") || strings.HasPrefix(low, "<html") {
			return "HTML document"
		}
	}
	return "data"
}

func isPOSIXTar(data []byte) bool {
	if len(data) >= 263 {
		magic := data[257:263]
		if bytes.Equal(magic, []byte("ustar\x00")) || bytes.Equal(magic, []byte("ustar ")) {
			return true
		}
	}
	if len(data) < 512 {
		return false
	}
	// Verify classic V7/POSIX tar header checksum (octal ASCII in [148:156]).
	chkField := strings.TrimSpace(strings.TrimRight(string(data[148:156]), "\x00 "))
	if chkField == "" {
		return false
	}
	expected, err := strconv.ParseInt(chkField, 8, 64)
	if err != nil || expected <= 0 {
		return false
	}
	var unsignedSum, signedSum int64
	for i := 0; i < 512; i++ {
		b := data[i]
		if i >= 148 && i < 156 {
			b = ' '
		}
		unsignedSum += int64(b)
		signedSum += int64(int8(b))
	}
	return unsignedSum == expected || signedSum == expected
}

// extractFileDataForTCPWithOffset returns the file payload carried in a TCP
// stream together with the byte offset in raw where that file payload begins.
// When the stream carries an HTTP request/response, it extracts the HTTP body
// (unwrapping multipart/form-data file parts when present); otherwise it
// returns the raw TCP payload at offset 0.
func extractFileDataForTCPWithOffset(raw []byte) ([]byte, int) {
	if len(raw) == 0 || !LooksLikeHTTP(raw) {
		return raw, 0
	}
	sepIdx := bytes.Index(raw, []byte("\r\n\r\n"))
	sepLen := 4
	if sepIdx < 0 {
		sepIdx = bytes.Index(raw, []byte("\n\n"))
		sepLen = 2
	}
	if sepIdx < 0 || sepIdx+sepLen >= len(raw) {
		return raw, 0
	}
	headers := raw[:sepIdx]
	bodyOffset := sepIdx + sepLen
	body := raw[bodyOffset:]

	// Check for multipart/form-data upload and extract the first file part.
	if ct := extractHeaderValue(headers, "Content-Type"); ct != "" {
		if mediaType, params, err := mime.ParseMediaType(ct); err == nil && strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
			if boundary := params["boundary"]; boundary != "" {
				mr := multipart.NewReader(bytes.NewReader(body), boundary)
				for i := 0; i < 16; i++ {
					part, err := mr.NextPart()
					if err != nil {
						break
					}
					partBytes, readErr := io.ReadAll(io.LimitReader(part, maxHTTPDecompressBytes))
					_ = part.Close()
					if readErr == nil && len(partBytes) > 0 {
						if part.FileName() != "" || DetectFileMagic(partBytes) != "data" {
							return partBytes, bodyOffset
						}
					}
				}
			}
		}
	}

	if ce := extractHeaderValue(headers, "Content-Encoding"); ce != "" {
		if decoded, ok := decompressHTTPBody(body, ce); ok && len(decoded) > 0 {
			return decoded, bodyOffset
		}
	}
	return body, bodyOffset
}

func extractFileDataForTCP(raw []byte) []byte {
	data, _ := extractFileDataForTCPWithOffset(raw)
	return data
}

func extractHeaderValue(headers []byte, name string) string {
	lines := bytes.Split(headers, []byte("\n"))
	for i := 1; i < len(lines); i++ {
		k, v, ok := bytes.Cut(lines[i], []byte(":"))
		if !ok {
			continue
		}
		if bytes.EqualFold(bytes.TrimSpace(k), []byte(name)) {
			return strings.TrimSpace(strings.TrimSuffix(string(v), "\r"))
		}
	}
	return ""
}

func fileSizeMatcher(c *matchContext) error {
	if c.Rule == nil || c.Rule.ContentRuleConfig == nil || c.Rule.ContentRuleConfig.FileSize == nil {
		return nil
	}
	data := c.GetBuffer(modifier.FileData)
	if len(data) == 0 {
		data = c.GetBuffer(modifier.Default)
	}
	c.Must(len(data) > 0 && c.Rule.ContentRuleConfig.FileSize.Match(len(data)))
	return nil
}
