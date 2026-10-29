"""Exact per-occurrence PIM spans from each source-constructed owning row.
Never match a repeated value by searching Raw. Wrappers refer to child Address,
not AF/encoding prefixes. Verification metadata remains outside the wire arena.
"""
import re
from pim_prefix_tree_expected02 import field_rows

def projection(session,bindings_source):
 out=[];rows=field_rows(session)
 def emit(name,value,span=None,origin='pdu'):
  item=dict(name=name,value=value,origin=origin)
  if origin=='pdu':
   assert span is not None,(name,value,'missing exact owning row span')
   assert len(span)==2 and all(type(x)is int for x in span)and 0<=span[0]<span[1]
   item['pdu_range']=dict(offset=span[0],length=span[1]-span[0])
  out.append(item)
 for name,key,bits,nested in re.findall(r'\{"([^"]+)", "([^"]+)", (\d+), (true|false)\}',bindings_source):
  for row in rows if nested=='true'else[session]:
   value=row.get(key)
   if type(value)is int and value>=0:emit('pim.'+name,value,row.get('Wire Field Ranges',{}).get(key))
 for name,key,nested in re.findall(r'\{"([^"]+)", "([^"]+)", (true|false)\}',bindings_source):
  for row in rows if nested=='true'else[session]:
   if name.startswith('source_addr.')and row.get('Kind')!='source':continue
   if name.startswith('group_addr.')and row.get('Kind')!='group':continue
   value=row.get(key)
   if type(value)is bool:emit('pim.'+name,value,row.get('Wire Field Ranges',{}).get(key))
 for role,path in [('upstream_neighbor','Upstream Neighbor'),('source','Source'),('group','Group'),('originator','Originator'),('bsr','BSR'),('rp','RP')]:
  for v6 in(False,True):
   for row in rows:
    if'Dummy Header'in row and row.get('IP Version')!=0:continue
    value=row.get(path);span=row.get('Wire Field Ranges',{}).get(path)
    if path=='Group'and row.get('Kind')=='group':value=row.get('Address');span=row.get('Wire Field Ranges',{}).get('Address')
    if path=='Source'and row.get('Kind')=='source':value=row.get('Address');span=row.get('Wire Field Ranges',{}).get('Address')
    if path!='Group'and isinstance(value,dict):span=value.get('Wire Field Ranges',{}).get('Address');value=value.get('Address')
    if type(value)is str and(':'in value)==v6:emit('pim.'+role+('_ip6'if v6 else''),value,span)
 for name,path in [('join_ip','Joined Sources'),('prune_ip','Pruned Sources'),('address_list','Addresses')]:
  for v6 in(False,True):
   for row in rows:
    for value in row.get(path)or[]:
     address=value.get('Address')
     if type(address)is str and(':'in address)==v6:emit('pim.'+name+(('_ip6'if name=='address_list'else'6')if v6 else''),address,value.get('Wire Field Ranges',{}).get('Address'))
 if type(session.get('Checksum Verified'))is bool:emit('pim.observed.checksum_verified',session['Checksum Verified'],origin='context')
 for key in('Source','Group'):
  row=session.get('Register IP Header')or{};value=row.get(key)
  if type(value)is str:emit('pim.observed.register.'+key.lower(),value,row.get('Wire Field Ranges',{}).get(key))
 return sorted(out,key=lambda x:x['name'])
