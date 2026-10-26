"""Independently reassemble owned HTTP/3 bidirectional streams from TShark's
authenticated QUIC STREAM bytes; compare ordered DATA delivery per connection,
direction and stream. This does not substitute HTTP/3 column counts or reuse the
native reassembler. Reference JSON must include quic.stream_data_raw (-T json -x).
"""
from pathlib import Path
import base64,collections,hashlib,json,sys
oracle,events,output=map(Path,sys.argv[1:4])
load=lambda p:json.loads(p.read_text(encoding='utf-8-sig'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
checks=0
def check(ok,why):
 global checks
 checks+=1
 if not ok:raise AssertionError(why)
def values(node,key):
 if isinstance(node,dict):return [x for k,v in node.items() for x in ((v if isinstance(v,list) else [v]) if k==key else values(v,key))]
 if isinstance(node,list):return [x for v in node for x in values(v,key)]
 return []
def vi(w,at):
 if at>=len(w):return None
 n=1<<(w[at]>>6)
 if at+n>len(w):return None
 return int.from_bytes(w[at:at+n],'big')&((1<<(n*8-2))-1),at+n
streams={};frame_connections={};first_endpoints={};reference_stream_frames=collections.Counter();stream_frames=0;duplicate_bytes=0
for packet in load(oracle):
 l=packet['_source']['layers'];number=int(l['frame']['frame.number'])
 ports=values(l.get('udp',{}),'udp.srcport'),values(l.get('udp',{}),'udp.dstport')
 check(len(ports[0])==len(ports[1])==1,f'frame {number}: selected single UDP view')
 quics=l.get('quic',[]);quics=quics if isinstance(quics,list) else [quics]
 for q in quics:
  conn=values(q,'quic.connection.number')
  if not conn:continue
  check(len(set(conn))==1,f'frame {number}: one QUIC connection per packet')
  cid=conn[0]
  first_endpoints.setdefault(cid,(ports[0][0],ports[1][0]))
  check(number not in frame_connections or frame_connections[number]==cid,f'frame {number}: mixed connection datagram')
  frame_connections[number]=cid
  frames=q.get('quic.frame',[]);frames=frames if isinstance(frames,list) else [frames]
  for f in frames:
   typ=int(f.get('quic.frame_type','0'),0)
   if not 8<=typ<=15:continue
   sid=int(f['quic.stream.stream_id'])
   if sid&2:continue # owned request/response streams; no control/push guessing
   start=int(f.get('quic.stream.offset','0'));raw=f.get('quic.stream_data_raw')
   body=bytes.fromhex(raw[0]) if raw else b''
   if 'quic.stream_data' in f:check(bytes.fromhex(f['quic.stream_data'].replace(':',''))==body,f'frame {number}: literal/reference stream bytes')
   check(start+len(body)<=1<<20,f'frame {number}: owned stream budget')
   key=(cid,ports[0][0],ports[1][0],sid);s=streams.setdefault(key,{'bytes':{},'fin':None,'packets':set()})
   reference_stream_frames[(number,*key,start,len(body),hashlib.sha256(body).hexdigest(),bool(typ&1))]+=1
   s['packets'].add(number);stream_frames+=1
   if typ&1:
    check(s['fin'] in [None,start+len(body)],f'{key}: conflicting FIN')
    s['fin']=start+len(body)
   for at,b in enumerate(body,start):
    if at in s['bytes']:check(s['bytes'][at]==b,f'{key}: conflicting overlap at {at}');duplicate_bytes+=1
    s['bytes'][at]=b
expected={};ledger=[]
for key,s in streams.items():
 end=max(s['bytes'],default=-1)+1
 check(all(i in s['bytes'] for i in range(end)),f'{key}: observed stream still has a byte hole')
 if s['fin'] is not None:check(s['fin']==end,f'{key}: FIN/source length')
 w=bytes(s['bytes'][i] for i in range(end));at=0;data=[];ranges=[]
 while at<len(w):
  first=at;v=vi(w,at);check(v is not None,f'{key}: incomplete HTTP/3 type');typ,at=v
  v=vi(w,at);check(v is not None,f'{key}: incomplete HTTP/3 length');n,at=v
  check(at+n<=len(w),f'{key}: incomplete HTTP/3 payload')
  if typ==0:data.append(hashlib.sha256(w[at:at+n]).hexdigest());ranges.append({'frame_offset':first,'payload_offset':at,'length':n,'sha256':data[-1]})
  at+=n
 expected[key]=data
 ledger.append({'connection':key[0],'source_port':key[1],'destination_port':key[2],'stream_id':key[3],'bytes':len(w),'FIN_observed':s['fin'] is not None,'contributing_frames':sorted(s['packets']),'DATA':ranges})
actual=collections.defaultdict(list);native_events=0;native_stream_frames=collections.Counter()
for line in events.open(encoding='utf-8-sig'):
 e=json.loads(line)
 for fr in (e.get('session') or {}).get('Frames',[]):
  if fr.get('Frame Type')=='STREAM' and not int(fr['Stream ID'])&2:
   refs=e['source_bytes'].get('PacketRefs') or []
   check(len(refs)==1 and refs[0]['Number'] in frame_connections,f"event {e['id']}: physical STREAM source binding")
   check(e['session'].get('Authentication Verified') is True,f"event {e['id']}: STREAM authenticated plaintext")
   b=base64.b64decode(fr.get('Stream Data') or '',validate=True)
   check(int(fr['Length'])==len(b),f"event {e['id']}: STREAM length")
   native_stream_frames[(refs[0]['Number'],frame_connections[refs[0]['Number']],e['source'].rsplit(':',1)[1],e['destination'].rsplit(':',1)[1],int(fr['Stream ID']),int(fr['Offset']),len(b),hashlib.sha256(b).hexdigest(),bool(fr['FIN']))]+=1
  for h in fr.get('HTTP3 Frames',[]):
   if h.get('Frame Type')!='DATA' or 'Payload' not in h:continue
   native_events+=1
   check(e['protocol']=='http3' and e['status'] in ['decoded','deferred'],f"event {e['id']}: DATA outcome")
   check(e['session'].get('Authentication Verified') is True,f"event {e['id']}: unverified plaintext")
   refs=e['source_bytes'].get('PacketRefs') or []
   check(bool(refs),f"event {e['id']}: missing capture connection binding")
   conns={frame_connections.get(r['Number']) for r in refs}
   check(len(conns)==1 and None not in conns,f"event {e['id']}: mixed/unobserved connection references")
   sp=e['source'].rsplit(':',1)[1];dp=e['destination'].rsplit(':',1)[1]
   key=(next(iter(conns)),sp,dp,int(fr['Stream ID']))
   check(e['direction']==(0 if (sp,dp)==first_endpoints[key[0]] else 1),f"event {e['id']}: observed direction")
   body=base64.b64decode(h['Payload'],validate=True)
   check(int(h['DATA Length'])==len(body),f"event {e['id']}: DATA declared length")
   actual[key].append(hashlib.sha256(body).hexdigest())
check(reference_stream_frames==native_stream_frames,'physical STREAM fields differ: source frame, connection, endpoints, stream ID, offset, length, bytes or FIN')
for key in expected.keys()|actual.keys():check(expected.get(key,[])==actual.get(key,[]),f'{key}: ordered DATA payloads differ, expected {len(expected.get(key,[]))}, native {len(actual.get(key,[]))}')
result={'oracle_sha256':sha(oracle),'events_sha256':sha(events),'checks':checks,'QUIC_STREAM_views':stream_frames,'native_QUIC_STREAM_views':sum(native_stream_frames.values()),'identical_overlap_bytes':duplicate_bytes,'bidirectional_stream_directions':len(streams),'connections':len({k[0] for k in streams}),'reference_stream_reconstructed_DATA':sum(map(len,expected.values())),'native_DATA':native_events,'selected_differences':[],'streams':ledger,'scope':'Owned HTTP/3 bidirectional streams only. Physical STREAM source, connection/endpoints, stream ID, offset, length, bytes and FIN verified against reference. Independently reconstructed from original TShark authenticated QUIC STREAM bytes, including gaps/overlap/FIN and ordered DATA by capture connection/direction/stream. This certifies observed payload reconstruction, not QPACK, packet-column agreement, stream-field highlight ranges or general QUIC completeness.'}
output.write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
print(json.dumps({k:v for k,v in result.items() if k!='streams'},indent=2))
