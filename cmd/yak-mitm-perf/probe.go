package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/gorm"
	"google.golang.org/grpc"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type probeEnvironment struct {
	client     ypb.YakClient
	server     *yakgrpc.Server
	grpcServer *grpc.Server
	connection *grpc.ClientConn
	listener   net.Listener
	tempDir    string
	upstream   *httptest.Server
}

type scenarioConfig struct {
	name              string
	disableTraffic    bool
	realtimeQueries   bool
	cancelBeforeDrain bool
	requests          int
	concurrency       int
	queryHz           int
	timeout           time.Duration
}

type scenarioResult struct {
	warmupRequestMS    float64
	requestLatenciesMS []float64
	queryLatenciesMS   []float64
	requestElapsed     time.Duration
	persistElapsed     time.Duration
	postStopDrain      time.Duration
	shutdownElapsed    time.Duration
	requestErrors      int64
	persisted          int
	persistedAtStop    int
	queuePeak          int64
	queueAtStop        int64
	goroutinePeakDelta int64
	goroutineAfter     int64
	queryPeakInFlight  int64
	queryErrors        int64
	pushMessages       int64
	systemTimingCount  int64
	systemTiming       pipelineTimingSamples
	pushDeliveryMS     []float64
	cpuBusySeconds     float64
	cpuWallElapsed     time.Duration
	postShutdownCPU    float64
}

type pipelineTimingSamples struct {
	backendQueryMS            []float64
	backendCountMS            []float64
	backendDataQueryMS        []float64
	backendConversionMS       []float64
	countExecuted             int64
	asyncWriteQueueDepth      []float64
	persistQueueWaitMS        []float64
	persistWriteMS            []float64
	databaseChangeDetectionMS []float64
	requestToFlowBuiltMS      []float64
	responseToFlowBuiltMS     []float64
	requestToProbeReceiveMS   []float64
	responseToProbeReceiveMS  []float64
	persistToProbeReceiveMS   []float64
	observedFlowCount         int64
}

func runProbe(r *report, cfg profileConfig) error {
	env, err := newProbeEnvironment(cfg.BodyBytes)
	if err != nil {
		return err
	}
	defer env.close()

	r.Config["requests"] = cfg.Requests
	r.Config["concurrency"] = cfg.Concurrency
	r.Config["body_bytes"] = cfg.BodyBytes
	r.Config["seed_rows"] = cfg.SeedRows
	r.Config["repetitions"] = cfg.Repetitions
	r.Config["query_hz"] = cfg.QueryHz

	if err := recordDatabaseReadMetrics(r, env.client, "fresh", cfg.QuerySamples); err != nil {
		return err
	}
	if cfg.SeedRows > 0 {
		if err := seedHTTPFlows(env.server.GetProjectDatabase(), cfg.SeedRows); err != nil {
			return fmt.Errorf("seed HTTP flows: %w", err)
		}
	}
	if err := recordDatabaseReadMetrics(r, env.client, fmt.Sprintf("seeded_%d", cfg.SeedRows), cfg.QuerySamples); err != nil {
		return err
	}

	scenarios := []scenarioConfig{
		{
			name:            "baseline",
			realtimeQueries: true,
			requests:        cfg.Requests,
			concurrency:     cfg.Concurrency,
			queryHz:         cfg.QueryHz,
			timeout:         cfg.ScenarioTimeout,
		},
		{
			name:            "trafficguard_off",
			disableTraffic:  true,
			realtimeQueries: true,
			requests:        cfg.Requests,
			concurrency:     cfg.Concurrency,
			queryHz:         cfg.QueryHz,
			timeout:         cfg.ScenarioTimeout,
		},
		{
			name:        "realtime_off",
			requests:    cfg.Requests,
			concurrency: cfg.Concurrency,
			queryHz:     cfg.QueryHz,
			timeout:     cfg.ScenarioTimeout,
		},
		{
			name:              "shutdown_backlog",
			realtimeQueries:   true,
			cancelBeforeDrain: true,
			requests:          cfg.Requests * 2,
			concurrency:       cfg.Concurrency,
			queryHz:           cfg.QueryHz,
			timeout:           cfg.ScenarioTimeout,
		},
	}

	for _, scenario := range scenarios {
		var runs []scenarioResult
		for repeat := 0; repeat < cfg.Repetitions; repeat++ {
			result, err := runScenario(env, scenario, repeat)
			if err != nil {
				r.addCheck("mitm."+scenario.name+".completed", "fail", "mitm_pipeline", err.Error())
				return err
			}
			runs = append(runs, result)
		}
		recordScenarioMetrics(r, scenario, runs)
	}
	return nil
}

func newProbeEnvironment(bodyBytes int) (*probeEnvironment, error) {
	tempDir, err := os.MkdirTemp("", "yak-mitm-perf-")
	if err != nil {
		return nil, err
	}
	cleanupOnError := func() { _ = os.RemoveAll(tempDir) }

	profileDBPath := filepath.Join(tempDir, "profile.db")
	projectDBPath := filepath.Join(tempDir, "project.db")
	// A brand-new profile DB normally triggers the production engine-version
	// cache cleanup. Seed the current version so this isolated benchmark never
	// touches the user's shared yakit-projects/temp directory.
	profileDB, err := consts.CreateProfileDatabase(profileDBPath)
	if err != nil {
		cleanupOnError()
		return nil, err
	}
	if err := yakit.SetKey(profileDB, "_YAKLANG_ENGINE_VERSION", consts.GetYakVersion()); err != nil {
		_ = profileDB.Close()
		cleanupOnError()
		return nil, err
	}
	if err := profileDB.Close(); err != nil {
		cleanupOnError()
		return nil, err
	}
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
		yakgrpc.WithProfileDatabasePath(profileDBPath),
		yakgrpc.WithProjectDatabasePath(projectDBPath),
	)
	if err != nil {
		cleanupOnError()
		return nil, err
	}
	// The async HTTPFlow writer resolves the project DB through consts, so bind the
	// benchmark server's temporary handles before any traffic is admitted.
	consts.BindProfileDatabase(server.GetProfileDatabase(), profileDBPath)
	consts.BindProjectDatabaseWithReader(server.GetProjectDatabase(), server.GetProjectReadDatabase(), projectDBPath)

	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)
	ypb.RegisterYakServer(grpcServer, server)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cleanupOnError()
		return nil, err
	}
	go func() { _ = grpcServer.Serve(listener) }()

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dialCancel()
	connection, err := grpc.DialContext(dialCtx, listener.Addr().String(), grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100*1024*1024),
			grpc.MaxCallSendMsgSize(100*1024*1024),
		))
	if err != nil {
		grpcServer.Stop()
		_ = listener.Close()
		cleanupOnError()
		return nil, err
	}

	body := makeSafeResponseBody(bodyBytes)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = writer.Write(body)
	}))

	return &probeEnvironment{
		client:     ypb.NewYakClient(connection),
		server:     server,
		grpcServer: grpcServer,
		connection: connection,
		listener:   listener,
		tempDir:    tempDir,
		upstream:   upstream,
	}, nil
}

func (e *probeEnvironment) close() {
	if e.upstream != nil {
		e.upstream.Close()
	}
	if e.connection != nil {
		_ = e.connection.Close()
	}
	if e.grpcServer != nil {
		e.grpcServer.Stop()
	}
	if e.listener != nil {
		_ = e.listener.Close()
	}
	if e.server != nil {
		if db := e.server.GetProjectDatabase(); db != nil {
			_ = db.Close()
		}
		if db := e.server.GetProfileDatabase(); db != nil {
			_ = db.Close()
		}
	}
	if e.tempDir != "" {
		_ = os.RemoveAll(e.tempDir)
	}
}

func makeSafeResponseBody(size int) []byte {
	if size < 2 {
		size = 2
	}
	chunk := []byte(`{"items":[{"id":1,"name":"widget","price":9.99}],"message":"ordinary response body"}`)
	body := make([]byte, 0, size)
	body = append(body, '[')
	for len(body)+len(chunk)+1 <= size {
		body = append(body, chunk...)
		body = append(body, ',')
	}
	for len(body) < size-1 {
		body = append(body, ' ')
	}
	body = append(body, ']')
	return body
}

func seedHTTPFlows(db *gorm.DB, count int) error {
	if count <= 0 {
		return nil
	}
	sqlDB := db.DB()
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	statement, err := tx.Prepare(`INSERT INTO http_flows
        (created_at, updated_at, hash, url, path, path_suffix, method, source_type, request, response, status_code, tags)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	now := time.Now().Add(-time.Hour)
	for i := 0; i < count; i++ {
		suffix := []string{".html", ".json", ".js", ".css"}[i%4]
		tags := ""
		if i%10 == 0 {
			tags = "seed|YAKIT_COLOR_BLUE"
		}
		path := fmt.Sprintf("/seed/%d%s", i, suffix)
		if _, err := statement.Exec(now, now, fmt.Sprintf("mitm-perf-seed-%d", i), "http://seed.invalid"+path,
			path, suffix, "GET", schema.HTTPFlow_SourceType_MITM, `"GET / HTTP/1.1\\r\\n\\r\\n"`,
			`"HTTP/1.1 200 OK\\r\\n\\r\\nseed"`, 200, tags); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func recordDatabaseReadMetrics(r *report, client ypb.YakClient, label string, samples int) error {
	if samples < 1 {
		samples = 1
	}
	queryDurations := make([]float64, 0, samples)
	backendQueryDurations := make([]float64, 0, samples)
	backendCountDurations := make([]float64, 0, samples)
	backendDataQueryDurations := make([]float64, 0, samples)
	backendConversionDurations := make([]float64, 0, samples)
	fieldGroupDurations := make([]float64, 0, samples)
	systemTimingResponses := 0
	for i := 0; i < samples; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		start := time.Now()
		response, err := client.QueryHTTPFlows(ctx, newRealtimeQuery())
		queryDurations = append(queryDurations, durationMS(time.Since(start)))
		cancel()
		if err != nil {
			return fmt.Errorf("query HTTP flows (%s): %w", label, err)
		}
		if timing := response.GetSystemTiming(); timing != nil {
			systemTimingResponses++
			backendQueryDurations = append(backendQueryDurations, float64(timing.GetQueryDurationUs())/1000)
			if timing.GetCountExecuted() {
				backendCountDurations = append(backendCountDurations, float64(timing.GetCountDurationUs())/1000)
			}
			backendDataQueryDurations = append(backendDataQueryDurations, float64(timing.GetDataQueryDurationUs())/1000)
			backendConversionDurations = append(backendConversionDurations, float64(timing.GetConversionDurationUs())/1000)
		}

		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		start = time.Now()
		_, err = client.HTTPFlowsFieldGroup(ctx, &ypb.HTTPFlowsFieldGroupRequest{RefreshRequest: true, IsAll: true})
		fieldGroupDurations = append(fieldGroupDurations, durationMS(time.Since(start)))
		cancel()
		if err != nil {
			return fmt.Errorf("query HTTP flow field group (%s): %w", label, err)
		}
	}
	r.addMetric(metric{
		Name: "db_read." + label + ".query_httpflows_p95_ms", Value: percentile(queryDurations, 0.95), Unit: "ms",
		Direction: directionLower, Area: "database_query", Description: "QueryHTTPFlows including COUNT(*)",
	})
	r.addMetric(metric{
		Name: "db_read." + label + ".backend_query_p95_ms", Value: percentile(backendQueryDurations, 0.95), Unit: "ms",
		Direction: directionLower, Area: "database_query", Description: "QueryHTTPFlows SQL/COUNT time reported by the backend",
	})
	r.addMetric(metric{
		Name: "db_read." + label + ".backend_count_p95_ms", Value: percentile(backendCountDurations, 0.95), Unit: "ms",
		Direction: directionLower, Area: "database_query", Description: "QueryHTTPFlows COUNT time reported by the backend",
	})
	r.addMetric(metric{
		Name: "db_read." + label + ".backend_data_query_p95_ms", Value: percentile(backendDataQueryDurations, 0.95), Unit: "ms",
		Direction: directionLower, Area: "database_query", Description: "QueryHTTPFlows row SELECT time reported by the backend",
	})
	r.addMetric(metric{
		Name: "db_read." + label + ".backend_conversion_p95_ms", Value: percentile(backendConversionDurations, 0.95), Unit: "ms",
		Direction: directionLower, Area: "database_query", Description: "HTTPFlow database model to protobuf conversion time",
	})
	r.addMetric(metric{
		Name: "db_read." + label + ".field_group_p95_ms", Value: percentile(fieldGroupDurations, 0.95), Unit: "ms",
		Direction: directionLower, Area: "frontend_refresh", Description: "HTTPFlowsFieldGroup(IsAll=true)",
	})
	r.addCheck("db_read."+label+".system_timing", passFail(systemTimingResponses == samples), "observability",
		fmt.Sprintf("timing_responses=%d/%d", systemTimingResponses, samples))
	return nil
}

func newRealtimeQuery() *ypb.QueryHTTPFlowRequest {
	return &ypb.QueryHTTPFlowRequest{
		SourceType:          schema.HTTPFlow_SourceType_MITM,
		Pagination:          &ypb.Paging{Page: 1, Limit: 30, OrderBy: "id", Order: "desc"},
		IncludeSystemTiming: true,
	}
}

func newRealtimeQueryForToken(token string) *ypb.QueryHTTPFlowRequest {
	query := newRealtimeQuery()
	query.SearchURL = token
	// The realtime probe consumes Data and pipeline timings, never Total. Match
	// the MITM incremental-list contract while static db_read metrics retain an
	// exact COUNT baseline.
	query.SkipTotal = true
	return query
}

func runScenario(env *probeEnvironment, cfg scenarioConfig, repeat int) (scenarioResult, error) {
	var result scenarioResult
	if err := waitForDBQueueEmpty(5 * time.Second); err != nil {
		return result, err
	}
	baseGoroutines := int64(runtime.NumGoroutine())

	token := fmt.Sprintf("mitmperf-%s-%d-%d", cfg.name, repeat, time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	stream, err := env.client.MITMV2(ctx)
	if err != nil {
		cancel()
		return result, err
	}
	mitmListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		return result, err
	}
	mitmPort := mitmListener.Addr().(*net.TCPAddr).Port
	_ = mitmListener.Close()

	ready := make(chan struct{})
	streamDone := make(chan struct{})
	var readyOnce sync.Once
	go func() {
		defer close(streamDone)
		for {
			message, recvErr := stream.Recv()
			if recvErr != nil {
				return
			}
			if message.GetHaveMessage() && strings.Contains(string(message.GetMessage().GetMessage()), "starting mitm serve") {
				readyOnce.Do(func() { close(ready) })
			}
		}
	}()
	if err := stream.Send(&ypb.MITMV2Request{
		Host:                "127.0.0.1",
		Port:                uint32(mitmPort),
		DisableTrafficGuard: cfg.disableTraffic,
	}); err != nil {
		cancel()
		return result, err
	}
	select {
	case <-ready:
	case <-ctx.Done():
		cancel()
		return result, fmt.Errorf("MITM %s did not become ready: %w", cfg.name, ctx.Err())
	}

	proxyURL, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(mitmPort))
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		MaxIdleConns:        cfg.concurrency * 2,
		MaxIdleConnsPerHost: cfg.concurrency * 2,
		MaxConnsPerHost:     cfg.concurrency * 2,
		IdleConnTimeout:     15 * time.Second,
	}
	httpClient := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	// Keep warm-up URLs outside the scenario token's SearchURL filter; otherwise
	// the deliberately old warm-up row dominates the end-to-end visibility P95.
	warmupToken := fmt.Sprintf("mitmperf-warmup-%d-%d", repeat, time.Now().UnixNano())
	warmupURL := fmt.Sprintf("%s/%s/0", env.upstream.URL, warmupToken)
	result.warmupRequestMS, err = performProxyRequest(ctx, httpClient, warmupURL, warmupToken)
	if err != nil {
		transport.CloseIdleConnections()
		cancel()
		return result, fmt.Errorf("warm up MITM %s: %w", cfg.name, err)
	}
	if _, err = waitForScenarioFlows(env.server.GetProjectDatabase(), warmupToken, 1, cfg.timeout); err != nil {
		transport.CloseIdleConnections()
		cancel()
		return result, fmt.Errorf("persist MITM warm-up %s: %w", cfg.name, err)
	}
	if err = waitForDBQueueEmpty(5 * time.Second); err != nil {
		transport.CloseIdleConnections()
		cancel()
		return result, err
	}

	pushCancel := func() {}
	var pushCount atomic.Int64
	var pushMu sync.Mutex
	var pushDeliveryMS []float64
	if cfg.realtimeQueries {
		pushCancel, err = startPushCounter(env.client, &pushCount, func(elapsedMS float64) {
			pushMu.Lock()
			pushDeliveryMS = append(pushDeliveryMS, elapsedMS)
			pushMu.Unlock()
		})
		if err != nil {
			transport.CloseIdleConnections()
			cancel()
			return result, err
		}
		// HTTPFlow broadcasts are throttled globally for one second. Keep A/B
		// repetitions independent without including this wait in load timing.
		time.Sleep(1050 * time.Millisecond)
	}

	queryCtx, stopQueries := context.WithCancel(context.Background())
	var queryWG sync.WaitGroup
	var queryMu sync.Mutex
	var queryLatencies []float64
	var queryInFlight atomic.Int64
	var queryPeak atomic.Int64
	var queryErrors atomic.Int64
	querySystemTiming := pipelineTimingSamples{}
	sampledFlowIDs := make(map[uint64]struct{})
	var systemTimingCount int64
	if cfg.realtimeQueries && cfg.queryHz > 0 {
		queryWG.Add(1)
		go func() {
			defer queryWG.Done()
			interval := time.Second / time.Duration(cfg.queryHz)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			limit := make(chan struct{}, 8)
			var calls sync.WaitGroup
			defer calls.Wait()
			launch := func() {
				select {
				case limit <- struct{}{}:
				default:
					return
				}
				calls.Add(1)
				go func() {
					defer calls.Done()
					defer func() { <-limit }()
					current := queryInFlight.Add(1)
					updateAtomicMax(&queryPeak, current)
					defer queryInFlight.Add(-1)
					callCtx, callCancel := context.WithTimeout(context.Background(), 10*time.Second)
					start := time.Now()
					response, callErr := env.client.QueryHTTPFlows(callCtx, newRealtimeQueryForToken(token))
					receivedAt := time.Now()
					elapsed := durationMS(time.Since(start))
					callCancel()
					if callErr != nil {
						queryErrors.Add(1)
						return
					}
					queryMu.Lock()
					queryLatencies = append(queryLatencies, elapsed)
					if appendPipelineTimingSamples(&querySystemTiming, response, receivedAt, sampledFlowIDs) {
						systemTimingCount++
					}
					queryMu.Unlock()
				}()
			}
			launch()
			for {
				select {
				case <-queryCtx.Done():
					return
				case <-ticker.C:
					launch()
				}
			}
		}()
	}

	var queuePeak atomic.Int64
	var goroutinePeak atomic.Int64
	samplerDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-samplerDone:
				return
			case <-ticker.C:
				updateAtomicMax(&queuePeak, int64(len(yakit.DBSaveAsyncChannel)))
				updateAtomicMax(&goroutinePeak, int64(runtime.NumGoroutine())-baseGoroutines)
			}
		}
	}()

	jobs := make(chan int)
	var workerWG sync.WaitGroup
	var latencyMu sync.Mutex
	var requestLatencies []float64
	var requestErrors atomic.Int64
	loadStarted := time.Now()
	cpuStarted := readProcessCPUSeconds()
	for worker := 0; worker < cfg.concurrency; worker++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for id := range jobs {
				requestURL := fmt.Sprintf("%s/%s/%d", env.upstream.URL, token, id)
				elapsed, requestErr := performProxyRequest(ctx, httpClient, requestURL, token)
				if requestErr != nil {
					requestErrors.Add(1)
					continue
				}
				latencyMu.Lock()
				requestLatencies = append(requestLatencies, elapsed)
				latencyMu.Unlock()
			}
		}()
	}
	for i := 0; i < cfg.requests; i++ {
		jobs <- i
	}
	close(jobs)
	workerWG.Wait()
	result.requestElapsed = time.Since(loadStarted)
	result.requestErrors = requestErrors.Load()
	result.requestLatenciesMS = requestLatencies

	if cfg.cancelBeforeDrain {
		result.persistedAtStop, _ = countScenarioFlows(env.server.GetProjectDatabase(), token)
		result.queueAtStop = int64(len(yakit.DBSaveAsyncChannel))
		transport.CloseIdleConnections()
		result.shutdownElapsed = stopMITM(cancel, streamDone, mitmPort, 10*time.Second)
		drainStarted := time.Now()
		result.persisted, err = waitForScenarioFlows(env.server.GetProjectDatabase(), token, cfg.requests, cfg.timeout)
		result.postStopDrain = time.Since(drainStarted)
	} else {
		result.persisted, err = waitForScenarioFlows(env.server.GetProjectDatabase(), token, cfg.requests, cfg.timeout)
		result.persistElapsed = time.Since(loadStarted)
		transport.CloseIdleConnections()
		result.shutdownElapsed = stopMITM(cancel, streamDone, mitmPort, 10*time.Second)
	}

	stopQueries()
	queryWG.Wait()
	if cfg.realtimeQueries {
		// Always take one scoped final sample after persistence. Short smoke
		// scenarios can finish between periodic ticks; without this sample their
		// report would accidentally describe a previous scenario's rows.
		callCtx, callCancel := context.WithTimeout(context.Background(), 10*time.Second)
		start := time.Now()
		response, callErr := env.client.QueryHTTPFlows(callCtx, newRealtimeQueryForToken(token))
		receivedAt := time.Now()
		elapsed := durationMS(time.Since(start))
		callCancel()
		if callErr != nil {
			queryErrors.Add(1)
		} else {
			queryMu.Lock()
			queryLatencies = append(queryLatencies, elapsed)
			if appendPipelineTimingSamples(&querySystemTiming, response, receivedAt, sampledFlowIDs) {
				systemTimingCount++
			}
			queryMu.Unlock()
		}
	}
	pushCancel()
	close(samplerDone)
	result.queuePeak = queuePeak.Load()
	result.goroutinePeakDelta = goroutinePeak.Load()
	result.queryPeakInFlight = queryPeak.Load()
	result.queryErrors = queryErrors.Load()
	result.pushMessages = pushCount.Load()
	pushMu.Lock()
	result.pushDeliveryMS = append([]float64(nil), pushDeliveryMS...)
	pushMu.Unlock()
	result.cpuBusySeconds = readProcessCPUSeconds() - cpuStarted
	result.cpuWallElapsed = time.Since(loadStarted)
	if result.cpuBusySeconds < 0 {
		result.cpuBusySeconds = 0
	}
	queryMu.Lock()
	result.queryLatenciesMS = append([]float64(nil), queryLatencies...)
	result.systemTiming = querySystemTiming
	result.systemTimingCount = systemTimingCount
	queryMu.Unlock()
	postShutdownStarted := time.Now()
	postShutdownCPUStarted := readProcessCPUSeconds()
	time.Sleep(250 * time.Millisecond)
	postShutdownCPUSeconds := readProcessCPUSeconds() - postShutdownCPUStarted
	if postShutdownCPUSeconds > 0 {
		result.postShutdownCPU = postShutdownCPUSeconds / time.Since(postShutdownStarted).Seconds()
	}
	result.goroutineAfter = int64(runtime.NumGoroutine()) - baseGoroutines
	if result.goroutineAfter < 0 {
		result.goroutineAfter = 0
	}

	if err != nil {
		return result, fmt.Errorf("scenario %s persisted %d/%d flows: %w", cfg.name, result.persisted, cfg.requests, err)
	}
	return result, nil
}

func performProxyRequest(ctx context.Context, client *http.Client, requestURL, token string) (float64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("X-MITM-Perf", token)
	started := time.Now()
	response, err := client.Do(request)
	elapsed := durationMS(time.Since(started))
	if err != nil {
		return elapsed, err
	}
	_, copyErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if copyErr != nil {
		return elapsed, copyErr
	}
	return elapsed, closeErr
}

func stopMITM(cancel context.CancelFunc, streamDone <-chan struct{}, port int, timeout time.Duration) time.Duration {
	started := time.Now()
	cancel()
	deadline := time.Now().Add(timeout)
	select {
	case <-streamDone:
	case <-time.After(time.Until(deadline)):
		return timeout
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 25*time.Millisecond)
		if err != nil {
			return time.Since(started)
		}
		_ = connection.Close()
		time.Sleep(10 * time.Millisecond)
	}
	return timeout
}

func startPushCounter(client ypb.YakClient, count *atomic.Int64, onDelivery func(float64)) (func(), error) {
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.DuplexConnection(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			message, recvErr := stream.Recv()
			if recvErr != nil {
				return
			}
			if message.GetMessageType() == yakit.ServerPushType_HttpFlow {
				count.Add(1)
				if timestamp := message.GetTimestamp(); timestamp > 0 && onDelivery != nil {
					elapsed := time.Since(time.Unix(0, timestamp))
					if elapsed >= 0 && elapsed < time.Minute {
						onDelivery(durationMS(elapsed))
					}
				}
			}
		}
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}, nil
}

func appendPipelineTimingSamples(
	target *pipelineTimingSamples,
	response *ypb.QueryHTTPFlowResponse,
	receivedAt time.Time,
	seenFlowIDs map[uint64]struct{},
) bool {
	if target == nil || response == nil || response.GetSystemTiming() == nil {
		return false
	}
	timing := response.GetSystemTiming()
	target.backendQueryMS = append(target.backendQueryMS, float64(timing.GetQueryDurationUs())/1000)
	if timing.GetCountExecuted() {
		target.backendCountMS = append(target.backendCountMS, float64(timing.GetCountDurationUs())/1000)
		target.countExecuted++
	}
	target.backendDataQueryMS = append(target.backendDataQueryMS, float64(timing.GetDataQueryDurationUs())/1000)
	target.backendConversionMS = append(target.backendConversionMS, float64(timing.GetConversionDurationUs())/1000)
	target.asyncWriteQueueDepth = append(target.asyncWriteQueueDepth, float64(timing.GetAsyncWriteQueueDepth()))
	receivedAtUnixMs := receivedAt.UnixMilli()
	for _, flowTiming := range timing.GetFlowTimings() {
		id := flowTiming.GetId()
		if id == 0 {
			continue
		}
		if _, exists := seenFlowIDs[id]; exists {
			continue
		}
		seenFlowIDs[id] = struct{}{}
		target.observedFlowCount++
		appendUnixDuration(&target.persistQueueWaitMS, flowTiming.GetPersistStartedAtUnixMs(), flowTiming.GetPersistEnqueuedAtUnixMs())
		appendUnixDuration(&target.persistWriteMS, flowTiming.GetPersistedAtUnixMs(), flowTiming.GetPersistStartedAtUnixMs())
		appendUnixDuration(&target.databaseChangeDetectionMS, flowTiming.GetDatabaseChangeDetectedAtUnixMs(), flowTiming.GetPersistedAtUnixMs())
		appendUnixDuration(&target.requestToFlowBuiltMS, flowTiming.GetFlowBuiltAtUnixMs(), flowTiming.GetRequestHijackAtUnixMs())
		appendUnixDuration(&target.responseToFlowBuiltMS, flowTiming.GetFlowBuiltAtUnixMs(), flowTiming.GetResponseMirrorAtUnixMs())
		appendUnixDuration(&target.requestToProbeReceiveMS, receivedAtUnixMs, flowTiming.GetRequestHijackAtUnixMs())
		appendUnixDuration(&target.responseToProbeReceiveMS, receivedAtUnixMs, flowTiming.GetResponseMirrorAtUnixMs())
		appendUnixDuration(&target.persistToProbeReceiveMS, receivedAtUnixMs, flowTiming.GetPersistedAtUnixMs())
	}
	return true
}

func appendUnixDuration(target *[]float64, endUnixMs, startUnixMs int64) {
	if target == nil || endUnixMs <= 0 || startUnixMs <= 0 || endUnixMs < startUnixMs {
		return
	}
	*target = append(*target, float64(endUnixMs-startUnixMs))
}

func countScenarioFlows(db *gorm.DB, token string) (int, error) {
	var count int
	query := db.Model(&schema.HTTPFlow{}).
		Where("source_type = ?", schema.HTTPFlow_SourceType_MITM).
		Where("url LIKE ?", "%/"+token+"/%").
		Count(&count)
	return count, query.Error
}

func waitForScenarioFlows(db *gorm.DB, token string, expected int, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	last := 0
	for time.Now().Before(deadline) {
		count, err := countScenarioFlows(db, token)
		if err != nil {
			return count, err
		}
		last = count
		if count >= expected {
			return count, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return last, fmt.Errorf("timed out after %s", timeout)
}

func waitForDBQueueEmpty(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(yakit.DBSaveAsyncChannel) == 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("DB save queue still contains %d item(s) after %s", len(yakit.DBSaveAsyncChannel), timeout)
}

func recordScenarioMetrics(r *report, cfg scenarioConfig, runs []scenarioResult) {
	prefix := "mitm." + cfg.name + "."
	values := func(read func(scenarioResult) float64) []float64 {
		out := make([]float64, 0, len(runs))
		for _, run := range runs {
			out = append(out, read(run))
		}
		return out
	}
	add := func(name, unit string, direction metricDirection, area, description string, read func(scenarioResult) float64) {
		r.addMetric(metric{Name: prefix + name, Value: median(values(read)), Unit: unit, Direction: direction, Area: area, Description: description})
	}

	add("request_p95_ms", "ms", directionLower, "mitm_pipeline", "client-observed proxy latency p95", func(run scenarioResult) float64 {
		return percentile(run.requestLatenciesMS, 0.95)
	})
	add("warmup_request_ms", "ms", directionLower, "mitm_pipeline", "one request before the steady-state sample, including lazy initialization", func(run scenarioResult) float64 {
		return run.warmupRequestMS
	})
	add("request_throughput", "flow/s", directionHigher, "mitm_pipeline", "completed proxy responses per second", func(run scenarioResult) float64 {
		return float64(cfg.requests) / run.requestElapsed.Seconds()
	})
	add("persist_throughput", "flow/s", directionHigher, "database_write", "flows visible in SQLite per second", func(run scenarioResult) float64 {
		elapsed := run.persistElapsed
		if cfg.cancelBeforeDrain {
			elapsed = run.requestElapsed + run.postStopDrain
		}
		return float64(cfg.requests) / elapsed.Seconds()
	})
	add("cpu_busy_ms_per_flow", "cpu-ms/flow", directionLower, "cpu", "Go user and runtime CPU consumed per proxied flow", func(run scenarioResult) float64 {
		return run.cpuBusySeconds * 1000 / float64(cfg.requests)
	})
	add("cpu_average_cores", "cores", directionNeutral, "cpu", "informational average busy CPU cores during traffic, persistence, and shutdown", func(run scenarioResult) float64 {
		if run.cpuWallElapsed <= 0 {
			return 0
		}
		return run.cpuBusySeconds / run.cpuWallElapsed.Seconds()
	})
	add("post_shutdown_cpu_cores", "cores", directionLower, "shutdown", "average process CPU for 250ms after stream, query, push, and DB drain completion", func(run scenarioResult) float64 {
		return run.postShutdownCPU
	})
	add("queue_peak", "items", directionLower, "database_write", "peak DBSaveAsyncChannel depth", func(run scenarioResult) float64 {
		return float64(run.queuePeak)
	})
	add("goroutine_peak_delta", "goroutines", directionLower, "concurrency", "peak goroutines above the ready-state baseline", func(run scenarioResult) float64 {
		return float64(run.goroutinePeakDelta)
	})
	add("goroutine_after_shutdown_delta", "goroutines", directionLower, "shutdown", "goroutines remaining 250ms after shutdown", func(run scenarioResult) float64 {
		return float64(run.goroutineAfter)
	})
	add("shutdown_ms", "ms", directionLower, "shutdown", "MITMV2 stream cancellation to Recv completion", func(run scenarioResult) float64 {
		return durationMS(run.shutdownElapsed)
	})
	add("query_p95_ms", "ms", directionLower, "database_query", "QueryHTTPFlows latency while traffic is active", func(run scenarioResult) float64 {
		return percentile(run.queryLatenciesMS, 0.95)
	})
	add("backend_query_p95_ms", "ms", directionLower, "database_query", "backend SQL/COUNT time from QueryHTTPFlows system timing", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.backendQueryMS, 0.95)
	})
	add("backend_count_p95_ms", "ms", directionLower, "database_query", "backend COUNT time from QueryHTTPFlows system timing", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.backendCountMS, 0.95)
	})
	add("backend_data_query_p95_ms", "ms", directionLower, "database_query", "backend row SELECT time from QueryHTTPFlows system timing", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.backendDataQueryMS, 0.95)
	})
	add("count_executions", "queries", directionNeutral, "database_query", "realtime QueryHTTPFlows responses that executed COUNT", func(run scenarioResult) float64 {
		return float64(run.systemTiming.countExecuted)
	})
	add("backend_conversion_p95_ms", "ms", directionLower, "database_query", "HTTPFlow model-to-protobuf conversion time", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.backendConversionMS, 0.95)
	})
	add("observed_async_queue_depth_p95", "items", directionLower, "database_write", "DBSaveAsyncChannel depth observed by realtime queries", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.asyncWriteQueueDepth, 0.95)
	})
	add("persist_queue_wait_p95_ms", "ms", directionLower, "database_write", "HTTPFlow enqueue to SQL start for unique sampled flows", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.persistQueueWaitMS, 0.95)
	})
	add("persist_write_p95_ms", "ms", directionLower, "database_write", "HTTPFlow SQL start to successful insert for unique sampled flows", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.persistWriteMS, 0.95)
	})
	add("database_change_detection_p95_ms", "ms", directionLower, "frontend_refresh", "persisted high-water to database watcher detection", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.databaseChangeDetectionMS, 0.95)
	})
	add("request_to_flow_built_p95_ms", "ms", directionLower, "mitm_pipeline", "MITM request hijack entry to HTTPFlow model built", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.requestToFlowBuiltMS, 0.95)
	})
	add("response_to_flow_built_p95_ms", "ms", directionLower, "mitm_pipeline", "MITM response mirror entry to HTTPFlow model built", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.responseToFlowBuiltMS, 0.95)
	})
	add("request_to_probe_receive_p95_ms", "ms", directionLower, "frontend_refresh", "MITM request hijack entry to direct gRPC probe receive; excludes Electron and React", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.requestToProbeReceiveMS, 0.95)
	})
	add("response_to_probe_receive_p95_ms", "ms", directionLower, "frontend_refresh", "MITM response mirror to direct gRPC probe receive; excludes Electron and React", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.responseToProbeReceiveMS, 0.95)
	})
	add("persist_to_probe_receive_p95_ms", "ms", directionLower, "frontend_refresh", "HTTPFlow persisted to direct gRPC probe receive; excludes Electron and React", func(run scenarioResult) float64 {
		return percentile(run.systemTiming.persistToProbeReceiveMS, 0.95)
	})
	add("duplex_delivery_p95_ms", "ms", directionLower, "frontend_refresh", "HTTPFlow broadcast creation to direct gRPC probe receive", func(run scenarioResult) float64 {
		return percentile(run.pushDeliveryMS, 0.95)
	})
	add("system_timing_responses", "responses", directionNeutral, "observability", "realtime query responses carrying system timing", func(run scenarioResult) float64 {
		return float64(run.systemTimingCount)
	})
	add("observed_flow_timing_samples", "flows", directionNeutral, "observability", "unique persisted flows correlated through QueryHTTPFlows", func(run scenarioResult) float64 {
		return float64(run.systemTiming.observedFlowCount)
	})
	add("query_peak_inflight", "queries", directionLower, "frontend_refresh", "peak overlapping realtime queries", func(run scenarioResult) float64 {
		return float64(run.queryPeakInFlight)
	})
	add("push_messages", "messages", directionLower, "frontend_refresh", "HTTPFlow duplex push messages observed", func(run scenarioResult) float64 {
		return float64(run.pushMessages)
	})
	if cfg.cancelBeforeDrain {
		add("queue_at_stop", "items", directionLower, "shutdown", "DB queue depth when MITMV2 is cancelled", func(run scenarioResult) float64 {
			return float64(run.queueAtStop)
		})
		add("persisted_at_stop", "flows", directionNeutral, "shutdown", "flows committed when MITMV2 is cancelled", func(run scenarioResult) float64 {
			return float64(run.persistedAtStop)
		})
		add("post_stop_drain_ms", "ms", directionLower, "shutdown", "time for outstanding flows to become visible after MITMV2 cancellation", func(run scenarioResult) float64 {
			return durationMS(run.postStopDrain)
		})
	}

	requestErrors := int64(0)
	queryErrors := int64(0)
	persistedOK := true
	shutdownOK := true
	pushDeliveryOK := true
	systemTimingOK := true
	for _, run := range runs {
		requestErrors += run.requestErrors
		queryErrors += run.queryErrors
		persistedOK = persistedOK && run.persisted >= cfg.requests
		shutdownOK = shutdownOK && run.shutdownElapsed < 10*time.Second
		if cfg.realtimeQueries {
			pushDeliveryOK = pushDeliveryOK && run.pushMessages > 0
			systemTimingOK = systemTimingOK && run.systemTimingCount > 0
		}
	}
	r.addCheck(prefix+"requests", passFail(requestErrors == 0), "mitm_pipeline", fmt.Sprintf("request_errors=%d", requestErrors))
	r.addCheck(prefix+"persisted", passFail(persistedOK), "database_write", fmt.Sprintf("expected=%d per repetition", cfg.requests))
	r.addCheck(prefix+"queries", passFail(queryErrors == 0), "database_query", fmt.Sprintf("query_errors=%d", queryErrors))
	r.addCheck(prefix+"shutdown_bounded", passFail(shutdownOK), "shutdown", "stream must stop within 10s")
	if cfg.realtimeQueries {
		r.addCheck(prefix+"push_delivery", passFail(pushDeliveryOK), "frontend_refresh", "at least one HTTPFlow duplex notification per repetition")
		r.addCheck(prefix+"system_timing", passFail(systemTimingOK), "observability", "at least one realtime query returned bounded system timing")
	}
}

func updateAtomicMax(target *atomic.Int64, candidate int64) {
	for {
		current := target.Load()
		if candidate <= current || target.CompareAndSwap(current, candidate) {
			return
		}
	}
}

func durationMS(duration time.Duration) float64 {
	return float64(duration.Nanoseconds()) / float64(time.Millisecond)
}

func passFail(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}
