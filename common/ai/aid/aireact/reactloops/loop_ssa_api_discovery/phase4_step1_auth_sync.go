package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

// runPhase4Step1AuthSync refreshes Phase1 credentials programmatically (no Step1 ReAct).
func runPhase4Step1AuthSync(ctx context.Context, r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	step := "phase4.step1.auth_sync"
	started := time.Now()
	rt.execStepStart(step, "programmatic")
	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	reportPath := pl.GetStep1AuthReportPath()
	if strings.TrimSpace(reportPath) == "" {
		reportPath = filepath.Join(dir, "step1_auth_result.md")
		pl.SetStep1AuthReportPath(reportPath)
	}

	creds, err := rt.Repo.ListVerifiedAuthCredentials(rt.Session.ID)
	if err != nil {
		rt.execStepError(step, "programmatic", started, err, nil)
		return err
	}
	var lines []string
	lines = append(lines, "# [阶段 4/5 - Step1] Phase4 Step1: API鉴权（复用 Phase1 凭证）", "")
	lines = append(lines, "本步骤**不重复登录**；仅同步/刷新 Phase1 已写入的 `auth_credentials`。", "")

	if len(creds) == 0 {
		lines = append(lines, "## 凭证状态", "", "（无已验证凭证；后续漏洞检测将以未登录状态继续。）", "")
		if rt.Session.TargetReachable && authLikelyRequired(rt) {
			_ = recordPipelineWaiver(rt, 4, waiverAuthCredentialsMissing, "Phase1 未写入 auth_credentials；Phase4 跳过重复鉴权")
			if r != nil {
				r.AddToTimeline("[ssa_phase4_step1]", "无 Phase1 凭证；已记录 waiver，不启动 Step1 ReAct")
			}
		}
	} else {
		lines = append(lines, "## 凭证清单（Phase1 同步）", "")
		lines = append(lines, "| id | realm | type | user | refresh | verified |", "|---|---|---|---|---|---|")
		for _, c := range creds {
			refreshNote := "ok"
			if ctx != nil {
				if rerr := EnsureFreshCredential(ctx, rt, c.ID); rerr != nil {
					refreshNote = "refresh_failed: " + rerr.Error()
					log.Warnf("ssa_api_discovery: phase4 step1 refresh cred %d: %v", c.ID, rerr)
				}
			}
			tokenHint := redactTokenHint(c)
			lines = append(lines, fmt.Sprintf("| %d | %s | %s | %s | %s | %v |",
				c.ID, c.AuthRealm, c.AuthType, c.Username, refreshNote, c.Verified))
			_ = tokenHint
		}
		lines = append(lines, "", "## 说明", "", "- 凭证来源：Phase1 鉴权校准/证据驱动登录", "- 本步仅调用 refresh hook，不重新探测登录面", "")
		if r != nil {
			r.AddToTimeline("[ssa_phase4_step1]", fmt.Sprintf("synced %d Phase1 credential(s)", len(creds)))
		}
	}

	body := strings.Join(lines, "\n")
	if err := os.WriteFile(reportPath, []byte(body), 0o644); err != nil {
		rt.execStepError(step, "programmatic", started, err, []string{reportPath})
		return err
	}
	rt.execStepEnd(step, "programmatic", started, []string{reportPath})
	return nil
}

func redactTokenHint(c store.AuthCredential) string {
	v := strings.TrimSpace(c.HeaderValue)
	if v == "" {
		v = strings.TrimSpace(c.TokenValue)
	}
	if len(v) <= 8 {
		return v
	}
	return v[:8] + "..."
}
