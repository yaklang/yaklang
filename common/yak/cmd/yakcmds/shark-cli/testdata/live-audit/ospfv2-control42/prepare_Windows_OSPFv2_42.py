"""Freeze Windows cases from original public packets; use exact extracted bytes.

Each one-packet replay is a derived inspection fixture, not a fresh live capture
or a claim that the full public capture fits the default20k history cache.
Original public frame identity and hashes are retained in the external plan.
"""
from pathlib import Path
import hashlib,json,struct
root=Path(__file__).parent;out=root/'Windows-OSPFv2-source-plan42-01';out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
grid=root/'OSPFv2-independent-source39-01/source-grid03/input-37-source-grid.json'
summary=grid.with_name('summary.json');assert sha(summary)=='2aaf89759d9b4a2e44ff1b075778694d2b2c9eea8db3a3f12b19bae0ffdbbc65'
record=next(r for r in json.loads(summary.read_text('utf8'))['captures']if r['input']==37);assert record['source_grid_SHA256']==sha(grid)
rows=json.loads(grid.read_text('utf8'))
hello=next(r for r in rows if r['source']['fields']['Header']['Type']==1 and r['source']['fields']['Header']['Authentication Type']==0 and 'LLS'in r['source']['fields'])
update=next(r for r in rows if r['source']['fields']['Header']['Type']==4 and len(r['source']['fields']['LSAs'])==1 and r['source']['fields']['LSAs'][0]['LS Type']==2)
crypto=next(r for r in rows if r['source']['fields']['Header']['Authentication Type']==2 and 'LLS'in r['source']['fields'])
cases=[]
for name,fact,path,field,value,down in [('Hello',hello,['Header','Type'],'Type','1 (Hello)',0),('Network-LSA',update,['LSAs',0,'LS Type'],'LS Type','2 (Network-LSA)',26),('Cryptographic-LLS',crypto,['LLS','TLVs',1,'Type'],'TLV Type','2 (LLS Cryptographic Authentication)',44)]:
 raw=Path(fact['original_frame']).read_bytes();assert sha(fact['original_frame'])==fact['original_frame_SHA256'];first,last=fact['PDU_range'];wire=raw[first:last];assert hashlib.sha256(wire).hexdigest()==fact['PDU_SHA256']
 node=fact['source']['fields']
 for key in path[:-1]:node=node[key]
 a,b=node['Wire Field Ranges'][path[-1]]
 capture=out/(name+'.pcap');ts=fact['time_ns'];sec,ns=divmod(ts,10**9)
 capture.write_bytes(struct.pack('<IHHIIII',0xa1b23c4d,2,4,0,0,65535,1)+struct.pack('<IIII',sec,ns,len(raw),fact['physical_original_length'])+raw)
 helper=root/'windows-PIM-helper26/native-console.exe';renderer=root/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
 cases.append(dict(name=name,original_public_capture=fact['capture'],original_public_capture_SHA256=fact['capture_SHA256'],original_public_frame=fact['frame'],source_grid=str(grid),source_grid_SHA256=sha(grid),capture=str(capture),capture_SHA256=sha(capture),physical_file=fact['original_frame'],physical_SHA256=sha(fact['original_frame']),physical_bytes=len(raw),PDU_range=[first,last],PDU_SHA256=fact['PDU_SHA256'],PDU_bytes=len(wire),query='fields ospf.version == 2',field_name=field,field_value=value,field_down=down,field_PDU_range=[a,b],field_physical_range=[first+a,first+b],field_hex=wire[a:b].hex(),progressive_stack=['Ethernet','IPv4','OSPFv2'],helper=str(helper),helper_SHA256=sha(helper),renderer=str(renderer),renderer_SHA256=sha(renderer),packet_cache_default=20000,history_budget_default_mib=64,derived_one_packet_not_live=True))
write(out/'cases.json',cases)
write(out/'plan.json',dict(scope=__doc__,source_grid=str(grid),source_grid_SHA256=sha(grid),cases_SHA256=sha(out/'cases.json'),candidate_read=False,source_cases=3,console_cells_not_terminal_pixels=True))
print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'),cases=[dict(name=c['name'],frame=c['original_public_frame'],selected=c['field_PDU_range'])for c in cases])))
