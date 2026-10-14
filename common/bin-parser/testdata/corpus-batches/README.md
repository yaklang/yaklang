# Versioned traffic corpus

Capture-backed tests read password ZIP batches through
[`internal/trafficfixture`](../../../../internal/trafficfixture). Password:
`bin-parser`. This is a packaging convention, not protection for credentials.
No additional ZIP library, download or extraction step is needed for Go tests.

`index.json` pins each ZIP SHA256, every member's SHA256 and expanded size,
and historical path aliases. Indexed inputs always come from the ZIP; a loose
copy cannot shadow them. All reads return owned bytes. Filename-only APIs use
verified copies in test-owned temporary directories, removed by `t.Cleanup`.

The baseline seals 694 captures (690 unique content hashes) and their original
answers from `2fb090d177c3bf52277f240b2dca05ea80216485`. The source ZIP SHA256 is
`2ac6c0e1861b3d9487230fd8d89106780c2f856e67c2873fe99c270d0b92b234`.
The repository batch preserves every capture byte and original path.
The auxiliary batch preserves remaining binary inputs, manifests and endpoint
evidence. Provenance, fixed upstream commits, licenses and evidence categories
remain inside these archives. Artificial and upstream fixtures do not become
claims of real capture merely because they are archived.

The supporting-materials batch seals 86 further files from `2598e4d6aafbd369a46be8c71aff3cbc799cebdb`:
generators and their tests, capture/build recipes, validator scripts, its original
107-test inventory, endpoint configs, six public test certificates, five
generated loopback session keylogs and the minimized Go fuzz regression seed.
The quality runner resolves the tested checkout through `YAK_TRAFFIC_REPO_ROOT`;
its companion validator and inventory still come from the verified workspace.
Original generator bytes, versions, configs and file paths remain available.
Independent Go oracle code and Go regression tests remain reviewable source.

The sealed answer inventory has 622 partially mapped and 72 unmapped cases.
Its executable assertions include 434 selected-input parses, 102 selected-input
rejections and 30 exact whole-capture protocol/status count maps. A selected
frame's assertions are not a golden for every message in the capture.
`TestFrozenCorpusSelectedInputs` consumes the sealed field/error assertions;
`TestFrozenCorpusReplay` executes every capture, checks full/deferred fields,
errors, associations, stream offsets and resource release, and compares sealed
event counts where available. Replay parity alone is not independent semantics.
The existing endpoint, field, negative and worker/callback/chunk regression
tests keep their stronger assertions while reading the same archived inputs.

Run from the repository root:

```sh
go run ./internal/trafficfixture/cmd/corpus test
go test -json -count=1 -timeout=10m ./internal/trafficfixture/... ./common/bin-parser/... ./common/pcapx/... ./common/yak/cmd/yakcmds/shark-cli ./scripts/ci > traffic.jsonl
go run ./internal/trafficfixture/cmd/corpus exec -- python3 @scripts/protocol-tests/check_go_test_json_v2.py traffic.jsonl --inventory @scripts/protocol-tests/required-tests-traffic-unified-v83.json --tier full
```

The single `corpus` entry point uses the same bounded Go ZIP loader as tests.
`test` validates Python/shell/Node source, executes the validator and builder
unit tests, and compiles archived Go recipes without running capture generators,
Docker or downloads. It uses existing `go`, `python3`, `bash` and `node` binaries;
it never installs dependencies. Child failure remains a command failure.

`exec -- COMMAND` resolves `@repository/path` arguments to verified private
copies and removes the workspace afterwards; other arguments and the caller's
working directory retain their usual meanings. `exec --workspace -- COMMAND`
runs from that private workspace. `export NEW_DIRECTORY` creates a persistent
private workspace for the archived reproduction instructions and Go builders;
existing output directories and symlinks are rejected. Exports include original
path aliases, module files, the small current Go source dependencies and an
`EXPORT-MANIFEST.json` with every file's hash. Expanded exports are bounded to
384 MiB. No tool automatically starts capture or regenerates accepted answers.

```sh
go run ./internal/trafficfixture/cmd/corpus verify
go run ./internal/trafficfixture/cmd/corpus export /tmp/new-traffic-workspace
go run ./internal/trafficfixture/cmd/corpus exec -- python3 @scripts/protocol-tests/run-quality-v2.py quick --output /tmp/new-quality-result
```

Add future captures and answers as a **new** batch. Do not rewrite old ZIPs,
replace old aliases, regenerate answers from the implementation under test,
or turn an unmapped oracle into a passing semantic result. Preserve fixed
source/version, content hash, format, packet counts, license, evidence category,
independent expected fields/errors and their validation scope. Deduplicate by
content hash and preserve distinct path aliases. Keep new batches below 25 MiB
compressed and each member below 25 MiB expanded; the loader also caps expanded
batch size and rejects unsupported encryption, traversal and duplicate members.
New answer-bearing batches use `validation/expected.json` schema 1 and enter the
frozen gates automatically. Add adapters for new assertion kinds before claiming
their semantic coverage. The existing synthetic session keys are test inputs;
production credentials, user private keys and material without redistribution
permission belong in an external fixed-source download manifest.

Current test requirements are sealed at `scripts/protocol-tests/required-tests-traffic-unified-v83.json`;
the historical inventories and all earlier batches remain unchanged.
The supplemental small-corpus batch contains 26 minimal controls, their explicit
expected facts, an offline generator and the compact pinned source manifest.
Two Modbus cases are sanitized derivatives of a licensed upstream resegmentation;
its attribution and changes are in `validation/sources.json`. The coalesced case
has a recorded tshark 4.2.5 field oracle; that version does not recognize the
seven-byte initial prefix. Full/deferred, workers 1/2/4, observer parity and split
session feeds are asserted by `TestSmallCorpusSealedControls`; LOGINACK also has
byte-boundary, malformed-length and ownership tests. These are scoped control
assertions, not a semantic golden for the entire upstream collection. Process
logs and the original large downloads remain outside the repository.

Historical inventory v40 remains available; the active local acceptance and published gate use v47. v26 adds Call requirements, v27 adds scalar arrays and v28 adds extended builtin fields and v29 adds bounded value graphs and complete diagnostic wire fields; v30 adds message-local RTPS INFO_SRC/INFO_DST context, v31 bounded SPDP fragments and owned answer-cache requirements, and v32 legacy RTPS2.0 DATA wire, budget, lifecycle and ownership requirements. Older
versioned inventories stay available as historical inputs; they are not the
current acceptance entry. The DLMS HDLC batch contains owned tunnel/Get-normal
controls and their independent fields/errors, CRC/byte validator, offline
generator and pinned source metadata. Its tests assert complete selected
messages, association, sequence reuse and disconnection, across TCP chunks and
UDP capture domains. It does not promote the received duplicate DLMS capture
with invalid FCS or same-direction UDP replies to a successful semantic golden.

The selected OpenDroneID v2 batch seals 63 owned UDP controls (11 positive and
52 expected refusals), their complete ordered field/error answers and offline
generation/verification tools. Pinned Apache-2.0 `opendroneid-core-c` reference
sources, attribution and a local build harness reproduce the recorded Basic ID,
Location and selected-pack answers using an existing C compiler. This carrier
is explicitly selected raw UDP, not BLE/Wi-Fi Remote ID or authenticated aircraft
identity. The received `ODID`-prefixed attachment remains external and is not
a valid positive. The separate `opendroneid-system` UDP selection adds version-2
System observations, alone or exactly once alongside up to two Basic IDs and
one Location. Its new batch preserves 37 minimal controls and full scalar
answers from the same pinned C implementation, including coordinate/height
quantization, classifications, area and unsigned time. Framing refusals remain
selected-profile boundaries, without an aircraft, geography or authentication
claim. The original `opendroneid` selection and all its answers remain unchanged.
The `opendroneid-identity` selection adds Self ID and Operator ID wire
observations, with optional Basic ID/Location/System messages. NUL-terminated
reference-codec byte content is distinct from complete original wire bytes;
non-ASCII values, tails after NUL and reserved fields remain visible. Its 45
controls reuse four captures from the System batch and seal 41 new captures
with independent answers and reproduction scripts. Identity, charset,
registration, authentication and broadcast transport are not inferred.
The mandatory gate also checks that a native UDP NeedMore
event increments Incomplete rather than Malformed, using an earlier ZIP input.

The selected C37.118 v1/v2 batch seals 55 owned TCP/UDP controls (26 positives
and 29 refusals or missing-context boundaries) with complete raw field/error
answers, independent byte/CRC validation and a fixed tshark 4.2.5 PDML check.
The native profile decodes CFG-2 and multiple PMU DATA blocks using each observed
publisher direction and capture domain. It handles configuration changes in any
PMU, rejects unusable configuration and bounded resource overload, and owns its
returned fields. Integer/raw IEEE floating-point values are retained; no
engineering measurement-validity claim is made. CFG-1 does not establish DATA
context. This historical batch validates CFG-2; the complete CFG-3 profile is described below.
Pinned pypmu reference failures for nonzero DIGUNIT masks are retained in the
archive and do not count as passing independent reference runs.

The current industrial link batch includes 61 SV/EtherCAT and TCP generation controls, plus 38 corrected carrier cases sharing 37 additional capture hashes. Historical synthetic ACK=0 carriers and their original application answers remain immutable; current application regressions bind the corrected carriers to those same answers, and separately verify that invalid reverse carriers are discarded. Rebuild and independent verification tools are sealed in that batch.

The selected Wrapper v1 GET-with-list batch contains 51 original logical controls,
50 unique captures and complete source-informed byte/field/error answers with
generation and independent validation scripts. Native tests bind each answer and
input hash to the inventory, cover TCP/UDP and all12 worker/deferred/observer
configurations, and retain resource/context/ownership assertions. These synthetic
controls do not prove negotiated conformance, authentication, object/selector
meaning or real meter behavior. That original batch records the pre-supplement
structured Data boundary; the supplement below adds bounded array/structure
Data for GET-with-list. Other services remain open.

The structured GET-with-list supplement seals 28 logical controls with 26 new capture hashes, one historical capture reused and one internal content alias. All positive and refusal answers bind complete input bytes; TCP chunking, ordered response association, aggregate Data limits, atomic refusal and nested ownership are tested. The pinned Gurux source stays external; the archived reference harness verifies its hashes and executes the client, Data codec and list receiver with an existing Python runtime. Python three-byte-count and empty-container receiver differences remain explicit, not passing reference results. A loopback UDP witness demonstrates that byte-identical replies after an idle reset lack an observable generation discriminator; observed matching is not device transaction proof.

The bit-string supplement adds A-XDR Data tag4 to the bounded Wrapper GET-with-list
profile, including nested values and selective parameters. Its 26 original controls
check bit counts versus value bytes, MSB-first semantics, exact cursors, retained
unused low bits, local byte limits and complete observed Session/ID snapshots.
The archived independent harness executes the pinned Gurux Data codec and list
receiver; its three-byte-count rejection remains an explicit reference difference.
GET-normal bit-string, HDLC, GBT, negotiated association, security and
meter attribute interpretation remain separate requirements.

The floating Data supplement adds tags23/24 (IEEE binary32/binary64) only to
Wrapper GET-with-list, including nested values. It retains exact value bytes,
sign, width and zero/subnormal/normal/infinite/NaN class. Finite values are numeric;
nonfinite values use `NaN`, `Infinity` or `-Infinity` strings for JSON-safe output.
Raw bytes retain NaN payloads and negative zero. The 29 original controls bind
complete fields, Session, IDs and refusals, with independent pinned codec/list
receiver execution. These observations do not assert units or meter validity.

PFCP v1 UDP session-deletion UsageReport observations and strict whole-field/session association controls are preserved in `validation-pfcp-usage-6e3f9ce9481.zip`. Selected usage fields are syntax observations; URR configuration, actual metering and device operation remain unverified. The current integrated inventory includes these controls.

Classic PCAP length metadata remains strict by default. The explicit offline `WithLegacyPcapLengthNormalization(true)` option normalizes original lengths smaller than captured lengths while preserving the original scalar in `PcapOriginalLength`. Snaplen, allocation, actual payload and PCAPNG bounds remain strict; live/native capture and PacketAnalyzer reject this option. Synthetic record metadata controls and the validator are in `validation-pcap-legacy-d5e218e32eea.zip`. This proves container compatibility, not original application/session semantics.

The integrated GET-normal block profile observes Wrapper v1 unciphered canonical
GET-next/GET-response-with-data-block exchanges. It assembles encoded Data only
from an observed confirmed initial GET-normal, block1 and each consecutive next
request/response in the same direction, wPort and invoke context. TransactionID
retains the original request; ResponseTo identifies each immediate next request.
Missing or duplicate hops retire association and bytes. The selected bounds are
64 blocks, 1024 aggregate encoded bytes, 256 Data nodes and8 Data levels, further
limited by the caller budget. Partial transfers remain outstanding until Close;
refusals retire context. GET-with-list block results additionally preserve ordered
Data/access-error results and assembled-body offsets, with count equality against
the observed initial descriptors. The additive 35-case list batch covers mixed
results, interleaved invoke IDs, exact aggregate byte/node boundaries and typed
refusals, with a pinned external receiver harness. GBT, HDLC segmentation and
negotiated AA/security/object/device semantics remain pending. The small synthetic batch
contains whole fields/state/ID answers and an external fixed-reference receiver
harness. All prior inventories and ZIPs remain immutable; the current unified
inventory includes DLMS, PFCP usage and core capture controls.

The current whole-field PFCP UsageReport validator is `common/pcapx/pcaputil/pfcp-verifier/verify-controls.py`, run with `corpus exec -- python3 @...`. It reuses the immutable usage inputs and answers, preserves response-only field shape, and validates both historical metadata schemas without dropping any field assertions. The original validator remains archived as historical material.

The additive normal selected-access batch contains31 author-owned minimal case
identities, complete custom/default-budget answers and deterministic builder,
byte verifier and pinned-reference harnesses. It observes canonical selection1,
opaque selector bytes and one bounded Data value over Wrapper v1 TCP/UDP; object
selector meaning, access permission and device operation are unverified. The
original one-packet selective-access capture and original refused answer remain
immutable; its now-supported answer is matched by exact input SHA and frame.
The active inventory is `scripts/protocol-tests/required-tests-traffic-unified-v83.json`.

The small-graph budget batch reuses accepted graph captures/answers and adds one
249-byte UDP control for child count255 without child bytes. Its standalone proof
checks materialized node counts against available tag bytes; the absolute256-node
and8-level limits stay unchanged. Malformed graphs retain typed errors and cannot
revive a previously observed request. Tools reuse the hash-bound author oracle in
the normal selected-access batch; no third-party source or repeated captures are
packaged.

The selected calendar Data supplement seals 39 controls (38 new content hashes,
one immutable historical truncated capture reused) for Wrapper v1 normal, list,
and optional selection parameters. Complete component and ordered conversation
answers distinguish literal values, unspecified fields, special month/day markers,
raw signed deviation, and status bytes. Concrete Gregorian dates are checked;
unselected ranges stay explicit profile refusals. It does not normalize an
observed timestamp, apply host timezone rules, or prove meter clock accuracy.
The pinned external Gurux reader is actually exercised: weekday loss, time
fraction scaling and pre-1900 normalization are recorded as reference differences,
not full semantic agreement. Reference source and execution logs stay external.

The current compact-array batch adds 66 owned minimal controls with 177 complete
message/field/session/ID answers. They cover Wrapper v1 normal/list results,
normal/list selection parameters, observed normal/list block assembly, strict
contents/schema boundaries, node/row limits and adjacent calendar carriers.
The fixed Gurux codec executes 16 distinct positive compact values: 11 agree
completely and five retain explicit reference diagnostics (root arrays, nested
long-form structure counts, nullable columns, UTF-8 and fixed-array row shape).
Official Wireshark v4.6.8 grammar review is recorded separately from execution;
the installed v4.2.5 emitted no DLMS fields and is not counted as a reference
pass. Owned independent answers retain complete captured bytes, fields and
context. No object access permission, device association or configured selector
meaning is inferred. Raw upstream code and reference logs remain external.

The native HDLC GET-with-list supplement seals46 logical controls with45 new
unique captures and one internal capture reuse. Whole ordered field/Session/ID,
CRC, direction, count, retry, budget and ownership answers are bound to the
inventory. GET-normal/link controls retain their historical contracts and inputs.
The UDP byte-limit regression retires pending requests before a late response;
full invoke priority/service flags and service choice/count must match the
observed request. This profile handles complete, unsegmented type3 TCP/UDP
tunnel frames and bounded list Data; ACSE, authentication/ciphering, segmented
HDLC, block/GBT, negotiated capabilities and meter object semantics remain open.
Pinned external Gurux framing/client/list-receiver checks are reproducible using
the archived harness; reference calendar precision and UTF-8 representation
diagnostics remain distinct from complete reference value agreements.

The Semtech correlation supplement is an opt-in passive PULL_RESP/TX_ACK profile.
Thirteen owned minimal captures bind173 full message/field/Session/ID answers,
including17 expected context/format/resource refusals. Historical downstream
field observations remain stateless. Tokens are capture-lifetime quarantined
after completion, collision, timeout or refusal; byte-identical pending duplicates
retain the original request ID. Scope excludes authentication, gateway identity
binding, device execution and RF delivery. Original large captures, upstream
source and logs are not bundled.

`validation-dlms-hdlc-normal-data.zip` contains31 owned minimal native normal-GET capture cases and72 ordered independent answers (53 complete byte/field observations,19 expected refusals). It covers TCP/UDP full/deferred, workers1/2/4, observer off/on, TCP feed chunks, nested compact, node/depth boundaries, malformed/trailing contents, strict text, full flags and refusal followed by late feedback. The existing native scalar contract and zero TransactionID are retained. Its source pins and executable independent builder/oracle/reference harness stay in the small ZIP; original reference sources and execution logs stay outside Git. Actual fixed reference codec display/calendar differences remain diagnostics rather than device passes. Current mandatory inventory is `scripts/protocol-tests/required-tests-traffic-unified-v34.json`.

The historical `dlms-hdlc/array` capture is reused in place. Its original `UnsupportedFeature` answer remains immutable; `historical-array-upgrade.json` binds the original capture/answer hashes to two current full-message answers for the now-supported legal empty-array response. No historical capture is copied into the new ZIP and other historical refusals retain their assertions.

`validation-dlms-hdlc-normal-access.zip` seals38 minimal captures and92 ordered
independent answers (64 observations,28 typed refusals) for native normal-GET
selection parameters. It includes exact byte/CRC/context oracles, owned builders,
the fixed reference harness and currentv13 inventory. TCP/UDP, retry/collision,
wrong direction/full flags, truncated/malformed parameters, exact allocation
boundaries, node/depth limits, ownership, chunks and Close release are asserted.
The fixed external client produces12 exact selected APDUs; the framer separately
reproduces29 distinct complete HDLC frames. Eleven distinct parameter values
agree completely with its codec. UTF-8 display and the reference XML handler's
UTF-8 error remain explicit diagnostics rather than successful field agreements.
These are envelope/parameter observations, not selector semantics or meter tests.

`validation-dlms-postcompletion-refusal.zip` binds52 logical native HDLC
scenarios to50 unique minimal captures and252 ordered whole-message answers
(160 literal/context observations and92 expected typed refusals). It preserves
adjacent traffic while preventing late response reassignment after completed
exchanges, parser/resource refusals, unseen peer progression or future/half-space
RR acknowledgements. Both ordinary and modulo8-wrap sequences, TCP/UDP,
full/deferred, workers1/2/4, observation callbacks, ownership and chunks are
covered; shared-byte exact boundaries and capture section/interface isolation
are separately asserted. Two logical byte-limit profiles share existing members
in this batch rather than storing duplicate captures. The independent builder,
byte/context oracle, fixed-version external framer harness and currentv14
inventory are sealed; upstream source, logs and raw large collections stay
outside Git. No device or authenticated-generation claim is made.

`validation-dlms-hdlc-blocks.zip` contains112 distinct owned minimal native HDLC
TCP/UDP captures and944 whole independent answers:798 literal field/ordered
association observations and146 expected typed refusals. It covers normal/list
Get-next chains, full invoke flags and logical addresses, missing/repeated/gapped
blocks, premature continuation, modulo8 wrap,64/65 block and1024/1025 byte
boundaries, malformed/unsupported bodies, ownership, domain isolation, budgets
and unfinished Close. Fixed Gurux commit6e409ae9f7eb1aca3a03a9270bd03a3201408870
actually regenerated183 distinct complete frames and decoded42 positive APDU
block chains including ordered values and Get-next acknowledgements. This is an
APDU codec/framer reference check, not full native receiver or meter execution;
its permissive pairing and last-only list error are not independent proof of our
selected negative association policy or every item error. Owner byte/state
oracles supply those explicit expectations. No raw download, upstream source,
dependency, log or cache is included. All56 preceding ZIPs remain unchanged.
The current inventory is `scripts/protocol-tests/required-tests-traffic-unified-v34.json`.

`validation-dlms-hdlc-unobserved-next.zip` adds12 distinct minimal captures and54
whole answers (36 observations,18 expected ContextRequired refusals). An already
completed ordinary/normal-block/list-block exchange followed by Get-next without
an observed initial block makes the retained native conversation uncertain. The
later request and late block must have no ResponseTo or Session. Six adjacent
exchanges retain their exact complete field/ID assertions. The existing-API
725092fb16fa0c0a136a81913a93aadb08172b4c unpublished candidate actually attached a
late response to request ID4; the refusal guard was fixed before publication.
The supplemental owner state oracle and14 actual pinned APDU receiver executions
are separate evidence; no reference/device validation of missing-context pairing
is claimed. The former block ZIP remains immutable and its v15 inventory remains
archived; the current v16 inventory includes the supplemental regressions.

`validation-dlms-hdlc-segments.zip` adds56 distinct minimal native HDLC captures
and452 whole byte/state answers (400 observations,52 expected typed refusals).
It observes up to64 consecutive response information frames and1027 joined
information bytes after a complete unciphered LN GET request. The first frame
contains LLC and the complete service header; later frames carry continuation
bytes without another LLC. Partial observations have no ResponseTo; only the
complete APDU consumes the observed request. Final captured header, CRCs, Raw
and NS/NR remain literal and the joined information is separately labelled.
Normal, list and selected Data-block response bodies reuse their existing
bounded decoders. Duplicate last fragments, gaps, wrap, RR, full flags, wrong
direction/address, orphan progression, budgets, Close and owned projections
are asserted. All58 earlier ZIPs stay unchanged. The actual pinned native HDLC
receiver succeeds on8 chains;4 list/block executions expose its premature
partial-service decode errors and remain explicit reference diagnostics, not
semantic agreement. Owner byte/state answers cover all56 controls. Segmented
initial requests, GBT, negotiated link/AA/security and meter success remain
unfinished requirements. This batch preserves its archived unifiedv17 inventory;
the current executable entry is identified below.

`validation-dlms-udp-idle.zip` contains12 owned minimal UDP captures and34 ordered
whole-message answers:26 literal field observations and8 typed context refusals.
They check29/30/31second boundaries, completed/request reuse, delayed identical
responses and backward capture timestamps. All carry complete fields, Session,
per-event TransactionID/ResponseTo and packet order. The independent validator
actually executes two self-owned loopback endpoints that release requestA's
response after identical requestB; expiry in the fixtures uses logical time,
not an unrecorded31second network delay or device claim. Historical expiry
capture/answer hashes bind its corrected association answer; no old ZIP is
modified. Shared-protocol expiry, collection limits, domain isolation, byte
ownership and Close release also have regressions. Current executable inventory
is `scripts/protocol-tests/required-tests-traffic-unified-v34.json`; earlier
versioned inventories remain archived.


`validation-dlms-udp-adjacent.zip` adds18 content-unique minimal UDP captures
and86 whole byte/field/context answers (78 observations,8 typed refusals).
A fully completed exchange retains bounded identity history after idle: selected
native HDLC continues only observed NS/NR progression without reusing an
invoke/sequence pair; Wrapper v1 permits only an unused logical-endpoint/invoke
token. Used Wrapper tokens stay retired and charged at256 bytes each plus the
512-byte conversation slot. A delayed response, wrong direction/flags or old
request cannot acquire the new request identity. An unfinished request, response
segment or Data-block expiry still needs capture-lifetime quarantine. No timed
expiry establishes a new authenticated/negotiated peer generation.
The existing mandatory public-API, budget and matrix roots include these cases;
the416-root unifiedv20 inventory was the executable entry for that batch. These
18 captures run216 full/deferred, worker1/2/4 and observer-off/on combinations,
in addition to the immutable12-capture idle matrix. Owner byte/CRC/context
answers and actual self-owned loopback ordering are separate from the fixed
Gurux codec's three actual APDU values; neither claims device or authentication
success. All60 preceding ZIPs and original answers remain unchanged.

`validation-opendroneid-auth-pages-51c136.zip` adds explicit UDP
`opendroneid-auth-pages`: version2 Authentication page observations, exact page-zero
capacity/time and full17/23-byte data, ordered repeated and mixed packs. These
are opaque observations; no signature verification, aircraft identity, page
assembly or standard BLE/WiFi transport is inferred. The existing mandatory
`TestOpenDroneIDSealedReplay` and `TestOpenDroneIDBoundsOwnershipAndAdmission`
cover44 controls with independent fixed official C codec fields and typed
refusals;41 captures are new and three reuse immutable System inputs. The ZIP
contains answers, source pins and generation/reference-build/verify/seal scripts.

`nmea` also observes complete unit-labelled VTG sentences with optional
A/D/E/M/N/S or empty FAA mode. Both courses and both reported speed units are
literal fields; rounding, missing data or mode N cannot establish a valid GNSS
fix. The older unlabelled form, later mode extensions and AIS remain separate
open work. `TestNMEAMultipleSentencesBudgetAndOwnership/vtg-profile` loads51
sealed controls (21 positives/30 refusals) in all12 worker/deferred/observer
configurations, with complete fields/Session and immutable queries. Historical
GGA depth-one refusal, depth-two normal, recovery and Close use the original
input/answer; no historical ZIP is rewritten. Primary receiver grammar and
pinned GPSD3.25 `processVTG` source-function execution are distinct from real
receiver validation. Authored captures, answers and required generation,
reference-build, byte-verification and seal scripts reside in the small ZIP.

The `nmea-gsa/controls.json` entry adds twelve-slot GSA observations with literal
manual/automatic selection, reported fix type, ordered PRN spelling/presence,
DOP spelling/presence and optional System ID0..6. Duplicate slots stay duplicate;
blank fields do not become zero and sentinel DOP is not validated quality.
`TestNMEAMultipleSentencesBudgetAndOwnership/gsa-profile` checks53 independent
answers in full/deferred, workers1/2/4 and observer off/on configurations, plus
collection/depth refusal, recovery and Close. `nmea-gsa/source-pin.json` fixes the
primary/reference versions; the shipped reference runner takes the fixed GPSD
source path as its argument and executes mode/DOP assignment only. It does not
supply constellation mapping, receiver context, vendor extensions, complete GSV cycles/AIS or
physical position verification.

The `nmea-gsv/controls.json` entry preserves each GSV sentence's declared count,
part number, ordered zero-to-four satellite slots, exact spelling and nullable
SV/elevation/azimuth/signal-level values. Optional Signal ID is literal hexadecimal
(4.10 form); no constellation or signal-level unit is inferred. Sparse nonfinal
parts, duplicates and mixed talkers stay separate observations, not an assembled
sky picture. `TestNMEAMultipleSentencesBudgetAndOwnership/gsv-profile` checks68
independent answers across12 configurations, plus slot/message/depth budgets,
recovery, Close and data ownership. The small ZIP contains generation, byte
verification and reference-build scripts. `nmea-gsv/source-pin.json` pins Trimble,
u-blox F9 HPS1.20 R04 and GPSD3.25; actual GPSD function execution uses explicit
identity mapping shims. Its decimal Signal ID and complete-picture filtering
differences are retained in reference records. The original Trimble eighth
example's bad checksum is a refusal; its repaired variant is separately named.
Complete-cycle association and receiver validity remain open work.

The `nmea-ais/controls.json` entry covers complete single-fragment AIVDM/AIVDO
Class A position reports (types1/2/3), with74 independently answered cases in
12 replay configurations. Raw 168-bit fields, unavailable sentinels, scaled
position/rate/course, timestamp status and conditional SOTDMA/ITDMA fields stay
literal observations. Sequence and channel labels do not establish association
or authenticated ship identity. Multipart reports and other message types remain
unsupported in this profile. The small ZIP includes minimal PCAPs, complete
answers and generation/byte-validation/reference-build scripts. Primary pins
identify ITU-R M.1371-5 (2014) and framing-reference versions;44 payload vectors
were executed against unmodified, pinned Apache-2.0 libais sources kept outside
the archive. That codec validates binary field extraction, not envelope checks,
native refusal policy, receiver operation or physical accuracy. Resource tests
also check typed byte/sentence/depth refusal, recovery, Close and query ownership.

MAVLink common UDP supports HEARTBEAT, SYS_STATUS, GPS_RAW_INT, ATTITUDE and
GLOBAL_POSITION_INT. Signatures remain opaque by default. The optional
`WithProtocolMAVLinkSigning` / `WithSessionMAVLinkSigning` receiver context
requires a copied 32-byte key and an independent 48-bit receiver clock (10us
since 2015-01-01 UTC), and refuses unsigned messages, bad digests and replay.
History is bounded by stream and shared retained-byte limits until Close;
streams share identity across endpoint pairs within a capture domain, while
interfaces, sections and tunnels isolate their clocks/history. Entire datagrams
commit atomically. `TestMAVLinkV2ZeroTailAndOpaqueSignature/signing` consumes the
small `mavlink-signing` ZIP and official fixed C receiver answers, verifies
complete fields/Session and ownership, and runs the capture matrix. The public
synthetic test key is not a device credential. Verified bytes do not establish
physical telemetry, endpoint identity, provisioning or persistence across captures.

AIS Type5 static/voyage data supports exactly two ordered AIVDM/AIVDO
fragments within one UDP datagram, with matching literal sentence/sequence/channel
labels, both XOR checksums and 424-bit payload validation. Raw text, padding,
dimensions, ETA, draught and DTE are retained with unavailable/lower-bound
markers. `TestNMEAMultipleSentencesBudgetAndOwnership/ais-type5-datagram`
loads the `nmea-ais-type5` ZIP containing minimal complete answers and scripts;
the fixed external libais codec checks six payload vectors, including ignored
armoring fill bits and text termination with nonzero trailing data. Separate UDP
datagrams are never concatenated. These fields do not prove ship identity,
voyage, physical dimensions, calendar ETA or successful receiver operation.

The `nmea-ais-classb/controls.json` entry adds single-fragment Type18 Class B
168-bit reports. It preserves all navigation raw values, unavailable/lower-bound
markers, reserved bits and literal unit/display/DSC/band/Message22/assigned/RAIM
flags. For SOTDMA/ITDMA units it exposes only the selected radio-state branch;
CS units retain the 19-bit fill and its standard-pattern comparison without
misinterpreting it as a time slot. Undefined navigation values retain the
historical `UnsupportedFeature` admission boundary. Reported capabilities and
coordinates are not authenticated or independently established device facts.
The small sealed batch supplies complete field/Session answers and reproducible
synthetic inputs; its scripts check all fields/branches against fixed libais
`87dadfaa1b4db0b0e9953680051391f2c4375455` using separately hash-pinned sources.
No upstream source, large downloads, runtime logs or compiled codecs are stored
in the ZIP. The Type18 batch does not assemble reports across datagrams.

The `nmea-ais-static/controls.json` entry observes Type24 Part A and Part B
independently. Part A accepts its 160-bit and 168-bit forms; Part B requires
168 bits. A short Part B is rejected without rewriting its literal Part Number.
Part B preserves both the old seven-character vendor representation and the
M.1371-4/5 manufacturer/model/serial layout, without inferring equipment version.
Auxiliary MMSI prefix 98 selects the raw mothership MMSI; other prefixes select
dimension fields and their unavailable/lower-bound markers. Equal reported MMSIs
do not establish entity identity or authorize joining parts across datagrams.
`TestNMEAMultipleSentencesBudgetAndOwnership/ais-static-profile` loads three
small grouped captures with 32 independently specified datagram scenarios,
including 15 refusals and a co-packed pair that stays as two independent reports.
Each scenario runs full/deferred, workers 1/2/4 and callbacks off/on. Fixed libais
checks the old codec's common fields; its short-Part-B relabel tolerance and
legacy vendor/dimension interpretation are explicitly distinguished from the
versioned normative bit-layout answers. Raw physical identity, receiver operation
and authentication remain unverified. No upstream sources or build outputs are
included in the sealed ZIP.

The `nmea-ais-extended/controls.json` entry adds exact single-fragment 312-bit
Type19 extended Class B reports. It preserves raw/scaled navigation fields and
unavailable markers, name wire text and first-`@` termination, dimensions and
their unavailable/lower-bound markers, raw ship/fix type, reserved bits and
literal RAIM/DTE/assigned flags. Type19 has no Class A navigation-status/ROT
or Type18 radio-state fields. Shapes outside the exact profile remain
`UnsupportedFeature`, retaining the historical short mutated-Type18 refusal;
invalid envelopes remain `MalformedMessage`. The selected profile does not
authenticate name, dimensions, physical fix, receiver operation or ship identity.
`TestNMEAMultipleSentencesBudgetAndOwnership/ais-extended-profile` loads three
small grouped captures with 34 datagram scenarios: 17 complete observations
and 17 normative refusals, all in the twelve replay configurations. The sealed
scripts check 13 complete vectors against fixed libais and build all inputs
independently of this implementation. Original archives and answers stay intact.


`validation-nmea-ais-session.zip` adds explicit `nmea-ais-multipart` UDP
observation of Type5 across exactly two ordered whole AIVDM/AIVDO sentences.
Nonempty digit/channel labels, directed endpoints and capture domain delimit
context. Default `nmea` still never joins separate datagrams. This opt-in profile
retains complete424-bit Type5 fields, both canonical physical contributors,
fragment event IDs and private field/Session snapshots. It sets no request/reply
IDs and never verifies a sender, generation, receiver operation or vessel identity.
An exact active first-fragment retry does not extend the30second local deadline.
Expired/completed/conflicting/refused labels remain quarantined for the capture;
untracked malformed, truncated or budget-denied context fails closed. History,
projection bytes, element/depth and32-contributor limits stay bounded. Close
reports an outstanding first fragment and releases all charged storage.

The24 declarative scenarios reuse one capture for different memory budgets:
23 content-unique PCAPs total7612 bytes, ten complete observations and selected
pending/refusal/close events. All24 run12 full/deferred, worker1/2/4 and
observer-off/on combinations with complete fields, Session, event IDs, direction,
contributors, typed errors and byte accounting. Capture truncation remains an
expected reader error. The sealed generator reuses the fixed historical Type5
payload answer; no new reference device/session execution is claimed. Original
archives remain immutable. That batch used unifiedv20 (416 unique full-tier roots); its inventory and
prior v18 source materials remain archived.


AIS Type27 long-range position observations
-----------------------------------------

`validation-nmea-ais-longrange.zip` seals3 minimal synthetic captures and33
ordered datagram answers (14 full observations,19 typed codec refusals).
The fixed ITU-R M.1371-5 Annex8 Table84 defines exactly96 payload bits,
signed coordinates in1/10 minute, SOG in integer knots and COG in integer
degrees. Defined unavailable values retain their raw values with no scaled
projection. Bit94 reports position latency relative to5 seconds; it does
not prove GNSS validity. No timestamp, heading or radio-state is inferred.
Noncanonical repeat/spare values remain literal observations with explicit
conformance flags. AIVDM/AIVDO, literal sequence/channel labels and co-packed
complete sentences preserve the normal stateless native Session contract.

Ten independent normative vectors were actually decoded by hash-pinned
libais; all selected raw fields and scaled coordinates were checked. Its
inverse field named `gnss` is used only to check the raw latency bit; the
ITU field meaning is authoritative. The reference source stays outside Git;
the small ZIP contains fixed hashes, independent answers, deterministic
builders and the reference validation adapter. The profile validates full
fields/Session, all prefixes, unavailable/boundary/reserved values, refusals,
ownership, projection budgets and12 full/deferred/workers1/2/4/observer
combinations. This is no physical position, vessel identity or device check.
All historical archives remain byte-identical; that batch used unifiedv20.

`validation-c37118-context.zip` adds seven owned minimal captures (5,608 raw
bytes) and 47 independent ordered event answers for C37.118 v1/v2 CFG-2/DATA.
PMU/channel collection refusal retires the replaced publisher context; a later
affordable CFG-2 restores it. UDP idle expiry uses the conversation's own
monotonic capture clock, preserving other interfaces and peers. Accepted field
and Session queries use private snapshots even after public map replacement.
The tests cover full/deferred, workers 1/2/4, observer off/on, exact raw fields,
directions, flow identities, typed errors, packet evidence and Close release.
These are raw codec and local capture-policy proofs; they do not establish
engineering-unit accuracy, physical measurement validity, device success or
native CFG-3 support by this historical context-only batch. Complete and TCP-fragmented CFG-3 are covered by the following batches; applied scaling and physical measurement validation remain open.


`validation-c37118-cfg3.zip` contains21 owned minimal captures and47 ordered
independent answers for C37.118-2011 version2 complete CFG-3 (CONT_IDX=0) and
DATA from the same observed publisher, stream ID and version. Profile
`c37118-v2-cfg3-complete-data` retains UTF-8 station/channel names, RFC UUID
bytes, raw phasor/analog conversion factors, digital masks, coordinate binary32
bits, service class, signed window/group delay, nominal flags and revision.
Conversion factors are observations; they are not applied engineering scaling.
The matrix covers TCP/UDP, full/deferred, workers1/2/4 and observer off/on;
standalone TCP controls additionally use1/3/14/16/255-byte feeds. It asserts
complete fields, private Session queries, raw IEEE values including NaN/Inf,
direction/ID/version isolation, collection-refused replacement/recovery,
typed malformed UTF-8/length/CRC and Close release. That complete-only batch records fragmented CFG-3 as an unsupported boundary.
The following batch adds bounded TCP continuation assembly. Reference endpoint
and device execution, applied scaling and physical quality remain unverified.
Scripts, independently authored byte codec, fixed GPA/GSF source pins and
full answers are sealed inside the small ZIP. Original references and logs
stay outside Git. That batch used unifiedv22 (424 unique roots).


`validation-c37118-cfg3-continuation.zip` adds25 owned TCP controls (23 unique
capture byte sequences, with two repeated inputs reused) for version2 CFG-3 in
fixed GPA/GSF `22a9aa1028c6e53d115c4ea81026055a57b803c2` producer framing.
Profile `c37118-v2-cfg3-continuation-data` accepts a65535-byte first frame,
consecutive65535-byte middle frames and a final CONT_IDX=65535 frame. It keeps
the original first header for cumulative CRC, including a CRC split across a
17-byte final frame, and installs DATA configuration only after full validation.
Publisher directions, capture interfaces, stream IDs and SOC/FRACSEC do not
borrow configuration. A30-second capture-time lifetime, collection and logical
byte budgets and conservative memory reservation bound pending state. Every TCP
message advances only its publisher's monotonic capture watermark, including
DATA without usable configuration; backdated tails and new first fragments
cannot revive expired state. Denied,
expired or inconsistent replacements clear the affected publisher context.
Pending fragments report ContextRequired with Cumulative CRC Validated=false;
this is an explicit diagnostic, not a successful configuration. The matrix
compares98 ordered independent event answers, full/deferred, workers1/2/4
and observation off/on, including rejected cases. Standalone feed boundaries,
caller/query ownership, incomplete Close and idempotent resource release are
also required. Packet contributor lists respect the configured collection
ceiling; buffer-state refusals retain their established context-required capture
status and typed ResourceExceeded error. The executed independent Python codec
checks cumulative CRC, exact configuration and raw DATA; reading fixed reference
source is not actual reference endpoint or device execution. UDP continuation,
engineering scaling, source authentication and physical measurement validity
remain open. Only owned MVP captures, answers and deterministic scripts are
sealed; original references, downloads and logs stay outside Git. Current
entry for that continuation batch was unifiedv23 (428 unique full-tier roots);
its inventory and ZIP remain immutable.

`validation-c37118-engineering.zip` adds an explicit `c37118-engineering`
UDP selection and `WithProtocolC37118Engineering` / `WithSessionC37118Engineering`
for configured C37.118 DATA quantities. The default native view retains raw
numeric encodings. The numerical view covers CFG-2 phasor and frequency values,
CFG-3 phasor magnitude/angle and analog multiplier/adder, literal GPA/GSF status
flags, and digital normal/valid masks. Angles use radians; fixed signed-minimum
phasor/frequency encodings are tagged undefined, and nonfinite values stay tagged.
CFG-2 integer analog conventions require independent context and return
`ContextRequired`; digital label-to-bit association and physical validity are
not inferred. The fixed GPA/GSF source review and executed owned Python oracle
are separate evidence from reference endpoint or device execution, which has not
been performed. Twenty-one prior captures are reused by pinned alias and SHA256;
only the additional minimal owned inputs, independent answers and build scripts
are stored in this batch. `TestC37118EngineeringSealedReplay` checks all raw and
derived fields and Session across full/deferred, workers 1/2/4 and observers
on/off. Raw fields and structural values compare exactly; finite derived
quantities permit eight representable floating-point values of cross-runtime
rounding, without an absolute tolerance near zero. The current mandatory entry
is unifiedv31 (1093 mandatory full-tier entries).

The media-owned-queries supplement contains three selected UDP RTCP BYE controls
using two unique 100-byte PCAPs: one-source observation, inconsistent two-source
count, and the same valid wire under a refused retention budget. Complete accepted
fields/session and typed refusal answers are sealed with original offline scripts.
Wireshark 4.2.5 independently decoded the positive wire and reported the count
mismatch malformed. Association and resource answers assert this library's passive
observation contract, not device authentication or complete compound-RTCP compliance.
`TestRTSPMediaFinalSemanticCacheParity` includes all 12 worker/deferred/observer
combinations and verifies that caller edits cannot rewrite accepted queries or
private refusals; packet-reference ownership and exact logical history-budget
boundaries are also mandatory. Historical ZIPs remain unchanged.

The OPC UA BrowseNext supplement (`validation-opcua-browsenext.zip`) contains
39 minimal owned captures, complete independent field/session answers, pinned
codec regeneration and generation scripts. `DecodeOPCUABrowseNext` exposes
None-policy services533/536, ordered literal continuation bytes and complete
selected BrowseResult fields. Release replies require empty Results and
DiagnosticInfos. Token validity, authenticated sessions and successful address
space traversal are not inferred. Shared codec/capture helpers and the preceding
Read/Browse/Write inputs are reused by immutable aliases and SHA256.

`TestOPCUASelectedSequenceReplayExistingAPI` and
`TestOPCUASelectedSequenceIsolation` cover stale responses after completed ID
reuse, retirement and the unchanged default envelope observer. Selected
application profiles reject repeated/backwards sequences; forward capture gaps
and legacy rollover remain observable. None-policy sequence consistency is not
cryptographic replay protection. The sealed BrowseNext matrix checks full and
deferred decoding, workers1/2/4, packet observers off/on and TCP chunks0/1/7,
including rollover boundaries and token changes without a sequence reset.
Historical ZIPs and their answers remain unchanged.


The OPC UA Call supplement (`validation-opcua-call.zip`) contains 46 original
minimal captures and complete fixed-codec projections, positive and typed
negative answers, and pinned generation/verification scripts. `DecodeOPCUACall`
observes None-policy 712/715 selected headers, plain ObjectId/MethodId, ordered
scalar arguments and per-method results. It checks observed direction, channel,
service, request ID, handle, result cardinality and input argument result counts
per Method. Output cardinality is not inferred from input cardinality. Bad
operation results must not carry output arguments; Uncertain outputs remain
observable. The shared sequence guard, bounded chunks, abort/Close, allocation
refusal retirement, late response isolation and owned outputs apply. Complete
fields and association are checked across full/deferred, workers1/2/4,
observer off/on and TCP chunks0/1/7. This is wire observation, not Method
metadata/signature validation, authorization or endpoint invocation success.
Array/structured Variants, custom headers, nonempty diagnostics and protected
bodies remain explicit unsupported boundaries. Historical ZIPs are unchanged;
shared scripts are reused through content hashes rather than copied again.


The shared Variant array supplement (`validation-opcua-variant-arrays.zip`)
contains 116 original minimal inputs for selected Read, Write and Call messages.
`DecodeOPCUAVariantArrays` explicitly extends those selected profiles with arrays
of their supported scalar types. Wire order, null versus empty arrays, exact
64-bit values, floating-point bits and flattened multidimensional shapes are
retained. Dimensions require rank at least two, positive lengths and a product
matching the flattened element count; count/shape/truncation/resource boundaries
are rejected without unbounded allocation or multiplication overflow. A Method
argument containing an array remains one argument. Complete fields, association,
errors, packet contributors and owned queries are checked through full/deferred,
workers1/2/4, observer off/on and TCP chunks0/1/7. The default scalar-only behavior
is preserved. Nested Variant/ExtensionObject arrays and unsupported built-in
types remain explicit refusals; wire dimensions do not verify server ValueRank,
IndexRange results, method signatures, authorization or device success.


The extended Variant supplement (`validation-opcua-extended-variants.zip`)
contains 153 Read/Write/Call scenarios, with150 new minimal captures and3
immutable historical inputs reused by alias and SHA256. QualifiedName namespace
and name, LocalizedText presence/locale/text, and ExpandedNodeId URI/server
fields are observed in scalar and array values. A URI-bearing ExpandedNodeId
retains its literal wire namespace index and explicitly marks that index
ignored; an empty URI with the URI flag is rejected. XmlElement is retained as
UTF-8 wire text without XML parsing or entity resolution. Future type IDs26..31
use the specified opaque ByteString fallback and are tagged as observations of
unknown future meaning. Null and empty values remain distinct. Exact fields,
Session, association, order, errors, source identities, ownership and allocation
boundaries are checked across full/deferred, workers1/2/4, observer off/on and
TCP chunks0/1/7. Fixed asyncua v1.1.8 codec execution is recorded separately from
normative answers: its XML codec and null future scalar variants fail in this
version, so those cases use independent normative ByteString/tag construction,
not a claim of reference-codec success. No namespace resolution, XML grammar
validity, authentication or device invocation success is inferred. Nested
Variants/ExtensionObjects and remaining professional protocol functionality
remain open; historical ZIPs and answers are unchanged.


The structured-value supplement (`validation-opcua-structured-values.zip`)
contains80 selected scenarios,77 new minimal captures and3 reused historical
inputs. `DecodeOPCUAStructuredValues` enables bounded nested DataValues, arrays
of Variants, DiagnosticInfo and opaque ExtensionObject bodies for selected
services. Full diagnostics include independent mask/wire-order checks, response
string-table bounds and resolved fields, absent/null context refusal and a
separate ten-level diagnostic recursion ceiling. Value graphs also respect the
configured ParserBudget depth, aggregate collection and allocation limits.
`WithSessionOPCUAObservation(OPCUAObservationConfig{...})` exposes the same
service selection through standalone sessions while retaining their budget,
event ownership and Close semantics; UDP/session configuration conflicts reject.
Resource/context refusal retires selected associations; late responses do not
match retired requests. Read/Write/Call and adjacent Browse/BrowseNext use full
and deferred decoding, workers1/2/4, observer off/on and TCP chunks0/1/7.
Fixed reference DiagnosticInfo mask and recursive-serialization gaps are
recorded separately; normative diagnostics are not reported as successful
reference codec execution. Encoded ExtensionObject bodies, including custom
AdditionalHeader content, remain explicitly opaque observations rather than
custom schema, authentication or endpoint success. Historical batches remain
unchanged.

`validation-rtps-receiver-context.zip` adds 34 minimal authored captures and reuses one historical SPDP capture. RTPS 2.1–2.5 INFO_SRC and INFO_DST affect subsequent selected discovery/lifecycle DATA only within that UDP message. INFO_SRC clears the prior timestamp; destination UNKNOWN stays explicitly unknown in passive observations. Outer header fields and earlier DATA remain unchanged. The archived constructor and verifier reproduce complete independent wire/field answers, including short context, unsupported source version, datagram isolation, ownership and exact element/byte budgets. This does not implement DATA_FRAG, topic schemas, reliable writer state or DDS Security.

`validation-rtps-fragments.zip` contains 44 minimal authored SPDP fragment scenarios. Complete same-datagram DATA_FRAG ranges can be decoded without cross-packet state. `WithProtocolDecodeAs("udp", port, "rtps-spdp-fragments")` and `WithSessionRTPSFragments(port)` select bounded cross-datagram observations; session callers also supply UDP transport and endpoint ports. The profile checks ordered/out-of-order ranges, equal overlap, conflicts, interleaved writer sequences, regular DATA replacement, QoS consistency, observed domain/direction, caller byte/count/depth limits and resource/Close release. Pending samples expire after 30 seconds without retry extension; retired identities remain quarantined until Close. Unknown refused histories fail closed. Raw and SourceBytes identify the current literal datagram; accepted per-change Serialized Payload and Fragment Event IDs describe reconstructed bytes and contributors. No topic schema, reliable-writer state, authenticated identity or DDS Security completion is claimed. Archived constructors and verification scripts reproduce whole independent answers.

Answer inventories are validated once against the immutable loaded archive snapshot and returned as independent deep copies. SHA256/archive checks, input binding, complete case coverage and nil/empty semantics remain enforced. Returned nested maps, raw JSON, frames and concurrent callers cannot modify later answers. This reduces repeated test-data decompression; it does not cache production packet state or remove tests.


`validation-rtps20-data.zip` contains 66 authored RTPS2.0 DATA/NOKEY_DATA
wire controls with independent construction answers and verification scripts.
The native UDP decoder uses the legacy fixed key suffix and optional key prefix,
status and ordered QoS layout; it preserves complete opaque CDR/PL_CDR bytes,
checks parameter-list structure and does not infer an application type or value.
INFO_SRC/INFO_DST/INFO_TS are message-local, with timestamp invalidation on a
source change. Count/depth/byte budgets, complete datagram boundaries, owned
fields and Close release apply. The original cooked sample includes versions1.3
and2.0 and invalid declared lengths; it remains external and unqualified.
RTPS1.3, reliable endpoint state, application schemas, DDS Security and general
QoS policy interpretation remain explicit requirements for subsequent work.

`validation-rtps-controls.zip` adds authored minimal RTPS control wire cases for the explicit UDP `rtps-control` profile. ACKNACK, GAP, HEARTBEAT, NACK_FRAG and HEARTBEAT_FRAG retain exact sequence words, signed counters, ordered bitmap words and selected offsets, reader/writer source context and unknown destination prefixes. Common layouts are pinned to OMG2.0/2.1/2.2/2.3/2.5 documents; document2.3 specifies wire2.4. Group extensions are decoded for wire2.4/2.5 and filtered count for wire2.5; earlier group flag bits remain uninterpreted extensions. This profile reports wire claims; it does not establish reliable delivery, authenticate endpoints or interpret DATA application types. Unsupported DATA/security/context-changing submessages refuse the entire control observation. The default SPDP/legacy DATA profile retains its prior boundaries.

The ZIP includes256 complete field/error scenarios (256 unique captures), independent deterministic generation and verification scripts, source pins and the v33 mandatory inventory. Full/deferred, workers1/2/4 and packet observation on/off require3072 replay configurations; native sessions separately cover probe purity, datagram isolation, byte/count/depth refusal, recovery, Close and owned fields. Fixed-spec wire answers and actual independent Wireshark scalar comparisons are distinct from device or reliability validation. All prior ZIPs remain immutable.

`validation-rtps-control-wire.zip` adds16 independent minimal group HEARTBEAT wire cases and four hash-bound errata for the immutable preceding control answers. The UDP PSM9.4.5.7 layout contains three GSNs after the ordinary HEARTBEAT body (52 body bytes total). The PIM logical writer-set digest attributes have no concrete wire members there: unspecified tail octets remain owned `Extension Bytes`, not fabricated digest fields. Minimum, one-byte truncation, opaque tail and refusal/recovery cases retain complete field/error/session/ownership checks across192 configurations. Input and prior-answer SHA256 must both match before applying an independent correction; historical captures/ZIPs/answers are never overwritten. Current mandatory inventory is v34.

`validation-rtps-wire.zip` adds116 unique authored minimal UDP captures with236
wire/selected-type answer profiles. `rtps-wire` observes ordered legacy2.0
DATA/NOKEY_DATA, modern2.1–2.5 DATA, common reliability controls and message-local
INFO_SRC/INFO_DST/INFO_TS context. Serialized payloads remain owned opaque bytes.
`WithProtocolRTPSShapeWriter` and `WithSessionRTPSShapeWriter` additionally select
one exact source GUID prefix/writer entity per port for the final ShapeType in
DDSI-RTPS2.5 section10.7. The selected wire2.4/2.5 PLAIN_CDR1 data/key projection
checks64 Char8 UTF8 non-NUL elements, signed32 x/y/size, independent endian and
XTypes1.3 options/trailing padding. Other writers remain opaque; older selected
alignment needs context and unsupported representations are typed refusals.
Selection supplies a schema association, not authenticated writer/topic evidence.
Mixed messages share one submessage/QoS/bitmap/application budget and fail
atomically; datagrams do not join and no ResponseTo/TransactionID is invented.
Fixed sources, complete independent answers, generator and separate CDR verifier
are in the small batch; original downloads/specification PDFs remain outside Git.
The12 full/deferred, workers1/2/4 and observer configurations also check complete
Session, fields, ordered errors, owned snapshots, probe purity and Close/recovery.
Reliable reader/writer state, arbitrary IDL/CDR2/security and original capture
qualification remain separate unfinished work. That batch sealed inventory v35;
historical ZIPs, answers and inventories stay immutable.


`validation-rtps-shape-fragments.zip` adds160 authored selected application sample
scenarios;158 new minimal captures and two hash-pinned historical captures.
`WithProtocolRTPSShapeFragments` and `WithSessionRTPSShapeFragments` select the
same exact caller-associated final ShapeType as the wire observer, with bounded
UDP DATA_FRAG reconstruction and consistent full DATA replacement. Wire2.4/2.5
PLAIN_CDR1 full/key-only type fields are checked only for that source GUID and
writer entity. Other writers retain literal geometry without type inference.

Each ordered `Messages` entry keeps its literal DATA/DATA_FRAG bytes. Pending
entries have observed fragment counts and contributor event IDs; complete
entries add separately owned `Reconstructed Payload` and exact application
fields. Full DATA keeps its literal `Serialized Payload`. Sample-level retained
QoS and fragment1 timestamp are separately named from the current message's
wire claims. Association is explicitly unverified; no ResponseTo or transaction
ID is inferred. Direction, capture section/interface/encapsulation and logical
reader/writer prefixes/entities/sequence isolate pending observations.

Conflicting overlap, key/geometry/version/vendor/QoS, completion or30second
expiry retire a sample identity for this capture lifetime. Retries do not extend
expiry. Backdated or untrackable refused datagrams close the directed observer
conservatively; Close releases all charges and incomplete contributor snapshots.
These are local observer policies, not a reliable DDS endpoint or proof that a
reader received a sample. Probe performs pure datagram-local validation without
predicting cross-datagram history. Byte/count/depth/contributor bounds, exact
sequence words, reordered ranges, DATA replacement and ownership are required.

The archive includes fixed OMG source hashes, complete independent positive and
negative answers, deterministic generators, a separate capture/range/CDR verifier
and build recipe. Complete type projections, pending observations and expected
refusals are separate evidence categories. The1920 full/deferred,workers1/2/4,
observer-off/on replay combinations compare full fields/Session/errors/contributors;
raw original captures, arbitrary schemas/CDR2/security and DDS reliability remain
unfinished requirements. The active local acceptance inventory is v36; historical
ZIPs and their answers/inventories are immutable. No rules changed in this batch.


`validation-rtps13-wire.zip` adds190 authored minimal RTPS1.3 UDP scenarios for
explicit `rtps13-wire` ingress / `WithSessionRTPS13Wire`. The16-byte header and
8-byte reported prefixes, hash-before-sequence DATA, NOKEY opaque bytes,
ACKNACK/GAP/HEARTBEAT and literal INFO_TS/SRC/DST are separate from2.x layouts.
All messages preserve order, message-local receiver context, timestamp resets,
raw sequence high/low words (including zero/UNKNOWN), signed counter views and
owned payload/parameter/bitmap bytes. No endpoint association or transaction ID
is inferred. Reported1.0–1.2 source INFO claims can be observed; their following
DATA/control layout still requires independently selected context.

The legacy parameter cursor follows declared lengths without importing2.x
length alignment or must-understand masks. PAD and repeated IDs remain ordered;
extended-ID encodings remain unselected. The byte observer advances by literal
legacy submessage lengths; it does not infer physical receiver acceptance from
an aligned or unaligned header. Selected bitmaps are bounded to256 bits by a
local ResourceExceeded capacity policy. ACK Count is selected only for an exact
four-byte tail; longer tails remain `Uninterpreted Counter Tail Bytes` instead
of guessed counters. These are wire observations, not QoS negotiation,
application type/CDR validation, DDS reliable state, authentication or delivery.

Fixed Wireshark4.2.5 source hashes, independent authored full field/error answers,
a deterministic generator, separate capture/parameter/sequence/bitmap cursor,
build recipe, optional already-installed pinned TShark comparison and current
local inventory v37 are sealed. The actual reference check matches604 wire
values from82 captures. Its legacy standalone QoS display passes the full DATA
body length into a subtree after the fixed body and can throw a bounds exception;
those original standalone cases retain independent checks, while identical QoS
with a following valid message is also tested against the actual reference.
Reference gaps do not become endpoint or application-semantic passes.

The 2280 full/deferred, workers1/2/4 and observer-off/on configurations compare
complete fields, Session, raw bytes, order and typed refusals. Native tests also
cover shared byte/projection pressure, probe purity, exact count/depth bounds,
stateless recovery, invalid/truncated carriers, caller mutations and idempotent
Close. Strict offline truncated-record rejection is preserved separately from
live carrier refusal. Original captures/specifications/reference source remain
outside Git; historical ZIPs/answers/inventories are unchanged. No rule archive
is regenerated for this native-only batch.


`validation-rtps13-parameters.zip` adds106 authored scenarios (104 distinct
minimal captures) for explicit `rtps13-parameters` / `WithSessionRTPS13Parameters`.
On the inherited RTPS1.3 UDP wire layout it interprets inline locator PIDs0x2f-0x33
as literal signed kind, uint32 port and16 address bytes. Selected UDPv4/v6
address fields are reported; unknown kinds, IPv4 prefix bytes and extensions
remain owned observations. A zero or wider-than-UDP port is retained, not promoted
to an established endpoint. PID0x55 reports ordered bitmap words and at most
four four-word content-filter records as wire bit patterns. These records are
not cryptographic signatures or a filter evaluation result. Other parameters
retain their ordered raw values, including duplicates and PAD.

The additional fields share the datagram collection, recursion and projection
budgets with wire fields. Truncated lengths/counts are malformed; the selected
four-record capacity and configured resource limits produce resource errors.
No association, participant identity, negotiated QoS or reliability is inferred.
Full fields and Session, original bytes, typed refusal, recovery, caller mutation,
Close and the worker/deferred/observer matrix are required. Independent rebuild
and a separate capture/parameter cursor check the sealed inputs and answers;
70 positive captures also matched210 selected fields in actual TShark4.2.5.
The source pin, authored generator, verifier and current v38 inventory are in
the ZIP. Raw received captures, full reference source and process reports stay
outside the repository; original device/application oracle gaps remain open.


`validation-rtps20-rti-filter.zip` seals82 minimum authored RTPS2.0 scenarios
for explicit `rtps20-rti-filter` / `WithSessionRTPS20RTIFilter`. Inline PID0x8006
is interpreted only under reported logical vendor0101. INFO_SRC updates that
vendor according to formal/2008-04-09 receiver state; the outer header cannot
select a colliding proprietary meaning after a logical-source change.

The selected fields preserve channel counts, name/expression length and raw
bytes/padding, and ordered multichannel locator records with kind/address/port
layout. This differs from ordinary locator kind/port/address. Unknown kinds,
wide ports, IPv4 prefix bytes and extensions remain literal claims. Strings
need not acquire a borrowed NUL/charset contract. No expression is evaluated,
no vendor is authenticated and no address/device/filter result is established.

Channels and locators share the wire/parameter collection allowance, depth and
logical projection budget; impossible lengths/overflowing strings, unsupported
vendor context and resource limits produce typed atomic refusal. Full fields
and Session/raw/order, Probe purity, recovery, caller mutation, carrier limits
and Close are checked. The984 worker1/2/4/full-deferred/observer configurations
use immutable input and independent answer hashes. A separate bounded cursor
and deterministic regeneration check the archive;40 positive captures matched
302 fields in actual TShark4.2.5. Its standalone NOKEY subtree-window exception
is not counted as reference success: standalone independent cases are retained
and identical QoS plus valid adjacent DATA is actually compared. Normative
logical-vendor controls use the independent model rather than a reference claim.
Source pins, generator/verifier and the current v39 requirements are sealed;
raw originals, PDFs, full reference source and process outputs stay external.

### MMS ISO TCP association observations

`validation-mms-iso.zip` seals 34 authored minimal captures and 35 complete
independent answers, reusing the hash-pinned historical nDPI capture without
copying it. `WithProtocolMMSISO(port)` explicitly selects TCP capture routing;
`WithSessionMMSISO()` selects the same standalone session path. No default
port-only or byte-search MMS admission is introduced.

The selected profile is MMS version 1 over RFC1006 TPKT, class0 COTP CR/CC/DT,
normal presentation mode with observed ACSE and MMS BER context negotiation,
unprotected AARQ/AARE, Initiate, domain-specific named Read requests with
failure AccessResults, named GetVariableAccessAttributes requests with access
ServiceErrors, Conclude, and ACSE release. Complete bounded BER trees and raw
fields remain visible beside selected typed values. No executable rule graph
is constructed in this capture path; historical bare-PDU rules remain intact.

Tests assert complete event fields, order, passive Session, both zero and maximum
InvokeID, opposite-direction request association, service/result cardinality,
peer serving capability, negotiated outstanding/receive limits, COTP segment
provenance, refusal, resource release, Close and private output ownership. TCP
full/deferred, workers1/2/4, observer off/on and Feed chunks1/7/full are covered.
After fatal refusal the shared stream stops; late data cannot fabricate a
response association. Retired InvokeID reuse remains an explicit ambiguity
refusal until a new observed TCP association.

Full MMS success Data/types, named variable lists, other services, VMD/AA names,
modifiers/extensions, authentication, indefinite BER, application state and
real device qualification are still pending; this batch does not close those
requirements. The original capture is not relicensed. Its exact reuse supplies
wire evidence, not device correctness. Independent reference comparisons record
Wireshark4.2.5's signed display of maximum Unsigned32 InvokeID separately from
its correct BER bytes and the normative unsigned answer. Source pins and the
authored generator/verifier/reference harness are inside the small ZIP. Raw
third-party downloads, reference sources and execution logs remain outside Git.

Current mandatory entry: `scripts/protocol-tests/required-tests-traffic-unified-v40.json`.

### Selected MMS service values and type declarations

`WithProtocolMMSServiceValues(port)` and `WithSessionMMSServiceValues()` select
`mms-iso-services-values`. The historical `mms-iso-ber` profile and its refusal
answers remain intact. The extended profile adds Status and Identify, named
Read results with success/failure Data, GetVariableAccessAttributes type/address
observations, VMD/AA names and returned variable specifications. It checks the
returned specification against the request, service/cardinality/peer capability,
negotiated nesting, resources, directional IDs, lifetime and owned queries.

Selected Data includes bounded arrays/structures, BOOLEAN, BIT STRING/boolean
arrays, exact arbitrary-width INTEGER/unsigned/BCD, IEEE754 binary32/binary64
with preserved bits, octets, VisibleString, UTF8 MMSString, binary/UTC time and
bounded uint64 OID subidentifiers. Time bytes/quality are reported observations;
clock validity, synchronization and application units are not authenticated.
Out-of-day binary-time contents retain a diagnostic rather than silently losing
raw values. Declared type arrays are observed without allocating the declared
number of device elements. Fixed libIEC reference layouts supplement the older
ASN.1 source for floating, MMSString and UTC type declarations.

For captures starting at an actual CP/CPA/AARQ/AARE/MMS Initiate exchange, the
profile can derive an observed application association while retaining false
COTP connection/TPDU-negotiation evidence markers. It does not scan a tail for
MMS or infer an association from arbitrary DT data. Optional selected BER
EXTERNAL direct references and opaque descriptors are recorded and checked.
Authentication/encryption, unselected encoding, named variable lists, alternate
access, modifiers, other services and retired InvokeID generation reuse remain
open. Original received captures and reference source remain outside Git.

The new minimal ZIP includes exact independent complete answers, deterministic
generation/verification and source/helper pins. Wide-number fallback bytes and
empty-bit-string/text display differences in Wireshark4.2.5 are separate from
the normative answer; a missing numeric reference field is not a codec pass.
Full/deferred, workers1/2/4, observers off/on, Feed chunks1/7/full, ownership,
resources and close/refusal boundaries remain required.

Current gate: `required-tests-traffic-unified-v83.json` with
`check_go_test_json_v2.py`. The indexed validator preserves missing/unpassed,
package invocation completion, descendant skips, failures and non-JSON checks;
its mutation/differential harness is sealed with the material. Older checkers
and inventories remain immutable historical artifacts.

The current explicit MMS service-values profile additionally observes selected GetNameList1 and Write5. GetNameList basic classes0–11 and VMD/domain/association scopes preserve identifiers, continuation and the ASN.1 `moreFollows` default true. Reported lists are neither a verified complete catalogue nor a maintained device namespace. Write named variable/Data pairs are parsed positionally because both lists can use tag0; exact ordered values and per-variable success/failure codes are correlated only with a unique opposite-direction observed InvokeID. Request variable names and query context are privately owned and budgeted; reported success does not prove a physical write. Named variable-list definitions, alternate access and wider services remain open. The `mms-discovery-write` ZIP aliases contain the86 minimal authored inputs, complete independent answers, reconstruction/reference scripts and current v42 inventory; historical ZIPs remain immutable.

The current MMS service-values profile also observes named-list Define11/GetAttributes12/Delete13 and named Read4/Write5. A definition NULL acknowledgement or a matched attribute reply establishes only observed members for that receiving endpoint and TCP association. Member order, object-name tuple/scope, request direction and InvokeID remain exact. Named requests expand only a retained observed list and charge derived collection/output costs before cloning; caller mutations cannot change definitions or in-flight snapshots. Literal error classes0–12, additional integer/description fields and selected delete-specific partial counts are observed, not local parser/physical-device errors.

Delete requests quarantine matching observations. Reported zero deletion restores unchanged prior context; positive, partial or unspecified effects invalidate matching observations again at the reply, including newer proofs while the operation was pending. Conflicting schema generations produce typed context refusals; aggregate counts never identify which individual list survived. Same-value primitive and definite constructed ObjectName identifiers compare by typed name value while raw BER/encoding markers stay owned. X.690 character-string segments use OCTET STRING04/24; nonstandard nested VisibleString1A forms are explicit negative cases, irrespective of reference-codec tolerance. Definite BER only, directly named members only; alternate access/address members, other services, indefinite BER, authentication, complete/current device namespace and physical execution remain unimplemented or unqualified.

The `mms-named-lists` aliases seal133 authored minimum inputs/answers and fixed-source/generation/verification scripts plus current v43 requirements. Full upstream code/PDFs, original downloads and process evidence remain outside Git; history is immutable.


The current explicit MMS services/values profile also observes `Address` numeric,
VisibleString and opaque-octet selectors, including definite constructed strings,
and the name/address/variableDescription/invalidated VariableSpecification choices
in direct Read/Write and named-list definition/lookup. Names keep their prior field
shape; other choices carry `Variable Choice Tag`, literal Address and optional
Type Specification. Ordered returned specifications must match the privately
accepted selector/declaration content; allowed string segmentation does not change
identity. Retained nested metadata charges shared byte/count budgets before derived
projection. These are passive wire declarations, without verified namespace/memory
layout, declaration-to-Data conformance or physical execution. Alternate/scattered
access, BCD size interpretation and original device/oracle gaps remain pending.

`validation-mms-access-selectors.zip` seals106 content-new authored captures and
complete answers in682232 bytes (SHA256
`da79337761e1585e0855c9b687663942a3397010dbf6cecd9217c49d8508e028`).
The106-case matrix covers full/deferred, workers1/2/4, observer-off/on and Feed1/7/full.
Independent regeneration checks complete selected fields/Session/order/IDs/raw/error
semantics. TShark4.2.5 selected-field comparison separately records unsigned displays
and six missing leaves in four legal empty constructed address cases; absent codec
leaves are not reported as actual decoded agreement. All139 prior ZIPs remain unchanged.
That batch uses inventory v44; historical inventories remain available.


`WithProtocolMMSDescriptionValidation(port)` / `WithSessionMMSDescriptionValidation()`
select `mms-iso-described-data`, retaining literal service observations and adding
bounded comparisons against directly observed variableDescription schemas.
Known tag/array/structure/fixed bit/octet/ASCII length/integer width/floating format/
binary-time date mode mismatches are typed MalformedMessage. Named types, BCD,
generalized/MMS string size context, negative variable-length parameters and zero
integer width remain ContextRequired in this selection; no unsupported meaning is
invented. Reported peer failures remain correlated wire reports, not successful Data.
Names/addresses without descriptions remain literal observations, not certified schemas.
Named-list generation checks precede value comparison. Private schema ownership,
late response retirement, pending Close and default capture/configuration compatibility
have executed regressions. The original literal profile and old answers stay unchanged.

`validation-mms-described-data.zip` contains67 complete selected answers and66 new
captures; one capture is reused by hash from the existing access-selector batch.
The ZIP is468993 bytes, SHA256
`0e883985b8a0db96356af3bd7cbb30513c2ba6b74cf4e65d22a129b7a9541c13`.
All140 historical ZIPs remain byte-identical. Full/deferred, workers1/2/4,
observer-off/on and Feed1/7/full remain required. Actual pinned reference comparisons
scope type declarations separately from Data where field names collide; modern
floating/UTC declaration gaps in the pinned decoder are separately qualified.
That batch uses inventory v45; earlier inventories are preserved.


Pending MMS direct Read/Write descriptions now share the retained member-history
collection limit with observed named-list context. Non-control requests cannot retain
additional copied schema trees beyond that bound. The failing request is refused
before acceptance/correlation; late replies cannot reuse the refused state.
`validation-mms-pending-history.zip` seals12 complete controls with10 content-unique
captures, independent native-budget and default-capture answers, deterministic recipes
and the current v46 inventory. Native64/default4096 and captured default4096 are
separate configurations with separate answers; larger captures use COTP fragments
within the observed1024-byte TPDU size. Adjacent fitting requests and literal
no-private-schema reads remain accepted. Historical141 ZIPs remain unchanged.


`validation-mms-alternate-access.zip` adds48 minimal literal AlternateAccess cases.
Component, index, index range, all-elements, nested and named selectors retain
complete wire fields and participate in observed Read and named-list identity.
Named Define/lookup/Read/Write preserve private selection context; changing list
members while a Read is pending refuses stale association. Selected Data with an
unresolved description is `ContextRequired` in the described profile, rather than
being validated against the entire variable. These observations do not establish
endpoint array existence, selected representation, authentication or device success.
The independent generator, exact answers, source pins and v47 inventory are sealed;
TShark4.2.5 actually agrees on64 selector scalar leaves across26 normal captures.
The full replay matrix retains eager/deferred, workers1/2/4 and observer off/on;
native sessions additionally cover Feed chunking, private ownership, shared budgets,
Close idempotence and late-input rejection. All historical ZIPs remain immutable.


`validation-iec104-command-context.zip` adds19 minimal command-echo controls,
complete independent wire/session answers, deterministic generation/verification
and fixed lib60870 source pins. Selected control/interrogation commands match the originator,
Test flag, complete object bytes and activation/deactivation phase in addition to
direction, TypeID, common address and IOA. Different command qualifiers, values
and Select/Execute modes remain unmatched and cannot retire another command.
Legacy rule fields, metadata and observed Session now keep private accepted
snapshots; changing public views cannot turn a retained typed refusal into success.
The independent matrix covers112 complete message observations,42 observed
matches and10 explicitly unmatched responses. TShark4.2.5 actually agrees on518
wire scalars across19 captures; that verifies wire decoding, separately from the
source/model association contract. Clock/test/reset response contracts retain their legacy coarse observations;
full service-specific interpretation remains open. Legacy numeric association IDs remain zero.
Device actuation/authentication and indistinguishable repeated application
generations remain unresolved. That batch was sealed with inventory v48; the current entry is documented below.


DNP3 complete-link header observation retains the existing IEEE1815 rule field
shape while isolating bounded READ/WRITE requests by captured direction, link
addresses and 4-bit application sequence. Identical retries share their slot;
conflicting outstanding sequence reuse is an explicit context refusal.
UNSOLICITED and CONFIRM do not replace or retire solicited requests. Unsupported
or no-response functions stay untracked. Complete CRC-validated application
refusals are reported per message; CRC/framing/resource failures remain fatal.
Transport/application fragments explicitly require assembly context and cannot
be interpreted as independent complete requests. Affected link context is
invalidated without erasing other address domains. Returned rule/session trees
and typed refusals are private snapshots. Pending collection and logical
memory limits apply before insertion; normal responses do not reserve an extra
request slot. Full fragment/object/measurement decoding, authentication/device
execution and indistinguishable reused-generation identity remain open.
`validation-dnp3-application-context.zip` contains minimal authored positive and
negative captures, independent complete field/session/error answers, source
pins and deterministic generation/verification tools; that batch was sealed with v49.


`WithDNP3ReadPoints(true)` / `WithSessionDNP3ReadPoints(true)` explicitly select
`dnp3-read-points` on TCP. Legacy header-only behavior remains the default.
The selected observer decodes class READ selectors (G60V1..4, qualifier6), static
binary input G1V2, binary output G10V2 and signed32 analog input G30V1.
Point qualifiers0/1 provide inclusive8/16-bit ranges; qualifiers0x17/0x28 provide
8/16-bit counts and explicit indices. Point values, complete quality flags,
reserved bits and indices remain visible without asserting calibration or
endpoint success. Duplicate static indices, truncated/descending/empty ranges
and unsupported groups/qualifiers are explicit typed refusals. Static data
cannot claim coverage from an observed READ that omitted class0. Only a decoded
response consumes its observed pending request. Collection/depth limits are
checked before allocation; refusal queries and normal trees preserve ownership.

The source-derived case retains only the two complete DNP3 PDUs from the fixed
CC-BY4.0 ICS-Security-Tools READ capture, with documentation network envelopes,
necessary handshake/close and full attribution in `SOURCE-INPUT.json`.
`validation-dnp3-read-points.zip` contains minimal cases, exact independent
field/session/error answers and deterministic extraction/generation/check tools.
Primary user-data link function is required before application state mutation;
other control frames cannot smuggle requests through CRC-valid payloads.
Other object/event/function layouts, fragment assembly, UDP, authentication,
calibration/device execution and indistinguishable generation identity remain
open. That point batch was sealed with v50; current transport entry is below.


`WithDNP3TransportFragments(true)` / `WithSessionDNP3TransportFragments(true)`
explicitly enable `dnp3-transport` on TCP; combining with the READ point option
selects `dnp3-read-points-transport`. Each CRC-validated link frame retains its
original Raw and link-rule fields. Transport fields identify buffering, duplicate,
restart and complete observations; only a complete FIN exposes `Application Data`
and application/point interpretation. Capture direction, link addresses and DIR
bit isolate retained transport streams. Sequence numbers advance modulo64; an
unexpected segment is refused without appending bytes, allowing the expected
segment to arrive later. An exact duplicate last continuation is observed without
re-appending; a conflicting duplicate discards that assembly and its link pair's
request context. New FIR replaces incomplete bytes. Application fragmentation
still requires separate context. Logical memory, assembled length, segment count
and concurrent assembly count apply before storage. Close reports incomplete
assemblies deterministically and releases the session budget. Legacy observation
without the option keeps its fragment refusal behavior.

`validation-dnp3-transport.zip` seals20 minimal positive/negative captures, exact
independent wire/session/error answers, resource vectors and generation/check
tools. The source-derived PDUs retain fixed CC-BY4.0 attribution. Actual unchanged
opendnp33.1.2 TransportRx code was compiled outside the repository with bounded
byte/message/logger adapters:12 cases/26 complete APDUs matched exactly, including
sequence wrap and selected retry/interleave paths. This is receive-algorithm
execution, not a full SDK or device endpoint. TShark4.2.5 independently checked
1,456 link/transport scalars and596 application/static-point scalars; its missing
complete response on the isolated wrap case is retained as a reference limitation.
Conflicting-duplicate refusal is an explicit observer policy, separate from SDK
equivalence. Wider object/events, application fragments, UDP, authentication,
calibration/device behavior and indistinguishable application generations remain
open. That transport batch was sealed with v51; current static analog entry is below.


The selected `dnp3-read-points` observer now includes all static analog G30V1..6
wire layouts: signed32/16 with flags (V1/V2), signed32/16 without flags (V3/V4),
and IEEE binary32/64 with flags (V5/V6). Existing V1 fields remain compatible.
Without a quality octet, `Quality` is `not-reported` and no online/error flags are
fabricated from value bytes. Floating observations retain exact `Raw Bits`,
`Encoding` and `Value Status` (`finite`, `nan`, `positive-infinity`,
`negative-infinity`); only finite values expose numeric `Value`. NaN payloads,
signed zero and subnormal bits survive without emitting non-JSON numbers.
These are encoded wire values, not engineering units/calibration or device
quality inference. Qualifier ranges/prefixes, context, budgets and ownership use
the existing selected observer contracts.

`validation-dnp3-static-analogs.zip` contains36 minimal authored positive/negative
captures, exact complete field/session/error answers, fixed Group30 source pins
and generation/check tools. Integer limits, zero, all IEEE classes, complete
quality flags and missing quality are checked in23 actual TShark4.2.5 captures
with1,568 wire/index/quality/integer/IEEE comparisons. Native tests additionally
compare finite IEEE bits (including the sign of zero), refuse truncated and
descending layouts, preserve pending context, and check collection boundaries
and query ownership. All preceding ZIPs remain unchanged. Other objects/events,
application fragments, individual-object READ selectors, UDP/authentication,
device/calibration and indistinguishable generations remain open. Current
that analog batch was sealed with v52; current entry is below.


`WithDNP3DirectStaticReads(true)` / `WithSessionDNP3DirectStaticReads(true)` opt
into `dnp3-direct-static-read` (or `-transport` when transport assembly is enabled).
This profile includes G30V0 all-object qualifier6 and inclusive8/16-bit ranges
(qualifiers0/1), plus the existing class READ headers. Ordered `Read Selectors`
retain the literal observed headers. Private normalized range unions check every
returned static point's group/index before its request is retired. Out-of-range,
gap and wrong-group responses have ContextRequired and leave the request for a
later covered response. Class0 permits the supported static families; class1..3
alone do not authorize static data. `Read Context` and `Read Coverage` separately
identify observed direct/class0/unobserved/no-static-data context; these are
observations, not endpoint execution or completeness certification. Existing
class-only options keep their established behavior.

Range state owns its storage independently of public header maps; retained
capacity is included in the logical capture memory budget. Header count applies
before retention. Normalized sorted unions avoid quadratic append copies and
point-by-selector scans. Close/late-input semantics remain bounded. The shared
traffic typed-error assertion now explicitly refuses a nil typed error rather
than dereferencing it; subprocess injection proves old panic/new clear rejection.

`validation-dnp3-direct-static-read.zip` seals31 positive/negative minimal
captures, exact independent fields/Session/coverage/errors, resource vectors and
source-pinned generation/check tools. Full/deferred, workers1/2/4, observer off/on
and native1/7/full chunking include the transport composition capture. TShark4.2.5
actually checked571 selector/response wire scalars across16 captures; the
unchanged fixed transport receiver separately checked both composed APDUs. Fixed
opendnp3 READ handler/database source supplies selector semantics. Requested
specific variations, other direct object groups, index-prefix READs, packed
binary observations, application fragments, UDP/authentication/device/completeness
and generation identity remain open. No historical batch is rewritten. Current
This preceding batch used v53; the current effective entry is v64.


### Selected typed static DNP3 READ and packed binary points

`WithDNP3TypedStaticReads` / `WithSessionDNP3TypedStaticReads` explicitly select
TCP G1V0/1/2, G10V0/2 and G30V0..6 all/range READ observations with qualifier
0/1/6. Selected responses retain the existing G1V2/G10V2/G30V1..6 layouts and add
G1V1 contiguous packed bits, including cross-byte and 16-bit index boundaries.
Packed data exposes boolean Value and `Quality:not-reported`; absent quality
octets never produce fabricated Flags, Online or other quality fields. Unused
last-octet padding bits do not become extra points or quality flags.

Under this selected reference database model, later overlapping headers replace
the selected variation per index. Class0 and variation0 preserve unknown
configured defaults; they do not infer an outstation configuration. Selected
G1V1 may be promoted to flagged G1V2 only when observed quality is not
online-only. Wrong group, range or selected variation returns ContextRequired
before retiring the request; a later covered reply remains usable. The bounded
interval sweep avoids expanding all possible indices, and retained capacities
are charged before admission. Public header/field mutation cannot change it.

`validation-dnp3-typed-static-read.zip` seals57 content-unique minimal cases and
complete independent field/Session/order/error answers plus required scripts.
They include both overlap orders, wildcard/class0 ordering, quality promotion
and refusal, duplicate/truncated/prefix diagnostics, ID reuse, wrong direction,
sequence and link peer, resource/Close/ownership and actual transport composition.
All full/deferred × workers1/2/4 × observeroff/on configurations and native1/7/full
chunks are required. Integer types, IEEE raw bits, signed zero, nonfinite state
and absent packed quality have additional assertions.

Actual TShark4.2.5 wire/quality/bit/IEEE checks, verbatim fixed opendnp3 selection
and promotion method bodies with declared bounded database/type adapters, and
unchanged transport receive code provide distinct reference evidence. Primary
source bytes stay outside Git; source pins and deterministic adapters/scripts
are sealed. This is not full SDK, serializer, endpoint, authenticated identity,
real database/configuration, range-completeness or physical measurement proof.
Other static/event groups, G10V1, index-prefix READ, application fragments, UDP
and security remain active unimplemented requirements. No historical batch is
rewritten. That batch used v54 (23,767 checks); the current full mandatory entry is v64 (32,838 checks); older
inventories above describe their historical delivery snapshots.


### Selected static and frozen DNP3 counter points

`WithDNP3CounterPoints` / `WithSessionDNP3CounterPoints` explicitly select TCP
G20V1/2/5/6 and G21V1/2/5/6/9/10 observations, retaining selected typed static
points. Direct READ selectors support V0 or those concrete variations with
all-object and inclusive 8/16-bit ranges (qualifiers 6/0/1). Ordered observed
selector coverage separates counter and frozen-counter groups; mismatched
responses cannot retire a pending request. Class 0 expands to the five supported
static groups only in this opt-in profile. Old modes retain their boundaries.

Values are unsigned `uint32`, including promoted 16-bit values. Flagged layouts
report rollover/discontinuity separately from analog quality; no-flags layouts
report `Quality: not-reported` and omit flags. G21V5/6 expose the 48-bit
`Timestamp [ms]` since 1970 with `Timestamp Quality: not-reported`; the wire
layout does not establish synchronization. Other variations omit timestamps.
Private selector storage is budgeted; Close releases it. Transport fragment
composition remains a separate explicit TCP option.

`validation-dnp3-counter-points.zip` seals 49 content-unique synthetic minimal
inputs with complete independently authored bytes, fields, Session, ordered
association and refusal answers, plus deterministic generation and validation
scripts. Tests cover unsigned/time boundaries, absent quality, wrong group,
variation/range, direction/sequence/link peers, retries, malformed/truncated
objects, private input/output ownership, resource refusal and adjacent recovery.
Native whole/1/7-byte and all full/deferred × workers 1/2/4 × observation callback
combinations run through the normal APIs. Fixed-version TShark fields, extracted
reference selection methods, exact counter Read/Write methods with pinned
ser4cpp headers, and unchanged TransportRx code are independently executed.
Declared reference adapters are not a full SDK or device endpoint.

Counter events, freezing operations, other objects, application fragments, UDP,
authentication, inferred device defaults, clock synchronization, complete/current
device state, physical metering and indistinguishable generation identity remain
unimplemented or unqualified. Previous ZIPs remain immutable. That batch used v55 (24,668 checks); the current full
mandatory entry is v56 (25,749 checks); earlier inventories remain available.


### Selected indexed DNP3 counter events

`WithDNP3CounterEvents` / `WithSessionDNP3CounterEvents` explicitly select TCP
counter G22 and frozen-counter G23V1/2/5/6, retaining selected static points.
Responses require indexed qualifiers23/40 and preserve repeated indices and
wire order. Values are unsigned `uint32`; flagged quality retains separate
rollover/discontinuity bits. V5/6 expose48-bit milliseconds with clock quality
not-reported. Every event point has `Event Class: not-reported`; an observed
class READ is not proof of a particular device event class.

Direct event READ supports V0/concrete all-object qualifier6 and class1/2/3
selectors. Event all-object selection keeps the first observed type selection:
later event headers do not override already selected events. Static range
selection retains its existing later-header precedence. Wrong type/variation
cannot retire a pending request. Unsolicited responses and confirmations stay
outside solicited request retirement; confirmation is only observed, not sent.
Unsupported count/range/index event READs remain explicit refusals. Separate
TCP transport-fragment assembly can be composed with this profile.

The59 synthetic minimal controls in `validation-dnp3-counter-events.zip` carry
complete independent bytes/fields/Session/order/association/refusal answers,
source/model pins and necessary reconstruction/reference scripts. They include
repeated indices, variant precedence, timestamp/unsigned boundaries, class0
versus event context, direction/sequence/link peers, retry/conflicting requests,
malformed/truncated headers/points, unsolicited/confirmation isolation,
selector ownership, count/memory/depth refusal, release and recovery. All
whole/1/7-byte native feeds and full/deferred × workers1/2/4 × observeroff/on
capture combinations are required. Actual TShark fields, fixed Group22/23
Read/Write code, exact EventSelection methods and unchanged TransportRx execute
independently with explicitly declared adapters. These are not full SDK or
endpoint/device proofs. The reference class-selection source has a retained
TODO about resetting previously used variations; fresh declared reference
records do not establish long-lived SDK reset equivalence.

Other event layouts/services, event count/range/index READs, application
fragments, UDP/security, actual device classes/defaults/current configuration,
completeness, metering and indistinguishable generations remain open. Historical
ZIPs are immutable. That event batch used v56; the active full mandatory inventory is v57 (25,896 checks).


### DNP3 observed link-role domain isolation

Pending request/reply keys include the captured DIR role as well as capture
direction, link addresses and application sequence. A foreign DIR response is
retained as a wire observation without claiming a matched request; a subsequent
normal response retains its valid association. Fragment loss invalidates only
its own directional/address/role domain, including the reverse reply domain;
foreign-role or wrong-direction loss cannot erase another pending request.
Private request budget accounting and all existing public fields remain intact.

The8 minimal `validation-dnp3-link-role.zip` controls preserve complete independent
byte/field/Session/order/association/error answers, raw role fields, ownership and
Close. Native whole/1/7-byte and full/deferred × workers1/2/4 × observeroff/on
capture matrices are required. Old runtime APIs reproduce5 domain-isolation
failures with3 adjacent normal/own-loss positives; patched code passes the same
checks. Fixed LinkContext::OnFrame code is executed with a declared fresh
receiver and no-op dispatch adapters; actual TShark fields remain separate.
These observed-only boundaries do not certify full SDK delivery, authentication,
physical devices, role identity or operational success. Historical ZIPs are
immutable. That role batch used v57; the current full inventory is v64,32,838 required checks.


### DNP3 direct count-limited event READ observations

`WithDNP3EventCountReads` / `WithSessionDNP3EventCountReads` add bounded TCP
G22/G23V0/1/2/5/6 READ count qualifiers7/8 to the selected counter-event
profile. Count limits apply to ordered event records, including repeated point
indices. Successive headers select subsequent records; they cannot change an
already selected event's variation. Invalid count/order/variation responses do
not mutate or consume retained request quotas. Counts across object headers and
event types are checked; fewer or empty observations do not imply device
completeness. The previous opt-in profiles keep their original grammar.

The52 minimal synthetic controls in `validation-dnp3-event-count-reads.zip`
carry exact independent bytes/fields/Session/order/association/error answers,
ownership/Close and resource boundaries. All native whole/1/7-byte and capture
full/deferred x workers1/2/4 x observeroff/on configurations are required.
Fixed-source count parsing, event selection and codecs actually execute with
specified adapters; TShark independently checks count and point fields.
Finite count selectors mixed with event-class selectors remain refused because
wire points do not report their class. Class-count READs, application fragments,
wider event groups, device defaults/class/configuration/completeness, security
and complete endpoint execution remain pending requirements. Historical ZIPs
and original answers remain immutable. That count batch used v58,26,847 checks. Current effective full entry: v64,
32,838 mandatory checks; reference and API-contract evidence are distinct.


### DNP3 static status observations and selected-snapshot isolation

`WithDNP3StaticStatusPoints` / its Session counterpart add TCP G3V1/2,
G10V1/2 and G40V1..4 with V0/concrete all/range READs to typed static
observation. Packed one/two-bit fields retain every bit and index without
inventing quality. Double-bit values keep all four encoded states. Output
status signed integers and IEEE binary32/64 preserve raw bits, signed zero,
NaN payloads and subnormals; nonfinite values remain explicit and JSON safe.
Known quality flags remain literal wire observations. G40 bits5/6 have conflicting
fixed reference descriptions and are exposed as `Uninterpreted Quality Bits`,
not claimed as independently verified OverRange/ReferenceError or reserved.

Explicit direct/typed selected snapshots require a single record per group/index,
including across variations. A refused duplicate response does not consume its
READ, so an adjacent valid response still associates. Ordered event records may
repeat indices; legacy class-only literal observations remain compatible.
Raw fields, Session, direction, IDs including0, ordering, errors, budget bounds,
private queries and Close are asserted. Wider objects/class-count/application
fragments, configuration/units/calibration/security and endpoint proof remain open.

The52 minimal controls in `validation-dnp3-static-status-points.zip` have exact
independent whole-message answers,52x12 capture configurations and52x3 native
chunks. Fixed packed/GetValue/selection/codec source methods actually execute
with declared adapters; TShark checks wire fields independently. The pinned
ser4cpp float converters lose signed zero, NaN payload and subnormals. Those
actual differences are retained as reference limitations, not bit-oracle passes;
wire/Python/TShark expectations and IUT assertions are not weakened to mimic them.
All previous ZIPs and answers are immutable. That status batch used v59,27,799 full required entries; current effective gate:
v64,32,838 full required entries. Source/API and independent semantic proof remain
separate from replay agreement and unresolved quality/device semantics.


### Binary events and message-local common time

`WithDNP3BinaryEvents` / Session counterpart select TCP G2/G4V1..3 and
G11V1/2 indexed observations. Direct V0/concrete all/count READs retain ordered
first-unselected event quotas per type, repeated event indices, and refusal
without consuming request context. Prior counter-event and typed-static profiles
remain available; explicit static-status and transport options compose.

Relative16 time requires a prior G51V1/2 CTO in the same complete application
message. Replacement uses the latest CTO; next messages begin without one.
The unsigned time48+relative16 sum retains carry. Synchronized/unsynchronized
is a literal reported quality, not physical clock certification. Absolute event
times and point classes remain unreported quality/class. CTO has one value;
multi-value CTO is explicitly unsupported. Class/direct finite count mixing,
application fragments, device configuration/completeness and security remain open.

The 62 minimal controls have whole byte/field/Session/association/error/ownership
answers and 62x12 capture / 62x3 native configurations, plus collection/depth/selector
admission boundaries. Independent fixed SDK codec/scalar/selection/CTO/transport
methods actually execute with declared adapters. TShark4.2.5 independently checks
literal flags, count/index/time fields; its unsynchronized CTO display and flagged
double-bit scalar gaps are kept distinct from executed SDK evidence.
All historical archives and answers stay immutable. Current effective inventory:
v60, 28,934 full required entries. Original raw inputs lacking semantic oracles
and inherited quality/device gaps remain pending, regardless of replay agreement.


### Selected analog events

`WithDNP3AnalogEvents` / Session counterpart select TCP G32/G42V1..8
indexed observations and explicit V0/concrete all/count READs. Signed16/32,
IEEE binary32/64 raw bits and time48 preserve wire widths. Ordered repeated-index
event records and independent per-type first-unselected quotas retain refused
request context. Binary/counter events and typed static observations remain available;
explicit transport composition keeps original link and assembled application bytes.
G32 quality flags remain literal observations; disputed G42 bits5/6 remain
`Uninterpreted Quality Bits`. Absolute timestamp quality and point class stay
unreported, without clock/device/default/configuration/units/security inference.

The 138 authored minimal inputs have complete byte/field/Session/direction/order/
ID/association/error/budget/ownership answers, 138x12 capture and 138x3 native
configurations, plus count/depth/admission/Close boundaries. Pinned codec,selection
and TransportRx methods actually execute through declared adapters; TShark checks
wire values independently. 59 actual reference IEEE conversion losses preserve
input-bound proof of non-equivalence. IUT/Python/TShark raw-bit/negative-zero/NaN/
subnormal assertions remain strict. G42 quality meaning and all inherited original
requirements remain pending where independent semantics are missing.

All previous ZIPs and original answers remain immutable. Current effective
inventory: v61, 31,437 full required entries. Wider objects, class-count mixing,
application fragments and device/endpoint completeness are not certified.


### BACnet NPDU boundaries and captured query authority

The existing BACnet/IP0x81 original unicast/broadcast fixed-header observation
accepts all four legal network priorities and the literal expecting-reply flag.
NPDU network/destination/source flags change its layout and are not interpreted
as the fixed-offset APDU. Reserved bits4/6 are refused. Routed/source-header
bytes cannot fabricate ReadProperty/WriteProperty from address octets.
Routing, full APDU services/values/transaction association/device/security remain
pending; these boundaries do not close those original requirements.

Accepted observations privately retain exact fields,metadata presence(including
nil) and Session after public Fields/Structured/Session/Raw edits. Captured
unrecognized UDP diagnostics retain their original query refusal after injected
public caches/status/protocol changes. Manually constructed event caches remain
compatible. Errors,raw bytes,IDs,packet evidence and unknown accounting remain.

The 22 minimal captures and independent answers cover22 × 12 configurations with
workers1/2/4,full/deferred, observers off/on, and queries after capture closes. UDP
split payloads are separate diagnostics,notjoined into a BACnet message.
The fixed C NPDU methods and TShark actually verify priority and header offsets;
source decoder tolerance of reserved bits is explicitly not a valid-wire pass.
No full SDK/IUT service/physical endpoint proof is inferred.
All old ZIPs/answers stay immutable. This preceding batch used v62 inventory:31,808
full required entries; process/reference source/dependencies/logs remain external.


### Selected BACnet ReadProperty observations

`WithProtocolDecodeAs("udp", port, "bacnet-read-property")` or Go
`WithSessionBACnetReadProperty()` selects original-unicast BACnet/IP,
NPDUv1 fixed headers and unsegmented ReadProperty request/ComplexACK,
Error/Reject/server Abort. Default opaque BACnet observations remain compatible.
Exact object/property/optional index, primitive application tags0-12, unsigned64,
signed/enumerated32 and literal encoded values are retained. IEEE bit strings
preserve negative zero, subnormal and nonfinite payloads; nonfinite numeric values
are JSON-safe. Character encoding and raw calendar values do not imply text,
calendar, object-schema or device-operation validity.

Only reversed endpoint/domain/invoke and matching reply descriptors associate.
Interleaving is supported; retries, used IDs, late/backdated/orphan replies and
conflicts stay context-required. Capture idle does not invent a UDP generation.
Budget refusal, truncated carriers and malformed message boundaries retire
association context; state history stays charged until idempotent Close.
Fields, Session, typed errors and encoded data have private query ownership.
Routed/network NPDUs, segmentation, constructed property schemas, client Abort,
other services, authenticated device semantics and original pending work remain
unimplemented or unverified; this finite profile does not close those requirements.

The41 minimal capture cases have independent complete answers and41x12 replay
coverage plus41x2 native datagram configurations. Fixed1.4.1 source service
encoders and TShark run independently; primitive adapters, sentinel/nonminimal
index differences and TShark float display rounding are explicitly qualified.
The preceding ReadProperty batch used v63 inventory:32,514 full required entries. All historical
ZIPs/answers are immutable; raw downloads, upstream sources and run evidence
remain outside Git.


### Routed ReadProperty and originating invoke namespaces

Explicit `bacnet-routed-read-property` UDP selection additionally observes
NPDUv1 source/destination network and one-to-six-byte logical MAC addresses,
actual APDU offset and present-only hop count. Reversed physical endpoints,
logical routing addresses, capture domain, invoke and descriptor must match.
No authenticated logical device, applied routing, forwarding BVLC, broadcast
peer, longer-address carrier, segmentation or constructed property claim is made.

Source-origin invoke history is shared across destination/service contexts.
A complete valid unconfirmed APDU has no invoke and cannot poison confirmed
transactions. Unsupported confirmed service headers retire their observed
identity; unrelated IDs remain eligible. Same origin ID across peers is
ambiguous; distinct logical originating devices behind one relay stay separate.
Refused unknown routes quarantine the bounded physical relay pair. Histories
remain charged until idempotent Close; public edits cannot rewrite accepted
field/Session/error answers. Probe remains wire-only and does not mutate state.

Nineteen minimal cases have complete independent answers,19x12 capture and19x2
native configurations. Fixed C NPDU and TSM allocator methods execute with
explicit bounded adapters; TShark independently validates actual wire fields.
Reference evidence is not device/router/client conformance or authentication.
All159 preceding ZIPs remain unchanged. The preceding routed ReadProperty batch used v64:32,838 full
mandatory entries. Original pending protocol and independent-real-input work
remain pending, with evidence and process reports outside Git.


### Classical J1939 transport observations

`WithCANDecodeAs(interface, "j1939-tp")` adds classical TP BAM and unicast
RTS/CTS/EOM ACK/Abort to the existing Request/Address Claim observations.
Directed source/destination and capture section/interface/encapsulation isolate
state. Announced size/count, control PGN, CTS direction/window and every data
sequence must agree before completion. Identical retransmissions preserve bytes;
conflicting repeats and overlapping starts retire the bounded pair. Foreign
PGN control cannot consume the real pending transfer. Unicast data completion
waits for matching EOM ACK. Abort, phase expiry, refused captures, budgets and
idempotent Close retain diagnostics and release allocated payload state.

The profile reports unverified captured bytes and literal final padding. TP has
no transaction ID; it cannot establish authenticated ECU identity, prove physical
delivery or distinguish an unobservable stale frame that exactly impersonates a
new transfer. No ETP, CAN FD application, application-PGN payload semantics,
automotive ISO-TP, UDS/OBD or device conformance claim is made. Those original
requirements remain open. The existing `j1939` selection remains stateless.

Forty-eight minimal synthetic cases carry independent literal field/Session,
order, contributor, complete payload and refusal answers in the new small ZIP.
The 48x12 full/deferred, worker and observer capture matrix is mandatory; record
chunks are indivisible, so stream splitting is inapplicable. Source/domain,
resource, ownership and phase-boundary regressions add direct ingress coverage.
TShark 4.2.5 executes wire-field comparisons. Two exact layout helpers from pinned
Linux v6.6 source execute through a declared userspace adapter; the full kernel
receiver/socket/timers and real ECU endpoints are not executed. Source pins and
reference scripts remain archived, with upstream source and build outputs outside
Git. All160 preceding ZIPs remain byte-for-byte unchanged.

The preceding J1939 batch used v65 inventory: 33,482 full mandatory entries.


### Explicit physical CAN ISO-TP and selected UDS

`WithCANISOTransport(interface, CANISOTransport{...})` identifies request/response
CAN IDs, including explicit EFF flags, classical or FD carrier, normal/extended/
mixed address context, padding and bounded WAIT/capture timeout policies.
SF/FF/CF/FC observations preserve literal bytes,12/32-bit lengths, modulo16
sequence, initial static block/STmin parameters, contributor order and full payload.
Changed FC tuples require a separate dynamic profile. Strict phase/gap/sequence,
overlap/truncation/budget refusal and idempotent Close prevent partial resurrection.
Capture timestamps are required for the stateful timing contract.

Optional UDS observes DiagnosticSessionControl and single-DID ReadDataByIdentifier,
exact SID/descriptor/timing/NRC and raw data-record bytes. Only exact active retries
remain eligible; changed suppression, reused request identities and unknown DID-list
context cannot manufacture association. NRC has no descriptor and is refused after
multiple captured service generations. Suppressed positives do not fabricate device
success; unsuppressed pending requests emit a Close diagnostic. Histories and owned
query snapshots remain bounded and independent of public edits.

These are unverified byte/context observations. No authenticated ECU, physical
service execution, validated VIN/OEM data, general multiple-DID schema, functional
broadcast/multiple responders, dynamic FC, security or complete UDS claim is made.
Original pending requirements remain.51 minimal captures and full independent
field/Session/payload/refusal answers run51x12 capture configurations. Whole CAN
records are indivisible; stream splits do not apply. Actual fixed TShark wire,
reassembled payload and selected UDS fields are separate from source-derived
association answers. The exact pinned Linux canonical-length helper executes in a
userspace adapter; full kernel receiver/socket/timers and ECU endpoints do not.
All161 preceding ZIPs remain byte-for-byte unchanged. Raw captures, dependencies,
reference source/build output and process evidence stay outside Git.

Current effective v83 inventory: 44,460 entries; 44,453 are mandatory for the full tier.

The opt-in `DynamicFlowControl` profile follows Linux v6.12 FC tuple updates (including WAIT without granting data). Disjoint address extensions or classical/FD carriers may share CAN IDs; unknown selectors cannot choose or mutate a conversation. Ambiguous selectors are rejected. Seven sealed cases exercise full/deferred, workers 1/2/4, and observer off/on, with exact fields, byte collections, contributor references, errors and resource ownership. The existing static FC profile remains unchanged. Pinned upstream FC and filter functions execute with declared userspace stubs; this is not a kernel/ECU endpoint. TShark 4.2.5 checks wire tuples and two complete payloads, but cannot independently reassemble the shared-address transcripts.

Selected CAN UDS control observations add TesterPresent zero subfunction and ECUReset types 1..5 on explicitly configured physical pairs. Suppress-positive-response requests may still receive matching NRC; exact active retries retain the original event, while stale/reused identities and mismatching service/subfunction lose or cannot consume eligibility. Rapid shutdown requires its raw time byte; 255 is retained without known seconds interpretation. Twenty-three minimal inputs check full fields/session/refs/errors/ownership in all12 required configurations. TShark4.2.5 executes wire-field checks; exact fixed udsoncan service functions execute using declared import replacements, not a complete codec or ECU endpoint. No reset execution, liveness, security or full UDS-schema claim is made.

Selected CAN UDS parameter control observations cover RoutineControl types 1..3 with exact routineID and raw option/status records; CommunicationControl types 0..5 with canonical message/subnet bytes and explicit enhanced nodeID shapes; DTC setting types 1..2 with raw option records. Complete request digests reject changed parameters disguised as active retries. NRC lacks echoed routineID and cannot choose among reused service generations. Forty minimal transcripts verify exact fields/session/order/refs/errors and nested owned record bytes in480 required configurations, plus domain/history/Close resource regressions. Pinned udsoncanv1.26.1 functions execute through declared import replacements and TShark4.2.5 verifies selected wire fields. No routine execution, communication/DTC state, vendor schema, full installed codec or device endpoint claim is made.

DoIP TCP diagnostic-generation observations distinguish a new DSC descriptor or logical target from an indistinguishable reused identity. Repeated request bytes remain observable, while replies cannot claim a generation absent a unique descriptor. Service-only NRC and short ACK/NACK after multiple target generations are typed context refusals; complete uniquely echoed responses remain eligible. Only three private complete-frame association refusals permit continued parsing. Length, resource and other errors still invalidate the session. Request event IDs are assigned before retaining routing/alive/diagnostic slots, and successful responses refer to the actual request event. Private field/error ownership also covers Close NeedMore. Thirty-six minimal captures exercise1/7/full TCP chunks across432 full/deferred, workers1/2/4, observeroff/on configurations; actual TShark4.2.5 verifies219 complete wire messages and1860 fields. This does not certify ECU execution, authentication, all UDS services or all original raw-capture oracles.

TCP capture-prefix recovery accepts only reader-proven loss after complete IP/TCP headers, while retaining capture-truncation errors. Only actual captured bytes enter the stream. FIN is anchored after the declared wire payload, so retransmission may fill its missing suffix and a remaining gap stays incomplete. Header, options, wire bounds, sequence-window and resource errors remain rejected. Twenty-four own minimal IPv4/IPv6 captures include sequence wrap, FIN recovery/gaps/duplicate FIN and malformed lengths/headers across288 full/deferred, workers1/2/4, observeroff/on configurations. The ZIP contains independent exact stream, MQTT fields/Session, error and resource expectations and deterministic generators; actual TShark4.2.5 checks72 wire frames and280 fields. This does not certify live endpoint conformance, all tunnel truncation behavior or device sample oracles.

DoIP context continuation validates complete selected DSC/ACK/NACK wire shapes before any recoverable association refusal. Invalid codes/lengths/services remain fatal. A distinct valid request following an unassignable response is observable with explicit previous-completion uncertainty; its old eligibility is retired and bounded history remains. Late retired target/descriptor replies cannot consume a fresh request. Forty-eight minimal captures span TCP chunks1/7/full and576 full/deferred, workers1/2/4, observeroff/on configurations with exact messages, fields/Session, errors, event refs, ownership and resources. Fixed python-doipclientv1.1.7 codec classes execute with stdlib dependencies, without import replacements; they verify payload/address/code/echo bytes and NACK enums. Passive association and modern DSC length rules have separate explicit answers; no ECU execution or installed endpoint claim.

DoIP ACK previous-message echoes with suppressed DSC high bits are rejected before retired-descriptor history lookup. Nine independent minimal captures span 0x80/0x83/0xff and TCP chunks1/7/full with108 execution configurations; exact fields, errors, event links, ownership and resources are checked. Existing archives remain immutable.

Complete known late DoIP diagnostic replies after completion receive an explicit missing-active-context refusal without consuming future requests. Wire validity, observed target/descriptor history, direction and address boundaries remain strict; retained identity budgets cannot be bypassed. Forty-two minimal captures cover fourteen independent positive/context/refusal sequences and TCP chunks1/7/full with504 execution configurations.

Wrapper GBT headers and bounded first directional transfer reuse complete selected unciphered Get decoders. Exact observed ACK links, bytes/fields/Session/order and ownership are verified through24 minimal TCP/UDP captures, full/deferred workers1/2/4 observeroff/on and native splitting. Conflict, gap, reused/unknown context, resource refusal and Close stay explicit; negotiated window/security/device semantics are not claimed. Fixed Gurux6e409ae9 public encoder and receiver produce the independent ACK/value256; its default receiverReady attribute failure remains documented externally.

Wrapper v1 ACSE selected no-authentication LN context 2.16.756.5.8.1.1 seals32 minimal TCP/UDP captures. AARQ/AARE, result/diagnostic, optional protocol version and optional user-information, initiate v6 conformance bit order/PDU/VAA are observed with exact bytes/fields/Session/IDs. Only first unambiguous reversed carrier/wPort response is linked; missing initiate, reused/idle/refused generations and unproposed selection never certify correlation. Fixed unmodified Gurux6e409ae9 parses44 field-bearing messages in local buffers; authentication, ciphering, HDLC ACSE, object rights and actual device success remain pending. Full/deferred workers1/2/4 observeroff/on plus native splitting and owned results are required.

The HDLC ACSE supplement seals44 minimal TCP/UDP captures. Selected unciphered LN AARQ/AARE/initiatev6 preserve complete logical addresses, HCS/FCS, LLC, fields, Session, modulo8 NS/NR and exact active retries. Used address/sequence tuples and observed orphan acknowledgements retain charged refusal history; later ambiguous/expired/refused responses cannot own a fresh request. Unmatched ACSE Close emits NeedMore and releases resources.528 capture configurations plus220 native split/deferred/ownership combinations are required. Fixed unmodified Gurux6e409ae9 checks32 unique frame encodings and116 ACSE messages; ITUX.2271995 supplies optional envelope-field evidence. Authentication, ciphering, ACSE information segmentation, SN and object/device semantics remain pending; no physical meter success is claimed.

The Wrapper/HDLC lifecycle supplement seals72 minimal inputs for selected RLRQ/RLRE/ABRT, optional reason without invented defaults, selected initiate information and literal unverified abort information. Scope-specific retirement preserves unrelated logical endpoints, blocks late Get/block/fragment ownership and retains bounded history. Wrapper repeats remain ambiguous; exact active HDLC retries retain the original identity and fresh observed AA may restore its endpoint.864 capture configurations plus360 native ownership/chunk configurations are required, alongside budget/idle/typed Close regressions. Fixed Gurux6e409ae9 actually produces both release requests and20 unique HDLC encodings; selected initiate codec runs separately. Its malformed server RLRE and non-validating release-response pass branch are explicitly not successful semantic oracles. Positive reason/BER fields use X2271995 independent normative transcripts, not IUT output; no authenticated/device-success claim.

Selected AARQ/AARE HDLC information assembly seals36 minimal TCP/UDP captures across432 full/deferred workers1/2/4 observeroff/on configurations and180 native split/ownership combinations. Every carrier CRC/address/control remains literal; assembled LLC/APDU is explicitly identified in metadata. Complete request identity belongs to the final request event and response ACK confirms the last request frame. Exact active segment retries, wrong address/direction/ACK, orphan/gap/conflict/CRC/BER refusal, typed incomplete Close, budgets/idle and owned projections are checked. Pools are capped at8 distinct sequence frames and4099 information bytes, further capped by caller limits. Fixed unmodified Gurux6e409ae9 executes22 unique frame encodings,46 complete ACSE codec parses and both complete public segment streams; passive association and negative expected boundaries are separate independent transcripts. Segmented release/abort, authentication/cipher/SN and original device/object/oracle obligations remain pending.

Selected Wrapper/type3HDLC LOW mechanism2.16.756.5.8.2.1 observations seal80 unique minimal TCP/UDP captures,960 capture configurations and400 native chunk/deferred/ownership combinations. Exact requirements padding/component order, calling GraphicString bytes1..64, unchanged no-auth defaults and observed proposals never verify credentials or devices. Charged canonical orphan AARE history prevents later false request ownership, including first reverse UDP arrival. Complete fields, Session, zero/nonzero links, ordered errors, budget/idle refusal, typed unmatched LOW Close and ownership are mandatory. Unmodified fixed Gurux6e409ae9 executes112 normal complete parses,4 expected typed rejection exceptions,84 LOW value checks,5 unique HDLC encodings and three public client value-length anchors. Normative malformed/unselected/refusal boundaries remain separate independent answers. Values are public synthetic fixtures; HLS/cipher/SN, device/object and original semantic obligations remain pending.

Selected SOME/IP1 explicit UDP TP observations seal39 unique minimal synthetic captures and full literal/assembly answers. Exact direction/domain/message/request identity, contiguous offsets, reserved-bit receiver tolerance, complete contributor IDs, typed conflicts/gaps/restarts/reuse/refusal, bounded bytes/segments/idle and owned Close are covered. Ordinary wire observations can repeat request IDs without clearing retired TP history. This passive first-observed contract does not infer PduR routing, configured ECU restart behavior, SD, payload schemas or authenticated invocation. Fixed AUTOSAR R22-11 requirements and installed Wireshark4.2.5 independently anchor selected wire facts; originals and specification PDF remain outside Git.

Selected PPI version0 flags0 Ethernet captures retain optional aggregation-interface identity in the capture domain. Exact header/TLV bounds reject unselected metadata without guessing prefixes on other declared links. Inner DCP Ethernet bytes preserve existing independent answers. IPv4/IPv6 fragmented transports remain isolated by aggregate interface. Emitted IP diagnostics privately retain their original field-query refusal against caller cache/protocol/status edits, preserving public errors/ExpertCode and manually constructed cache compatibility. Selected SOME/IP1 SD interface1 wire observations preserve ordered Find/Offer/Stop/Subscribe/Ack/Nack fields, two option runs (including zero-count nonzero-index receiver tolerance), IPv4/IPv6 endpoint/multicast tuples and discardable opaque extensions with shared count/depth/byte/pool budgets. No service availability, receiver state machine, subscription/reboot/authentication or device execution is verified. The new batch seals66 unique minimal captures with exact independent answers and own scripts; all earlier archives remain unchanged.

Selected plain type3 HDLC lifecycle RLRQ/RLRE/ABRT information now has bounded8-frame/4099-byte assembly, independent frame CRC/address/sequence validation and selected complete ACSE fields. Exact accepted final request retries retain the first complete request identity; retirement still rejects reused/unknown generations. Only complete validated information creates a reply link. Existing GET/LOW continuation bytes that resemble lifecycle prefixes remain payload, and completed normal GET profiles come from the assembled result. Native DLMS UDP Close maps physical endpoints back to caller-relative direction; capture orientation remains unchanged. Full fields, Session, IDs, typed errors, source/provenance, Close/resource/ownership, and54 unique minimal captures with independent exact answers are sealed. Reference HDLC encoding is separate from normative wire/state answers and proves neither credential verification nor actual meter release. All earlier ZIPs and original pending requirements remain intact.
