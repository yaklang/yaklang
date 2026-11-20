package sessionruntime

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
)

// ImageCatalogForSession is an optional read surface for retained image resources.
// It does not expose arbitrary filesystem paths or grant model tool access.
func (r *reActSessionRuntime) ImageCatalogForSession(sessionID string) (*aimedia.Catalog, error) {
	r.mu.Lock()
	entry := r.entries[sessionID]
	if entry == nil || entry.runtime == nil || entry.runtime.react == nil {
		r.mu.Unlock()
		return nil, nil
	}
	config := entry.runtime.react.GetConfig()
	r.mu.Unlock()
	if c := aicommon.ImageConfigFrom(config); c != nil {
		return c.GetImageCatalog()
	}
	return nil, nil
}
