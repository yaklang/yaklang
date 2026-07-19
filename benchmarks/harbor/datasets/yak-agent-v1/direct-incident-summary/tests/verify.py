import json
from pathlib import Path


path = Path("/app/result.json")
scores = {"outcome": 0.0, "evidence": 0.0, "format": 0.0}
try:
    data = json.loads(path.read_text())
    if isinstance(data, dict):
        scores["format"] = 1.0
    expected = {
        "account": "svc-export",
        "source_ip": "10.20.1.7",
        "successful_endpoint": "/admin/export",
        "classification": "credential-stuffing",
    }
    if all(data.get(key) == value for key, value in expected.items()):
        scores["outcome"] = 1.0
    if {"evt-103", "evt-104"}.issubset(set(data.get("evidence", []))):
        scores["evidence"] = 1.0
except (OSError, ValueError, TypeError):
    pass

scores["reward"] = round(
    scores["outcome"] * 0.7 + scores["evidence"] * 0.2 + scores["format"] * 0.1,
    4,
)
Path("/logs/verifier/reward.json").write_text(json.dumps(scores, sort_keys=True))

