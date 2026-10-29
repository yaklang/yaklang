"""Isolated pinned TShark run on tiny authored OSPFv2 source controls; no keys."""
import argparse,hashlib,json,os,shutil,subprocess,time,xml.etree.ElementTree as ET
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for n in('source','tshark','out'):ap.add_argument('--'+n,required=True,type=Path)
 a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir();source=read(a.source/'summary.json');cap=Path(source['capture']);assert sha(cap)==source['capture_SHA256'];binding={str(p):sha(p)for p in(Path(__file__),a.tshark,a.source/'summary.json',cap)}
 cfg=a.out/'empty-config';cfg.mkdir();env=os.environ.copy();env.update(APPDATA=str(cfg),WIRESHARK_CONFIG_DIR=str(cfg));env.pop('SSLKEYLOGFILE',None);args=[str(a.tshark),'-n','-2','-r',str(cap),'-T','pdml'];start=time.monotonic()
 with(a.out/'reference.pdml').open('xb')as o,(a.out/'stderr').open('xb')as e:r=subprocess.run(args,stdout=o,stderr=e,env=env,creationflags=0x08000000,timeout=30)
 assert r.returncode==0;packets=ET.parse(a.out/'reference.pdml').getroot().findall('packet');numbers=[int(p.find("proto[@name='geninfo']/field[@name='num']").get('show'))for p in packets];assert numbers==list(range(1,source['authored_frames']+1));assert binding=={p:sha(p)for p in binding}
 report={'scope':__doc__,'arguments':args,'exit_code':r.returncode,'seconds':round(time.monotonic()-start,3),'authored_physical_frames':len(numbers),'all_source_frames_present':True,'inputs':binding,'inputs_unchanged':True,'export_SHA256':sha(a.out/'reference.pdml'),'export_bytes':(a.out/'reference.pdml').stat().st_size,'stderr_SHA256':sha(a.out/'stderr'),'fresh_isolated_configuration':True,'authentication_keys_supplied':False,'no_native_output_read':True,'real_traffic_claim':False}
 write(a.out/'producer.json',report);print(json.dumps({'producer':str(a.out/'producer.json'),'SHA256':sha(a.out/'producer.json'),'frames':len(numbers),'exit_code':r.returncode}))
if __name__=='__main__':main()
