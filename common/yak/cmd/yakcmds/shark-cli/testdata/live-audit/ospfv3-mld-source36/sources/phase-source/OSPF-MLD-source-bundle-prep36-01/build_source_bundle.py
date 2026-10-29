"""Copy reviewed OSPF/MLD audit sources only, with raw-byte provenance.

No capture/key/complete case, plan, expectation, export, PDML, image, executable
or log is copied. Prior38-source NATT dependencies are referenced by manifest
and exact SHA, never duplicated. No production source or Go command is used.
"""
import argparse,ast,base64,collections,datetime,hashlib,json,re,shutil,sys
from pathlib import Path

PHASE=Path(__file__).resolve().parent.parent
OLD_NATT_SHA='c206f729e4ead2430be02b297f86c833fa9820aad736598a8b9ccbe9ff0002e7'
SOURCE='636816f1da7c97228a6adf11cbc363a41c53c33f'


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def write(p,x):
    with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2,ensure_ascii=False);f.write('\n')


def proof(relative,scope,expected=None):
    path=PHASE/relative;actual=sha(path)
    if expected:assert actual==expected,relative
    return dict(path=str(path),SHA256=actual,scope=scope,copied=False)


def payload_scan(path,packet_bytes):
    raw=path.read_bytes();text=raw.decode('utf-8-sig');candidates=[];static_events=0
    if path.suffix=='.py':
        tree=ast.parse(text)
        for node in ast.walk(tree):
            if isinstance(node,ast.Constant):
                value=node.value
                if isinstance(value,bytes)and len(value)>=16:candidates.append(value)
                if isinstance(value,str):
                    compact=''.join(value.split())
                    if len(compact)>=32 and len(compact)%2==0 and re.fullmatch('[0-9a-fA-F]+',compact):
                        candidates.append(bytes.fromhex(compact))
                    if len(compact)>=48 and len(compact)%4==0 and re.fullmatch('[A-Za-z0-9+/]+={0,2}',compact):
                        try:candidates.append(base64.b64decode(compact,validate=True))
                        except ValueError:pass
            if isinstance(node,(ast.List,ast.Tuple))and len(node.elts)>=16:
                if all(isinstance(x,ast.Constant)and type(x.value)is int and 0<=x.value<=255 for x in node.elts):
                    candidates.append(bytes(x.value for x in node.elts))
            if isinstance(node,ast.Dict):
                keys={k.value for k in node.keys if isinstance(k,ast.Constant)and isinstance(k.value,str)}
                if {'raw','protocol','source_bytes'}<=keys:
                    try:ast.literal_eval(node);static_events+=1
                    except (ValueError,TypeError,SyntaxError):pass
    secret_lines=len(re.findall(r'(?:CLIENT_RANDOM|(?:CLIENT|SERVER)_(?:HANDSHAKE_)?TRAFFIC_SECRET(?:_\d+)?)\s+[0-9a-fA-F]{64}\s+[0-9a-fA-F]{32,}',text))
    private_keys=len(re.findall(r'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----',text))
    unique=set(candidates);full_matches=sum(any(packet in value for packet in packet_bytes if len(packet)<=len(value))for value in unique)
    return dict(source_SHA256=hashlib.sha256(raw).hexdigest(),literal_byte_candidates=len(candidates),
                unique_literal_byte_candidates=len(unique),full_original_packet_literal_matches=full_matches,
                NSS_secret_line_matches=secret_lines,private_key_block_matches=private_keys,
                complete_static_event_object_literals=static_events,
                safe=full_matches==secret_lines==private_keys==static_events==0,
                result_scope='Counts/hashes only. AST literals plus exact full-original-frame containment; source code manually reviewed for dynamic source loading, not embedded packet/secret material.')


def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists()
    old_manifest=PHASE/'NATT-source-only-bundle-a82-01/bundle-manifest.json';assert sha(old_manifest)==OLD_NATT_SHA
    old=read(old_manifest);assert old['copied_files']==38
    old_by_sha={r['SHA256']:r for r in old['source_files']}
    for row in old['source_files']:assert sha(old_manifest.parent/row['path'])==row['SHA256']
    proofs={
      'inputs':proof('OSPFv3-independent-source37-a82-01/inputs01/inputs.json','Original37 input inventory, not copied','d7f1ce273e976e2a37193cdf3c27dd50bb6d95a3717c62f96f473af105e13aa4'),
      'census':proof('OSPFv3-independent-source37-a82-01/scan01/summary.json','Original383936 records;807 source-complete OSPFv3 observations','78bb8a45668758d38f31e9c3a77a0a3ed15a9cbb23f9065328553e59d167fb2b'),
      'WS':proof('OSPFv3-independent-source37-a82-01/wireshark01/summary.json','Fresh37 closed TShark reference runs','3783c568cc131493b3b6afea2ab76e8d8ac24e68ac98e62ce803c735843aaaae'),
      'fieldsWS':proof('OSPFv3-independent-source37-a82-01/reference-field-audit01/summary.json','28344 named source/reference field checks;11 literal semantic presentation notes retained','795071791d702ccedbe9ad48dfbcd0302675019e9412cd0fe6ac33f114d2019e'),
      'fragments':proof('OSPFv3-independent-source37-a82-01/fragments01/summary.json','136 IPv6 fragments,68 reconstructed groups;no added OSPF source','ad5acdb0b6fd17574e0b77fe81c2636d8badfab3532426d6f8840d388fd83b95'),
      'canonical':proof('OSPFv3-independent-source37-a82-01/counts-and-canonical01/summary.json','Counts/RFC applicability and immutable a82 canonical source review','b13ac164680fb56aedc80ecb32d7da27ae312a4d080c316591e3f7a75830e819'),
      'legacy':proof('OSPFv3-legacy-errors-source-a82-01/actual01/summary.json','Pinned gopacket1.3.1 access model and old a82 CLI43 failures','2da5bf2fcee195f10e294afbcecfdd2298420186d2ca5e30f2b30113266ee14e'),
      'contract01':proof('OSPFv3-field-contract36.json','Original pure naming/type contract;historical incorrect dedicated exact_value assumption','6d7e5d649ecdd224eea1d081692a6a721d38471488494b3aac39adec40d6af4e'),
      'contract02':proof('OSPFv3-field-contract36-v02.json','Corrected public Bits>53 exact_value contract;no packet values','ae0359a175fdf9fe8350f37284a1398ca85904fce75b5153c08aec9f9c601a7d'),
      'field01':proof('OSPFv3-field-plan-before-native-01/frozen01/plan.json','Historical field-only source plan,external only','e88420197a8d6c9e403c56c31d129608604cfa481136e6fdd2acbc9c98783e5b'),
      'field02':proof('OSPFv3-field-plan-before-native-02/frozen02/plan.json','Current807 exact source fields/session and typed occurrence plan,external only','0f7caa92c06cf6971fe13cae804810b6cab4bbda38235d1f7cd78dd7a3326036'),
      'fieldCorrection':proof('OSPFv3-field-plan-before-native-02/bridge02.json','Contract correction only;807 fields/session trees literal,13846 dedicated exact_value removals','cfd1afd01eb9c40f91362468466849a32f8c353966e743880155bbb0f9dd7157'),
      'fieldControl01':proof('OSPFv3-field-plan-before-native-01/validator-selftest01/summary.json','Historical controller controls,not production pass','b7bec1310c8ff356225f54970444453c6fad9bfb73b85ccf75864df3a68a63c8'),
      'fieldControl02':proof('OSPFv3-field-plan-before-native-02/validator-selftest02/summary.json','15 current verifier controls;reject fabricated32bit exact_value','169e9ffc4183aa94ca0e60a2da909345cc890c7269b80f41e860f6be865bf457'),
      'nativePlan':proof('OSPFv3-native-whole-plan-before36-02/plan.json','Source-frozen full native plan with explicit43 DecodeErrors source delta','cdd52b2aa22d1a1d67b363cbf605932a123b5ccc911706ca47c0a631d4ee1e01'),
      'native':proof('OSPFv3-native37-whole-636-01/summary.json','Actual636 closed native37 full objects,307262 events+2 flows','463250f963d5816b1e9b3565fb27ae3e444f74f4cae4455caa183d5727936022'),
      'ledger':proof('OSPFv3-native-ledger-closure36-01.json','74 explicit ID ledgers reconstructed from37 hash-bound exports','b79a428f0badf6919588ce6b677944444cc4f65257359c7f9c1f809d19f7df12'),
      'CLI01':proof('OSPFv3-CLI-whole-plan-before36-01/plan.json','Historical source plan before verifier-only Windows case-key correction','2c2a6434c79c9266a6892e3a4a249b84fa21b3178748ddcdabdf08eee22b3614'),
      'CLI02':proof('OSPFv3-CLI-whole-plan-before36-02/plan.json','Current37 whole-row source plan;no candidate values used','db59536b0b55a0d6f2f76676031f1b8459eee09d19c2dc866fe6a1567710f804'),
      'CLICorrection':proof('OSPFv3-CLI-whole-plan-before36-02/source-expectation-bridge01-to02.json','807 whole source rows,37cases and old ID bindings remain byte-literal','f306c0f89737e6991caf10d14ed9290ca856863ed7ab8aa07ff1561174f2bab6'),
      'CLIControl':proof('OSPFv3-CLI-whole-plan-before36-02/validator-selftest02/summary.json','16 auditor controls plus narrow numeric-collision control','6cb3520768632ed1fdf3fd13a897a601533e54f8c56c94a90c4a3219c4aa1437'),
      'CLI':proof('OSPFv3-CLI37-whole-636-01/summary.json','Actual636 full384518 rows;807 source-built changes;known ID references only','bfe0ad78db7d0125a3e97d80f0cdf4b74a1d184a64a13d2c32763f84fbc32ca0'),
      'WinPlan':proof('Windows-OSPFv3-source-plan-before36-01/plan.json','Source-frozen636 Windows byte/highlight plan;case not copied','5b87ce29fbd8befebc90b867073fbce4bc2e24ab73692fff3ec4aabca5a40b02'),
      'Win':proof('windows-OSPFv3-636-default01/proof.json','Actual636 automatic default-budget Windows proof;old visual_review_pending remains untouched','e2013402e53275abc3c9c98111132dcf688c1e88af5ddfff36bb0dec9fc5d8f4'),
      'full36':proof('OSPFv3-all-five-full36/receipt.json','Historical285554pass24skip3fail,not relabeled as full pass','348a8f56bc1f7cc0beb0b6529c418c9a760ea4c65d058410cba271cfd11de663'),
      'target657':proof('OSPFv3-observer36-06/receipt.json','Later targeted657pass;separate scope,not a full-suite rerun'),
      'MLD':proof('MLD-independent-source-recheck36-01/manifest01.json','Independent source/old-a82 comparison,not a new native run','20ec65922245cc92a33c9862d37f04fbe0a77063702857cbd54a1c9fd8905e3a'),
      'MLDfields':proof('MLD-independent-source-recheck36-01/field-audit01/summary.json','7340 source/private/typed/PDML checks,0differences','83be4fe75d8bb4629f048bb5c4168e72162b77cbe8abc954ec2e8d10747d2519'),
      'MLDadvanced':proof('MLD-independent-source-recheck36-01/advanced-source01/summary.json','5 existing inputs/7 observations,69 source/PDML checks,not new production execution','b487bb0acc4d4f1f7362cf6dc820dfcb934d2bf26c96b5500d91f568a22d29ea'),
      'MLDenvelope':proof('MLD-independent-source-recheck36-01/independent-envelope-source02/summary.json','220 independent source checksum/envelope observations,not native authentication','5a1b798f7dacbb070d1af2403cf09bb431ca05150a4fb481e953af606ddbbe90'),
      'MLDfailure':proof('MLD-independent-source-recheck36-01/source01/failure.json','Preserved initial display-filter controller failure;not parser failure','6f9aaf51dde72ba1c9815d2b484e64afc016b0436b300adfeab0644c3b99d8fe'),
      'CIconnector':proof('OSPF-MLD-source-bundle-prep36-01/CI-connector636-readonly01.json','Read-only connector statuses;PR automatic traffic skip retained','baccce10abc301ce8bf00d43f5856ae76147cc3a9694c1d8b5fd05e0d24583fb'),
      'CIexplicit':proof('OSPF-MLD-source-bundle-prep36-01/CI-explicit-traffic636-readonly01.json','Explicit workflow_dispatch success with head_sha636 verified','9a3344d68fc967a2a6e0390c6cf5f72d6d653444f18535e88ece501e73df0941'),
    }
    registrations=[]
    def reg(relative,role,proof_keys,dependencies=(),recommended=True):
        file=PHASE/relative;assert file.is_file()and file.suffix in('.py','.ps1')
        digest=sha(file);assert digest not in old_by_sha,'prior NATT source must be referenced,not copied'
        registrations.append(dict(original_path=str(file),relative='sources/phase-source/'+relative,
            SHA256=digest,bytes=file.stat().st_size,role=role,recommended_entrypoint=recommended,
            dependencies=list(dependencies),result_certificates=[proofs[k]for k in proof_keys]))
    base='OSPFv3-independent-source37-a82-01/'
    for name,role,certs in[
        ('inventory_inputs.py','Inventory37 original capture inputs and physical source domains',['inputs','census']),
        ('ospf_source_codec.py','Independent RFC5340 source scalar/prefix/checksum codec',['census','fieldsWS']),
        ('scan_source37.py','Bounded original L2/IP/extension/tunnel census and explicit parent refusals',['census']),
        ('audit_fragment_sources.py','Independent IPv6 fragment reconstruction and terminal protocol census',['fragments']),
        ('run_reference.py','Fresh closed TShark37 reference execution',['WS']),
        ('verify_reference_fields01.py','Exact source-vs-PDML field/value/span checks and literal notes',['fieldsWS']),
        ('summarize_sources.py','Counts/RFC applicability and immutable a82 canonical review',['canonical'])]:reg(base+name,role,certs,('Original captures and source proofs external;reader via prior NATT manifest','Local ospf_source_codec/scan_source37 when imported'))
    reg('OSPFv3-legacy-errors-source-a82-01/audit_legacy.py','Immutable gopacket1.3.1 prefix-access model;old-a82 error attribution',['legacy'],('Pinned gopacket1.3.1 layers/ospf.go external,not copied','Old a82 CLI/native exports external'))
    for directory,names,certs,current in[
        ('OSPFv3-field-plan-before-native-01',['build_source_field_plan.py','verify_field_exports.py'],['field01','fieldControl01'],False),
        ('OSPFv3-field-plan-before-native-02',['build_source_field_plan02.py','verify_field_exports02.py','bridge_contract_correction02.py'],['field02','fieldCorrection','fieldControl02'],True)]:
        for name in names:reg(directory+'/'+name,'Independent complete field/session/typed source construction or strict field-only validation;contract correction preserved',certs,('Pure contract01/02 external hashes;complete source grids/plans excluded',),current)
    nroot='OSPFv3-native-whole-plan-before36-02/frozen/'
    for name in('bridge.py','ospfv3_native_expected36.py','known_pdu_reference_paths.py','known_pdu_reference_paths_v2.py'):
        reg(nroot+name,'Full native source constructor/verifier or explicit generated-PDU reference schema',['nativePlan','native','ledger'],('External fieldplan02/source grid/closed exports;local frozen constructor/reference helpers',))
    reg('OSPFv3-native-whole-plan-before36-01/frozen/bridge.py','Preserved original verifier candidate before source-proved flow DecodeErrors delta',['legacy','nativePlan'],('Original native plan01 external;superseded by02',),False)
    reg('close_OSPFv3_native_ledgers36.py','Reconstruct and hash-bind74 exact ID ledger files from closed native exports',['ledger','native'],('External37 before/after exports and native full proof',))
    croot='OSPFv3-CLI-whole-plan-before36-02/frozen/'
    for name in('cli_source_plan.py','cli_source_expected.py','cli_pdu_references.py','native_source_schema.py','public_cli_literal_continuity.py'):
        reg(croot+name,'Complete CLI source construction/strict whole-row verification/closed-producer binding',['CLI02','CLICorrection','CLI'],('External old CLI35/fieldplan02/native ledger closure;local same-directory helpers',))
    reg('OSPFv3-CLI-whole-plan-before36-01/frozen/cli_source_plan.py','Preserved original controller with Windows relative-case-key mismatch,noticed before candidate verification',['CLI01','CLICorrection'],('Original CLI plan01 external;not current verifier',),False)
    reg('OSPFv3-CLI-whole-plan-before36-01/auditor-controls/selftest_cli_verifier.py','Historical controller negative controls before relative-path ingress control',['CLI01'],('External original source templates;not production test',),False)
    reg('OSPFv3-CLI-whole-plan-before36-02/auditor-controls/selftest_cli_verifier.py','16 whole-row controls and collision/path-key checks',['CLIControl'],('External source templates;not production test',))
    reg('OSPFv3-CLI-whole-plan-before36-02/verify_closed_cli36.ps1','Exact636 strict verifier invocation with pinned closed native summary/ledger SHA',['CLI'],('Python;current CLI verifier;closed37 producers external',))
    win_plan=read(Path(proofs['WinPlan']['path']))
    for name in('prepare_Windows_OSPFv3_36.py','run_Windows_OSPFv3_36.py','verify_Windows_OSPFv3_36.py'):
        assert sha(PHASE/name)==win_plan['files'][name]['SHA256']
        reg(name,'Source-frozen636 Windows preparation/execution/byte-source and exact prefix-highlight checking',['WinPlan','Win'],('External636 binary/build and original captures/keys/source case;renderer+native console source via prior NATT manifest',))
    mroot='MLD-independent-source-recheck36-01/'
    for name in('mld_source_layout.py','audit_mld_sources.py','audit_mld_sources02.py','verify_mld_source_fields.py','audit_existing_mld_controls.py','audit_mld_envelopes.py'):
        reg(mroot+name,'Independent MLD source layout/timers/advanced record extents/reference/envelope audit;no future full object oracle',['MLD','MLDfields','MLDadvanced','MLDenvelope','MLDfailure'],('Original inputs/old-a82 diagnostics external;capture readers via prior NATT;local MLD modules',),name!='audit_mld_sources.py')
    reg('MLD-source-before-fix-04/frozen-tools/mld_carrier_source.py','Existing independent literal parent/source position helper',['MLD'],('Original captured record,not future Native.Raw',))
    reg('OSPF-MLD-source-bundle-prep36-01/build_source_bundle.py','Review/copy/payload-scan/counts registration controller for this source-only package',[],('All registered source originals and historical proof metadata external',))
    assert len(registrations)==39
    packets=set()
    for folder in(PHASE/'OSPFv3-independent-source37-a82-01/scan01',PHASE/'MLD-independent-source-recheck36-01/source02',PHASE/'MLD-independent-source-recheck36-01/advanced-source01'):
        for file in folder.rglob('frame-*.bin'):packets.add(file.read_bytes())
    assert packets
    scans=[]
    for row in registrations:
        scan=payload_scan(Path(row['original_path']),packets);scans.append(scan)
        assert scan['source_SHA256']==row['SHA256']and scan['safe'],'source must remain external due payload/secret/static-event literal scan'
        row['payload_sensitive_scan']=scan
        if Path(row['original_path']).suffix=='.py':
            tree=ast.parse(Path(row['original_path']).read_text('utf-8-sig'))
            row['declared_imports']=sorted({v for n in ast.walk(tree)for v in ([a.name for a in n.names]if isinstance(n,ast.Import)else[n.module]if isinstance(n,ast.ImportFrom)else[])if v})
    full=read(Path(proofs['full36']['path']));assert full['exit_code']==1 and full['test_actions']=={'pass':285554,'skip':24,'fail':3}
    targeted=read(Path(proofs['target657']['path']));assert targeted['exit_code']==0 and targeted['test_actions']=={'pass':657}
    native=read(Path(proofs['native']['path']));cli=read(Path(proofs['CLI']['path']));win=read(Path(proofs['Win']['path']))
    assert native['source']==cli['source']==win['source']==SOURCE
    assert native['event_objects']==307262 and native['flow_objects']==2 and cli['whole_rows']==384518 and cli['complete_source_OSPF_rows']==807
    assert native['unplanned_differences']==cli['unplanned_differences']==0
    assert win['passed']and win['default_history_mib']==64 and win['default_packet_cache']==20000 and not win['budget_increase']
    connector=read(Path(proofs['CIconnector']['path']));explicit=read(Path(proofs['CIexplicit']['path']))
    by_id={r['id']:r for r in connector['reports'][0]['workflow_runs']}
    for identifier in(40841330734,40841330693,40841330709):assert by_id[identifier]['status']=='completed'and by_id[identifier]['conclusion']=='success'
    assert by_id[40841330896]['conclusion']=='skipped'
    assert explicit['run']['head_sha']==SOURCE and explicit['run']['conclusion']=='success'
    dependencies=[]
    for digest in('ee8248c78b9be42f8a3c4c2517c5c149a0dd009dc7cd5a1571f13e66644f5feb','dddcdcdf2d557c115272ccc5808a1266dd44cf1b55e8156e761f97722ddbf9b6','427379a3b9c1ac7e8257a81d3d6e478904fd96659468f231bbec57a55e336e83','aa0f00677d0330dde993a9006e78ea1109aace3fda863ed6ad9ebf9ff6c8391d','384b934071d956dd5caac5d7ac8e44d7de13954df0d4dd12dc8555151e28ce32'):
        item=old_by_sha[digest];dependencies.append(dict(original_path=item['original_path'],prior_bundle_path=str(old_manifest.parent/item['path']),SHA256=digest,role=item['role'],copied_again=False))
    old_go=Path(r'C:\Users\V\go\pkg\mod\github.com\gopacket\gopacket@v1.3.1\layers\ospf.go')
    dependencies.append(dict(original_path=str(old_go),SHA256=sha(old_go),role='Immutable legacy source model dependency,version1.3.1',copied_again=False))
    a.out.mkdir();copies=[]
    for row in registrations:
        src=Path(row['original_path']);dest=a.out/row['relative'];dest.parent.mkdir(parents=True,exist_ok=True)
        assert sha(src)==row['SHA256'];shutil.copyfile(src,dest)
        assert dest.read_bytes()==src.read_bytes()and sha(dest)==row['SHA256'];row['byte_identical']=True;copies.append(row)
    groups=collections.defaultdict(list)
    for r in copies:groups[r['SHA256']].append(r['relative'])
    counts=dict(scope='Aggregate metadata only for tested636 and independent source audits. No raw packets/keys/objects/outputs included.',
        source=SOURCE,native=dict(inputs=37,event_objects=307262,flow_objects=2,source_OSPF_insertions=807,unplanned_differences=0,
            flow_DecodeErrors_deltas=dict(sharon=-21,weber=-22),other_complete_flow_fields_literal=True,proof=proofs['native']),
        CLI=dict(inputs=37,whole_rows=384518,complete_source_OSPF_rows=807,legacy_error_rows=43,existing_rows_with_explicit_ID_changes=cli['existing_rows_with_explicit_generated_reference_changes'],unplanned_differences=0,proof=proofs['CLI']),
        Windows=dict(proof=proofs['Win'],default_history_mib=64,default_packet_cache=20000,budget_increase=False,automatic_pass=True,
            original_proof_visual_review_pending=win['visual_review_pending'],root_visual_review_attestation=dict(images=2,source=SOURCE,authority='Root direct task instruction confirms two636 images personally viewed;original proof not edited'),
            later_LS_Type_UI_change_excluded=True,later_UI_build_or_execution_claimed=False),
        full36_historical=dict(pass_count=285554,skip_count=24,fail_count=3,run_count=285581,exit_code=1,proof=proofs['full36'],relabelled_as_full_pass=False),
        later_targeted=dict(pass_count=657,exit_code=0,proof=proofs['target657'],does_not_replace_historical_full_run=True),
        CI636=dict(checked_read_only=True,successful_runs=[dict(id=i,name=by_id[i]['name'],conclusion='success')for i in(40841330734,40841330693,40841330709)]+[dict(id=40841322721,name=explicit['run']['name'],conclusion='success',head_sha=SOURCE)],automatic_PR_traffic=dict(id=40841330896,conclusion='skipped'),proofs=[proofs['CIconnector'],proofs['CIexplicit']]),
        OSPF_source=dict(inputs=37,physical_records=383936,source_messages=807,sharon=390,weber=417,fragments=136,reassembled_non_OSPF_groups=68,legacy_errors=43,reference_field_checks=28344,literal_reference_semantic_notes=11,uncovered_real_source_cases=['AF-instance>=32','AT/LLS','active-routing-header','OSPF fragment recovery','LSA3/4/7','unknown full LSA']),
        MLD_source_only=dict(historical_observations=237,complete_IPv6_bodies=220,noncanonical_IPv4_observations=17,historical_body_incomplete=10,all_incomplete_cross_family=True,checks=7340,advanced_checks=69,differences=0,new_native_producer=False,new_production_execution=False,proofs=[proofs['MLD'],proofs['MLDfields'],proofs['MLDadvanced'],proofs['MLDenvelope']]),
        contracts=dict(original01_preserved=True,contract01_to02_is_controller_schema_correction_not_product_bug=True,fields_and_session_trees_807_unchanged=True,removed_dedicated_exact_value_keys=13846,CLI01_to02_path_key_correction_preserved=True,all_source_row_expectations_byte_literal=True),
        copied_source_files=len(copies),copied_Python=sum(Path(r['original_path']).suffix=='.py'for r in copies),copied_PowerShell=sum(Path(r['original_path']).suffix=='.ps1'for r in copies),copied_Go=0,old_NATT38_copied_again=0,
        payload_scan_counts=dict(full_original_packet_literal_matches=sum(r['full_original_packet_literal_matches']for r in scans),NSS_secret_line_matches=sum(r['NSS_secret_line_matches']for r in scans),private_key_block_matches=sum(r['private_key_block_matches']for r in scans),static_event_objects=sum(r['complete_static_event_object_literals']for r in scans)),
        old_plans_and_failures_external_preserved=True,repository_modified=False,Go_run=False)
    write(a.out/'counts-report.json',counts)
    manifest=dict(scope=__doc__,frozen_at_UTC=datetime.datetime.now(datetime.timezone.utc).isoformat(),source_scope=SOURCE,
        builder_path=str(Path(__file__)),builder_SHA256=sha(__file__),prior_NATT_manifest=str(old_manifest),prior_NATT_manifest_SHA256=OLD_NATT_SHA,
        prior_NATT38_source_files_duplicated=0,source_files=copies,source_dependencies=dependencies,proof_bindings=proofs,
        duplicate_new_source_content_groups=[dict(SHA256=d,paths=v)for d,v in groups.items()if len(v)>1],
        external_sensitive_original_frame_count_for_scan=len(packets),counts_report_SHA256=sha(a.out/'counts-report.json'),
        only_added_metadata_files=['source-manifest.json','counts-report.json'],allowed_source_extensions=['.py','.ps1'],
        excluded=['pcap','keylog','complete cases','complete plans','complete expectations','exports','PDML','images','executables','logs','Go snapshots'],
        all_copied_source_bytes_literal=True,payload_sensitive_output_is_counts_and_hashes_only=True)
    write(a.out/'source-manifest.json',manifest)
    allowed={r['relative']for r in copies}|{'source-manifest.json','counts-report.json'}
    actual={f.relative_to(a.out).as_posix()for f in a.out.rglob('*')if f.is_file()}
    assert actual==allowed
    for r in copies:assert sha(r['original_path'])==sha(a.out/r['relative'])==r['SHA256']
    print(json.dumps(dict(bundle=str(a.out),source_files=len(copies),files=len(actual),source_bytes=sum(r['bytes']for r in copies),manifest_SHA256=sha(a.out/'source-manifest.json'),counts_report_SHA256=sha(a.out/'counts-report.json'),Go_copied=0,payload_or_secret_matches=0)),flush=True)


if __name__=='__main__':main()
