package yakit

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	customMultipart "github.com/yaklang/yaklang/common/utils/multipart"
)

// multipartSkeletonMarker is the prefix of the single-line placeholder that
// replaces a spilled file part inside the in-DB skeleton body. It carries the
// part index, original filename and size so the skeleton stays human-readable
// and the part can be located in the sidecar directory on rebuild.
//
// Example:
//
//	[[yakit: multipart file spilled, part=0, file=Yakit-1.4.8.exe, size=167232]]
const multipartSkeletonMarker = "[[yakit: multipart file spilled"

// multipartSpillMarkerPattern matches the placeholder line produced by
// spillMultipartFilesIfNeeded. Kept for callers that want to parse part
// index/filename/size out of a skeleton (e.g. a future single-part download).
var multipartSpillMarkerPattern = regexp.MustCompile(
	`(?m)^\[\[yakit: multipart file spilled, part=(\d+), file=(.*?), size=(\d+)\]\]$`,
)

// multipartSpillResult is the multipart-aware counterpart of
// largeRequestSpillResult. When IsTooLarge is true the request was a
// multipart/form-data body carrying at least one file part, and:
//   - StoredPacket is the in-DB skeleton (header + skeleton body with
//     placeholders) — small and editable.
//   - HeaderFile / BodyFile follow the same contract as the flat spill path:
//     HeaderFile holds the request headers, BodyFile holds the *rebuilt*
//     complete multipart body (skeleton text fields + spilled file parts
//     streamed back from disk). This keeps GetHTTPFlowBodyById unchanged.
//   - MultipartDir is the sidecar directory containing the per-part files.
//     It is always filepath.Dir(BodyFile); kept here for convenience.
//   - Manifest lists every file part, primarily for tests/diagnostics.
type multipartSpillResult struct {
	StoredPacket    []byte
	IsTooLarge      bool
	HeaderFile      string
	BodyFile        string
	MultipartDir    string
	OriginalBodyLen int
	Manifest        []multipartPartMeta
}

// multipartPartMeta records metadata about one spilled file part.
type multipartPartMeta struct {
	Index       int
	FieldName   string
	Filename    string
	ContentType string
	Size        int64
	File        string // disk filename inside MultipartDir (part-N-<sanitized>)
}

// spillMultipartFilesIfNeeded inspects an oversized request packet and, when
// it is a multipart/form-data body carrying at least one file part, spills each
// file part to a sidecar file, writes the rebuilt complete body to a temp body
// file, and returns a skeleton packet safe to store in the DB.
//
// When the packet is not multipart or carries no file part, IsTooLarge is false
// and the caller falls back to the flat large-request spill path.
func spillMultipartFilesIfNeeded(packet []byte) (multipartSpillResult, error) {
	var res multipartSpillResult
	if len(packet) == 0 {
		return res, nil
	}

	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	res.OriginalBodyLen = len(body)
	// Only engage for oversized bodies; small multiparts take the normal path.
	if len(body) <= maxHTTPFlowRequestBodyInDBBytes {
		return res, nil
	}

	ct := lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Type")
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		return res, nil
	}
	boundary, ok := params["boundary"]
	if !ok || boundary == "" {
		return res, nil
	}

	// First pass: enumerate parts using the boundary-tolerant reader and
	// decide whether this is worth skeletonizing (>=1 file part).
	mr := customMultipart.NewReader(bytes.NewReader(body))
	var partInfos []struct {
		header textproto.MIMEHeader
		isFile bool
	}
	for {
		p, err := mr.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			// Malformed multipart: bail out to flat spill so we never lose data.
			return res, nil
		}
		isFile := p.FileName() != ""
		partInfos = append(partInfos, struct {
			header textproto.MIMEHeader
			isFile bool
		}{header: copyMIMEHeader(p.Header), isFile: isFile})
	}
	if len(partInfos) == 0 {
		return res, nil
	}
	hasFilePart := false
	for _, pi := range partInfos {
		if pi.isFile {
			hasFilePart = true
			break
		}
	}
	if !hasFilePart {
		// Oversized multipart with only text fields: keep flat path, no
		// benefit in skeletonizing tiny editable fields.
		return res, nil
	}

	// Prepare sidecar directory for per-part files. It is derived from the
	// body file name (see multipartSidecarDirFromBodyFile) so cleanup can
	// locate it from the persisted TooLargeRequestBodyFile path alone.
	uid := ksuid.New().String()
	suffix := fmt.Sprintf("%v_%v", time.Now().Format(utils.DatetimePretty()), uid)
	bodyFileBase := fmt.Sprintf("large-request-body-%v.txt", suffix)
	bodyFileRel := bodyFileBase
	// Defer the real creation until after we know the temp dir, but we need
	// the dir up front for part files. Use the same base dir as OpenTempFile.
	tempDir := consts.GetDefaultYakitBaseTempDir()
	dir := filepath.Join(tempDir, strings.TrimSuffix(bodyFileBase, filepath.Ext(bodyFileBase))+"-parts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}
	res.MultipartDir = dir

	// Second pass: spill file parts to disk and build the skeleton body.
	// We write the multipart framing manually into skeletonBuf so the
	// skeleton keeps the original boundary and part headers verbatim (the
	// standard multipart.Writer would normalize them).
	mr = customMultipart.NewReader(bytes.NewReader(body))
	skeletonBuf := new(bytes.Buffer)

	partIndex := 0
	for {
		p, err := mr.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			_ = os.RemoveAll(dir)
			return res, nil
		}

		isFile := p.FileName() != ""
		// Opening boundary line for this part.
		skeletonBuf.WriteString("--" + boundary + "\r\n")
		if err := writePartHeaders(skeletonBuf, p.Header); err != nil {
			_ = os.RemoveAll(dir)
			return res, err
		}
		skeletonBuf.WriteString("\r\n")

		if isFile {
			fieldName := p.FormName()
			filename := p.FileName()
			contentType := p.GetHeader("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			partFileName := fmt.Sprintf("part-%d-%s", partIndex, sanitizeFilename(filename))
			partPath := filepath.Join(dir, partFileName)
			f, err := os.Create(partPath)
			if err != nil {
				_ = os.RemoveAll(dir)
				return res, err
			}
			n, copyErr := io.Copy(f, p)
			f.Close()
			if copyErr != nil {
				_ = os.RemoveAll(dir)
				return res, copyErr
			}

			res.Manifest = append(res.Manifest, multipartPartMeta{
				Index:       partIndex,
				FieldName:   fieldName,
				Filename:    filename,
				ContentType: contentType,
				Size:        n,
				File:        partFileName,
			})

			// Skeleton placeholder line replaces the file content; the
			// trailing CRLF terminates the part body.
			fmt.Fprintf(skeletonBuf, "%s, part=%d, file=%s, size=%d]]\r\n",
				multipartSkeletonMarker, partIndex, filename, n)
		} else {
			// Text field: preserve value verbatim in the skeleton.
			if _, err := io.Copy(skeletonBuf, p); err != nil {
				_ = os.RemoveAll(dir)
				return res, err
			}
			skeletonBuf.WriteString("\r\n")
		}
		partIndex++
	}
	// Closing boundary.
	skeletonBuf.WriteString("--" + boundary + "--\r\n")

	// Write the header file (same contract as flat spill).
	headerFP, err := utils.OpenTempFile(fmt.Sprintf("large-request-header-%v.txt", suffix))
	if err != nil {
		_ = os.RemoveAll(dir)
		return res, err
	}
	if _, err := headerFP.Write([]byte(header)); err != nil {
		headerFP.Close()
		_ = os.RemoveAll(dir)
		return res, err
	}
	headerPath := headerFP.Name()
	headerFP.Close()

	// Rebuild the complete body into the body file: stream file parts back
	// from disk alongside the skeleton's text fields. The body file name
	// matches the base used to derive the sidecar directory so cleanup can
	// locate the parts from the persisted body file path.
	bodyFP, err := utils.OpenTempFile(bodyFileRel)
	if err != nil {
		_ = os.RemoveAll(dir)
		_ = os.Remove(headerPath)
		return res, err
	}
	bodyPath := bodyFP.Name()
	if err := rebuildMultipartBodyToWriter(skeletonBuf.Bytes(), dir, bodyFP); err != nil {
		bodyFP.Close()
		_ = os.RemoveAll(dir)
		_ = os.Remove(headerPath)
		_ = os.Remove(bodyPath)
		return res, err
	}
	bodyFP.Close()

	stored := lowhttp.ReplaceHTTPPacketBody([]byte(header), skeletonBuf.Bytes(), false)
	res.StoredPacket = stored
	res.IsTooLarge = true
	res.HeaderFile = headerPath
	res.BodyFile = bodyPath
	return res, nil
}

// writePartHeaders writes the given part headers in a stable, verbatim style
// (key: value\r\n) into w. It preserves the original header casing stored by
// the custom multipart reader.
func writePartHeaders(w io.Writer, header textproto.MIMEHeader) error {
	// textproto.MIMEHeader canonicalizes keys, but the custom reader stores
	// them via CanonicalMIMEHeaderKey too, so this is consistent with the
	// reader's view. Iterate a stable order for reproducibility.
	keys := make([]string, 0, len(header))
	for k := range header {
		keys = append(keys, k)
	}
	// Put Content-Disposition first for readability of the skeleton.
	if cd, ok := header["Content-Disposition"]; ok && len(cd) > 0 {
		if _, err := fmt.Fprintf(w, "Content-Disposition: %s\r\n", cd[0]); err != nil {
			return err
		}
	}
	for _, k := range keys {
		if k == "Content-Disposition" {
			continue
		}
		vals := header[k]
		if len(vals) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(w, "%s: %s\r\n", k, vals[0]); err != nil {
			return err
		}
	}
	return nil
}

// rebuildMultipartBodyToWriter parses a skeleton multipart body and writes the
// complete body to dst, streaming each spilled file part from sidecarDir.
// Text fields are copied verbatim from the skeleton; file parts are replaced
// by the contents of the corresponding part-N-<filename> file on disk.
func rebuildMultipartBodyToWriter(skeletonBody []byte, sidecarDir string, dst io.Writer) error {
	// Detect boundary from the skeleton's first boundary line.
	boundary, err := detectBoundary(skeletonBody)
	if err != nil {
		return err
	}

	mr := customMultipart.NewReader(bytes.NewReader(skeletonBody))
	mw := multipart.NewWriter(dst)
	if err := mw.SetBoundary(boundary); err != nil {
		return err
	}
	partIndex := 0
	for {
		p, err := mr.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		isFile := p.FileName() != ""
		if isFile {
			partFileName := fmt.Sprintf("part-%d-%s", partIndex, sanitizeFilename(p.FileName()))
			partPath := filepath.Join(sidecarDir, partFileName)
			f, ferr := os.Open(partPath)
			if ferr != nil {
				return utils.Wrapf(ferr, "open spilled part file %q failed", partPath)
			}
			partWriter, werr := mw.CreatePart(p.Header)
			if werr != nil {
				f.Close()
				return werr
			}
			if _, cerr := io.Copy(partWriter, f); cerr != nil {
				f.Close()
				return cerr
			}
			f.Close()
		} else {
			// Text field: write the value verbatim from the skeleton into the
			// part body returned by CreatePart.
			partWriter, werr := mw.CreatePart(p.Header)
			if werr != nil {
				return werr
			}
			if _, cerr := io.Copy(partWriter, p); cerr != nil {
				return cerr
			}
		}
		partIndex++
	}
	return mw.Close()
}

// detectBoundary extracts the boundary string from a multipart skeleton body
// by locating the first "--<boundary>" line.
func detectBoundary(body []byte) (string, error) {
	lines := bytes.Split(body, []byte("\n"))
	for _, line := range lines {
		trimmed := bytes.TrimRight(line, "\r")
		if bytes.HasPrefix(trimmed, []byte("--")) {
			candidate := string(trimmed[2:])
			// A trailing "--" marks the closing delimiter, not a boundary.
			if strings.HasSuffix(candidate, "--") {
				candidate = candidate[:len(candidate)-2]
			}
			if candidate != "" {
				return candidate, nil
			}
		}
	}
	return "", utils.Error("multipart: boundary not found in skeleton body")
}

// containsMultipartSpillMarker reports whether the stored packet carries the
// multipart skeleton placeholder. Used to detect legacy/serialized flows that
// were skeletonized.
func containsMultipartSpillMarker(packet []byte) bool {
	return bytes.Contains(packet, []byte(multipartSkeletonMarker))
}

// sanitizeFilename strips path separators and other filesystem-unsafe chars
// from a part filename so it can be used as a disk filename inside the
// sidecar directory.
func sanitizeFilename(name string) string {
	if name == "" {
		return "unnamed"
	}
	base := filepath.Base(name)
	if base == "" || base == "." || base == string(os.PathSeparator) {
		base = "unnamed"
	}
	// Replace any remaining separators inside the base just in case.
	base = strings.ReplaceAll(base, string(os.PathSeparator), "_")
	base = strings.ReplaceAll(base, "/", "_")
	return base
}

func copyMIMEHeader(h textproto.MIMEHeader) textproto.MIMEHeader {
	out := make(textproto.MIMEHeader, len(h))
	for k, vs := range h {
		cp := make([]string, len(vs))
		copy(cp, vs)
		out[k] = cp
	}
	return out
}

// multipartSidecarDirFromBodyFile derives the per-part sidecar directory path
// from the persisted TooLargeRequestBodyFile path. The sidecar directory is
// the body file name with its extension replaced by "-parts", sitting next to
// the body file in the same temp dir. Returns "" when bodyFile is empty.
func multipartSidecarDirFromBodyFile(bodyFile string) string {
	if bodyFile == "" {
		return ""
	}
	dir := filepath.Dir(bodyFile)
	base := filepath.Base(bodyFile)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(dir, stem+"-parts")
}

// cleanupMultipartSidecar removes the per-part sidecar directory derived from
// bodyFile. Safe to call when bodyFile is a flat spill file (no sidecar
// directory exists).
func cleanupMultipartSidecar(bodyFile string) {
	dir := multipartSidecarDirFromBodyFile(bodyFile)
	if dir == "" {
		return
	}
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		_ = os.RemoveAll(dir)
	}
}