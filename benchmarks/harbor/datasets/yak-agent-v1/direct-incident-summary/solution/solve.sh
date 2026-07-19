#!/usr/bin/env bash
set -euo pipefail

python3 - <<'PY'
import json
from pathlib import Path

result = {
    "account": "svc-export",
    "source_ip": "10.20.1.7",
    "successful_endpoint": "/admin/export",
    "classification": "credential-stuffing",
    "evidence": ["evt-103", "evt-104"],
}
Path("/app/result.json").write_text(json.dumps(result, indent=2) + "\n")
PY

