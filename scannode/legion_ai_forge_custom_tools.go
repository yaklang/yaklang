package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"github.com/yaklang/yaklang/scannode/inputresolver"
	"google.golang.org/protobuf/proto"
)

func contextForgeToolSHA256(tool *aiv1.ContextForgeTool) (string, error) {
	copy := proto.Clone(tool).(*aiv1.ContextForgeTool)
	copy.Sha256 = ""
	raw, err := (proto.MarshalOptions{Deterministic: true}).Marshal(copy)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func validateLegionForgeToolSnapshots(release *aiv1.ContextForgeRelease) error {
	snapshots := release.GetToolSnapshots()
	if len(snapshots) == 0 || len(snapshots) > maxLegionForgeTools {
		return fmt.Errorf("custom Forge requires 1-%d tool snapshots", maxLegionForgeTools)
	}
	names := map[string]bool{}
	declared := map[string]bool{}
	for _, name := range release.GetDeclaredToolNames() {
		declared[name] = true
	}
	previous, total := "", 0
	for _, tool := range snapshots {
		if tool == nil || tool.GetName() == "" || strings.TrimSpace(tool.GetName()) != tool.GetName() || tool.GetName() <= previous || len(tool.GetName()) > 256 || strings.TrimSpace(tool.GetToolId()) == "" || strings.TrimSpace(tool.GetOwnerUserId()) == "" {
			return fmt.Errorf("custom Forge tool identities must be complete, unique and name-sorted")
		}
		previous = tool.GetName()
		for _, managed := range legionForgeReportTools {
			if managed == tool.GetName() {
				return fmt.Errorf("custom tool cannot replace managed report tool %q", managed)
			}
		}
		if !declared[tool.GetName()] {
			return fmt.Errorf("undeclared custom tool %q", tool.GetName())
		}
		if strings.TrimSpace(tool.GetCode()) == "" || len(tool.GetCode()) > 256<<10 || len(tool.GetParamsJson()) > 64<<10 {
			return fmt.Errorf("custom Forge tool exceeds code/schema limit")
		}
		var definition map[string]any
		if json.Unmarshal(tool.GetParamsJson(), &definition) != nil || definition["type"] != "object" {
			return fmt.Errorf("custom Forge tool requires an object parameter schema")
		}
		total += len(tool.GetCode()) + len(tool.GetParamsJson())
		if total > 512<<10 {
			return fmt.Errorf("custom Forge tool snapshots exceed total size limit")
		}
		digest, err := contextForgeToolSHA256(tool)
		if err != nil || digest != tool.GetSha256() {
			return fmt.Errorf("custom Forge tool %q checksum mismatch", tool.GetName())
		}
		names[tool.GetName()] = true
	}
	for _, name := range legionForgeReportTools {
		names[name] = true
	}
	for name := range declared {
		if !names[name] {
			return fmt.Errorf("declared custom tool %q has no snapshot", name)
		}
	}
	return nil
}

// The inventory is release-scoped, not an OS sandbox. Original Yak code retains
// the runtime process's filesystem/network rights (tracked separately in #529).
func legionForgeCustomToolOptions(ctx context.Context, release *aiv1.ContextForgeRelease, workspace *inputresolver.Workspace) ([]aicommon.ConfigOption, error) {
	if err := validateLegionForgeToolSnapshots(release); err != nil {
		return nil, err
	}
	sources := make([]*schema.AIYakTool, 0, len(release.GetToolSnapshots()))
	for _, snapshot := range release.GetToolSnapshots() {
		sources = append(sources, &schema.AIYakTool{Name: snapshot.GetName(), Description: snapshot.GetDescription(), Content: snapshot.GetCode(), Params: string(snapshot.GetParamsJson())})
	}
	tools := yak.YakTool2AIToolWithForgeHandle(sources)
	if len(tools) != len(sources) {
		return nil, fmt.Errorf("custom Forge tool conversion lost a snapshot")
	}
	for i, tool := range tools {
		if tool == nil || tool.Name != sources[i].Name {
			return nil, fmt.Errorf("custom Forge tool conversion changed identity")
		}
		callback := tool.Callback
		tool.Callback = func(callCtx context.Context, params aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, stdout, stderr io.Writer) (any, error) {
			mapped, err := legionForgeCustomResourceParams(callCtx, release, workspace, params)
			if err != nil {
				return nil, err
			}
			return callback(callCtx, mapped, runtime, stdout, stderr)
		}
	}
	managed, err := legionForgeReportToolObjects(ctx, release, workspace)
	if err != nil {
		return nil, err
	}
	declared := map[string]bool{}
	for _, name := range release.GetDeclaredToolNames() {
		declared[name] = true
	}
	for _, tool := range managed {
		if declared[tool.Name] {
			tools = append(tools, tool)
		}
	}
	return restrictedLegionForgeToolOptions(tools), nil
}

// Translate only resources actually pinned by this release. The script receives
// a real runtime-local filename; prompts and immutable snapshots keep portable
// relative names. This mapping is not a restriction on what arbitrary Yak can do.
func legionForgeCustomResourceParams(ctx context.Context, release *aiv1.ContextForgeRelease, workspace *inputresolver.Workspace, params aitool.InvokeParams) (aitool.InvokeParams, error) {
	paths, err := legionForgeResourcePaths(release)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return params, nil
	}
	if workspace == nil {
		return nil, fmt.Errorf("custom Forge input workspace unavailable")
	}
	available := map[string]bool{}
	for _, file := range workspace.Files() {
		available[file.RelativePath] = true
	}
	resolved := map[string]string{}
	for _, path := range paths {
		if !available[path] || !inputresolver.SafeInputPath(path) {
			return nil, fmt.Errorf("custom Forge resource binding unavailable")
		}
		// Read validates the lease, cancellation and no-follow readonly file
		// before passing its materialized path to the original script.
		if _, err := workspace.Read(ctx, path, 0, 1); err != nil {
			return nil, err
		}
		resolved[path] = filepath.Join(workspace.RootForDiagnostics(), filepath.FromSlash(path))
	}
	var mapValue func(any) any
	mapValue = func(value any) any {
		switch v := value.(type) {
		case string:
			if path, ok := resolved[v]; ok {
				return path
			}
		case []any:
			out := make([]any, len(v))
			for i, item := range v {
				out[i] = mapValue(item)
			}
			return out
		case map[string]any:
			out := map[string]any{}
			for key, item := range v {
				out[key] = mapValue(item)
			}
			return out
		}
		return value
	}
	result := aitool.InvokeParams{}
	for key, value := range params {
		result[key] = mapValue(value)
	}
	return result, nil
}
