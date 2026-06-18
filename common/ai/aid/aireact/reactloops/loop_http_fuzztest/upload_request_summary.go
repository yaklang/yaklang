package loop_http_fuzztest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	yakmultipart "github.com/yaklang/yaklang/common/utils/multipart"
)

var (
	uploadBoundaryRegexp  = regexp.MustCompile(`(?i)boundary\s*=\s*([^;\s"]+|"[^"]+")`)
	uploadContentProfiles = map[string][]byte{
		"empty":             {},
		"text":              []byte("yak fuzz upload probe"),
		"svg_xss":           []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"php_probe":         []byte("<?php echo 'fuzz_probe'; ?>"),
		"polyglot_jpeg_php": buildPolyglotJPEGPHPProbe(),
	}
)

func buildPolyglotJPEGPHPProbe() []byte {
	// Small JPEG magic bytes + harmless PHP marker for MIME/extension mismatch tests.
	return []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9,
		'\n', '<', '?', 'p', 'h', 'p', ' ', 'e', 'c', 'h', 'o', ' ', '\'', 'f', 'u', 'z', 'z', '_', 'p', 'r', 'o', 'b', 'e', '\'', ';', ' ', '?', '>', '\n',
	}
}

func resolveUploadContentProfile(profile string) ([]byte, error) {
	profile = strings.TrimSpace(strings.ToLower(profile))
	if profile == "" {
		return nil, fmt.Errorf("content_profile cannot be empty")
	}
	content, ok := uploadContentProfiles[profile]
	if !ok {
		return nil, fmt.Errorf("unknown content_profile %q, supported: empty, text, svg_xss, php_probe, polyglot_jpeg_php", profile)
	}
	return append([]byte(nil), content...), nil
}

func resolveUploadContentBytes(contentTemplate, contentProfile, fileResourceID string, loop *reactloops.ReActLoop) ([]byte, error) {
	if fileResourceID = strings.TrimSpace(fileResourceID); fileResourceID != "" {
		return loadLoopHTTPUploadFileResourceContent(loop, fileResourceID)
	}
	if contentProfile = strings.TrimSpace(contentProfile); contentProfile != "" {
		return resolveUploadContentProfile(contentProfile)
	}
	if contentTemplate != "" {
		return []byte(contentTemplate), nil
	}
	return nil, fmt.Errorf("file_content requires content_template, content_profile, or file_resource_id")
}

func loadLoopHTTPUploadFileResourceContent(loop *reactloops.ReActLoop, resourceID string) ([]byte, error) {
	if loop == nil {
		return nil, fmt.Errorf("loop is nil")
	}
	refs := getLoopHTTPUploadFileResources(loop)
	for _, ref := range refs {
		if ref.ID != resourceID {
			continue
		}
		if ref.Path == "" {
			return nil, fmt.Errorf("upload file resource %q has empty path", resourceID)
		}
		raw, err := os.ReadFile(ref.Path)
		if err != nil {
			return nil, fmt.Errorf("read upload file resource %q: %w", resourceID, err)
		}
		return raw, nil
	}
	return nil, fmt.Errorf("upload file resource %q not found", resourceID)
}

func getLoopHTTPUploadFileResources(loop *reactloops.ReActLoop) []loopHTTPUploadFileResource {
	if loop == nil {
		return nil
	}
	raw := strings.TrimSpace(loop.Get(loopHTTPUploadFileResourceRefsKey))
	if raw == "" {
		return nil
	}
	var refs []loopHTTPUploadFileResource
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		log.Warnf("http_fuzztest: unmarshal upload file resources failed: %v", err)
		return nil
	}
	return refs
}

func setLoopHTTPUploadFileResources(loop *reactloops.ReActLoop, refs []loopHTTPUploadFileResource) {
	if loop == nil {
		return
	}
	if len(refs) == 0 {
		loop.Set(loopHTTPUploadFileResourceRefsKey, "")
		return
	}
	payload, err := json.Marshal(refs)
	if err != nil {
		log.Warnf("http_fuzztest: marshal upload file resources failed: %v", err)
		return
	}
	loop.Set(loopHTTPUploadFileResourceRefsKey, string(payload))
}

func syncLoopHTTPUploadContext(loop *reactloops.ReActLoop, rawPacket []byte, isHTTPS bool, resetBaseline bool) {
	if loop == nil || len(bytes.TrimSpace(rawPacket)) == 0 {
		return
	}

	summary, resources, promptSafe := analyzeAndPrepareUploadRequest(loop, rawPacket)
	summaryText := formatUploadRequestSummaryForPrompt(summary)

	loop.Set(loopHTTPUploadRequestSummaryKey, summaryText)
	setLoopHTTPUploadFileResources(loop, resources)
	loop.Set(loopHTTPUploadCurrentPromptSafeKey, promptSafe)
	if resetBaseline {
		loop.Set(loopHTTPUploadOriginalPromptSafeKey, promptSafe)
	}

	if summary != nil && summary.IsMultipart {
		_, uploadSummary := buildUploadAwareHTTPRequestStreamSummary(string(rawPacket), isHTTPS, summary)
		if strings.TrimSpace(uploadSummary) != "" {
			loop.Set("current_request_summary", uploadSummary)
			if resetBaseline {
				loop.Set("original_request_summary", uploadSummary)
			}
		}
	}
}

func analyzeAndPrepareUploadRequest(loop *reactloops.ReActLoop, rawPacket []byte) (*loopHTTPUploadRequestSummary, []loopHTTPUploadFileResource, string) {
	fixed := lowhttp.FixHTTPRequest(rawPacket)
	if !lowhttp.IsMultipartFormDataRequest(fixed) {
		return &loopHTTPUploadRequestSummary{IsMultipart: false}, nil, sanitizeHTTPRequestForPrompt(fixed, nil)
	}

	summary, err := parseMultipartUploadSummary(fixed)
	if err != nil || summary == nil {
		log.Warnf("http_fuzztest: parse multipart upload summary failed: %v", err)
		return &loopHTTPUploadRequestSummary{IsMultipart: true}, nil, sanitizeHTTPRequestForPrompt(fixed, nil)
	}

	_, body := lowhttp.SplitHTTPPacketFast(fixed)
	externalizeAllFiles := len(body) > loopHTTPUploadBodyExternalizeThreshold

	resources := make([]loopHTTPUploadFileResource, 0)
	if loop != nil {
		resources = append(resources, getLoopHTTPUploadFileResources(loop)...)
	}
	resourceIndex := make(map[string]loopHTTPUploadFileResource, len(resources))
	for _, ref := range resources {
		resourceIndex[ref.FieldName] = ref
	}

	for i := range summary.Parts {
		part := &summary.Parts[i]
		if !part.IsFile {
			continue
		}
		shouldExternalize := externalizeAllFiles ||
			part.Size > int64(loopHTTPUploadPartExternalizeThreshold) ||
			!utf8.ValidString(part.Preview)
		if !shouldExternalize {
			continue
		}
		if loop == nil {
			part.Preview = buildUploadFileRefPlaceholder(part.FieldName, part.FileName, part.Digest, part.Size, "")
			continue
		}
		if existing, ok := resourceIndex[part.FieldName]; ok && existing.SHA256 == part.Digest && existing.Path != "" {
			part.ResourceID = existing.ID
			part.Preview = buildUploadFileRefPlaceholder(part.FieldName, part.FileName, part.Digest, part.Size, existing.ID)
			continue
		}
		content, readErr := readMultipartFilePartContent(fixed, part.FieldName, part.FileName)
		if readErr != nil {
			log.Warnf("http_fuzztest: read multipart file part %s failed: %v", part.FieldName, readErr)
			part.Preview = buildUploadFileRefPlaceholder(part.FieldName, part.FileName, part.Digest, part.Size, "")
			continue
		}
		ref, saveErr := saveLoopHTTPUploadFileResource(loop, part.FieldName, part.FileName, part.ContentType, content)
		if saveErr != nil {
			log.Warnf("http_fuzztest: externalize upload file part %s failed: %v", part.FieldName, saveErr)
			part.Preview = buildUploadFileRefPlaceholder(part.FieldName, part.FileName, part.Digest, part.Size, "")
			continue
		}
		part.ResourceID = ref.ID
		part.Preview = buildUploadFileRefPlaceholder(part.FieldName, part.FileName, ref.SHA256, ref.Size, ref.ID)
		resourceIndex[part.FieldName] = ref
		resources = append(resources, ref)
	}

	promptSafe := sanitizeHTTPRequestForPrompt(fixed, summary)
	return summary, resources, promptSafe
}

func parseMultipartUploadSummary(rawPacket []byte) (*loopHTTPUploadRequestSummary, error) {
	req, err := lowhttp.ParseBytesToHttpRequest(rawPacket)
	if err != nil {
		return nil, err
	}
	boundary := extractMultipartBoundary(req.Header.Get("Content-Type"))
	if boundary == "" {
		return &loopHTTPUploadRequestSummary{IsMultipart: true}, nil
	}

	reader, err := req.MultipartReader()
	if err != nil {
		return nil, err
	}

	summary := &loopHTTPUploadRequestSummary{
		IsMultipart: true,
		Boundary:    boundary,
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return summary, err
		}
		partSummary := parseMultipartPartSummary(part)
		summary.Parts = append(summary.Parts, partSummary)
		_ = part.Close()
	}
	return summary, nil
}

func parseMultipartPartSummary(part *multipart.Part) loopHTTPUploadPartSummary {
	fieldName := part.FormName()
	fileName := part.FileName()
	contentType := strings.TrimSpace(part.Header.Get("Content-Type"))
	content, _ := io.ReadAll(part)
	digest := sha256.Sum256(content)
	partSummary := loopHTTPUploadPartSummary{
		FieldName:   fieldName,
		IsFile:      fileName != "",
		FileName:    fileName,
		ContentType: contentType,
		Size:        int64(len(content)),
		Digest:      hex.EncodeToString(digest[:]),
	}
	if partSummary.IsFile {
		partSummary.Preview = buildUploadPartPreview(content)
	} else {
		partSummary.Preview = utils.ShrinkString(string(content), loopHTTPUploadPreviewMaxBytes)
	}
	return partSummary
}

func readMultipartFilePartContent(rawPacket []byte, fieldName, fileName string) ([]byte, error) {
	req, err := lowhttp.ParseBytesToHttpRequest(rawPacket)
	if err != nil {
		return nil, err
	}
	reader, err := req.MultipartReader()
	if err != nil {
		return nil, err
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if part.FormName() != fieldName {
			_ = part.Close()
			continue
		}
		if fileName != "" && part.FileName() != fileName {
			_ = part.Close()
			continue
		}
		content, readErr := io.ReadAll(part)
		_ = part.Close()
		return content, readErr
	}
	return nil, fmt.Errorf("multipart file part %q not found", fieldName)
}

func saveLoopHTTPUploadFileResource(loop *reactloops.ReActLoop, fieldName, fileName, contentType string, content []byte) (loopHTTPUploadFileResource, error) {
	ref := loopHTTPUploadFileResource{}
	if loop == nil {
		return ref, fmt.Errorf("loop is nil")
	}
	dataDir := loop.GetLoopContentDir("data")
	if dataDir == "" {
		return ref, fmt.Errorf("loop data dir is empty")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return ref, err
	}

	digest := sha256.Sum256(content)
	digestHex := hex.EncodeToString(digest[:])
	ref = loopHTTPUploadFileResource{
		ID:          uuid.NewString(),
		FieldName:   fieldName,
		FileName:    fileName,
		ContentType: contentType,
		Size:        int64(len(content)),
		SHA256:      digestHex,
		Path: filepath.Join(
			dataDir,
			fmt.Sprintf("upload_%s_%s_%s.bin", sanitizeUploadFileToken(fieldName), sanitizeUploadFileToken(fileName), digestHex[:12]),
		),
	}
	if err := os.WriteFile(ref.Path, content, 0o644); err != nil {
		return loopHTTPUploadFileResource{}, err
	}
	return ref, nil
}

func sanitizeUploadFileToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "part"
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func buildUploadPartPreview(content []byte) string {
	if len(content) == 0 {
		return "(empty)"
	}
	if !utf8.Valid(content) {
		return fmt.Sprintf("(binary %d bytes)", len(content))
	}
	return utils.ShrinkString(string(content), loopHTTPUploadPreviewMaxBytes)
}

func buildUploadFileRefPlaceholder(fieldName, fileName, digest string, size int64, resourceID string) string {
	if resourceID != "" {
		return fmt.Sprintf(`{{upload_file_ref(field=%q, filename=%q, sha256=%q, size=%d, id=%q)}}`, fieldName, fileName, digest, size, resourceID)
	}
	return fmt.Sprintf(`{{upload_file_ref(field=%q, filename=%q, sha256=%q, size=%d)}}`, fieldName, fileName, digest, size)
}

func sanitizeHTTPRequestForPrompt(rawPacket []byte, summary *loopHTTPUploadRequestSummary) string {
	rawPacket = lowhttp.FixHTTPRequest(rawPacket)
	if !lowhttp.IsMultipartFormDataRequest(rawPacket) {
		return string(rawPacket)
	}
	headers, _ := lowhttp.SplitHTTPPacketFast(rawPacket)
	sanitizedBody := sanitizeMultipartBodyForPrompt(rawPacket, summary)
	if strings.TrimSpace(headers) == "" {
		return string(sanitizedBody)
	}
	if !strings.HasSuffix(headers, "\r\n\r\n") && !strings.HasSuffix(headers, "\n\n") {
		headers += "\r\n\r\n"
	}
	return headers + string(sanitizedBody)
}

func sanitizeMultipartBodyForPrompt(rawPacket []byte, summary *loopHTTPUploadRequestSummary) []byte {
	_, body := lowhttp.SplitHTTPPacketFast(rawPacket)
	if len(body) == 0 {
		return body
	}

	placeholderByField := buildUploadPlaceholderByField(summary)

	type sanitizedPart struct {
		header  textproto.MIMEHeader
		content []byte
	}
	parts := make([]sanitizedPart, 0)

	if err := lowhttp.ParseMultiPartFormWithCallback(rawPacket, func(part *yakmultipart.Part) {
		fieldName := part.FormName()
		fileName := part.FileName()
		content, _ := io.ReadAll(part)

		finalContent := content
		if fileName != "" {
			if placeholder, ok := placeholderByField[fieldName]; ok {
				finalContent = []byte(placeholder)
			} else if len(content) > loopHTTPUploadPartExternalizeThreshold || !utf8.Valid(content) {
				digest := sha256.Sum256(content)
				placeholder := buildUploadFileRefPlaceholder(fieldName, fileName, hex.EncodeToString(digest[:]), int64(len(content)), "")
				finalContent = []byte(placeholder)
			}
		}

		parts = append(parts, sanitizedPart{
			header:  cloneMIMEHeader(part.Header),
			content: finalContent,
		})
	}); err != nil {
		return []byte("(multipart body omitted for prompt)")
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if boundary := extractBoundaryFromSummary(summary, body); boundary != "" {
		_ = writer.SetBoundary(boundary)
	}

	for _, record := range parts {
		partWriter, err := writer.CreatePart(record.header)
		if err != nil {
			return []byte("(multipart body omitted for prompt)")
		}
		_, _ = partWriter.Write(record.content)
	}
	_ = writer.Close()
	return buffer.Bytes()
}

func buildUploadPlaceholderByField(summary *loopHTTPUploadRequestSummary) map[string]string {
	placeholderByField := make(map[string]string)
	if summary == nil {
		return placeholderByField
	}
	for _, part := range summary.Parts {
		if !part.IsFile {
			continue
		}
		if part.ResourceID != "" || part.Size > int64(loopHTTPUploadPartExternalizeThreshold) || !utf8.ValidString(part.Preview) {
			placeholderByField[part.FieldName] = buildUploadFileRefPlaceholder(part.FieldName, part.FileName, part.Digest, part.Size, part.ResourceID)
		}
	}
	return placeholderByField
}

func extractBoundaryFromSummary(summary *loopHTTPUploadRequestSummary, body []byte) string {
	if summary != nil && strings.TrimSpace(summary.Boundary) != "" {
		return summary.Boundary
	}
	return extractMultipartBoundaryFromBody(body)
}

func extractMultipartBoundary(reqContentType string) string {
	if reqContentType == "" {
		return ""
	}
	if match := uploadBoundaryRegexp.FindStringSubmatch(reqContentType); len(match) >= 2 {
		return strings.Trim(match[1], `"`)
	}
	return ""
}

func extractMultipartBoundaryFromBody(body []byte) string {
	line, _, _ := bytes.Cut(body, []byte("\n"))
	line = bytes.TrimSpace(line)
	return strings.TrimPrefix(string(line), "--")
}

func cloneMIMEHeader(header textproto.MIMEHeader) textproto.MIMEHeader {
	if header == nil {
		return make(textproto.MIMEHeader)
	}
	cloned := make(textproto.MIMEHeader, len(header))
	for key, values := range header {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func formatUploadRequestSummaryForPrompt(summary *loopHTTPUploadRequestSummary) string {
	if summary == nil || !summary.IsMultipart || len(summary.Parts) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("multipart/form-data upload\n")
	if summary.Boundary != "" {
		out.WriteString(fmt.Sprintf("boundary: %s\n", summary.Boundary))
	}
	for _, part := range summary.Parts {
		if part.IsFile {
			out.WriteString(fmt.Sprintf("- file field %q filename=%q content_type=%q size=%d sha256=%s",
				part.FieldName, part.FileName, part.ContentType, part.Size, part.Digest))
			if part.ResourceID != "" {
				out.WriteString(fmt.Sprintf(" resource_id=%q", part.ResourceID))
			}
			out.WriteString("\n")
			if part.Preview != "" {
				out.WriteString(fmt.Sprintf("  preview: %s\n", part.Preview))
			}
			continue
		}
		out.WriteString(fmt.Sprintf("- field %q value=%q size=%d\n", part.FieldName, part.Preview, part.Size))
	}
	return strings.TrimSpace(out.String())
}

func buildUploadAwareHTTPRequestStreamSummary(raw string, isHTTPS bool, summary *loopHTTPUploadRequestSummary) (string, string) {
	requestURL := extractRequestURL(raw, isHTTPS)
	if summary == nil || !summary.IsMultipart {
		_, body := lowhttp.SplitHTTPPacketFast([]byte(raw))
		return requestURL, fmt.Sprintf("URL: %s BODY: [(%d) bytes]", requestURL, len(body))
	}

	fileCount := 0
	fieldCount := 0
	for _, part := range summary.Parts {
		if part.IsFile {
			fileCount++
		} else {
			fieldCount++
		}
	}
	return requestURL, fmt.Sprintf("URL: %s MULTIPART: files=%d fields=%d", requestURL, fileCount, fieldCount)
}

func getLoopOriginalRequestForPrompt(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	if safe := strings.TrimSpace(loop.Get(loopHTTPUploadOriginalPromptSafeKey)); safe != "" {
		return safe
	}
	return strings.TrimSpace(loop.Get("original_request"))
}

func getLoopRepresentativeRequestForPrompt(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	if safe := strings.TrimSpace(loop.Get(loopHTTPUploadRepresentativeSafeKey)); safe != "" {
		return safe
	}
	return strings.TrimSpace(loop.Get("representative_request"))
}

func sanitizeRequestTextForPrompt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !lowhttp.IsMultipartFormDataRequest([]byte(raw)) {
		return raw
	}
	summary, err := parseMultipartUploadSummary([]byte(raw))
	if err != nil {
		return sanitizeHTTPRequestForPrompt([]byte(raw), nil)
	}
	return sanitizeHTTPRequestForPrompt([]byte(raw), summary)
}

func compareRequestsForPrompt(original, modified string) string {
	return compareRequests(sanitizeRequestTextForPrompt(original), sanitizeRequestTextForPrompt(modified))
}
