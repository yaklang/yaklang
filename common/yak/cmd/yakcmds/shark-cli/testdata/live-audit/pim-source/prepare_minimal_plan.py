"""Construct the minimum PIM false-derived-field correction from source facts."""
import argparse,collections,copy,hashlib,json,sys
from pathlib import Path
KEYS={'pim.metric_pref':'Metric Preference','pim.rpt':'RPT','pim.prune_indicator':'Prune Indicator','pim.prune_now':'Prune Now','pim.assert_override':'Assert Override'}
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def key(n,v):return n,type(v).__name__,json.dumps(v,sort_keys=True)
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--prefix-plan',type=Path,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(Path(__file__).parent));from pim_alias_expected import decode
 plans=[]
 for case in('weber','sharon'):
  for l in(a.prefix_plan/(case+'-whole-prefix-plan.jsonl')).open(encoding='utf8'):
   x=json.loads(l);s=x['source'];old=x['whole_old_native'];ip=next(h for h in reversed(s['location']['headers'])if h['kind']in('IPv4','IPv6'));facts=decode(bytes.fromhex(s['body_hex']),ip['source'],ip['destination'],s['declared_PIM_bytes']);pool=collections.Counter(key(f['alias'],f['value'])for f in facts['typed_alias_occurrences']);bad=[]
   for v in old['field_values']['values']:
    if v['origin']!='pdu'or not v['name'].startswith('pim.'):continue
    k=key(v['name'],v['value'])
    if pool[k]:pool[k]-=1
    else:bad.append(v['name'])
   if not bad:continue
   assert case=='sharon'and all(name in KEYS for name in bad);expected=copy.deepcopy(old)
   for name in bad:del expected['session'][KEYS[name]]
   expected['field_values']['values']=[v for v in expected['field_values']['values']if v['name']not in bad];err=copy.deepcopy(x['literal_prefix_expected']['diagnostic']);err['absolute_offset']=s['location']['offset']+err['offset']
   plans.append(dict(frame=s['frame'],old_id=old['id'],source=s,first_own_grammar_error=err,source_confirmed_prefix_fields=x['literal_prefix_expected']['fields'],required_absent_session_keys=sorted(KEYS[n]for n in bad),required_absent_typed_aliases=sorted(bad),whole_old_native=old,minimal_no_fabricated_fields_whole_expected=expected,scope='Minimum derived-field correction only. Scope/extent/progressive-field improvements require their separately frozen full schema; nothing else in this object is waived.'))
 assert len(plans)==6 and sum(len(x['required_absent_typed_aliases'])for x in plans)==21
 f=a.out/'six-source-whole-plan.jsonl';f.write_text(''.join(json.dumps(x,ensure_ascii=False,separators=(',',':'))+'\n'for x in plans),encoding='utf8');summary=dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),prefix_plan_SHA256=sha(a.prefix_plan/'summary.json'),plan_SHA256=sha(f),events=6,fields=21);(a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(a.out/'summary.json')
if __name__=='__main__':main()
