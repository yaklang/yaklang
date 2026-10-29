"""Verify three direct-only budget/pseudo-header controls from source fields.

This is no native-ingress/label proof. A root-written execution receipt must
bind source, helper_source_SHA256, binary_SHA256, arguments, exit_code and
export_SHA256. No private typed snapshot is exported by this direct probe.
"""
import argparse,hashlib,importlib.util,json,traceback
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def load(p):
    spec=importlib.util.spec_from_file_location('direct_session_checker',p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def texts(tree):
    if not isinstance(tree,dict):return []
    return [tree['text']]+[x for child in tree.get('children',[])for x in texts(child)]

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('grid','metadata-contract','field-verifier','cases','export','receipt','helper-source','binary','out'):ap.add_argument('--'+n,type=Path,required=True)
    ap.add_argument('--source',required=True);a=ap.parse_args();assert not a.out.exists();a.out.mkdir()
    try:
        assert sha(a.grid/'plan.json')=='8b77033c3bd933124f15b0af9c80e3823b86c64510fe6cf9dec3c64c9ed4280d'
        plan=read(a.grid/'plan.json');assert sha(plan['data_file'])==plan['data_SHA256']and sha(a.metadata_contract)==plan['metadata_contract_SHA256']
        grid={r['case']:r for r in read(plan['data_file'])if r['direct_codec_invocation_required']};assert len(grid)==3
        cases={r['name']:r for r in read(a.cases)if r.get('requires_direct_codec_invocation')};assert set(cases)==set(grid)
        assert sha(a.cases.parent/'plan.json')==plan['source_plan_SHA256']
        receipt=read(a.receipt);assert receipt['source']==a.source and receipt['exit_code']==0
        assert receipt['helper_source_SHA256']==sha(a.helper_source)and receipt['binary_SHA256']==sha(a.binary)and receipt['export_SHA256']==sha(a.export)
        assert receipt['arguments']==[str(a.binary),str(a.cases)]
        bound={str(p):sha(p)for p in[Path(__file__),a.grid/'plan.json',Path(plan['data_file']),a.metadata_contract,a.field_verifier,a.cases,a.cases.parent/'plan.json',a.export,a.receipt,a.helper_source,a.binary]}
        for c in cases.values():
            for k in('capture','original_frame','source_facts'):assert sha(c[k])==c[k+'_SHA256'];bound[c[k]]=c[k+'_SHA256']
        rows=[json.loads(line)for line in a.export.open(encoding='utf-8-sig')];assert len(rows)==3 and set(r['case']for r in rows)==set(grid)
        v=load(a.field_verifier);meta=read(a.metadata_contract);results=[];failures=[]
        for row in rows:
            name=row['case'];c=cases[name];f=grid[name]
            try:
                assert row['direct_codec_only']is True and row['source_frame_SHA256']==c['original_frame_SHA256']
                assert row['budget']==c.get('raw_codec_collection_budget',4096)
                assert row['pseudo_header_unavailable']is c.get('raw_codec_context_override',{}).get('PseudoHeaderUnavailable',False)
                leaves=v.verify_session(f,row['fields'],meta);alltext=texts(row['error_tree']);assert row['error']==row['error_tree']['text']
                budget=name.startswith('budget1-');integrity=name=='budget1-bad-packet-sum'
                assert row['budget_identity']is budget
                if budget:assert any(meta['shared']['budget_sentinel']in s for s in alltext)
                if name=='pseudo-destination-unavailable':assert row['context_identity']is True and not row['budget_identity']
                assert any(s==meta['shared']['integrity_sentinel']for s in alltext)is integrity,'sentinel leaf must represent independently proven packet corruption'
                if 'integrity_identity'in row:assert row['integrity_identity']is integrity
                results.append(dict(case=name,source_leaves=leaves,source_scalars_ranges_checksum_applicability_verified=True,budget_identity=budget,integrity_sentinel_leaf=integrity,Go_errors_Is_integrity_checked='integrity_identity'in row,typed_snapshot_not_exported=True))
            except Exception as e:failures.append(dict(case=name,error=repr(e),traceback=traceback.format_exc()))
        after={p:sha(p)for p in bound};assert bound==after
        report=dict(scope=__doc__,source=a.source,passed_cases=len(results),failed_cases=len(failures),source_leaves=sum(r['source_leaves']for r in results),results=results,failures=failures,private_typed_snapshot_checked=False,native_ingress_or_CLI_claim=False,candidate_output_used_to_construct_expectations=False,input_hashes=bound,unchanged_input_hashes=bound==after,verifier_SHA256=sha(__file__))
        with(a.out/'summary.json').open('x',encoding='utf8')as out:json.dump(report,out,indent=2);out.write('\n')
        print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),passed=len(results),failed=len(failures))),flush=True)
        if failures:raise SystemExit(1)
    except Exception:
        with(a.out/'controller-failure.json').open('x',encoding='utf8')as out:json.dump(dict(error=traceback.format_exc()),out,indent=2);out.write('\n')
        raise
if __name__=='__main__':main()
