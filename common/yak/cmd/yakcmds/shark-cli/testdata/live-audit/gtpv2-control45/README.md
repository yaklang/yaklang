# GTPv2 source and execution controllers

The 39 source files retain their executed bytes and SHA256 values in
`source-manifest.json`. The external Go probe is stored as `.go.txt` so this
archive does not introduce another package. Existing capture readers, literal
comparers and the Windows console helper are pinned as repository dependencies.

The source census, official-reference audit, independent boundary constructor,
API leaf checker, private native/CLI checker, default-budget Windows checker and
fuzz runners have separate entry points. Native/CLI execution requires explicit
closed build receipts and a frozen external plan. The controller self-test uses
source-derived synthetic exports and rejects 12 injected faults without starting
candidate processes.

Reproduction requires the original external phase directory and the hash-bound
capture/plan files named by the controllers. Those inputs, full expected objects,
native/CLI exports, PDML, logs, binaries and console images remain outside Git.
Historical failed checker/IPC attempts remain recorded externally; successful
checkers preserve their source-byte and expected-field contracts.

Production integration is in `f1068e2` and the receive-length/helper follow-up
`df30fc4`. Aggregate counts and explicit coverage limits are documented in
`docs/exec-plans/shark-public-corpus-20261025/gtpv2-df30.md` and its JSON summary.

The observed profile covers bounded EPC headers, TLIV/group93 and selected
Recovery/EBI/Cause/F-TEID values. Mandatory procedure semantics, NAS, subscriber,
bearer/session ownership, non-EPC interfaces and peer authentication remain
unverified. Repeated modes are not additional independent packet samples.
