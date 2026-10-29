"""37-input whole-CLI verifier bound to the pre-output complete source plan."""
from pathlib import Path
import argparse,hashlib,json,subprocess,sys,traceback
FROZEN01=Path(r'C:\tmp\yak-shark-ESP-SSH-UI-20261028\ESP-NATT-whole-plan-before-01')
sys.path.insert(0,str(FROZEN01))
from natt_expected import canonical
ROOT=Path(__file__).resolve().parent
EXPECTED_PLAN_SHA256='5bbf37d579bf461d4a382863edac7a340a7831110809191633d9b4e8aae316df'
EXPECTED_NATIVE_PLAN_SHA256='4b066e4419e40f49f8ba14ddf23adff179e9cdc123571b9b14041925de627ce7'
EXPECTED_WHOLE_VERIFIER_SHA256='9f25dfaa8bab22ac8074827ee2f57ed8158c872e2207a341152d704ec5240353'
MODULES=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang\common\yak\cmd\yakcmds\shark-cli\testdata\live-audit')
def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,indent=2,ensure_ascii=False)+'\n',encoding='utf8')
def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ['after','out']:p.add_argument('--'+name,type=Path,required=True)
    p.add_argument('--after-source',required=True);a=p.parse_args();a.out.mkdir(exist_ok=False)
    try:
        cp=ROOT/'frozen02/complete-CLI-plan.json';native=FROZEN01/'frozen01/plan.json';verifier=FROZEN01/'verify_whole.py'
        assert sha(cp)==EXPECTED_PLAN_SHA256 and sha(native)==EXPECTED_NATIVE_PLAN_SHA256 and sha(verifier)==EXPECTED_WHOLE_VERIFIER_SHA256
        plan=read(cp);bindings=plan['source_contract_bindings']
        contract=cp.parent/bindings['parent_contract_file'];assert sha(contract)==bindings['parent_contract_SHA256']
        assert sha(ROOT/'prepare_CLI.py')==bindings['constructor_SHA256']
        assert sha(FROZEN01/'manifest01.json')==bindings['frozen01_manifest_SHA256']
        paths=[cp,native,verifier,contract,ROOT/'prepare_CLI.py',FROZEN01/'manifest01.json',Path(__file__)]
        for source in bindings['existing_production_source_snapshots'].values():
            path=cp.parent/source['source_file'];assert sha(path)==source['SHA256'];paths.append(path)
        before={str(path):sha(path) for path in paths}
        args=[sys.executable,'-B','-X','utf8',str(verifier),'--kind','CLI','--plan',str(native),
            '--complete-cli-plan',str(cp),'--after',str(a.after),'--after-source',a.after_source,
            '--audit-modules',str(MODULES),'--out',str(a.out/'whole37')]
        with (a.out/'whole37.stdout').open('xb') as stdout,(a.out/'whole37.stderr').open('xb') as stderr:
            result=subprocess.run(args,stdout=stdout,stderr=stderr)
        assert result.returncode==0,'Whole37 verification failed; original failure and stderr retained'
        digest=hashlib.sha256();count=0
        with (a.after/'preservation/weber/yak.jsonl').open('rb') as f:
            for raw in f:digest.update(canonical(json.loads(raw))+b'\n');count+=1
        assert count==plan['expected_Weber_whole_rows'] and digest.hexdigest()==plan['expected_Weber_whole_export_canonical_SHA256']
        after={p:sha(p) for p in before};assert before==after
        value=dict(scope=__doc__,after_source=a.after_source,source_plan_SHA256=sha(cp),native_plan_SHA256=sha(native),
            whole37_proof_SHA256=sha(a.out/'whole37/summary.json'),expected_Weber_canonical_SHA256=digest.hexdigest(),
            whole_Weber_rows=count,cases=37,exact_source_corrections=7,unplanned_differences=0,
            source_before=before,source_after=after,sources_byte_equal=True,ID_mapping=False,
            auditor_SHA256=sha(__file__))
        write(a.out/'summary.json',value)
        print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),cases=37,source_corrections=7,unplanned_differences=0)))
    except Exception:
        write(a.out/'failure.json',dict(failure=traceback.format_exc(),no_success_certificate=True));raise
if __name__=='__main__':main()
