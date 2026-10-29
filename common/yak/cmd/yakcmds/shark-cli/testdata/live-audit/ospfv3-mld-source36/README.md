# OSPFv3 and MLD source auditors

The unchanged `source-manifest.json` binds 39 historical sources (38 Python and
one PowerShell, 354162 bytes) and their original external producer proofs.
Its SHA256 is `d9e4aaf3b52e1a7bc58d57912afde0b5c8d26cba25852770c47fbb9c79599497`.
The directory attributes preserve their raw audited bytes; do not reformat them.
`counts-report.json` describes source636 and independent MLD rechecks only.
Later UI685 and new boundary work require separate producer receipts.

The sources retain explicit external paths and source-specific contracts. Use
the manifest's roles, dependencies and recommended entrypoints before running
with `python -B -X utf8`. The prior NATT source archive supplies shared helpers;
its 38 sources are referenced by exact hash rather than copied again. Original
captures, keys and frozen source plans must be supplied separately. Any adapted
controller needs a new source hash and a fresh external output directory.

No capture, key, complete case/plan/expected object, export, PDML, image, binary
or log is included. Metadata contains counts and hash references only. Initial
contract01 and CLI path-key controller errors remain historical; the original
full36 run remains 285554 pass, 24 skip and 3 failures. Later targeted tests and
successful CI are recorded separately, never relabeling that execution.

See the [results and remaining scope](../../../../../../../../docs/exec-plans/shark-public-corpus-20261025/ospfv3-636-685.md).
