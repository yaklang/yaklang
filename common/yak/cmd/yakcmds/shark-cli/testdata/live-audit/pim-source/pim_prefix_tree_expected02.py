"""Whole PIM prefix tree/typed projection from original bounded wire facts.
Record/null/list shape follows the explicit pre-runtime contract. This module
never reads candidate output and never scans bytes for a plausible child.
"""
import base64,copy,ipaddress,re
from pim_literal_prefix02 import decode
SCOPE='captured field prefixes only; control message not fully validated; no installed routing or verified neighbor state'
ALIASES=[('Version','Version and Type'),('Type','Version and Type'),('Border','Register Flags'),('Null Register','Register Flags'),('RPT','Metric Preference Word'),('Metric Preference','Metric Preference Word'),('Prune Indicator','Refresh Flags'),('Prune Now','Refresh Flags'),('Assert Override','Refresh Flags'),('Tracking Support','Propagation Flags'),('Propagation Delay','Propagation Flags'),('Bidirectional','Flags'),('Admin Scope Zone','Flags'),('Sparse','Flags'),('Wildcard','Flags'),('RPT','Flags')]
def get(root,path):
 for key in path:root=root[key]
 return root

def put(root,path,value):
 at=root
 for i,key in enumerate(path[:-1]):
  want_list=type(path[i+1])is int
  if type(key)is int:
   while len(at)<=key:at.append(None)
   if at[key]is None:at[key]=[]if want_list else{}
  elif at.get(key)is None:at[key]=[]if want_list else{}
  at=at[key]
 key=path[-1]
 if type(key)is int:
  while len(at)<=key:at.append(None)
  at[key]=value
 else:at[key]=value

def ranges(info):
 own=info.setdefault('Wire Field Ranges',{})
 for alias,word in ALIASES:
  if alias in info and word in own:own[alias]=own[word][:]
 return info

def tree(body,baseline,facts):
 old=baseline.get('session')
 if old is None or baseline['status']=='decoded'or facts.get('grammar_complete'):
  return copy.deepcopy(old)
 typ=body[0]&15
 if typ==1:
  # Register inner-header partial projection is deliberately unchanged.
  out=copy.deepcopy(old)
  for key in('Border','Null Register'):
   if 'Register Flags'not in out:out.pop(key,None)
  return ranges(out)
 out={'Packet Name':old['Packet Name'],'Authentication Verified':False,'Observation Scope':SCOPE}
 if typ==0:out['Options']=None
 if typ in(2,5,9):
  out['Group']=None;out['Source']=None
  if typ==9:out['Originator']=None
 if typ in(3,6,7):out['Upstream Neighbor']=None;out['Groups']=None
 if typ==4:out['BSR']=None;out['Groups']=None;out['Bootstrap Reassembly']='not attempted; fragment advertisements only'
 if typ==8:out['RP']=None;out['Advertised Groups']=None
 if typ==4:out['No Forward']=bool(body[1]&128)
 if typ==2:out['Packed Register Stop Capable']=bool(body[1]&1)
 for f in facts['fields']:
  # Values have been read within the actual own envelope by the source oracle.
  v=f['value']
  if f['kind']=='address':
   a=ipaddress.ip_address(v)
   if a.version==6 and a.ipv4_mapped is not None:v='::ffff:'+str(a.ipv4_mapped)
  put(out,f['path'],v);parent=get(out,f['path'][:-1]);parent.setdefault('Wire Field Ranges',{})[f['path'][-1]]=[f['offset'],f['offset']+f['length']]
 for node in facts['address_nodes']:
  info=get(out,node['path']);info['Kind']=node['kind']
  if node['kind']=='source'and'Flags'in info and'Mask Length'in info:
   width={1:32,2:128}.get(info.get('Address Family'));flags=info['Flags'];info['Source Semantics Valid']=info['Mask Length']==width and(not flags&2 or bool(flags&1))
  ranges(info)
 for group in out.get('Groups')or[]:
  if typ in(3,6,7):group.setdefault('Joined Sources',None);group.setdefault('Pruned Sources',None)
  elif typ==4:group.setdefault('RPs',None)
  ranges(group)
  if typ==4:
   for rp in group.get('RPs')or[]:ranges(rp)
 for path in facts['completed_bootstrap_groups']:
  group=get(out,path);assert'RP Count'in group and'Fragment RP Count'in group
  group['RP Set Complete In Message']=group['RP Count']==group['Fragment RP Count']
 if typ==0:
  for option in out['Options']or[]:
   if option.get('Option Type')in(24,65001):option.setdefault('Addresses',None)
   ranges(option)
 out['Unparsed Body']=base64.b64encode(body[facts['observed_prefix_end']:]).decode()
 return ranges(out)

def field_rows(root):
 rows=[]
 def walk(x):
  if isinstance(x,dict):
   if'Wire Field Ranges'in x:rows.append(x)
   for key,value in x.items():
    if key!='Wire Field Ranges':walk(value)
  elif isinstance(x,list):
   for v in x:walk(v)
 walk(root)
 return sorted(rows,key=lambda m:min((v[0]for v in m['Wire Field Ranges'].values()),default=float('inf')))

def projection(session,bindings_source):
 out=[];rows=field_rows(session)
 def emit(name,value,origin='pdu',range_key=None):
  item=dict(name=name,value=value,origin=origin)
  if origin=='pdu'and range_key:
   span=session.get('Wire Field Ranges',{}).get(range_key)
   if span is not None:item['pdu_range']=dict(offset=span[0],length=span[1]-span[0])
  out.append(item)
 for name,key,bits,nested in re.findall(r'\{"([^"]+)", "([^"]+)", (\d+), (true|false)\}',bindings_source):
  for row in rows if nested=='true'else[session]:
   value=row.get(key)
   if type(value)is int and value>=0:emit('pim.'+name,value,range_key=key if nested=='false'else None)
 for name,key,nested in re.findall(r'\{"([^"]+)", "([^"]+)", (true|false)\}',bindings_source):
  for row in rows if nested=='true'else[session]:
   if name.startswith('source_addr.')and row.get('Kind')!='source':continue
   if name.startswith('group_addr.')and row.get('Kind')!='group':continue
   value=row.get(key)
   if type(value)is bool:emit('pim.'+name,value,range_key=key if nested=='false'else None)
 for role,path in [('upstream_neighbor','Upstream Neighbor'),('source','Source'),('group','Group'),('originator','Originator'),('bsr','BSR'),('rp','RP')]:
  for v6 in(False,True):
   for row in rows:
    if'Dummy Header'in row and row.get('IP Version')!=0:continue
    value=row.get(path)
    if path=='Group'and row.get('Kind')=='group':value=row.get('Address')
    if path=='Source'and row.get('Kind')=='source':value=row.get('Address')
    if path!='Group'and isinstance(value,dict):value=value.get('Address')
    if type(value)is str and(':'in value)==v6:emit('pim.'+role+('_ip6'if v6 else''),value)
 for name,path in [('join_ip','Joined Sources'),('prune_ip','Pruned Sources'),('address_list','Addresses')]:
  for v6 in(False,True):
   for row in rows:
    for value in row.get(path)or[]:
     address=value.get('Address')
     if type(address)is str and(':'in address)==v6:emit('pim.'+name+(('_ip6'if name=='address_list'else'6')if v6 else''),address)
 if type(session.get('Checksum Verified'))is bool:emit('pim.observed.checksum_verified',session['Checksum Verified'],'context')
 for key in('Source','Group'):
  value=(session.get('Register IP Header')or{}).get(key)
  if type(value)is str:emit('pim.observed.register.'+key.lower(),value)
 return sorted(out,key=lambda x:x['name'])

def whole_event(source,baseline,bindings_source):
 body=bytes.fromhex(source['body_hex']);ip=next(h for h in reversed(source['location']['headers'])if h['kind']in('IPv4','IPv6'));facts=decode(body,ip['source'],ip['destination'],source['declared_PIM_bytes']);session=tree(body,baseline,facts);out=copy.deepcopy(baseline)
 if session!=baseline['session']:
  out['session']=session
  if out['fields']is not None:out['fields']=copy.deepcopy(session)
  values=[copy.deepcopy(v)for v in out['field_values']['values']if not v['name'].startswith('pim.')];values.extend(projection(session,bindings_source));out['field_values']['values']=values  # Existing API reserves event batch before protocol batch.
 return out,facts
