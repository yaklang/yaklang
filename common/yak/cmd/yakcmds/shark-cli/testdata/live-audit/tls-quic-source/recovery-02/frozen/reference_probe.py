"""Fresh isolated TShark references, before sparse-epoch candidate execution."""
import argparse,hashlib,json,os,shutil,subprocess,time
from pathlib import Path
def sha(path):return hashlib.sha256(Path(path).read_bytes()).hexdigest()
def save(path,v):
    with path.open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def main(a):
    assert shutil.disk_usage('C:/').free>20*1024**3
    a.out.mkdir(exist_ok=False);cfg=a.out/'empty-config';cfg.mkdir();env=os.environ.copy();env['WIRESHARK_CONFIG_DIR']=str(cfg);env['APPDATA']=str(cfg)
    args=[str(a.tshark),'-2','-n','-r',str(a.capture),'-o','tls.keylog_file:'+str(a.keylog),'-o','tcp.reassemble_out_of_order:TRUE']
    if a.quic_port:args+=['-d','udp.port=='+str(a.quic_port)+',quic','-T','pdml']
    else:
        args+=['-d','tcp.port==65178,tls','-d','tcp.port==65179,tls','-T','fields','-E','header=y','-E','occurrence=a']
        for f in ['frame.number','frame.time_epoch','frame.cap_len','frame.len','frame.protocols','_ws.col.Protocol','tcp.stream','tcp.seq_raw','tcp.len','tcp.analysis.lost_segment','tls.record.content_type','tls.record.length','tls.handshake.type','http.request.method','http.request.uri','http.response.code','_ws.expert.message']:args+=['-e',f]
    name='reference.pdml'if a.quic_port else'reference.tsv';start=time.time()
    with(a.out/name).open('xb')as stdout,(a.out/'stderr').open('xb')as stderr:p=subprocess.run(args,stdout=stdout,stderr=stderr,env=env,timeout=180)
    receipt=dict(arguments=args,environment={'WIRESHARK_CONFIG_DIR':str(cfg),'APPDATA':str(cfg)},tshark_SHA256=sha(a.tshark),capture_SHA256=sha(a.capture),keylog_SHA256=sha(a.keylog),exit_code=p.returncode,seconds=round(time.time()-start,3),export_SHA256=sha(a.out/name),stderr_SHA256=sha(a.out/'stderr'),explicit_owned_keys=True,fresh_isolated_configuration=True)
    save(a.out/'producer.json',receipt);assert p.returncode==0;print(json.dumps(dict(out=str(a.out),SHA256=receipt['export_SHA256'])))
if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    for n in ['tshark','capture','keylog','out']:p.add_argument('--'+n,type=Path,required=True)
    p.add_argument('--quic-port',type=int);main(p.parse_args())
