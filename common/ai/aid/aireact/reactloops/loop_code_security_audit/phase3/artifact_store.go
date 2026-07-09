package phase3

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

// findingArtifactStore serializes the final verified_vulns.json write after parallel verification.
// Per-finding results live in AuditState in memory during Phase3; only the merged file is persisted.
type findingArtifactStore struct {
	state *model.AuditState
	mu    sync.Mutex
}

func newFindingArtifactStore(state *model.AuditState) *findingArtifactStore {
	return &findingArtifactStore{state: state}
}

// MergeAll writes merged verified_vulns.json once Phase3 sub-agents finish.
func (s *findingArtifactStore) MergeAll(auditDir string) error {
	if s == nil || s.state == nil || auditDir == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		return err
	}

	verifiedFile := filepath.Join(auditDir, "verified_vulns.json")
	return s.state.PersistVerifiedVulns(verifiedFile)
}
