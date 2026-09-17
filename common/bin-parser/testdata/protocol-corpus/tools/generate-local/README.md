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

`corrections.py` writes separate positive companions to
`captures/generated-validated/`; it never changes the original generated
captures. Run it with the same Scapy environment. These companions cover
malformed originals and additional precisely scoped layouts. For example,
the original `gen-6to4` bytes are IPv6-in-IPv4 but do not have a 6to4 address;
that original is retained as `6in4`.

Use repeatable `--only` options to add a companion without rewriting earlier
ones. Unknown names fail before writing any capture. With no option the script
retains its full deterministic regeneration behavior.

```bash
./.venv/bin/python corrections.py --only gen-ncp-valid --only gen-sebek-valid
```

Matching a display filter is only an identification check. The Go corpus
tests additionally check lengths, actual field values, truncation, stateful
sequences and independent byte-level expectations. Neither local recipe is
counted as upstream evidence. Recipe hashes and capture digests are recorded
in `sources.json` and verified before regenerating the manifest.
