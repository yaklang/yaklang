package inputresolver

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const MaxReadBytes = 256 << 10
const MaxSearchResults = 200

type File struct {
	ResourceID   string `json:"resource_id"`
	RelativePath string `json:"relative_path"`
	SizeBytes    uint64 `json:"size_bytes"`
	SHA256       string `json:"sha256"`
}

func (w *Workspace) Files() []File {
	files := make([]File, 0, len(w.manifest.Resources))
	for _, resource := range w.manifest.Resources {
		files = append(files, File{resource.ResourceId, resource.RelativePath, resource.SizeBytes, resource.Sha256})
	}
	return files
}

func (w *Workspace) Info() map[string]any {
	return map[string]any{"workspace_id": w.manifest.WorkspaceId,
		"manifest_id": w.manifest.ManifestId, "inputs": w.Files(), "output_path": "outputs",
		"max_read_bytes": MaxReadBytes, "max_search_results": MaxSearchResults}
}

// Identity is for trusted event/result correlation, not model context.
func (w *Workspace) Identity() Event {
	return Event{RunID: w.manifest.RunId, SessionID: w.manifest.SessionId, AttemptID: w.manifest.AttemptId, WorkspaceID: w.manifest.WorkspaceId, ManifestID: w.manifest.ManifestId}
}

func (w *Workspace) ManifestID() string { return w.manifest.ManifestId }

// check and every data operation run under the workspace lock. Cleanup cancels
// first, waits for active reads, then removes the only directory it owns.
func (w *Workspace) check(ctx context.Context) error {
	if w.closed || w.ctx.Err() != nil || ctx.Err() != nil {
		return fail("input_cancelled", "")
	}
	return nil
}

func inputSelection(value string) (string, error) {
	if value == "" || value == "." {
		return "inputs", nil
	}
	if value == "inputs" {
		return value, nil
	}
	if !SafeInputPath(value) {
		return "", fail("input_path_denied", "")
	}
	return value, nil
}

func selected(file, selection string) bool {
	return file == selection || strings.HasPrefix(file, selection+"/")
}

func (w *Workspace) List(ctx context.Context, selection string) (map[string]any, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	selection, err := inputSelection(selection)
	if err != nil {
		return nil, err
	}
	files := make([]map[string]any, 0)
	for _, resource := range w.manifest.Resources {
		if selected(resource.RelativePath, selection) {
			files = append(files, map[string]any{"path": resource.RelativePath, "relative_path": resource.RelativePath,
				"resource_id": resource.ResourceId, "size": resource.SizeBytes, "size_bytes": resource.SizeBytes, "sha256": resource.Sha256})
		}
	}
	return map[string]any{"path": selection, "files": files, "count": len(files), "truncated": false}, nil
}

func (w *Workspace) resource(value string) (*aiv1.InputResource, error) {
	if !SafeInputPath(value) {
		return nil, fail("input_path_denied", "")
	}
	for _, resource := range w.manifest.Resources {
		if resource.RelativePath == value {
			return resource, nil
		}
	}
	return nil, fail("input_path_denied", "")
}

func (w *Workspace) open(resource *aiv1.InputResource) (*os.File, error) {
	file, err := openBeneath(w.root, resource.RelativePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fail("input_path_denied", resource.ResourceId)
	}
	info, err := file.Stat()
	if err != nil || uint64(info.Size()) != resource.SizeBytes || info.Mode().Perm()&0222 != 0 {
		file.Close()
		return nil, fail("input_file_changed", resource.ResourceId)
	}
	return file, nil
}

func (w *Workspace) Read(ctx context.Context, name string, offset, limit int64) (map[string]any, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	resource, err := w.resource(name)
	if err != nil {
		return nil, err
	}
	if offset < 0 || uint64(offset) > resource.SizeBytes {
		return nil, fail("input_range_invalid", resource.ResourceId)
	}
	if limit <= 0 || limit > MaxReadBytes {
		limit = MaxReadBytes
	}
	file, err := w.open(resource)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Align an arbitrary byte offset forward; never expose a partial leading rune.
	offset, err = alignInputUTF8ReadOffset(file, offset, int64(resource.SizeBytes))
	if err != nil {
		return nil, fail("input_read_failed", resource.ResourceId)
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, fail("input_read_failed", resource.ResourceId)
	}
	content, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, fail("input_read_failed", resource.ResourceId)
	}
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	readBytes := int64(len(content))
	w.event("input.file.access", Event{ResourceID: resource.ResourceId, Path: name, Operation: "read", Offset: offset, EndOffset: offset + readBytes, BytesRead: readBytes})
	// A page boundary may split a UTF-8 sequence. Trim at most three trailing
	// bytes and return next_offset so callers can continue without losing data.
	for i := 0; i < 3 && !utf8.Valid(content) && len(content) > 0; i++ {
		content = content[:len(content)-1]
	}
	if len(content) == 0 && readBytes > 0 {
		return nil, fail("input_range_invalid", resource.ResourceId)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return nil, fail("input_text_required", resource.ResourceId)
	}
	sum := sha256.Sum256(content)
	return map[string]any{"path": name, "resource_id": resource.ResourceId, "offset": offset,
		"next_offset": offset + int64(len(content)), "content": string(content), "read_bytes": len(content),
		"file_size": resource.SizeBytes, "sha256": hex.EncodeToString(sum[:]),
		"truncated": uint64(offset)+uint64(len(content)) < resource.SizeBytes}, nil
}

// Search streams bounded line fragments, retaining only a query-length overlap.
// A single huge line cannot grow the heap, and access events record the bytes
// actually examined even when a match limit or cancellation ends the scan.
func (w *Workspace) Search(ctx context.Context, selection, query string, caseSensitive bool, limit int) (map[string]any, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	selection, err := inputSelection(selection)
	if err != nil {
		return nil, err
	}
	if query == "" || !utf8.ValidString(query) || len(query) > 1024 || strings.ContainsRune(query, '\x00') || strings.ContainsRune(query, '\n') {
		return nil, fail("input_query_invalid", "")
	}
	if limit <= 0 || limit > MaxSearchResults {
		limit = MaxSearchResults
	}
	match := inputSearchMatcher(query, caseSensitive)
	overlap := len(query) - 1
	if !caseSensitive {
		overlap = utf8.RuneCountInString(query)*utf8.UTFMax + utf8.UTFMax
	}
	results := make([]map[string]any, 0)
	var total int64
	truncated := false
	for _, resource := range w.manifest.Resources {
		if !selected(resource.RelativePath, selection) {
			continue
		}
		if len(results) >= limit {
			truncated = true
			break
		}
		file, err := w.open(resource)
		if err != nil {
			return nil, err
		}
		reader := bufio.NewReaderSize(file, 64<<10)
		var scanned int64
		line := int64(1)
		tail := ""
		scanErr := func() error {
			defer file.Close()
			defer func() {
				w.event("input.file.access", Event{ResourceID: resource.ResourceId, Path: resource.RelativePath,
					Operation: "search", BytesRead: scanned, EndOffset: scanned, StartLine: 1, EndLine: line})
			}()
			for {
				if err := w.check(ctx); err != nil {
					return err
				}
				fragment, readErr := reader.ReadSlice('\n')
				previous := scanned
				scanned += int64(len(fragment))
				text := tail + string(fragment)
				// The overlap can contain an already reported match. Skip those
				// while preserving the original (not lowercased) byte positions.
				searchFrom := 0
				for searchFrom < len(text) {
					span := match(text[searchFrom:])
					if span == nil {
						break
					}
					index, matchEnd := searchFrom+span[0], searchFrom+span[1]
					if matchEnd <= len(tail) {
						searchFrom = matchEnd
						continue
					}
					offset := previous - int64(len(tail)) + int64(index)
					end := index + 512
					if end > len(text) {
						end = len(text)
					}
					results = append(results, map[string]any{"path": resource.RelativePath, "resource_id": resource.ResourceId,
						"line": line, "offset": offset, "content": strings.ToValidUTF8(text[index:end], "�")})
					if len(results) >= limit {
						truncated = uint64(scanned) < resource.SizeBytes
						return nil
					}
					break
				}
				if readErr == bufio.ErrBufferFull {
					keep := overlap
					if keep > len(text) {
						keep = len(text)
					}
					tail = text[len(text)-keep:]
				} else {
					tail = ""
					if len(fragment) > 0 && fragment[len(fragment)-1] == '\n' {
						line++
					}
				}
				if readErr == io.EOF {
					return nil
				}
				if readErr != nil && readErr != bufio.ErrBufferFull {
					return fail("input_read_failed", resource.ResourceId)
				}
			}
		}()
		total += scanned
		if scanErr != nil {
			return nil, scanErr
		}
	}
	return map[string]any{"path": selection, "query": query, "matches": results, "count": len(results), "scanned_bytes": total, "truncated": truncated}, nil
}

// WriteOutput exposes a bounded writable capability, never an input mutation.
// Only one flat output filename is accepted, avoiding directory/symlink races.
func (w *Workspace) WriteOutput(ctx context.Context, name, content string) (map[string]any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(name, "outputs/") || path.Clean(name) != name || strings.Count(name, "/") != 1 ||
		strings.ContainsAny(name, "\\:\x00") || len(name) > 240 || path.Base(name) == "." || strings.TrimSpace(name) != name {
		return nil, fail("input_path_denied", "")
	}
	if uint64(len(content)) > w.resolver.config.OutputBytes-w.outputBytes {
		return nil, fail("input_output_quota_exceeded", "")
	}
	file, err := openBeneath(w.root, name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fail("input_output_write_failed", "")
	}
	n, writeErr := io.WriteString(file, content)
	w.outputBytes += uint64(n)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return nil, fail("input_output_write_failed", "")
	}
	return map[string]any{"path": name, "written_bytes": n}, nil
}

func (w *Workspace) RootForDiagnostics() string { return filepath.Clean(w.root) }

func (w *Workspace) String() string { return fmt.Sprintf("input workspace %s", w.manifest.WorkspaceId) }

func alignInputUTF8ReadOffset(input *os.File, requestedOffset, fileSize int64) (int64, error) {
	if requestedOffset == 0 || requestedOffset == fileSize {
		return requestedOffset, nil
	}
	probeSize := int64(utf8.UTFMax)
	if remaining := fileSize - requestedOffset; remaining < probeSize {
		probeSize = remaining
	}
	probe := make([]byte, probeSize)
	readBytes, err := input.ReadAt(probe, requestedOffset)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	for skip := 0; skip < readBytes; skip++ {
		if probe[skip]&0xc0 != 0x80 {
			return requestedOffset + int64(skip), nil
		}
	}
	return requestedOffset + int64(readBytes), nil
}

// ASCII folding is the fast path for large logs. Unicode folding must search
// the original text because lowercasing can change UTF-8 byte lengths.
func inputSearchMatcher(query string, caseSensitive bool) func(string) []int {
	literal := func(text, needle string) []int {
		index := strings.Index(text, needle)
		if index < 0 {
			return nil
		}
		return []int{index, index + len(needle)}
	}
	if caseSensitive {
		return func(text string) []int { return literal(text, query) }
	}
	folded := strings.ToLower(query)
	matcher := regexp.MustCompile("(?i:" + regexp.QuoteMeta(query) + ")")
	ascii := func(text string) bool {
		for i := 0; i < len(text); i++ {
			if text[i] >= utf8.RuneSelf {
				return false
			}
		}
		return true
	}
	queryASCII := ascii(query)
	return func(text string) []int {
		if queryASCII && ascii(text) {
			return literal(strings.ToLower(text), folded)
		}
		return matcher.FindStringIndex(text)
	}
}
