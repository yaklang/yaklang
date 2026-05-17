// aibalance opencode tool_call 调查 - 反向代理双向 SSE 字节抓包工具.
//
// 替代 mitmproxy: 用 Node 内置 http 监听 18223, 把请求转发到 127.0.0.1:8223,
// 同时把 client->aibalance 与 aibalance->client 两个方向的完整字节流落盘到
// /tmp/aibalance_proxy_dump_<ts>/ 目录. 不依赖外部包.
//
// 用法:
//   node common/aibalance/cmd/debug_proxy_dump.mjs
//
// 环境变量:
//   AIBALANCE_PROXY_LISTEN     listen port (default 18223)
//   AIBALANCE_PROXY_TARGET     upstream host:port (default 127.0.0.1:8223)
//   AIBALANCE_PROXY_DUMP_DIR   dump root (default /tmp/aibalance_proxy_dump_<ts>)
//
// 抓到的字节布局:
//   <dump_dir>/index.json                  - manifest, [{id, ts, method, url, status}]
//   <dump_dir>/<id>.req.headers.json       - client request headers
//   <dump_dir>/<id>.req.body.bin           - client request body (raw bytes)
//   <dump_dir>/<id>.resp.headers.json      - aibalance response headers
//   <dump_dir>/<id>.resp.body.bin          - aibalance response body (raw bytes, SSE)
//
// 关键词: aibalance debug 反向代理, opencode SSE 抓包, 三层流对齐
//        no-mitmproxy node http reverse proxy

import http from "node:http";
import fs from "node:fs";
import path from "node:path";

function pad(n, width) {
    const s = String(n);
    return s.length >= width ? s : "0".repeat(width - s.length) + s;
}

function nowTs() {
    const d = new Date();
    return (
        d.getFullYear() + pad(d.getMonth() + 1, 2) + pad(d.getDate(), 2)
        + "_"
        + pad(d.getHours(), 2) + pad(d.getMinutes(), 2) + pad(d.getSeconds(), 2)
    );
}

const LISTEN = parseInt(process.env.AIBALANCE_PROXY_LISTEN || "18223", 10);
const TARGET = process.env.AIBALANCE_PROXY_TARGET || "127.0.0.1:8223";
const [TARGET_HOST, TARGET_PORT_STR] = TARGET.split(":");
const TARGET_PORT = parseInt(TARGET_PORT_STR || "8223", 10);
const DUMP_DIR = process.env.AIBALANCE_PROXY_DUMP_DIR
    || `/tmp/aibalance_proxy_dump_${nowTs()}`;

fs.mkdirSync(DUMP_DIR, { recursive: true });
const indexFile = path.join(DUMP_DIR, "index.json");
const manifest = [];
let counter = 0;
function nextId() {
    counter += 1;
    return pad(counter, 4);
}

function appendManifest(entry) {
    manifest.push(entry);
    fs.writeFileSync(indexFile, JSON.stringify(manifest, null, 2));
}

const server = http.createServer((clientReq, clientRes) => {
    const id = nextId();
    const ts = new Date().toISOString();
    const reqBodyChunks = [];
    const reqHeadersFile = path.join(DUMP_DIR, `${id}.req.headers.json`);
    const reqBodyFile = path.join(DUMP_DIR, `${id}.req.body.bin`);
    const respHeadersFile = path.join(DUMP_DIR, `${id}.resp.headers.json`);
    const respBodyFile = path.join(DUMP_DIR, `${id}.resp.body.bin`);

    fs.writeFileSync(reqHeadersFile, JSON.stringify({
        method: clientReq.method,
        url: clientReq.url,
        httpVersion: clientReq.httpVersion,
        headers: clientReq.headers,
    }, null, 2));

    const respBodyStream = fs.createWriteStream(respBodyFile, { flags: "w" });

    clientReq.on("data", (c) => {
        reqBodyChunks.push(Buffer.from(c));
    });

    clientReq.on("end", () => {
        if (reqBodyChunks.length > 0) {
            fs.writeFileSync(reqBodyFile, Buffer.concat(reqBodyChunks));
        }
    });

    const upstream = http.request({
        host: TARGET_HOST,
        port: TARGET_PORT,
        method: clientReq.method,
        path: clientReq.url,
        headers: { ...clientReq.headers, host: `${TARGET_HOST}:${TARGET_PORT}` },
    }, (upstreamRes) => {
        fs.writeFileSync(respHeadersFile, JSON.stringify({
            statusCode: upstreamRes.statusCode,
            statusMessage: upstreamRes.statusMessage,
            httpVersion: upstreamRes.httpVersion,
            headers: upstreamRes.headers,
        }, null, 2));
        // 注意: Node 已经按上游 transfer-encoding/content-length 解码了 body,
        // 这里再原样把 headers 写回客户端会让 Node 再做一次编码 -> 双重 chunked
        // 或长度不匹配. 必须剥掉这两个 hop-by-hop 头, 让 Node 自行决定.
        // 关键词: proxy_dump strip hop-by-hop, no double chunking
        const fwdHeaders = { ...upstreamRes.headers };
        delete fwdHeaders["transfer-encoding"];
        delete fwdHeaders["content-length"];
        delete fwdHeaders["connection"];
        clientRes.writeHead(upstreamRes.statusCode || 502, fwdHeaders);

        let respBytes = 0;
        upstreamRes.on("data", (c) => {
            respBytes += c.length;
            respBodyStream.write(c);
            clientRes.write(c);
        });
        upstreamRes.on("end", () => {
            respBodyStream.end();
            clientRes.end();
            appendManifest({
                id,
                ts,
                method: clientReq.method,
                url: clientReq.url,
                status: upstreamRes.statusCode,
                respBytes,
                reqBytes: reqBodyChunks.reduce((s, b) => s + b.length, 0),
            });
            console.log(`[proxy] ${id} ${clientReq.method} ${clientReq.url} -> ${upstreamRes.statusCode} (${respBytes}B resp)`);
        });
        upstreamRes.on("error", (err) => {
            respBodyStream.end();
            try { clientRes.end(); } catch (_) {}
            console.error(`[proxy] ${id} upstream resp error:`, err.message);
        });
    });

    upstream.on("error", (err) => {
        console.error(`[proxy] ${id} upstream connect error:`, err.message);
        try {
            clientRes.writeHead(502, { "content-type": "text/plain" });
            clientRes.end("upstream connect error: " + err.message);
        } catch (_) {}
        respBodyStream.end();
    });

    clientReq.pipe(upstream);
});

server.on("clientError", (err, socket) => {
    console.error("[proxy] clientError:", err.message);
    try { socket.destroy(); } catch (_) {}
});

server.listen(LISTEN, "127.0.0.1", () => {
    console.log(`[proxy] listening on 127.0.0.1:${LISTEN}, forward -> ${TARGET_HOST}:${TARGET_PORT}`);
    console.log(`[proxy] dump dir: ${DUMP_DIR}`);
});

process.on("SIGINT", () => {
    console.log("[proxy] SIGINT, manifest at", indexFile);
    server.close(() => process.exit(0));
});
process.on("SIGTERM", () => {
    console.log("[proxy] SIGTERM, manifest at", indexFile);
    server.close(() => process.exit(0));
});
