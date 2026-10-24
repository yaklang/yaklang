"""Replay all 30 declared families against Wireshark and Shark.

Usage: matrix.py VERIFIED_EXPORTED_CORPUS OUTPUT_DIR TSHARK YAK EVENTS
These archived fixtures supplement fresh live captures; they are never reported
as freshly generated production service traffic. Case inventory comes from the
versioned TOP30 test, preventing a second hand-maintained coverage list.
"""
import collections, concurrent.futures, csv, hashlib, io, json, re, struct, subprocess, sys
from pathlib import Path

corpus,out,tshark,yak,audit=map(Path,sys.argv[1:6]);out.mkdir(exist_ok=True)
test=Path(__file__).parents[3]/'top30_test.go'
source=test.read_text(encoding='utf8')
sessions='common/pcapx/pcaputil/testdata/protocol-sessions/'
captures='common/bin-parser/testdata/protocol-corpus/captures/'
def expr(text):
    text=text.strip()
    if text=='tls': return sessions+'first-batch-m1/tls-h2-bidi.pcap'
    if text=='tlsKey': return sessions+'first-batch-m1/tls-h2-bidi.keys'
    text=text.replace('top30Sessions + ',json.dumps(sessions)+' + ').replace('top30Corpus + ',json.dumps(captures)+' + ')
    return ''.join(json.loads(x) for x in re.findall(r'"[^"\n]*"',text)).replace('../../../../','common/')
cases=[]
for line in source[source.index('func top30Cases()'):source.index('func top30Replay')].splitlines():
    match=re.search(r'\{"(\d\d [^"]+)", (.*?), (.*?), (nil|\[\]string\{[^}]*\}),',line)
    if match:
        name,pcap,key,labels=match.groups()
        cases.append((name,expr(pcap),expr(key),re.findall(r'"([^"]+)"',labels)))
assert len(cases)==30
def command(args,file):
    p=subprocess.run([str(a) for a in args],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120)
    file.write_bytes(p.stdout);file.with_suffix(file.suffix+'.stderr').write_bytes(p.stderr)
    assert p.returncode==0,(str(file),p.returncode,p.stderr[:600])
    return p.stdout.decode('utf-8-sig')
def run(case):
    name,file,key,labels=case;directory=out/name[:2];directory.mkdir(exist_ok=True)
    if file=='synthetic:ethernet-arp':
        frame=bytes.fromhex('ffffffffffff02000000000108060001080006040001020000000001c0000201000000000000c0000202')
        raw=struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1)+struct.pack('<IIII',1,0,len(frame),len(frame))+frame
        pcap=directory/'arp.pcap';pcap.write_bytes(raw)
    else: pcap=corpus/file
    args=[tshark,'-2','-n','-r',pcap,'-o','tcp.reassemble_out_of_order:TRUE','-T','fields','-E','header=y','-e','frame.number','-e','frame.protocols','-e','_ws.col.Protocol','-e','_ws.col.Info','-e','_ws.expert.message']
    ysargs=[yak,'shark','-r',pcap,'--json'];esargs=[audit,'replay',pcap]
    if key:
        args+=['-o','tls.keylog_file:'+str(corpus/key)];ysargs+=['--tls-keylog',corpus/key];esargs+=[corpus/key]
    ws=list(csv.DictReader(io.StringIO(command(args,directory/'tshark.tsv')),delimiter='\t',quoting=csv.QUOTE_NONE))
    ys=[json.loads(x) for x in command(ysargs,directory/'yak.jsonl').splitlines()]
    es=[json.loads(x) for x in command(esargs,directory/'events.jsonl').splitlines()]
    assert len(ws)==len(ys),(name,len(ws),len(ys))
    observed=collections.Counter(x['protocol'] for x in ys)
    for label in labels:assert observed[label]>0,(name,label,observed)
    with (directory/'frame-comparison.csv').open('w',newline='',encoding='utf-8-sig') as f:
        w=csv.writer(f);w.writerow(['frame','wireshark','yak','connection_stack','wireshark_info','yak_info'])
        for a,b in zip(ws,ys):
            assert int(a['frame.number'])==b['number']
            w.writerow([b['number'],a['_ws.col.protocol'],b['protocol'],' > '.join(b.get('connection_stack',[])),a['_ws.col.info'],b['info']])
    return dict(name=name,source=file,sha256=hashlib.sha256(pcap.read_bytes()).hexdigest(),frames=len(ys),wireshark=dict(collections.Counter(x['_ws.col.protocol'] for x in ws)),yak=dict(observed),native=dict(collections.Counter(e['protocol']+'/'+e['status'] for e in es)))
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
    results=list(pool.map(run,cases))
(out/'matrix-summary.json').write_text(json.dumps(results,indent=2),encoding='utf8')
print(json.dumps([dict(name=x['name'],frames=x['frames'],native=x['native']) for x in results],indent=2))
