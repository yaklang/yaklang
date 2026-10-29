from pathlib import Path
import copy,hashlib,json
root=Path(__file__).parent;old=root/'Windows-OSPFv2-source-plan42-01';out=root/'Windows-OSPFv2-crypto-source-plan42-02';out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
assert sha(old/'plan.json')=='a626cc75a9cbc64be3398970d76a0243f28724d4711a746f4d18c4b2e0a7c814'
original=next(r for r in json.loads((old/'cases.json').read_text('utf8'))if r['name']=='Cryptographic-LLS');updated=copy.deepcopy(original);updated['field_down']=60
assert {k:v for k,v in updated.items()if k!='field_down'}=={k:v for k,v in original.items()if k!='field_down'}
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
write(out/'cases.json',[updated]);write(out/'plan.json',dict(scope='Navigation-only retry after the actual pane did not yet show the selected cryptographic LLS TLV. Exact source bytes, field name/value/ranges and all other expectations are unchanged.',prior_plan_SHA256=sha(old/'plan.json'),prior_cases_SHA256=sha(old/'cases.json'),source_grid=original['source_grid'],source_grid_SHA256=original['source_grid_SHA256'],cases_SHA256=sha(out/'cases.json'),source_cases=1,candidate_read=False,UI_navigation_observed=True,only_adjusted_key='field_down',old_down=44,new_down=60,source_expectations_literal=True))
print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'))))
