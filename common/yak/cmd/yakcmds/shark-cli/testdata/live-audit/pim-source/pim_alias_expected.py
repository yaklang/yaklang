"""PIM typed-alias source contracts layered over frozen literal prefix facts.
This models the current public native alias roles, including the generic source
alias on encoded joined/pruned sources. It does not claim WS shares all roles.
Every occurrence is paired with its own literal PDU bytes and span.
"""
import copy
from pim_literal_expected import decode as prefix_decode

def decode(w,source,destination,declared_bytes,budget=4096):
 result=prefix_decode(w,source,destination,declared_bytes,budget);facts=copy.deepcopy(result['fields'])
 for field in result['fields']:
  path=field['path']
  if path and path[-1]=='Address'and any(x in('Joined Sources','Pruned Sources')for x in path):
   other=copy.deepcopy(field);other['alias']='pim.source_ip6'if field['length']==16 else'pim.source';facts.append(other)
 # The Register's Version nibble is a real inner wire fact, not inferred family.
 register=[f for f in result['fields']if f['path'][:1]==['Register IP Header']]
 if register and len(w)>8:
  facts.append(dict(path=['Register IP Header','IP Version'],alias='pim.ip_version',offset=8,length=1,value=w[8]>>4,raw_hex=w[8:9].hex(),mask=240,kind='unsigned'))
 result['typed_alias_occurrences']=[f for f in facts if f['alias']]
 return result
