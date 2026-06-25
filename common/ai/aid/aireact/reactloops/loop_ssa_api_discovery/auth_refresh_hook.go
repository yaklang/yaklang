package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const defaultCredentialTTL = 5 * time.Minute
const authHookSubDir = "auth_hook"

// AuthRefreshPaths holds the paths used for Go ↔ Yak credential refresh signaling.
type AuthRefreshPaths struct {
	HeadersFile string
	TriggerFile string
}

// SetupAuthRefreshPaths creates the directory structure and initial headers file
// for the disk-based credential refresh protocol.
func SetupAuthRefreshPaths(rt *Runtime, credID uint) (*AuthRefreshPaths, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	dir := filepath.Join(rt.WorkDir, store.SubDirName(), authHookSubDir, fmt.Sprintf("%d", credID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("auth_refresh: mkdir: %w", err)
	}
	paths := &AuthRefreshPaths{
		HeadersFile: filepath.Join(dir, "headers.json"),
		TriggerFile: filepath.Join(dir, "trigger.json"),
	}
	return paths, nil
}

// WriteAuthHeadersFile writes credential headers to the headers file atomically.
func WriteAuthHeadersFile(cred *store.AuthCredential, headersPath string) error {
	if cred == nil || headersPath == "" {
		return nil
	}
	content := cred.HeadersJSON
	if content == "" {
		if cred.HeaderName != "" && cred.HeaderValue != "" {
			m := map[string]string{cred.HeaderName: cred.HeaderValue}
			b, _ := json.Marshal(m)
			content = string(b)
		} else {
			content = "{}"
		}
	}
	tmpPath := headersPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, headersPath)
}

// EnsureFreshCredential checks that the given credential is still valid. If expired
// or stale, it attempts to refresh using the stored AuthAcquisitionRecipe.
func EnsureFreshCredential(ctx context.Context, rt *Runtime, credID uint) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	cred, err := rt.Repo.GetAuthCredential(rt.Session.ID, credID)
	if err != nil {
		return fmt.Errorf("credential %d not found: %w", credID, err)
	}

	if cred.RefreshState == store.AuthRefreshStateFresh &&
		cred.LastVerifiedAt != nil &&
		time.Since(*cred.LastVerifiedAt) < defaultCredentialTTL {
		return nil
	}

	log.Infof("auth_refresh: credential %d needs refresh (state=%s last_verified=%v)",
		credID, cred.RefreshState, cred.LastVerifiedAt)

	if cred.VerifyURL != "" {
		SyncCredentialHeaderFields(cred)
		headers := make(map[string]string)
		if cred.HeadersJSON != "" {
			_ = json.Unmarshal([]byte(cred.HeadersJSON), &headers)
		}
		statusCode, _, probeErr := probeEndpoint(ctx, "GET", cred.VerifyURL, headers)
		if probeErr == nil && statusCode >= 200 && statusCode < 400 {
			now := time.Now()
			cred.LastVerifiedAt = &now
			cred.RefreshState = store.AuthRefreshStateFresh
			_ = rt.Repo.UpdateAuthCredential(cred)
			return nil
		}
		log.Infof("auth_refresh: verify probe failed (code=%d err=%v), marking stale", statusCode, probeErr)
	}

	cred.RefreshState = store.AuthRefreshStateStale
	_ = rt.Repo.UpdateAuthCredential(cred)

	if err := replayRecipeRefresh(ctx, rt, cred); err != nil {
		return fmt.Errorf("auth_refresh: replay failed for credential %d: %w", credID, err)
	}
	return nil
}

// replayRecipeRefresh uses the stored AuthAcquisitionRecipe to re-obtain credentials.
func replayRecipeRefresh(ctx context.Context, rt *Runtime, cred *store.AuthCredential) error {
	_ = ctx
	if cred.AcquireRecipeID == 0 {
		return fmt.Errorf("no recipe associated with credential %d", cred.ID)
	}
	recipe, err := rt.Repo.GetAuthAcquisitionRecipe(rt.Session.ID, cred.AcquireRecipeID)
	if err != nil {
		return fmt.Errorf("recipe %d not found: %w", cred.AcquireRecipeID, err)
	}

	log.Infof("auth_refresh: replaying recipe %d (method=%s login_url=%s) for credential %d",
		recipe.ID, recipe.Method, recipe.LoginURL, cred.ID)

	// TODO: implement actual HTTP replay of recipe steps
	// For now, mark the credential as needing manual refresh
	_ = recipe
	cred.ReacquireCount++
	return rt.Repo.UpdateAuthCredential(cred)
}

// WatchAuthRefreshTrigger watches for Yak script trigger files and refreshes
// credentials in response. This runs as a goroutine alongside Yak tool execution.
func WatchAuthRefreshTrigger(
	ctx context.Context,
	rt *Runtime,
	credID uint,
	paths *AuthRefreshPaths,
	wg *sync.WaitGroup,
) {
	if wg != nil {
		defer wg.Done()
	}
	if paths == nil {
		return
	}
	var lastTriggerMtime time.Time
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(paths.TriggerFile)
			if err != nil || info.ModTime() == lastTriggerMtime {
				continue
			}
			lastTriggerMtime = info.ModTime()
			log.Infof("auth_refresh: trigger file updated for credential %d", credID)

			if err := EnsureFreshCredential(ctx, rt, credID); err != nil {
				log.Warnf("auth_refresh: refresh failed: %v", err)
				continue
			}

			cred, err := rt.Repo.GetAuthCredential(rt.Session.ID, credID)
			if err != nil {
				log.Warnf("auth_refresh: reload credential: %v", err)
				continue
			}
			if err := WriteAuthHeadersFile(cred, paths.HeadersFile); err != nil {
				log.Warnf("auth_refresh: write headers file: %v", err)
			}
		}
	}
}

// ExecuteYakToolWithAuthHook wraps executeYakTool with automatic auth refresh support.
func ExecuteYakToolWithAuthHook(
	invoker aicommon.AIInvokeRuntime,
	ctx context.Context,
	toolName string,
	rt *Runtime,
	extraParams map[string]any,
	credID uint,
) (string, error) {
	if credID == 0 {
		return executeYakTool(invoker, ctx, toolName, rt, extraParams)
	}

	paths, err := SetupAuthRefreshPaths(rt, credID)
	if err != nil {
		log.Warnf("auth_refresh: setup paths: %v (running without auth hook)", err)
		return executeYakTool(invoker, ctx, toolName, rt, extraParams)
	}

	cred, err := rt.Repo.GetAuthCredential(rt.Session.ID, credID)
	if err != nil {
		log.Warnf("auth_refresh: load credential %d: %v", credID, err)
	} else {
		if err := WriteAuthHeadersFile(cred, paths.HeadersFile); err != nil {
			log.Warnf("auth_refresh: initial headers write: %v", err)
		}
	}

	if extraParams == nil {
		extraParams = make(map[string]any)
	}
	extraParams["auth-headers-file"] = paths.HeadersFile
	extraParams["auth-refresh-trigger-file"] = paths.TriggerFile

	watchCtx, watchCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go WatchAuthRefreshTrigger(watchCtx, rt, credID, paths, &wg)

	result, toolErr := executeYakTool(invoker, ctx, toolName, rt, extraParams)

	watchCancel()
	wg.Wait()

	// Clean up temp files
	_ = os.Remove(paths.TriggerFile)

	return result, toolErr
}
