"""Freeze source-only progressive PIM field facts before a production repair."""
import argparse,base64,collections,hashlib,json,sys,xml.etree.ElementTree as ET
from pathlib import Path

def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def dump(p,x):p.write_text(json.dumps(x,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
def at_path(v,path):
 try:
  for key in path:v=v[key]
  return True,v
 except(KeyError,IndexError,TypeError):return False,None

def ws_value(f,expected):
 text=f.get('show')
 if type(expected)is bool:return text=='True'if expected else text=='False'
 if type(expected)is int:
  try:return int(text,0)==expected
  except(ValueError,TypeError):
   try:return int(text)==expected
   except(ValueError,TypeError):return False
 return text==expected

def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('source','ws','out'):p.add_argument('--'+n,type=Path,required=True)
 a=p.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(Path(__file__).parent));from pim_literal_expected import decode
 summary={'scope':__doc__,'constructor_SHA256':sha(Path(__file__).with_name('pim_literal_expected.py')),'driver_SHA256':sha(Path(__file__)),'cases':{}}
 for case in('weber','sharon'):
  q={int(x.find(".//field[@name='frame.number']").get('show')):x for x in ET.parse(a.ws/(case+'.pdml')).getroot().findall('packet')};plans=[];missing=[];different=[];WS_equal=[];WS_absent=[];WS_diff=[];ranges_absent=[];status=collections.Counter();diag=collections.Counter()
  for line in(a.source/(case+'-whole-source-bindings.jsonl')).open(encoding='utf8'):
   row=json.loads(line);s=row['source'];e=row['whole_old_native'];ip=next(h for h in reversed(s['location']['headers'])if h['kind']in('IPv4','IPv6'));body=bytes.fromhex(s['body_hex']);result=decode(body,ip['source'],ip['destination'],s['declared_PIM_bytes']);status[(e['status'],result['complete_observation'])]+=1;diag[(result.get('diagnostic')or{}).get('kind','none')]+=1
   plans.append(dict(source=s,whole_old_native=e,literal_prefix_expected=result))
   for f in result['fields']:
    item=dict(frame=s['frame'],native_id=e['id'],field=f)
    if f['length']==0:continue
    exists,v=at_path(e.get('session')or{},f['path'])
    if not exists:missing.append(item)
    elif type(v)is not type(f['value'])or v!=f['value']:different.append(dict(**item,actual=v))
    if not f['alias']or f['alias'].startswith('pim.observed.'):continue
    pos=s['location']['offset']+f['offset'];matches=[x for x in q[s['frame']].iter('field')if x.get('name')==f['alias']and x.get('pos')==str(pos)and x.get('size')==str(f['length'])]
    wi=dict(frame=s['frame'],native_id=e['id'],name=f['alias'],offset=pos,length=f['length'],expected=f['value'],raw_hex=f['raw_hex'])
    if not matches:WS_absent.append(wi)
    elif any(ws_value(x,f['value'])for x in matches):WS_equal.append(wi)
    else:WS_diff.append(dict(**wi,WS_fields=[x.attrib for x in matches]))
   for v in(e.get('field_values')or{}).get('values',[]):
    if v['name'].startswith('pim.')and v.get('origin')=='pdu'and 'pdu_range'not in v:ranges_absent.append(dict(frame=s['frame'],native_id=e['id'],name=v['name'],value=v['value']))
  (a.out/(case+'-whole-prefix-plan.jsonl')).write_text(''.join(json.dumps(x,ensure_ascii=False,separators=(',',':'))+'\n'for x in plans),encoding='utf8')
  for n,x in [('native-prefix-missing',missing),('native-prefix-different',different),('WS-equal',WS_equal),('WS-unavailable',WS_absent),('WS-differences',WS_diff),('native-pdu-range-absent',ranges_absent)]:dump(a.out/(case+'-'+n+'.json'),x)
  summary['cases'][case]=dict(events=len(plans),native_status_vs_complete_source=[dict(status=k[0],source_complete=k[1],count=v)for k,v in sorted(status.items())],own_grammar_diagnostics=dict(diag),prefix_fields_missing=len(missing),prefix_values_different=len(different),WS_equal_fields=len(WS_equal),WS_unavailable=len(WS_absent),WS_field_differences=len(WS_diff),native_wire_alias_ranges_absent=len(ranges_absent),plan_SHA256=sha(a.out/(case+'-whole-prefix-plan.jsonl')))
 dump(a.out/'summary.json',summary);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'cases':summary['cases']},ensure_ascii=False))
if __name__=='__main__':main()
