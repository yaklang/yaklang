"""Mechanical verifier correction; original source constructions stay literal."""
from pathlib import Path
import hashlib,json,shutil
root=Path(__file__).parent;prior=root/'OSPFv2-whole-source-plan41-01';out=root/'OSPFv2-whole-source-plan42-02';out.mkdir(exist_ok=False);frozen=out/'frozen';frozen.mkdir()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
assert sha(prior/'plan.json')=='bde795e998f963d818c92569cbb09950bff28c704c2b30998b527718c6ebb4c3'
old=(prior/'frozen/ospfv2_whole_bridge41.py').read_text('utf8')
before="""   left=iter(rows(old))
   for e in rows(new):
    if e.get('profile')=='ospfv2-bounded-control':continue
    assert same(refs.remap_known_pdu_references(next(left),mapping),e),('unplanned retained whole event',relative,e['id'])
    stats['retained_native_whole_objects']+=1
   if not selected_rows:assert old.read_bytes()==new.read_bytes();stats['whole_literal_native_exports']+=1
"""
after="""   if selected_rows:
    left=iter(rows(old))
    for e in rows(new):
     if e.get('profile')=='ospfv2-bounded-control':continue
     assert same(refs.remap_known_pdu_references(next(left),mapping),e),('unplanned retained whole event',relative,e['id'])
     stats['retained_native_whole_objects']+=1
   else:
    # The old two-public-input reference schema intentionally does not admit
    # unrelated HTTP/3 transaction numbers. An unaffected complete export
    # must be byte literal; it has no generated-ID substitution to perform.
    assert all(k==v for k,v in mapping.items())and old.read_bytes()==new.read_bytes()
    stats['retained_native_whole_objects']+=case['rows'];stats['whole_literal_native_exports']+=1
"""
assert old.count(before)==1;updated=old.replace(before,after)
(root/'ospfv2_whole_bridge41_v02.py').write_text(updated,encoding='utf8')
p=json.loads((prior/'plan.json').read_text('utf8'))
for name,digest in p['files'].items():
 src=prior/'frozen'/name;assert sha(src)==digest
 if name=='ospfv2_whole_bridge41.py':(frozen/name).write_text(updated,encoding='utf8')
 else:shutil.copyfile(src,frozen/name);assert sha(frozen/name)==digest
 p['files'][name]=sha(frozen/name)
for name in('cases.json','source-construction-witnesses.json'):
 shutil.copyfile(prior/name,out/name);assert (prior/name).read_bytes()==(out/name).read_bytes()
p.update(prior_plan_SHA256=sha(prior/'plan.json'),verifier_only_change='For unaffected exports require whole-file byte identity with identity ID ledger before considering the two-public-input generated-reference schema. No new protocol/path/transaction normalization is admitted.',original_source_expectations_literal=True,verifier_fix_after_candidate_read=True)
(out/'plan.json').write_text(json.dumps(p,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'),cases_SHA256=sha(out/'cases.json'),witnesses_SHA256=sha(out/'source-construction-witnesses.json'),source_contract_SHA256=sha(frozen/'ospfv2_source_contract41.py'))))
