"""Change only the snapshot helper binding; preserve capture/field/range facts."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent;old=root/'Windows-GTPv2-source-plan44-01';out=root/'Windows-GTPv2-source-plan45-02';out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
assert sha(old/'plan.json')=='5f0795e3a24c68a631654e0498603c6873c3f0c0269b287d8a6d3587eae416cf'
helper=root/'Windows-console-ack-build45-01/native-console.exe';build=read(helper.with_name('receipt.json'));assert build['exit_code']==0 and build['source_unchanged']and sha(helper)==build['binary_SHA256']
cases=read(old/'cases.json');before=read(old/'cases.json')
for c,o in zip(cases,before):
 c['helper'],c['helper_SHA256']=str(helper),sha(helper)
 assert {k:v for k,v in c.items()if k not in('helper','helper_SHA256')}=={k:v for k,v in o.items()if k not in('helper','helper_SHA256')}
(out/'cases.json').write_text(json.dumps(cases,indent=2)+'\n',encoding='utf8')
proof=dict(scope=__doc__,candidate_read=False,source_cases=2,cases_SHA256=sha(out/'cases.json'),original_plan_SHA256=sha(old/'plan.json'),original_cases_SHA256=sha(old/'cases.json'),only_helper_transport_changed=True,helper_build_SHA256=sha(helper.with_name('receipt.json')),field_navigation_unchanged=True,console_cells_not_terminal_pixels=True,constructor_SHA256=sha(__file__))
(out/'plan.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'))))
