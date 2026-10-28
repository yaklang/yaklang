"""Strict complete export bridge; only original-source-derived whole ICMPv4 objects may differ."""
import argparse,copy,hashlib,itertools,json,sys
from pathlib import Path
ap=argparse.ArgumentParser()
for n in('before','after','expected','bins','out'):ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--source-after',required=True);a=ap.parse_args();a.out.mkdir(exist_ok=False)
from icmp4_expected_source import same

def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
meta=json.loads((a.expected/'summary.json').read_text());built=json.loads((a.bins/'events-receipt.json').read_text());assert built['source']==a.source_after and built['exit_code']==0 and built['binary_SHA256']==sha(a.bins/'events.exe');assert meta['source_constructor_SHA256']==sha(Path(__file__).with_name('icmp4_expected_source.py'))and meta['literal_parser_SHA256']==sha(Path(__file__).with_name('literal_own_header.py'))
expected={};all_count=0;before_hashes={};before_proofs={}
for corpus in meta['corpora']:
 path=a.expected/corpus['corpus']/'expected.jsonl';assert corpus['uncovered']==0 and sha(path)==corpus['expected_ledger_SHA256'];rows=[json.loads(x)for x in path.open()];assert len(rows)==corpus['selected_all_ICMP_events']==corpus['whole_before_source_verified'];expected[corpus['corpus']]={x['id']:x for x in rows if x['intentional_change']};assert len(expected[corpus['corpus']])==corpus['changed'];all_count+=len(rows);before_hashes[corpus['corpus']]=corpus['SDK_SHA256'];before_proofs[corpus['corpus']]=sha(path)
assert all_count==5009 and sum(map(len,expected.values()))==475
oldfiles={p.relative_to(a.before):p for p in a.before.rglob('events.jsonl')};newfiles={p.relative_to(a.after):p for p in a.after.rglob('events.jsonl')};assert oldfiles.keys()==newfiles.keys();files=[];observed={name:set()for name in expected};unexpected=[]
for rel in sorted(oldfiles):
 old,new=oldfiles[rel],newfiles[rel];op=old.with_name('events-producer.json');np=new.with_name('events-producer.json');om=json.loads(op.read_text());nm=json.loads(np.read_text());assert om['source']==meta['source_before']and nm['source']==a.source_after and om['exit_code']==nm['exit_code']==0 and om['capture_SHA256']==nm['capture_SHA256'];assert om['export_SHA256']==sha(old)and nm['export_SHA256']==sha(new)and nm['binary_SHA256']==built['binary_SHA256']
 kind=rel.parts[1]if rel.parts[0]=='preservation'and len(rel.parts)==3 else None
 if kind in before_hashes:assert om['export_SHA256']==before_hashes[kind]
 count=0;literal=0;changes=[]
 with old.open('rb')as xfile,new.open('rb')as yfile:
  for number,(x,y)in enumerate(itertools.zip_longest(xfile,yfile),1):
   assert x is not None and y is not None,(str(rel),'different export length');count+=1
   if x==y:literal+=1;continue
   oe,ne=json.loads(x),json.loads(y)
   if kind in expected and oe['id']in expected[kind]:
    row=expected[kind][oe['id']];assert same(oe,row['old_event'])and same(ne,row['expected_new_event']),(str(rel),number,oe['id'],{k:[ne.get(k),row['expected_new_event'].get(k)]for k in ne.keys()|row['expected_new_event'].keys()if not same(ne.get(k),row['expected_new_event'].get(k))});assert oe['id']not in observed[kind];observed[kind].add(oe['id']);changes.append(dict(frame=row['frame'],id=oe['id'],old_status=oe['status'],new_status=ne['status'],type=ne['session']['Type'],code=ne['session']['Code']))
   else:unexpected.append(dict(file=str(rel),row=number,old_id=oe['id'],new_id=ne['id'],changed_keys=[k for k in oe.keys()|ne.keys()if not same(oe.get(k),ne.get(k))]))
 files.append(dict(input=str(rel),rows=count,literal_unchanged_rows=literal,whole_source_expected_changes=len(changes),changes=changes,before_SHA256=om['export_SHA256'],after_SHA256=nm['export_SHA256'],before_producer_SHA256=sha(op),after_producer_SHA256=sha(np)))
assert not unexpected and all(observed[k]==expected[k].keys()for k in expected),(unexpected,{k:len(v)for k,v in observed.items()})
flows=[]
for old in sorted(a.before.rglob('flows.json')):
 new=a.after/old.relative_to(a.before);assert sha(old)==sha(new),(str(old),'flows differ');flows.append(dict(input=str(old.relative_to(a.before)),SHA256=sha(old)))
negative=[];allrows=[r for k in expected.values()for r in k.values()];short=next(r for r in allrows if r['expected_new_event']['status']=='incomplete');union=next(r for r in allrows if 'Rest of Header'in r['expected_new_event']['session']);mtu=next(r for r in allrows if 'MTU'in r['expected_new_event']['session'])
for name,row,fn in[
 ('missing-prefix-status',short,lambda e:e.__setitem__('status','decoded')),
 ('invented-completion-on-short-body',short,lambda e:e['source_bytes'].__setitem__('CompletedIn',copy.deepcopy(e['source_bytes']['PacketRefs'][0]))),
 ('invented-partial-time-word',short,lambda e:e['session'].__setitem__('Transmit Timestamp',0)),
 ('false-zero-echo-ID-for-union',union,lambda e:e['session'].__setitem__('Identifier',0)),
 ('false-sequence-for-MTU',mtu,lambda e:e['session'].__setitem__('Sequence',e['session']['MTU'])),
 ('MTU-number-coerced-to-text',mtu,lambda e:e['session'].__setitem__('MTU',str(e['session']['MTU']))),
 ('wire-body-trust-invention',union,lambda e:e['session'].__setitem__('Checksum Verified',True)),
 ('forged-flow-ownership',union,lambda e:e.__setitem__('flow',1)),
 ('foreign-source-domain',union,lambda e:e['domain'].__setitem__('Interface',e['domain']['Interface']+1)),
 ('PDU-alias-reassignment',union,lambda e:e.__setitem__('id',e['id']+1)),
 ('snapshot-origin-substitution',union,lambda e:e['field_values'].__setitem__('pdu',e['id']+1))]:
 changed=copy.deepcopy(row['expected_new_event']);fn(changed);assert not same(changed,row['expected_new_event']);negative.append(name)
summary=dict(scope='Strict complete37 export bridge:475 entire changed ICMPv4 objects reconstructed from original wire; all other rows byte-identical. No scalar/path/domain/PDU-ID normalization.',source_before=meta['source_before'],source_after=a.source_after,inputs=len(files),total_rows=sum(x['rows']for x in files),literal_rows=sum(x['literal_unchanged_rows']for x in files),whole_source_expected_changes=sum(x['whole_source_expected_changes']for x in files),old_source_objects_verified=all_count,negative_controls=negative,flows_entire_files_literal=flows,expected_source_proof_SHA256=sha(a.expected/'summary.json'),expected_ledgers_SHA256=before_proofs,source_constructor_SHA256=sha(Path(__file__).with_name('icmp4_expected_source.py')),auditor_SHA256=sha(Path(__file__)),files=files)
path=a.out/'summary.json';path.write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(path),SHA256=sha(path),counts={k:summary[k]for k in('inputs','total_rows','literal_rows','whole_source_expected_changes','old_source_objects_verified')})),flush=True)
