"""Pinned isolated TShark for literal source-selected OSPFv2 physical frames.

Selection is by the independent original frame roster, not the reference's
protocol label. No auth keys or candidate/native output are supplied or read.
"""
import argparse,hashlib,json,os,shutil,subprocess,time,traceback,xml.etree.ElementTree as ET
from pathlib import Path

def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb')as f:
        for b in iter(lambda:f.read(1024*1024),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('grid','tshark','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir();assert sha(a.grid/'summary.json')=='89e4217477a176740ac9150d4f2cf2222ab31fa1e5a5b95dc8b89d536198c107';grid=read(a.grid/'summary.json');binary=sha(a.tshark);results=[]
    try:
        for c in grid['captures']:
            assert shutil.disk_usage('C:/').free>20*2**30;capture=Path(c['capture']);assert sha(capture)==c['capture_SHA256']and sha(c['source_grid'])==c['source_grid_SHA256'];rows=read(c['source_grid']);roster=[r['frame']for r in rows];assert len(roster)==len(set(roster))==278
            d=a.out/('input-%02d'%c['input']);d.mkdir();cfg=d/'empty-config';cfg.mkdir();env=os.environ.copy();env.update(APPDATA=str(cfg),WIRESHARK_CONFIG_DIR=str(cfg));env.pop('SSLKEYLOGFILE',None)
            expression='frame.number in {'+','.join(map(str,roster))+'}';args=[str(a.tshark),'-n','-2','-r',str(capture),'-Y',expression,'-T','pdml'];start=time.monotonic()
            with(d/'reference.pdml').open('xb')as out,(d/'stderr').open('xb')as err:p=subprocess.run(args,stdout=out,stderr=err,env=env,creationflags=0x08000000,timeout=180)
            assert p.returncode==0;packets=ET.parse(d/'reference.pdml').getroot().findall('packet');numbers=[]
            for packet in packets:
                f=[f for f in packet.iter('field')if f.get('name')=='frame.number'];assert len(f)==1;numbers.append(int(f[0].get('show')))
            assert numbers==roster and sha(capture)==c['capture_SHA256']and sha(a.tshark)==binary
            receipt=dict(input=c['input'],capture=str(capture),capture_SHA256=c['capture_SHA256'],source_grid_SHA256=c['source_grid_SHA256'],source_grid_summary_SHA256=sha(a.grid/'summary.json'),TShark_SHA256=binary,arguments=args,exit_code=p.returncode,seconds=round(time.monotonic()-start,3),selected_source_frames=len(roster),literal_original_frame_numbers_exact=True,export_SHA256=sha(d/'reference.pdml'),export_bytes=(d/'reference.pdml').stat().st_size,stderr_SHA256=sha(d/'stderr'),fresh_isolated_configuration=True,authentication_keys_supplied=False,candidate_or_native_output_read=False,driver_SHA256=sha(__file__))
            write(d/'producer.json',receipt);results.append(receipt);print(json.dumps(dict(input=c['input'],frames=len(roster),exit_code=0)),flush=True)
        write(a.out/'summary.json',dict(scope=__doc__,source_grid_summary_SHA256=sha(a.grid/'summary.json'),results=results,source_frames=556,all_selected_original_frames_present=True,authentication_or_native_correctness_claim=False))
        print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),frames=556)),flush=True)
    except Exception:
        write(a.out/'controller-failure.json',dict(error=traceback.format_exc()));raise
if __name__=='__main__':main()
