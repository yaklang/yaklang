package scannode

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const (
	maxServerFocusEntryBytes    = 512 * 1024
	maxServerFocusSidekickBytes = 512 * 1024
	maxServerFocusBundleBytes   = 1024 * 1024
	maxServerFocusSidekicks     = 32
	focusReleaseSeparator       = "\x00"
)

var (
	serverFocusRuntimeNamePattern = regexp.MustCompile(`^legion_release_[a-z0-9_]+_[0-9a-f]{12}$`)
	serverFocusRegistryMu         sync.Mutex
	serverFocusRegistry           = map[string]string{}
)

func registerContextFocusRelease(release *aiv1.ContextFocusRelease) (string, error) {
	if release == nil {
		return "", nil
	}
	validated, err := validateContextFocusRelease(release)
	if err != nil {
		return "", err
	}

	serverFocusRegistryMu.Lock()
	defer serverFocusRegistryMu.Unlock()
	if checksum, ok := serverFocusRegistry[validated.runtimeName]; ok {
		if checksum != validated.sha256 {
			return "", fmt.Errorf("server focus runtime %q is already registered with different content", validated.runtimeName)
		}
		return validated.runtimeName, nil
	}

	bundle := &reactloops.FocusModeBundle{
		Name:      validated.runtimeName,
		FixedName: true,
		EntryFile: validated.entryFile,
		EntryCode: validated.entryCode,
	}
	for _, sidekick := range validated.sidekicks {
		bundle.Sidekicks = append(bundle.Sidekicks, reactloops.FocusModeSidekick{
			Path:    sidekick.Path,
			Content: sidekick.Content,
		})
	}
	if err := reactloops.RegisterYakFocusModeFromBundle(bundle); err != nil {
		return "", fmt.Errorf("register server focus release %q: %w", validated.releaseID, err)
	}
	serverFocusRegistry[validated.runtimeName] = validated.sha256
	return validated.runtimeName, nil
}

type validatedFocusRelease struct {
	releaseID   string
	runtimeName string
	entryFile   string
	entryCode   string
	sha256      string
	sidekicks   []reactloops.FocusModeSidekick
}

func validateContextFocusRelease(release *aiv1.ContextFocusRelease) (validatedFocusRelease, error) {
	validated := validatedFocusRelease{
		releaseID:   strings.TrimSpace(release.GetReleaseId()),
		runtimeName: strings.TrimSpace(release.GetRuntimeName()),
		entryFile:   strings.TrimSpace(release.GetEntryFile()),
		entryCode:   release.GetEntryCode(),
		sha256:      strings.ToLower(strings.TrimSpace(release.GetSha256())),
	}
	focusName := strings.TrimSpace(release.GetFocusName())
	version := strings.TrimSpace(release.GetVersion())
	if validated.releaseID == "" || focusName == "" || version == "" {
		return validatedFocusRelease{}, fmt.Errorf("server focus release identity is incomplete")
	}
	if !serverFocusRuntimeNamePattern.MatchString(validated.runtimeName) {
		return validatedFocusRelease{}, fmt.Errorf("server focus runtime name %q is invalid", validated.runtimeName)
	}
	if path.Base(validated.entryFile) != validated.entryFile || !strings.HasSuffix(validated.entryFile, reactloops.FocusModeFileSuffix) {
		return validatedFocusRelease{}, fmt.Errorf("server focus entry file %q is invalid", validated.entryFile)
	}
	if len(validated.entryCode) == 0 || len(validated.entryCode) > maxServerFocusEntryBytes {
		return validatedFocusRelease{}, fmt.Errorf("server focus entry code size %d is invalid", len(validated.entryCode))
	}
	if len(release.GetSidekicks()) > maxServerFocusSidekicks {
		return validatedFocusRelease{}, fmt.Errorf("server focus sidekick count %d exceeds limit", len(release.GetSidekicks()))
	}

	totalBytes := len(validated.entryCode)
	seenPaths := make(map[string]struct{}, len(release.GetSidekicks()))
	for _, sidekick := range release.GetSidekicks() {
		if sidekick == nil {
			return validatedFocusRelease{}, fmt.Errorf("server focus sidekick is nil")
		}
		sidekickPath := strings.TrimSpace(sidekick.GetPath())
		if path.Base(sidekickPath) != sidekickPath || !strings.HasSuffix(sidekickPath, reactloops.FocusModeYakFileSuffix) || strings.HasSuffix(sidekickPath, reactloops.FocusModeFileSuffix) {
			return validatedFocusRelease{}, fmt.Errorf("server focus sidekick path %q is invalid", sidekickPath)
		}
		if _, duplicate := seenPaths[sidekickPath]; duplicate {
			return validatedFocusRelease{}, fmt.Errorf("server focus sidekick path %q is duplicated", sidekickPath)
		}
		seenPaths[sidekickPath] = struct{}{}
		if len(sidekick.GetContent()) > maxServerFocusSidekickBytes {
			return validatedFocusRelease{}, fmt.Errorf("server focus sidekick %q exceeds size limit", sidekickPath)
		}
		totalBytes += len(sidekick.GetContent())
		validated.sidekicks = append(validated.sidekicks, reactloops.FocusModeSidekick{
			Path:    sidekickPath,
			Content: sidekick.GetContent(),
		})
	}
	if totalBytes > maxServerFocusBundleBytes {
		return validatedFocusRelease{}, fmt.Errorf("server focus bundle size %d exceeds limit", totalBytes)
	}
	sort.Slice(validated.sidekicks, func(i, j int) bool {
		return validated.sidekicks[i].Path < validated.sidekicks[j].Path
	})
	if len(validated.sha256) != sha256.Size*2 {
		return validatedFocusRelease{}, fmt.Errorf("server focus release checksum is invalid")
	}
	if !strings.HasSuffix(validated.runtimeName, "_"+validated.sha256[:12]) {
		return validatedFocusRelease{}, fmt.Errorf("server focus runtime name does not match release checksum")
	}
	if validated.releaseID != focusName+"@"+version+"+"+validated.sha256[:12] {
		return validatedFocusRelease{}, fmt.Errorf("server focus release id does not match release checksum")
	}
	if actual := contextFocusReleaseChecksum(focusName, version, validated.entryFile, validated.entryCode, validated.sidekicks); actual != validated.sha256 {
		return validatedFocusRelease{}, fmt.Errorf("server focus release checksum mismatch")
	}
	return validated, nil
}

func contextFocusReleaseChecksum(focusName, version, entryFile, entryCode string, sidekicks []reactloops.FocusModeSidekick) string {
	hash := sha256.New()
	for _, value := range []string{focusName, version, entryFile, entryCode} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte(focusReleaseSeparator))
	}
	for _, sidekick := range sidekicks {
		_, _ = hash.Write([]byte(sidekick.Path))
		_, _ = hash.Write([]byte(focusReleaseSeparator))
		_, _ = hash.Write([]byte(sidekick.Content))
		_, _ = hash.Write([]byte(focusReleaseSeparator))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func pinnedFocusReleaseID(runtimeOptions []byte) string {
	options, err := decodeYakRuntimeOptions(runtimeOptions)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(options.FocusReleaseID)
}
