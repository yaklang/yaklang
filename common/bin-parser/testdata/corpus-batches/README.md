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
go test -json -count=1 -timeout=5m ./internal/trafficfixture/... ./common/bin-parser/... ./common/pcapx/... ./common/yak/cmd/yakcmds/shark-cli ./scripts/ci > traffic.jsonl
go run ./internal/trafficfixture/cmd/corpus exec -- python3 @scripts/protocol-tests/check_go_test_json.py traffic.jsonl --inventory @scripts/protocol-tests/required-tests-traffic-unified-v20.json --tier full
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

Current test requirements are sealed at `scripts/protocol-tests/required-tests-traffic-unified-v20.json`;
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

The current workflow and executable gate use `required-tests-traffic-unified-v20.json`. Older
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
context, and CFG-3 remains explicitly unsupported in this native profile.
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
The active inventory is `scripts/protocol-tests/required-tests-traffic-unified-v20.json`.

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

`validation-dlms-hdlc-normal-data.zip` contains31 owned minimal native normal-GET capture cases and72 ordered independent answers (53 complete byte/field observations,19 expected refusals). It covers TCP/UDP full/deferred, workers1/2/4, observer off/on, TCP feed chunks, nested compact, node/depth boundaries, malformed/trailing contents, strict text, full flags and refusal followed by late feedback. The existing native scalar contract and zero TransactionID are retained. Its source pins and executable independent builder/oracle/reference harness stay in the small ZIP; original reference sources and execution logs stay outside Git. Actual fixed reference codec display/calendar differences remain diagnostics rather than device passes. Current mandatory inventory is `scripts/protocol-tests/required-tests-traffic-unified-v20.json`.

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
The current inventory is `scripts/protocol-tests/required-tests-traffic-unified-v20.json`.

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
is `scripts/protocol-tests/required-tests-traffic-unified-v20.json`; earlier
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
the416-root unifiedv20 inventory is the current executable entry. These
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
archives remain immutable. Active mandatory inventory is unifiedv20 (416 unique
full-tier roots); prior v18 and its source materials remain archived.


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
All historical archives remain byte-identical; unifiedv20 is the active gate.
