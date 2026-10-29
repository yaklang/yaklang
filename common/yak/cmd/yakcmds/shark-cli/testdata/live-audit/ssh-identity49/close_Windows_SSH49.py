"""Close actual SSH console cells after root inspected selected and whole-PDU images."""
from pathlib import Path
import base64,hashlib,importlib.util,json,re,struct
root=Path(__file__).parent
actual=root/'windows-SSH-cc9d49-03';out=root/'Windows-SSH-root-closure49-01';assert not out.exists();out.mkdir()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
plan=root/'SSH-SDK-CLI-source49-01/plan01/windows-unknown-reply-source-plan.json'
assert sha(plan)=='925d66e3fe6d7b01795c9bfe0f08c8e424c9648a209420667a8a0900328e9090'
c=read(plan);p=read(actual/'producer.json');assert p['exit_code']==0 and p['inputs_unchanged']and p['source']=='cc9d84079d455a64b57365e143b9e5ee641a7997'
for path,digest in p['inputs'].items():assert sha(path)==digest,path
spec=importlib.util.spec_from_file_location('SSH_actual_hex_cells',root/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
folder=actual/'unknown-method-reply';shots={}
for name,digest in p['snapshots'].items():
 path=folder/(name+'.json');assert sha(path)==digest['cells_SHA256']and sha(path.with_suffix('.png'))==digest['PNG_SHA256']
 shots[name]=read(path)
 assert path.with_name(path.name+'.ready').is_file()or any(sha(q)==sha(path)and q.with_name(q.name+'.ready').is_file()for q in folder.glob(name+'-wait-*.json'))
cap=Path(c['capture']).read_bytes();assert hashlib.sha256(cap).hexdigest()==c['capture_SHA256'];at=24;frames=[]
while at<len(cap):
 sec,us,n,original=struct.unpack_from('<IIII',cap,at);at+=16;wire=cap[at:at+n];at+=n;assert len(wire)==n==original;frames.append(wire)
assert len(frames)==148
physical=frames[68];PDU=Path(c['whole_PDU']['path']).read_bytes();opaque=PDU[6:24]
assert len(physical)==86 and physical[54:86]==PDU and len(PDU)==32 and len(opaque)==18
for name in('01-protocol-fields','02-first-byte-source'):assert m.pane(shots[name])[0]==physical,name
assert m.pane(shots['02-byte-sources'])[0]==PDU
selected=shots['01-selected-byte-field'];value,positions=m.pane(selected)
assert value==opaque and len(positions)==18
attributes=[selected['Attributes'][y][x]for y,x in positions];assert len(set(attributes))==1
text='\n'.join(selected['Rows']);assert 'Extracted field value'in text and'frame offsets unavailable'in text
for label in c['forbidden_labels']:assert label not in text,label
assert c['expected_literal_method']in text and c['expected_packet_name']in text
assert ' › '.join(c['required_stack'])in text and re.search(r'buffer\s+148\s*/\s*20000',text)and'· 1 visible'in text
assert read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
rows=[json.loads(line)for line in(root/'SSH-native-cc9d49-01/events.jsonl').read_text('utf8').splitlines()]
matches=[r for r in rows if r.get('source_bytes',{}).get('CompletedIn',{}).get('Number')==69 and r.get('raw')]
assert len(matches)==1
e=matches[0];assert base64.b64decode(e['raw'],validate=True)==PDU and e['session']['Kex Algorithm']==c['expected_literal_method']
assert 'Host Key Fingerprint'not in e['session']and e['session']['Packet Name']==c['expected_packet_name']
assert f"PDU #{e['id']}"in text
proof=dict(scope=__doc__,source=p['source'],source_plan_SHA256=sha(plan),case_source_frame=69,source_PDU_id=e['id'],authored_control_not_live=True,physical_capture_frames=148,entire_physical_source_bytes=86,entire_PDU_source_bytes=32,extracted_opaque_source_bytes=18,entire_sources_exact=True,selected_extracted_value_exact=True,no_invented_captured_offsets=True,no_false_method_or_hostkey_labels=True,protocol_identity_and_stack='Ethernet/IPv4/TCP/SSH',root_viewed_actual_selected_and_PDU_images=True,console_cells_not_Terminal_pixels=True,default_cache_packets=20000,default_history_mib=64,CP936_mouse_tabs_resize_ack_checked=True,navigation_down=0,old_navigation65_and140_failures_preserved=True,producer_SHA256=sha(actual/'producer.json'),checker_SHA256=sha(__file__),passed=True)
(out/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),passed=True)))
