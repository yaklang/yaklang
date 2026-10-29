"""Freeze actual Windows page expectations from independent packet sources."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
field_plan=root/'OSPFv3-field-plan-before-native-02/frozen02/plan.json'
assert sha(field_plan)=='0f7caa92c06cf6971fe13cae804810b6cab4bbda38235d1f7cd78dd7a3326036'
fp=json.loads(field_plan.read_text('utf8'));grid=Path(fp['fields_session_grid']);assert sha(grid)==fp['fields_session_grid_SHA256']
source=next(r for line in grid.open(encoding='utf8')for r in[json.loads(line)]if r['source_identity']['input']==37 and r['source_identity']['frame']==48906)
closure=root/'OSPFv3-native-ledger-closure36-01.json';assert sha(closure)=='b79a428f0badf6919588ce6b677944444cc4f65257359c7f9c1f809d19f7df12'
c=json.loads(closure.read_text('utf8'));record=next(r for r in c['files']if r['export']=='preservation/weber/events.jsonl')
ids=record['ledgers'][1];assert sha(ids['path'])==ids['SHA256'];pdu=json.loads(Path(ids['path']).read_text('utf8'))['48906']
assert pdu==40097
raw=Path(source['original_frame_file']).read_bytes();assert sha(source['original_frame_file'])==source['original_frame_SHA256']
start,end=source['source_message_physical_range'];wire=raw[start:end]
prefix=source['expected_fields']['LSAs'][2]['Body']['Prefixes'][0]
a,b=prefix['Wire Field Ranges']['Prefix'];assert [a,b]==[144,152] and len(raw)==334 and [start,end]==[90,330] and len(wire)==240
helper=root/'windows-PIM-helper26/native-console.exe';hr=helper.parent/'receipt.json'
assert sha(helper)=='d18d9f2ae0eb965d5c1cebdbb2de3f93ef189f96c7dc0186dfc7ab20bcd7bb5f'
renderer=root/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
capture=Path(r'C:\tmp\yak-shark-public-corpus-20261024\weber-20260710\The-Ultimate-PCAP.pcapng');keys=capture.with_name('tls.keys')
assert sha(capture)==source['source_identity']['capture_SHA256']
out=root/'Windows-OSPFv3-source-plan-before37-01';out.mkdir()
case=dict(name='OSPFv3-Weber48906',source='68527c4d9355d2df37c5c0c9b8bc19f7766ca9c6',frame=48906,pdu=pdu,protocol='OSPFv3',query='fields pdu.id == '+str(pdu),field='Prefix: '+prefix['Prefix'],field_name='Prefix',field_value=prefix['Prefix'],field_down=85,field_PDU_range=[a,b],field_physical_range=[start+a,start+b],physical_file=source['original_frame_file'],physical_SHA256=sha(source['original_frame_file']),physical_bytes=len(raw),PDU_range=[start,end],PDU_bytes=len(wire),PDU_SHA256=hashlib.sha256(wire).hexdigest(),field_wire_hex=wire[a:b].hex(),progressive_stack=['FPP','Ethernet','Dot1Q','IPv6','IPSecAH','OSPFv3'],capture=str(capture),capture_SHA256=sha(capture),keys=str(keys),keylog_SHA256=sha(keys),physical_records=51328,packet_cache_default=20000,history_budget_default_mib=64,no_budget_increase=True,helper=str(helper),helper_SHA256=sha(helper),helper_receipt=str(hr),helper_receipt_SHA256=sha(hr),renderer=str(renderer),renderer_SHA256=sha(renderer),field_plan=str(field_plan),field_plan_SHA256=sha(field_plan),native_ledger_closure=str(closure),native_ledger_closure_SHA256=sha(closure),authentication='AH observed; authentication unverified')
case['required_human_type_names']=['LS Type: 0x2009 (Intra-Area-Prefix-LSA)', 'Referenced LS Type: 0x2001 (Router-LSA)']
(out/'case.json').write_text(json.dumps(case,indent=2)+'\n',encoding='utf8')
files={}
for name in['prepare_Windows_OSPFv3_37.py','run_Windows_OSPFv3_37.py','verify_Windows_OSPFv3_37.py']:
 p=root/name;files[name]=dict(path=str(p),SHA256=sha(p))
plan=dict(scope=__doc__,allowed_source=case['source'],source_case_SHA256=sha(out/'case.json'),files=files,expectations_from='independent original807 source grid and closed whole-native ID proof; no Windows output read',actual_UI_execution_performed=False,default_budgets_required=True,console_cells_not_terminal_pixel_screenshot=True)
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'),case_SHA256=sha(out/'case.json'))))
