package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

// BaseCalibrationReport base URL / context 组合探测评分。
type BaseCalibrationReport struct {
	GeneratedAt time.Time `json:"generated_at"`
	TargetBase  string    `json:"target_base"`
	Variants    []struct {
		Base        string `json:"base"`
		Score       int    `json:"score"`
		BestURL     string `json:"best_sample_url,omitempty"`
		BestStatus  int    `json:"best_status,omitempty"`
	} `json:"variants"`
	Warnings []string `json:"warnings,omitempty"`
}

// RunApiBaseCalibrator 对 Target base 与预分析中的 context-path 候选做轻量 HEAD 探测并落盘。
func RunApiBaseCalibrator(rt *Runtime) (*BaseCalibrationReport, error) {
	if rt == nil || rt.Session == nil || rt.Repo == nil {
		return nil, utils.Error("nil runtime")
	}
	sess := rt.Session
	base := strings.TrimSpace(EffectiveTargetBaseURL(sess))
	rep := &BaseCalibrationReport{GeneratedAt: time.Now().UTC(), TargetBase: base}
	if base == "" {
		rep.Warnings = append(rep.Warnings, "no target_base_url; skip calibration")
		b, _ := json.Marshal(rep)
		sess.ApiBaseCalibrationMetaJSON = string(b)
		_ = rt.Repo.UpdateSession(sess)
		rt.Session = sess
		return rep, nil
	}

	variants := []string{strings.TrimRight(base, "/")}
	if pre := loadPreanalysisFull(rt.WorkDir); pre != nil {
		seen := map[string]struct{}{variants[0]: {}}
		for _, c := range pre.ConfigBaseCandidates {
			v := strings.TrimSpace(c.Value)
			if v == "" || v == "/" {
				continue
			}
			if !strings.HasPrefix(v, "/") {
				v = "/" + v
			}
			tb := strings.TrimRight(base, "/")
			cand := tb + v
			if _, ok := seen[cand]; ok {
				continue
			}
			seen[cand] = struct{}{}
			variants = append(variants, cand)
		}
	}

	pathSamples := []string{"/", "/health", "/actuator/health", "/api", "/swagger-ui.html", "/v3/api-docs"}
	if eps, err := rt.Repo.ListHttpEndpoints(sess.ID); err == nil && len(eps) > 0 {
		n := 0
		for _, e := range eps {
			pp := strings.TrimSpace(e.PathPattern)
			if pp == "" || pp == "/" || strings.Contains(pp, "*") {
				continue
			}
			if !strings.HasPrefix(pp, "/") {
				pp = "/" + pp
			}
			pathSamples = append(pathSamples, pp)
			n++
			if n >= 6 {
				break
			}
		}
	}

	client := &http.Client{Timeout: 8 * time.Second}
	ctx := context.Background()
	scores := make(map[string]int)
	bestURL := make(map[string]string)
	bestStatus := make(map[string]int)

	for _, v := range variants {
		for _, p := range pathSamples {
			if len(p) > 512 {
				continue
			}
			u := strings.TrimRight(v, "/") + p
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
			if err != nil {
				continue
			}
			resp, err := client.Do(req)
			sc := 0
			if err == nil && resp != nil {
				sc = resp.StatusCode
				resp.Body.Close()
			}
			scores[v] += baseProbeScoreDelta(sc)
			if sc > 0 && (bestStatus[v] == 0 || baseBetterStatus(sc, bestStatus[v])) {
				bestStatus[v] = sc
				bestURL[v] = u
			}
		}
	}

	for _, v := range variants {
		s := scores[v]
		rep.Variants = append(rep.Variants, struct {
			Base       string `json:"base"`
			Score      int    `json:"score"`
			BestURL    string `json:"best_sample_url,omitempty"`
			BestStatus int    `json:"best_status,omitempty"`
		}{Base: v, Score: s, BestURL: bestURL[v], BestStatus: bestStatus[v]})
	}

	sort.Slice(rep.Variants, func(i, j int) bool { return rep.Variants[i].Score > rep.Variants[j].Score })

	outPath := store.ApiBaseCalibrationReportPath(rt.WorkDir)
	_ = os.MkdirAll(filepath.Dir(outPath), 0o755)
	if b, err := json.MarshalIndent(rep, "", "  "); err == nil {
		_ = os.WriteFile(outPath, b, 0o644)
	}

	sum := map[string]any{
		"full_report_path": outPath,
		"target_base":      base,
		"variant_count":    len(rep.Variants),
	}
	if len(rep.Variants) > 0 {
		sum["top_variant"] = rep.Variants[0].Base
		sum["top_score"] = rep.Variants[0].Score
	}
	sumB, _ := json.Marshal(sum)
	sess.ApiBaseCalibrationMetaJSON = string(sumB)
	if err := rt.Repo.UpdateSession(sess); err != nil {
		return rep, err
	}
	rt.Session = sess
	return rep, nil
}

func baseProbeScoreDelta(sc int) int {
	if sc >= 200 && sc < 400 || sc == 401 || sc == 403 || sc == 405 {
		return 4
	}
	if sc == 404 {
		return -1
	}
	if sc > 0 {
		return 1
	}
	return 0
}

func baseBetterStatus(cur, prev int) bool {
	if isGood(cur) && !isGood(prev) {
		return true
	}
	if !isGood(cur) && isGood(prev) {
		return false
	}
	return cur < prev && cur > 0
}

func isGood(sc int) bool {
	return sc >= 200 && sc < 400 || sc == 401 || sc == 403 || sc == 405
}

func loadPreanalysisFull(workDir string) *APIPreanalysisReport {
	p := store.ApiPreanalysisReportPath(workDir)
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return nil
	}
	var r APIPreanalysisReport
	if err := json.Unmarshal(b, &r); err != nil {
		return nil
	}
	return &r
}
