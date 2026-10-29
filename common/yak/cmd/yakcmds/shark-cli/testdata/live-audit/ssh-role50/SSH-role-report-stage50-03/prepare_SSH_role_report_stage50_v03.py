"""Stage role50 counts/report and exact Python source only, outside the repo.
This file never invokes Git, Go, TShark, SDK binaries or copied runner code.
"""
from pathlib import Path
import ast,hashlib,json,re,shutil,collections
P=Path(r'C:/tmp/yak-shark-ESP-SSH-UI-20261028')
R=Path(r'C:/Users/V/.codex/worktrees/shark-protocol-infrastructure/yaklang')
D=Path(__file__).resolve().parent
SRC='26f01f4baa5fd39b88deadf901257a70bc5c1dfe'
UI='7fcbf0ac931c6881fd5a3baf4eba8d5c9c9889f8'
def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb')as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def demand(t,msg):
    if not t:raise AssertionError(msg)
def put(p,x):
    with Path(p).open('x',encoding='utf-8')as f:json.dump(x,f,ensure_ascii=False,indent=2,sort_keys=True);f.write('\n')
def cert(rel):
    p=P/rel;return {'path':str(p),'SHA256':sha(p),'bytes':p.stat().st_size}
CERTS={
 'source50':'SSH-role-source50-01/plan02/manifest.json',
 'source_selftest':'SSH-role-source50-01/selftest02/summary.json',
 'SDK_plan03':'SSH-role-API-CLI-source50-03/plan03/manifest.json',
 'SDK_selftest03':'SSH-role-API-CLI-source50-03/selftest03/summary.json',
 'SDK_role50':'SSH-role-native-26f50-01/independent-audit03/summary.json',
 'SDK_identity49_compatibility':'SSH-identity-native-26f50-01/independent-audit/summary.json',
 'reference50':'SSH-role-reference50-01/closure01/summary.json',
 'reference_comparison02':'SSH-role-reference50-01/comparison02/summary.json',
 'reference_erratum':'SSH-role-reference50-01/closure01/erratum.json',
 'targeted_red':'test-SSH-role-red50-01/receipt.json',
 'targeted_green':'test-SSH-role-green50-02/receipt.json',
 'full':'test-SSH-26f-full50-03/receipt.json',
 'fuzz':'SSH-role-fuzz-26f50-01/summary.json',
 'CI':'SSH-role-CI-26f50-01.json',
 'native_preservation':'SSH-role-public-native-bridge50-01/summary.json',
 'CLI_preservation':'SSH-role-public-CLI-bridge50-01/summary.json',
 'Windows_producers':'windows-SSH-role-26f50-01/producer-summary.json',
 'Windows_root_closure':'Windows-SSH-role-root-closure50-01/summary.json',
 'Windows_root_source_inspection':'Windows-SSH-role-scope-discovery50-01/observations.json',
 'Windows_child_readonly_inspection':'SSH-preview-UI-readonly50-01/summary.json',
 'preview_policy02':'SSH-role-API-CLI-source50-02/plan02/opaque-preview-policy.json',
 'summary_metadata_supplement03':'SSH-role-API-CLI-source50-03/plan03/Summary-metadata-supplement.json',
}
# Check paths before creating the archive; no candidate runtime output is loaded.
for name,rel in CERTS.items():demand((P/rel).is_file(),'certificate exists '+name+': '+rel)
metadata={k:read(P/v)for k,v in CERTS.items()}
audit=metadata['SDK_role50'];identity=metadata['SDK_identity49_compatibility']
demand(audit['passed']and identity['passed'],'closed SDK audits')
demand(audit['original64_fields_unchanged'],'original fields unchanged')
demand(audit['counts']['payload_callbacks']==64 and audit['counts']['SDK_callbacks']==71,'callback scope')
demand(audit['counts']['status_counts']=={'decoded':58,'malformed':4,'context-required':2},'source status counts')
demand(audit['counts']['API_literal_occurrences']==645 and audit['counts']['private_SSH_occurrences']==243,'source field counts')
full=metadata['full'];demand(full['exit_code']==0 and full['test_actions']=={'pass':285825,'skip':24},'full tests closed')
demand(full['all_runs_have_unique_terminal']and full['run_count']==285849,'full terminal closure')
demand(sha(P/'test-SSH-26f-full50-03/tests.jsonl')==full['log_SHA256'],'full log exact receipt SHA')
ci=metadata['CI'];demand(ci['source']==SRC and ci['all_four_exact_source_runs_success'],'CI source')
demand(len(ci['runs'])==4 and all(x['head']==SRC and x['conclusion']=='success'and x['attempt']==1 for x in ci['runs']),'four first-attempt source CI')
nb=metadata['native_preservation'];cb=metadata['CLI_preservation']
demand(nb['before_source'].startswith('cc9d840')and nb['after_source']==SRC and nb['all_exports_byte_identical']and nb['differences']==0 and nb['ID_mapping']is False,'native whole literal preservation')
demand(nb['native_event_rows']==307819 and nb['whole_flow_files']==2 and nb['inputs']==37,'native preservation counts')
demand(cb['after_source']==SRC and cb['whole_rows']==384518 and cb['differences']==0 and cb['ID_mapping']is False,'CLI whole literal preservation')
fuzz=[]
for target,expected in [('FuzzSSHClientRoleNegotiationIsolation',104687),('FuzzSSHNativeRefusalFieldIsolation',84705)]:
    receipt=P/'SSH-role-fuzz-26f50-01'/(target+'-receipt.json');r=read(receipt);log=receipt.parent/(target+'.log')
    demand(r['source']==SRC and r['exit_code']==0 and sha(log)==r['log_SHA256'],'fuzz producer '+target)
    xs=re.findall(r'execs: (\d+)',log.read_text(encoding='utf-8'));demand(xs and int(xs[-1])==expected,'fuzz executions')
    fuzz.append({'target':target,'executions':expected,'configured_fuzz_seconds':60,'parallel_workers':2,'exit_code':0,'failures':0,'receipt':cert(str(receipt.relative_to(P))),'log_SHA256':r['log_SHA256']})
# Windows data is historical closed observation metadata, not a source oracle.
win=metadata['Windows_producers'];demand(win['source']==SRC and win['closed_cases']==2,'Windows two closed cases')
for run in win['runs']:demand(run['exit_code']==0 and run['inputs_unchanged'],'Windows helper closed')
obs=metadata['Windows_root_source_inspection'];demand(len(obs)==7,'root whole-byte observations')
for x in obs:
    demand(x['displayed_frame_lengths']==[246,246],'actual physical frame lengths')
    demand(x['equals_entire_physical']if x['snapshot']!='02-first-byte-source'else x['equals_retained_preview'],'actual whole byte-source check')
closure=metadata['Windows_root_closure'];demand(closure['source']==SRC and closure['all_physical_frame_lengths_and_bytes_preserved']and closure['role_diagnostic_page_passed']and closure['encrypted_preview_caption_passed']is False,'root historical UI closure');demand(sha(P/'close_Windows_SSH_role50.py')==closure['checker_SHA256'],'root closure exact checker source')
ws=metadata['reference50'];demand(ws['all_TShark_processes_closed']and ws['real_capture_count']==0,'reference closed authored scope')

# Only Python sources; preserve original bytes and keep original folder layout.
items=[]
def add(rel,role,evidence):items.append((P/rel,role,evidence))
for rel,role in [
 ('SSH-role-source50-01/ssh_role_source50.py','original failed source guard; retained historical code'),
 ('SSH-role-source50-01/ssh_role_source50_v02.py','source-only nine-connection wire constructor and selftest'),
 ('SSH-role-API-CLI-source50-01/ssh_role_API50.py','independent neutral source to existing SSH API labels'),
 ('SSH-role-API-CLI-source50-01/ssh_role_native_cli50.py','original whole SDK/CLI verifier and source plan'),
 ('SSH-role-API-CLI-source50-02/ssh_role_native_cli50_v02.py','source-derived opaque preview64 supplement and strict producer binding'),
 ('SSH-role-API-CLI-source50-03/ssh_role_native_cli50_v03.py','four Summary metadata corrections from immutable source formula'),
 ('SSH-role-reference50-01/ssh_role_reference50.py','first source to TShark field/span mapping and comparison'),
 ('SSH-role-reference50-01/ssh_role_reference50_v02.py','reference missing-direction alias correction; original primitive table preserved'),
 ('SSH-role-reference50-01/close_ssh_role_reference50.py','reference closure and preserved differences'),
 ('SSH-preview-UI-readonly50-01/audit_ssh_preview_UI50.py','first read-only source/cells auditor; retained wrong policy-path failure'),
 ('SSH-preview-UI-readonly50-01/audit_ssh_preview_UI50_v02.py','closed whole physical/preview cells byte-source audit')]:add(rel,role,'source50 / SDK03 / reference50 / Windows child inspection as applicable')
for rel,role in [
 ('run_SSH_role_native50.py','root actual events/CLI/descriptor execution controller'),
 ('run_SSH_identity_native50.py','root source49 compatibility execution wrapper'),
 ('inspect_SSH_role_before50.py','root old-code four wrong-role discovery'),
 ('inspect_SSH_role_binding50.py','root opaque Raw-length/hash-only binding discovery'),
 ('prepare_Windows_SSH_role50.py','root source-frozen Windows cases constructor'),
 ('run_Windows_SSH_role50.py','root Windows source/binary/hash-bound execution controller'),
 ('inspect_Windows_SSH_role50.py','root whole physical/retained-preview byte inspection'),
 ('close_Windows_SSH_role50.py','root source-bound Windows whole-byte and no-invented-role-range closure; old caption failure preserved'),
 ('run_go_SSH50.py','root constrained Go test runner wrapper; code only, not executed by staging'),
 ('run_build_SSH50.py','root constrained build runner wrapper; code only, not executed by staging'),
 ('run_fuzz_SSH50.py','root constrained fuzz runner wrapper; code only, not executed by staging')]:add(rel,role,'closed root producers / tests / fuzz')
add(str(Path(__file__).relative_to(P)),'external source-only archival/counts report generator; no runtime/Git invocation','this staging manifest')

# Existing source-only dependencies are referenced and hash checked, not copied.
archive_base=R/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit'
depends=[]
for group,names in [
 ('ssh-identity49',['source/build_ssh_source49.py','native/ssh_source_API49.py','native/ssh_native_cli49.py','native/ssh_native_cli49_v02.py','native/ssh_native_cli49_v03.py','run_SSH_native49.py','reference/run_ssh_reference49.py','windows/renderer.py']),
 ('rsvp-control47',['run_go_temp.py','run_build_temp.py','run_fuzz_membership.py','verify_Windows_OSPFv3_36.py'])]:
    manifest=archive_base/group/'source-manifest.json';m=read(manifest)
    table={x['path']:x for x in m['files']}
    for name in names:
        demand(name in table,'existing dependency manifest member '+group+'/'+name)
        path=archive_base/group/name;rec=table[name];demand(sha(path)==rec['SHA256'],'existing source dependency SHA')
        depends.append({'path':str(path.relative_to(R)).replace('\\','/'),'SHA256':sha(path),'bytes':path.stat().st_size,'manifest_path':str(manifest.relative_to(R)).replace('\\','/'),'manifest_SHA256':sha(manifest),'copied':False})
for name in ['public_native_literal_continuity.py','public_cli_literal_continuity.py']:
    path=archive_base/name;depends.append({'path':str(path.relative_to(R)).replace('\\','/'),'SHA256':sha(path),'bytes':path.stat().st_size,'copied':False})
# Existing native-console source is only a dependency pointer, never copied here.
p=archive_base/'discovery/console/main_windows.go';depends.append({'path':str(p.relative_to(R)).replace('\\','/'),'SHA256':sha(p),'bytes':p.stat().st_size,'language':'Go','copied':False,'role':'already present Windows native-console helper source; actual executable remains external'})

archive=D/'source-archive';archive.mkdir()
entries=[];scan={'parsed_Python_sources':0,'private_key_markers':0,'literal_bytes_ge64':0,'hex_string_literals_ge128':0,'base64_string_literals_ge256':0,'forbidden_file_suffixes':0}
for original,role,evidence in items:
    demand(original.suffix=='.py','Python only');raw=original.read_bytes();text=raw.decode('utf-8-sig');tree=ast.parse(text,filename=str(original))
    scan['parsed_Python_sources']+=1
    # Diagnostics only report counts/length/hash, never literal contents.
    for node in ast.walk(tree):
        if not isinstance(node,ast.Constant):continue
        v=node.value
        if isinstance(v,bytes)and len(v)>=64:scan['literal_bytes_ge64']+=1
        if isinstance(v,str):
            scan['private_key_markers']+=bool(re.search(r'-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----',v))
            scan['hex_string_literals_ge128']+=len(v)>=128 and bool(re.fullmatch(r'[0-9a-fA-F\s]+',v))
            scan['base64_string_literals_ge256']+=len(v)>=256 and bool(re.fullmatch(r'[A-Za-z0-9+/=\r\n]+',v))
    demand(not re.search(rb'-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----',raw),'no embedded private key marker')
    rel=original.relative_to(P);destination=archive/rel;destination.parent.mkdir(parents=True,exist_ok=True)
    with destination.open('xb')as f:f.write(raw)
    demand(sha(destination)==sha(original),'source archive byte identity')
    entries.append({'path':str(rel).replace('\\','/'),'original_path':str(original),'bytes':len(raw),'SHA256':sha(original),'language':'Python','role':role,'historical_evidence_scope':evidence})
demand(scan['literal_bytes_ge64']==0 and scan['hex_string_literals_ge128']==0 and scan['base64_string_literals_ge256']==0,'no literal capture/packet or key-like blobs in selected code')
# No external source facts/expected rows or future ranges generator are copied.
source_manifest={'scope':'Exact Python source only; no captures/raw/expected/runtime outputs/receipts/images/logs/vendor source copied. Restore original external input layout and referenced dependencies to rerun. Copying code is not executing it.','production_source':SRC,'source_files':len(entries),'source_bytes':sum(e['bytes']for e in entries),'files':entries,'existing_repository_dependencies':depends,'scan':scan,'excluded':['PCAP/PCAPNG/keylogs/keys','raw authored or captured wire blobs','full plans/cases/expected/source JSON','actual SDK/CLI/PDML/TSV exports','producer receipts/full test logs','PNG/cells/binaries','downloaded RFC or vendor C','future SSH wire-ranges source51 ledger/scripts'],'original_failed_code_retained':True}
put(archive/'source-manifest.json',source_manifest)

history=[
 ('SSH-role-source50-01/plan01/failure.json','source guard typo before any packet construction; kept, v02 derives the existing pinned49 hash'),
 ('SSH-role-reference50-01/comparison01/summary.json','original missing generated direction alias all([]) bug; comparator02 only corrects unknown-role evidence,726 field rows unchanged'),
 ('SSH-role-native-26f50-01/independent-audit01/summary.json','checker01 incorrectly required complete192-byte SDK Raw for a preexisting64-byte opaque preview'),
 ('SSH-role-native-26f50-01/independent-audit02/summary.json','checker02 static Summary conditional misread, not a codec regression'),
 ('SSH-role-before-cc9d50-01/independent-audit02/summary.json','old product still fails source role status atframe24 after preview schema correction'),
 ('SSH-preview-UI-readonly50-01/audit01-failure.json','read-only inspector used policy path without plan02; v02 fixes only path'),
]
for rel,_ in history:demand((P/rel).is_file(),'historical evidence remains '+rel)
aggregate={
 'scope':'SSH known-method sender-role correctness at source26f. All nine source50 connections are authored, not live. This counts-only document does not close all SSH conformance or cryptographic correctness.',
 'production_source':SRC,'separate_UI51_source':UI,'staging_did_not_invoke':['Git','Go','SDK','TShark','native UI helper'],
 'source50':{'authored_connections':9,'observed_SYN_generations':8,'noSYN_role_unknown_generations':1,'physical_frames':115,'payload_callbacks':64,'real_captures':0,'same_tuple_reuse_in_source50':False,'same_tuple_reuse_evidence':'retained source49 case06 tocase07, independently revalidated with26f','bad_then_correct_same_connection_evidence':'root144 method-role/direction/deferred/chunk unit variants; bad source50 capture flows end withFIN and do not independently prove next-correct-packet recovery'},
 'SDK':audit['counts'],'source49_compatibility':{'passed':True,'payload_callbacks':76,'close_diagnostics':11,'SDK_total_callbacks':87,'physical_rows':148,'API_literal_occurrences':842,'private_alias_occurrences':275,'SSH_descriptors':12,'all_original_payload_fields_unchanged':True},
 'public_preservation':{'inputs':37,'mix':'frozen public/owned/matrix preservation suite; not37 newly captured live inputs','before_source':nb['before_source'],'after_source':SRC,'native_event_rows':307819,'whole_flow_objects':2,'CLI_whole_rows':384518,'differences':0,'all_exports_byte_identical':True,'ID_mapping':False,'limitation':'preservation proves unchanged whole exports, not correctness of every existing protocol interpretation'},
 'tests':{'full':{'pass':285825,'skip':24,'fail':0,'runs':285849,'all_runs_have_unique_terminal':True,'seconds':full['seconds'],'log_SHA256':full['log_SHA256']},'historical_red_actions':metadata['targeted_red']['test_actions'],'targeted_green_actions':metadata['targeted_green']['test_actions'],'role_direction_deferred_chunk_leaf_variants':144,'fuzz':fuzz},
 'CI':{'source':SRC,'verified_UTC':ci['verified_UTC'],'four_exact_source_first_attempts_success':True,'runs':ci['runs'],'no_UI51_CI_claim':True},
 'reference':{'tool':'Wireshark TShark4.6.9','decode_as':'tcp.port==2222,ssh only explicit reference adapter; not a product content-identification proof','physical_frames':115,'per_dissection_context_counts':ws['contexts']['explicit-ssh-reference']['counts'],'natural_direction_labels_disagree_with_SYN':10,'wrong_role_no_reference_role_expert':[24,36,89,103],'wrong_role_reference_hostkey_fields':[36,103],'private_method_padding_unrendered_frame':47,'primitive_vs_owner_scope':'671 exact rendered primitives,36 zero-width observations with no nonempty highlight proof,6 aggregate owners,12 optional whole blobs not rendered,1 padding not rendered; not726 exact primitive matches','primary_RFC_links':['https://www.rfc-editor.org/rfc/rfc4253#section-8','https://www.rfc-editor.org/rfc/rfc5656#section-4','https://www.rfc-editor.org/rfc/rfc4419#section-3'],'vendor_source_external_URL':ws['reference_implementation']['URL'],'vendor_source_external_SHA256':ws['reference_implementation']['SHA256'],'vendor_source_copied':False},
 'Windows26f':{'closed_cases':2,'source_frames':[36,13],'full_physical_bytes_each':246,'actual_frame_length_labels':[246,246],'whole_byte_inspections':7,'wrong_client_reply':'SSH/malformed, expectedserver/observedclient; no successful method field or fingerprint claim','remaining_actual_defect':'64-byte opaque slice called SSH reassembled PDU; even fully retained48-byte encrypted stream chunk has no proven SSH packet boundary','physical_capture_data_corruption':False,'source_preview_frame13':{'observed_stream_bytes':192,'retained_Raw_bytes':64,'not_retained_by_SDK_bytes':128,'remaining_tail_only_proven_by_physical_capture_and_input_SHA':True},'root_corrected_visual_misread':'64 onwire/64 captured was not present; all original frame13 views retained246/246'},
 'Windows26f_root_closure':{'role_diagnostic_page_passed':True,'all_physical_frame_lengths_and_bytes_preserved':True,'encrypted_preview_caption_passed':False,'known_UI_caption_gap_preserved':True,'role_metadata_selection_does_not_change_right_byte_attributes':True,'default_cache_packets':20000,'default_history_mib':64,'root_viewed_actual_images':True},
 'staging_history':{'previous_stage':str(P/'SSH-role-report-stage50-02/staging-manifest.json'),'previous_stage_SHA256':sha(P/'SSH-role-report-stage50-02/staging-manifest.json'),'reason':'report prose reviewed; same closed evidence and scope asstage02; earlier stages preserved'},
 'UI51':{'source':UI,'scope':'display-only protectedSSHEvent observation/encrypted stream bytes orpreview wording; API/private pdu.id/physical metadata unchanged','state_at_role50_staging':'source fix and read-only review recorded; bins51 build/runtime results are outside this report','no_runtime_pass_claim':True},
 'checker_corrections':{'original64_source_rows_SHA256':'fbbcda79bd880324bfc541649f271580b626a764e88af14cd554e39eee5eac77','all_original_field_trees_preserved':True,'v02':'only opaque frame13 retained source prefix64 of192; complete source wire stays frozen; whole-tail SDK coverage expresslyfalse','v03':'four Summary strings from immutable consumeSession source formula SSH plusfrozenPacketName, not candidate field values','runtime_stats_API_executed_for_preview_accounting':False},
 'open_boundaries':['SSH GetFields/typed/UI wire ranges are not implemented by26f orUI51; independent source51 ledger stays external and is not archived here','complete SSH message ordering/rekey ornegotiated hostkey/signature conformance beyond selected scope remains open','no encrypted SFTP plaintext orcryptographic authentication/host trust claim','not a live SSH capture campaign; source50 only authored9 and source49 retained12'],
 'historical_failures':[{'certificate':cert(rel),'interpretation':why}for rel,why in history],
 'certificates':{k:cert(rel)for k,rel in CERTS.items()},
 'source_archive':{'source_files':len(entries),'source_bytes':sum(e['bytes']for e in entries),'manifest_path':str(archive/'source-manifest.json'),'manifest_SHA256':sha(archive/'source-manifest.json'),'scan':scan,'source_bytes_unchanged':True},
 'disk_free_bytes_at_stage':shutil.disk_usage(P).free,
}
put(D/'aggregate-summary.json',aggregate)
report=f'''# SSH known-method sender roles: source 26f

Production `{SRC}` rejects a complete, clear SSH method message when its sender contradicts an independently observed TCP role. The refused message keeps SSH identity and a precise diagnostic, without standard method fields or a host-key fingerprint. Its complete transport boundary allows a later valid message to proceed. Unknown roles, private methods, ignored guesses and post-NEWKEYS bytes remain separate cases. A missing earlier message alone is not a sender-role contradiction. The selected roles follow [RFC4253 §8](https://www.rfc-editor.org/rfc/rfc4253#section-8), [RFC5656 §4](https://www.rfc-editor.org/rfc/rfc5656#section-4) and [RFC4419 §3](https://www.rfc-editor.org/rfc/rfc4419#section-3).

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

The 37-input suite mixes frozen public, owned and matrix inputs. Entire exports are byte-identical, including IDs; there is no ID rewriting or field normalization. This proves preservation, rather than the correctness of every existing protocol interpretation. The full receipt binds log `{full['log_SHA256']}` and all 285,849 terminal test results. Source26f CI comprises Essential 40954741876, Diff 40954741831, case 40954741828 and traffic 40955120040. No later UI51 CI result is claimed here.

The first role regression retains 145 failed test actions: 144 leaf role/direction/deferred/chunk variants and their parent, with 2 other passes. The later focused run has 543 passes. Same-connection recovery after a bad message is proven by the root's 144 variants. Source50's bad capture flows end with FIN, so they do not independently prove next-message recovery. Same-tuple reuse belongs to source49 cases 06→07, revalidated with source26f; source50 has nine distinct tuples.

## Independent Wireshark comparison

TShark 4.6.9 ran in isolated natural and explicit `tcp.port==2222,ssh` contexts. This adapter is a reference configuration, not proof of content identification. Each context accounts for **671 exact rendered primitives, 36 zero-width observations, 6 aggregate-owner spans, 12 optional whole blobs not rendered and 1 unrendered padding field**. Zero-width fields do not prove a nonempty highlight, and aggregate owners are not exact primitive spans. These are not 726 exact primitive matches.

Reference packets 24, 36, 89 and 103 have no sender-role expert; packets 36 and 103 still render host-key fields. Natural dissection has 10 direction labels inconsistent with the observed SYN when the server banner arrives first; the explicit adapter matches the known role. A no-SYN reference direction is not source role proof. Private-method frame 47 reaches reference fallback DH and loses padding rendering, although its captured generic SSH envelope is complete. These reference differences remain preserved. The official vendor C source is referenced by URL/hash only and is not copied.

## Windows and opaque preview boundary

Two source26f Windows cases closed on frames 36 and 13. Frame 36 shows SSH/malformed with Expected Sender Role=server and Observed Sender Role=client. The root's closure proves complete **246-byte physical sources**, exact retained sources and unchanged byte attributes when generated role metadata is selected: there is no invented role range. It ran with default 64 MiB history and 20,000 cached packets.

The physical capture pane and frame header remain **246 bytes on wire / 246 captured**. The earlier visual suspicion of 64/64 was corrected; it was not capture corruption. There is a real old-page defect: the retained 64-byte encrypted slice is called `SSH · reassembled PDU`. Even a fully retained 48-byte post-NEWKEYS arrival is not a proven SSH PDU. The historical root certificate therefore records `role_diagnostic_page_passed=true` and `encrypted_preview_caption_passed=false`.

Frame 13 contains 192 observed opaque stream bytes. The preexisting ProbeBytes=64 policy retains 64 SDK Raw bytes while Length remains 192. Across both opaque observations, 240 bytes are consumed, 112 retained and 128 absent from SDK Raw. SourceBytes SHA/refs cover the retained preview. Only the physical capture and input hashes prove the remaining 128 bytes; the SDK does not expose them. Neither a complete encrypted message boundary nor whole-tail SDK coverage is claimed. Counter interpretation was read from immutable code; a separate stats API probe was not run.

UI source `{UI}` separately changes this caption to an observation and encrypted stream bytes, or a retained/observed preview count. It does not change private pdu.id, SDK fields, Raw, Length or physical metadata. This report records that source fix and its read-only review only; later bins51, Windows and public-suite results belong to a separate report.

## Preserved history and archive

Every failed version remains external. Source-prepare01 failed a guard-hash typo before packet construction. Checker01 assumed full 192-byte opaque Raw retention. Checker02 corrected that expectation from preexisting source policy without changing any of the original 64 field trees. Checker03 changes only four Summary metadata expectations using the immutable `consumeSession` formula (`SSH ` plus frozen PacketName); its earlier outer-condition reading was wrong. Codec fields, CLI captions, preview policy and role facts were not weakened. The reference empty-direction-alias correction and the read-only inspector's policy-path typo also remain as harness history.

The staged archive contains **{len(entries)} exact Python files, {sum(e['bytes']for e in entries):,} bytes**. Its manifest records original paths, roles, SHA256 and existing ssh-identity49/rsvp-control47 dependencies without copying those dependencies again. No capture, key, raw wire, complete source/expected plan, SDK/CLI/PDML export, producer receipt, image, log, binary, RFC or vendor C is included. Runners keep their original external-path and hash guards. Archiving source does not execute it. `aggregate-summary.json` binds the external certificates and the original failures by hash.

SSH wire ranges remain open. Source51's independent 1,487-span ledger is external preparation, rather than an implemented GetFields/typed/UI feature, and is excluded from this archive. Broader message-order/rekey and host-key/signature conformance also remain open. There is no claim to decrypt SFTP, authenticate a signature, trust a host key or complete all core protocols.
'''
with(D/'report.md').open('x',encoding='utf-8')as f:f.write(report)
put(D/'staging-manifest.json',{'scope':'external staging only; root reviews/copies later; repository and frozen source evidence not modified','production_source':SRC,'separate_UI_source':UI,'source_archive_files':len(entries),'source_archive_bytes':sum(e['bytes']for e in entries),'artifacts':[{'path':str(D/n),'SHA256':sha(D/n),'bytes':(D/n).stat().st_size}for n in ['report.md','aggregate-summary.json','source-archive/source-manifest.json']],'generator':{'path':str(Path(__file__)),'SHA256':sha(__file__)},'external_frozen_sources_unchanged':all(sha(e['original_path'])==e['SHA256']for e in entries),'future_range_plan_not_touched':True,'runtime_processes_started':0})
print(json.dumps({'directory':str(D),'manifest_SHA256':sha(D/'staging-manifest.json'),'report_SHA256':sha(D/'report.md'),'summary_SHA256':sha(D/'aggregate-summary.json'),'source_manifest_SHA256':sha(archive/'source-manifest.json'),'source_files':len(entries),'source_bytes':sum(e['bytes']for e in entries),'scan':scan},ensure_ascii=False))