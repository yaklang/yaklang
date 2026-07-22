package yakit

import (
	"bytes"
	"encoding/json"
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
	"github.com/yaklang/yaklang/common/schema"
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
//   - HeaderFile holds the request headers (same contract as flat spill).
//   - BodyFile is a 0-byte placeholder: the real file contents live in the
//     sidecar part files (MultipartDir). The complete multipart body is
//     rebuilt on demand by GetHTTPFlowBodyById via io.Pipe streaming. The
//     placeholder name anchors the sidecar directory derivation
//     (multipartSidecarDirFromBodyFile) so cleanup/locating parts work from
//     the persisted TooLargeRequestBodyFile path alone.
//   - MultipartDir is the sidecar directory containing the per-part files
//     and manifest.json.
//   - Manifest lists every file part; also persisted to manifest.json in
//     MultipartDir for downstream readers.
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
// file part to a sidecar file, writes manifest.json, leaves a 0-byte body
// placeholder, and returns a skeleton packet safe to store in the DB. The
// complete multipart body is rebuilt on demand by GetHTTPFlowBodyById.
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

	// Prepare sidecar directory for per-part files. Its name is derived from
	// the spill suffix so cleanup can locate it from the persisted
	// TooLargeRequestBodyFile path (its parent dir) without extra state.
	uid := ksuid.New().String()
	suffix := fmt.Sprintf("%v_%v", time.Now().Format(utils.DatetimePretty()), uid)
	tempDir := consts.GetDefaultYakitBaseTempDir()
	dir := filepath.Join(tempDir, fmt.Sprintf("large-request-body-%v-parts", suffix))
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

			partFileName := fmt.Sprintf("part-%d-%s.txt", partIndex, sanitizeFilename(filename))
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

	// Write manifest.json into the sidecar directory so downstream readers
	// (toHTTPFlowGRPCModel, GetHTTPFlowBodyById) can enumerate file parts
	// without re-parsing the skeleton.
	if err := writeMultipartManifest(dir, res.Manifest); err != nil {
		_ = os.RemoveAll(dir)
		_ = os.Remove(headerPath)
		return res, err
	}

	// TooLargeRequestBodyFile points at the first spilled part file. It is a
	// real file (not a placeholder), serves as the anchor from which the
	// sidecar directory is derived (its parent dir), and keeps
	// IsTooLargeRequest / non-multipart read paths consistent. The complete
	// multipart body is rebuilt on demand by GetHTTPFlowBodyById (streamed via
	// io.Pipe) and LoadHTTPFlowRequestPacket.
	bodyPath := filepath.Join(dir, res.Manifest[0].File)

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
			partFileName := fmt.Sprintf("part-%d-%s.txt", partIndex, sanitizeFilename(p.FileName()))
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

// FlowIsMultipartSpill reports whether a stored HTTPFlow's request was
// skeletonized as a multipart spill: the request is marked too-large and the
// in-DB skeleton carries the multipart placeholder.
func FlowIsMultipartSpill(flow *schema.HTTPFlow) bool {
	if flow == nil || !flow.IsTooLargeRequest || flow.TooLargeRequestBodyFile == "" {
		return false
	}
	return containsMultipartSpillMarker([]byte(flow.GetRequest()))
}

// FlowMultipartSidecarDir returns the sidecar directory path for a flow's
// spilled multipart parts, derived from TooLargeRequestBodyFile. Returns ""
// when not applicable.
func FlowMultipartSidecarDir(flow *schema.HTTPFlow) string {
	if flow == nil {
		return ""
	}
	return multipartSidecarDirFromBodyFile(flow.TooLargeRequestBodyFile)
}

// LoadFlowMultipartManifest loads the part manifest for a flow from its
// sidecar directory. Returns nil when the flow is not a multipart spill or
// the manifest is absent.
func LoadFlowMultipartManifest(flow *schema.HTTPFlow) ([]multipartPartMeta, error) {
	if !FlowIsMultipartSpill(flow) {
		return nil, nil
	}
	return loadMultipartManifest(FlowMultipartSidecarDir(flow))
}

// FlowMultipartSkeletonBody returns the skeleton body (headers stripped) of a
// multipart-spilled flow, used as the template for on-demand rebuild.
func FlowMultipartSkeletonBody(flow *schema.HTTPFlow) []byte {
	if !FlowIsMultipartSpill(flow) {
		return nil
	}
	_, body := lowhttp.SplitHTTPPacketFast(flow.GetRequest())
	return []byte(body)
}

// OpenFlowMultipartPart opens one spilled part file by manifest entry index.
// partIndex must match a manifest entry's PartIndex.
func OpenFlowMultipartPart(flow *schema.HTTPFlow, partIndex int) (*os.File, string, error) {
	manifest, err := LoadFlowMultipartManifest(flow)
	if err != nil {
		return nil, "", err
	}
	var meta *multipartPartMeta
	for i := range manifest {
		if manifest[i].Index == partIndex {
			meta = &manifest[i]
			break
		}
	}
	if meta == nil {
		return nil, "", utils.Errorf("multipart part %d not found in manifest", partIndex)
	}
	f, err := openMultipartPart(FlowMultipartSidecarDir(flow), *meta)
	if err != nil {
		return nil, "", err
	}
	return f, meta.Filename, nil
}

// RebuildFlowMultipartBody returns a reader streaming the complete rebuilt
// multipart body for a flow (skeleton text fields + spilled file parts from
// disk), via io.Pipe so large parts never load fully into memory.
func RebuildFlowMultipartBody(flow *schema.HTTPFlow) (io.Reader, error) {
	if !FlowIsMultipartSpill(flow) {
		return nil, utils.Error("flow is not a multipart spill")
	}
	return rebuildMultipartBodyToReader(FlowMultipartSkeletonBody(flow), FlowMultipartSidecarDir(flow)), nil
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
// from the persisted TooLargeRequestBodyFile path. For multipart spills the
// body file is the first spilled part file, so its parent directory is the
// sidecar. Returns "" when bodyFile is empty.
func multipartSidecarDirFromBodyFile(bodyFile string) string {
	if bodyFile == "" {
		return ""
	}
	return filepath.Dir(bodyFile)
}

// cleanupMultipartSidecar removes the per-part sidecar directory holding the
// spilled file parts (and manifest.json) for a multipart flow. The body file
// for a multipart spill is the first part file, so the sidecar is its parent
// directory. To avoid nuking the shared temp root for flat (non-multipart)
// spills — whose body file also lives in the temp root — only a directory
// whose base name ends with "-parts" is removed. Safe to call for any flow.
func cleanupMultipartSidecar(bodyFile string) {
	dir := multipartSidecarDirFromBodyFile(bodyFile)
	if dir == "" {
		return
	}
	if !strings.HasSuffix(filepath.Base(dir), "-parts") {
		return
	}
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		_ = os.RemoveAll(dir)
	}
}

// manifestFileName is the sidecar file recording the per-part metadata so
// downstream readers can enumerate file parts without re-parsing the skeleton.
const manifestFileName = "manifest.json"

// writeMultipartManifest persists the manifest entries as JSON into sidecarDir.
func writeMultipartManifest(sidecarDir string, manifest []multipartPartMeta) error {
	if sidecarDir == "" {
		return utils.Error("empty sidecar dir")
	}
	data, err := jsonMarshalManifest(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sidecarDir, manifestFileName), data, 0o644)
}

// loadMultipartManifest reads manifest.json from sidecarDir. Returns nil
// manifest and no error when the file is absent (e.g. flat spill or legacy).
func loadMultipartManifest(sidecarDir string) ([]multipartPartMeta, error) {
	if sidecarDir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(sidecarDir, manifestFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return jsonUnmarshalManifest(data)
}

// openMultipartPart opens the on-disk file for one spilled part.
func openMultipartPart(sidecarDir string, meta multipartPartMeta) (*os.File, error) {
	if sidecarDir == "" {
		return nil, utils.Error("empty sidecar dir")
	}
	return os.Open(filepath.Join(sidecarDir, meta.File))
}

// manifestJSONEntry is the on-disk JSON representation of a part meta. Field
// names are stable and lowerCamel to match the gRPC MultipartFileInfo shape.
type manifestJSONEntry struct {
	PartIndex    int    `json:"partIndex"`
	FieldName    string `json:"fieldName"`
	Filename     string `json:"filename"`
	ContentType  string `json:"contentType"`
	Size         int64  `json:"size"`
	File         string `json:"file"`
}

func jsonMarshalManifest(manifest []multipartPartMeta) ([]byte, error) {
	entries := make([]manifestJSONEntry, len(manifest))
	for i, m := range manifest {
		entries[i] = manifestJSONEntry{
			PartIndex:   m.Index,
			FieldName:   m.FieldName,
			Filename:    m.Filename,
			ContentType: m.ContentType,
			Size:        m.Size,
			File:        m.File,
		}
	}
	return json.MarshalIndent(entries, "", "  ")
}

func jsonUnmarshalManifest(data []byte) ([]multipartPartMeta, error) {
	var entries []manifestJSONEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	out := make([]multipartPartMeta, len(entries))
	for i, e := range entries {
		out[i] = multipartPartMeta{
			Index:       e.PartIndex,
			FieldName:   e.FieldName,
			Filename:    e.Filename,
			ContentType: e.ContentType,
			Size:        e.Size,
			File:        e.File,
		}
	}
	return out, nil
}

// rebuildMultipartBodyToReader returns a reader that streams the complete
// multipart body rebuilt from the skeleton (text fields verbatim) plus the
// spilled file parts (streamed from disk). It uses an io.Pipe so large file
// parts never need to be fully loaded into memory: a goroutine writes into the
// pipe while the caller reads from the returned reader.
//
// If the rebuild fails, the goroutine closes the pipe with the error and the
// caller's Read will surface it.
func rebuildMultipartBodyToReader(skeletonBody []byte, sidecarDir string) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		err := rebuildMultipartBodyToWriter(skeletonBody, sidecarDir, pw)
		_ = pw.CloseWithError(err)
	}()
	return pr
}