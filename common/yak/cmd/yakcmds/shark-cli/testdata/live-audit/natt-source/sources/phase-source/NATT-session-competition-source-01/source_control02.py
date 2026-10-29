"""Independent IPv4/v6 STUN/CoAP/TFTP/RADIUS controls for the final UDP fallback.

Only Python constructs source wire and existing TShark interprets it. No Go or
native replay occurs here;693 fallback selection is a source-level prediction.
"""
from pathlib import Path
import csv,hashlib,ipaddress,json,os,struct,subprocess
root=Path(__file__).resolve().parent;out=root/'actual02';out.mkdir(exist_ok=False)
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
source_commit='69346fc94722c5b530e0d59b14a9783b6de3f530'
tshark=Path(r'C:\tmp\yak-shark-protocol-audit-20261024\wireshark-portable\App\Wireshark\tshark.exe')
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def cs(b):
 if len(b)%2:b+=b'\0'
 n=sum(struct.unpack('!%dH'%(len(b)//2),b))
 while n>>16:n=(n&65535)+(n>>16)
 return (~n)&65535
def frame(wire,port,version):
 src=ipaddress.ip_address('192.0.2.1' if version==4 else '2001:db8::1').packed
 dst=ipaddress.ip_address('192.0.2.2' if version==4 else '2001:db8::2').packed
 udp=struct.pack('!HHHH',4500,port,len(wire)+8,0)+wire
 pseudo=src+dst+(b'\0\x11'+len(udp).to_bytes(2,'big') if version==4 else len(udp).to_bytes(4,'big')+b'\0\0\0\x11')
 check=cs(pseudo+udp);udp=udp[:6]+(check or 65535).to_bytes(2,'big')+udp[8:]
 assert cs(pseudo+udp)==0
 if version==4:
  ip=struct.pack('!BBHHHBBH4s4s',0x45,0,len(udp)+20,1,0x4000,64,17,0,src,dst)
  ip=ip[:10]+cs(ip).to_bytes(2,'big')+ip[12:];assert cs(ip)==0;typ=0x800
 else:ip=struct.pack('!IHBB16s16s',6<<28,len(udp),17,64,src,dst);typ=0x86dd
 return bytes.fromhex('020000000002020000000001')+typ.to_bytes(2,'big')+ip+udp
stun=bytes.fromhex('000100002112a4420102030405060708090a0b0c')
coap=bytes.fromhex('40010001')+b'\xb7sensors\x04temp'
tftp=b'\0\x01example.txt\0octet\0'
radius=bytes([1,7])+bytes.fromhex('001a')+bytes(range(1,17))+b'\x01\x06test'
samples=[('STUN',3478,stun),('CoAP',5683,coap),('TFTP',69,tftp),('RADIUS',1812,radius)]
source_cases=[]
for version in [4,6]:
 for name,port,wire in samples:
  assert 12<=len(wire)<=65527 and int.from_bytes(wire[:4],'big')!=0
  source_cases.append(dict(version=version,protocol=name,destination_port=port,wire=wire))
pcap=out/'controls.pcap'
with pcap.open('xb') as f:
 f.write(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
 for number,case in enumerate(source_cases,1):
  wire=frame(case['wire'],case['destination_port'],case['version'])
  f.write(struct.pack('<IIII',number,0,len(wire),len(wire))+wire)
config=out/'empty-config';config.mkdir()
args=[str(tshark),'-n','-o','ip.check_checksum:TRUE','-o','udp.check_checksum:TRUE','-r',str(pcap),'-T','fields',
 '-E','separator=,','-E','quote=d','-e','frame.number','-e','_ws.col.Protocol','-e','frame.protocols',
 '-e','udp.srcport','-e','udp.dstport','-e','udp.length','-e','_ws.col.Info','-e','_ws.expert.message']
with (out/'wireshark.csv').open('xb') as stdout,(out/'stderr.log').open('xb') as stderr:
 result=subprocess.run(args,stdout=stdout,stderr=stderr,env=dict(os.environ,WIRESHARK_CONFIG_DIR=str(config)))
assert result.returncode==0
rows=list(csv.reader((out/'wireshark.csv').open(encoding='utf8',newline='')));assert len(rows)==8
details=[]
for number,(case,row) in enumerate(zip(source_cases,rows),1):
 default_label='ESP' if case['protocol']=='CoAP' else case['protocol']
 assert int(row[0])==number and row[1]==default_label,(case,row)
 assert row[3]=='4500' and int(row[4])==case['destination_port']
 details.append(dict(frame=number,IPv=case['version'],source_wire_grammar=case['protocol'],expected_automatic_Wireshark_label=default_label,UDP_source=4500,UDP_destination=case['destination_port'],
  wire_hex=case['wire'].hex(),wire_length=len(case['wire']),wire_SHA256=hashlib.sha256(case['wire']).hexdigest(),
  UDP_payload_range=[42 if case['version']==4 else 62,(42 if case['version']==4 else 62)+len(case['wire'])],
  Wireshark=row,pre_fix_693_SPI_length_port_condition=True,source_level_not_Go_execution=True))
# Default CoAP competition remains explicitly ESP in the literal reference.
# Independently selecting the known source layout proves its complete CoAP
# fields, without presenting that diagnostic run as automatic-label parity.
args_coap=[str(tshark),'-n','-d','udp.port==4500,coap','-r',str(pcap),'-Y','frame.number==2 || frame.number==6','-T','pdml']
with (out/'CoAP-explicit.pdml').open('xb') as stdout,(out/'CoAP-explicit.stderr').open('xb') as stderr:
 coap_result=subprocess.run(args_coap,stdout=stdout,stderr=stderr,env=dict(os.environ,WIRESHARK_CONFIG_DIR=str(config)))
assert coap_result.returncode==0
import xml.etree.ElementTree as ET
coap_observations=[]
for packet in ET.parse(out/'CoAP-explicit.pdml').getroot().findall('packet'):
 number=int(packet.find(".//field[@name='frame.number']").get('show'))
 coap_proto=packet.find("proto[@name='coap']");assert coap_proto is not None
 fields=[dict(f.attrib) for f in coap_proto.iter('field') if f.get('name','').startswith('coap.')]
 paths=[f['show'] for f in fields if f['name']=='coap.opt.uri_path'];assert paths==['sensors','temp']
 assert any(f['name']=='coap.version' and f.get('show')=='1' for f in fields)
 assert any(f['name']=='coap.code' and f.get('show')=='1' for f in fields)
 coap_observations.append(dict(frame=number,explicit_known_layout_only=True,fields=fields))
assert {r['frame'] for r in coap_observations}=={2,6}
snapshots={}
for path in ['common/pcapx/pcaputil/protocol_registry.go','common/pcapx/pcaputil/bin_parser_framing.go','common/pcapx/pcaputil/bin_parser_datagram.go']:
 p=subprocess.run(['git','show',source_commit+':'+path],cwd=repo,stdout=subprocess.PIPE,stderr=subprocess.PIPE);assert p.returncode==0
 target=out/(Path(path).name+'.693-source.txt');target.write_bytes(p.stdout);snapshots[path]=dict(file=target.name,SHA256=sha(target))
value=dict(scope=__doc__,driver_SHA256=sha(__file__),source_commit=source_commit,immutable_source_snapshots=snapshots,
 capture=str(pcap),capture_SHA256=sha(pcap),TShark_SHA256=sha(tshark),TShark_arguments=args,TShark_exit=0,
 TShark_csv_SHA256=sha(out/'wireshark.csv'),cases=details,no_Go_or_native_execution=True,
 CoAP_automatic_reference_is_ESP_not_relabelled=True,CoAP_explicit_arguments=args_coap,CoAP_explicit_exit=coap_result.returncode,
 CoAP_explicit_PDML_SHA256=sha(out/'CoAP-explicit.pdml'),CoAP_explicit_fields=coap_observations,
 conclusion='NativeDatagram fallback precedes SessionDatagram STUN/TFTP/CoAP/RADIUS. Default Wireshark agrees on STUN/TFTP/RADIUS;CoAP conflicts with UDP4500 registration and is only independently decoded in a separate explicit diagnostic. Preserve all literal labels; final native fallback should preserve existing stronger source grammar and never claim SA/replay. Public7 expected objects unchanged.')
(out/'summary.json').write_text(json.dumps(value,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),frames=8,protocols=[row[1] for row in rows])))
