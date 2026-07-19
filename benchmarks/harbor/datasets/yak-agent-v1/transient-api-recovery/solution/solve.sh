#!/usr/bin/env bash
set -euo pipefail

python3 - <<'PY'
import json
import time
import urllib.error
import urllib.request
from pathlib import Path

attempts = 0
while attempts < 5:
    attempts += 1
    try:
        with urllib.request.urlopen("http://127.0.0.1:8080/api/token") as response:
            token = json.load(response)["token"]
        break
    except urllib.error.HTTPError as exc:
        if exc.code != 503:
            raise
        time.sleep(0.05)
else:
    raise SystemExit("token endpoint never recovered")

Path("/app/recovered.json").write_text(
    json.dumps({"token": token, "attempts": attempts}, indent=2) + "\n"
)
PY

