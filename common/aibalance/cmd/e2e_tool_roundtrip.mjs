// aibalance Tool Call 完整闭环 E2E (Node.js / TypeScript SDK 矩阵)
//
// 用 npm install openai ai @ai-sdk/openai zod 后运行:
//   node common/aibalance/cmd/e2e_tool_roundtrip.mjs
//
// 矩阵 = SDK(2: openai-npm / vercel-ai-sdk) x stream(2) x tool_mode(single / parallel) = 8 case 每个 model.
//
// 环境变量 (同 e2e_tool_roundtrip.py 风格):
//   AIBALANCE_BASE                aibalance 端点 (默认 http://127.0.0.1:8080/v1)
//   AIBALANCE_KEY                 aibalance API key
//   AIBALANCE_KEY_FILE            aibalance API key 文件路径 (优先 AIBALANCE_KEY)
//   AIBALANCE_MODEL               逗号分隔 model 列表
//   AIBALANCE_ROUND_TRIP_TIMEOUT  单请求超时秒 (默认 30)
//   AIBALANCE_SKIP_VERCEL         skip vercel ai sdk path (节省安装依赖时使用)
//   AIBALANCE_ONLY_STREAM         only test stream=True
//   AIBALANCE_ONLY_SINGLE         only test 单 tool
//
// 关键词: aibalance node openai-sdk e2e, vercel ai sdk e2e, tool round-trip matrix

import fs from "node:fs";
import os from "node:os";
import path from "node:path";

// ---------- helpers ----------

function loadApiKey() {
    const envKey = (process.env.AIBALANCE_KEY || "").trim();
    if (envKey) return envKey;
    const file = process.env.AIBALANCE_KEY_FILE
        || path.join(os.homedir(), "yakit-projects/aibalance-key-z.txt");
    if (!fs.existsSync(file)) {
        throw new Error(
            `AIBALANCE_KEY env not set and key file not found: ${file}\n` +
            `Set AIBALANCE_KEY=xxx or AIBALANCE_KEY_FILE=/path/to/key`,
        );
    }
    return fs.readFileSync(file, "utf-8").trim();
}

function loadConfig() {
    const baseURL = process.env.AIBALANCE_BASE || "http://127.0.0.1:8080/v1";
    const apiKey = loadApiKey();
    const timeoutSec = parseInt(process.env.AIBALANCE_ROUND_TRIP_TIMEOUT || "30", 10);
    const envModels = (process.env.AIBALANCE_MODEL || "").trim();
    const models = envModels
        ? envModels.replace(/;/g, ",").split(",").map(s => s.trim()).filter(Boolean)
        : ["mock-native", "mock-dumb"];
    const onlyStream = ["1", "true", "yes"].includes((process.env.AIBALANCE_ONLY_STREAM || "").toLowerCase());
    const onlySingle = ["1", "true", "yes"].includes((process.env.AIBALANCE_ONLY_SINGLE || "").toLowerCase());
    const skipVercel = ["1", "true", "yes"].includes((process.env.AIBALANCE_SKIP_VERCEL || "").toLowerCase());
    return { baseURL, apiKey, timeoutSec, models, onlyStream, onlySingle, skipVercel };
}

// 关键词: e2e fake tool implementations
function fakeWeather(city) {
    return { city, temperature_c: 21, condition: "sunny", wind: "north 2 m/s" };
}
function fakeNews(topic) {
    return { topic, headlines: ["headline-1", "headline-2"] };
}

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

function buildUserPrompt(parallel) {
    if (parallel) {
        return (
            "I need TWO things in parallel: (1) current weather in Beijing via "
            + "get_current_weather(city='Beijing'), and (2) latest news about AI via "
            + "fetch_latest_news(topic='AI'). Please call BOTH tools now. Do not answer in plain text."
        );
    }
    return (
        "I want the current weather in Beijing. Please call "
        + "get_current_weather(city='Beijing'). Do not answer in plain text."
    );
}

// ---------- openai npm sdk path ----------

async function runWithOpenAI({ baseURL, apiKey, timeoutSec }, { model, stream, parallel }) {
    const { default: OpenAI } = await import("openai");
    const client = new OpenAI({ baseURL, apiKey, timeout: timeoutSec * 1000 });
    const tools = parallel ? [WEATHER_TOOL, NEWS_TOOL] : [WEATHER_TOOL];
    const firstMessages = [{ role: "user", content: buildUserPrompt(parallel) }];

    const r1 = await client.chat.completions.create({
        model,
        messages: firstMessages,
        tools,
        tool_choice: "auto",
        stream,
        max_tokens: 512,
    });

    let assistantToolCalls;
    if (stream) {
        // streaming accumulator
        const acc = new Map();
        let finishReason = "";
        for await (const chunk of r1) {
            const choice = chunk.choices?.[0];
            if (!choice) continue;
            if (choice.finish_reason) finishReason = choice.finish_reason;
            const delta = choice.delta;
            if (delta?.tool_calls) {
                for (const tc of delta.tool_calls) {
                    const idx = tc.index ?? 0;
                    if (!acc.has(idx)) {
                        acc.set(idx, { id: "", type: "function", name: "", arguments: "" });
                    }
                    const slot = acc.get(idx);
                    if (tc.id) slot.id = tc.id;
                    if (tc.type) slot.type = tc.type;
                    if (tc.function?.name) slot.name = tc.function.name;
                    if (tc.function?.arguments) slot.arguments += tc.function.arguments;
                }
            }
        }
        if (finishReason !== "tool_calls" || acc.size === 0) {
            throw new Error(`round1 expected tool_calls; finish_reason=${finishReason}; acc=${JSON.stringify(Array.from(acc.entries()))}`);
        }
        assistantToolCalls = Array.from(acc.entries())
            .sort((a, b) => a[0] - b[0])
            .map(([_, v], i) => ({
                id: v.id || `call_${i}`,
                type: v.type || "function",
                function: { name: v.name, arguments: v.arguments },
            }));
    } else {
        const choice = r1.choices?.[0];
        if (!choice || choice.finish_reason !== "tool_calls" || !choice.message.tool_calls?.length) {
            throw new Error(`round1 expected tool_calls; got finish_reason=${choice?.finish_reason}; tool_calls=${JSON.stringify(choice?.message?.tool_calls)}`);
        }
        assistantToolCalls = choice.message.tool_calls.map(tc => ({
            id: tc.id,
            type: tc.type,
            function: { name: tc.function.name, arguments: tc.function.arguments },
        }));
    }

    const secondMessages = [
        ...firstMessages,
        { role: "assistant", content: "", tool_calls: assistantToolCalls },
    ];
    for (const tc of assistantToolCalls) {
        let args = {};
        try { args = JSON.parse(tc.function.arguments || "{}"); } catch (_) {}
        let result;
        if (tc.function.name === "get_current_weather") result = fakeWeather(args.city || "Beijing");
        else if (tc.function.name === "fetch_latest_news") result = fakeNews(args.topic || "AI");
        else result = { echo: args };
        secondMessages.push({
            role: "tool",
            tool_call_id: tc.id,
            name: tc.function.name,
            content: JSON.stringify(result),
        });
    }

    const r2 = await client.chat.completions.create({
        model,
        messages: secondMessages,
        tools,
        tool_choice: "auto",
        stream,
        max_tokens: 512,
    });

    if (stream) {
        let content = "";
        let fr = "";
        for await (const chunk of r2) {
            const choice = chunk.choices?.[0];
            if (!choice) continue;
            if (choice.finish_reason) fr = choice.finish_reason;
            if (choice.delta?.content) content += choice.delta.content;
        }
        if (fr !== "stop") throw new Error(`round2 finish_reason=${fr}; content=${content.slice(0, 120)}`);
        if (!content.trim()) throw new Error(`round2 content empty`);
        return content.trim();
    }
    const r2c = r2.choices?.[0];
    if (!r2c || r2c.finish_reason !== "stop") {
        throw new Error(`round2 finish_reason=${r2c?.finish_reason}; content=${r2c?.message?.content}`);
    }
    const content = (r2c.message?.content || "").trim();
    if (!content) throw new Error("round2 content empty");
    return content;
}

// ---------- vercel ai sdk path ----------

async function runWithVercel({ baseURL, apiKey, timeoutSec }, { model, stream, parallel }) {
    let createOpenAI, generateText, streamText, tool;
    try {
        ({ createOpenAI } = await import("@ai-sdk/openai"));
        ({ generateText, streamText, tool } = await import("ai"));
    } catch (e) {
        throw new Error(`vercel ai sdk not installed (npm install ai @ai-sdk/openai zod): ${e.message}`);
    }
    const { z } = await import("zod");

    const provider = createOpenAI({ baseURL, apiKey, compatibility: "compatible" });
    const tools = parallel
        ? {
            get_current_weather: tool({
                description: "Get the current weather of a city.",
                inputSchema: z.object({ city: z.string() }),
                execute: async ({ city }) => fakeWeather(city),
            }),
            fetch_latest_news: tool({
                description: "Fetch latest news for a topic.",
                inputSchema: z.object({ topic: z.string() }),
                execute: async ({ topic }) => fakeNews(topic),
            }),
        }
        : {
            get_current_weather: tool({
                description: "Get the current weather of a city.",
                inputSchema: z.object({ city: z.string() }),
                execute: async ({ city }) => fakeWeather(city),
            }),
        };
    const userPrompt = buildUserPrompt(parallel);
    const opts = {
        model: provider(model),
        prompt: userPrompt,
        tools,
        toolChoice: "auto",
        stopWhen: () => false, // 让 Vercel SDK 自动跑完工具结果 round-trip
        maxRetries: 0,
        abortSignal: AbortSignal.timeout(timeoutSec * 1000),
    };

    if (stream) {
        const result = streamText(opts);
        let text = "";
        for await (const chunk of result.textStream) text += chunk;
        if (!text.trim()) throw new Error("vercel streamText empty text");
        return text.trim();
    }
    const result = await generateText(opts);
    if (!result.text || !result.text.trim()) {
        throw new Error(`vercel generateText empty text; toolResults=${JSON.stringify(result.toolResults || []).slice(0, 200)}`);
    }
    return result.text.trim();
}

// ---------- runner ----------

async function runOne(label, fn) {
    const started = Date.now();
    try {
        const text = await fn();
        const elapsed = ((Date.now() - started) / 1000).toFixed(2);
        return { ok: true, label, detail: `OK in ${elapsed}s, final=${JSON.stringify(text.slice(0, 140))}` };
    } catch (e) {
        return { ok: false, label, detail: `exception: ${e?.name || "Error"}: ${(e?.message || String(e)).slice(0, 200)}` };
    }
}

async function main() {
    const cfg = loadConfig();
    console.log("aibalance Node E2E Tool Round-trip Matrix");
    console.log(`  base_url   = ${cfg.baseURL}`);
    console.log(`  models     = ${JSON.stringify(cfg.models)}`);
    console.log(`  skipVercel = ${cfg.skipVercel}`);
    console.log("");

    const streams = cfg.onlyStream ? [true] : [true, false];
    const toolModes = cfg.onlySingle ? [false] : [false, true];

    const results = [];
    for (const model of cfg.models) {
        for (const stream of streams) {
            for (const parallel of toolModes) {
                const ctx = { model, stream, parallel };
                const tag = `${model}/${stream ? "stream" : "non-stream"}/${parallel ? "parallel-tools" : "single-tool"}`;
                results.push(await runOne(`openai-npm/${tag}`, () => runWithOpenAI(cfg, ctx)));
                if (!cfg.skipVercel) {
                    results.push(await runOne(`vercel-ai-sdk/${tag}`, () => runWithVercel(cfg, ctx)));
                }
            }
        }
    }
    for (const r of results) {
        console.log((r.ok ? "PASS" : "FAIL"), `[${r.label}]`, r.detail);
    }
    const passed = results.filter(r => r.ok).length;
    console.log("");
    console.log(`Tool round-trip summary: ${passed}/${results.length} passed`);
    process.exit(passed === results.length ? 0 : 1);
}

await main();
