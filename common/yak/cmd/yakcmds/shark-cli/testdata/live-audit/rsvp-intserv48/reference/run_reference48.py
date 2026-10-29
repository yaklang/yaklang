"""Execute isolated pinned TShark after a source alias/span plan is frozen."""
from pathlib import Path
import argparse,hashlib,json,os,subprocess,time
TOOL=Path(r'C:\tmp\yak-shark-protocol-audit-20261024\wireshark-portable\App\Wireshark\tshark.exe')
TOOL_SHA='662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b'

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):
 with p.open('x',encoding='utf-8')as f:json.dump(x,f,ensure_ascii=False,indent=2);f.write('\n')
def run(plan,out):
 assert not out.exists();out.mkdir();m=read(plan/'manifest.json')
 for p,h in m['inputs'].items():assert sha(p)==h
 for n,h in m['files'].items():assert sha(plan/n)==h
 assert sha(TOOL)==TOOL_SHA
 items=read(plan/'source-to-reference-mapping.json');aliases=sorted({x['mapping']['alias']for x in items if x['mapping']['alias']})
 fields=['frame.number','frame.cap_len','frame.len','frame.time_epoch','frame.protocols','_ws.col.Protocol','_ws.expert.message']+aliases
 runs=[];inputs={str(Path(__file__)):sha(__file__),str(plan/'manifest.json'):sha(plan/'manifest.json'),str(TOOL):sha(TOOL)}
 for context,cap in read(plan/'captures.json').items():
  folder=out/context;folder.mkdir();profile=folder/'empty-profile';profile.mkdir();env=dict(os.environ,APPDATA=str(profile),WIRESHARK_CONFIG_DIR=str(profile));env.pop('SSLKEYLOGFILE',None)
  capture=Path(cap['capture']);assert sha(capture)==cap['capture_SHA256'];inputs[str(capture)]=sha(capture)
  for kind in('pdml','fields'):
   args=[str(TOOL),'-n','-2','-r',str(capture),'-T',kind]
   if kind=='fields':
    args+=['-E','header=y','-E','separator=/t','-E','quote=d','-E','occurrence=a']
    for field in fields:args+=['-e',field]
   export=folder/('reference.pdml'if kind=='pdml'else'reference-fields.tsv');err=folder/(kind+'.stderr');start=time.monotonic()
   with export.open('xb')as f,err.open('xb')as e:r=subprocess.run(args,env=env,stdout=f,stderr=e,timeout=60,creationflags=0x08000000)
   record={'context':context,'kind':kind,'arguments':args,'profile':str(profile),'exit_code':r.returncode,'seconds':time.monotonic()-start,'capture_SHA256':sha(capture),'tool_SHA256':sha(TOOL),'output':str(export),'output_SHA256':sha(export),'stderr':str(err),'stderr_SHA256':sha(err)};runs.append(record);print(json.dumps({'context':context,'kind':kind,'exit_code':r.returncode,'bytes':export.stat().st_size}),flush=True)
   assert r.returncode==0
 assert inputs=={p:sha(p)for p in inputs}
 put(out/'producer.json',{'plan_SHA256':sha(plan/'manifest.json'),'inputs':inputs,'runs':runs,'all_processes_closed':True,'source_facts_and_mapping_frozen_before_PDML':True,'candidate_Go_SDK_CLI_not_read':True,'capture_keys_or_full_exports_never_for_repository':True})
 print(json.dumps({'producer':str(out/'producer.json'),'SHA256':sha(out/'producer.json')}))
if __name__=='__main__':
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--plan',type=Path,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args();run(a.plan,a.out)