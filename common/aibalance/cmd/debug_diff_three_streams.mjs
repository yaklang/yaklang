// aibalance opencode tool_call 调查 - SSE / chat completion 流分析对齐工具.
//
// 输入: debug_proxy_dump.mjs 落盘的 dump 目录 (含 index.json 与每条请求的
//       <id>.{req,resp}.{headers,body}) - 也可以再加 --upstream <file> /
//       --opencode-log <file> 做三层对齐. 这里先支持 dump 目录本身的解析,
//       上游字节级抓包是可选项.
//
// 用法:
//   node common/aibalance/cmd/debug_diff_three_streams.mjs <dump_dir>
//   node common/aibalance/cmd/debug_diff_three_streams.mjs <dump_dir> \
//        --opencode-log /tmp/opencode_run.log
//
// 关键词: aibalance opencode tool_call diff, SSE 解析, finish_reason 对齐
//        role 重复检测, tool_calls 字段省略检测, usage 帧检测

import fs from "node:fs";
import path from "node:path";

const args = process.argv.slice(2);
if (args.length < 1) {
    console.error("usage: node debug_diff_three_streams.mjs <dump_dir> [--opencode-log <file>]");
    process.exit(2);
}
const dumpDir = args[0];
let opencodeLog = null;
for (let i = 1; i < args.length; i += 1) {
    if (args[i] === "--opencode-log" && args[i + 1]) {
        opencodeLog = args[i + 1];
        i += 1;
    }
}

if (!fs.existsSync(dumpDir)) {
    console.error(`dump dir not found: ${dumpDir}`);
    process.exit(2);
}
const indexFile = path.join(dumpDir, "index.json");
if (!fs.existsSync(indexFile)) {
    console.error(`index.json not found in ${dumpDir} - is the proxy still running? did it serve any request?`);
    process.exit(2);
}
const manifest = JSON.parse(fs.readFileSync(indexFile, "utf-8"));

function readBufferIfExists(file) {
    if (!fs.existsSync(file)) return null;
    return fs.readFileSync(file);
}

function parseRequestBody(buf) {
    if (!buf || buf.length === 0) return null;
    try {
        return JSON.parse(buf.toString("utf-8"));
    } catch (_) {
        return null;
    }
}

// 解析 SSE 流为 [{event, data, raw}]
function parseSse(buf) {
    if (!buf || buf.length === 0) return [];
    const text = buf.toString("utf-8");
    const events = [];
    const blocks = text.split(/\r?\n\r?\n/);
    for (const block of blocks) {
        if (!block.trim()) continue;
        let event = "message";
        const dataLines = [];
        const lines = block.split(/\r?\n/);
        for (const line of lines) {
            if (line.startsWith("event:")) {
                event = line.slice(6).trim();
            } else if (line.startsWith("data:")) {
                dataLines.push(line.slice(5).replace(/^\s/, ""));
            }
        }
        if (dataLines.length === 0) continue;
        const data = dataLines.join("\n");
        events.push({ event, data, raw: block });
    }
    return events;
}

function safeParseJSON(s) {
    try { return JSON.parse(s); } catch (_) { return null; }
}

function summarizeRound(req, sseEvents, respHeaders) {
    const requestSummary = {
        model: req?.model,
        stream: req?.stream === true,
        toolCount: Array.isArray(req?.tools) ? req.tools.length : 0,
        messageCount: Array.isArray(req?.messages) ? req.messages.length : 0,
        toolChoice: req?.tool_choice,
        hasAssistantToolCalls: false,
        hasRoleTool: false,
    };
    if (Array.isArray(req?.messages)) {
        for (const m of req.messages) {
            if (m.role === "assistant" && Array.isArray(m.tool_calls) && m.tool_calls.length > 0) {
                requestSummary.hasAssistantToolCalls = true;
            }
            if (m.role === "tool") {
                requestSummary.hasRoleTool = true;
            }
        }
    }
    requestSummary.kind = requestSummary.hasAssistantToolCalls || requestSummary.hasRoleTool ? "round2" : "round1";

    const ssestat = {
        eventCount: sseEvents.length,
        firstChunkHasRole: false,
        firstChunkHasContentField: null,
        contentChunks: 0,
        reasoningChunks: 0,
        toolCallChunks: 0,
        toolCallRoleChunks: 0,
        toolCallNoArgFieldFirstFrames: [],
        finishReason: null,
        donePresent: false,
        usagePresent: false,
        usageEstimated: null,
        toolCalls: new Map(), // index -> {id,type,name,argLen,framesNamed,framesArgs}
        rawDataChunks: 0,
        errorEvents: 0,
    };

    let chunkIdx = 0;
    for (const ev of sseEvents) {
        if (ev.data === "[DONE]") {
            ssestat.donePresent = true;
            continue;
        }
        const j = safeParseJSON(ev.data);
        if (!j) {
            ssestat.errorEvents += 1;
            continue;
        }
        ssestat.rawDataChunks += 1;
        const choice = (j.choices || [])[0] || {};
        const delta = choice.delta || {};
        if (chunkIdx === 0) {
            if (delta.role) ssestat.firstChunkHasRole = true;
            if ("content" in delta) ssestat.firstChunkHasContentField = "content";
            else if ("reasoning_content" in delta) ssestat.firstChunkHasContentField = "reasoning_content";
        }
        if (typeof delta.content === "string" && delta.content.length > 0) {
            ssestat.contentChunks += 1;
        }
        if (typeof delta.reasoning_content === "string" && delta.reasoning_content.length > 0) {
            ssestat.reasoningChunks += 1;
        }
        if (Array.isArray(delta.tool_calls) && delta.tool_calls.length > 0) {
            ssestat.toolCallChunks += 1;
            if (delta.role === "assistant") ssestat.toolCallRoleChunks += 1;
            for (const tc of delta.tool_calls) {
                const idx = tc.index ?? 0;
                let slot = ssestat.toolCalls.get(idx);
                if (!slot) {
                    slot = {
                        index: idx,
                        id: "",
                        type: "",
                        name: "",
                        argChars: 0,
                        framesNamed: 0,
                        framesArgs: 0,
                        firstNameFrameHasArgsField: null,
                        firstNameFrameHasEmptyArgsValue: null,
                    };
                    ssestat.toolCalls.set(idx, slot);
                }
                if (tc.id) slot.id = tc.id;
                if (tc.type) slot.type = tc.type;
                if (tc.function?.name) {
                    slot.name = tc.function.name;
                    slot.framesNamed += 1;
                    if (slot.firstNameFrameHasArgsField === null) {
                        slot.firstNameFrameHasArgsField = "arguments" in tc.function;
                        slot.firstNameFrameHasEmptyArgsValue = (tc.function.arguments === "");
                    }
                }
                if (typeof tc.function?.arguments === "string") {
                    slot.argChars += tc.function.arguments.length;
                    if (tc.function.arguments.length > 0) slot.framesArgs += 1;
                }
            }
        }
        if (choice.finish_reason) {
            ssestat.finishReason = choice.finish_reason;
        }
        if (j.usage) {
            ssestat.usagePresent = true;
            if (typeof j.usage.estimated === "boolean") {
                ssestat.usageEstimated = j.usage.estimated;
            }
        }
        chunkIdx += 1;
    }

    const toolCallsArr = Array.from(ssestat.toolCalls.values()).sort((a, b) => a.index - b.index);

    return {
        request: requestSummary,
        sse: {
            ...ssestat,
            toolCalls: toolCallsArr,
        },
    };
}

function summarizeRequest(entry) {
    const reqHdrFile = path.join(dumpDir, `${entry.id}.req.headers.json`);
    const reqBodyFile = path.join(dumpDir, `${entry.id}.req.body.bin`);
    const respHdrFile = path.join(dumpDir, `${entry.id}.resp.headers.json`);
    const respBodyFile = path.join(dumpDir, `${entry.id}.resp.body.bin`);
    const reqHdr = fs.existsSync(reqHdrFile) ? JSON.parse(fs.readFileSync(reqHdrFile, "utf-8")) : null;
    const reqBodyBuf = readBufferIfExists(reqBodyFile);
    const respHdr = fs.existsSync(respHdrFile) ? JSON.parse(fs.readFileSync(respHdrFile, "utf-8")) : null;
    const respBodyBuf = readBufferIfExists(respBodyFile);
    const reqJson = parseRequestBody(reqBodyBuf);
    const sseEvents = parseSse(respBodyBuf);
    const summary = summarizeRound(reqJson, sseEvents, respHdr?.headers);
    return {
        id: entry.id,
        ts: entry.ts,
        method: entry.method,
        url: entry.url,
        status: entry.status,
        reqBytes: reqBodyBuf?.length || 0,
        respBytes: respBodyBuf?.length || 0,
        respContentType: respHdr?.headers?.["content-type"] || null,
        ...summary,
    };
}

function flagSummary(s) {
    const flags = [];
    if (s.sse.toolCallChunks > 0) {
        flags.push(`tool_call_frames=${s.sse.toolCallChunks}`);
        const repeatedRole = s.sse.toolCallRoleChunks;
        if (repeatedRole > 1) {
            flags.push(`role-assistant-repeated-in-tool_calls(${repeatedRole}x) [!]`);
        }
        for (const tc of s.sse.toolCalls) {
            if (tc.firstNameFrameHasArgsField === false) {
                flags.push(`tc[${tc.index}](${tc.name}) first-name-frame missing function.arguments [!]`);
            }
            if (tc.firstNameFrameHasArgsField === true && tc.firstNameFrameHasEmptyArgsValue === false) {
                // arguments was sent as non-empty in first name frame, unusual
                flags.push(`tc[${tc.index}] first-name-frame arguments non-empty (uncommon)`);
            }
        }
    }
    if (s.sse.eventCount > 0 && !s.sse.firstChunkHasRole) {
        flags.push("first-chunk-missing-role-assistant [info]");
    }
    if (s.sse.finishReason === null && s.sse.eventCount > 0) {
        flags.push("missing finish_reason [!]");
    }
    if (s.request.kind === "round1" && s.sse.toolCallChunks === 0 && s.request.toolCount > 0) {
        flags.push("round1 with tools but no tool_calls emitted [?]");
    }
    if (s.request.kind === "round2" && s.sse.toolCallChunks > 0) {
        flags.push("round2 with tool_calls re-emitted (model still asking tools)");
    }
    if (s.sse.errorEvents > 0) {
        flags.push(`unparseable_data_events=${s.sse.errorEvents} [!]`);
    }
    return flags;
}

function renderTable(rows) {
    const cols = ["id", "kind", "stream", "tools", "msgs", "tcFrames", "tcRoleX", "fr", "usage", "done", "errs", "flags"];
    const lines = [];
    lines.push(cols.join(" | "));
    lines.push(cols.map(() => "---").join(" | "));
    for (const r of rows) {
        const cells = [
            r.id,
            r.request.kind,
            r.request.stream ? "y" : "n",
            r.request.toolCount,
            r.request.messageCount,
            r.sse.toolCallChunks,
            r.sse.toolCallRoleChunks,
            r.sse.finishReason || "(none)",
            r.sse.usagePresent ? (r.sse.usageEstimated === true ? "est" : "y") : "n",
            r.sse.donePresent ? "y" : "n",
            r.sse.errorEvents,
            flagSummary(r).join(", "),
        ];
        lines.push(cells.map(String).join(" | "));
    }
    return lines.join("\n");
}

function renderToolCallTrace(rows) {
    const out = [];
    for (const r of rows) {
        if (r.sse.toolCalls.length === 0) continue;
        out.push(`### request ${r.id} (${r.request.kind}) tool_calls trace`);
        out.push("");
        out.push("| index | id | type | name | argChars | framesNamed | framesArgs | firstNameFrameHasArgs | firstFrameArgsEmpty |");
        out.push("| --- | --- | --- | --- | --- | --- | --- | --- | --- |");
        for (const tc of r.sse.toolCalls) {
            out.push(`| ${tc.index} | ${tc.id} | ${tc.type} | ${tc.name} | ${tc.argChars} | ${tc.framesNamed} | ${tc.framesArgs} | ${tc.firstNameFrameHasArgsField} | ${tc.firstNameFrameHasEmptyArgsValue} |`);
        }
        out.push("");
    }
    return out.join("\n");
}

function scanOpencodeLog(file) {
    if (!file || !fs.existsSync(file)) return null;
    const text = fs.readFileSync(file, "utf-8");
    const lines = text.split(/\r?\n/);
    const interesting = [];
    const keywords = [
        "tool_call", "toolCall", "ToolCall",
        "finish_reason", "finishReason",
        "no tool", "without tool",
        "ai-sdk", "openai-compatible",
        "error", "ERROR", "fatal", "panic",
        "InvalidResponse", "InvalidJSON", "JSONParseError",
        "stop", "step",
    ];
    const re = new RegExp(keywords.join("|"));
    for (const l of lines) {
        if (re.test(l)) interesting.push(l);
    }
    return {
        totalLines: lines.length,
        matches: interesting,
    };
}

const summaries = manifest.map(summarizeRequest);
const opencode = scanOpencodeLog(opencodeLog);

const md = [];
md.push("# aibalance opencode tool_call diagnostic report");
md.push("");
md.push(`- dump_dir: ${dumpDir}`);
md.push(`- request_count: ${summaries.length}`);
md.push(`- opencode_log: ${opencodeLog || "(not provided)"}`);
md.push("");
md.push("## per-request summary");
md.push("");
md.push(renderTable(summaries));
md.push("");
md.push("## tool_calls trace");
md.push("");
md.push(renderToolCallTrace(summaries) || "_no tool_calls observed_");
md.push("");
if (opencode) {
    md.push("## opencode log highlights");
    md.push("");
    md.push("```");
    for (const l of opencode.matches.slice(0, 200)) md.push(l);
    md.push("```");
    md.push("");
    md.push(`(matched ${opencode.matches.length} / ${opencode.totalLines} lines)`);
}
md.push("");
md.push("## decision tree mapping");
md.push("");
const decisions = [];
for (const r of summaries) {
    const flags = flagSummary(r);
    if (r.request.kind === "round1" && r.request.toolCount > 0) {
        if (r.sse.toolCallChunks === 0) {
            decisions.push(`- ${r.id} round1: aibalance NOT emitting tool_calls -> aibalance side bug (callback / writer / extractor)`);
        } else if (flags.some(f => f.includes("[!]"))) {
            decisions.push(`- ${r.id} round1: aibalance emitted tool_calls but with format anomalies (${flags.filter(f => f.includes("[!]")).join("; ")}) -> client SDK schema mismatch suspect`);
        } else {
            decisions.push(`- ${r.id} round1: aibalance emitted tool_calls correctly -> client (opencode) side issue if no invoke`);
        }
    }
    if (r.request.kind === "round2") {
        decisions.push(`- ${r.id} round2: msgs=${r.request.messageCount}, tool_call_frames=${r.sse.toolCallChunks}, finish=${r.sse.finishReason}`);
    }
}
md.push(decisions.length > 0 ? decisions.join("\n") : "_no diagnostic mapping inferred_");
md.push("");

const reportFile = path.join(dumpDir, "diag_report.md");
fs.writeFileSync(reportFile, md.join("\n"));
console.log(`report written: ${reportFile}`);
console.log("");
console.log(md.join("\n"));
