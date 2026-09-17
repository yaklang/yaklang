package yakgrpc

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
	"github.com/yaklang/yaklang/common/yak/c2ssa/preprocess"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const defaultCHeaderPreviewMaxBytes = 512 * 1024

func (s *Server) GetCHeadersDir(ctx context.Context, _ *ypb.Empty) (*ypb.GetCHeadersDirResponse, error) {
	return &ypb.GetCHeadersDirResponse{Dir: consts.GetDefaultCHeadersDir()}, nil
}

func (s *Server) ListCHeaders(ctx context.Context, _ *ypb.Empty) (*ypb.ListCHeadersResponse, error) {
	base, err := absCHeadersDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, utils.Wrap(err, "list c-headers")
	}
	resp := &ypb.ListCHeadersResponse{}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		kind := "file"
		if e.IsDir() {
			kind = "directory"
		} else if strings.EqualFold(filepath.Ext(e.Name()), ".zip") {
			kind = "zip"
		}
		resp.Packs = append(resp.Packs, &ypb.CHeaderPack{
			Name:       e.Name(),
			Kind:       kind,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime().Unix(),
		})
	}
	return resp, nil
}

func (s *Server) ListCHeaderEntries(ctx context.Context, req *ypb.ListCHeaderEntriesRequest) (*ypb.ListCHeaderEntriesResponse, error) {
	if req == nil {
		req = &ypb.ListCHeaderEntriesRequest{}
	}
	rel, err := sanitizeCHeaderRel(req.GetRelativePath())
	if err != nil {
		return nil, err
	}
	rootFS, err := openCHeaderPackFS(req.GetPackName())
	if err != nil {
		return nil, err
	}
	if c, ok := any(rootFS).(io.Closer); ok {
		defer c.Close()
	}
	dir := rel
	if dir == "" {
		dir = "."
	}
	entries, err := rootFS.ReadDir(dir)
	if err != nil {
		return nil, utils.Wrapf(err, "list c-header entries %q", dir)
	}
	resp := &ypb.ListCHeaderEntriesResponse{}
	for _, e := range entries {
		name := e.Name()
		child := name
		if rel != "" {
			child = path.Join(rel, name)
		}
		var size int64
		if info, err := e.Info(); err == nil && info != nil {
			size = info.Size()
		}
		resp.Entries = append(resp.Entries, &ypb.CHeaderEntry{
			Name:         name,
			RelativePath: child,
			IsDir:        e.IsDir(),
			SizeBytes:    size,
		})
	}
	return resp, nil
}

func (s *Server) ImportCHeaderPack(ctx context.Context, req *ypb.ImportCHeaderPackRequest) (*ypb.GeneralResponse, error) {
	if req == nil || strings.TrimSpace(req.GetLocalPath()) == "" {
		return nil, utils.Error("LocalPath is required")
	}
	src := filepath.Clean(req.GetLocalPath())
	info, err := os.Stat(src)
	if err != nil {
		return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
	}

	destName := strings.TrimSpace(req.GetDestName())
	if destName == "" {
		destName = filepath.Base(src)
	}
	if req.GetExtractZip() && !info.IsDir() && strings.EqualFold(filepath.Ext(destName), ".zip") {
		destName = strings.TrimSuffix(destName, filepath.Ext(destName))
	}
	dest, err := confinedCHeaderChild(destName)
	if err != nil {
		return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o777); err != nil {
		return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
	}
	if _, err := os.Stat(dest); err == nil {
		if err := os.RemoveAll(dest); err != nil {
			return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
		}
	}

	switch {
	case req.GetExtractZip() && !info.IsDir():
		if err := ziputil.DeCompress(src, dest); err != nil {
			return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
		}
	case info.IsDir():
		if err := utils.CopyDirectory(src, dest, false); err != nil {
			return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
		}
	default:
		if err := utils.CopyFile(src, dest); err != nil {
			return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
		}
	}
	return &ypb.GeneralResponse{Ok: true}, nil
}

func (s *Server) DeleteCHeaderPack(ctx context.Context, req *ypb.DeleteCHeaderPackRequest) (*ypb.GeneralResponse, error) {
	if req == nil {
		return nil, utils.Error("empty request")
	}
	target, err := confinedCHeaderChild(req.GetName())
	if err != nil {
		return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
	}
	if err := os.RemoveAll(target); err != nil {
		return &ypb.GeneralResponse{Ok: false, Reason: err.Error()}, nil
	}
	return &ypb.GeneralResponse{Ok: true}, nil
}

func (s *Server) PreviewCHeaderFile(ctx context.Context, req *ypb.PreviewCHeaderFileRequest) (*ypb.PreviewCHeaderFileResponse, error) {
	if req == nil {
		req = &ypb.PreviewCHeaderFileRequest{}
	}
	rel, err := sanitizeCHeaderRel(req.GetRelativePath())
	if err != nil {
		return nil, err
	}
	rootFS, err := openCHeaderPackFS(req.GetPackName())
	if err != nil {
		return nil, err
	}
	if c, ok := any(rootFS).(io.Closer); ok {
		defer c.Close()
	}
	if rel == "" {
		return nil, utils.Error("RelativePath is required")
	}
	maxBytes := req.GetMaxBytes()
	if maxBytes <= 0 {
		maxBytes = defaultCHeaderPreviewMaxBytes
	}

	f, err := rootFS.Open(rel)
	if err != nil {
		if sep := rootFS.GetSeparators(); sep != '/' {
			alt := strings.ReplaceAll(rel, "/", string(sep))
			f, err = rootFS.Open(alt)
		}
		if err != nil {
			return nil, utils.Wrapf(err, "open c-header %q", rel)
		}
	}
	defer f.Close()

	limited := io.LimitReader(f, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, utils.Wrap(err, "read c-header")
	}
	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return &ypb.PreviewCHeaderFileResponse{Content: data, Truncated: truncated}, nil
}

func (s *Server) DownloadOfficialCHeaders(ctx context.Context, req *ypb.DownloadOfficialCHeadersRequest) (*ypb.DownloadOfficialCHeadersResponse, error) {
	if req == nil {
		req = &ypb.DownloadOfficialCHeadersRequest{}
	}
	dest, err := preprocess.OfficialCHeadersPackPath()
	if err != nil {
		return &ypb.DownloadOfficialCHeadersResponse{Ok: false, Reason: err.Error()}, nil
	}
	if _, err := os.Stat(dest); err == nil && !req.GetForce() {
		return &ypb.DownloadOfficialCHeadersResponse{
			Ok:       false,
			Reason:   preprocess.OfficialCHeadersZipName + " already exists (set Force to overwrite)",
			PackPath: dest,
			Version:  preprocess.FetchOfficialCHeadersVersion(ctx),
		}, nil
	}

	packPath, version, err := preprocess.EnsureOfficialCHeaders(ctx, req.GetForce())
	if err != nil {
		log.Errorf("DownloadOfficialCHeaders OSS download failed: %v", err)
		return &ypb.DownloadOfficialCHeadersResponse{Ok: false, Reason: err.Error()}, nil
	}
	return &ypb.DownloadOfficialCHeadersResponse{
		Ok:       true,
		Version:  version,
		PackPath: packPath,
	}, nil
}

func absCHeadersDir() (string, error) {
	base := consts.GetDefaultCHeadersDir()
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", utils.Wrap(err, "resolve c-headers dir")
	}
	return abs, nil
}

func validatePackName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return utils.Error("empty pack name")
	}
	if name != filepath.Base(name) {
		return utils.Error("pack name must be a basename")
	}
	if name == "." || name == ".." || strings.ContainsRune(name, 0) {
		return utils.Error("invalid pack name")
	}
	if strings.ContainsAny(name, `/\`) {
		return utils.Error("pack name must not contain path separators")
	}
	return nil
}

func confinedCHeaderChild(name string) (string, error) {
	if err := validatePackName(name); err != nil {
		return "", err
	}
	base, err := absCHeadersDir()
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(base, name))
	if err != nil {
		return "", err
	}
	if target == base || !utils.IsSubPath(target, base) {
		return "", utils.Error("path escapes c-headers directory")
	}
	return target, nil
}

func sanitizeCHeaderRel(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	rel = strings.ReplaceAll(rel, "\\", "/")
	if rel == "" {
		return "", nil
	}
	cleaned := path.Clean(rel)
	if cleaned == "." {
		return "", nil
	}
	if path.IsAbs(cleaned) || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", utils.Error("relative path escapes pack")
	}
	return cleaned, nil
}

// cHeaderFS is the subset of filesys used for listing and preview.
type cHeaderFS interface {
	fs.ReadDirFS
	Open(name string) (fs.File, error)
	GetSeparators() rune
}

func openCHeaderPackFS(packName string) (cHeaderFS, error) {
	base, err := absCHeadersDir()
	if err != nil {
		return nil, err
	}
	packName = strings.TrimSpace(packName)
	root := base
	if packName != "" {
		target, err := confinedCHeaderChild(packName)
		if err != nil {
			return nil, err
		}
		root = target
	}
	opened, err := preprocess.OpenExternalRoot(root)
	if err != nil {
		return nil, err
	}
	fsImpl, ok := opened.(cHeaderFS)
	if !ok {
		return nil, utils.Error("c-header filesystem does not support listing")
	}
	return fsImpl, nil
}
