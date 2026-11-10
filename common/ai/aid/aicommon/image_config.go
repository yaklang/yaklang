package aicommon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func DefaultImageInputConfig() *ypb.AIImageInputConfig {
	return &ypb.AIImageInputConfig{Mode: "auto", VisualTokenLimit: 24000, MaxLongEdge: 1536, OutputReserveTokens: 4096, MaxRequestBytes: 24 * 1024 * 1024}
}

func WithImageInputMode(mode string, supportsImages ...bool) ConfigOption {
	return func(c *Config) error {
		policy := c.GetImageInputConfig()
		policy.Mode = mode
		if len(supportsImages) > 0 {
			policy.MainModelSupportsImages = proto.Bool(supportsImages[0])
		}
		return WithImageInput(policy)(c)
	}
}

func WithImageInput(config *ypb.AIImageInputConfig) ConfigOption {
	return func(c *Config) error {
		if config == nil {
			return nil
		}
		v := proto.Clone(config).(*ypb.AIImageInputConfig)
		d := DefaultImageInputConfig()
		if v.Mode == "" {
			v.Mode = d.Mode
		}
		if v.Mode != "auto" && v.Mode != "native" && v.Mode != "describe" {
			return fmt.Errorf("invalid image input mode %q", v.Mode)
		}
		if v.VisualTokenLimit == 0 {
			v.VisualTokenLimit = d.VisualTokenLimit
		}
		if v.MaxLongEdge == 0 {
			v.MaxLongEdge = d.MaxLongEdge
		}
		if v.OutputReserveTokens == 0 {
			v.OutputReserveTokens = d.OutputReserveTokens
		}
		if v.MaxRequestBytes == 0 {
			v.MaxRequestBytes = d.MaxRequestBytes
		}
		// MaxActiveImages is accepted only for configuration compatibility.
		// Timeline, not an independent active-image selector, owns retention.
		if v.VisualTokenLimit < 1 || v.MaxLongEdge < 64 || v.MaxLongEdge > 4096 || v.OutputReserveTokens < 0 || v.MaxRequestBytes < 1024 || v.MaxRequestBytes > 128*1024*1024 {
			return fmt.Errorf("invalid image input limits")
		}
		c.m.Lock()
		c.ImageInput = v
		c.m.Unlock()
		return nil
	}
}

func (c *Config) GetImageInputConfig() *ypb.AIImageInputConfig {
	if c == nil {
		return DefaultImageInputConfig()
	}
	// Some existing callers clone a literal Config before runtime setup.
	if c.m != nil {
		c.m.Lock()
		defer c.m.Unlock()
	}
	if c.ImageInput == nil {
		return DefaultImageInputConfig()
	}
	return proto.Clone(c.ImageInput).(*ypb.AIImageInputConfig)
}

// ResolveImageModel uses the same tier as the loop request. Capabilities must
// be explicit; a model name containing "vision" is not a capability contract.
func (c *Config) ResolveImageModel(lightweight bool) (*ypb.AIModelConfig, error) {
	c.m.Lock()
	custom := c.legacyAICallbackConfigured
	c.m.Unlock()
	if custom {
		return nil, nil
	}
	tier := consts.TierIntelligent
	if lightweight && !c.IsSingleAIModelMode() {
		tier = consts.TierLightweight
	}
	if !consts.IsTieredAIModelConfigEnabled() {
		return nil, nil
	}
	return resolveTierModelConfig(tier, c.IsSingleAIModelMode())
}

func (c *Config) GetImageCatalog() (*aimedia.Catalog, error) {
	state := c.GetSessionPromptState()
	workdir := c.GetOrCreateWorkDir()
	state.m.Lock()
	defer state.m.Unlock()
	if state.imageCatalog != nil {
		return state.imageCatalog, nil
	}
	key := c.PersistentSessionId
	if key == "" {
		key = c.Id
	}
	if state.imageNamespace != "" {
		key = state.imageNamespace
	}
	root := ImageCatalogDirectory(workdir, key)
	var err error
	if state.inheritedImageCatalog != nil {
		state.imageCatalog, err = state.inheritedImageCatalog.Fork(c.GetContext(), root)
	} else {
		state.imageCatalog, err = aimedia.OpenCatalog(root)
	}
	if err == nil {
		owner := c.PersistentSessionId
		if owner == "" {
			owner = c.Id
		}
		err = state.imageCatalog.BindOwner(owner)
		if err != nil {
			state.imageCatalog = nil
		}
	}
	return state.imageCatalog, err
}

// Pure-text sessions do not create a resource index or extra work directory.
func (c *Config) HasImageResources() bool {
	state := c.GetSessionPromptState()
	state.m.Lock()
	ready := state.imageCatalog != nil || state.inheritedImageCatalog != nil
	namespace := state.imageNamespace
	state.m.Unlock()
	if ready {
		return true
	}
	dir := c.Workdir
	if c.IsWorkDirReady() {
		dir = c.GetOrCreateWorkDir()
	}
	if dir == "" {
		return false
	}
	if namespace == "" {
		namespace = c.PersistentSessionId
		if namespace == "" {
			namespace = c.Id
		}
	}
	_, err := os.Stat(filepath.Join(ImageCatalogDirectory(dir, namespace), "index.json"))
	return err == nil || !os.IsNotExist(err)
}

func (c *Config) ImageScope() string {
	state := c.GetSessionPromptState()
	state.m.Lock()
	defer state.m.Unlock()
	if state.imageNamespace != "" {
		return state.imageNamespace
	}
	if c.PersistentSessionId != "" {
		return c.PersistentSessionId
	}
	return c.Id
}

func ImageCatalogDirectory(workdir, sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	return filepath.Join(workdir, ".ai_images", hex.EncodeToString(digest[:16]))
}

// ImageConfigFrom exposes optional capabilities without expanding the runtime
// interface implemented by external engines and existing test adapters.
func ImageConfigFrom(c AICallerConfigIf) *Config {
	switch v := c.(type) {
	case *Config:
		return v
	case *tierAwareConsumptionCaller:
		return v.Config
	default:
		return nil
	}
}

func ImportImageAttachment(ctx context.Context, c *Config, path, event string, ordinal ...int) (aimedia.Resource, error) {
	img, err := aimedia.FromFile(ctx, path)
	if err != nil {
		return aimedia.Resource{}, err
	}
	img.Source = aimedia.Source{Kind: "attachment", TurnID: event, ObservedAt: time.Now()}
	img.Source.OriginalName = filepath.Base(path)
	if len(ordinal) > 0 {
		img.Source.Ordinal = ordinal[0]
	}
	store, err := c.GetImageCatalog()
	if err != nil {
		return aimedia.Resource{}, err
	}
	return store.Register(ctx, event+":"+path, img)
}

func newImageNamespace() string { return uuid.NewString() }

func (c *Config) RegisterToolImages(ctx context.Context, task AITask, result *aitool.ToolResult) error {
	store, err := c.GetImageCatalog()
	if err != nil {
		return err
	}
	turn := ""
	if stateful, ok := task.(AIStatefulTask); ok {
		turn = stateful.GetId()
	}
	for i, img := range result.Images {
		owned := *img
		owned.Source.TurnID = turn
		o, err := store.Register(ctx, fmt.Sprintf("tool:%s:%d", result.ToolCallID, i), &owned)
		if err != nil {
			return err
		}
		result.Images[i] = o.Image
		if e := c.GetEmitter(); e != nil {
			_, _ = e.EmitJSON("structured", "image_context", map[string]any{"state": "registered", "observation": o, "scope": c.ImageScope()})
		}
	}
	return nil
}
