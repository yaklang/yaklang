"""Record the authored zero/nonzero Fletcher pair with a pinned native binary."""
from pathlib import Path
import argparse, hashlib, json, os, subprocess

phase=Path(__file__).parent
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
ap=argparse.ArgumentParser(description=__doc__)
for n in ('out','bins'):ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--expected-source',required=True)
a=ap.parse_args();a.out.mkdir()
cap=phase/'OSPF-Fletcher-zero-field-source39-01/authored-zero-Fletcher-pair.pcap'
source=phase/'OSPF-Fletcher-zero-field-source39-01/summary.json'
assert sha(source)=='e27093563920e23e67dc0ccf223ab0cbcb775a40ba21feacaa657577c3c66403'
binary=a.bins/'events.exe';receipt=a.bins/'events-receipt.json';build=json.loads(receipt.read_text())
assert build['source']==a.expected_source and build['exit_code']==0 and sha(binary)==build['binary_SHA256']
before=sha(cap);args=[str(binary),'replay',str(cap),'-','--ignore-embedded-secrets']
with(a.out/'events.jsonl').open('xb')as log,(a.out/'stderr.log').open('xb')as err:
    r=subprocess.run(args,stdout=log,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000,timeout=60)
assert sha(cap)==before and sha(binary)==build['binary_SHA256']
observations=[]
for l in(a.out/'events.jsonl').read_text().splitlines():
    e=json.loads(l);s=e['session'];row=s['LSAs'][0]
    observations.append(dict(PDU=e['id'],protocol=e['protocol'],status=e['status'],LSA_checksum=row['Checksum'],LSA_checksum_verified=row['Checksum Verified'],packet_checksum_verified=s['Checksum Verified']))
f=a.out/'producer.json'
f.write_text(json.dumps(dict(source=a.expected_source,arguments=args,build_SHA256=sha(receipt),binary_SHA256=sha(binary),capture_SHA256=before,source_summary_SHA256=sha(source),export_SHA256=sha(a.out/'events.jsonl'),exit_code=r.returncode,observed=observations,controller_SHA256=sha(__file__),correctness_requires_separate_comparison=True),indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(proof=str(f),SHA256=sha(f),observed=observations)))
assert r.returncode==0 and len(observations)==2
