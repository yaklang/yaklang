"""Compare every SDK message or CLI physical row without normalizing any field.

Only JSON object-key ordering and insignificant JSON whitespace may differ.
Identity numbers, protocol labels, typed values, wire bytes, fields, sources,
flows, transactions and associations must retain their exact types and values.
Inputs and detailed results stay outside Git.
"""
import argparse, collections, hashlib, itertools, json
from pathlib import Path

ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','out']:ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--kind',choices=['SDK','CLI'],required=True)
a=ap.parse_args()
for p in [a.before,a.after,a.out]:
    assert not any((q/'.git').exists()for q in [p.resolve(),*p.resolve().parents]),'raw exports and detailed certificates remain outside Git'

def sha(path):
    h=hashlib.sha256()
    with path.open('rb')as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()

def rows(path):
    def invalid(v):raise ValueError('non-JSON numeric constant: '+v)
    def unique(items):
        d={}
        for k,v in items:
            assert k not in d,'duplicate JSON key'
            d[k]=v
        return d
    for line in path.open(encoding='utf-8-sig'):
        yield json.loads(line,parse_constant=invalid,object_pairs_hook=unique)

def canonical(row):return json.dumps(row,sort_keys=True,separators=(',',':'),ensure_ascii=True).encode()

def paths(x,y,path=''):
    if type(x)is not type(y):return [path+' (type)']
    if isinstance(x,dict):
        result=[]
        for k in sorted(x.keys()|y.keys()):
            if k not in x or k not in y:result.append(path+'/'+k+' (presence)')
            else:result.extend(paths(x[k],y[k],path+'/'+k))
            if len(result)>=24:break
        return result[:24]
    if isinstance(x,list):
        if len(x)!=len(y):return [path+' (length)']
        result=[]
        for i,(v,w)in enumerate(zip(x,y)):
            result.extend(paths(v,w,path+'/'+str(i)))
            if len(result)>=24:break
        return result[:24]
    return []if canonical(x)==canonical(y)else[path]

count=0;differences=[];protocols=collections.Counter();digest=hashlib.sha256()
for index,(old,new)in enumerate(itertools.zip_longest(rows(a.before),rows(a.after)),1):
    assert old is not None and new is not None,('complete ordered row count differs',index)
    assert isinstance(old,dict)and isinstance(new,dict),'each row is an object'
    count+=1;protocols[new['protocol']]+=1
    if canonical(old)!=canonical(new):
        if len(differences)<64:differences.append({'row':index,'id':new.get('id'),'frame':new.get('number'),'paths':paths(old,new)})
    digest.update(canonical(new)+b'\n')
assert count,'empty exports are not a successful comparison'
result={'scope':__doc__,'kind':a.kind,'records_compared':count,'before_sha256':sha(a.before),'after_sha256':sha(a.after),'ordered_semantic_export_sha256':digest.hexdigest(),'protocol_record_counts':dict(sorted(protocols.items())),'changed_records_first_64':differences,'all_records_exact':not differences,'identity_or_field_normalization':False}
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
assert not differences,('unexpected complete-export differences',differences[:3])
print(json.dumps({'kind':a.kind,'records_compared':count,'all_records_exact':True,'semantic_sha256':digest.hexdigest()}))
