"""Execute the unchanged official9 against closed pre-RSVP SDK and CLI binaries."""
from pathlib import Path
import collections, hashlib, json, os, subprocess, time
root = Path(__file__).parent
out = root/'RSVP-official-before-df30-46-01'
out.mkdir(exist_ok=False)
def sha(p): return hashlib.sha256(Path(p).read_bytes()).hexdigest()
cap = root/'RSVP-primary-download45-01/rsvp-PATH-RESV.pcap'
assert sha(cap) == 'aa7d8448cd5eaf17112e67adf3e801cb45119e62c63c75f132ad596edfc10361'
results = []
for kind in ('events','yak'):
    binary = root/f'bins-45/{kind}.exe'
    receipt = binary.with_name(kind+'-receipt.json')
    build = json.loads(receipt.read_text('utf8'))
    assert build['source'] == 'df30fc4bd909d6489844f8e0d1cd0ab03826f35c' and build['exit_code'] == 0 and sha(binary) == build['binary_SHA256']
    args = [str(binary),'replay',str(cap),'-','--ignore-embedded-secrets'] if kind=='events' else [str(binary),'shark','-r',str(cap),'--json','--ignore-embedded-secrets']
    frozen = {str(p):sha(p) for p in (cap,binary,receipt,Path(__file__))}
    output, err = out/(kind+'.jsonl'), out/(kind+'.stderr')
    start = time.monotonic()
    with output.open('xb') as f, err.open('xb') as e:
        run = subprocess.run(args,stdout=f,stderr=e,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
    assert run.returncode == 0 and frozen == {p:sha(p) for p in frozen}
    rows = [json.loads(line) for line in output.read_bytes().splitlines() if line.strip()]
    if kind == 'yak': assert len(rows) == 9
    results.append(dict(kind=kind,source=build['source'],arguments=args,exit_code=run.returncode,seconds=time.monotonic()-start,rows=len(rows),protocol_counts=dict(collections.Counter(x.get('protocol','') for x in rows)),status_counts=dict(collections.Counter(x.get('status','') for x in rows)),inputs=frozen,export_SHA256=sha(output),stderr_SHA256=sha(err)))
proof = dict(scope=__doc__,source=build['source'],original_capture_SHA256=sha(cap),physical_frames=9,runs=results,controller_SHA256=sha(__file__))
(out/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(proof))
