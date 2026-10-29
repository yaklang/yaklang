"""Replay source-frozen official9+authored24 using exact committed SDK/CLI builds."""
from pathlib import Path
import argparse, hashlib, importlib.util, json, os, subprocess, time

root = Path(__file__).parent
repo = Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
ap = argparse.ArgumentParser(description=__doc__)
ap.add_argument('--bins', required=True, type=Path)
ap.add_argument('--source', required=True)
ap.add_argument('--out', required=True, type=Path)
a = ap.parse_args()
a.out.mkdir(exist_ok=False)
def sha(p): return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p): return json.loads(Path(p).read_text('utf8'))
def put(p,x):
    with p.open('x',encoding='utf8') as f: json.dump(x,f,indent=2); f.write('\n')
def source_guard():
    assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,encoding='utf8').strip() == a.source
    rows = subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],cwd=repo,encoding='utf8').splitlines()
    assert not [r for r in rows if r[3:].endswith(('.go','.yaml','.yml','.zst','.mod','.sum'))]

plan = root/'RSVP-native-CLI-source-plan46-01'
checker = root/'rsvp_native_cli_checker46.py'
assert sha(plan/'manifest.json') == '1f291b0e4bf9ad0db09b12d1cd952f3fe69be41b40d9261598d96a63bdb1305c'
assert sha(checker) == '24124ac22523e73b72fa43191b8bf35505312b6f16cd76b36bf7bb03b6c3ae7e'
spec = importlib.util.spec_from_file_location('frozen_RSVP_native_source',checker)
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
m.load(plan)
cmdplan = read(plan/'runtime-command-plan.json')
builds = {}
for kind in ('events','yak'):
    binary = a.bins/(kind+'.exe'); receipt = a.bins/(kind+'-receipt.json'); b = read(receipt)
    assert b['source'] == a.source and b['exit_code'] == 0 and sha(binary) == b['binary_SHA256']
    builds[kind] = (binary,receipt,b)
proofs = []
def run(kind,args,path,cap=None):
    source_guard(); binary,receipt,b = builds[kind]
    frozen = {str(p):sha(p) for p in (binary,receipt,checker,plan/'manifest.json',Path(__file__))}
    if cap: frozen[str(cap)] = sha(cap)
    start = time.monotonic(); arguments = [str(binary),*args]
    with path.open('xb') as output, path.with_suffix('.stderr').open('xb') as err:
        p = subprocess.run(arguments,stdout=output,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
    source_guard(); assert frozen == {p:sha(p) for p in frozen}; m.load(plan)
    r = dict(source=a.source,kind=kind,arguments=arguments,exit_code=p.returncode,seconds=time.monotonic()-start,inputs=frozen,export_SHA256=sha(path),stderr_SHA256=sha(path.with_suffix('.stderr')))
    put(path.with_name(kind+'-producer.json'),r); proofs.append(r)
    assert p.returncode == 0

descriptors = a.out/'field-descriptors.json'
run('yak',cmdplan['field_descriptors_args'],descriptors)
contexts = []
for context in cmdplan['commands']:
    folder = a.out/context['context']; folder.mkdir()
    cap = Path(context['events_args'][1])
    run('events',context['events_args'],folder/'events.jsonl',cap)
    run('yak',context['CLI_args'],folder/'yak.jsonl',cap)
    audit = folder/'independent-source-audit'
    m.verify_exports(plan,context['context'],folder/'events.jsonl',folder/'yak.jsonl',audit,descriptors)
    s = read(audit/'summary.json')
    contexts.append(dict(context=context['context'],counts=s['counts'],summary_SHA256=sha(audit/'summary.json')))
put(a.out/'summary.json',dict(scope=__doc__,source=a.source,plan_manifest_SHA256=sha(plan/'manifest.json'),controller_SHA256=sha(__file__),runs=proofs,contexts=contexts,source_cases=33,all_frozen_inputs_unchanged=True,passed=True))
print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),contexts=contexts)))
