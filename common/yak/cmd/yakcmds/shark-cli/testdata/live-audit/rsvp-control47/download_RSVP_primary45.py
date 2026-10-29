"""Obtain public primary RSVP sources and the official Wiki capture externally."""
from pathlib import Path
import hashlib,json,sys,time,urllib.request,urllib.error
root=Path(__file__).parent;out=root/sys.argv[1];out.mkdir(exist_ok=False)
urls=[('rsvp-PATH-RESV.pcap','https://wiki.wireshark.org/uploads/__moin_import__/attachments/SampleCaptures/rsvp-PATH-RESV.pcap')]+[(f'rfc{n}.txt',f'https://www.rfc-editor.org/rfc/rfc{n}.txt')for n in(2205,2961,2747)]
records=[]
for name,url in urls:
 start=time.monotonic()
 try:
  with urllib.request.urlopen(urllib.request.Request(url,headers={'User-Agent':'Yak-Shark-source-audit'}),timeout=35)as r:
   data=r.read(16*2**20+1);assert len(data)<=16*2**20
   row=dict(url=url,final_url=r.url,status=r.status,bytes=len(data),content_type=r.headers.get('Content-Type'),seconds=time.monotonic()-start,SHA256=hashlib.sha256(data).hexdigest())
  if name.endswith('.pcap'):assert data[:4]in(bytes.fromhex('d4c3b2a1'),bytes.fromhex('a1b2c3d4'),bytes.fromhex('4d3cb2a1'),bytes.fromhex('0a0d0d0a'))
  (out/name).write_bytes(data);row['saved']=name
 except Exception as e:row=dict(url=url,error_type=type(e).__name__,error=str(e),seconds=time.monotonic()-start,saved=False)
 records.append(row);print(json.dumps(row),flush=True)
(out/'download-manifest.json').write_text(json.dumps(dict(controller_SHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),public_primary_sources=True,capture_not_repository=True,records=records),indent=2)+'\n',encoding='utf8')
