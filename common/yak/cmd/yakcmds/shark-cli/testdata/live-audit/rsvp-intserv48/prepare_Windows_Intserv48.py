"""Freeze two original-frame Intserv console checks before candidate execution."""
from pathlib import Path
import hashlib,json,struct
root=Path(__file__).parent
out=root/'Windows-Intserv-source-plan48-01'
assert not out.exists();out.mkdir()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def put(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
source=root/'Intserv-source48-01/frozen-plan01'
assert sha(source/'manifest.json')=='1b57dd45ed6ea226d62569a1f9c6f06981bc85bef2210d115e30d68fc3e7ad66'
facts=read(source/'source-facts.json')
grid=root/'RSVP-baseline-source45-01/source-grid.json'
assert sha(grid)=='9d31302c649ded298b26474aa98d1e245e2116086ac839bd48355852606262ad'
frame=next(x for x in read(grid)if x['frame']==1)
physical=Path(frame['physical_path']).read_bytes();wire=Path(frame['PDU_path']).read_bytes()
first,last=frame['PDU_physical_range'];assert physical[first:last]==wire
helper=root/'Windows-console-ack-build45-01/native-console.exe'
renderer=root/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
assert sha(helper)=='12e0aa1b8b3cfb4696e1d680334dae490e269de37773e01aafa35bc4a6d8b68c'
assert sha(renderer)=='427379a3b9c1ac7e8257a81d3d6e478904fd96659468f231bbec57a55e336e83'
rows=[]
for name,cls,key,nav in [('Official-Intserv-Token-Rate',12,'Token Bucket Rate',110),('Official-Intserv-Path-MTU',13,'Composed Path MTU',205)]:
 fact=next(x for x in facts if x['origin']=='real'and x['sourcecase'].startswith('real-frame1-')and x['object_class']==cls)
 leaf=next(x for x in fact['source']['leaves']if x['literal']and x['path'].endswith('.Body.'+key))
 lo,hi=leaf['range'];assert len(wire[lo:hi])==4
 if leaf['kind']=='ieee32':
  value=leaf['value']['exact_decimal'];field='Value';expected=leaf['value']['wire_uint32']
 else:value=str(leaf['value']);field=key;expected=leaf['value']
 assert int.from_bytes(wire[lo:hi],'big')==expected
 rawpath=out/(name+'.frame');rawpath.write_bytes(physical)
 cap=out/(name+'.pcap');sec,nano=divmod(frame['time_ns'],10**9);assert nano%1000==0
 cap.write_bytes(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1)+struct.pack('<IIII',sec,nano//1000,len(physical),len(physical))+physical)
 rows.append(dict(name=name,original_public_capture=str(root/'RSVP-primary-download45-01/rsvp-PATH-RESV.pcap'),original_public_capture_SHA256='aa7d8448cd5eaf17112e67adf3e801cb45119e62c63c75f132ad596edfc10361',original_public_frame=1,source_grid=str(grid),source_grid_SHA256=sha(grid),Intserv_source_manifest_SHA256=sha(source/'manifest.json'),Intserv_sourcecase=fact['sourcecase'],neutral_field_path=leaf['path'],capture=str(cap),capture_SHA256=sha(cap),physical_file=str(rawpath),physical_SHA256=sha(rawpath),physical_bytes=len(physical),PDU_range=[first,last],PDU_SHA256=sha(frame['PDU_path']),PDU_bytes=len(wire),query='proto:rsvp',field_name=field,field_value=value,field_down=nav,field_PDU_range=[lo,hi],field_physical_range=[first+lo,first+hi],field_hex=wire[lo:hi].hex(),progressive_stack=['Ethernet','IPv4','RSVP'],helper=str(helper),helper_SHA256=sha(helper),renderer=str(renderer),renderer_SHA256=sha(renderer),packet_cache_default=20000,history_budget_default_mib=64,derived_one_packet_not_live=True,authored_control=False))
put(out/'cases.json',rows)
put(out/'plan.json',dict(scope=__doc__,candidate_read=False,source_cases=2,cases_SHA256=sha(out/'cases.json'),constructor_SHA256=sha(__file__),source_plan_sha256=sha(source/'manifest.json'),console_cells_not_terminal_pixels=True,navigation_only_may_need_later_adjustment=True))
print(json.dumps(dict(plan=str(out),SHA256=sha(out/'plan.json'),cases=[dict(name=x['name'],key=x['field_name'],value=x['field_value'],PDU=x['field_PDU_range'])for x in rows])))
