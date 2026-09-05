# Local protocol sample generator

Builds tiny identification PCAPs with Scapy (and loopback daemons when Docker
is available). Each capture is kept only if `tshark` matches the recorded
display filter.

```bash
export PATH="/Applications/Wireshark.app/Contents/MacOS:$PATH"
./.venv/bin/python generate.py
```

Output lands in `captures/generated-local/` plus `generated-index.json`.
The corpus generator then hashes frames without pretending the files came
from GitHub.
