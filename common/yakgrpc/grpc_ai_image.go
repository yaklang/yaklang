package yakgrpc

import (
	"context"
	"fmt"
	"image"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ReadAIImage(ctx context.Context, req *ypb.AIImageRequest) (*ypb.AIImageResponse, error) {
	if req.GetSessionID() == "" || req.GetImageID() == "" {
		return nil, fmt.Errorf("session and image IDs are required")
	}
	var store *aimedia.Catalog
	var err error
	if provider, ok := s.getReActSessionRuntime().(interface {
		ImageCatalogForSession(string) (*aimedia.Catalog, error)
	}); ok {
		store, err = provider.ImageCatalogForSession(req.SessionID)
		if err != nil {
			return nil, err
		}
	}
	if store == nil {
		runtime, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(s.GetProjectDatabase(), req.SessionID)
		if err != nil {
			return nil, err
		}
		if runtime.WorkDir == "" {
			return nil, fmt.Errorf("session image workspace is unavailable")
		}
		store, err = aimedia.OpenCatalog(aicommon.ImageCatalogDirectory(runtime.WorkDir, req.SessionID))
		if err != nil {
			return nil, err
		}
	}
	if req.Scope != "" && req.Scope != req.SessionID {
		store, err = aimedia.OpenCatalog(aicommon.ImageCatalogDirectory(filepath.Dir(filepath.Dir(store.Root())), req.Scope))
		if err != nil {
			return nil, err
		}
		if store.Owner() != req.SessionID {
			return nil, fmt.Errorf("image scope is not owned by this session")
		}
	}
	if owner := store.Owner(); owner != "" && owner != req.SessionID {
		return nil, fmt.Errorf("image context belongs to a different session")
	}
	for _, o := range store.Snapshot() {
		if o.ID == req.ImageID || o.Image.ID == req.ImageID {
			img := o.Image
			switch req.Variant {
			case "", "thumbnail":
				view, err := aimedia.Derive(ctx, img, image.Rectangle{}, 480)
				if err != nil {
					return nil, err
				}
				img = view.Image
			case "original":
			case "model":
				// Compatibility alias: model views are request-local, not mutable
				// state on a retained resource. Return a standard overview.
				view, err := aimedia.Derive(ctx, img, image.Rectangle{}, 1536)
				if err != nil {
					return nil, err
				}
				img = view.Image
			default:
				return nil, fmt.Errorf("unknown image variant")
			}
			raw, err := img.Bytes(ctx)
			if err != nil {
				return nil, err
			}
			return &ypb.AIImageResponse{Data: raw, MIMEType: img.MIMEType, Width: int64(img.Width), Height: int64(img.Height), ImageID: o.ID}, nil
		}
	}
	return nil, fmt.Errorf("image is not visible in this session")
}
