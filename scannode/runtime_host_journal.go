package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const runtimeHostJournalVersion = 1

type runtimeHostOperationRecord struct {
	CleanupKey  string    `json:"cleanup_key"`
	LeaseToken  string    `json:"lease_token"`
	SessionID   string    `json:"session_id"`
	ReleaseID   string    `json:"release_id"`
	ContainerID string    `json:"container_id,omitempty"`
	LastCommand string    `json:"last_command_id"`
	State       string    `json:"state"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type runtimeHostOperationJournal struct {
	Version    int                                   `json:"version"`
	Operations map[string]runtimeHostOperationRecord `json:"operations"`
}

func (e *runtimeHostExecutor) journalPath() string {
	return filepath.Join(e.baseDir, "legion", "runtime-host", "operations.json")
}

func (e *runtimeHostExecutor) loadOperationJournal() error {
	path := e.journalPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		e.operations = make(map[string]runtimeHostOperationRecord)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Runtime Host operation journal: %w", err)
	}
	journal := runtimeHostOperationJournal{}
	if err := json.Unmarshal(data, &journal); err != nil {
		return fmt.Errorf("decode Runtime Host operation journal: %w", err)
	}
	if journal.Version != runtimeHostJournalVersion {
		return fmt.Errorf("unsupported Runtime Host operation journal version %d", journal.Version)
	}
	if journal.Operations == nil {
		journal.Operations = make(map[string]runtimeHostOperationRecord)
	}
	e.operations = journal.Operations
	return nil
}

func (e *runtimeHostExecutor) saveOperationJournal() error {
	dir := filepath.Dir(e.journalPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(runtimeHostOperationJournal{
		Version: runtimeHostJournalVersion, Operations: e.operations,
	}, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, "operations-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, e.journalPath()); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func (e *runtimeHostExecutor) recoverOwnedContainers(ctx context.Context) error {
	changed := false
	for cleanupKey, record := range e.operations {
		if record.State == "stopped" {
			continue
		}
		container, found, err := e.docker.FindContainer(ctx, cleanupKey)
		if err != nil {
			// Docker may be temporarily stopped. Keep the durable intent so a
			// later command can discover and adopt the container after recovery.
			continue
		}
		if !found {
			record.State = "missing"
			record.UpdatedAt = time.Now().UTC()
			e.operations[cleanupKey] = record
			changed = true
			continue
		}
		if err := validateRuntimeContainerRecord(container, record, e.agentInstallationID); err != nil {
			return err
		}
		record.ContainerID = container.ID
		if container.Running {
			record.State = "running"
		} else {
			record.State = "stopped_container"
		}
		record.UpdatedAt = time.Now().UTC()
		e.operations[cleanupKey] = record
		changed = true
	}
	if changed {
		return e.saveOperationJournal()
	}
	return nil
}

func validateRuntimeContainerRecord(container runtimeHostContainer, record runtimeHostOperationRecord, owner string) error {
	if container.Labels[runtimeHostCleanupLabel] != record.CleanupKey ||
		container.Labels[runtimeHostLeaseLabel] != record.LeaseToken ||
		container.Labels[runtimeHostSessionLabel] != record.SessionID ||
		container.Labels[runtimeHostReleaseLabel] != record.ReleaseID ||
		container.Labels[runtimeHostOwnerLabel] != strings.TrimSpace(owner) {
		return fmt.Errorf("discovered Runtime container does not match the local operation journal")
	}
	return nil
}

func (e *runtimeHostExecutor) recordOperation(commandID string, command *runtimeHostOperationRecord) error {
	command.LastCommand = commandID
	command.UpdatedAt = time.Now().UTC()
	previous, existed := e.operations[command.CleanupKey]
	e.operations[command.CleanupKey] = *command
	if err := e.saveOperationJournal(); err != nil {
		if existed {
			e.operations[command.CleanupKey] = previous
		} else {
			delete(e.operations, command.CleanupKey)
		}
		return err
	}
	return nil
}
