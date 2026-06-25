// seed_java_sqli_bench_session 为 java-sqli-bench 靶场生成 vuln_batch_scan 测试用 SQLite session。
//
//	go run ./common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/cmd/seed_java_sqli_bench_session \
//	  -workdir /tmp/vuln_batch_scan_bench8090 -base http://192.168.1.8:8090
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func main() {
	workDir := flag.String("workdir", "/tmp/vuln_batch_scan_bench8090", "任务目录（将创建 ssa_discovery/session.sqlite3）")
	baseRaw := flag.String("base", "http://192.168.1.8:8090", "靶场 base URL")
	uuidFixed := flag.String("uuid", "", "固定 discovery_sessions.uuid（空则随机）")
	force := flag.Bool("force", true, "覆盖已存在的 session.sqlite3")
	flag.Parse()

	baseURL, err := url.Parse(strings.TrimSpace(*baseRaw))
	if err != nil || baseURL.Host == "" {
		fmt.Fprintf(os.Stderr, "invalid base URL %q: %v\n", *baseRaw, err)
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
		CodeRootPath: "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/java-sqli-bench",
		CodePathOK:   true,
		TargetRaw:    strings.TrimRight(*baseRaw, "/"),
		TargetHost:   host,
		TargetPort:   port,
		TargetScheme: scheme,
		Language:     "java",
		Phase:        "ssa_done",
		Notes:        "seed: java-sqli-bench vuln links",
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

	routes := []struct {
		method, path, handler string
	}{
		{"GET", "/", "HomeController"},
		{"GET", "/api/users/search", "UserController.search"},
		{"POST", "/api/auth/login", "AuthController.login"},
		{"GET", "/api/orders/{orderId}", "OrderController.getOrder"},
		{"GET", "/api/products", "ProductController.listProducts"},
		{"GET", "/h2-console", "H2Console"},
	}
	for _, r := range routes {
		ep := &store.HttpEndpoint{
			SessionID:     sess.ID,
			Method:        r.method,
			PathPattern:   r.path,
			Source:        "java_sqli_bench_seed",
			Status:        store.EndpointStatusPendingValidation,
			HandlerClass:  r.handler,
			HandlerMethod: r.path,
		}
		if err := repo.CreateHttpEndpoint(ep); err != nil {
			fmt.Fprintf(os.Stderr, "CreateHttpEndpoint %s %s: %v\n", r.method, r.path, err)
			os.Exit(1)
		}
	}

	base := strings.TrimRight(*baseRaw, "/")
	verifiedSamples := []struct {
		method, path, sampleURL, queryParamsJSON, bodyHintJSON string
	}{
		{"GET", "/api/users/search", base + "/api/users/search?keyword=test", `[{"name":"keyword","example":"test"}]`, ""},
		{"GET", "/api/products", base + "/api/products?category=books", `[{"name":"category","example":"books"}]`, ""},
		{"GET", "/api/orders/{orderId}", base + "/api/orders/1", `[{"name":"orderId","example":"1","in":"path"}]`, ""},
		{"POST", "/api/auth/login", base + "/api/auth/login", "", `{"username":"admin","password":"Admin@2024!"}`},
	}
	for _, v := range verifiedSamples {
		if err := repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
			SessionID:       sess.ID,
			Method:          v.method,
			PathPattern:     v.path,
			FullSampleURL:   v.sampleURL,
			QueryParamsJSON: v.queryParamsJSON,
			BodyHintJSON:    v.bodyHintJSON,
			Verified:        true,
			Source:          "java_sqli_bench_seed",
		}); err != nil {
			fmt.Fprintf(os.Stderr, "UpsertVerifiedHttpApi %s %s: %v\n", v.method, v.path, err)
			os.Exit(1)
		}
	}
	sess.TargetReachable = true
	_ = repo.UpdateSession(sess)

	fmt.Printf("workdir=%s\n", *workDir)
	fmt.Printf("session_uuid=%s\n", sid)
	fmt.Printf("sqlite=%s\n", dbPath)
	fmt.Printf("bench_base_url=%s\n", strings.TrimRight(*baseRaw, "/"))
	fmt.Printf("endpoints=%d\n", len(routes))
}
