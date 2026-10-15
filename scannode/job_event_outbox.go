package scannode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/node"
)

const jobOutboxMaxEvents = 10000
const jobOutboxMaxBytes = 256 << 20
const jobOutboxMaxEventBytes = 8 << 20
const jobOutboxMaxAttempts = 256

type durableJobEvent struct {
	Version   int             `json:"version"`
	Sequence  uint64          `json:"sequence"`
	NodeID    string          `json:"node_id"`
	Ref       jobExecutionRef `json:"ref"`
	EventID   string          `json:"event_id"`
	Subject   string          `json:"subject"`
	Payload   []byte          `json:"payload"`
	SHA256    string          `json:"sha256"`
	CreatedAt time.Time       `json:"created_at"`
	Terminal  bool            `json:"terminal"`
	BrokerAck bool            `json:"broker_ack"`
}

func (e durableJobEvent) filename() string {
	sum := sha256.Sum256([]byte(e.EventID))
	return fmt.Sprintf("%020d-%x.json", e.Sequence, sum[:16])
}

func (e durableJobEvent) delivery() node.PendingJobDelivery {
	return node.PendingJobDelivery{AttemptID: e.Ref.AttemptID, JobID: e.Ref.JobID, SubtaskID: e.Ref.SubtaskID, CommandID: e.Ref.CommandID, NodeSessionID: e.Ref.NodeSessionID, TerminalEventID: e.EventID}
}

type jobEventOutbox struct {
	mu       sync.Mutex
	dir      string
	entries  map[string]durableJobEvent
	bytes    int
	sequence uint64
	// Old-session replay is enabled only after the bootstrap handshake succeeds.
	recovered map[string]bool
	completed map[string]jobCompletionTombstone
	err       error
}

func (b *jobEventOutbox) admissionError() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	if len(b.entries) >= jobOutboxMaxEvents || b.bytes >= jobOutboxMaxBytes || len(b.completed) >= 100000 {
		return fmt.Errorf("durable job receipt storage is full")
	}
	return nil
}

func openJobEventOutbox(dir string) (*jobEventOutbox, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if stat, err := os.Lstat(dir); err != nil {
		return nil, err
	} else if !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 || stat.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("job outbox must be a private directory")
	}
	box := &jobEventOutbox{dir: dir, entries: make(map[string]durableJobEvent), recovered: make(map[string]bool), completed: make(map[string]jobCompletionTombstone)}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".done") {
			if err := box.loadCompletion(f); err != nil {
				return nil, err
			}
			continue
		}
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		stat, err := f.Info()
		if err != nil {
			return nil, err
		}
		if !stat.Mode().IsRegular() || stat.Mode().Perm()&0077 != 0 || stat.Size() > jobOutboxMaxEventBytes*2 {
			return nil, fmt.Errorf("unsafe job outbox entry %s", f.Name())
		}
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, err
		}
		var e durableJobEvent
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, fmt.Errorf("corrupt job outbox entry %s: %w", f.Name(), err)
		}
		if err := validateDurableJobEvent(e); err != nil {
			return nil, fmt.Errorf("corrupt job outbox entry %s: %w", f.Name(), err)
		}
		if f.Name() != e.filename() {
			return nil, fmt.Errorf("job outbox filename does not match receipt")
		}
		if _, duplicate := box.entries[e.EventID]; duplicate {
			return nil, fmt.Errorf("duplicate job outbox receipt")
		}
		box.entries[e.EventID] = e
		box.bytes += len(e.Payload)
		if e.Sequence > box.sequence {
			box.sequence = e.Sequence
		}
	}
	if len(box.entries) > jobOutboxMaxEvents || box.bytes > jobOutboxMaxBytes {
		return nil, fmt.Errorf("job outbox exceeds its capacity")
	}
	return box, nil
}

func validateDurableJobEvent(e durableJobEvent) error {
	sum := sha256.Sum256(e.Payload)
	if e.Version != 1 || e.Sequence == 0 || e.CreatedAt.IsZero() || e.NodeID == "" || e.Ref.NodeSessionID == "" || e.Ref.AttemptID == "" || e.Ref.JobID == "" || e.Ref.SubtaskID == "" || e.Ref.CommandID == "" || e.EventID == "" || len(e.Payload) == 0 || len(e.Payload) > jobOutboxMaxEventBytes || e.SHA256 != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("invalid envelope or checksum")
	}
	return validateDurableJobEnvelope(e)
}

func (b *jobEventOutbox) enqueue(e durableJobEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	if existing, ok := b.entries[e.EventID]; ok {
		if existing.NodeID != e.NodeID || existing.Ref != e.Ref || existing.Subject != e.Subject {
			return fmt.Errorf("job event identity collision")
		}
		return nil // Retry keeps the first payload, timestamps and event identity.
	}
	attempts := make(map[string]bool)
	for _, entry := range b.entries {
		attempts[entry.Ref.AttemptID] = true
	}
	if len(b.entries) >= jobOutboxMaxEvents || b.bytes+len(e.Payload) > jobOutboxMaxBytes || !attempts[e.Ref.AttemptID] && len(attempts) >= jobOutboxMaxAttempts {
		return fmt.Errorf("durable job outbox is full")
	}
	b.sequence++
	e.Sequence = b.sequence
	e.Version = 1
	sum := sha256.Sum256(e.Payload)
	e.SHA256 = hex.EncodeToString(sum[:])
	if err := validateDurableJobEvent(e); err != nil {
		return err
	}
	if err := b.write(e); err != nil {
		b.err = err
		return err
	}
	b.entries[e.EventID] = e
	b.bytes += len(e.Payload)
	return nil
}

// A successful enqueue means data and rename have both reached stable storage.
func (b *jobEventOutbox) write(e durableJobEvent) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(b.dir, ".pending-")
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
	if err := os.Rename(f.Name(), filepath.Join(b.dir, e.filename())); err != nil {
		return err
	}
	return syncJobOutboxDir(b.dir)
}

func syncJobOutboxDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func (b *jobEventOutbox) snapshot() []durableJobEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := make([]durableJobEvent, 0, len(b.entries))
	for _, e := range b.entries {
		items = append(items, e)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	return items
}

func (b *jobEventOutbox) brokerAck(eventID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entries[eventID]
	if !ok {
		return nil
	}
	if e.Terminal {
		if e.BrokerAck {
			return nil
		}
		e.BrokerAck = true
		if err := b.write(e); err != nil {
			b.err = err
			return err
		}
		b.entries[eventID] = e
		return nil
	}
	return b.removeLocked(e, false)
}

func (b *jobEventOutbox) removeLocked(e durableJobEvent, retired bool) error {
	path := filepath.Join(b.dir, e.filename())
	if retired {
		dir := filepath.Join(b.dir, "retired")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.Rename(path, filepath.Join(dir, e.filename())); err != nil {
			return err
		}
		if err := syncJobOutboxDir(dir); err != nil {
			return err
		}
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := syncJobOutboxDir(b.dir); err != nil {
		b.err = err
		return err
	}
	delete(b.entries, e.EventID)
	b.bytes -= len(e.Payload)
	return nil
}

func (b *jobEventOutbox) applyReceipts(receipts []node.JobDeliveryReceipt) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, r := range receipts {
		e, ok := b.entries[r.TerminalEventID]
		if !ok || !e.Terminal || e.Ref.AttemptID != r.AttemptID {
			continue
		}
		if r.Disposition != "projected" && r.Disposition != "retired" {
			return fmt.Errorf("invalid job delivery disposition")
		}
		if err := b.rememberCompletion(e); err != nil {
			return err
		}
		for _, entry := range b.entries {
			if entry.Ref.AttemptID == r.AttemptID {
				if err := b.removeLocked(entry, r.Disposition == "retired"); err != nil {
					return err
				}
			}
		}
		delete(b.recovered, r.AttemptID)
	}
	return nil
}

func (b *jobEventOutbox) pendingDeliveries() []node.PendingJobDelivery {
	items := []node.PendingJobDelivery{}
	for _, e := range b.snapshot() {
		if e.Terminal {
			items = append(items, e.delivery())
		}
	}
	return items
}

func (b *jobEventOutbox) acceptRecovery(session node.SessionState) error {
	if len(b.pendingDeliveries()) > 0 && !session.JobDeliveryRecoverySupported {
		return fmt.Errorf("platform does not support durable job delivery recovery")
	}
	for _, e := range b.snapshot() {
		if e.NodeID != session.NodeID {
			return fmt.Errorf("durable job outbox belongs to another node; preserve its state separately")
		}
	}
	if err := b.applyReceipts(session.JobDeliveryReceipts); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recovered = make(map[string]bool)
	for _, id := range session.AcceptedJobDeliveryAttempts {
		b.recovered[id] = true
	}
	for _, e := range b.entries {
		if e.Terminal && e.Ref.NodeSessionID != session.SessionID && !b.recovered[e.Ref.AttemptID] {
			return fmt.Errorf("platform omitted job delivery recovery disposition")
		}
	}
	return nil
}
