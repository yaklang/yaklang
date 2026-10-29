"""Change console navigation only; retain both RSVP source-field contracts byte-for-byte."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
old=root/'Windows-RSVP-source-plan46-01'
out=root/'Windows-RSVP-source-plan46-02'
out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
assert sha(old/'plan.json')=='e62048a0e2cee13bd17b0eef7e03403fe1f95427c81bb3a516cac6a4470c4f57'
before=json.loads((old/'cases.json').read_text('utf8'))
cases=json.loads((old/'cases.json').read_text('utf8'))
assert len(cases)==2
for c,o,down in zip(cases,before,(64,96)):
 c['field_down']=down
 assert {k:v for k,v in c.items()if k!='field_down'}=={k:v for k,v in o.items()if k!='field_down'}
(out/'cases.json').write_text(json.dumps(cases,indent=2)+'\n',encoding='utf8')
proof=dict(scope=__doc__,candidate_read=False,source_cases=2,cases_SHA256=sha(out/'cases.json'),old_plan_SHA256=sha(old/'plan.json'),old_cases_SHA256=sha(old/'cases.json'),only_navigation_changed=True,source_expectations_unchanged=True,actual_console_navigation_read=True,old_failed_run_preserved=True,constructor_SHA256=sha(__file__),console_cells_not_terminal_pixels=True)
(out/'plan.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'))))
