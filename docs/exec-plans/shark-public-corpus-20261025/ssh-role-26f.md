# SSH known-method sender roles: source 26f

Production `26f01f4baa5fd39b88deadf901257a70bc5c1dfe` rejects a complete, clear SSH method message when its sender contradicts an independently observed TCP role. The refused message keeps SSH identity and a precise diagnostic, without standard method fields or a host-key fingerprint. Its complete transport boundary allows a later valid message to proceed. Unknown roles, private methods, ignored guesses and post-NEWKEYS bytes remain separate cases. A missing earlier message alone is not a sender-role contradiction. The selected roles follow [RFC4253 §8](https://www.rfc-editor.org/rfc/rfc4253#section-8), [RFC5656 §4](https://www.rfc-editor.org/rfc/rfc5656#section-4) and [RFC4419 §3](https://www.rfc-editor.org/rfc/rfc4419#section-3).

## Closed scope

Source50 contains **9 authored connections and 115 physical frames**. Eight connections have SYN role proof; one has no SYN and stays role-unknown. This is not a live SSH capture. Its 64 payload callbacks comprise 58 decoded observations, 4 malformed sender-role diagnostics and 2 encrypted/context observations. Seven FIN-triggered diagnostics give **71 SDK callbacks**, rather than 71 physical packets. They are published on physical FIN records; there are no extra standalone CLI records.

The frozen checker03 validates all 64 Session/GetFields trees, **645 API literal occurrences, 243 private SSH alias occurrences and 12 descriptors**, plus 64 payload and 7 FIN SDK/CLI bridges. Wrong-role GetFields remains nil/error. A client reply cannot gain server host-key fields or a fingerprint. Private names remain literal negotiated names with an opaque layout. The two protected sentinels are authored opaque bytes; they do not prove successful cryptography.

The retained source49 compatibility campaign also passes: **76 payload callbacks + 11 FIN diagnostics = 87 SDK callbacks**, over 148 physical frames, with 842 API literal occurrences and 275 private aliases. These are separate corpora and their event counts must not be described as one physical-packet count.

| Closed source26f check | Result |
|---|---:|
| Whole native preservation against cc9, 37 inputs |307,819 event rows + 2 whole flow objects; 0 differences|
| Whole CLI preservation against cc9, 37 inputs |384,518 complete rows; 0 differences|
| Full five-package tests |285,825 passed; 24 skipped; 0 failed|
| Focused SSH tests |543 passed|
| FuzzSSHClientRoleNegotiationIsolation |104,687 executions; 60 configured fuzz seconds; 0 failures|
| FuzzSSHNativeRefusalFieldIsolation |84,705 executions; 60 configured fuzz seconds; 0 failures|
| Exact-source CI |4 runs passed on attempt 1|

The 37-input suite mixes frozen public, owned and matrix inputs. Entire exports are byte-identical, including IDs; there is no ID rewriting or field normalization. This proves preservation, rather than the correctness of every existing protocol interpretation. The full receipt binds log `dcdd291115b50bd8568e773331377477d2edb7d78126e5652d53443c99a0cb27` and all 285,849 terminal test results. Source26f CI comprises Essential 40954741876, Diff 40954741831, case 40954741828 and traffic 40955120040. No later UI51 CI result is claimed here.

The first role regression retains 145 failed test actions: 144 leaf role/direction/deferred/chunk variants and their parent, with 2 other passes. The later focused run has 543 passes. Same-connection recovery after a bad message is proven by the root's 144 variants. Source50's bad capture flows end with FIN, so they do not independently prove next-message recovery. Same-tuple reuse belongs to source49 cases 06→07, revalidated with source26f; source50 has nine distinct tuples.

## Independent Wireshark comparison

TShark 4.6.9 ran in isolated natural and explicit `tcp.port==2222,ssh` contexts. This adapter is a reference configuration, not proof of content identification. Each context accounts for **671 exact rendered primitives, 36 zero-width observations, 6 aggregate-owner spans, 12 optional whole blobs not rendered and 1 unrendered padding field**. Zero-width fields do not prove a nonempty highlight, and aggregate owners are not exact primitive spans. These are not 726 exact primitive matches.

Reference packets 24, 36, 89 and 103 have no sender-role expert; packets 36 and 103 still render host-key fields. Natural dissection has 10 direction labels inconsistent with the observed SYN when the server banner arrives first; the explicit adapter matches the known role. A no-SYN reference direction is not source role proof. Private-method frame 47 reaches reference fallback DH and loses padding rendering, although its captured generic SSH envelope is complete. These reference differences remain preserved. The official vendor C source is referenced by URL/hash only and is not copied.

## Windows and opaque preview boundary

Two source26f Windows cases closed on frames 36 and 13. Frame 36 shows SSH/malformed with Expected Sender Role=server and Observed Sender Role=client. The root's closure proves complete **246-byte physical sources**, exact retained sources and unchanged byte attributes when generated role metadata is selected: there is no invented role range. It ran with default 64 MiB history and 20,000 cached packets.

The physical capture pane and frame header remain **246 bytes on wire / 246 captured**. The earlier visual suspicion of 64/64 was corrected; it was not capture corruption. There is a real old-page defect: the retained 64-byte encrypted slice is called `SSH · reassembled PDU`. Even a fully retained 48-byte post-NEWKEYS arrival is not a proven SSH PDU. The historical root certificate therefore records `role_diagnostic_page_passed=true` and `encrypted_preview_caption_passed=false`.

Frame 13 contains 192 observed opaque stream bytes. The preexisting ProbeBytes=64 policy retains 64 SDK Raw bytes while Length remains 192. Across both opaque observations, 240 bytes are consumed, 112 retained and 128 absent from SDK Raw. SourceBytes SHA/refs cover the retained preview. Only the physical capture and input hashes prove the remaining 128 bytes; the SDK does not expose them. Neither a complete encrypted message boundary nor whole-tail SDK coverage is claimed. Counter interpretation was read from immutable code; a separate stats API probe was not run.

UI source `7fcbf0ac931c6881fd5a3baf4eba8d5c9c9889f8` separately changes this caption to an observation and encrypted stream bytes, or a retained/observed preview count. It does not change private pdu.id, SDK fields, Raw, Length or physical metadata. This report records that source fix and its read-only review only; later bins51, Windows and public-suite results belong to a separate report.

## Preserved history and archive

Every failed version remains external. Source-prepare01 failed a guard-hash typo before packet construction. Checker01 assumed full 192-byte opaque Raw retention. Checker02 corrected that expectation from preexisting source policy without changing any of the original 64 field trees. Checker03 changes only four Summary metadata expectations using the immutable `consumeSession` formula (`SSH ` plus frozen PacketName); its earlier outer-condition reading was wrong. Codec fields, CLI captions, preview policy and role facts were not weakened. The reference empty-direction-alias correction and the read-only inspector's policy-path typo also remain as harness history.

The staged archive contains **23 exact Python files, 194,175 bytes**. Its manifest records original paths, roles, SHA256 and existing ssh-identity49/rsvp-control47 dependencies without copying those dependencies again. No capture, key, raw wire, complete source/expected plan, SDK/CLI/PDML export, producer receipt, image, log, binary, RFC or vendor C is included. Runners keep their original external-path and hash guards. Archiving source does not execute it. `aggregate-summary.json` binds the external certificates and the original failures by hash.

SSH wire ranges remain open. Source51's independent 1,487-span ledger is external preparation, rather than an implemented GetFields/typed/UI feature, and is excluded from this archive. Broader message-order/rekey and host-key/signature conformance also remain open. There is no claim to decrypt SFTP, authenticate a signature, trust a host key or complete all core protocols.
