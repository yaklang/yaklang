import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


# Docker path by default; locally redirect via CHALLENGE_AUDIT_LOG so the
# verifier can read the same file without root privileges.
AUDIT = Path(os.environ.get("CHALLENGE_AUDIT_LOG", "/var/log/challenge-audit.jsonl"))
STATE = {"token_requests": 0}


class Handler(BaseHTTPRequestHandler):
    def write_json(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def audit(self, path, status):
        with AUDIT.open("a") as handle:
            handle.write(
                json.dumps(
                    {
                        "path": path,
                        "status": status,
                        "attempt": STATE["token_requests"],
                    }
                )
                + "\n"
            )

    def do_GET(self):
        if self.path == "/health":
            self.write_json(200, {"status": "ok"})
            return
        if self.path == "/":
            body = (
                "<html><h1>Recovery API Lab</h1>"
                "<p>Goal: retrieve the recovery token from <code>/api/token</code>.</p>"
                "<p>The upstream is known to return two HTTP 503 responses while "
                "warming up. Retry a small, bounded number of times.</p></html>"
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if self.path == "/api/token":
            STATE["token_requests"] += 1
            if STATE["token_requests"] <= 2:
                self.audit(self.path, 503)
                self.write_json(503, {"error": "upstream warming", "retryable": True})
            else:
                self.audit(self.path, 200)
                self.write_json(200, {"token": "LAB-RETRY-7C91"})
            return
        self.write_json(404, {"error": "not found"})

    def log_message(self, *_):
        return


AUDIT.parent.mkdir(parents=True, exist_ok=True)
AUDIT.write_text("")
# Docker defaults to 8080; locally CHALLENGE_PORT picks a free port and the
# runner rewrites the instruction's 127.0.0.1:8080 accordingly.
_PORT = int(os.environ.get("CHALLENGE_PORT", "8080"))
ThreadingHTTPServer(("0.0.0.0", _PORT), Handler).serve_forever()

