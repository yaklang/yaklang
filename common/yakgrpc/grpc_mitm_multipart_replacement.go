package yakgrpc

import (
	"os"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

type manualMultipartReplacementUpload struct {
	file *os.File
	path string
}

// manualMultipartReplacementStore owns the bounded chunks uploaded while a
// request is waiting in manual hijack. Completed files remain engine-local and
// are removed as soon as the hijack task finishes.
type manualMultipartReplacementStore struct {
	active    map[int]*manualMultipartReplacementUpload
	completed map[int]string
}

func newManualMultipartReplacementStore() *manualMultipartReplacementStore {
	return &manualMultipartReplacementStore{
		active:    make(map[int]*manualMultipartReplacementUpload),
		completed: make(map[int]string),
	}
}

func (s *manualMultipartReplacementStore) consume(
	partIndex int,
	data []byte,
	start bool,
	eof bool,
	cancel bool,
) error {
	if partIndex < 0 {
		return utils.Errorf("invalid multipart part index %d", partIndex)
	}
	if cancel {
		s.discard(partIndex)
		return nil
	}
	if start {
		s.discard(partIndex)
		tempDir := consts.GetDefaultYakitBaseTempDir()
		if err := os.MkdirAll(tempDir, 0o755); err != nil {
			return err
		}
		f, err := os.CreateTemp(tempDir, "mitm-multipart-replacement-*")
		if err != nil {
			return err
		}
		s.active[partIndex] = &manualMultipartReplacementUpload{file: f, path: f.Name()}
	}
	upload := s.active[partIndex]
	if upload == nil {
		return utils.Errorf("multipart replacement part %d was not started", partIndex)
	}
	if len(data) > 0 {
		if _, err := upload.file.Write(data); err != nil {
			s.discard(partIndex)
			return err
		}
	}
	if eof {
		if err := upload.file.Close(); err != nil {
			s.discard(partIndex)
			return err
		}
		upload.file = nil
		delete(s.active, partIndex)
		s.completed[partIndex] = upload.path
	}
	return nil
}

func (s *manualMultipartReplacementStore) hasCompleted() bool {
	return len(s.completed) > 0
}

func (s *manualMultipartReplacementStore) hasActive() bool {
	return len(s.active) > 0
}

func (s *manualMultipartReplacementStore) paths() map[int]string {
	out := make(map[int]string, len(s.completed))
	for partIndex, path := range s.completed {
		out[partIndex] = path
	}
	return out
}

func (s *manualMultipartReplacementStore) discard(partIndex int) {
	if upload := s.active[partIndex]; upload != nil {
		if upload.file != nil {
			_ = upload.file.Close()
		}
		_ = os.Remove(upload.path)
		delete(s.active, partIndex)
	}
	if path := s.completed[partIndex]; path != "" {
		_ = os.Remove(path)
		delete(s.completed, partIndex)
	}
}

func (s *manualMultipartReplacementStore) close() {
	for partIndex := range s.active {
		s.discard(partIndex)
	}
	for partIndex := range s.completed {
		s.discard(partIndex)
	}
}
