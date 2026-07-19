import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


# Docker path by default; locally redirect via CHALLENGE_AUDIT_LOG so the
# verifier can read the same file without root privileges.
AUDIT = Path(os.environ.get("CHALLENGE_AUDIT_LOG", "/var/log/challenge-audit.jsonl"))
ORDERS = {
    1001: {
        "order_id": 1001,
        "tenant": "blue",
        "owner": "Blue Retail",
        "status": "standard",
    },
    1002: {
        "order_id": 1002,
        "tenant": "red",
        "owner": "Red Aerospace",
        "status": "exception",
        "report_code": "EXC-Q4-8841",
    },
}


class Handler(BaseHTTPRequestHandler):
    def send_json(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def audit(self, order_id, status):
        with AUDIT.open("a") as handle:
            handle.write(
                json.dumps(
                    {
                        "path": self.path,
                        "order_id": order_id,
                        "tenant_token": self.headers.get("X-Tenant-Token", ""),
                        "status": status,
                    }
                )
                + "\n"
            )

    def do_GET(self):
        if self.path == "/health":
            self.send_json(200, {"status": "ok"})
            return
        if self.path == "/":
            body = (
                "<html><h1>Northwind Customer Orders</h1>"
                "<p>Review your order at <code>/api/orders/1001</code>.</p>"
                "<p>Send <code>X-Tenant-Token: tenant-blue</code>.</p>"
                "<p><a href='/docs'>API documentation</a></p></html>"
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if self.path == "/docs":
            self.send_json(
                200,
                {
                    "endpoint": "GET /api/orders/{order_id}",
                    "header": "X-Tenant-Token",
                    "note": "Legacy numeric order identifiers are still supported.",
                },
            )
            return
        if self.path.startswith("/api/orders/"):
            try:
                order_id = int(self.path.rsplit("/", 1)[1])
            except ValueError:
                self.send_json(400, {"error": "invalid order id"})
                return
            if self.headers.get("X-Tenant-Token") != "tenant-blue":
                self.audit(order_id, 401)
                self.send_json(401, {"error": "invalid token"})
                return
            order = ORDERS.get(order_id)
            if not order:
                self.audit(order_id, 404)
                self.send_json(404, {"error": "order not found"})
                return
            # Deliberate lab flaw: object tenant is never compared to token tenant.
            self.audit(order_id, 200)
            self.send_json(200, order)
            return
        self.send_json(404, {"error": "not found"})

    def log_message(self, *_):
        return


AUDIT.parent.mkdir(parents=True, exist_ok=True)
AUDIT.write_text("")
# Docker defaults to 8080; locally CHALLENGE_PORT picks a free port and the
# runner rewrites the instruction's 127.0.0.1:8080 accordingly.
_PORT = int(os.environ.get("CHALLENGE_PORT", "8080"))
ThreadingHTTPServer(("0.0.0.0", _PORT), Handler).serve_forever()

