package phase2

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"github.com/yaklang/yaklang/common/log"
)

// categoryArtifactStore serializes per-category artifact writes during parallel sub-agent scans.
// Phase2 writes scan_obs_{category}.json per category and merges scan_findings.json once at the end.
type categoryArtifactStore struct {
	state *model.AuditState
	mu    sync.Mutex
	locks sync.Map // categoryID -> *sync.Mutex
}

func newCategoryArtifactStore(state *model.AuditState) *categoryArtifactStore {
	return &categoryArtifactStore{state: state}
}

func (s *categoryArtifactStore) lockCategory(categoryID string) *sync.Mutex {
	if v, ok := s.locks.Load(categoryID); ok {
		return v.(*sync.Mutex)
	}
	m := &sync.Mutex{}
	actual, _ := s.locks.LoadOrStore(categoryID, m)
	return actual.(*sync.Mutex)
}

func categoryObservationPath(auditDir, categoryID string) string {
	return filepath.Join(auditDir, util.CategoryObservationFilename(categoryID))
}

// PersistCategoryObservation writes scan_obs_{category}.json for that category only.
func (s *categoryArtifactStore) PersistCategoryObservation(auditDir, categoryID string, obs *model.ScanObservation) error {
	if s == nil || obs == nil || categoryID == "" || auditDir == "" {
		return nil
	}
	s.lockCategory(categoryID).Lock()
	defer s.lockCategory(categoryID).Unlock()

	data, err := json.MarshalIndent(obs, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(categoryObservationPath(auditDir, categoryID), data, 0o644)
}

// MergeAll writes merged scan_findings.json once Phase2 sub-agents finish.
func (s *categoryArtifactStore) MergeAll(auditDir string) error {
	if s == nil || s.state == nil || auditDir == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		return err
	}

	findingsFile := filepath.Join(auditDir, "scan_findings.json")
	return s.state.PersistFindings(findingsFile)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func persistCategoryObservation(store *categoryArtifactStore, auditDir, categoryID string, obs *model.ScanObservation) {
	if store == nil || obs == nil || categoryID == "" || auditDir == "" {
		return
	}
	if err := store.PersistCategoryObservation(auditDir, categoryID, obs); err != nil {
		log.Warnf("[CodeAudit/Phase2] Persist category observation failed: %v", err)
	}
}
