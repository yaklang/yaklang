"""Isolated reference-only pass over fresh owned Windows TLS/QUIC captures.

Ports are literal known-peer fixture declarations, not protocol evidence. No Yak
output is read and no native producer is executed. Every input is hashed before
and after; all capture/key/PDML/labels remain outside a Git repository.
"""
import argparse,csv,hashlib,json,os,shutil,subprocess,time,traceback
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('capture-root','tshark','out'):ap.add_argument('--'+n,type=Path,required=True)
    ap.add_argument('--kind',choices=['TLS','QUIC'],required=True);a=ap.parse_args();assert not a.out.exists()
    for p in(a.capture_root,a.out):assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    assert shutil.disk_usage('C:/').free>20*2**30
    capture=a.capture_root/'live.pcap';keylog=a.capture_root/'session.keys';manifest=a.capture_root/'manifest.json';peer=read(manifest)
    bound={str(p):sha(p)for p in[Path(__file__),a.tshark,capture,keylog,manifest]};a.out.mkdir();cfg=a.out/'empty-config';cfg.mkdir()
    env=os.environ.copy();env.update(WIRESHARK_CONFIG_DIR=str(cfg),APPDATA=str(cfg));env.pop('SSLKEYLOGFILE',None)
    base=[str(a.tshark),'-2','-n','-r',str(capture),'-o','tls.keylog_file:'+str(keylog),'-o','tcp.reassemble_out_of_order:TRUE']
    if a.kind=='TLS':
        ports=[p['server_port']for p in peer['profiles']]
        for port in ports:base+=['-d','tcp.port=='+str(port)+',tls']
    else:
        ports=[peer['server_port']];base+=['-d','udp.port=='+str(ports[0])+',quic']
    fields=['frame.number','frame.time_epoch','frame.cap_len','frame.len','frame.protocols','_ws.col.Protocol','tcp.stream','tcp.srcport','tcp.dstport','tcp.seq_raw','tcp.len','tcp.analysis.retransmission','tcp.analysis.out_of_order','tcp.analysis.lost_segment','tls.record.content_type','tls.record.length','tls.handshake.type','tls.handshake.ciphersuite','tls.resumed','http.request.method','http.request.uri','http.response.code','quic.connection.number','quic.packet_number','quic.key_phase','quic.decryption_failed','quic.dcid','quic.scid','http3.frame_type','_ws.expert.message']
    commands=[('reference.pdml',base+['-T','pdml']),('labels.tsv',base+['-T','fields','-E','header=y','-E','occurrence=a']+sum((['-e',f]for f in fields),[]))]
    executions=[]
    try:
        for name,args in commands:
            assert shutil.disk_usage('C:/').free>20*2**30;started=time.monotonic()
            with(a.out/name).open('xb')as out,(a.out/(name+'.stderr')).open('xb')as err:p=subprocess.run(args,stdout=out,stderr=err,env=env,creationflags=0x08000000,timeout=180)
            receipt=dict(arguments=args,exit_code=p.returncode,seconds=round(time.monotonic()-started,3),export_SHA256=sha(a.out/name),export_bytes=(a.out/name).stat().st_size,stderr_SHA256=sha(a.out/(name+'.stderr')))
            write(a.out/(name+'.receipt.json'),receipt);executions.append(receipt);assert p.returncode==0
        after={p:sha(p)for p in bound};assert bound==after
        with(a.out/'labels.tsv').open(encoding='utf8')as f:labels=list(csv.DictReader(f,delimiter='\t'))
        assert [int(r['frame.number'])for r in labels]==list(range(1,len(labels)+1))
        write(a.out/'producer.json',dict(scope=__doc__,kind=a.kind,capture_SHA256=sha(capture),keylog_SHA256=sha(keylog),peer_manifest_SHA256=sha(manifest),tshark_SHA256=sha(a.tshark),physical_frames=len(labels),owned_known_fixture_decode_as_ports=ports,executions=executions,inputs_before=bound,inputs_after=after,inputs_unchanged=True,environment={'WIRESHARK_CONFIG_DIR':str(cfg),'APPDATA':str(cfg),'SSLKEYLOGFILE':'unset'},secrets_or_plaintext_printed=False,native_or_Yak_output_read=False,free_bytes_after=shutil.disk_usage('C:/').free))
        print(json.dumps(dict(producer=str(a.out/'producer.json'),SHA256=sha(a.out/'producer.json'),kind=a.kind,physical_frames=len(labels),all_exit0=True)),flush=True)
    except Exception:
        write(a.out/'controller-failure.json',dict(error=traceback.format_exc()));raise
if __name__=='__main__':main()
