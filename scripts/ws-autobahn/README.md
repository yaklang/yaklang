# Yak WebSocket Autobahn Regression

完整的测试分层、发布验收标准和第三轮可靠性计划见
[RELIABILITY_TESTING.md](./RELIABILITY_TESTING.md)。

完整压缩测试发现的 WebSocket 流落库内存放大、背压设计和后续验收标准见
[WEBSOCKET_FLOW_PERSISTENCE_BACKPRESSURE.md](./WEBSOCKET_FLOW_PERSISTENCE_BACKPRESSURE.md)。

The runner starts the digest-pinned Autobahn fuzzing server, runs the Yak lowhttp
client, then compares a Gorilla reference client directly and through Yakit
MITM. All agents are written to the same HTML report. The runner also checks
`index.json` and fails when an agent has a hard failure or the MITM introduces
a hard failure relative to the direct Gorilla baseline.

Run the smoke profile with scripts/ws-autobahn/run.sh.

Profiles:

- AUTOBAHN_PROFILE=smoke scripts/ws-autobahn/run.sh
- AUTOBAHN_PROFILE=core scripts/ws-autobahn/run.sh
- AUTOBAHN_PROFILE=compression-smoke scripts/ws-autobahn/run.sh
- AUTOBAHN_PROFILE=compression scripts/ws-autobahn/run.sh

The full compression profile contains 216 cases, sends 1000 messages per case,
and defines per-case timeouts up to 480 seconds. Use `compression-smoke` for
normal development and reserve `compression` for long-running validation.
The compression profiles use the Yak lowhttp client on both the direct and
MITM paths because Gorilla only supports a narrower set of compression
negotiation responses. Smoke and core retain the Gorilla differential baseline.

The full compression profile disables WebSocket flow storage by default while
testing the MITM path. Frame parsing, RFC 7692 transformation, and forwarding
remain enabled; avoiding hundreds of thousands of large SQLite/UI records keeps
the conformance run focused and memory-bounded. Set
`AUTOBAHN_MITM_DISABLE_FLOW_STORAGE=false` to include persistence as a separate
load test. Core, compression-smoke, and Vulinbox tests keep storage enabled.

Modes:

- AUTOBAHN_MODE=yak-client scripts/ws-autobahn/run.sh
- AUTOBAHN_MODE=mitm scripts/ws-autobahn/run.sh
- AUTOBAHN_MODE=all scripts/ws-autobahn/run.sh

gorilla-direct is the differential baseline. A case that passes for
gorilla-direct and hard-fails for gorilla-via-yak-mitm is a Yakit MITM
regression. Direct Gorilla failures remain visible in the report but do not
fail the run because Gorilla does not enforce every Autobahn UTF-8 case.
`NON-STRICT` results are reported as warnings: a WebSocket proxy can close on a
protocol error before a previously forwarded application echo returns, which
changes Autobahn's observed event ordering without accepting the invalid frame.
Reports are written under reports/autobahn/<profile>/clients.

The image, port, report directory, per-case timeout, suite timeout, and Go test
timeouts can be overridden with AUTOBAHN_IMAGE, AUTOBAHN_PORT,
AUTOBAHN_REPORT_DIR, AUTOBAHN_CASE_TIMEOUT, AUTOBAHN_SUITE_TIMEOUT,
AUTOBAHN_LOWHTTP_TEST_TIMEOUT, AUTOBAHN_MITM_TEST_TIMEOUT, and
AUTOBAHN_MITM_DISABLE_FLOW_STORAGE.
