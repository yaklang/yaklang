"""Frozen selected-original-frame Wireshark export, with pre/post source hashes."""
from pathlib import Path
import hashlib,json,os,subprocess,time,xml.etree.ElementTree as ET
phase=Path(r'C:\tmp\yak-shark-ESP-SSH-UI-20261028')
root=phase/'ESP-OSPFv3-source-readonly30-01'
out=root/'wireshark03';out.mkdir(exist_ok=False)
def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb') as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
facts=root/'source02/source-facts.json';s=json.loads(facts.read_text(encoding='utf8'))
capture=Path(s['capture']);binary=Path(r'C:\tmp\yak-shark-protocol-audit-20261024\wireshark-portable\App\Wireshark\tshark.exe')
config=out/'empty-config';config.mkdir()
v3facts=root/'source03/all-417-OSPFv3-source-facts.json'
nattfacts=root/'source03/all-34-NATT-source-facts.json'
numbers=sorted(set(r['frame'] for r in s['selected'])|set(r['frame'] for r in json.loads(v3facts.read_text()))|set(r['frame'] for r in json.loads(nattfacts.read_text())))
args=[str(binary),'-n','-o','tcp.reassemble_out_of_order:TRUE','-o','ip.check_checksum:TRUE',
 '-o','udp.check_checksum:TRUE','-r',str(capture),'-Y',' || '.join('frame.number=='+str(n) for n in numbers),'-T','pdml']
before={str(p):sha(p) for p in [Path(__file__),facts,v3facts,nattfacts,capture,binary]}
env=os.environ.copy();env['WIRESHARK_CONFIG_DIR']=str(config)
start=time.time()
with (out/'selected.pdml').open('xb') as stdout,(out/'stderr.log').open('xb') as stderr:
 result=subprocess.run(args,stdout=stdout,stderr=stderr,env=env)
after={p:sha(p) for p in before}
assert before==after and result.returncode==0
rows=[]
for packet in ET.parse(out/'selected.pdml').getroot().findall('packet'):
 number=int(packet.find(".//field[@name='frame.number']").get('show'))
 row=dict(frame=number,protocols=[],fields=[])
 for proto in packet.findall('proto'):
  if proto.get('name') in ['eth','vlan','fpp','ip','ipv6','ah','udp','udpencap','esp','ospf']:
   row['protocols'].append(dict(proto.attrib))
  for field in proto.iter('field'):
   name=field.get('name','')
   if name.startswith(('ospf.','ah.','esp.','udpencap.')) or name in ['_ws.expert.message','_ws.expert.group','_ws.expert.severity','ip.len','ip.proto','ip.checksum.status','ipv6.plen','ipv6.nxt','udp.srcport','udp.dstport','udp.length','udp.checksum.status']:
    row['fields'].append(dict(field.attrib))
 rows.append(row)
assert sorted(r['frame'] for r in rows)==sorted(numbers)
(out/'selected-fields.json').write_text(json.dumps(rows,indent=2)+'\n',encoding='utf8')
receipt=dict(scope=__doc__,arguments=args,environment_changes={'WIRESHARK_CONFIG_DIR':str(config)},
 exit_code=result.returncode,seconds=time.time()-start,source_before=before,source_after=after,
 sources_byte_equal=True,selected_original_frames=numbers,outputs={p.name:sha(p) for p in [out/'selected.pdml',out/'stderr.log',out/'selected-fields.json']},
 keylog_supplied=False,reference_ESP_authentication_or_decryption_not_enabled=True)
(out/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(receipt=str(out/'receipt.json'),SHA256=sha(out/'receipt.json'),frames=len(rows))))
