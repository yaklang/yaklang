// seed_benchmark_discovery_session 生成用于联调 endpoint_batch_probe 的模拟 discovery SQLite（靶场 vuln-detect-benchmark + 嵌入式 Cookie）。
//
// 用法（在 yaklang 仓库根目录）：
//
//	go run ./common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/cmd/seed_benchmark_discovery_session -workdir /path/to/task
//
// 环境变量 BENCH_BASE_URL 默认 http://127.0.0.1:8777（与靶场默认 Listen 一致；也可用 BENCH_ADDR=:18779 起靶场后设 export BENCH_BASE_URL=http://127.0.0.1:18779）。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

const benchCodeRoot = "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/vuln-detect-benchmark/src"

// 与 benchmark main.go 中 benchEmbeddedDevCookie 一致。
const benchEmbeddedDevCookie = "bench_dev_static_session_7a3f9c2e"

func main() {
	workDir := flag.String("workdir", "/home/murkfox/yakit-projects/aispace/bench_vuln_detect_batch_probe", "任务目录（将创建 ssa_discovery/session.sqlite3）")
	uuidFixed := flag.String("uuid", "", "固定 discovery_sessions.uuid（空则随机）")
	force := flag.Bool("force", true, "若 true，覆盖已存在的 session.sqlite3")
	flag.Parse()

	baseRaw := os.Getenv("BENCH_BASE_URL")
	if baseRaw == "" {
		baseRaw = "http://127.0.0.1:8777"
	}
	baseURL, err := url.Parse(baseRaw)
	if err != nil || baseURL.Host == "" {
		fmt.Fprintf(os.Stderr, "invalid BENCH_BASE_URL %q: %v\n", baseRaw, err)
		os.Exit(1)
	}
	scheme := baseURL.Scheme
	if scheme == "" {
		scheme = "http"
	}
	host := baseURL.Hostname()
	port := baseURL.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	dbPath := store.DBPath(*workDir)
	if *force {
		_ = os.Remove(dbPath)
	}

	db, err := store.OpenSessionDB(*workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OpenSessionDB: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if sqlDB := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	repo := store.NewRepository(db)
	sid := strings.TrimSpace(*uuidFixed)
	if sid == "" {
		sid = uuid.NewString()
	}

	sess := &store.DiscoverySession{
		UUID:         sid,
		CodeRootPath: benchCodeRoot,
		CodePathOK:   true,
		TargetRaw:    baseRaw,
		TargetHost:   host,
		TargetPort:   port,
		TargetScheme: scheme,
		Language:     "golang",
		Phase:        "ssa_done",
		Notes:        "seed: vuln-detect-benchmark + embedded Cookie",
	}
	if err := repo.CreateSession(sess); err != nil {
		fmt.Fprintf(os.Stderr, "CreateSession: %v\n", err)
		os.Exit(1)
	}
	sess, err = repo.GetSessionByUUID(sid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetSessionByUUID: %v\n", err)
		os.Exit(1)
	}

	headers := map[string]string{"Cookie": "bench_session=" + benchEmbeddedDevCookie}
	headersJSON, _ := json.Marshal(headers)
	now := time.Now()
	ac := &store.AuthCredential{
		SessionID:      sess.ID,
		AuthType:       "cookie_session",
		Username:       "embedded_bench",
		HeadersJSON:    string(headersJSON),
		Verified:       true,
		VerifyURL:      baseRaw + "/api/me",
		Notes:          "vuln-detect-benchmark benchEmbeddedDevCookie (no POST /api/login)",
		LastVerifiedAt: &now,
	}
	if err := repo.CreateAuthCredential(ac); err != nil {
		fmt.Fprintf(os.Stderr, "CreateAuthCredential: %v\n", err)
		os.Exit(1)
	}

	if err := writeAuthHookHeaders(*workDir, ac.ID, headersJSON); err != nil {
		fmt.Fprintf(os.Stderr, "auth_hook headers: %v\n", err)
		os.Exit(1)
	}

	routes := []struct{ method, path string }{
		{"GET", "/"},
		{"GET", "/api/me"},
		{"GET", "/api/user"},
		{"GET", "/api/search"},
		{"GET", "/api/ping"},
		{"GET", "/api/fetch"},
		{"GET", "/api/file"},
		{"GET", "/api/render"},
		{"POST", "/api/xml"},
		{"POST", "/api/deserialize"},
		{"GET", "/api/include"},
		{"POST", "/api/login"},
		{"GET", "/admin"},
		{"GET", "/actuator/env"},
	}
	for _, r := range routes {
		ep := &store.HttpEndpoint{
			SessionID:     sess.ID,
			Method:        r.method,
			PathPattern:   r.path,
			Source:        "benchmark_seed",
			Status:        store.EndpointStatusPendingValidation,
			HandlerClass:  "main",
			HandlerMethod: r.path,
		}
		if err := repo.CreateHttpEndpoint(ep); err != nil {
			fmt.Fprintf(os.Stderr, "CreateHttpEndpoint %s %s: %v\n", r.method, r.path, err)
			os.Exit(1)
		}
	}

	fmt.Printf("workdir=%s\n", *workDir)
	fmt.Printf("session_uuid=%s\n", sid)
	fmt.Printf("sqlite=%s\n", dbPath)
	fmt.Printf("bench_base_url=%s (override with BENCH_BASE_URL)\n", baseRaw)
	fmt.Printf("auth_credential_id=%d\n", ac.ID)
}

func writeAuthHookHeaders(workDir string, credID uint, headersJSON []byte) error {
	dir := filepath.Join(workDir, store.SubDirName(), "auth_hook", fmt.Sprintf("%d", credID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "headers.json"), headersJSON, 0o644)
}
