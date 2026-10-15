package scannode

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// A completed command must not run again after its delivery payload is removed.
// JetStream deduplication has a shorter window than scheduler outbox retries.
type jobCompletionTombstone struct {
	Ref         jobExecutionRef `json:"ref"`
	NodeID      string          `json:"node_id"`
	RetainUntil time.Time       `json:"retain_until"`
}

func completionFilename(attemptID string) string {
	sum := sha256.Sum256([]byte(attemptID))
	return fmt.Sprintf("%x.done", sum[:])
}

func (b *jobEventOutbox) loadCompletion(f os.DirEntry) error {
	stat, err := f.Info()
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Mode().Perm()&0077 != 0 || stat.Size() > 8192 {
		return fmt.Errorf("unsafe completion receipt")
	}
	raw, err := os.ReadFile(filepath.Join(b.dir, f.Name()))
	if err != nil {
		return err
	}
	var c jobCompletionTombstone
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	if c.NodeID == "" || c.Ref.AttemptID == "" || c.Ref.CommandID == "" || c.Ref.JobID == "" || c.Ref.SubtaskID == "" || f.Name() != completionFilename(c.Ref.AttemptID) {
		return fmt.Errorf("corrupt completion receipt")
	}
	if !c.RetainUntil.After(time.Now()) {
		return os.Remove(filepath.Join(b.dir, f.Name()))
	}
	b.completed[c.Ref.AttemptID] = c
	return nil
}

func (b *jobEventOutbox) rememberCompletion(e durableJobEvent) error {
	if _, ok := b.completed[e.Ref.AttemptID]; ok {
		return nil
	}
	for id, c := range b.completed {
		if !c.RetainUntil.After(time.Now()) {
			if err := os.Remove(filepath.Join(b.dir, completionFilename(id))); err != nil && !os.IsNotExist(err) {
				return err
			}
			delete(b.completed, id)
		}
	}
	if len(b.completed) >= 100000 {
		return fmt.Errorf("job completion receipt capacity reached")
	}
	c := jobCompletionTombstone{Ref: e.Ref, NodeID: e.NodeID, RetainUntil: time.Now().UTC().Add(24 * time.Hour)}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(b.dir, ".completion-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), filepath.Join(b.dir, completionFilename(e.Ref.AttemptID))); err != nil {
		return err
	}
	if err := syncJobOutboxDir(b.dir); err != nil {
		return err
	}
	b.completed[e.Ref.AttemptID] = c
	return nil
}

func (b *jobEventOutbox) completedCommand(ref jobExecutionRef) (bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	match := func(previous jobExecutionRef) bool {
		return previous.CommandID == ref.CommandID && previous.JobID == ref.JobID && previous.SubtaskID == ref.SubtaskID
	}
	if c, ok := b.completed[ref.AttemptID]; ok && c.RetainUntil.After(time.Now()) {
		return true, match(c.Ref)
	}
	for _, e := range b.entries {
		if e.Terminal && e.Ref.AttemptID == ref.AttemptID {
			return true, match(e.Ref)
		}
	}
	return false, false
}
