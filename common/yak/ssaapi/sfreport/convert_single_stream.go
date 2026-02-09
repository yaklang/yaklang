package sfreport

import (
	"bytes"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type streamDedupState struct {
	mu       sync.Mutex
	seen     map[string]struct{}
	lastUsed time.Time
}

var (
	streamDedup      sync.Map // key(string) -> *streamDedupState
	streamDedupSweep int64
)

func getStreamDedupState(key string) *streamDedupState {
	if key == "" {
		return nil
	}
	if v, ok := streamDedup.Load(key); ok {
		st := v.(*streamDedupState)
		st.mu.Lock()
		st.lastUsed = time.Now()
		st.mu.Unlock()
		return st
	}
	st := &streamDedupState{seen: make(map[string]struct{}, 256), lastUsed: time.Now()}
	if v, loaded := streamDedup.LoadOrStore(key, st); loaded {
		return v.(*streamDedupState)
	}
	return st
}

func maybeSweepStreamDedup() {
	now := time.Now()
	last := atomic.LoadInt64(&streamDedupSweep)
	if last > 0 && now.Unix()-last < 60 {
		return
	}
	if !atomic.CompareAndSwapInt64(&streamDedupSweep, last, now.Unix()) {
		return
	}
	ttl := 15 * time.Minute
	streamDedup.Range(func(k, v any) bool {
		key, _ := k.(string)
		st, _ := v.(*streamDedupState)
		if key == "" || st == nil {
			streamDedup.Delete(k)
			return true
		}
		st.mu.Lock()
		lu := st.lastUsed
		st.mu.Unlock()
		if !lu.IsZero() && now.Sub(lu) > ttl {
			streamDedup.Delete(k)
		}
		return true
	})
}

// ResetStreamFileDedup clears per-stream file-content dedup state.
// Intended to be called at the end of a scan task (streamKey=task_id).
func ResetStreamFileDedup(streamKey string) {
	if streamKey == "" {
		return
	}
	streamDedup.Delete(streamKey)
}

// ConvertSingleResultToStreamJSONWithOptions is a streaming-friendly variant of ConvertSingleResultToJSONWithOptions:
// - Returns the JSON string + number of risks included in this report (for accurate counters)
// - Optionally dedupes file.Content by ir_source_hash across calls in the same streamKey (usually task_id)
// This avoids yak scripts doing json.loads/json.dumps per result.
func ConvertSingleResultToStreamJSONWithOptions(
	result *ssaapi.SyntaxFlowResult,
	streamKey string,
	reportType ReportType,
	showDataflowPath bool,
	showFileContent bool,
	withFile bool,
	dedupFileContent bool,
) (string, int, error) {
	if result == nil {
		return "", 0, nil
	}

	report := NewReport(reportType)
	if showDataflowPath {
		report.config.showDataflowPath = true
	}
	if showFileContent {
		report.config.showFileContent = true
	}

	report.AddSyntaxFlowResult(result)
	if !withFile {
		report.File = nil
		report.IrSourceHashes = make(map[string]struct{})
		report.FileCount = 0
	}
	if len(report.Risks) == 0 {
		return "", 0, nil
	}

	addedRisks := len(report.Risks)
	if withFile && showFileContent && dedupFileContent && len(report.File) > 0 && streamKey != "" {
		st := getStreamDedupState(streamKey)
		if st != nil {
			st.mu.Lock()
			for _, f := range report.File {
				if f == nil {
					continue
				}
				h := strings.TrimSpace(f.IrSourceHash)
				if h == "" {
					continue
				}
				if _, ok := st.seen[h]; ok {
					if f.Content != "" {
						f.Content = ""
					}
					continue
				}
				st.seen[h] = struct{}{}
			}
			st.lastUsed = time.Now()
			st.mu.Unlock()
		}
	}

	buf := bytes.NewBuffer(nil)
	if err := report.Write(buf); err != nil {
		return "", 0, err
	}
	maybeSweepStreamDedup()
	return buf.String(), addedRisks, nil
}

// ConvertSingleResultToStreamPayload is a yak-script-friendly wrapper that returns a single map payload
// (so the scripting runtime doesn't need to support 3-value returns).
func ConvertSingleResultToStreamPayload(
	result *ssaapi.SyntaxFlowResult,
	streamKey string,
	reportType ReportType,
	showDataflowPath bool,
	showFileContent bool,
	withFile bool,
	dedupFileContent bool,
) (map[string]any, error) {
	j, n, err := ConvertSingleResultToStreamJSONWithOptions(result, streamKey, reportType, showDataflowPath, showFileContent, withFile, dedupFileContent)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"json":        j,
		"risk_count":  n,
		"has_payload": j != "",
	}, nil
}
