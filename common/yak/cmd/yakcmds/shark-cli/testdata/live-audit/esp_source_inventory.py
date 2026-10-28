"""Portable source ESP inventory; runs only TShark and a literal Python reader, never the Go toolchain."""
import collections,csv,hashlib,ipaddress,json,struct,subprocess,sys,time
from pathlib import Path
import argparse
ap=argparse.ArgumentParser(description='Build an external Wireshark ESP/raw-source inventory; never modifies the captures.')
ap.add_argument('--out',type=Path,required=True)
ap.add_argument('--tshark',type=Path,required=True)
ap.add_argument('--capture-weber',type=Path,required=True)
ap.add_argument('--capture-sharon',type=Path,required=True)
ap.add_argument('--audit-modules',type=Path,default=Path(__file__).resolve().parent)
args=ap.parse_args();OUT=args.out;OUT.mkdir(exist_ok=False);AUDIT=args.audit_modules;TS=args.tshark
sys.path.insert(1,str(AUDIT))
from vxlan_endpoint_projection_audit import capture_records
from esp_source_wire import network
CAPTURES={'weber':args.capture_weber,'sharon':args.capture_sharon}

def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def run(name,cap):
 target=OUT/name;target.mkdir(exist_ok=True)
 oracle=target/'tshark-esp.tsv';err=target/'tshark-esp.stderr'
 fields=['frame.number','frame.interface_id','frame.len','frame.cap_len','frame.protocols','_ws.col.protocol','ip.src','ip.dst','ipv6.src','ipv6.dst','udp.srcport','udp.dstport','esp.spi','esp.sequence','_ws.expert.message']
 command=[str(TS),'-n','-r',str(cap),'-Y','esp','-T','fields','-E','header=y','-E','separator=\t','-E','quote=d','-E','occurrence=a']
 for key in fields:command+=['-e',key]
 started=time.time()
 with oracle.open('xb')as output,err.open('xb')as errors:
  result=subprocess.run(command,stdout=output,stderr=errors,timeout=180)
 assert result.returncode==0,(name,result.returncode)
 oracle_export_sha256=sha(oracle)
 csv.field_size_limit(64<<20)
 with oracle.open(encoding='utf-8-sig',newline='')as f:expected={int(r['frame.number']):r for r in csv.DictReader(f,delimiter='\t')}
 all_candidates=[];literal_counts=collections.Counter();frames=0
 with(target/'sources.jsonl').open('w',encoding='utf8')as out:
  for frames,rec in enumerate(capture_records(cap),1):
   literal=network(rec['wire'],rec['link'])
   if 'selection'not in literal and frames not in expected:continue
   ws=expected.get(frames)
   row={k:v for k,v in rec.items()if k!='wire'}
   row.update(frame=frames,wire_sha256=hashlib.sha256(rec['wire']).hexdigest(),literal=literal,wireshark=ws)
   if ws:
    assert int(ws['frame.cap_len'])==rec['captured_length'] and int(ws['frame.len'])==rec['original_length']
    assert int(ws['frame.interface_id'])==rec['interface']
    if 'esp_offset'in literal:
     comparison={}
     for key,wskey in [('spi','esp.spi'),('sequence','esp.sequence')]:
      word=ws[wskey]
      if key in literal:
       comparison[key]=dict(literal=literal[key],wireshark=word,match=bool(word)and ','not in word and literal[key]==int(word,0))
      else:comparison[key]=dict(literal_absent=True,wireshark=word,match=not word)
     row['header_comparison']=comparison
   literal_counts[(literal.get('selection','unselected'),literal.get('ip_version'),bool(ws))]+=1
   out.write(json.dumps(row,sort_keys=True)+'\n');all_candidates.append(row)
 summary=dict(corpus=name,frames=frames,wireshark_esp_frames=len(expected),source_candidates=len(all_candidates),literal_selection_counts=[dict(selection=k[0],ip_version=k[1],wireshark_esp=k[2],count=v)for k,v in literal_counts.items()],all_selected_ws_header_words_literal_match=all(c['match']for r in all_candidates for c in r.get('header_comparison',{}).values()),header_comparison_mismatches=[dict(frame=r['frame'],comparison=r['header_comparison'])for r in all_candidates if any(not c['match']for c in r.get('header_comparison',{}).values())],header_length_counts=dict(collections.Counter(r['literal']['esp_length']for r in all_candidates if 'esp_length'in r['literal'])),oracle_export_sha256=oracle_export_sha256,exit_code=0,unselected_ws_frames=[dict(frame=r['frame'],reason=r['literal'].get('reason'),wireshark_path=r['wireshark']['frame.protocols'],wire_sha256=r['wire_sha256'])for r in all_candidates if r['wireshark']and 'esp_offset'not in r['literal']],command=command,elapsed_seconds=round(time.time()-started,3),capture=dict(path=str(cap),sha256=sha(cap)),tshark=dict(path=str(TS),sha256=sha(TS)),oracle=dict(path=str(oracle),sha256=sha(oracle)),sources=dict(path=str(target/'sources.jsonl'),sha256=sha(target/'sources.jsonl')))
 (target/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8')
 print(json.dumps({k:summary[k]for k in('corpus','frames','wireshark_esp_frames','source_candidates','literal_selection_counts')}),flush=True)
 return summary
if __name__=='__main__':
 summaries=[run(name,cap)for name,cap in CAPTURES.items()]
 summary=dict(read_only=True,no_Go_toolchain=True,auditor_sha256=sha(Path(__file__)),reader=dict(path=str(AUDIT/'vxlan_endpoint_projection_audit.py'),sha256=sha(AUDIT/'vxlan_endpoint_projection_audit.py')),corpora=summaries)
 (OUT/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8')
