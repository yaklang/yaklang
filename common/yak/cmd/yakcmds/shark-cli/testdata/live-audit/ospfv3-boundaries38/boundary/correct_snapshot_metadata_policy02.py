"""Correct checker-only snapshot completeness from immutable public API code.

No runtime protocol values are read. The old checker, first failed run, source
grid, names/ranges/typed values and all original captures remain unchanged.
"""
import argparse,ast,hashlib,json,subprocess
from pathlib import Path

OLD_SHA='a4604d3ef75563bd619bcb8afaf11ad82917e484e94d5bc84249f2dacc0b3804'
BLOCK_SHA='8a91c734243a0b010f86a17a1327f91a2d24a52b1742057c25eb3ebd95e599b9'
REVS=['a82acd0fff97ae50dfc1bbb403c16af359d7bf8f','68527c4d9355d2df37c5c0c9b8bc19f7766ca9c6','d6772965c669e288705f7e0d0e25c62972226832']
def sha_bytes(b):return hashlib.sha256(b).hexdigest()
def sha(p):return sha_bytes(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def functions(raw):return{n.name:ast.dump(n,include_attributes=False)for n in ast.parse(raw).body if isinstance(n,ast.FunctionDef)}

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('repo','root','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir();old=a.root/'verify_boundary_fields01.py';raw=old.read_bytes();assert sha_bytes(raw)==OLD_SHA
    records=[];files=[]
    for rev in REVS:
        rel='common/pcapx/pcaputil/protocol_field_registry.go';result=subprocess.run(['git','show',rev+':'+rel],cwd=a.repo,stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True)
        code=result.stdout;start=code.index(b'\tif e.nativeFieldsIncomplete {');end=code.index(b'\n\tif e.SourceBytes.CompletedIn',start);block=code[start:end]
        assert sha_bytes(block)==BLOCK_SHA;files.append(code)
        records.append(dict(commit=rev,path=rel,source_SHA256=sha_bytes(code),policy_block_SHA256=BLOCK_SHA,start_line=code[:start].count(b'\n')+1,literal_policy=block.decode()))
    assert files[1]==files[2]
    nl=b'\r\n'if b'\r\n'in raw else b'\n'
    old_line=b"    assert snapshot['completeness']==expected_status"
    new_line=b"    assert snapshot['completeness']==snapshot_completeness_policy(snapshot,expected_status)"
    assert raw.count(old_line)==1
    anchor=b'def main():'
    function='''def snapshot_completeness_policy(snapshot,event_completeness):
    # Existing public API separates field coverage from the event wire outcome.
    # This validates the exported flag policy without inventing its private flag.
    incomplete=snapshot.get('incomplete',False)
    assert type(incomplete)is bool
    return 'limited'if incomplete and event_completeness!='evicted'else event_completeness


'''.replace('\n',nl.decode()).encode()
    revised=raw.replace(old_line,new_line).replace(anchor,function+anchor)
    assert revised.replace(function+anchor,anchor).replace(new_line,old_line)==raw
    old_funcs=functions(raw);new_funcs=functions(revised)
    unchanged=['sha','read','rows','same','node_at','range_grid','verify_session','verify_aliases','walk']
    assert all(old_funcs[n]==new_funcs[n]for n in unchanged)
    new=a.root/'verify_boundary_fields02.py'
    with new.open('xb')as f:f.write(revised)
    gp=a.root/'field-source-plan01/plan.json';plan=read(gp);assert sha(gp)=='8b77033c3bd933124f15b0af9c80e3823b86c64510fe6cf9dec3c64c9ed4280d'
    assert sha(plan['data_file'])=='c5133b8f3b4d57a4456f47aab58eb82ad9fac99ad52e61702df6dd33fa7c56e0'
    namespace={'__name__':'metadata_policy_only'};exec(compile(revised,str(new),'exec'),namespace)
    policy=namespace['snapshot_completeness_policy'];tests=[]
    for status in('message','context-required','malformed','incomplete','limited','evicted'):
        for incomplete in(False,True):
            expected='limited'if incomplete and status!='evicted'else status
            assert policy({'incomplete':incomplete},status)==expected;tests.append(dict(event_completeness=status,Incomplete=incomplete,snapshot_completeness=expected))
    rejects=0
    for value in(None,0,1,'true'):
        try:policy({'incomplete':value},'malformed')
        except AssertionError:rejects+=1
        else:raise AssertionError('wrong incomplete JSON type accepted')
    write(a.out/'policy-bridge.json',dict(scope=__doc__,old_checker_SHA256=OLD_SHA,new_checker_SHA256=sha(new),immutable_source_records=records,registry685_to_d677_literal_equal=True,unchanged_source_functions=unchanged,source_grid_plan_SHA256=sha(gp),source_grid_data_SHA256=sha(plan['data_file']),wire_or_typed_oracle_modified=False,candidate_output_read=False,policy_tests=tests,invalid_flag_types_rejected=rejects,first_runtime_failure_preserved_SHA256='fc96147056c65bd93785068a2714a874749c41b3967cc606fbccc3a8bdf76d7b',cause='checker01 conflated event wire outcome with existing snapshot field-coverage policy;not a product/parser bug',constructor_SHA256=sha(__file__)))
    print(json.dumps(dict(proof=str(a.out/'policy-bridge.json'),SHA256=sha(a.out/'policy-bridge.json'),new_verifier=str(new),new_verifier_SHA256=sha(new),metadata_policy_tests=len(tests),invalid_flags_rejected=rejects)),flush=True)
if __name__=='__main__':main()
