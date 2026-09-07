package yakgrpc

import (
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type manualLargeRequestReplacementTarget struct {
	replaceBody bool
	partIndex   int
}

func newManualLargeRequestReplacementTarget(replaceBody bool, partIndex int) (manualLargeRequestReplacementTarget, error) {
	if replaceBody {
		return manualLargeRequestReplacementTarget{replaceBody: true}, nil
	}
	if partIndex < 0 {
		return manualLargeRequestReplacementTarget{}, utils.Errorf("invalid multipart part index %d", partIndex)
	}
	return manualLargeRequestReplacementTarget{partIndex: partIndex}, nil
}

func (t manualLargeRequestReplacementTarget) description() string {
	if t.replaceBody {
		return "request body"
	}
	return fmt.Sprintf("multipart part %d", t.partIndex)
}

type manualLargeRequestReplacementUpload struct {
	file *os.File
	path string
}

// manualLargeRequestReplacementStore owns the bounded chunks uploaded while
// an oversized request is waiting in manual hijack. A target is either the
// complete non-multipart request body or one multipart file part. Completed
// files remain engine-local and are removed when the hijack task finishes.
type manualLargeRequestReplacementStore struct {
	active    map[manualLargeRequestReplacementTarget]*manualLargeRequestReplacementUpload
	completed map[manualLargeRequestReplacementTarget]string
}

func newManualLargeRequestReplacementStore() *manualLargeRequestReplacementStore {
	return &manualLargeRequestReplacementStore{
		active:    make(map[manualLargeRequestReplacementTarget]*manualLargeRequestReplacementUpload),
		completed: make(map[manualLargeRequestReplacementTarget]string),
	}
}

func (s *manualLargeRequestReplacementStore) consume(
	replaceBody bool,
	partIndex int,
	data []byte,
	start bool,
	eof bool,
	cancel bool,
) error {
	target, err := newManualLargeRequestReplacementTarget(replaceBody, partIndex)
	if err != nil {
		return err
	}
	if cancel {
		s.discard(target)
		return nil
	}
	if start {
		s.discard(target)
		tempDir := consts.GetDefaultYakitBaseTempDir()
		if err := os.MkdirAll(tempDir, 0o755); err != nil {
			return err
		}
		f, err := os.CreateTemp(tempDir, "mitm-large-request-replacement-*")
		if err != nil {
			return err
		}
		s.active[target] = &manualLargeRequestReplacementUpload{file: f, path: f.Name()}
	}
	upload := s.active[target]
	if upload == nil {
		return utils.Errorf("large request replacement %s was not started", target.description())
	}
	if len(data) > 0 {
		if _, err := upload.file.Write(data); err != nil {
			s.discard(target)
			return err
		}
	}
	if eof {
		if err := upload.file.Close(); err != nil {
			s.discard(target)
			return err
		}
		upload.file = nil
		delete(s.active, target)
		s.completed[target] = upload.path
	}
	return nil
}

func (s *manualLargeRequestReplacementStore) hasCompleted() bool {
	return len(s.completed) > 0
}

func (s *manualLargeRequestReplacementStore) hasActive() bool {
	return len(s.active) > 0
}

func (s *manualLargeRequestReplacementStore) multipartPaths() map[int]string {
	out := make(map[int]string, len(s.completed))
	for target, path := range s.completed {
		if !target.replaceBody {
			out[target.partIndex] = path
		}
	}
	return out
}

func (s *manualLargeRequestReplacementStore) bodyPath() string {
	return s.completed[manualLargeRequestReplacementTarget{replaceBody: true}]
}

func (s *manualLargeRequestReplacementStore) discard(target manualLargeRequestReplacementTarget) {
	if upload := s.active[target]; upload != nil {
		if upload.file != nil {
			_ = upload.file.Close()
		}
		_ = os.Remove(upload.path)
		delete(s.active, target)
	}
	if path := s.completed[target]; path != "" {
		_ = os.Remove(path)
		delete(s.completed, target)
	}
}

func (s *manualLargeRequestReplacementStore) close() {
	for target := range s.active {
		s.discard(target)
	}
	for target := range s.completed {
		s.discard(target)
	}
}

// renderMITMSubmittedRequest is the common execution boundary for a manually
// edited request in both MITM generations. Generated request-resource tags and
// user-authored Fuzztags are expanded once on the engine, then Content-Length
// is recalculated from the concrete wire body.
func renderMITMSubmittedRequest(packet []byte) ([]byte, error) {
	results, err := mutate.FuzzTagExec(packet, mutate.Fuzz_WithEnableDangerousTag(), mutate.Fuzz_WithResultLimit(1))
	if err != nil {
		return nil, utils.Wrap(err, "render MITM request fuzztag")
	}
	if len(results) == 0 {
		return nil, utils.Error("render MITM request fuzztag produced no packet")
	}
	rendered := []byte(results[0])
	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	if header == "" {
		return nil, utils.Error("rendered MITM request has no HTTP headers")
	}
	return lowhttp.ReplaceHTTPPacketBodyEx([]byte(header), body, false, true), nil
}

// renderMITMV2SubmittedRequest is the single execution boundary for a manual
// SendPacket action. It points file tags at completed replacement uploads,
// renders them through the existing fuzztag engine
// as WebFuzzer, and fixes Content-Length after expansion. Forward actions
// intentionally bypass this function and return the original wire request.
func renderMITMV2SubmittedRequest(
	packet []byte,
	bodyFile string,
	multipart bool,
	replacements *manualLargeRequestReplacementStore,
) ([]byte, int, error) {
	multipartPaths := map[int]string{}
	bodyPath := ""
	if replacements != nil {
		multipartPaths = replacements.multipartPaths()
		bodyPath = replacements.bodyPath()
	}
	rewritten, resourceCount, err := yakit.RewriteLargeRequestFileFuzzTags(
		packet,
		bodyFile,
		multipart,
		bodyPath,
		multipartPaths,
	)
	if err != nil {
		return nil, 0, err
	}
	rendered, err := renderMITMSubmittedRequest(rewritten)
	if err != nil {
		return nil, 0, err
	}
	return rendered, resourceCount, nil
}
