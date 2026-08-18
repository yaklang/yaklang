package scannode

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
)

const (
	legionCodeWorkspaceKindGit             = "git"
	legionCodeWorkspaceKindUploadedArchive = "uploaded_archive"
	legionCodeWorkspaceArchiveLocator      = "managed-source"

	legionCodeWorkspaceMaxFiles       = 20_000
	legionCodeWorkspaceMaxTotalBytes  = int64(256 * 1024 * 1024)
	legionCodeWorkspaceMaxFileBytes   = int64(1024 * 1024)
	legionCodeWorkspaceMaxReadBytes   = int64(256 * 1024)
	legionCodeWorkspaceMaxSearchItems = 200
	legionCodeWorkspaceMaxPathDepth   = 32
)

type legionCodeWorkspaceAuth struct {
	Kind       string `json:"kind,omitempty"`
	Username   string `json:"username,omitempty"`
	UserName   string `json:"user_name,omitempty"`
	Password   string `json:"password,omitempty"`
	Token      string `json:"token,omitempty"`
	KeyContent string `json:"key_content,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

type legionCodeWorkspaceProxy struct {
	URL      string `json:"url,omitempty"`
	Username string `json:"username,omitempty"`
	UserName string `json:"user_name,omitempty"`
	Password string `json:"password,omitempty"`
}

type legionCodeWorkspaceSpec struct {
	WorkspaceID      string                    `json:"workspace_id"`
	Kind             string                    `json:"kind"`
	Locator          string                    `json:"locator"`
	Branch           string                    `json:"branch,omitempty"`
	Subpath          string                    `json:"subpath,omitempty"`
	PayloadID        string                    `json:"payload_id,omitempty"`
	ExpectedRevision string                    `json:"expected_revision,omitempty"`
	ExpectedSHA256   string                    `json:"expected_sha256,omitempty"`
	ReadOnly         bool                      `json:"read_only"`
	MaxFiles         int                       `json:"max_files"`
	MaxTotalBytes    int64                     `json:"max_total_bytes"`
	MaxFileBytes     int64                     `json:"max_file_bytes"`
	MaxReadBytes     int64                     `json:"max_read_bytes"`
	MaxSearchResults int                       `json:"max_search_results"`
	Auth             *legionCodeWorkspaceAuth  `json:"auth,omitempty"`
	Proxy            *legionCodeWorkspaceProxy `json:"proxy,omitempty"`
}

type legionCodeWorkspaceMaterializeOptions struct {
	HTTPClient          *http.Client
	PlatformAPIBaseURL  string
	NodeSessionID       string
	PlatformBearerToken string
}

type legionCodeWorkspaceRuntime struct {
	spec           legionCodeWorkspaceSpec
	root           string
	lockedRevision string
	sha256         string
	files          int
	bytes          int64
	cleanup        func() error
	cleanupOnce    sync.Once
	cleanupErr     error
}

type legionCodeWorkspaceRuntimeHandle struct {
	handle    aiSessionRuntimeHandle
	workspace *legionCodeWorkspaceRuntime
}

func (h *legionCodeWorkspaceRuntimeHandle) SendInput(ctx context.Context, input aiSessionInput) error {
	return h.handle.SendInput(ctx, input)
}

func (h *legionCodeWorkspaceRuntimeHandle) AppendContext(ctx context.Context, update aiSessionContextUpdate) error {
	return h.handle.AppendContext(ctx, update)
}

func (h *legionCodeWorkspaceRuntimeHandle) Cancel(reason string) {
	h.handle.Cancel(reason)
	_ = h.workspace.Cleanup()
}

func (h *legionCodeWorkspaceRuntimeHandle) Close(reason string) {
	h.handle.Close(reason)
	_ = h.workspace.Cleanup()
}

func (h *legionCodeWorkspaceRuntimeHandle) activeTurnID() string {
	if provider, ok := h.handle.(aiSessionRuntimeTurnRefProvider); ok {
		return provider.activeTurnID()
	}
	return ""
}

var (
	prepareLegionCodeGitWorkspace   = ssagitworkdir.Prepare
	cloneLegionCodeGitWorkspace     = yakgit.Clone
	legionCodeWorkspaceIDPattern    = regexp.MustCompile(`^aicw_[0-9a-f]{32}$`)
	legionCodeGitSCPUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

func prepareLegionCodeWorkspace(
	ctx context.Context,
	runtimeOptionSnapshotJSON []byte,
	options legionCodeWorkspaceMaterializeOptions,
) (*legionCodeWorkspaceRuntime, []byte, error) {
	runtimeOptions, err := decodeYakRuntimeOptions(runtimeOptionSnapshotJSON, true)
	if err != nil {
		return nil, nil, fmt.Errorf("decode code workspace runtime options: %w", err)
	}
	if runtimeOptions.SourceWorkspace == nil {
		return nil, cloneBytes(runtimeOptionSnapshotJSON), nil
	}
	spec := *runtimeOptions.SourceWorkspace
	if spec.Auth != nil {
		auth := *spec.Auth
		spec.Auth = &auth
	}
	if spec.Proxy != nil {
		proxy := *spec.Proxy
		spec.Proxy = &proxy
	}
	if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
		return nil, nil, err
	}

	workspace, err := materializeLegionCodeWorkspace(ctx, spec, options)
	if err != nil {
		return nil, nil, err
	}
	publicSpec := spec
	publicSpec.Auth = nil
	publicSpec.Proxy = nil
	runtimeOptions.SourceWorkspace = &publicSpec
	sanitized, err := json.Marshal(runtimeOptions)
	if err != nil {
		_ = workspace.Cleanup()
		return nil, nil, fmt.Errorf("encode public code workspace runtime options: %w", err)
	}
	return workspace, sanitized, nil
}

func normalizeLegionCodeWorkspaceSpec(spec *legionCodeWorkspaceSpec) error {
	if spec == nil {
		return fmt.Errorf("source_workspace is required")
	}
	spec.WorkspaceID = strings.TrimSpace(spec.WorkspaceID)
	spec.Kind = strings.ToLower(strings.TrimSpace(spec.Kind))
	spec.Locator = strings.TrimSpace(spec.Locator)
	spec.Branch = strings.TrimSpace(spec.Branch)
	spec.Subpath = strings.TrimSpace(spec.Subpath)
	spec.PayloadID = strings.TrimSpace(spec.PayloadID)
	spec.ExpectedRevision = strings.TrimSpace(spec.ExpectedRevision)
	spec.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(spec.ExpectedSHA256))
	switch {
	case spec.WorkspaceID == "":
		return fmt.Errorf("source_workspace workspace_id is required")
	case !legionCodeWorkspaceIDPattern.MatchString(spec.WorkspaceID):
		return fmt.Errorf("source_workspace workspace_id is invalid")
	case spec.Kind != legionCodeWorkspaceKindGit && spec.Kind != legionCodeWorkspaceKindUploadedArchive:
		return fmt.Errorf("source_workspace kind %q is unsupported", spec.Kind)
	case spec.Locator == "":
		return fmt.Errorf("source_workspace locator is required")
	case !spec.ReadOnly:
		return fmt.Errorf("source_workspace must be read_only")
	case spec.ExpectedSHA256 != "" && (len(spec.ExpectedSHA256) != sha256.Size*2 || !isLowerHex(spec.ExpectedSHA256)):
		return fmt.Errorf("source_workspace expected_sha256 is invalid")
	case spec.Kind == legionCodeWorkspaceKindUploadedArchive && !managedSourcePayloadIDPattern.MatchString(spec.PayloadID):
		return fmt.Errorf("source_workspace payload_id is invalid")
	}
	if err := normalizeLegionCodeWorkspaceLocator(spec); err != nil {
		return err
	}
	if spec.Subpath != "" {
		if _, err := cleanLegionCodeRelativePath(spec.Subpath, false); err != nil {
			return fmt.Errorf("source_workspace subpath: %w", err)
		}
	}
	if spec.MaxFiles <= 0 || spec.MaxFiles > legionCodeWorkspaceMaxFiles {
		return fmt.Errorf("source_workspace max_files must be between 1 and %d", legionCodeWorkspaceMaxFiles)
	}
	if spec.MaxTotalBytes <= 0 || spec.MaxTotalBytes > legionCodeWorkspaceMaxTotalBytes {
		return fmt.Errorf("source_workspace max_total_bytes must be between 1 and %d", legionCodeWorkspaceMaxTotalBytes)
	}
	if spec.MaxFileBytes <= 0 || spec.MaxFileBytes > legionCodeWorkspaceMaxFileBytes {
		return fmt.Errorf("source_workspace max_file_bytes must be between 1 and %d", legionCodeWorkspaceMaxFileBytes)
	}
	if spec.MaxReadBytes <= 0 || spec.MaxReadBytes > legionCodeWorkspaceMaxReadBytes {
		return fmt.Errorf("source_workspace max_read_bytes must be between 1 and %d", legionCodeWorkspaceMaxReadBytes)
	}
	if spec.MaxSearchResults <= 0 || spec.MaxSearchResults > legionCodeWorkspaceMaxSearchItems {
		return fmt.Errorf("source_workspace max_search_results must be between 1 and %d", legionCodeWorkspaceMaxSearchItems)
	}
	return nil
}

func normalizeLegionCodeWorkspaceLocator(spec *legionCodeWorkspaceSpec) error {
	if spec == nil {
		return fmt.Errorf("source_workspace is required")
	}
	switch spec.Kind {
	case legionCodeWorkspaceKindGit:
		locator, err := normalizeLegionCodeGitLocator(spec.Locator)
		if err != nil {
			return err
		}
		spec.Locator = locator
		return nil
	case legionCodeWorkspaceKindUploadedArchive:
		if spec.Locator != legionCodeWorkspaceArchiveLocator {
			return fmt.Errorf("source_workspace uploaded_archive locator must be %q", legionCodeWorkspaceArchiveLocator)
		}
		return nil
	default:
		return fmt.Errorf("source_workspace kind %q is unsupported", spec.Kind)
	}
}

func normalizeLegionCodeGitLocator(locator string) (string, error) {
	if locator == "" {
		return "", fmt.Errorf("source_workspace locator is required")
	}
	for _, char := range locator {
		if char < 0x20 || char == 0x7f {
			return "", fmt.Errorf("source_workspace git locator is invalid")
		}
	}
	// A query or fragment is never needed to identify the immutable source.
	// Reject it for URL and SCP-like locators so secrets cannot be
	// reflected into public RuntimeOptions or source.workspace.info.
	if strings.ContainsAny(locator, "?#") {
		return "", fmt.Errorf("source_workspace git locator must not contain a query or fragment")
	}
	if !strings.Contains(locator, "://") {
		return normalizeLegionCodeGitSCPLocator(locator)
	}
	parsed, err := url.Parse(locator)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Opaque != "" || parsed.Host == "" {
		return "", fmt.Errorf("source_workspace git locator is invalid")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	switch parsed.Scheme {
	case "http", "https", "ssh":
	default:
		return "", fmt.Errorf("source_workspace git locator scheme is unsupported")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("source_workspace git locator must not contain URL userinfo")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return "", fmt.Errorf("source_workspace git locator must not contain a query or fragment")
	}
	return parsed.String(), nil
}

func normalizeLegionCodeGitSCPLocator(locator string) (string, error) {
	at := strings.IndexByte(locator, '@')
	if at <= 0 || at == len(locator)-1 {
		return "", fmt.Errorf("source_workspace git locator must be a remote http, https, ssh, or SCP-like locator")
	}
	username := locator[:at]
	remainder := locator[at+1:]
	colon := strings.IndexByte(remainder, ':')
	if colon <= 0 || colon == len(remainder)-1 {
		return "", fmt.Errorf("source_workspace git SCP-like locator is invalid")
	}
	host := remainder[:colon]
	repositoryPath := remainder[colon+1:]
	if !legionCodeGitSCPUsernamePattern.MatchString(username) ||
		strings.Contains(repositoryPath, "\\") || strings.ContainsAny(repositoryPath, " \t\r\n") {
		return "", fmt.Errorf("source_workspace git SCP-like locator is invalid")
	}
	hostURL, err := url.Parse("ssh://" + host)
	if err != nil || hostURL == nil || hostURL.Hostname() == "" || hostURL.User != nil || hostURL.Port() != "" || hostURL.Path != "" {
		return "", fmt.Errorf("source_workspace git SCP-like locator is invalid")
	}
	return username + "@" + strings.ToLower(hostURL.Host) + ":" + repositoryPath, nil
}

func materializeLegionCodeWorkspace(
	ctx context.Context,
	spec legionCodeWorkspaceSpec,
	options legionCodeWorkspaceMaterializeOptions,
) (*legionCodeWorkspaceRuntime, error) {
	switch spec.Kind {
	case legionCodeWorkspaceKindGit:
		return materializeLegionCodeGitWorkspace(ctx, spec)
	case legionCodeWorkspaceKindUploadedArchive:
		return materializeLegionCodeArchiveWorkspace(ctx, spec, options)
	default:
		return nil, fmt.Errorf("source_workspace kind %q is unsupported", spec.Kind)
	}
}

func materializeLegionCodeGitWorkspace(
	ctx context.Context,
	spec legionCodeWorkspaceSpec,
) (_ *legionCodeWorkspaceRuntime, finalErr error) {
	locator, err := normalizeLegionCodeGitLocator(strings.TrimSpace(spec.Locator))
	if err != nil {
		return nil, err
	}
	spec.Locator = locator
	local, cleanup, err := prepareLegionCodeGitWorkspace(ctx, os.Getpid())
	if err != nil {
		return nil, fmt.Errorf("prepare source_workspace git workdir: %w", err)
	}
	defer func() {
		if finalErr != nil {
			finalErr = errors.Join(finalErr, cleanup())
		}
	}()

	opts := []yakgit.Option{
		yakgit.WithContext(ctx),
		yakgit.WithRecuriveSubmodule(false),
	}
	if spec.Branch != "" {
		opts = append(opts, yakgit.WithBranch(spec.Branch))
	}
	if spec.Proxy != nil && strings.TrimSpace(spec.Proxy.URL) != "" {
		if err := validateLegionCodeProxyURL(spec.Proxy.URL); err != nil {
			return nil, err
		}
		proxyUsername := strings.TrimSpace(spec.Proxy.Username)
		if proxyUsername == "" {
			proxyUsername = strings.TrimSpace(spec.Proxy.UserName)
		}
		opts = append(opts, yakgit.WithProxy(spec.Proxy.URL, proxyUsername, spec.Proxy.Password))
	}
	if spec.Auth != nil {
		username := strings.TrimSpace(spec.Auth.Username)
		if username == "" {
			username = strings.TrimSpace(spec.Auth.UserName)
		}
		switch strings.ToLower(strings.TrimSpace(spec.Auth.Kind)) {
		case "", "none":
		case "password":
			opts = append(opts, yakgit.WithUsernamePassword(username, spec.Auth.Password))
		case "token":
			if username == "" {
				username = "oauth2"
			}
			token := strings.TrimSpace(spec.Auth.Token)
			if token == "" {
				token = spec.Auth.Password
			}
			opts = append(opts, yakgit.WithUsernamePassword(username, token))
		case "ssh_key":
			if strings.TrimSpace(spec.Auth.KeyContent) == "" {
				return nil, fmt.Errorf("source_workspace git ssh_key requires key_content")
			}
			if username == "" {
				username = "git"
			}
			opts = append(opts, yakgit.WithPrivateKeyContent(username, spec.Auth.KeyContent, spec.Auth.Passphrase))
		default:
			return nil, fmt.Errorf("source_workspace git auth kind %q is unsupported", spec.Auth.Kind)
		}
	}
	if err := cloneLegionCodeGitWorkspace(spec.Locator, local, opts...); err != nil {
		return nil, ssagitworkdir.WrapCloneError(ctx, local, err)
	}
	lockedRevision := strings.ToLower(strings.TrimSpace(yakgit.GetHeadHash(local)))
	if lockedRevision == "" {
		return nil, fmt.Errorf("source_workspace git clone has no HEAD revision")
	}
	if spec.ExpectedRevision != "" && !strings.EqualFold(spec.ExpectedRevision, lockedRevision) {
		return nil, fmt.Errorf(
			"source_workspace revision mismatch: expected=%s actual=%s",
			spec.ExpectedRevision,
			lockedRevision,
		)
	}
	root, err := legionCodeWorkspaceSubpath(local, spec.Subpath)
	if err != nil {
		return nil, err
	}
	files, bytesCount, digest, err := inspectLegionCodeWorkspace(root, spec)
	if err != nil {
		return nil, err
	}
	if spec.ExpectedSHA256 != "" && spec.ExpectedSHA256 != digest {
		return nil, fmt.Errorf("source_workspace sha256 mismatch: expected=%s actual=%s", spec.ExpectedSHA256, digest)
	}
	return &legionCodeWorkspaceRuntime{
		spec:           publicLegionCodeWorkspaceSpec(spec),
		root:           root,
		lockedRevision: lockedRevision,
		sha256:         digest,
		files:          files,
		bytes:          bytesCount,
		cleanup:        cleanup,
	}, nil
}

func materializeLegionCodeArchiveWorkspace(
	ctx context.Context,
	spec legionCodeWorkspaceSpec,
	options legionCodeWorkspaceMaterializeOptions,
) (_ *legionCodeWorkspaceRuntime, finalErr error) {
	archivePath, archiveSHA256, err := downloadManagedSourcePayloadBounded(
		ctx,
		options.HTTPClient,
		options.PlatformAPIBaseURL,
		options.NodeSessionID,
		options.PlatformBearerToken,
		spec.PayloadID,
		spec.MaxTotalBytes,
		spec.ExpectedSHA256,
	)
	if err != nil {
		return nil, fmt.Errorf("materialize source_workspace archive: %w", err)
	}
	defer os.Remove(archivePath)

	workspaceRoot, err := os.MkdirTemp("", "legion-code-workspace-*")
	if err != nil {
		return nil, fmt.Errorf("create source_workspace archive directory: %w", err)
	}
	cleanup := func() error { return os.RemoveAll(workspaceRoot) }
	defer func() {
		if finalErr != nil {
			finalErr = errors.Join(finalErr, cleanup())
		}
	}()
	if err := extractLegionCodeWorkspaceZip(archivePath, workspaceRoot, spec); err != nil {
		return nil, err
	}
	root, err := legionCodeWorkspaceSubpath(workspaceRoot, spec.Subpath)
	if err != nil {
		return nil, err
	}
	files, bytesCount, _, err := inspectLegionCodeWorkspace(root, spec)
	if err != nil {
		return nil, err
	}
	return &legionCodeWorkspaceRuntime{
		spec:           publicLegionCodeWorkspaceSpec(spec),
		root:           root,
		lockedRevision: strings.TrimSpace(spec.ExpectedRevision),
		sha256:         archiveSHA256,
		files:          files,
		bytes:          bytesCount,
		cleanup:        cleanup,
	}, nil
}

func extractLegionCodeWorkspaceZip(
	archivePath string,
	destination string,
	spec legionCodeWorkspaceSpec,
) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open source_workspace zip: %w", err)
	}
	defer reader.Close()

	seen := make(map[string]struct{}, len(reader.File))
	var files int
	var total int64
	for _, entry := range reader.File {
		if entry == nil {
			return fmt.Errorf("source_workspace zip contains an invalid entry")
		}
		name, err := cleanLegionCodeArchivePath(entry.Name)
		if err != nil {
			return err
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("source_workspace zip contains duplicate path %q", name)
		}
		seen[name] = struct{}{}
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			return fmt.Errorf("source_workspace zip entry %q has unsupported file type %s", name, mode.Type())
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if mode.IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return fmt.Errorf("create source_workspace directory %q: %w", name, err)
			}
			continue
		}
		files++
		if files > spec.MaxFiles {
			return fmt.Errorf("source_workspace file count exceeds %d", spec.MaxFiles)
		}
		declared := int64(entry.UncompressedSize64)
		if declared > spec.MaxFileBytes {
			return fmt.Errorf("source_workspace file %q exceeds %d bytes", name, spec.MaxFileBytes)
		}
		if declared > spec.MaxTotalBytes-total {
			return fmt.Errorf("source_workspace total bytes exceed %d", spec.MaxTotalBytes)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("create source_workspace parent for %q: %w", name, err)
		}
		input, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open source_workspace zip entry %q: %w", name, err)
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
		if err != nil {
			input.Close()
			return fmt.Errorf("create source_workspace file %q: %w", name, err)
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, spec.MaxFileBytes+1))
		closeErr := errors.Join(output.Close(), input.Close())
		if copyErr != nil || closeErr != nil {
			return fmt.Errorf("extract source_workspace file %q: %w", name, errors.Join(copyErr, closeErr))
		}
		if written > spec.MaxFileBytes {
			return fmt.Errorf("source_workspace file %q exceeds %d bytes", name, spec.MaxFileBytes)
		}
		total += written
		if total > spec.MaxTotalBytes {
			return fmt.Errorf("source_workspace total bytes exceed %d", spec.MaxTotalBytes)
		}
	}
	return nil
}

func inspectLegionCodeWorkspace(
	root string,
	spec legionCodeWorkspaceSpec,
) (files int, bytesCount int64, digest string, finalErr error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		if rel == "." || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		if files > spec.MaxFiles {
			return fmt.Errorf("source_workspace file count exceeds %d", spec.MaxFiles)
		}
		_, _ = io.WriteString(hash, rel+"\x00")
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(current)
			if err != nil {
				return err
			}
			_, _ = io.WriteString(hash, "symlink\x00"+target+"\x00")
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source_workspace path %q has unsupported file type %s", rel, info.Mode().Type())
		}
		if info.Size() > spec.MaxFileBytes {
			return fmt.Errorf("source_workspace file %q exceeds %d bytes", rel, spec.MaxFileBytes)
		}
		bytesCount += info.Size()
		if bytesCount > spec.MaxTotalBytes {
			return fmt.Errorf("source_workspace total bytes exceed %d", spec.MaxTotalBytes)
		}
		input, err := os.Open(current)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(hash, input)
		closeErr := input.Close()
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		_, _ = io.WriteString(hash, "\x00")
		return nil
	})
	if err != nil {
		return 0, 0, "", fmt.Errorf("inspect source_workspace: %w", err)
	}
	return files, bytesCount, hex.EncodeToString(hash.Sum(nil)), nil
}

func (w *legionCodeWorkspaceRuntime) Cleanup() error {
	if w == nil {
		return nil
	}
	w.cleanupOnce.Do(func() {
		if w.cleanup != nil {
			w.cleanupErr = w.cleanup()
		}
	})
	return w.cleanupErr
}

func (w *legionCodeWorkspaceRuntime) info() map[string]any {
	if w == nil {
		return nil
	}
	return map[string]any{
		"source_workspace": map[string]any{
			"workspace_id":       w.spec.WorkspaceID,
			"kind":               w.spec.Kind,
			"locator":            w.spec.Locator,
			"branch":             w.spec.Branch,
			"subpath":            w.spec.Subpath,
			"payload_id":         w.spec.PayloadID,
			"expected_revision":  w.spec.ExpectedRevision,
			"expected_sha256":    w.spec.ExpectedSHA256,
			"read_only":          true,
			"max_files":          w.spec.MaxFiles,
			"max_total_bytes":    w.spec.MaxTotalBytes,
			"max_file_bytes":     w.spec.MaxFileBytes,
			"max_read_bytes":     w.spec.MaxReadBytes,
			"max_search_results": w.spec.MaxSearchResults,
		},
		"locked_revision": w.lockedRevision,
		"sha256":          w.sha256,
		"files":           w.files,
		"bytes":           w.bytes,
	}
}

func (w *legionCodeWorkspaceRuntime) lockedAsset(target string) aiFocusAssetResult {
	payload, _ := json.Marshal(map[string]any{
		"workspace_id":    w.spec.WorkspaceID,
		"kind":            w.spec.Kind,
		"locked_revision": w.lockedRevision,
		"sha256":          w.sha256,
		"files":           w.files,
		"bytes":           w.bytes,
		"subpath":         w.spec.Subpath,
	})
	return aiFocusAssetResult{
		Kind:        "source_locked",
		Title:       "Source workspace locked",
		Target:      target,
		IdentityKey: "source_locked:" + w.spec.WorkspaceID,
		Payload:     payload,
	}
}

func (w *legionCodeWorkspaceRuntime) list(params map[string]any) (map[string]any, error) {
	root, rel, err := w.resolve(focusRuntimeString(params, "path"))
	if err != nil {
		return nil, err
	}
	limit := utils.InterfaceToInt(params["limit"])
	if limit <= 0 || limit > w.spec.MaxSearchResults {
		limit = w.spec.MaxSearchResults
	}
	recursive := true
	if _, configured := params["recursive"]; configured {
		recursive = utils.InterfaceToBoolean(params["recursive"])
	}
	entries := make([]map[string]any, 0, limit)
	truncated := false
	err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			if !entry.IsDir() {
				return w.appendListEntry(current, rel, &entries, limit, &truncated)
			}
			return nil
		}
		childRel, err := filepath.Rel(w.root, current)
		if err != nil {
			return err
		}
		childRel = filepath.ToSlash(childRel)
		if childRel == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			if !recursive && filepath.Dir(current) != root {
				return filepath.SkipDir
			}
			return nil
		}
		if len(entries) >= limit {
			truncated = true
			return filepath.SkipAll
		}
		return w.appendListEntry(current, childRel, &entries, limit, &truncated)
	})
	if err != nil {
		return nil, fmt.Errorf("source.list: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i]["path"].(string) < entries[j]["path"].(string) })
	return map[string]any{"path": rel, "files": entries, "count": len(entries), "truncated": truncated}, nil
}

func (w *legionCodeWorkspaceRuntime) appendListEntry(
	current string,
	rel string,
	entries *[]map[string]any,
	limit int,
	truncated *bool,
) error {
	if len(*entries) >= limit {
		*truncated = true
		return nil
	}
	resolved, _, err := w.resolve(rel)
	if err != nil {
		return nil
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Size() > w.spec.MaxFileBytes {
		return nil
	}
	binary, err := legionCodeFileIsBinary(resolved, info.Size())
	if err != nil || binary {
		return nil
	}
	*entries = append(*entries, map[string]any{"path": filepath.ToSlash(rel), "size": info.Size()})
	return nil
}

func (w *legionCodeWorkspaceRuntime) read(params map[string]any) (map[string]any, error) {
	resolved, rel, err := w.resolve(focusRuntimeString(params, "path"))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source.read path %q is not a regular file", rel)
	}
	if info.Size() > w.spec.MaxFileBytes {
		return nil, fmt.Errorf("source.read file %q exceeds %d bytes", rel, w.spec.MaxFileBytes)
	}
	binary, err := legionCodeFileIsBinary(resolved, info.Size())
	if err != nil {
		return nil, fmt.Errorf("source.read inspect %q: %w", rel, err)
	}
	if binary {
		return nil, fmt.Errorf("source.read file %q is binary", rel)
	}
	offset := int64(utils.InterfaceToInt(params["offset"]))
	if offset < 0 || offset > info.Size() {
		return nil, fmt.Errorf("source.read offset is out of range")
	}
	limit := int64(utils.InterfaceToInt(params["max_bytes"]))
	if limit <= 0 || limit > w.spec.MaxReadBytes {
		limit = w.spec.MaxReadBytes
	}
	input, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	if _, err := input.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(io.LimitReader(input, limit+1))
	if err != nil {
		return nil, err
	}
	truncated := len(content) > int(limit)
	if truncated {
		content = content[:limit]
	}
	if !utf8.Valid(content) {
		return nil, fmt.Errorf("source.read file %q is not UTF-8 text", rel)
	}
	sum := sha256.Sum256(content)
	return map[string]any{
		"path":       rel,
		"offset":     offset,
		"content":    string(content),
		"read_bytes": len(content),
		"file_size":  info.Size(),
		"sha256":     hex.EncodeToString(sum[:]),
		"truncated":  truncated || offset+int64(len(content)) < info.Size(),
	}, nil
}

func (w *legionCodeWorkspaceRuntime) search(params map[string]any) (map[string]any, error) {
	query := focusRuntimeRawString(params, "query")
	if query == "" || len(query) > 1024 || strings.ContainsRune(query, '\x00') {
		return nil, fmt.Errorf("source.search query must contain between 1 and 1024 safe bytes")
	}
	root, rel, err := w.resolve(focusRuntimeString(params, "path"))
	if err != nil {
		return nil, err
	}
	limit := utils.InterfaceToInt(params["limit"])
	if limit <= 0 || limit > w.spec.MaxSearchResults {
		limit = w.spec.MaxSearchResults
	}
	caseSensitive := utils.InterfaceToBoolean(params["case_sensitive"])
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(needle)
	}
	results := make([]map[string]any, 0, limit)
	var scannedBytes int64
	truncated := false
	err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		childRel, err := filepath.Rel(w.root, current)
		if err != nil {
			return err
		}
		childRel = filepath.ToSlash(childRel)
		if childRel == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		if len(results) >= limit || scannedBytes >= w.spec.MaxReadBytes {
			truncated = true
			return filepath.SkipAll
		}
		resolved, _, err := w.resolve(childRel)
		if err != nil {
			return nil
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Size() > w.spec.MaxFileBytes {
			return nil
		}
		binary, err := legionCodeFileIsBinary(resolved, info.Size())
		if err != nil || binary {
			return nil
		}
		remaining := w.spec.MaxReadBytes - scannedBytes
		input, err := os.Open(resolved)
		if err != nil {
			return nil
		}
		reader := bufio.NewReader(io.LimitReader(input, remaining))
		lineNumber := 0
		for {
			line, readErr := reader.ReadString('\n')
			scannedBytes += int64(len(line))
			lineNumber++
			haystack := line
			if !caseSensitive {
				haystack = strings.ToLower(haystack)
			}
			if column := strings.Index(haystack, needle); column >= 0 {
				preview := strings.TrimRight(strings.ToValidUTF8(line, "�"), "\r\n")
				if len(preview) > 512 {
					preview = preview[:512]
				}
				results = append(results, map[string]any{
					"path": childRel, "line": lineNumber, "column": column + 1, "preview": preview,
				})
				if len(results) >= limit {
					truncated = true
					break
				}
			}
			if readErr != nil {
				break
			}
		}
		_ = input.Close()
		if scannedBytes >= w.spec.MaxReadBytes {
			truncated = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("source.search: %w", err)
	}
	return map[string]any{
		"path": rel, "query": query, "results": results, "count": len(results),
		"scanned_bytes": scannedBytes, "max_read_bytes": w.spec.MaxReadBytes, "truncated": truncated,
	}, nil
}

func (w *legionCodeWorkspaceRuntime) resolve(raw string) (string, string, error) {
	if w == nil || strings.TrimSpace(w.root) == "" {
		return "", "", fmt.Errorf("source workspace is unavailable")
	}
	rel, err := cleanLegionCodeRelativePath(raw, true)
	if err != nil {
		return "", "", err
	}
	candidate := filepath.Join(w.root, filepath.FromSlash(rel))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", "", fmt.Errorf("source path %q cannot be resolved: %w", rel, err)
	}
	rootResolved, err := filepath.EvalSymlinks(w.root)
	if err != nil {
		return "", "", fmt.Errorf("source workspace root cannot be resolved: %w", err)
	}
	inside, err := filepath.Rel(rootResolved, resolved)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		return "", "", fmt.Errorf("source path %q escapes the workspace", rel)
	}
	return resolved, rel, nil
}

func cleanLegionCodeRelativePath(raw string, allowRoot bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" && allowRoot {
		return ".", nil
	}
	if raw == "" || strings.Contains(raw, "\\") || strings.ContainsRune(raw, '\x00') || path.IsAbs(raw) {
		return "", fmt.Errorf("source path must be a safe relative path")
	}
	cleaned := path.Clean(raw)
	if cleaned == "." && allowRoot {
		return cleaned, nil
	}
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != raw {
		return "", fmt.Errorf("source path must be canonical and cannot traverse parents")
	}
	if depth := len(strings.Split(cleaned, "/")); depth > legionCodeWorkspaceMaxPathDepth {
		return "", fmt.Errorf("source path exceeds maximum depth %d", legionCodeWorkspaceMaxPathDepth)
	}
	return cleaned, nil
}

func cleanLegionCodeArchivePath(raw string) (string, error) {
	if strings.HasSuffix(raw, "/") {
		raw = strings.TrimSuffix(raw, "/")
	}
	cleaned, err := cleanLegionCodeRelativePath(raw, false)
	if err != nil {
		return "", fmt.Errorf("unsafe source_workspace zip path %q: %w", raw, err)
	}
	return cleaned, nil
}

func legionCodeWorkspaceSubpath(root string, subpath string) (string, error) {
	if strings.TrimSpace(subpath) == "" {
		return root, nil
	}
	cleaned, err := cleanLegionCodeRelativePath(subpath, false)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(root, filepath.FromSlash(cleaned))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("source_workspace subpath %q cannot be resolved: %w", cleaned, err)
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("source_workspace subpath %q escapes the workspace", cleaned)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("source_workspace subpath %q is not a directory", cleaned)
	}
	return resolved, nil
}

func legionCodeFileIsBinary(filename string, size int64) (bool, error) {
	input, err := os.Open(filename)
	if err != nil {
		return false, err
	}
	defer input.Close()
	probeSize := int64(8192)
	if size < probeSize {
		probeSize = size
	}
	probe, err := io.ReadAll(io.LimitReader(input, probeSize))
	if err != nil {
		return false, err
	}
	return strings.IndexByte(string(probe), 0) >= 0, nil
}

func validateLegionCodeProxyURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("source_workspace proxy URL is invalid")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5":
		return nil
	default:
		return fmt.Errorf("source_workspace proxy scheme %q is unsupported", parsed.Scheme)
	}
}

func publicLegionCodeWorkspaceSpec(spec legionCodeWorkspaceSpec) legionCodeWorkspaceSpec {
	spec.Auth = nil
	spec.Proxy = nil
	return spec
}

func validateLegionCodeWorkspaceContextPin(bindRaw, contextRaw []byte) error {
	bindOptions, err := decodeYakRuntimeOptions(bindRaw, true)
	if err != nil {
		return fmt.Errorf("decode bind source_workspace pin: %w", err)
	}
	contextOptions, err := decodeYakRuntimeOptions(contextRaw, true)
	if err != nil {
		return fmt.Errorf("decode context source_workspace pin: %w", err)
	}
	bound := bindOptions.SourceWorkspace
	received := contextOptions.SourceWorkspace
	switch {
	case bound == nil && received == nil:
		return nil
	case bound == nil:
		return fmt.Errorf("context source_workspace was not present at bind")
	case received == nil:
		return fmt.Errorf("context source_workspace pin is missing")
	case received.Auth != nil || received.Proxy != nil:
		return fmt.Errorf("context source_workspace must not contain auth or proxy")
	}
	boundLocator := *bound
	boundLocator.Kind = strings.ToLower(strings.TrimSpace(boundLocator.Kind))
	boundLocator.Locator = strings.TrimSpace(boundLocator.Locator)
	if err := normalizeLegionCodeWorkspaceLocator(&boundLocator); err != nil {
		return fmt.Errorf("bind source_workspace locator is invalid: %w", err)
	}
	receivedLocator := *received
	receivedLocator.Kind = strings.ToLower(strings.TrimSpace(receivedLocator.Kind))
	receivedLocator.Locator = strings.TrimSpace(receivedLocator.Locator)
	if err := normalizeLegionCodeWorkspaceLocator(&receivedLocator); err != nil {
		return fmt.Errorf("context source_workspace locator is invalid: %w", err)
	}
	if strings.TrimSpace(bound.WorkspaceID) != strings.TrimSpace(received.WorkspaceID) ||
		strings.ToLower(strings.TrimSpace(bound.Kind)) != strings.ToLower(strings.TrimSpace(received.Kind)) ||
		strings.TrimSpace(bound.ExpectedRevision) != strings.TrimSpace(received.ExpectedRevision) ||
		strings.ToLower(strings.TrimSpace(bound.ExpectedSHA256)) != strings.ToLower(strings.TrimSpace(received.ExpectedSHA256)) {
		return fmt.Errorf("context source_workspace pin does not match bind snapshot")
	}
	return nil
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
