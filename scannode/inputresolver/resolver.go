package inputresolver

import (
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
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/proto"
)

const (
	leaseVersion     = "legion.input-workspace-lease/v1"
	maxMetadataBytes = 128 << 10
)

// Error deliberately carries no URL, credential, host path or server body.
// The same resource-specific code is safe for persisted preparation events.
type Error struct{ Code, ResourceID string }

func (e *Error) Error() string {
	if e.ResourceID == "" {
		return e.Code
	}
	return e.Code + ": " + e.ResourceID
}

func fail(code, resource string) error { return &Error{Code: code, ResourceID: resource} }

// Identity must come from the authenticated Bind, not from the manifest itself.
type Identity struct{ OwnerUserID, SessionID, AttemptID, RunID, ProductKey string }

func ValidateBinding(manifest *aiv1.InputManifest, identity Identity, refs []*aiv1.AISessionAttachmentRef) error {
	if err := Validate(manifest); err != nil {
		return fail("input_manifest_invalid", "")
	}
	if identity.OwnerUserID == "" || identity.SessionID == "" || identity.AttemptID == "" ||
		manifest.OwnerUserId != identity.OwnerUserID || manifest.SessionId != identity.SessionID ||
		manifest.AttemptId != identity.AttemptID || (identity.RunID != "" && manifest.RunId != identity.RunID) ||
		(identity.ProductKey != "" && manifest.ProductKey != identity.ProductKey) {
		return fail("input_identity_mismatch", "")
	}
	if len(refs) != len(manifest.Resources) {
		return fail("input_authorization_mismatch", "")
	}
	authorized := make(map[string]*aiv1.AISessionAttachmentRef, len(refs))
	for _, ref := range refs {
		if ref == nil || ref.AttachmentId == "" || authorized[ref.AttachmentId] != nil {
			return fail("input_authorization_mismatch", "")
		}
		authorized[ref.AttachmentId] = ref
	}
	for _, resource := range manifest.Resources {
		ref := authorized[resource.ResourceId]
		if ref == nil || ref.SizeBytes != resource.SizeBytes || ref.Sha256 != resource.Sha256 ||
			ref.Filename != resource.Filename || ref.ContentType != resource.MediaType {
			return fail("input_authorization_mismatch", resource.ResourceId)
		}
	}
	return nil
}

type Event struct {
	RunID       string `json:"run_id"`
	SessionID   string `json:"session_id"`
	AttemptID   string `json:"attempt_id"`
	WorkspaceID string `json:"workspace_id"`
	ManifestID  string `json:"manifest_id"`
	ResourceID  string `json:"resource_id,omitempty"`
	Path        string `json:"relative_path,omitempty"`
	Operation   string `json:"operation,omitempty"`
	Bytes       int64  `json:"bytes_completed,omitempty"`
	BytesRead   int64  `json:"bytes_read,omitempty"`
	Files       []File `json:"files,omitempty"`
	TotalBytes  uint64 `json:"total_bytes,omitempty"`
	Offset      int64  `json:"offset,omitempty"`
	EndOffset   int64  `json:"end_offset,omitempty"`
	StartLine   int64  `json:"line_start,omitempty"`
	EndLine     int64  `json:"line_end,omitempty"`
	Code        string `json:"error_code,omitempty"`
	Message     string `json:"message,omitempty"`
}

type EmitFunc func(string, Event)

type Config struct {
	Root                                                                                  string
	MaxResourceBytes, MaxWorkspaceBytes, MaxReservedBytes, OutputBytes, DiskHeadroomBytes uint64
	MaxWorkspaces                                                                         int
	DownloadConcurrency                                                                   int
	StallTimeout, TotalTimeout, OrphanGrace                                               time.Duration
	ReclaimLimit                                                                          int
	// AvailableBytes is injectable only by trusted embedding code for disk-failure tests.
	AvailableBytes func(string) (uint64, error)
}

type Resolver struct {
	config    Config
	downloads chan struct{}
}

func New(config Config) (*Resolver, error) {
	if !Supported() {
		return nil, fail("input_platform_unsupported", "")
	}
	if config.Root == "" {
		return nil, fail("input_workspace_root_invalid", "")
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, fail("input_workspace_root_invalid", "")
	}
	config.Root = root
	if config.MaxResourceBytes == 0 {
		config.MaxResourceBytes = 8 << 30
	}
	if config.MaxWorkspaceBytes == 0 {
		config.MaxWorkspaceBytes = 16 << 30
	}
	if config.MaxReservedBytes == 0 {
		config.MaxReservedBytes = 32 << 30
	}
	if config.OutputBytes == 0 {
		config.OutputBytes = 64 << 20
	}
	if config.DiskHeadroomBytes == 0 {
		config.DiskHeadroomBytes = 64 << 20
	}
	if config.MaxWorkspaces <= 0 {
		config.MaxWorkspaces = 128
	}
	if config.DownloadConcurrency <= 0 {
		config.DownloadConcurrency = 2
	}
	if config.StallTimeout <= 0 {
		config.StallTimeout = 30 * time.Second
	}
	if config.TotalTimeout <= 0 {
		config.TotalTimeout = 2 * time.Hour
	}
	if config.OrphanGrace <= 0 {
		config.OrphanGrace = 5 * time.Minute
	}
	if config.ReclaimLimit <= 0 {
		config.ReclaimLimit = 32
	}
	if config.AvailableBytes == nil {
		config.AvailableBytes = availableBytes
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fail("input_disk_unavailable", "")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fail("input_workspace_root_invalid", "")
	}
	staging := filepath.Join(root, ".staging")
	if err := os.Mkdir(staging, 0700); err != nil && !os.IsExist(err) {
		return nil, fail("input_disk_unavailable", "")
	}
	stagingInfo, err := os.Lstat(staging)
	if err != nil || !stagingInfo.IsDir() || stagingInfo.Mode().Perm()&0077 != 0 {
		return nil, fail("input_workspace_root_invalid", "")
	}
	r := &Resolver{config: config, downloads: make(chan struct{}, config.DownloadConcurrency)}
	if _, err := r.Reclaim(context.Background()); err != nil {
		return nil, err
	}
	return r, nil
}

type DownloadOptions struct {
	APIBaseURL, NodeSessionID, BearerToken string
	HTTPClient                             *http.Client
}

type lease struct {
	Version       string              `json:"version"`
	Directory     string              `json:"directory"`
	CreatedAt     time.Time           `json:"created_at"`
	ReservedBytes uint64              `json:"reserved_bytes"`
	Manifest      *aiv1.InputManifest `json:"manifest"`
}

type Workspace struct {
	resolver    *Resolver
	manifest    *aiv1.InputManifest
	root        string
	leaseFile   *os.File
	ctx         context.Context
	cancel      context.CancelFunc
	emit        EmitFunc
	mu          sync.RWMutex
	closed      bool
	outputBytes uint64
	cleanupOnce sync.Once
	cleanupErr  error
}

func (r *Resolver) Prepare(ctx context.Context, manifest *aiv1.InputManifest, identity Identity,
	refs []*aiv1.AISessionAttachmentRef, options DownloadOptions, emit EmitFunc) (*Workspace, error) {
	if err := ValidateBinding(manifest, identity, refs); err != nil {
		return nil, err
	}
	if _, err := r.Reclaim(ctx); err != nil {
		return nil, err
	}
	manifest = proto.Clone(manifest).(*aiv1.InputManifest)
	if _, err := managedURL(options, manifest.Resources[0].ResourceId); err != nil {
		return nil, err
	}
	var total uint64
	for _, resource := range manifest.Resources {
		if resource.SizeBytes > r.config.MaxResourceBytes || resource.SizeBytes > r.config.MaxWorkspaceBytes-total {
			return nil, fail("input_quota_exceeded", resource.ResourceId)
		}
		total += resource.SizeBytes
	}
	ctx, cancel := context.WithCancel(ctx)
	w := &Workspace{resolver: r, manifest: manifest, ctx: ctx, cancel: cancel, emit: emit}
	w.event("input.workspace.preparing", Event{TotalBytes: total})
	if err := r.allocate(w, total); err != nil {
		cancel()
		w.failure(err)
		return nil, err
	}
	err := func() error {
		for _, resource := range manifest.Resources {
			if err := r.download(ctx, w, resource, options); err != nil {
				return err
			}
		}
		return makeInputsReadOnly(filepath.Join(w.root, "inputs"))
	}()
	if err != nil {
		w.failure(err)
		_ = w.Cleanup()
		return nil, err
	}
	w.event("input.workspace.ready", Event{TotalBytes: total, Files: w.Files()})
	return w, nil
}

func (w *Workspace) event(name string, event Event) {
	if w.emit == nil {
		return
	}
	event.RunID, event.AttemptID = w.manifest.RunId, w.manifest.AttemptId
	event.SessionID = w.manifest.SessionId
	event.WorkspaceID, event.ManifestID = w.manifest.WorkspaceId, w.manifest.ManifestId
	w.emit(name, event)
}

func (w *Workspace) failure(err error) {
	event := Event{Code: "input_prepare_failed", Message: "input workspace preparation failed"}
	var detail *Error
	if errors.As(err, &detail) {
		event.Code, event.ResourceID = detail.Code, detail.ResourceID
	}
	w.event("input.workspace.failed", event)
}

func (r *Resolver) allocationLock() (*os.File, error) {
	file, err := openBeneath(r.config.Root, ".allocator.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err == nil {
		err = lockFile(file, false)
	}
	if err != nil {
		if file != nil {
			file.Close()
		}
		return nil, fail("input_disk_unavailable", "")
	}
	return file, nil
}

func (r *Resolver) allocate(w *Workspace, bytes uint64) error {
	lock, err := r.allocationLock()
	if err != nil {
		return err
	}
	defer lock.Close()
	if w.ctx.Err() != nil {
		return fail("input_cancelled", "")
	}
	reserved, err := r.reservations()
	if err != nil {
		return err
	}
	if r.config.OutputBytes > r.config.MaxReservedBytes || bytes > r.config.MaxReservedBytes-r.config.OutputBytes {
		return fail("input_quota_exceeded", "")
	}
	needed := bytes + r.config.OutputBytes
	if reserved > r.config.MaxReservedBytes || needed > r.config.MaxReservedBytes-reserved {
		return fail("input_quota_exceeded", "")
	}
	available, err := r.config.AvailableBytes(r.config.Root)
	if err != nil || available < r.config.DiskHeadroomBytes || reserved > available-r.config.DiskHeadroomBytes || needed > available-r.config.DiskHeadroomBytes-reserved {
		return fail("input_disk_insufficient", "")
	}
	dir, err := os.MkdirTemp(filepath.Join(r.config.Root, ".staging"), "workspace-")
	if err != nil {
		return fail("input_disk_unavailable", "")
	}
	w.root = dir
	remove := true
	defer func() {
		if remove {
			_ = os.RemoveAll(dir)
		}
	}()
	file, err := openBeneath(dir, ".lease.lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return fail("input_disk_unavailable", "")
	}
	if err := lockFile(file, false); err != nil {
		file.Close()
		return fail("input_disk_unavailable", "")
	}
	w.leaseFile = file
	defer func() {
		if remove {
			file.Close()
			w.leaseFile = nil
		}
	}()
	metadata := lease{Version: leaseVersion, Directory: filepath.Base(dir), CreatedAt: time.Now().UTC(), ReservedBytes: needed, Manifest: w.manifest}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return fail("input_metadata_invalid", "")
	}
	metaFile, err := openBeneath(dir, ".lease.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fail("input_disk_unavailable", "")
	}
	_, writeErr := metaFile.Write(raw)
	err = errors.Join(writeErr, metaFile.Sync(), metaFile.Close())
	if err != nil {
		return fail("input_disk_unavailable", "")
	}
	for _, child := range []string{"inputs", "outputs"} {
		if err := os.Mkdir(filepath.Join(dir, child), 0700); err != nil {
			return fail("input_disk_unavailable", "")
		}
	}
	parent, err := os.Open(dir)
	if err != nil {
		return fail("input_disk_unavailable", "")
	}
	err = errors.Join(parent.Sync(), parent.Close())
	if err != nil {
		return fail("input_disk_unavailable", "")
	}
	// Publish the complete durable lease atomically. A crash before this
	// rename leaves only private staging metadata, never a poisoned active
	// reservation; resource downloads begin only after publication.
	finalDir := filepath.Join(r.config.Root, filepath.Base(dir))
	if err := os.Rename(dir, finalDir); err != nil {
		return fail("input_disk_unavailable", "")
	}
	dir = finalDir
	w.root = finalDir
	parentRoot, err := os.Open(r.config.Root)
	if err != nil {
		return fail("input_disk_unavailable", "")
	}
	err = errors.Join(parentRoot.Sync(), parentRoot.Close())
	if err != nil {
		return fail("input_disk_unavailable", "")
	}
	remove = false
	return nil
}

func readLease(root, name string) (*lease, error) {
	if !strings.HasPrefix(name, "workspace-") || filepath.Base(name) != name {
		return nil, fail("input_metadata_invalid", "")
	}
	file, err := openBeneath(root, name+"/.lease.json", os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxMetadataBytes+1))
	if err != nil || len(raw) > maxMetadataBytes {
		return nil, fail("input_metadata_invalid", "")
	}
	var metadata lease
	if json.Unmarshal(raw, &metadata) != nil || metadata.Version != leaseVersion || metadata.Directory != name ||
		metadata.CreatedAt.IsZero() || metadata.ReservedBytes == 0 || Validate(metadata.Manifest) != nil {
		return nil, fail("input_metadata_invalid", "")
	}
	return &metadata, nil
}

func (r *Resolver) reservations() (uint64, error) {
	rootDir, err := os.Open(r.config.Root)
	if err != nil {
		return 0, fail("input_disk_unavailable", "")
	}
	entries, err := rootDir.ReadDir(r.config.MaxWorkspaces + 9)
	rootDir.Close()
	if err != nil && err != io.EOF {
		return 0, fail("input_disk_unavailable", "")
	}
	if len(entries) > r.config.MaxWorkspaces+8 {
		return 0, fail("input_quota_exceeded", "")
	}
	var total uint64
	count := 0
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "workspace-") {
			continue
		}
		count++
		if count >= r.config.MaxWorkspaces {
			return 0, fail("input_quota_exceeded", "")
		}
		metadata, err := readLease(r.config.Root, entry.Name())
		// Never guess ownership or capacity for unrecognizable directories.
		if err != nil {
			return 0, fail("input_metadata_invalid", "")
		}
		if metadata.ReservedBytes > r.config.MaxReservedBytes-total {
			return 0, fail("input_quota_exceeded", "")
		}
		total += metadata.ReservedBytes
	}
	return total, nil
}

// Reclaim removes only an expired, recognized lease whose process lock is free.
// A crashed process releases the flock even if defer never ran. The scan and
// number of removals are bounded; unknown directories and live leases survive.
func (r *Resolver) Reclaim(ctx context.Context) (int, error) {
	lock, err := r.allocationLock()
	if err != nil {
		return 0, err
	}
	defer lock.Close()
	removed := 0
	for _, scanRoot := range []string{r.config.Root, filepath.Join(r.config.Root, ".staging")} {
		dir, err := os.Open(scanRoot)
		if err != nil {
			return removed, fail("input_disk_unavailable", "")
		}
		entries, err := dir.ReadDir(r.config.ReclaimLimit * 8)
		dir.Close()
		if err != nil && err != io.EOF {
			return removed, fail("input_disk_unavailable", "")
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return removed, ctx.Err()
			}
			if removed >= r.config.ReclaimLimit {
				break
			}
			metadata, err := readLease(scanRoot, entry.Name())
			if err != nil || time.Since(metadata.CreatedAt) < r.config.OrphanGrace {
				continue
			}
			file, err := openBeneath(scanRoot, entry.Name()+"/.lease.lock", os.O_RDWR, 0)
			if err != nil {
				continue
			}
			if lockFile(file, true) != nil {
				file.Close()
				continue
			}
			err = removeWorkspace(filepath.Join(scanRoot, entry.Name()))
			file.Close()
			if err != nil {
				return removed, fail("input_cleanup_failed", "")
			}
			removed++
		}
	}
	return removed, nil
}

func (w *Workspace) Cleanup() error {
	if w == nil {
		return nil
	}
	w.cleanupOnce.Do(func() {
		w.cancel()
		w.mu.Lock()
		defer w.mu.Unlock()
		w.closed = true
		if w.leaseFile == nil {
			return
		}
		lock, err := w.resolver.allocationLock()
		if err == nil {
			err = removeWorkspace(w.root)
			lock.Close()
		}
		w.leaseFile.Close()
		w.leaseFile = nil
		if err != nil {
			w.cleanupErr = fail("input_cleanup_failed", "")
			w.event("input.workspace.failed", Event{Code: "input_cleanup_failed"})
			return
		}
		w.event("input.workspace.cleaned", Event{})
	})
	return w.cleanupErr
}

func removeWorkspace(root string) error {
	if err := filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(current, 0700)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func makeInputsReadOnly(root string) error {
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink")
		}
		if entry.IsDir() {
			return os.Chmod(current, 0500)
		}
		return os.Chmod(current, 0400)
	})
	if err != nil {
		return fail("input_disk_unavailable", "")
	}
	return nil
}

func managedURL(options DownloadOptions, resourceID string) (string, error) {
	if options.NodeSessionID == "" || options.BearerToken == "" {
		return "", fail("input_credentials_missing", resourceID)
	}
	base, err := url.Parse(options.APIBaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" ||
		base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return "", fail("input_endpoint_invalid", resourceID)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/ai/attachments/" + url.PathEscape(resourceID) + "/download"
	base.RawPath = ""
	query := base.Query()
	query.Set("node_session_id", options.NodeSessionID)
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (r *Resolver) download(ctx context.Context, w *Workspace, resource *aiv1.InputResource, options DownloadOptions) error {
	select {
	case r.downloads <- struct{}{}:
		defer func() { <-r.downloads }()
	case <-ctx.Done():
		return fail("input_cancelled", resource.ResourceId)
	}
	ctx, cancel := context.WithTimeout(ctx, r.config.TotalTimeout)
	defer cancel()
	endpoint, err := managedURL(options, resource.ResourceId)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fail("input_endpoint_invalid", resource.ResourceId)
	}
	request.Header.Set("Authorization", "Bearer "+options.BearerToken)
	request.Header.Set("Accept-Encoding", "identity")
	client := http.DefaultClient
	if options.HTTPClient != nil {
		client = options.HTTPClient
	}
	copyClient := *client
	copyClient.Timeout = 0 // Total and idle budgets are independent of object size.
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var stalled atomic.Bool
	timer := time.AfterFunc(r.config.StallTimeout, func() { stalled.Store(true); cancel() })
	defer timer.Stop()
	response, err := copyClient.Do(request)
	if err != nil {
		if stalled.Load() {
			return fail("input_download_stalled", resource.ResourceId)
		}
		if ctx.Err() != nil {
			return fail("input_cancelled", resource.ResourceId)
		}
		return fail("input_download_failed", resource.ResourceId)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		code := "input_download_failed"
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "input_unauthorized"
		case http.StatusNotFound:
			code = "input_missing"
		}
		return fail(code, resource.ResourceId)
	}
	if response.ContentLength >= 0 && uint64(response.ContentLength) != resource.SizeBytes {
		return fail("input_size_mismatch", resource.ResourceId)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(w.root, resource.RelativePath)), 0700); err != nil {
		return fail("input_disk_unavailable", resource.ResourceId)
	}
	partID := sha256.Sum256([]byte(resource.ResourceId))
	partPath := ".part-" + hex.EncodeToString(partID[:])
	file, err := openBeneath(w.root, partPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fail("input_disk_unavailable", resource.ResourceId)
	}
	defer file.Close()
	buf := make([]byte, 64<<10)
	hash := sha256.New()
	var written int64
	lastProgress := time.Now()
	for {
		if ctx.Err() != nil {
			if stalled.Load() {
				return fail("input_download_stalled", resource.ResourceId)
			}
			return fail("input_cancelled", resource.ResourceId)
		}
		n, readErr := response.Body.Read(buf)
		if n > 0 {
			timer.Reset(r.config.StallTimeout)
			if uint64(written)+uint64(n) > resource.SizeBytes {
				return fail("input_size_mismatch", resource.ResourceId)
			}
			if _, err := file.Write(buf[:n]); err != nil {
				return fail("input_disk_unavailable", resource.ResourceId)
			}
			hash.Write(buf[:n])
			written += int64(n)
			if time.Since(lastProgress) >= time.Second || uint64(written) == resource.SizeBytes {
				w.event("input.workspace.progress", Event{ResourceID: resource.ResourceId, Path: resource.RelativePath, Bytes: written, TotalBytes: resource.SizeBytes})
				lastProgress = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if stalled.Load() {
				return fail("input_download_stalled", resource.ResourceId)
			}
			if ctx.Err() != nil {
				return fail("input_cancelled", resource.ResourceId)
			}
			return fail("input_download_truncated", resource.ResourceId)
		}
	}
	if uint64(written) != resource.SizeBytes {
		return fail("input_size_mismatch", resource.ResourceId)
	}
	if hex.EncodeToString(hash.Sum(nil)) != resource.Sha256 {
		return fail("input_hash_mismatch", resource.ResourceId)
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return fail("input_disk_unavailable", resource.ResourceId)
	}
	if err := os.Rename(filepath.Join(w.root, partPath), filepath.Join(w.root, resource.RelativePath)); err != nil {
		return fail("input_disk_unavailable", resource.ResourceId)
	}
	return nil
}
