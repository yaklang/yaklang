"""Per-frame comparison: compare.py CAPTURE_DIR TSHARK YAK EVENTS_HELPER."""
import collections,csv,io,json,subprocess,sys
from pathlib import Path
csv.field_size_limit(8000000)
cap=Path(sys.argv[1]);tshark,yak,audit=map(Path,sys.argv[2:5])
m=json.loads((cap/'manifest.json').read_text(encoding='utf8'));out=cap/'comparison';out.mkdir(exist_ok=True)
def run(args,name):
    p=subprocess.run([str(a) for a in args],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120)
    (out/name).write_bytes(p.stdout);(out/(name+'.stderr')).write_bytes(p.stderr)
    if p.returncode:raise RuntimeError((name,p.returncode,p.stderr.decode(errors='replace')[:2000]))
    return p.stdout.decode('utf-8-sig')
available={x.split('\t')[2] for x in subprocess.check_output([str(tshark),'-G','fields']).decode(errors='replace').splitlines() if x.startswith('F\t') and len(x.split('\t'))>2}
fields=['frame.number','frame.cap_len','frame.len','frame.protocols','_ws.col.Protocol','_ws.col.Info','tcp.stream','tcp.srcport','tcp.dstport','tcp.len','tcp.payload','udp.srcport','udp.dstport','tls.record.content_type','tls.record.version','tls.record.length','tls.handshake.type','ftp.request.command','ftp.request.arg','ftp.response.code','smtp.req.command','smtp.response.code','smtp.message','pop.request.command','pop.response','imap.request','imap.response','mqtt.msgtype','mqtt.msgid','mqtt.qos','mqtt.topic','mqtt.msg','ldap.messageID','pgsql.type','pgsql.query','mysql.command','mysql.query','smb2.cmd','smb2.msg_id','smb2.sesid','snmp.request_id','ntp.org','ntp.xmt','dhcp.id','websocket.opcode','websocket.payload','_ws.expert.message']
fields=[x for x in fields if x in available or x.startswith('_ws.col.')]
args=[tshark,'-2','-n','-r',cap/'live.pcap','-o','tls.keylog_file:'+str(cap/'session.keys'),'-o','tcp.reassemble_out_of_order:TRUE']
profiles={'ftp':'ftp','ftps':'ftp','smtp':'smtp','smb2':'nbss','imap':'imap','pop3':'pop','mqtt':'mqtt','ldap':'ldap','postgresql':'pgsql','mysql':'mysql','ssh':'ssh','websocket':'http','ntp':'ntp','snmp':'snmp','dhcp':'dhcp'}
for name,p in m['Ports'].items():
    if name not in profiles:continue
    carrier='udp' if name in ('ntp','snmp','dhcp') else 'tcp'
    args+=['-d',f'{carrier}.port=={p},{profiles[name]}']
args+=['-T','fields','-E','header=y','-E','occurrence=a']
for f in fields:args+=['-e',f]
ws=list(csv.DictReader(io.StringIO(run(args,'tshark.tsv')),delimiter='\t',quoting=csv.QUOTE_NONE))
ys=[json.loads(x) for x in run([yak,'shark','-r',cap/'live.pcap','--json','--tls-keylog',cap/'session.keys'],'yak.jsonl').splitlines()]
es=[json.loads(x) for x in run([audit,'replay',cap/'live.pcap',cap/'session.keys'],'events.jsonl').splitlines()]
services={}
for name,p in m['Ports'].items():
    if '_data_' in name:continue
    frames={int(w['frame.number']) for w in ws if str(p) in (w.get('tcp.srcport'),w.get('tcp.dstport'),w.get('udp.srcport'),w.get('udp.dstport'))}
    selected=[e for e in es if any(r['Number'] in frames for r in e['source_bytes'].get('PacketRefs') or [])]
    services[name]={'frames':len(frames),'wireshark_rows':dict(collections.Counter(w['_ws.col.protocol'] for w in ws if int(w['frame.number']) in frames)),'yak_rows':dict(collections.Counter(y['protocol'] for y in ys if y['number'] in frames)),'events':dict(collections.Counter(e['protocol']+'/'+e['status'] for e in selected))}
errors=[dict(protocol=e['protocol'],id=e['id'],flow=e['flow'],status=e['status'],summary=e['summary'],error=e.get('error'),frames=[r['Number'] for r in e['source_bytes'].get('PacketRefs') or []]) for e in es if e['status'] not in ('decoded','deferred','unrecognized')]
byframe=collections.defaultdict(list)
for e in es:
    for ref in e['source_bytes'].get('PacketRefs') or []:byframe[ref['Number']].append(e)
with (out/'frame-comparison.csv').open('w',newline='',encoding='utf-8-sig') as f:
    w=csv.writer(f);w.writerow(['frame','ws_protocol','yak_protocol','yak_connection','ws_info','yak_info','events'])
    for a,b in zip(ws,ys):w.writerow([a['frame.number'],a['_ws.col.protocol'],b['protocol'],' > '.join(b.get('connection_stack',[])),a['_ws.col.info'],b['info'],'; '.join(e['protocol']+'/'+e['status'] for e in byframe[b['number']])])
summary=dict(frames=[len(ws),len(ys)],events=dict(collections.Counter(e['protocol']+'/'+e['status'] for e in es)),services=services,errors=errors)
(out/'summary.json').write_text(json.dumps(summary,indent=2),encoding='utf8')
print(json.dumps(summary,indent=2))
assert len(ws)==len(ys)
