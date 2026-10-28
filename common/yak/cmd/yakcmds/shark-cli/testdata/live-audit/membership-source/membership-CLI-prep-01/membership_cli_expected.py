"""Independent source/code projection for the151 IGMP+237 MLD public rows.

Construct every output member from literal source, frozen whole native oracle,
and separately source-verified publisher IDs. No new CLI runtime is consumed.
The old row supplies only the UTC spelling of an independently checked instant.
"""
import copy,datetime,hashlib,ipaddress,json,re,unicodedata,zlib

def same(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
 return a==b

def safe(s):return ''.join(' 'if unicodedata.category(c)in('Cc','Cf')else c for c in s)
def go_ip(value):
 a=ipaddress.ip_address(value)
 return str(a.ipv4_mapped)if isinstance(a,ipaddress.IPv6Address)and a.ipv4_mapped else str(a)

def clock_ns(text):
 m=re.fullmatch(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?Z',text);assert m,'this public scope has UTC RFC3339 timestamps'
 d=datetime.datetime.strptime(m[1],'%Y-%m-%dT%H:%M:%S')-datetime.datetime(1970,1,1)
 return(d.days*86400+d.seconds)*10**9+int((m[2]or'').ljust(9,'0')or'0')

def physical_prefix(record,location):
 w=record['wire'];names=[];namespace='';peers=[];outer=''
 for h in location['headers']:
  kind=h['kind'];at=h['offset'];length=h['length'];assert type(at)is int and type(length)is int and 0<=at<=at+length<=len(w)
  if kind=='FPP':
   assert record['link']==274;start=1 if w and w[0]==0x50 else 0
   while start<len(w)and w[start]==0x55:start+=1;assert start<=64
   assert w[start]==0xd5 and start+1==at+length and len(w)-start-1>=18
   assert zlib.crc32(w[start+1:-4])==int.from_bytes(w[-4:],'little')
   names.append('FPP');namespace+='/mpacket:express'
  elif kind=='Ethernet':assert length==14;names.append('Ethernet')
  elif kind=='IPv4':
   assert w[at]>>4==4 and length==(w[at]&15)*4 and length>=20
   a,b=go_ip(w[at+12:at+16]),go_ip(w[at+16:at+20]);assert a==h['source']and b==h['destination'];names.append('IPv4');peers.append((a,b));outer='~'.join(sorted((a,b)))
   assert w[at+9]not in(4,41),'this public membership scope has no IP-in-IP source'
  elif kind=='IPv6':
   assert w[at]>>4==6 and length==40
   a,b=go_ip(w[at+8:at+24]),go_ip(w[at+24:at+40]);names.append('IPv6');peers.append((a,b));outer='~'.join(sorted((a,b)))
   assert w[at+6]not in(4,41),'this public membership scope has no IP-in-IP source'
  elif kind=='IPv6 extension':
   assert h['type']==0 and length==(w[at+1]+1)*8;names.append('IPv6HopByHop')
  elif kind=='GREv0':
   assert h['flags']==0 and h['key']is None and length==4 and w[at:at+4]==bytes.fromhex('000086dd');names.append('GRE');namespace+='/gre:'+outer+':0'
  else:raise AssertionError('unplanned public membership carrier '+kind)
 assert peers
 return names,dict(Section=record['section'],Interface=record['interface'],Encapsulation=namespace),peers[-1]

def legacy_membership_layer(w,family):
 """Literal dependency v1.3.1 behavior, not protocol admission or correctness."""
 if family=='igmp':
  if not w:return None,False # eager NextDecoder does not call a decoder on0B.
  t=w[0]
  if t==0x11:
   if len(w)==8 or len(w)>=12:return 'IGMP',False
   return None,True
  if t==0x22:return 'IGMP',False # dependency drops its v3 body parse error.
  assert t in(0x12,0x16,0x17)
  return('IGMP',False)if len(w)>=8 else(None,True)
 assert family=='mld'and len(w)>=4;body=w[4:];t=w[0]
 if not body:return None,False
 if t==130:
  if len(body)<=20:return('MLDv1MulticastListenerQuery',False)if len(body)>=20 else(None,True)
  if len(body)<24:return None,True
  count=int.from_bytes(body[22:24],'big')
  return('MLDv2MulticastListenerQuery',False)if len(body)>=24+16*count else(None,True)
 if t in(131,132):return(('MLDv1MulticastListenerReport'if t==131 else'MLDv1MulticastListenerDone'),False)if len(body)>=20 else(None,True)
 assert t==143
 if len(body)<4:return None,True
 at=4
 for _ in range(int.from_bytes(body[2:4],'big')):
  if at+20>len(body):return None,True
  count=int.from_bytes(body[at+2:at+4],'big');size=20+16*count+4*body[at+1]
  if at+size>len(body):return None,True
  at+=size
 return 'MLDv2MulticastListenerReport',False

def compact_pdu(event):
 fields={}
 for value in event['field_values']['values']:fields.setdefault(value['name'],[]).append(copy.deepcopy(value['value']))
 p=dict(id=event['id'],protocol=event['protocol'],status=event['status'],completeness=event['completeness'],response_to=event['response_to'],byte_source=copy.deepcopy(event['source_bytes']),fields=fields,field_values=copy.deepcopy(event['field_values']))
 if event.get('error')or event['expert']:p.update(summary=event['summary'],error=event.get('error',''),expert=event['expert'])
 assert not event.get('tcp_flow_id')and event['flow']==0
 # Conservative6x escape bound proves this single observation fits256KiB-1KiB.
 assert 6*len(json.dumps(p,ensure_ascii=False,separators=(',',':')).encode('utf8'))<256*1024-1024
 return p

def protocol_label(event,family):
 key='IGMP'if family=='igmp'else'MLD';version=event['session'][key].get('Version')
 return key+'v'+str(version)if type(version)is int and version in((1,2,3)if family=='igmp'else(1,2))else key

def event_info(event,family):
 key='IGMP'if family=='igmp'else'MLD';m=event['session'][key];parts=[safe(event['summary'])]
 records,count=('Group Records','Group Record Count')if family=='igmp'else('Address Records','Address Record Count')
 if count in m:
  assert type(m[count])is int;parts.append('records='+str(m[count]))
  rs=m.get(records)or[]
  if rs and isinstance(rs[0].get('Multicast Address'),str)and rs[0]['Multicast Address']:parts.append('first group='+safe(rs[0]['Multicast Address']))
 for name in('Group Address','Multicast Address'):
  if isinstance(m.get(name),str)and m[name]:parts.append('group='+safe(m[name]))
 if 'Source Count'in m:assert type(m['Source Count'])is int;parts.append('sources='+str(m['Source Count']))
 result=' \u00b7 '.join(parts)
 if event['status']=='decoded'and len(result)>384:result=result[:384]+'\u2026'
 return result

def expected_row(oldrow,record,location,event,family):
 assert type(record['frame'])is int and oldrow['number']==record['frame']and clock_ns(oldrow['time'])==record['time_ns']
 assert event['protocol']==('igmp'if family=='igmp'else'icmpv6')and event['profile']==('igmp-membership-wire'if family=='igmp'else'icmpv6-mld')
 assert event['status']in('decoded','incomplete','malformed','limited')and event['completeness']==('message'if event['status']=='decoded'else event['status'])
 prefix,domain,peers=physical_prefix(record,location);assert same(event['domain'],domain)
 assert event['carrier_layers']is None and event['transport']=='network'and event['flow']==0
 assert peers==(event['source'],event['destination'])
 own=record['wire'][location['offset']:location['end']];legacy,failed=legacy_membership_layer(own,family)
 physical=prefix+(['ICMPv6']if family=='mld'else[])+([legacy]if legacy else[])
 assert same(oldrow['layers'],physical),'unchanged literal dependency layer inventory differs'
 assert oldrow['source']==peers[0]and oldrow['destination']==peers[1]and same(oldrow['capture_domain'],domain)
 assert oldrow['length']==record['original_length']and oldrow['captured_length']==len(record['wire'])and oldrow['pdu_scope']=='observed_at_packet'
 assert not oldrow.get('published_pdus')and not any(k in oldrow for k in('transport','source_port','destination_port','message_source','message_destination','message_transport','message_domain','physical_source','physical_destination','connection_protocol','stream_id','tcp_flow_id','transport_view'))
 info=event_info(event,family);status=event['status'];decoded=status=='decoded';label=protocol_label(event,family)
 if failed:info+=' [incomplete/undecoded]'
 if record['original_length']>len(record['wire']):info+=' [truncated capture: %d/%d bytes]'%(len(record['wire']),record['original_length']);status='incomplete'
 return dict(capture_domain=domain,pdus=[compact_pdu(event)],pdu_scope='observed_at_packet',protocol_stack=prefix+(['ICMPv6']if family=='mld'else[])+[label],visibility='captured'if decoded else'partial-message',protocol_status=status,protocol_scope='captured membership wire fields'if decoded else'captured multicast membership fields; complete message not admitted',protocol_evidence='bounded message fields; checksum, parent validity and membership state unverified'if decoded else'wire prefix only; checksum, parent validity and membership state unverified',layers=physical,number=record['frame'],time=oldrow['time'],source=peers[0],destination=peers[1],protocol=label,length=record['original_length'],captured_length=len(record['wire']),info=info)
