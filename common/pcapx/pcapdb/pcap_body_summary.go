package pcapdb

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

const maxHTTPBodySummaryBytes = 16 << 20

var (
	httpPercentEscape  = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)
	httpEmailPattern   = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	httpJWTPattern     = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)
	httpAWSAccessKey   = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	httpSensitiveField = regexp.MustCompile(`(?i)["']?(password|passwd|pwd|secret|token|access_token|refresh_token|api[_-]?key|authorization|cookie|session|card[_-]?number|pan|cvv|pin|ssn|user(name)?|account|email)["']?\s*[:=]`)
)

// ProtocolBodySummary is a non-reversible summary of one complete HTTP/1
// message. It deliberately contains no body bytes or body prefixes.
type ProtocolBodySummary struct {
	ID                  uint     `json:"id"`
	Protocol            string   `json:"protocol"`
	Method              string   `json:"method,omitempty"`
	URI                 string   `json:"uri,omitempty"`
	Host                string   `json:"host,omitempty"`
	StatusCode          int      `json:"status_code,omitempty"`
	ContentType         string   `json:"content_type,omitempty"`
	ContentEncoding     string   `json:"content_encoding,omitempty"`
	TransferEncoding    []string `json:"transfer_encoding,omitempty"`
	MessageBytes        int      `json:"message_bytes"`
	DeclaredBodyBytes   *int64   `json:"declared_body_bytes,omitempty"`
	WireBodyBytes       int      `json:"wire_body_bytes"`
	DecodedBodyBytes    int      `json:"decoded_body_bytes"`
	TransferComplete    bool     `json:"transfer_complete"`
	DecodedComplete     bool     `json:"decoded_complete"`
	MessageSHA256       string   `json:"message_sha256"`
	WireBodySHA256      string   `json:"wire_body_sha256,omitempty"`
	DecodedBodySHA256   string   `json:"decoded_body_sha256,omitempty"`
	BodyKind            string   `json:"body_kind"`
	Magic               string   `json:"magic"`
	Compressed          bool     `json:"compressed"`
	EncodedForm         bool     `json:"encoded_form"`
	PercentEncoded      bool     `json:"percent_encoded"`
	Base64Like          bool     `json:"base64_like"`
	Binary              bool     `json:"binary"`
	TextLike            bool     `json:"text_like"`
	NULBytes            int      `json:"nul_bytes"`
	NonPrintableRatio   float64  `json:"non_printable_ratio"`
	Sensitive           bool     `json:"sensitive"`
	SensitiveIndicators []string `json:"sensitive_indicators,omitempty"`
	Limitations         []string `json:"limitations,omitempty"`
}

// BodySummaryPage summarizes one HTTP/1 request or response without returning
// its body. Hashes cover transfer-decoded and content-decoded representations
// separately; incomplete bodies never receive a body hash.
func (d *Database) BodySummaryPage(id uint, options ...QueryOption) (*ResultPage[ProtocolBodySummary], error) {
	page := emptyPage[ProtocolBodySummary](d.ID, 0, "record_id")
	c, err := parseDetailQuery(options)
	if err != nil {
		return failPage(page, err)
	}
	if id == 0 {
		return failPage(page, ErrNotFound)
	}
	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err := d.lockRead(ctx); err != nil {
		return failPage(page, err)
	}
	defer d.mu.RUnlock()
	if d.closed {
		return failPage(page, ErrClosed)
	}
	tx, _, err := d.readSnapshot(ctx, true, false)
	if err != nil {
		return failPage(page, err)
	}
	defer tx.Rollback()
	scoped := indexWithContext(ctx, tx)
	var message PCAPProtocolMessage
	err = scoped.Select("id,protocol,status,completeness,response_to,data,data_length").Where("id = ?", id).First(&message).Error
	if gorm.IsRecordNotFoundError(err) {
		return failPage(page, ErrNotFound)
	}
	if err != nil {
		return failPage(page, err)
	}
	if message.Protocol != "http" && message.Protocol != "doh" && message.Protocol != "ipp" {
		return failPage(page, fmt.Errorf("pcapdb: body summary requires an HTTP/1 message"))
	}
	if message.DataLength != len(message.Data) || len(message.Data) == 0 {
		return failPage(page, fmt.Errorf("pcapdb: stored HTTP message is incomplete or invalid"))
	}
	responseMethod := ""
	limitations := make([]string, 0, 1)
	if bytes.HasPrefix(message.Data, []byte("HTTP/")) {
		if message.ResponseTo > 0 {
			var request PCAPProtocolMessage
			if requestErr := scoped.Select("id,protocol,data,data_length").Where("id = ?", message.ResponseTo).First(&request).Error; requestErr == nil && request.DataLength == len(request.Data) {
				responseMethod = httpRequestMethod(request.Data)
			}
		}
		if responseMethod == "" {
			limitations = append(limitations, "request method unavailable; response body framing follows observed headers")
		}
	}
	summary, err := summarizeHTTPBody(message, responseMethod)
	if err != nil {
		return failPage(page, err)
	}
	summary.Limitations = appendUniqueStrings(summary.Limitations, limitations...)
	page.State = StateReady
	return pageItems(page, []ProtocolBodySummary{summary}, 1, c.resultBytes, func(row ProtocolBodySummary) int64 { return int64(row.ID) })
}

func summarizeHTTPBody(message PCAPProtocolMessage, responseMethod string) (ProtocolBodySummary, error) {
	raw := message.Data
	messageSum := sha256.Sum256(raw)
	summary := ProtocolBodySummary{
		ID:            message.ID,
		Protocol:      message.Protocol,
		MessageBytes:  len(raw),
		MessageSHA256: hex.EncodeToString(messageSum[:]),
		Magic:         detectBodyMagic(raw),
	}
	if message.Status == "incomplete" || message.Status == "malformed" || message.Status == "limited" {
		summary.Limitations = append(summary.Limitations, "message status is "+message.Status)
	}
	reader := bufio.NewReader(bytes.NewReader(raw))
	var body io.ReadCloser
	var headers http.Header
	var transferEncoding []string
	var declared int64
	if bytes.HasPrefix(raw, []byte("HTTP/")) {
		response, err := http.ReadResponse(reader, &http.Request{Method: responseMethod})
		if err != nil {
			return summary, fmt.Errorf("pcapdb: parse HTTP response body summary: %w", err)
		}
		summary.StatusCode = response.StatusCode
		body, headers = response.Body, response.Header
		transferEncoding = append([]string(nil), response.TransferEncoding...)
		declared = response.ContentLength
	} else {
		request, err := http.ReadRequest(reader)
		if err != nil {
			return summary, fmt.Errorf("pcapdb: parse HTTP request body summary: %w", err)
		}
		summary.Method = request.Method
		summary.URI = request.RequestURI
		summary.Host = request.Host
		body, headers = request.Body, request.Header
		transferEncoding = append([]string(nil), request.TransferEncoding...)
		declared = request.ContentLength
	}
	defer body.Close()
	if declared >= 0 {
		value := declared
		summary.DeclaredBodyBytes = &value
	}
	summary.TransferEncoding = transferEncoding
	summary.ContentType = headers.Get("Content-Type")
	summary.ContentEncoding = strings.TrimSpace(headers.Get("Content-Encoding"))
	wireBody, readErr := io.ReadAll(io.LimitReader(body, maxHTTPBodySummaryBytes+1))
	if len(wireBody) > maxHTTPBodySummaryBytes {
		readErr = fmt.Errorf("body exceeds %d bytes", maxHTTPBodySummaryBytes)
		wireBody = nil
	}
	if readErr != nil {
		summary.Limitations = append(summary.Limitations, "body read failed: "+readErr.Error())
	}
	if wireBody != nil && declared >= 0 && int64(len(wireBody)) != declared {
		summary.Limitations = append(summary.Limitations, fmt.Sprintf("declared body length %d differs from observed %d", declared, len(wireBody)))
	}
	messageComplete := message.Status != "incomplete" && message.Status != "malformed" && message.Status != "limited"
	summary.TransferComplete = readErr == nil && wireBody != nil && (declared < 0 || int64(len(wireBody)) == declared) && messageComplete
	if summary.TransferComplete {
		wireSum := sha256.Sum256(wireBody)
		summary.WireBodySHA256 = hex.EncodeToString(wireSum[:])
	}
	decodedBody, decodeErr := pcaputil.DecodeBody(wireBody, summary.ContentEncoding, maxHTTPBodySummaryBytes)
	if decodeErr != nil {
		summary.Limitations = append(summary.Limitations, "content decoding failed: "+decodeErr.Error())
		decodedBody = nil
	}
	summary.DecodedComplete = summary.TransferComplete && decodeErr == nil
	if summary.DecodedComplete {
		decodedSum := sha256.Sum256(decodedBody)
		summary.DecodedBodySHA256 = hex.EncodeToString(decodedSum[:])
	}
	summary.WireBodyBytes = len(wireBody)
	analysisBody := decodedBody
	if analysisBody == nil {
		analysisBody = wireBody
	} else {
		summary.DecodedBodyBytes = len(decodedBody)
	}
	analyzeBodyStatistics(&summary, analysisBody)
	summary.SensitiveIndicators = appendUniqueStrings(summary.SensitiveIndicators, scanHTTPSensitive(headers, analysisBody, summary.ContentType)...)
	sort.Strings(summary.SensitiveIndicators)
	summary.Sensitive = len(summary.SensitiveIndicators) > 0
	return summary, nil
}

func analyzeBodyStatistics(summary *ProtocolBodySummary, body []byte) {
	summary.Magic = detectBodyMagic(body)
	summary.Compressed = summary.ContentEncoding != "" && !strings.EqualFold(summary.ContentEncoding, "identity") || summary.Magic == "gzip" || summary.Magic == "zlib" || summary.Magic == "zip"
	mediaType, _, _ := mime.ParseMediaType(summary.ContentType)
	mediaType = strings.ToLower(mediaType)
	summary.EncodedForm = mediaType == "application/x-www-form-urlencoded"
	summary.PercentEncoded = len(httpPercentEscape.FindAll(body, -1)) > 0
	summary.Base64Like = looksBase64(body)
	summary.NULBytes = bytes.Count(body, []byte{0})
	nonPrintable := 0
	for _, b := range body {
		if b == 0 || b == '\r' || b == '\n' || b == '\t' {
			continue
		}
		if b < 0x20 || b == 0x7f {
			nonPrintable++
		}
	}
	if len(body) > 0 {
		summary.NonPrintableRatio = math.Round(float64(nonPrintable)/float64(len(body))*10000) / 10000
	}
	textualType := strings.HasPrefix(mediaType, "text/") || strings.Contains(mediaType, "json") || strings.Contains(mediaType, "xml") || strings.Contains(mediaType, "javascript") || strings.Contains(mediaType, "x-www-form-urlencoded")
	summary.Binary = len(body) > 0 && (summary.NULBytes > 0 || !utf8.Valid(body) || summary.NonPrintableRatio > 0.10 && !textualType)
	summary.TextLike = len(body) == 0 || !summary.Binary
	switch {
	case len(body) == 0:
		summary.BodyKind = "empty"
	case summary.Compressed:
		summary.BodyKind = "compressed"
	case summary.Binary:
		summary.BodyKind = "binary"
	case mediaType == "application/x-www-form-urlencoded":
		summary.BodyKind = "form"
	case strings.Contains(mediaType, "json"):
		summary.BodyKind = "json"
	case strings.Contains(mediaType, "xml"):
		summary.BodyKind = "xml"
	default:
		summary.BodyKind = "text"
	}
	if summary.EncodedForm {
		values, err := url.ParseQuery(string(body))
		if err == nil {
			for _, items := range values {
				for _, value := range items {
					if len(value) >= 32 && looksBase64([]byte(value)) {
						summary.SensitiveIndicators = appendUniqueStrings(summary.SensitiveIndicators, "encoded-payload")
					}
				}
			}
		}
	}
}

func scanHTTPSensitive(headers http.Header, body []byte, contentType string) []string {
	indicators := make([]string, 0, 4)
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie"} {
		if headers.Get(name) != "" {
			indicators = appendUniqueStrings(indicators, "credential-header")
		}
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if strings.EqualFold(mediaType, "application/x-www-form-urlencoded") {
		if values, err := url.ParseQuery(string(body)); err == nil {
			for key := range values {
				if kind := sensitiveHTTPKeyKind(key); kind != "" {
					indicators = appendUniqueStrings(indicators, kind)
				}
			}
		}
	}
	if httpEmailPattern.Match(body) {
		indicators = appendUniqueStrings(indicators, "email-address")
	}
	if httpJWTPattern.Match(body) {
		indicators = appendUniqueStrings(indicators, "jwt")
	}
	if httpAWSAccessKey.Match(body) {
		indicators = appendUniqueStrings(indicators, "aws-access-key")
	}
	if httpSensitiveField.Match(body) {
		indicators = appendUniqueStrings(indicators, "sensitive-body-field")
	}
	if bytes.Contains(body, []byte("-----BEGIN ")) && bytes.Contains(body, []byte("PRIVATE KEY-----")) {
		indicators = appendUniqueStrings(indicators, "private-key")
	}
	return indicators
}

func sensitiveHTTPKeyKind(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "password", "passwd", "pwd", "secret", "token", "access_token", "refresh_token", "api_key", "apikey", "authorization", "cookie", "session", "card", "card_number", "pan", "cvv", "pin", "ssn":
		return "credential-or-payment-field"
	case "u", "user", "username", "account", "email":
		return "user-identifier"
	default:
		return ""
	}
}

func httpRequestMethod(raw []byte) string {
	request, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return ""
	}
	defer request.Body.Close()
	return request.Method
}

func detectBodyMagic(body []byte) string {
	if len(body) == 0 {
		return "empty"
	}
	switch {
	case len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b:
		return "gzip"
	case len(body) >= 2 && body[0] == 0x78 && (body[1] == 0x01 || body[1] == 0x9c || body[1] == 0xda):
		return "zlib"
	case bytes.HasPrefix(body, []byte("PK\x03\x04")):
		return "zip"
	case bytes.HasPrefix(body, []byte("%PDF")):
		return "pdf"
	case bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(body, []byte("\xff\xd8\xff")):
		return "jpeg"
	case bytes.HasPrefix(body, []byte("\x7fELF")):
		return "elf"
	case bytes.HasPrefix(body, []byte("MZ")):
		return "pe"
	case utf8.Valid(body):
		return "text"
	default:
		return "unknown-binary"
	}
}

func looksBase64(body []byte) bool {
	compact := make([]byte, 0, len(body))
	for _, b := range body {
		switch b {
		case ' ', '\r', '\n', '\t':
			continue
		default:
			compact = append(compact, b)
		}
	}
	if len(compact) < 32 {
		return false
	}
	valid, alphaNum := 0, 0
	for _, b := range compact {
		if b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' {
			valid++
			alphaNum++
			continue
		}
		if b == '+' || b == '/' || b == '=' || b == '-' || b == '_' {
			valid++
		}
	}
	return valid*100 >= len(compact)*95 && alphaNum > 0
}

func appendUniqueStrings(dst []string, values ...string) []string {
	seen := make(map[string]bool, len(dst)+len(values))
	for _, value := range dst {
		seen[value] = true
	}
	for _, value := range values {
		if value != "" && !seen[value] {
			dst = append(dst, value)
			seen[value] = true
		}
	}
	return dst
}
