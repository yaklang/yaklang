"""Source-only default-budget RSVP/Intserv composition and schema projection.

No Go or candidate runtime is imported. Labels/types follow the published65
field schema. All values/ranges come from frozen source45/source48 models.
"""
from pathlib import Path
import base64,collections,copy,hashlib,importlib.util,re
P=Path(__file__).parent.parent
WR='Wire Field Ranges'

def load(p,name):
 s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
legacy=load(P/'rsvp_native_cli_checker46.py','unchanged_RSVP_export_schema46')
base=load(P/'rsvp_boundary_source45.py','unchanged_RSVP_base_source45')
neutral=load(P/'Intserv-source48-01/intserv_neutral_source48.py','frozen_Intserv_neutral_source48')
SPECS=list(legacy.SPECS)
for name,key,scope,bits in[
 ('version_and_unused','Version and Unused','intserv-header',16),('message_length_words','Message Length Words','intserv-header',16),
 ('service','Service Number','intserv-service',8),('service_flags','Flags','intserv-service',8),('service_length_words','Data Length Words','intserv-service',16),
 ('parameter','Parameter Number','intserv-parameter',8),('parameter_flags','Flags','intserv-parameter',8),('parameter_length_words','Data Length Words','intserv-parameter',16),
 ('minimum_policed_unit','Minimum Policed Unit','intserv-value',32),('maximum_packet_size','Maximum Packet Size','intserv-value',32),
 ('composed_hop_count','Composed IS Hop Count','intserv-value',32),('composed_minimum_latency','Composed Minimum Path Latency','intserv-value',32),('composed_mtu','Composed Path MTU','intserv-value',32)]:SPECS.append(('rsvp.intserv.'+name,key,scope,'unsigned',bits,False))
for name,key,scope,typ,bits in[
 ('version','Version','intserv-header','unsigned',4),('unused','Unused Bits','intserv-header','unsigned',12),
 ('service_break','Break Bit Observed','intserv-service','boolean',0),('service_reserved','Reserved Flag Bits','intserv-service','unsigned',7),
 ('parameter_invalid_observed','Invalid Bit Observed','intserv-parameter','boolean',0),('parameter_reserved','Reserved Flag Bits','intserv-parameter','unsigned',7)]:SPECS.append(('rsvp.intserv.'+name,key,scope,typ,bits,True))
for name,scope in [('token_bucket_rate','intserv-rate'),('token_bucket_size','intserv-bucket'),('peak_data_rate','intserv-peak'),('available_path_bandwidth','intserv-bandwidth')]:
 SPECS.extend([('rsvp.intserv.'+name+'_bits','IEEE754 Bits',scope,'unsigned',32,False),('rsvp.intserv.'+name,'Value',scope,'string',0,False),('rsvp.intserv.'+name+'_classification','Classification',scope,'string',0,True)])
assert len(SPECS)==65
FLOATS={'Token Bucket Rate','Token Bucket Size','Peak Data Rate','Composed Available Path Bandwidth'}

def parts(path):return [int(v)if v.isdigit()else v for v in re.split(r'\.|\[|\]',path)if v]
def assign(root,path,value):
 tokens=parts(path);node=root
 for i,key in enumerate(tokens[:-1]):
  if isinstance(node,list):
   while len(node)<=key:node.append({})
   node=node[key]
  else:node=node.setdefault(key,[]if isinstance(tokens[i+1],int)else{})
 node[tokens[-1]]=value

def parent(root,path):
 tokens=parts(path);node=root
 for key in tokens[:-1]:node=node[key]
 return node,tokens[-1]

def mapped(path):
 if path.startswith('Header.'):return'Intserv Header.'+path[7:]
 if path.startswith('Root.'):return{'Root.Opaque Message Data':'Opaque Intserv Data','Root.Unparsed Message Data':'Unparsed Intserv Data','Root.Trailing Message Bytes':'Trailing Intserv Bytes'}[path]
 if re.fullmatch(r'Services\[\d+\]\.Opaque Service Data',path):return path.replace('.Opaque Service Data','.Body.Opaque Service Data')
 return path

def intserv_body(source):
 out={}
 for leaf in source['leaves']:
  path=mapped(leaf['path']);value=copy.deepcopy(leaf['value'])
  if leaf['kind']=='ieee32':
   value={'IEEE754 Bits':value['wire_uint32'],'Value':value['exact_decimal']if value['exact_decimal']is not None else value['display'],'Classification':value['classification'],'Units':'bytes'if path.endswith('Token Bucket Size')else'bytes/s',WR:{'IEEE754 Bits':leaf['range'],'Value':leaf['range']}}
  assign(out,path,value)
  if leaf['literal']:
   node,key=parent(out,path);node.setdefault(WR,{})[key]=leaf['range']
 return out

def scoped(tree,scope):
 if not scope.startswith('intserv-'):return legacy.scoped(tree,scope)
 out=[]
 for obj in tree.get('Objects')or[]:
  if obj.get('Header',{}).get('Class')not in(9,12,13):continue
  body=obj.get('Body')or{}
  if scope=='intserv-header':
   if body.get('Intserv Header'):out.append(body['Intserv Header'])
   continue
  for service in body.get('Services')or[]:
   if scope=='intserv-service':out.append(service.get('Header')or{});continue
   for parameter in service.get('Parameters')or[]:
    if scope=='intserv-parameter':out.append(parameter.get('Header')or{});continue
    value=parameter.get('Body')or{}
    if scope=='intserv-value':out.append(value);continue
    key={'intserv-rate':'Token Bucket Rate','intserv-bucket':'Token Bucket Size','intserv-peak':'Peak Data Rate','intserv-bandwidth':'Composed Available Path Bandwidth'}.get(scope)
    if key and key in value:out.append(value[key])
 return out

def aliases(tree,raw):
 out=[]
 for name,key,scope,typ,bits,derived in SPECS:
  if name in('rsvp.checksum_omitted','rsvp.checksum_verified')and len(raw)<8:continue
  for row in scoped(tree,scope):
   if key not in row or row[key]is None:continue
   item={'name':name,'value':copy.deepcopy(row[key]),'origin':'derived'if derived else'pdu'}
   if not derived:
    span=row.get(WR,{}).get(key)
    if not span or span[1]<=span[0]:continue
    item['pdu_range']={'offset':span[0],'length':span[1]-span[0]}
   out.append(item)
 return sorted(out,key=lambda r:r['name'])

def compose(raw,targets):
 """WholePDU4096. The actual target model consumes nested service/param tokens.

All current inputs stay far below4096. Early hard/context child refusals stop
later base objects, whose owned remaining bytes are not decoded as siblings.
"""
 result=base.decode_source(raw,4096);tree=copy.deepcopy(result['fields']);objects=tree.get('Objects')or[];issues=list(result['issues']);nested=[];selected=True;kept=[];semantic=[]
 for oi,obj in enumerate(objects):
  head=obj.get('Header',{});cls=head.get('Class');ctype=head.get('C-Type');extent=head.get(WR,{}).get('Length')
  if cls in(9,12,13)and ctype==2 and extent:
   start=extent[0]+4;end=extent[0]+head['Length'];source=neutral.decode(raw,start,end,cls,4095);neutral.validate_source_leaves(raw,source)
   body=intserv_body(source);obj['Body']=body;nested.append({'object_index':oi,'body_start':start,'object_end':end,'source':source})
   selected=selected and source['field_layouts_complete']
   for x in source['issues']:
    issues.append({'kind':'need_more'if x['kind']=='shortage'else x['kind'],'reason':x['code'],'offset':x['range'][0]})
    if x['kind']=='semantic':semantic.append({'object_index':oi,'issue':x})
   kept.append(obj)
   if any(x['kind']in('format','shortage','budget','context')for x in source['issues']):
    cut=min(end,len(raw),int.from_bytes(raw[6:8],'big'))
    if cut<min(len(raw),int.from_bytes(raw[6:8],'big')):
     value=raw[cut:min(len(raw),int.from_bytes(raw[6:8],'big'))];tree['Unparsed Body']=base64.b64encode(value).decode();tree.setdefault(WR,{})['Unparsed Body']=[cut,cut+len(value)]
    break
  else:
   selected=selected and base.layout(cls,ctype)is not None
   kept.append(obj)
 if'Objects'in tree:tree['Objects']=kept or None
 hard=any(x['kind']in('format','need_more','budget','context','integrity')for x in issues)
 tree['Wire Grammar Complete']=not hard;tree['Field Layouts Complete']=not hard and selected
 # Source45 opaque observations are deliberately replaced, never added to the
 # Intserv leaf coverage. All non-Intserv base literal/range facts survive.
 return {'fields':tree,'issues':issues,'nested':nested,'semantic_issues':semantic,'raw_SHA256':hashlib.sha256(raw).hexdigest(),'default_global_budget':4096}

def inventory(tree,path=''):
 out={}
 if isinstance(tree,dict):
  for key,span in tree.get(WR,{}).items():out[(path+'.'if path else'Root.')+key]=(tree[key],span)
  for key,value in tree.items():
   if key!=WR:out.update(inventory(value,path+'.'+key if path else key))
 elif isinstance(tree,list):
  for i,value in enumerate(tree):out.update(inventory(value,path+'['+str(i)+']'))
 return out

def semantic_code(code,name):
 return {'token-bucket-must-be-positive':'token-rate-and-bucket-size-must-be-positive','minimum-policed-unit-zero-explicitly-illegal':'minimum-policed-unit-must-be-positive','maximum-packet-size-not-positive':'maximum-packet-size-must-be-positive','invalid-peak-rate-representation':'invalid-nonnegative-float-representation','invalid-finite-nonnegative-representation':'path-bandwidth-requires-finite-nonnegative'if name=='Composed Available Path Bandwidth'else'invalid-nonnegative-float-representation'}.get(code,code)