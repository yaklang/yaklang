import collections, copy, csv, hashlib, json, struct
from pathlib import Path
import compare

root=compare.root
raw=(compare.capture/'live.pcap').read_bytes()
assert raw[:4]==bytes.fromhex('4d3cb2a1')
frames=[];at=24
while at<len(raw):
    sec,ns,n,wire=struct.unpack_from('<IIII',raw,at);at+=16
    frames.append({'original':len(frames)+1,'sec':sec,'ns':ns,'wire':wire,'data':raw[at:at+n]});at+=n
assert at==len(raw)

def packet(f):
    b=f['data'];assert b[:4]==b'\x02\0\0\0' and b[4]>>4==4
    ip=4;tr=ip+4*(b[ip]&15);proto=b[ip+9]
    sp,dp=struct.unpack_from('!HH',b,tr)
    off=tr+(4*(b[tr+12]>>4) if proto==6 else 8)
    return dict(proto=proto,sp=sp,dp=dp,off=off,payload=b[off:],flags=b[tr+13] if proto==6 else 0)

ports=compare.manifest['Ports']
grpc=[i for i,f in enumerate(frames) if (p:=packet(f))['proto']==6 and p['dp']==ports['grpc'] and len(p['payload'])>1000]
assert len(grpc)>=3
target=grpc[-2]
qshort=[i for i,f in enumerate(frames) if (p:=packet(f))['proto']==17 and (p['sp']==ports['quic_client'] or p['dp']==ports['quic_client']) and p['payload'] and not p['payload'][0]&128 and len(p['payload'])>500]
assert len(qshort)>3
tlsclient=next(i for i,f in enumerate(frames) if (p:=packet(f))['proto']==6 and p['dp']==ports['tls772'] and p['payload'][:1]==b'\x16')
tlssport=packet(frames[tlsclient])['sp']

def save(name,fs,notes):
    out=root/name;out.mkdir(exist_ok=True)
    data=bytearray(raw[:24])
    for f in fs:
        data+=struct.pack('<IIII',f['sec'],f['ns'],len(f['data']),f['wire'])+f['data']
    (out/'live.pcap').write_bytes(data)
    (out/'provenance.json').write_text(json.dumps({'source_sha256':hashlib.sha256(raw).hexdigest(),'notes':notes,'original_frames':[f['original'] for f in fs]},indent=2))
    return out/'live.pcap',out

variants={}
def variant(name,fs,notes): variants[name]=save(name,fs,notes)

fs=copy.deepcopy(frames);a,b=grpc[-3],grpc[-2];fs[a],fs[b]=fs[b],fs[a]
variant('tcp-reorder',fs,{'swapped':[a+1,b+1],'timestamps':'original, intentionally not monotonic'})
variant('tcp-loss',[f for i,f in enumerate(frames) if i!=target],{'dropped':target+1})
fs=copy.deepcopy(frames);fs.insert(target+3,copy.deepcopy(frames[target]))
variant('tcp-duplicate',fs,{'duplicated':target+1,'inserted_after_original':target+3})
fs=copy.deepcopy(frames);f=fs[target];f['data']=f['data'][:packet(f)['off']+17]
snap=copy.deepcopy(fs);snap.insert(target+3,copy.deepcopy(frames[target]));variant('tcp-snaplen-recovered',snap,{'truncated':target+1,'retransmission_after':target+3})
variant('tcp-snaplen',fs,{'truncated':target+1,'captured_payload':17,'wire_length':'unchanged'})
fs=copy.deepcopy(frames);f=fs[target];b=bytearray(f['data']);b[packet(f)['off']+30]^=1;f['data']=bytes(b)
variant('tls-auth-failure',fs,{'ciphertext_bit_flip':target+1,'offset_in_tcp_payload':30,'transport_checksum':'Npcap loopback, validation disabled equally'})
variant('tls-missing-clienthello',[f for i,f in enumerate(frames) if i!=tlsclient],{'removed_clienthello':tlsclient+1,'client_port':tlssport})
fs=copy.deepcopy(frames);a,b=qshort[4],qshort[6];fs[a],fs[b]=fs[b],fs[a]
variant('quic-reorder',fs,{'swapped':[a+1,b+1]})
variant('quic-loss',[f for i,f in enumerate(frames) if i!=qshort[5]],{'dropped':qshort[5]+1})
fs=copy.deepcopy(frames);f=fs[qshort[5]];f['data']=f['data'][:packet(f)['off']+19]
variant('quic-snaplen',fs,{'truncated':qshort[5]+1,'captured_payload':19})
fs=copy.deepcopy(frames);f=fs[qshort[5]];b=bytearray(f['data']);b[packet(f)['off']+1]^=0x80;f['data']=bytes(b)
variant('quic-unknown-cid',fs,{'modified_short_dcid':qshort[5]+1})
removed=[i for i,f in enumerate(frames) if (p:=packet(f))['proto']==17 and ports['quic_client'] in (p['sp'],p['dp']) and p['payload'][0]&128 and (p['payload'][0]>>4)&3==0]
variant('quic-no-initial',[f for i,f in enumerate(frames) if i not in removed],{'removed_initial_datagrams':[i+1 for i in removed]})
syns=[i for i,f in enumerate(frames) if (p:=packet(f))['proto']==6 and ports['reuse_mux'] in (p['sp'],p['dp']) and p['flags']&2]
assert len(syns)==4,syns
variant('reuse-missing-second-syn',[f for i,f in enumerate(frames) if i not in syns[2:]],{'removed_second_generation_handshake':[i+1 for i in syns[2:]]})

if __name__=='__main__':
    aggregate={}
    baseline,_,_,_=compare.analyze(compare.capture/'live.pcap',root/'baseline')
    assert baseline['grpc_bytes_equal'] and baseline['h3_paths_equal'] and baseline['h3_yak_application_replies_verified']
    # Some Wireshark builds omit DATA when a QUIC STREAM arrives out of order.
    # Check the independent live application's reply hashes as a third oracle;
    # preserve the discrepancy in summary.json instead of forcing a match.
    assert not collections.Counter(baseline['h3_ws_hashes'])-collections.Counter(baseline['h3_yak_hashes'])
    oracle=sorted(o['ValueHash'] for o in compare.manifest['Operations'] if o['Protocol'] in ('grpc-request','grpc-response'))
    assert baseline['grpc_yak_hashes']==oracle,'fresh application oracle differs from wire decoding'
    for name,(cap,out) in variants.items():
        try:
            s,ws,ys,events=compare.analyze(cap,out)
            assert len(ws)==len(ys),'capture inspection stopped early'
            assert not s['authenticated_plaintext_without_verified_tag']
            # Missing evidence can reduce decoding, never create novel bytes.
            assert not collections.Counter(s['grpc_yak_hashes'])-collections.Counter(oracle)
            assert not collections.Counter(s['h3_yak_hashes'])-collections.Counter(baseline['h3_yak_hashes'])
            assert not any(e['protocol']=='coap' for e in events),'protected QUIC falsely admitted as CoAP'
            if name in ('tcp-reorder','tcp-duplicate','tcp-snaplen-recovered'):
                assert s['grpc_yak_hashes']==oracle
            if name in ('tcp-loss','tcp-snaplen'):
                assert s['grpc_bytes_equal'],'intact reverse-direction gRPC must survive missing peer control frames'
            if name=='quic-no-initial':assert not s['h3_yak_paths'] and not s['h3_yak_hashes']
            if name=='reuse-missing-second-syn':
                original=json.loads((out/'provenance.json').read_text())['original_frames']
                assert not any(y.get('connection_protocol') for y,n in zip(ys,original) if n>syns[2]+1),'reset connection identity leaked to unobserved generation'
            compact={k:v for k,v in s.items() if 'hashes' not in k and 'paths' not in k and k!='http_summaries'}
            compact['h3_paths']=[len(s['h3_ws_paths']),len(s['h3_yak_paths']),s['h3_paths_equal']]
            aggregate[name]=compact
            print(name, 'frames=',compact['frames'], 'grpc=',compact['grpc'], 'h3=',compact['h3_paths'], 'diagnostics=',len(compact['errors']),flush=True)
        except Exception as e:
            aggregate[name]={'failed':str(e)};print(name,'FAILED',str(e),flush=True)
    (root/'fault-summary.json').write_text(json.dumps(aggregate,indent=2),encoding='utf8')
    assert not any('failed' in r for r in aggregate.values()),'see fault-summary.json'
