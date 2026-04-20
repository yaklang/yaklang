package node

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
)

const agentInstallationIDFileName = "agent-installation-id"

func identityBaseDir(baseDir string) string {
	trimmed := strings.TrimSpace(baseDir)
	if trimmed != "" {
		return trimmed
	}
	return consts.GetDefaultYakitBaseDir()
}

func resolveAgentInstallationID(baseDir string, explicit string) (string, error) {
	normalized := normalizeAgentInstallationID(explicit)
	if normalized != "" {
		return normalized, nil
	}

	path := filepath.Join(
		identityBaseDir(baseDir),
		"legion",
		"identity",
		agentInstallationIDFileName,
	)
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		existing := normalizeAgentInstallationID(string(raw))
		if existing == "" {
			return "", fmt.Errorf("agent installation id file is empty: %s", path)
		}
		return existing, nil
	case !os.IsNotExist(err):
		return "", fmt.Errorf("read agent installation id: %w", err)
	}

	generated := normalizeAgentInstallationID(uuid.NewString())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create identity directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(generated+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("persist agent installation id: %w", err)
	}
	return generated, nil
}

func normalizeAgentInstallationID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeHostIdentity(input HostIdentity) HostIdentity {
	return HostIdentity{
		MachineID:  normalizeAgentInstallationID(input.MachineID),
		SystemUUID: normalizeAgentInstallationID(input.SystemUUID),
		InstanceID: normalizeAgentInstallationID(input.InstanceID),
	}
}
