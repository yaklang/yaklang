"""Pure Python verifier controls; no producer, no serialized expected events.

Ephemeral source-tree witnesses check the checker only. Positive tests are not
native correctness evidence; negative mutations must fail the independent grid.
"""
import argparse,base64,copy,hashlib,importlib.util,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def load(p):
    s=importlib.util.spec_from_file_location('frozen_field_verifier',p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def at(v,path):
    for k in path:v=v[k]
    return v

def witness(row,meta):
    f=row['source_fields'];s=copy.deepcopy(f['source_wire_tree'])
    for c in f['checksum_requirements']:
        n=at(s,c['path']);n['Checksum Verified']=c['verified'];n['Checksum Scope']=c['scope']if not c['scope'].startswith('unverified-')else'unverified: incomplete or invalid LSA boundary'
    s.update({'Packet Name':f['packet_name'],'Authentication Verified':False,'Observation Scope':'bounded wire advertisements only; no LSDB, verified neighbor or installed route'})
    bits=f['expected_packet_bits'];raw=Path(row['original_frame']).read_bytes()[54:]
    if bits and(bits['AT']or bits['LLS']):
        s['Authentication Trailer Bit']=bits['AT'];s['LLS Bit']=bits['LLS'];s['Trailing Bytes']=base64.b64encode(raw[int.from_bytes(raw[2:4],'big'):]).decode()
        if bits['LLS']and not bits['AT']:s['LLS Scope']=meta['shared']['LLS Scope']
    for spec in f['structural_requirements']:
        if spec.get('required'):
            start,end=spec['owned_bytes_range'];s[spec['path'][0]]=base64.b64encode(raw[start:end]).decode()
    return s

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('grid','metadata-contract','verifier','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();gp=read(a.grid/'plan.json');assert sha(a.grid/'plan.json')=='8b77033c3bd933124f15b0af9c80e3823b86c64510fe6cf9dec3c64c9ed4280d'
    assert sha(gp['data_file'])==gp['data_SHA256']and sha(a.metadata_contract)==gp['metadata_contract_SHA256']
    grid={r['case']:r for r in read(gp['data_file'])};meta=read(a.metadata_contract);v=load(a.verifier);results=[]
    for name,row in grid.items():
        v.verify_session(row,witness(row,meta),meta);v.verify_aliases(row,[copy.deepcopy(x)for group in row['expected_private_typed_aliases'].values()for x in group])
        results.append(dict(kind='positive source-only ephemeral witness',case=name,accepted=True))
    def reject(name,case,mutate,typed=False):
        row=grid[case]
        obj=[copy.deepcopy(x)for group in row['expected_private_typed_aliases'].values()for x in group]if typed else witness(row,meta)
        mutate(obj)
        try:
            v.verify_aliases(row,obj)if typed else v.verify_session(row,obj,meta)
        except(AssertionError,KeyError,TypeError,ValueError,IndexError):results.append(dict(kind='negative checker mutation',name=name,case=case,rejected=True));return
        raise AssertionError('checker accepted mutation: '+name)
    reject('scalar value changed','unknown-u1-then-known',lambda s:s['Header'].__setitem__('Router ID','198.51.100.99'))
    reject('scalar JSON type changed','unknown-u1-then-known',lambda s:s['Header'].__setitem__('Version','3'))
    reject('known sibling hidden','unknown-u1-then-known',lambda s:s['LSAs'].pop())
    reject('invented nil sibling','capture-short-after-unknown',lambda s:s['LSAs'].append(None))
    reject('unknown body guessed','unknown-u1-then-known',lambda s:s['LSAs'][0]['Body'].__setitem__('Flags',0))
    reject('complete LSA Fletcher match hidden','unknown-u1-then-known',lambda s:s['LSAs'][0].__setitem__('Checksum Verified',False))
    reject('bad packet checksum claimed valid','bad-packet-sum-unsupported-body',lambda s:s.__setitem__('Checksum Verified',True))
    reject('packet scope regresses to absent pseudoheader','unknown-u1-then-known',lambda s:s.__setitem__('Checksum Scope',meta['shared']['Checksum Scope']['missing_pseudo_header']))
    reject('missing captured prefix scalar','capture-short-next-LSA-prefix12',lambda s:s['LSAs'][1].pop('Advertising Router'))
    reject('uncaptured Length fabricated','capture-short-next-LSA-prefix18',lambda s:s['LSAs'][1].__setitem__('Length',24))
    reject('uncaptured Body fabricated','capture-short-next-LSA-prefix18',lambda s:s['LSAs'][1].__setitem__('Body',{}))
    reject('prefix range extends into absent Length','capture-short-next-LSA-prefix18',lambda s:s['LSAs'][1]['Wire Field Ranges'].__setitem__('Checksum',[60,64]))
    reject('prefix LSA checksum falsely verified','capture-short-next-LSA-prefix18',lambda s:s['LSAs'][1].__setitem__('Checksum Verified',True))
    reject('AT checksum omission falsely verified','AT-Hello-dummy-digest',lambda s:s.__setitem__('Checksum Verified',True))
    reject('LLS bit erased','LLS-only-good',lambda s:s.__setitem__('LLS Bit',False))
    reject('AF opaque body removed','unknown-AF-instance64',lambda s:s.pop('Unparsed Body'))
    def add_exact(values):
        x=next(x for x in values if type(x['value'])is int);x['exact_value']=str(x['value'])
    reject('invented exact_value on <=32-bit dedicated alias','unknown-u1-then-known',add_exact,True)
    def dedup(values):
        seen=set()
        for i,x in enumerate(values):
            key=(x['name'],json.dumps(x['value'],sort_keys=True))
            if key in seen:values.pop(i);return
            seen.add(key)
        raise AssertionError('source has no duplicate value occurrence')
    reject('same-value source occurrence deduplicated','unknown-u1-then-known',dedup,True)
    report=dict(scope=__doc__,positive_source_witnesses=20,negative_mutations=18,all_negative_mutations_rejected=True,native_or_CLI_run=False,native_correctness_claim=False,candidate_output_read=False,results=results,grid_plan_SHA256=sha(a.grid/'plan.json'),verifier_SHA256=sha(a.verifier),selftest_SHA256=sha(__file__))
    with a.out.open('x',encoding='utf8')as f:json.dump(report,f,indent=2);f.write('\n')
    print(json.dumps(dict(proof=str(a.out),SHA256=sha(a.out),positive=20,negative=18)),flush=True)
if __name__=='__main__':main()
