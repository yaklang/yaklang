"""Freeze the short second-LSA page from independent captured prefix bytes."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
source=root/'OSPFv3-boundary-authored-source-before-next-01'
assert sha(source/'frozen-source03/plan.json')=='e9f5b8e4f2820bf12ca79bf7049562442f0bec7540d1a910c7dd2d11da570cd0'
record=next(r for r in read(source/'frozen-source03/cases.json')if r['name']=='capture-short-next-LSA-prefix18')
facts=read(record['source_facts']);raw=Path(record['original_frame']).read_bytes()
assert sha(record['original_frame'])==record['original_frame_SHA256'] and sha(record['source_facts'])==record['source_facts_SHA256']
assert len(raw)==116 and facts['physical_PDU_start']==54 and facts['captured_PDU_bytes']==62
assert raw[100:102]==bytes([0x20,1])
closure=source/'source-result-closure04-01/manifest.json'
assert sha(closure)=='ad51df2a9ac6a76d5691fe7edd8d0095865823542fb87c0c1c493f892db8cd97'
old=root/'run_Windows_OSPFv3_38.py';run=root/'run_Windows_OSPFv3_boundary38.py';assert not run.exists()
s=old.read_text('utf8').replace('Windows-OSPFv3-source-plan-before38-01','Windows-OSPFv3-boundary-source-plan38-01').replace('verify_Windows_OSPFv3_38.py','verify_Windows_OSPFv3_boundary38.py')
s=s.replace("'capture','keys','helper'","'capture','helper'")
s=s.replace("('keys','keylog_SHA256'),",'')
s=s.replace("'native_ledger_closure'","'source_result_closure'").replace("'native_ledger_closure_SHA256'","'source_result_closure_SHA256'")
s=s.replace("case['keys'],case['query']","'-',case['query']")
s=s.replace("SHARK_QA_EXPECT_PACKETS='20000'","SHARK_QA_EXPECT_PACKETS='1'")
run.write_text(s,encoding='utf8')
out=root/'Windows-OSPFv3-boundary-source-plan38-01';out.mkdir()
helper=root/'windows-PIM-helper26/native-console.exe';renderer=root/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py';grid=source/'field-source-plan01/plan.json'
case=dict(name='OSPFv3-short-second-LSA',source='d6772965c669e288705f7e0d0e25c62972226832',frame=1,pdu=1,protocol='OSPFv3',query='fields pdu.id == 1',field='LS Type: 0x2001 (Router-LSA)',field_name='LS Type',field_value='0x2001 (Router-LSA)',field_down=40,field_PDU_range=[46,48],field_physical_range=[100,102],physical_file=record['original_frame'],physical_SHA256=sha(record['original_frame']),physical_bytes=116,physical_original_bytes=122,PDU_range=[54,116],PDU_bytes=62,PDU_SHA256=hashlib.sha256(raw[54:]).hexdigest(),field_wire_hex=raw[100:102].hex(),progressive_stack_prefix=['Ethernet','IPv6','OSPFv3'],capture=record['capture'],capture_SHA256=record['capture_SHA256'],physical_records=1,packet_cache_default=20000,history_budget_default_mib=64,no_budget_increase=True,helper=str(helper),helper_SHA256=sha(helper),helper_receipt=str(helper.parent/'receipt.json'),helper_receipt_SHA256=sha(helper.parent/'receipt.json'),renderer=str(renderer),renderer_SHA256=sha(renderer),field_plan=str(grid),field_plan_SHA256=sha(grid),source_result_closure=str(closure),source_result_closure_SHA256=sha(closure),authored_boundary_control_not_real_traffic=True,no_keys_used=True,expected_partial='Age/Type/ID/Advertising Router/Sequence/Checksum observed;Length/Body/full-LSA validation unavailable')
(out/'case.json').write_text(json.dumps(case,indent=2)+'\n',encoding='utf8')
files={}
for p in(Path(__file__),run,root/'verify_Windows_OSPFv3_boundary38.py',root/'verify_Windows_OSPFv3_36.py'):
    assert p.exists();files[p.name]=dict(path=str(p),SHA256=sha(p))
(out/'plan.json').write_text(json.dumps(dict(scope=__doc__,allowed_source=case['source'],source_case_SHA256=sha(out/'case.json'),files=files,expectations_from='independent authored source03 and closed field/range source proof;no Windows output',actual_UI_execution_performed=False,default_budgets_required=True,console_cells_not_terminal_pixel_screenshot=True),indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'),case_SHA256=sha(out/'case.json'))))
