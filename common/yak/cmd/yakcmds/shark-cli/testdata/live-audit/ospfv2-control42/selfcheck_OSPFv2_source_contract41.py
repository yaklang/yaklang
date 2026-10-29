from pathlib import Path
import collections,hashlib,json
import ospfv2_source_contract41 as contract
import verify_OSPFv2_direct_fields41 as oracle
root=Path(__file__).parent
_,facts,patterns,_=oracle.load_plan(root/'OSPFv2-direct-source-plan41-01');lookup={f['sourcecase']:f for f in facts};totals=collections.Counter()
for path in sorted((root/'OSPFv2-independent-source39-01/source-grid03').glob('input-*-source-grid.json')):
 for fact in json.loads(path.read_text('utf8')):
  name='real/input-'+str(fact['input'])+'/frame-'+str(fact['frame'])
  _,raw=contract.source_wire(fact);fields=contract.fields(fact)
  totals.update(oracle.verify_case(lookup[name],raw,4096,fields,'',{'context':False,'budget':False,'integrity':False,'format':False},patterns))
  event=contract.event(1,fact);assert event['fields']==fields;totals['source_constructed_events']+=1
  totals['typed_aliases']+=len(contract.aliases(fields))
print(json.dumps(dict(scope='source-only constructor selfcheck, no candidate read',contract_SHA256=hashlib.sha256(Path(contract.__file__).read_bytes()).hexdigest(),counts=dict(totals),passed=True)))
