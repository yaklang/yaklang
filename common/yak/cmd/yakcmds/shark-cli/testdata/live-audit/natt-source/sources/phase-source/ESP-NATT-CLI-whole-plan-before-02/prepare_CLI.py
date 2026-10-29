"""Freeze all seven full CLI rows from old originals and the agreed ESP contract.

Reads no new native/CLI producer or output. Physical UDP Layers are preserved;
only the source-derived PDU and explicit presentation contract are substituted.
"""
from pathlib import Path
import collections,copy,hashlib,json,subprocess,sys
FROZEN01=Path(r'C:\tmp\yak-shark-ESP-SSH-UI-20261028\ESP-NATT-whole-plan-before-01')
sys.path.insert(0,str(FROZEN01))
from natt_expected import same,canonical,sha_bytes
root=Path(__file__).resolve().parent;out=root/'frozen02';out.mkdir(exist_ok=False)
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
baseline_commit='e41da9c4401f9f97108daa2b99d3f463aa52a32d'
def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,indent=2,ensure_ascii=False,allow_nan=False)+'\n',encoding='utf8')

contract=dict(origin='Parent root explicit source/presentation contract before any new producer was read',
    protocol='ESP',status='limited',visibility='headers',scope='observed clear ESP header and opaque remainder',
    evidence='SA context unavailable; authentication, decryption and replay not verified',
    Info='exact frozen native e.Summary containing UDP encapsulation',physical_Layers='preserved original UDP layers',
    ProtocolStack='append ESP to original physical UDP chain',connection_or_flow_identity='none added',
    profile='esp-natt-observed-header',admission_in_public_capture='observed-UDP4500-nonzero-SPI',
    explicit_DecodeAs_guard_also_supported='explicit-decode-as; does not change seven public-capture expectations')
write(out/'presentation-contract.json',contract)
source_paths=['common/yak/cmd/yakcmds/shark-cli/presentation.go',
    'common/yak/cmd/yakcmds/shark-cli/packet_observation.go','common/yak/cmd/yakcmds/shark-cli/decode.go',
    'common/yak/cmd/yakcmds/shark-cli/command.go']
sources={}
for path in source_paths:
    proc=subprocess.run(['git','show',baseline_commit+':'+path],cwd=repo,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    assert proc.returncode==0
    target=out/(Path(path).name+'.e41-source.txt');target.write_bytes(proc.stdout)
    sources[path]=dict(commit=baseline_commit,source_file=target.name,SHA256=sha(target))
assert contract['scope'].encode() in (out/'presentation.go.e41-source.txt').read_bytes()
assert contract['evidence'].encode() in (out/'presentation.go.e41-source.txt').read_bytes()
plan=read(FROZEN01/'frozen01/plan.json');frozen_path=FROZEN01/'frozen01/seven-complete-native-expectations.json'
assert sha(frozen_path)==plan['outputs'][frozen_path.name]
frozen=read(frozen_path);complete=[]
for source in frozen:
    old=source['frozen_CLI_row'];native=source['expected_native_event']
    assert old['layers']==['Ethernet','IPv4','UDP'] and old['transport']=='udp' and old['source_port']==old['destination_port']==4500
    assert old['number']==source['frame'] and old['capture_domain']==native['domain']
    assert old['pdu_scope']=='observed_at_packet' and len(old['pdus'])==1
    assert old['pdus'][0]['id']==native['id'] and 'protocol_stack' not in old and 'visibility' not in old
    values=native['field_values'];fields={}
    for item in values['values']:fields.setdefault(item['name'],[]).append(item['value'])
    pdu=dict(id=native['id'],protocol=native['protocol'],status=native['status'],completeness=native['completeness'],
        response_to=native['response_to'],byte_source=copy.deepcopy(native['source_bytes']),
        fields=fields,field_values=copy.deepcopy(values))
    # No error/expert/summary PDU keys: recordPDUAtObservation adds those only
    # for actual error/expert or close publication, none of which applies.
    expected=copy.deepcopy(old)
    expected.update(pdus=[pdu],protocol='ESP',protocol_status='limited',visibility='headers',
        protocol_scope=contract['scope'],protocol_evidence=contract['evidence'],info=native['summary'],
        protocol_stack=copy.deepcopy(old['layers'])+['ESP'])
    assert expected['layers']==old['layers'] and expected['transport']==old['transport']
    assert all(key not in expected for key in ['connection_protocol','connection_stack','stream_id','conversation_id','tcp_flow_id'])
    complete.append(dict(frame=source['frame'],baseline_ordinal=source['baseline_CLI_ordinal'],
        baseline_line_SHA256=source['frozen_CLI_line_SHA256'],baseline_whole_row=copy.deepcopy(old),expected_whole_row=expected,
        expected_whole_row_canonical_SHA256=sha_bytes(canonical(expected))))
by_ordinal={r['baseline_ordinal']:r for r in complete}
weber=next(r for r in plan['CLI'] if r['relative_file']=='preservation/weber/yak.jsonl')
assert sha(weber['baseline_file'])==weber['baseline_SHA256']
digest=hashlib.sha256();seen=set();count=0
for ordinal,line in enumerate(Path(weber['baseline_file']).open('rb'),1):
    old=json.loads(line);value=old
    if ordinal in by_ordinal:
        row=by_ordinal[ordinal];assert same(old,row['baseline_whole_row']) and sha_bytes(line)==row['baseline_line_SHA256']
        value=row['expected_whole_row'];seen.add(ordinal)
    digest.update(canonical(value)+b'\n');count+=1
assert seen==by_ordinal.keys() and count==weber['baseline_rows']
negative=[];seed=complete[0]['expected_whole_row']
for name,mutation in [
    ('fake-physical-ESP-layer',lambda v:v['layers'].append('IPSecESP')),
    ('missing-progressive-ESP',lambda v:v['protocol_stack'].pop()),
    ('SA-verified-visibility',lambda v:v.__setitem__('visibility','authenticated-plaintext')),
    ('port-hint-only-label',lambda v:v.__setitem__('protocol','UDP')),
    ('old-preview-hash',lambda v:v['pdus'][0]['byte_source'].__setitem__('SHA256',complete[0]['baseline_whole_row']['pdus'][0]['byte_source']['SHA256'])),
    ('extra-PDU-error-key',lambda v:v['pdus'][0].__setitem__('error','')),
    ('new-conversation-ID',lambda v:v.__setitem__('conversation_id',1)),
    ('wrong-source-role',lambda v:v.__setitem__('source',v['destination']))]:
    value=copy.deepcopy(seed);mutation(value);assert not same(value,seed);negative.append(name)
value=dict(scope=__doc__,complete_expected_available=True,after_output_used=False,
    baseline_CLI_runtime_SHA256=plan['CLI_runtime_SHA256'],seven_native_plan_SHA256=sha(frozen_path),
    source_contract_bindings={'parent_contract_file':'presentation-contract.json','parent_contract_SHA256':sha(out/'presentation-contract.json'),
        'existing_production_source_snapshots':sources,'constructor_SHA256':sha(Path(__file__)),
        'frozen01_manifest_SHA256':sha(FROZEN01/'manifest01.json')},
    required_complete_rows=complete,expected_Weber_whole_export_canonical_SHA256=digest.hexdigest(),
    expected_Weber_whole_rows=count,all_other_CLI_rows_and_36_other_exports_literal=True,ID_mapping=False,
    source_before_new_output=True,negative_whole_object_controls=negative)
write(out/'complete-CLI-plan.json',value)
print(json.dumps(dict(plan=str(out/'complete-CLI-plan.json'),SHA256=sha(out/'complete-CLI-plan.json'),complete_rows=7,whole_Weber_rows=count,negative_controls=len(negative),after_output_used=False)))
