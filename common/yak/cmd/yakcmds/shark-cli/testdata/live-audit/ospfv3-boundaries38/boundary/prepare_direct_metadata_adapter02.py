"""Adapt receipt binding only; retain frozen direct source value/range checks.

The root's closed producer uses structured inputs_before/after rather than flat
helper/binary receipt keys. No direct output fields are read by this preparer.
"""
import argparse,ast,hashlib,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--root',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();a.out.mkdir()
    old=a.root/'verify_direct_fields01.py';assert sha(old)=='1a0b6ed40459384baf3574a0aee2c3412e3a7ffd848a186888c4032334d437fc'
    raw=old.read_bytes();nl=b'\r\n'if b'\r\n'in raw else b'\n'
    before="""        assert receipt['helper_source_SHA256']==sha(a.helper_source)and receipt['binary_SHA256']==sha(a.binary)and receipt['export_SHA256']==sha(a.export)
        assert receipt['arguments']==[str(a.binary),str(a.cases)]""".replace('\n',nl.decode()).encode()
    after="""        assert receipt['export_SHA256']==sha(a.export)and receipt['arguments']==[str(a.binary),str(a.cases)]
        assert receipt['inputs_unchanged']is True and receipt['inputs_before']==receipt['inputs_after']
        inputs=receipt['inputs_before']
        for record in inputs.values():assert sha(record['path'])==record['SHA256']and Path(record['path']).stat().st_size==record['bytes']
        assert inputs['helper']['path']==str(a.helper_source)and inputs['helper']['SHA256']==sha(a.helper_source)
        assert inputs['binary']['path']==str(a.binary)and inputs['binary']['SHA256']==sha(a.binary)
        assert inputs['cases']['path']==str(a.cases)and inputs['cases']['SHA256']==sha(a.cases)
        build=read(inputs['build']['path']);assert build['source']==a.source and build['HEAD_after']==a.source and build['exit_code']==0
        assert build['source_files_equal']is True and build['source_file_SHA256']==build['helper_after_SHA256']==sha(a.helper_source)
        assert build['binary_SHA256']==sha(a.binary)
        assert 'errors.Is(decodeErr,stream_parser.ErrOSPFv3WireIntegrity)'in a.helper_source.read_text('utf8')""".replace('\n',nl.decode()).encode()
    assert raw.count(before)==1;newraw=raw.replace(before,after)
    opt=b"                if 'integrity_identity'in row:assert row['integrity_identity']is integrity"
    required=b"                assert row['integrity_identity']is integrity"
    assert newraw.count(opt)==1;newraw=newraw.replace(opt,required);assert newraw.replace(required,opt).replace(after,before)==raw
    ast.parse(newraw);new=a.root/'verify_direct_fields02.py'
    with new.open('xb')as f:f.write(newraw)
    fp=a.root/'field-source-plan01/plan.json';grid=json.loads(fp.read_text('utf8'));assert sha(grid['data_file'])=='c5133b8f3b4d57a4456f47aab58eb82ad9fac99ad52e61702df6dd33fa7c56e0'
    proof=dict(scope=__doc__,old_verifier_SHA256=sha(old),new_verifier_SHA256=sha(new),value_range_checksum_checks_unchanged=True,metadata_only_changes=['structured root producer input/build proof binding','require explicitly exported Go errors.Is integrity flag'],source_grid_SHA256=sha(grid['data_file']),candidate_protocol_output_read=False,constructor_SHA256=sha(__file__))
    with(a.out/'receipt-adapter-bridge.json').open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps(dict(verifier=str(new),SHA256=sha(new),proof=str(a.out/'receipt-adapter-bridge.json'),proof_SHA256=sha(a.out/'receipt-adapter-bridge.json'))),flush=True)
if __name__=='__main__':main()
