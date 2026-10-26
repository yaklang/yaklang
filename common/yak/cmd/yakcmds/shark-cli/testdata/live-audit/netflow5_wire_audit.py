"""Independent fixed-record NetFlow v5 wire/reference audit.
Inputs: original/supplemental TShark JSON -x, native SDK JSONL, output aggregate.
UDP service selection/Decode-As and original reference omissions stay separate.
Reported records never become captured connections. Complete fixed layouts with invalid residual nanoseconds retain malformed
status and literal fields. Carrier lengths are independently checked.
Time-derived aliases are unselected; literal first/last uptime, export seconds/nanoseconds remain exact.
"""
from pathlib import Path
import sys,json,hashlib,base64,collections,ipaddress,re,struct
oracle,events,out=map(Path,sys.argv[1:4]);sha=lambda b:hashlib.sha256(b).hexdigest();u=lambda w:int.from_bytes(w,'big')
def spans(v):
 if isinstance(v,list) and len(v)>=3 and isinstance(v[0],str):return [v]
 if isinstance(v,list):return [a for x in v for a in spans(x)]
 return []
def flatten(node,values,ranges):
 if isinstance(node,dict):
  for k,v in node.items():
   if k.startswith('cflow.') and k.endswith('_raw'):ranges[k[:-4]].extend(spans(v))
   elif k.startswith('cflow.') and not isinstance(v,dict):
    if isinstance(v,list) and all(not isinstance(x,(dict,list)) for x in v):values[k].extend(v)
    elif not isinstance(v,list):values[k].append(v)
    else:flatten(v,values,ranges)
   else:flatten(v,values,ranges)
 elif isinstance(node,list):
  for v in node:flatten(v,values,ranges)
def canonical(v):
 if isinstance(v,str) and '.' in v:return ipaddress.ip_address(v).packed.hex()
 return int(v,16 if isinstance(v,str) and v.startswith('0x') else 10) if isinstance(v,str) else v
capture_frames={}
if len(sys.argv)>4:
 b=Path(sys.argv[4]).read_bytes();assert b[:4] in [bytes.fromhex('d4c3b2a1'),bytes.fromhex('a1b2c3d4')],'classic PCAP needed for independent fragment proof'
 endian='<' if b[:4]==bytes.fromhex('d4c3b2a1') else '>'
 assert len(b)>=24 and struct.unpack_from(endian+'I',b,20)[0]==1,'selected Ethernet capture required'
 at=24
 while at<len(b):
  assert at+16<=len(b);_,_,caplen,wirelen=struct.unpack_from(endian+'IIII',b,at);at+=16;assert caplen<=wirelen and at+caplen<=len(b)
  capture_frames[len(capture_frames)+1]=b[at:at+caplen];at+=caplen

def fragment_ids(l):
 ids=l.get('ip',{}).get('ip.fragments_tree',{}).get('ip.fragment',[])
 return [int(x) for x in (ids if isinstance(ids,list) else [ids])]

def observed_fragment_group(ids):
 if not ids or not capture_frames:return ids
 first=capture_frames.get(ids[0],b'')
 if len(first)<34:return ids
 key=(first[26:34],first[18:20],first[23]);group=[]
 # Include same-key overlapping fragments the reference may have discarded.
 # A later generation and unfragmented datagrams do not supply this proof.
 for number in range(min(ids),max(ids)+1):
  w=capture_frames.get(number,b'')
  if len(w)>=34 and w[12:14]==b'\x08\x00' and w[14]>>4==4 and u(w[20:22])&0x3fff and (w[26:34],w[18:20],w[23])==key:group.append(number)
 return group

def reconstruct(ids):
 if not capture_frames:return None,['reassembly proof requires original physical capture bytes']
 pieces=[];key=None;end=None
 for number in ids:
  if number not in capture_frames:return None,['fragment source frame unavailable']
  w=capture_frames[number]
  if len(w)<34 or w[12:14]!=b'\x08\x00' or w[14]>>4!=4:return None,['selected Ethernet IPv4 fragment header unavailable']
  ihl=(w[14]&15)*4;total=u(w[16:18]);flags=u(w[20:22]);start=(flags&0x1fff)*8
  if ihl<20 or total<ihl or 14+total>len(w) or w[23]!=17:return None,['fragment literal IPv4 length/protocol invalid']
  observed=(w[26:34],w[18:20],w[23]);part=w[14+ihl:14+total]
  if key is not None and key!=observed:return None,['fragment tuple/identifier mismatch']
  key=observed
  if not part or start+len(part)>65535 or flags&0x2000 and len(part)%8:return None,['fragment bounds/alignment invalid']
  if not flags&0x2000:
   tail=start+len(part)
   if end is not None and end!=tail:return None,['conflicting fragment tails']
   end=tail
  pieces.append((start,part))
 if end is None:return None,['missing final fragment']
 result=bytearray(end);seen=bytearray(end)
 for start,part in pieces:
  if start+len(part)>end:return None,['fragment extends beyond final tail']
  for i,value in enumerate(part,start):
   if seen[i] and result[i]!=value:return None,['conflicting fragment overlap; receiver quarantined']
   result[i],seen[i]=value,1
 if not all(seen):return None,['incomplete fragment byte coverage']
 return bytes(result),[]

def carrier_errors(l,raw):
 # Confirm the actual enclosing datagram, not just a reference dissector's
 # clipped application TVB. This selected audit checks direct complete UDP.
 reassembled=spans(l.get('ip',{}).get('ip.fragments_raw'))
 if reassembled:
  actual,errors=reconstruct(observed_fragment_group(fragment_ids(l)))
  if errors:return errors
  if actual!=bytes.fromhex(reassembled[0][0]):return ['independent reassembly and reference TVB disagree']
  if len(actual)<8 or u(actual[4:6])!=len(actual) or raw[1]!=8 or actual[8:]!=bytes.fromhex(raw[0]):return ['reassembled UDP header/payload boundaries disagree']
  return []
 frame=spans(l.get('frame_raw'));udps=spans(l.get('udp_raw'))
 if not frame or not udps or 'udp' not in l:return ['no direct captured UDP wire']
 if any(x in l['frame']['frame.protocols'].split(':') for x in ['icmp','icmpv6']):return ['quoted transport is not another captured UDP datagram']
 b=bytes.fromhex(frame[0][0]);at=udps[-1][1]
 if at<0 or at+8>len(b):return ['short actual UDP header']
 size=u(b[at+4:at+6])
 if size<8 or at+size>len(b):return ['UDP declared length exceeds complete captured bytes']
 ip=spans(l.get('ip_raw'));ip6=spans(l.get('ipv6_raw'))
 if ip:
  begin=ip[-1][1]
  if begin+20>len(b) or b[begin]>>4!=4:return ['invalid enclosing IPv4 header']
  ihl=(b[begin]&15)*4;total=u(b[begin+2:begin+4]);frag=u(b[begin+6:begin+8])
  if ihl<20 or total<ihl or begin+total>len(b) or b[begin+9]!=17 or at!=begin+ihl or at+size>begin+total:return ['IPv4 and UDP literal lengths or protocol disagree']
  if frag&0x3fff:return ['fragmented carrier needs separate reassembly evidence']
 elif ip6:
  begin=ip6[-1][1]
  if begin+40>len(b) or b[begin]>>4!=6 or b[begin+6]!=17:return ['IPv6 extension/reassembly carrier outside direct audit scope']
  total=40+u(b[begin+4:begin+6])
  if at!=begin+40 or begin+total>len(b) or at+size>begin+total:return ['IPv6 and UDP literal lengths disagree']
 else:return ['enclosing IP unavailable']
 if raw[1]!=at+8 or bytes.fromhex(raw[0])!=b[at+8:at+size]:return ['application TVB is not exactly the captured UDP payload']
 return []
header=[('Version','cflow.version',0,2),('Header Record Count','cflow.count',2,4),('System Uptime Milliseconds',None,4,8),('Unix Seconds','cflow.unix_secs',8,12),('Unix Nanoseconds','cflow.unix_nsecs',12,16),('Sequence Number','cflow.sequence',16,20),('Engine Type','cflow.engine_type',20,21),('Engine ID','cflow.engine_id',21,22)]
fields=[(8,'cflow.srcaddr',0,4),(12,'cflow.dstaddr',4,4),(15,'cflow.nexthop',8,4),(10,'cflow.inputint',12,2),(14,'cflow.outputint',14,2),(2,'cflow.packets',16,4),(1,'cflow.octets',20,4),(22,None,24,4),(21,None,28,4),(7,'cflow.srcport',32,2),(11,'cflow.dstport',34,2),(6,'cflow.tcpflags',37,1),(4,'cflow.protocol',38,1),(5,'cflow.tos',39,1),(16,'cflow.srcas',40,2),(17,'cflow.dstas',42,2),(9,'cflow.srcmask',44,1),(13,'cflow.dstmask',45,1)]
selected={a for _,a,_,_ in header if a}|{a for _,a,_,_ in fields if a}|{'cflow.samplingmode','cflow.samplerate'}
ref={};versions=collections.Counter();valid=set();refused_carriers={}
for x in json.loads(oracle.read_text(encoding='utf-8-sig')):
 l=x['_source']['layers'];n=int(l['frame']['frame.number']);vs,rs=collections.defaultdict(list),collections.defaultdict(list);flatten(l.get('cflow'),vs,rs);versions.update(map(str,vs.get('cflow.version',[])))
 wire=spans(l.get('cflow_raw'));ref[n]=(l,vs,rs,wire)
 for raw in wire:
  w=bytes.fromhex(raw[0])
  issues=carrier_errors(l,raw) if len(w)>=2 and u(w[:2])==5 else ['different export version']
  if len(w)>=2 and u(w[:2])==5 and issues:refused_carriers[n]=issues
  if not issues and len(w)>=24 and u(w[:2])==5 and 1<=u(w[2:4])<=30 and len(w)==24+48*u(w[2:4]) :valid.add(n)
checks=collections.Counter();diff=[];matched=set();records=0;extras=[];unprojected=collections.Counter();statuses=collections.Counter();range_views=collections.Counter()
def equal(label,got,want,n):
 checks[label]+=1
 if got!=want:diff.append({'frame':n,'check':label,'native':got,'reference_or_wire':want})
for line in events.open(encoding='utf-8-sig'):
 e=json.loads(line)
 if e.get('protocol')!='netflow-v5':continue
 statuses[e['status']]+=1;w=base64.b64decode(e['raw'] or '');refs=e['source_bytes'].get('PacketRefs') or []
 candidates=[(r['Number'],s) for r in refs if r['Number'] in ref for s in ref[r['Number']][3] if s[0]==w.hex()]
 if not candidates:extras.append({'native_id':e['id'],'status':e['status'],'refs':refs});continue
 n,physical=candidates[-1];matched.add(n);l,vs,rs,_=ref[n]
 if n not in valid:
  equal('invalid fixed-record wire never decoded',e['status']=='decoded',False,n);continue
 m=e.get('session') or {};count=u(w[2:4]);
 reassembled=bool(fragment_ids(l));range_views['reassembled UDP TVB' if reassembled else 'captured physical UDP']+=1
 equal('source view kind',e['source_bytes']['Kind'],'reassembled' if reassembled else 'captured',n)
 if reassembled:
  ids=[r['Number'] for r in refs];actual,errors=reconstruct(ids)
  equal('native contributor reconstruction errors',errors,[],n)
  equal('native contributor payload hash',sha((actual or b'')[8:]),sha(w),n)
  equal('native contributors are actual reference fragments',set(ids)<=set(fragment_ids(l)),True,n)
  equal('completion frame',e['source_bytes'].get('CompletedIn',{}).get('Number'),n,n)
  equal('no future fragment evidence',all(i<=n for i in ids),True,n)
  equal('classic capture contributor domains',[r['Domain'] for r in refs],[{'Section':0,'Interface':0,'Encapsulation':''} for _ in refs],n)
 equal('clock-sensitive status',e['status'],'decoded' if u(w[12:16])<1000000000 else 'malformed',n)
 equal('export timestamp validity',m.get('Export Timestamp Valid'),u(w[12:16])<1000000000,n)
 equal('current PDU source hash',e['source_bytes']['SHA256'],sha(w),n)
 equal('actual UDP transport',e['transport'],'udp',n)
 family='ipv6' if 'ipv6' in l else 'ip'
 for endpoint,hostkey,portkey in [('source','src','srcport'),('destination','dst','dstport')]:
  host=l[family][family+'.'+hostkey];host=('['+host+']') if ':' in host else host
  equal('captured endpoint is not a reported record',e[endpoint],host+':'+l['udp']['udp.'+portkey],n)
 equal('reported records do not allocate captured flow',e['flow'],0,n)
 equal('export counts do not allocate transaction',e['transaction'],0,n);equal('export report has no response association',e['response_to'],0,n);equal('reported direction is not captured direction',e['direction'],0,n)
 equal('stateless receiver has no synthetic FlowSets','FlowSets' in m,False,n)
 for name,alias,lo,hi in header:
  equal('literal header value',m.get(name),u(w[lo:hi]),n);equal('literal header range',(m.get('Wire Field Ranges') or {}).get(name),[lo,hi],n)
 equal('sampling mode two bits',m['Sampling Mode'],u(w[22:24])>>14,n);equal('sampling interval fourteen bits',m['Sampling Interval'],u(w[22:24])&0x3fff,n)
 for k in ['Sampling Mode','Sampling Interval']:equal('shared sampling storage range',m['Wire Field Ranges'].get(k),[22,24],n)
 rows=m.get('Records') or [];equal('record array length',len(rows),count,n);equal('record count metadata',m.get('Data Record Count'),count,n);equal('complete record count evidence',m.get('Record Count Verified'),True,n)
 for index,row in enumerate(rows):
  records+=1;at=24+48*index;equal('record index',row['Index'],index+1,n);fs=row.get('Fields') or [];equal('every field present',len(fs),len(fields),n)
  for f,(typ,alias,off,size) in zip(fs,fields):
   lo,hi=at+off,at+off+size;raw=w[lo:hi]
   equal('field IE-width-scope association',[f['Field Type'],f['Field Length'],f['Scope']],[typ,size,False],n)
   equal('field literal range',f['Wire Field Ranges'].get('Value'),[lo,hi],n)
   if typ in [8,12,15]:equal('reported IPv4 literal',ipaddress.IPv4Address(f['Value']).packed.hex(),raw.hex(),n)
   else:equal('reported unsigned literal',f['Value'],u(raw),n)
  for k,off,size in [('Padding 1',36,1),('Padding 2',46,2)]:
   equal('literal unused padding',row[k],u(w[at+off:at+off+size]),n);equal('unused padding range',row['Wire Field Ranges'][k],[at+off,at+off+size],n)
 native=collections.defaultdict(list)
 for v in e['field_values']['values']:
  if v['name'] in selected:native[v['name']].append(v)
 for alias,wants in {
  'cflow.observed.sysuptime_ms':[(u(w[4:8]),4,4)],
  'cflow.observed.first_uptime_ms':[(u(w[24+48*i+24:24+48*i+28]),24+48*i+24,4) for i in range(count)],
  'cflow.observed.last_uptime_ms':[(u(w[24+48*i+28:24+48*i+32]),24+48*i+28,4) for i in range(count)],
  'pdu.length':[(len(w),None,None)]}.items():
  got=[v for v in e['field_values']['values'] if v['name']==alias]
  equal('literal typed metadata '+alias,[v['value'] for v in got],[v for v,_,_ in wants],n)
  if alias!='pdu.length':equal('literal typed metadata range '+alias,[(v['pdu_range']['offset'],v['pdu_range']['length']) for v in got if 'pdu_range' in v],[(lo,size) for _,lo,size in wants],n)
 for k in sorted(selected):
  want=vs.get(k) or [];got=native.get(k) or []
  if not want and not got:continue
  expected=collections.Counter(canonical(v) for v in want);actual=collections.Counter(canonical(v['value']) for v in got)
  equal('typed alias exact occurrences '+k,actual,expected,n)
  ranges=rs.get(k) or []
  if len(ranges)==len(want):
   expected=collections.Counter((canonical(v),r[1],r[2]) for v,r in zip(want,ranges))
   actual=collections.Counter((canonical(v['value']),physical[1]+v['pdu_range']['offset'],v['pdu_range']['length']) for v in got if 'pdu_range' in v)
   equal('typed alias exact source-TVB range '+k,actual,expected,n)
 for k,v in vs.items():
  if k not in selected:unprojected[k]+=len(v)
result={'scope':__doc__,'oracle_sha256':sha(oracle.read_bytes()),'events_sha256':sha(events.read_bytes()),'original_reference_frames':len(ref),'original_versions':dict(versions),'selected_complete_UDP_layout_frames':len(valid),'unselected_carrier_assessments':refused_carriers,'matched_frames':len(matched),'native_statuses':dict(statuses),'native_records':records,'original_reference_v5_record_views':sum(len(vs.get('cflow.srcaddr',[])) for _,vs,_,_ in ref.values() if vs.get('cflow.version')==['5']),'checks':dict(checks),'checks_total':sum(checks.values()),'selected_differences':diff,'additional_native':extras,'unmatched_valid_reference':sorted(valid-matched),'unselected_reference_frames':sorted(set(ref)-valid),'reference_fields_not_projected':dict(unprojected),'source_range_views':dict(range_views),'fragment_capture_sha256':sha(Path(sys.argv[4]).read_bytes()) if len(sys.argv)>4 else None}
# Counters may have tuple keys: normalize only diagnostic representation, never values used for comparison.
def diagnostic(x):
 if isinstance(x,dict):return {str(k):diagnostic(v) for k,v in x.items()}
 if isinstance(x,(list,tuple)):return [diagnostic(v) for v in x]
 return x
out.write_bytes((json.dumps(diagnostic(result),indent=2)+'\n').encode())
print(json.dumps({k:v for k,v in result.items() if k not in ['checks','selected_differences','additional_native','scope','reference_fields_not_projected']}))
print('selected differences',len(diff),'extra native',len(extras),'missing valid',len(valid-matched))
raise SystemExit(bool(diff or extras or valid-matched))