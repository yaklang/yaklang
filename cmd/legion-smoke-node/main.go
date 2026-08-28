package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/spec"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/scannode"
)

type staticHostIdentityProvider struct {
	identity node.HostIdentity
}

func (p staticHostIdentityProvider) Snapshot() node.HostIdentity {
	return p.identity
}

func main() {
	if err := run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if shouldRunDistYak(args) {
		return runDistYak(args)
	}
	return runNode(args[1:])
}

func shouldRunDistYak(args []string) bool {
	return len(args) > 1 && args[1] == scannode.DistYakCommand.Name
}

func runDistYak(args []string) error {
	app := cli.NewApp()
	app.Commands = []cli.Command{scannode.DistYakCommand}
	return app.Run(args)
}

func runNode(args []string) error {
	flags := flag.NewFlagSet("legion-smoke-node", flag.ContinueOnError)
	apiURL := flags.String("api-url", "http://127.0.0.1:8080", "Legion platform HTTP API base URL")
	runtimeAPIURL := flags.String("runtime-api-url", strings.TrimSpace(os.Getenv("LEGION_RUNTIME_API_URL")), "Container-facing Legion platform HTTP API base URL for Runtime Host commands")
	enrollmentToken := flags.String("enrollment-token", "", "Legion node enrollment token")
	nodeID := flags.String("id", "", "Legacy node ID fallback; canonical node_id is assigned by platform")
	displayName := flags.String("name", "smoke-node", "Display name reported to Legion")
	agentInstallationID := flags.String("agent-installation-id", "", "Override persisted agent installation ID")
	kind := flags.String("kind", "", "Node kind: empty/host=host node, ai_session=AI session container node")
	dockerEndpoint := flags.String("docker-endpoint", strings.TrimSpace(os.Getenv("LEGION_DOCKER_ENDPOINT")), "Session-Manager-reachable Docker endpoint advertised by a host node")
	runtimeHost := flags.Bool("runtime-host", environmentBool("LEGION_RUNTIME_HOST"), "Allow this node to run AI session containers using its local Docker Engine")
	runtimeNetwork := flags.String("runtime-network", environmentValue("LEGION_RUNTIME_NETWORK", "bridge"), "Fixed local Docker network for AI session containers")
	baseDir := flags.String("base-dir", "", "Node local state base directory")
	ruleSnapshotCacheDir := flags.String(
		"rule-snapshot-cache-dir",
		strings.TrimSpace(os.Getenv("LEGION_RULE_SNAPSHOT_CACHE_DIR")),
		"Immutable rule snapshot bundle cache directory",
	)
	version := flags.String("version", "smoke", "Node version")
	engineReleaseID := flags.String("engine-release-id", strings.TrimSpace(os.Getenv("ENGINE_RELEASE_ID")), "Installed unified Yaklang release ID")
	engineDigest := flags.String("engine-digest", strings.TrimSpace(os.Getenv("ENGINE_RELEASE_SHA256")), "Installed Yaklang Node binary SHA-256")
	heartbeatInterval := flags.Duration(
		"heartbeat-interval",
		node.DefaultHeartbeatInterval,
		"Heartbeat interval",
	)
	maxRunningJobs := smokeNodeMaxRunningJobsFlag(flags)
	pprofAddr := flags.String("pprof-addr", "", "Optional pprof HTTP listen address, e.g. 127.0.0.1:18080")
	heapMonitorInterval := flags.Duration(
		"heap-monitor-interval",
		0,
		"Optional heap monitor interval; zero disables periodic heap logging/dumps",
	)
	heapDumpThresholdMB := flags.Uint64(
		"heap-dump-threshold-mb",
		0,
		"Heap dump/log threshold in MB; zero means log every monitor interval",
	)
	heapDumpDir := flags.String(
		"heap-dump-dir",
		"",
		"Optional directory for periodic heap profile dumps; empty means log-only monitoring",
	)
	heapDumpCount := flags.Int(
		"heap-dump-count",
		0,
		"Optional maximum number of periodic heap dumps; zero means unlimited while monitoring stays enabled",
	)
	heapMonitorRuntimeGC := flags.Bool(
		"heap-monitor-runtime-gc",
		true,
		"Whether heap monitor snapshots should force runtime.GC before the second sample",
	)
	hostMachineID := flags.String(
		"host-machine-id",
		"",
		"Optional host identity machine_id override for isolated local testing",
	)
	hostSystemUUID := flags.String(
		"host-system-uuid",
		"",
		"Optional host identity system_uuid override for isolated local testing",
	)
	hostInstanceID := flags.String(
		"host-instance-id",
		"",
		"Optional host identity instance_id override for isolated local testing",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedMaxRunningJobs, err := smokeNodeMaxRunningJobs(*maxRunningJobs)
	if err != nil {
		return err
	}
	if err := initializeAISessionCapabilities(*kind, syncAISessionCapabilities); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if addr := strings.TrimSpace(*pprofAddr); addr != "" {
		diagnostics.StartPprofServer(addr)
	}

	var stopHeapMonitor context.CancelFunc
	if *heapMonitorInterval > 0 {
		options := []diagnostics.HeapDumpOption{
			diagnostics.WithName(strings.TrimSpace(*displayName)),
			diagnostics.WithHTTPServer(strings.TrimSpace(*pprofAddr)),
			diagnostics.WithDumpCount(*heapDumpCount),
			diagnostics.WithRuntimeGC(*heapMonitorRuntimeGC),
		}
		if *heapDumpThresholdMB > 0 {
			options = append(options, diagnostics.WithHeapLimit(*heapDumpThresholdMB*1024*1024))
		}
		if dir := strings.TrimSpace(*heapDumpDir); dir != "" {
			options = append(options, diagnostics.WithDumpDir(dir))
		} else {
			options = append(options, diagnostics.WithDisable(true))
		}
		stopHeapMonitor = diagnostics.StartHeapMonitor(*heapMonitorInterval, options...)
		defer stopHeapMonitor()
	}

	var hostIdentityProvider node.HostIdentityProvider
	if strings.TrimSpace(*hostMachineID) != "" ||
		strings.TrimSpace(*hostSystemUUID) != "" ||
		strings.TrimSpace(*hostInstanceID) != "" {
		hostIdentityProvider = staticHostIdentityProvider{
			identity: node.HostIdentity{
				MachineID:  strings.TrimSpace(*hostMachineID),
				SystemUUID: strings.TrimSpace(*hostSystemUUID),
				InstanceID: strings.TrimSpace(*hostInstanceID),
			},
		}
	}

	// ai_session 容器 runtime：bootstrap 成功后回调 sessionmgr register 端点上报 node id。
	var postBootstrapHook func(node.SessionState)
	if strings.TrimSpace(*kind) == "ai_session" {
		postBootstrapHook = buildAISessionRegisterHook()
	}

	scanNodeOptions := []scannode.ScanNodeOption{
		scannode.WithRuleSnapshotCacheDir(*ruleSnapshotCacheDir),
	}
	if *runtimeHost && strings.TrimSpace(*kind) != "ai_session" {
		scanNodeOptions = append(scanNodeOptions, scannode.WithRuntimeHost(scannode.RuntimeHostConfig{
			Enabled: true, PlatformAPIBaseURL: *apiURL, RuntimePlatformAPIBaseURL: *runtimeAPIURL, EnrollmentToken: *enrollmentToken,
			AgentInstallationID: *agentInstallationID, Network: *runtimeNetwork,
			EngineReleaseID: *engineReleaseID, EngineDigest: *engineDigest,
		}))
	}
	scanNode, err := scannode.NewScanNode(node.BaseConfig{
		NodeType:             spec.NodeType_Scanner,
		Kind:                 strings.TrimSpace(*kind),
		DockerEndpoint:       hostDockerEndpoint(*kind, *dockerEndpoint),
		NodeID:               *nodeID,
		DisplayName:          *displayName,
		AgentInstallationID:  *agentInstallationID,
		BaseDir:              *baseDir,
		EnrollmentToken:      *enrollmentToken,
		PlatformAPIBaseURL:   *apiURL,
		Version:              *version,
		EngineReleaseID:      hostEngineValue(*kind, *engineReleaseID),
		EngineDigest:         hostEngineValue(*kind, *engineDigest),
		HeartbeatInterval:    *heartbeatInterval,
		MaxRunningJobs:       resolvedMaxRunningJobs,
		HostIdentityProvider: hostIdentityProvider,
		PostBootstrapHook:    postBootstrapHook,
	}, scanNodeOptions...)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanNode.Run()
	}()

	<-ctx.Done()
	scanNode.Shutdown()
	<-done
	return nil
}

func smokeNodeMaxRunningJobsFlag(flags *flag.FlagSet) *uint64 {
	return flags.Uint64(
		"max-running-jobs",
		1,
		"Maximum concurrent jobs; zero means unlimited",
	)
}

func smokeNodeMaxRunningJobs(value uint64) (uint32, error) {
	if value > uint64(^uint32(0)) {
		return 0, fmt.Errorf("max-running-jobs exceeds uint32 range")
	}
	return uint32(value), nil
}

func environmentBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func environmentValue(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func hostDockerEndpoint(kind, endpoint string) string {
	if strings.TrimSpace(kind) == "ai_session" {
		return ""
	}
	return strings.TrimSpace(endpoint)
}

func hostEngineValue(kind, value string) string {
	if strings.TrimSpace(kind) == "ai_session" {
		return ""
	}
	return strings.TrimSpace(value)
}

// initializeAISessionCapabilities keeps host Scan Nodes on their existing
// startup path while making the ephemeral AI session runtime self-contained.
// The full Yakit post-init pipeline also imports unrelated plugins and rules;
// synchronizing only the capability stores keeps session startup bounded.
func initializeAISessionCapabilities(kind string, syncFn func() error) error {
	if strings.TrimSpace(kind) != "ai_session" {
		return nil
	}
	if syncFn == nil {
		return fmt.Errorf("initialize AI session capabilities: sync function is nil")
	}
	if err := syncFn(); err != nil {
		return fmt.Errorf("initialize AI session capabilities: %w", err)
	}
	return nil
}

func syncAISessionCapabilities() error {
	// Capability search reads the profile database, while the tool manager also
	// keeps an in-memory registry. Ephemeral session containers previously only
	// populated the latter, so query_capabilities saw zero tools and blueprints.
	yakscripttools.OverrideYakScriptAiTools()
	yakscripttools.UpdateAIYakToolAIOutputOption()

	db := consts.GetGormProfileDatabase()
	toolCount, err := yakit.CountAIYakTools(db, nil)
	if err != nil {
		return fmt.Errorf("verify built-in AI tools: %w", err)
	}
	if toolCount == 0 {
		return fmt.Errorf("verify built-in AI tools: capability store is empty")
	}

	if err := aiforge.ForceSyncBuildInForge(); err != nil {
		return fmt.Errorf("sync built-in AI blueprints: %w", err)
	}
	if _, err := yakit.GetAIForgeByName(db, "hostscan"); err != nil {
		return fmt.Errorf("verify built-in AI blueprint hostscan: %w", err)
	}
	return nil
}

// buildAISessionRegisterHook 构造 ai_session 容器 runtime 的 bootstrap 回调：
// bootstrap 成功后 POST sessionmgr 的 register 端点上报 {node_id, node_session_id}。
// sessionmgr 地址从 LEGION_SESSIONMGR_URL 环境变量取，sessionID 从 LEGION_AI_SESSION_ID 取
// （由 sessionmgr 起容器时注入）。
func buildAISessionRegisterHook() func(node.SessionState) {
	sessionmgrURL := strings.TrimSpace(os.Getenv("LEGION_SESSIONMGR_URL"))
	sessionID := strings.TrimSpace(os.Getenv("LEGION_AI_SESSION_ID"))
	if sessionmgrURL == "" || sessionID == "" {
		log.Printf("ai_session register hook disabled: LEGION_SESSIONMGR_URL or LEGION_AI_SESSION_ID not set")
		return nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	return func(session node.SessionState) {
		body, err := json.Marshal(map[string]string{
			"node_id":         session.NodeID,
			"node_session_id": session.SessionID,
		})
		if err != nil {
			log.Printf("ai_session register marshal: %v", err)
			return
		}
		url := strings.TrimRight(sessionmgrURL, "/") + "/v1/sessionmgr/sessions/" + sessionID + "/register"
		resp, err := client.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("ai_session register callback to %s: %v", url, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			log.Printf("ai_session register callback %s returned status %d", url, resp.StatusCode)
			return
		}
		log.Printf("ai_session registered: node_id=%s session_id=%s", session.NodeID, session.SessionID)
	}
}
