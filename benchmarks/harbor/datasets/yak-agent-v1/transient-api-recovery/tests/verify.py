import json
from pathlib import Path


scores = {"outcome": 0.0, "evidence": 0.0, "format": 0.0}
try:
    data = json.loads(Path("/app/recovered.json").read_text())
    if isinstance(data, dict) and isinstance(data.get("attempts"), int):
        scores["format"] = 1.0
    if data.get("token") == "LAB-RETRY-7C91" and data.get("attempts", 0) >= 3:
        scores["outcome"] = 1.0
except (OSError, ValueError, TypeError):
    data = {}

try:
    entries = [
        json.loads(line)
        for line in Path("/var/log/challenge-audit.jsonl").read_text().splitlines()
    ]
    statuses = [entry.get("status") for entry in entries if entry.get("path") == "/api/token"]
    if statuses[:3] == [503, 503, 200]:
        scores["evidence"] = 1.0
except (OSError, ValueError):
    pass

scores["reward"] = round(
    scores["outcome"] * 0.7 + scores["evidence"] * 0.2 + scores["format"] * 0.1,
    4,
)
Path("/logs/verifier/reward.json").write_text(json.dumps(scores, sort_keys=True))

