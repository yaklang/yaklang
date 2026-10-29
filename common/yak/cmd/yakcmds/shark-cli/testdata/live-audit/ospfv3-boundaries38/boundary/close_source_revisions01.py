"""Close source01/02/03 literal authored bytes without reading a producer."""
import argparse,hashlib,json
from pathlib import Path

SHAS=['8603f16a3d23a7bb4c62741bb2b3bb623822757e71f273b110d25f91932bf21e','83aa407034d0f21f8ffab5217f6a6b990bbf7e2d2cddb678ff8b2d25d23cac19','e9f5b8e4f2820bf12ca79bf7049562442f0bec7540d1a910c7dd2d11da570cd0']
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--root',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists()
    sources=[];bridges=[]
    for i,expected in enumerate(SHAS,1):
        d=a.root/('frozen-source%02d'%i);p=d/'plan.json';assert sha(p)==expected
        plan=read(p);assert sha(d/'cases.json')==plan['cases_SHA256'];cases={x['name']:x for x in read(d/'cases.json')}
        assert len(cases)==[17,18,20][i-1]
        for c in cases.values():
            for key in('capture','original_frame','source_facts'):assert sha(c[key])==c[key+'_SHA256']
        sources.append((d,cases))
    for (before,old),(after,new)in zip(sources,sources[1:]):
        assert set(old)<=set(new);rows=[]
        for name,c in old.items():
            row=dict(case=name)
            for k in('capture','original_frame','source_facts'):
                assert Path(c[k]).read_bytes()==Path(new[name][k]).read_bytes();row[k+'_SHA256']=c[k+'_SHA256']
            oc={k:v for k,v in c.items()if k not in('capture','original_frame','source_facts')};nc={k:v for k,v in new[name].items()if k not in('capture','original_frame','source_facts')};assert oc==nc
            rows.append(row)
        bridges.append(dict(before=before.name,after=after.name,unchanged_cases=len(rows),unchanged_files=3*len(rows),added_cases=sorted(set(new)-set(old)),literal_original_bytes_equal=True,case_metadata_equal_except_absolute_paths=True,rows=rows))
    report=dict(scope=__doc__,source_plan_SHA256=SHAS,bridges=bridges,candidate_output_read=False,native_executed=False,driver_SHA256=sha(__file__))
    with a.out.open('x',encoding='utf8')as f:json.dump(report,f,indent=2);f.write('\n')
    print(json.dumps(dict(proof=str(a.out),SHA256=sha(a.out),unchanged_cases=[17,18],added_cases=[1,2])))
if __name__=='__main__':main()
