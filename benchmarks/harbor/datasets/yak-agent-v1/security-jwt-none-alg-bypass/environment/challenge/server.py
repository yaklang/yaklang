import base64
import hashlib
import hmac
import json
import os
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


# Deterministic signing secret. In a real deployment this would live in a
# secrets manager; here it is hard-coded purely so the lab is reproducible.
SECRET = b"lab-hs256-secret-2026"

# Docker path by default; locally redirect via CHALLENGE_AUDIT_LOG so the
# verifier can read the same file without root privileges.
AUDIT = Path(os.environ.get("CHALLENGE_AUDIT_LOG", "/var/log/challenge-audit.jsonl"))

ADMIN_FLAG = "FLAG-ADM-7421"
ISSUED_FOR = "analyst-2049"


def b64url_encode(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def b64url_decode(segment: str) -> bytes:
    pad = "=" * (-len(segment) % 4)
    return base64.urlsafe_b64decode(segment + pad)


def make_hs256(payload: dict) -> str:
    header = b64url_encode(
        json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode()
    )
    body = b64url_encode(json.dumps(payload, separators=(",", ":")).encode())
    signing_input = f"{header}.{body}".encode()
    signature = hmac.new(SECRET, signing_input, hashlib.sha256).digest()
    return f"{header}.{body}.{b64url_encode(signature)}"


def verify_token(token: str):
    """Return (payload, alg_label, reason).

    Deliberately flawed: a JWT whose header advertises ``alg=none`` is accepted
    with no signature check, mirroring the classic CVE-2015-9235 family of
    misimplementations. The HS256 path remains correct.
    """
    parts = token.split(".")
    if len(parts) != 3:
        return None, None, "malformed"
    header_b, body_b, sig_b = parts
    try:
        header = json.loads(b64url_decode(header_b))
        payload = json.loads(b64url_decode(body_b))
    except Exception:
        return None, None, "bad-encoding"
    if not isinstance(header, dict) or not isinstance(payload, dict):
        return None, None, "bad-encoding"
    alg = header.get("alg", "")
    if not isinstance(alg, str):
        return None, None, "bad-alg"
    if alg.lower() == "none":
        # Lab flaw: an unsigned token is trusted as-is.
        return payload, "none", "accepted-none"
    if alg.upper() == "HS256":
        signing_input = f"{header_b}.{body_b}".encode()
        expected = hmac.new(SECRET, signing_input, hashlib.sha256).digest()
        try:
            provided = b64url_decode(sig_b)
        except Exception:
            return None, "HS256", "bad-signature"
        if hmac.compare_digest(expected, provided):
            return payload, "HS256", "ok"
        return None, "HS256", "bad-signature"
    return None, alg, "unsupported-alg"


class Handler(BaseHTTPRequestHandler):
    def send_json(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def send_html(self, status, body):
        raw = body.encode()
        self.send_response(status)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def audit(self, path, alg, role, status, reason=""):
        with AUDIT.open("a") as handle:
            handle.write(
                json.dumps(
                    {
                        "path": path,
                        "alg": alg,
                        "role": role,
                        "status": status,
                        "reason": reason,
                    }
                )
                + "\n"
            )

    def bearer_payload(self):
        auth = self.headers.get("Authorization", "")
        if not auth.startswith("Bearer "):
            return None, None, None
        token = auth[len("Bearer "):].strip()
        payload, alg, reason = verify_token(token)
        return token, payload, (alg, reason)

    def do_GET(self):
        if self.path == "/health":
            self.send_json(200, {"status": "ok"})
            return
        if self.path == "/":
            body = (
                "<html><h1>Acme Internal Portal</h1>"
                "<p>Issue yourself a session at <code>GET /api/login</code>.</p>"
                "<p>Inspect your profile at <code>GET /api/me</code> "
                "(send the token as <code>Authorization: Bearer &lt;jwt&gt;</code>).</p>"
                "<p>Administrators may pull the runtime flag from "
                "<code>GET /api/admin/flag</code>.</p>"
                "<p><a href='/docs'>API documentation</a></p></html>"
            )
            self.send_html(200, body)
            return
        if self.path == "/docs":
            self.send_json(
                200,
                {
                    "login": "GET /api/login -> {token}",
                    "me": "GET /api/me (Authorization: Bearer <jwt>)",
                    "admin_flag": "GET /api/admin/flag (Authorization: Bearer <jwt>, role=admin)",
                    "token_format": "compact JWT (header.payload.signature)",
                    "note": "Sessions are HS256-signed by the gateway.",
                },
            )
            return
        if self.path == "/api/login":
            now = int(time.time())
            payload = {
                "sub": ISSUED_FOR,
                "role": "analyst",
                "iat": now,
                "exp": now + 3600,
            }
            token = make_hs256(payload)
            self.audit("/api/login", "HS256", "analyst", 200, "issued")
            self.send_json(200, {"token": token, "role": "analyst"})
            return
        if self.path == "/api/me":
            token, payload, (alg, reason) = self.bearer_payload()
            if payload is None:
                self.audit("/api/me", alg, None, 401, reason or "no-token")
                self.send_json(401, {"error": "invalid token"})
                return
            self.audit("/api/me", alg, payload.get("role"), 200, reason or "ok")
            self.send_json(
                200,
                {"sub": payload.get("sub"), "role": payload.get("role")},
            )
            return
        if self.path == "/api/admin/flag":
            token, payload, (alg, reason) = self.bearer_payload()
            role = payload.get("role") if payload else None
            if payload is None:
                self.audit("/api/admin/flag", alg, role, 401, reason or "no-token")
                self.send_json(401, {"error": "invalid token"})
                return
            if role != "admin":
                self.audit("/api/admin/flag", alg, role, 403, "non-admin")
                self.send_json(403, {"error": "admin role required"})
                return
            self.audit("/api/admin/flag", alg, role, 200, reason or "ok")
            self.send_json(
                200,
                {
                    "flag": ADMIN_FLAG,
                    "classification": "restricted",
                    "issued_for": payload.get("sub"),
                },
            )
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
