// aibalance opencode tool_call 调查 - 朴素 raw HTTP 直打 aibalance / 上游兼容端点,
// 把 SSE 字节落盘. 这一工具的目的是: 在 SDK (openai npm / vercel ai sdk / opencode)
// 之外提供一条"对照组"链路, 以最简单的 fetch+ReadableStream 拿到 aibalance 在
// 完全可控请求体下的真实 SSE, 用于对照 SDK 调用是否引入了额外格式差异.
//
// 用法:
//   node common/aibalance/cmd/debug_capture_upstream.mjs single-stream
//   node common/aibalance/cmd/debug_capture_upstream.mjs parallel-stream
//   node common/aibalance/cmd/debug_capture_upstream.mjs round2-stream
//
// 环境变量:
//   AIBALANCE_BASE       chat completions URL
//                        默认 http://127.0.0.1:8223/v1/chat/completions
//   AIBALANCE_KEY        API key (默认 "asd", 与 opencode aibalance-copy 一致)
//   AIBALANCE_MODEL      模型名 (默认 local-deepseek-v4-pro-free)
//   AIBALANCE_DUMP_FILE  raw SSE 落盘路径 (默认 /tmp/aibalance_raw_<scenario>_<ts>.sse)
//
// 关键词: aibalance raw SSE 对照组, opencode tool_call 调查, fetch ReadableStream
//        round1 round2 minimal repro

import fs from "node:fs";
import path from "node:path";

const scenario = process.argv[2] || "single-stream";
const SCENARIOS = new Set([
    "single-stream", "single-non-stream",
    "parallel-stream", "parallel-non-stream",
    "round2-stream",
]);
if (!SCENARIOS.has(scenario)) {
    console.error(`unknown scenario: ${scenario}, choose from: ${Array.from(SCENARIOS).join(", ")}`);
    process.exit(2);
}

const BASE = process.env.AIBALANCE_BASE || "http://127.0.0.1:8223/v1/chat/completions";
const KEY = process.env.AIBALANCE_KEY || "asd";
const MODEL = process.env.AIBALANCE_MODEL || "local-deepseek-v4-pro-free";

function pad2(n) { return String(n).padStart(2, "0"); }
function nowTs() {
    const d = new Date();
    return d.getFullYear() + pad2(d.getMonth() + 1) + pad2(d.getDate())
        + "_" + pad2(d.getHours()) + pad2(d.getMinutes()) + pad2(d.getSeconds());
}
const DUMP_FILE = process.env.AIBALANCE_DUMP_FILE
    || `/tmp/aibalance_raw_${scenario}_${nowTs()}.sse`;

const WEATHER_TOOL = {
    type: "function",
    function: {
        name: "get_current_weather",
        description: "Get the current weather of a city.",
        parameters: {
            type: "object",
            properties: { city: { type: "string" } },
            required: ["city"],
        },
    },
};
const NEWS_TOOL = {
    type: "function",
    function: {
        name: "fetch_latest_news",
        description: "Fetch latest news for a topic.",
        parameters: {
            type: "object",
            properties: { topic: { type: "string" } },
            required: ["topic"],
        },
    },
};

function buildBody() {
    if (scenario.startsWith("single")) {
        return {
            model: MODEL,
            messages: [{
                role: "user",
                content: "I want the current weather in Beijing. Please call get_current_weather(city='Beijing'). Do not answer in plain text.",
            }],
            tools: [WEATHER_TOOL],
            tool_choice: "auto",
            stream: scenario.endsWith("stream"),
            max_tokens: 512,
        };
    }
    if (scenario.startsWith("parallel")) {
        return {
            model: MODEL,
            messages: [{
                role: "user",
                content: "I need TWO things in parallel: (1) current weather in Beijing via "
                    + "get_current_weather(city='Beijing'), and (2) latest news about AI via "
                    + "fetch_latest_news(topic='AI'). Please call BOTH tools now. Do not answer in plain text.",
            }],
            tools: [WEATHER_TOOL, NEWS_TOOL],
            tool_choice: "auto",
            stream: scenario.endsWith("stream"),
            max_tokens: 512,
        };
    }
    if (scenario === "round2-stream") {
        // 模拟 opencode round2: 先有用户问, 再 assistant.tool_calls, 再 role:tool 回灌.
        return {
            model: MODEL,
            messages: [
                { role: "user", content: "What's the weather in Beijing?" },
                {
                    role: "assistant",
                    content: "",
                    tool_calls: [{
                        id: "call_round2_test_001",
                        type: "function",
                        function: {
                            name: "get_current_weather",
                            arguments: "{\"city\":\"Beijing\"}",
                        },
                    }],
                },
                {
                    role: "tool",
                    tool_call_id: "call_round2_test_001",
                    name: "get_current_weather",
                    content: JSON.stringify({ city: "Beijing", temperature_c: 21, condition: "sunny" }),
                },
            ],
            tools: [WEATHER_TOOL],
            tool_choice: "auto",
            stream: true,
            max_tokens: 512,
        };
    }
    throw new Error("unreachable scenario");
}

const body = buildBody();
const bodyText = JSON.stringify(body);
console.log(`[capture] scenario=${scenario}`);
console.log(`[capture] base   =${BASE}`);
console.log(`[capture] model  =${MODEL}`);
console.log(`[capture] stream =${body.stream}`);
console.log(`[capture] tools  =${(body.tools || []).map(t => t.function.name).join(",") || "(none)"}`);
console.log(`[capture] msgs   =${body.messages.length}`);
console.log(`[capture] dump   =${DUMP_FILE}`);

const requestStart = Date.now();
const resp = await fetch(BASE, {
    method: "POST",
    headers: {
        "content-type": "application/json",
        "authorization": `Bearer ${KEY}`,
        "accept": body.stream ? "text/event-stream" : "application/json",
    },
    body: bodyText,
});

const headerSummary = {
    status: resp.status,
    statusText: resp.statusText,
    headers: Object.fromEntries(resp.headers.entries()),
};
const headerFile = DUMP_FILE.replace(/\.[^.]+$/, ".headers.json");
fs.writeFileSync(headerFile, JSON.stringify(headerSummary, null, 2));
console.log(`[capture] http ${resp.status} ${resp.statusText}, headers -> ${headerFile}`);

const out = fs.createWriteStream(DUMP_FILE, { flags: "w" });
let totalBytes = 0;

if (!body.stream) {
    const buf = Buffer.from(await resp.arrayBuffer());
    out.write(buf);
    totalBytes = buf.length;
    out.end();
    console.log(`[capture] non-stream body bytes=${totalBytes}, dur=${Date.now() - requestStart}ms`);
    process.exit(0);
}

if (!resp.body) {
    console.error("[capture] no response body for stream request");
    out.end();
    process.exit(1);
}

const reader = resp.body.getReader();
while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    if (value && value.length) {
        out.write(Buffer.from(value));
        totalBytes += value.length;
    }
}
out.end();
console.log(`[capture] stream body bytes=${totalBytes}, dur=${Date.now() - requestStart}ms`);
