"""Review every original 94/39 field difference without normalizing it away.

Inputs, DNS oracle JSON and detailed proofs must stay outside Git. The final
review JSON contains only original frame numbers, lengths, hashes and reasons.
TLS byte identities require the reference frame's wire-bound inner IP/transport
headers, section/interface and native contributors. Delayed views additionally
require a spanning sibling in the same native flow/direction/domain. Numeric
flow IDs are not compared across dissectors; whole-export flow/association
invariants remain a separate check.
"""
import argparse, base64, collections, hashlib, ipaddress, json, subprocess, unicodedata
from pathlib import Path

p=argparse.ArgumentParser(description=__doc__)
p.add_argument("--baseline",type=Path,required=True)
p.add_argument("--weber-events",type=Path,required=True)
p.add_argument("--sharon-events",type=Path,required=True)
p.add_argument("--weber-capture",type=Path,required=True)
p.add_argument("--sharon-capture",type=Path,required=True)
p.add_argument("--tshark",type=Path,required=True)
p.add_argument("--out",type=Path,required=True)
p.add_argument("--original-review",type=Path,help="Retain the complete original reviewed case inventory, including already repaired DNS rows")
p.add_argument("--weber-tls-oracle",type=Path,help="Fresh complete TLS JSON oracle; otherwise the baseline oracle is explicitly reused")
p.add_argument("--sharon-tls-oracle",type=Path,help="Reuse a separately exported complete TLS JSON oracle instead of exporting it again")
a=p.parse_args()
for path in [a.baseline,a.weber_events,a.sharon_events,a.weber_capture,a.sharon_capture,a.out]:
    if any((d/".git").exists() for d in [path.resolve(),*path.resolve().parents]):raise SystemExit("Raw audit data must stay outside Git")
for path in [a.weber_tls_oracle,a.sharon_tls_oracle]:
    if path and any((d/".git").exists()for d in[path.resolve(),*path.resolve().parents]):raise SystemExit("Raw reference oracles must stay outside Git")
a.out.mkdir(parents=True,exist_ok=True)
sha=lambda w:hashlib.sha256(w).hexdigest()
def read(path):return json.loads(path.read_text(encoding="utf-8-sig"))
def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def refs(e):return [r["Number"] for r in e["source_bytes"].get("PacketRefs") or []]
def scope(e):return (e["flow"],e["direction"],json.dumps(e["domain"],sort_keys=True))
def raw_rows(v):return [v] if v and isinstance(v[0],str) else v or []

def field_strings(tree,key):
    if isinstance(tree,dict):
        for k,v in tree.items():
            if k==key:yield from v if isinstance(v,list)else[v]
            elif not k.endswith('_raw'):yield from field_strings(v,key)
    elif isinstance(tree,list):
        for v in tree:yield from field_strings(v,key)

def raw_nodes(tree,key):
    if isinstance(tree,dict):
        for k,v in tree.items():
            if k==key:yield from raw_rows(v)
            elif not k.endswith('_raw'):yield from raw_nodes(v,key)
    elif isinstance(tree,list):
        for v in tree:yield from raw_nodes(v,key)

def scope_endpoints(layers,wire):
    networks=[];endpoints=[]
    def physical_nodes(key):
        for r in raw_nodes(layers,key):
            b=bytes.fromhex(r[0]);at=int(r[1])
            # Virtual reassembly headers must not become physical evidence.
            if at>=0 and wire[at:at+len(b)]==b:yield at,b
    for at,b in physical_nodes('ip_raw'):
        if len(b)<20 or b[0]>>4!=4:continue
        ihl=(b[0]&15)*4;end=at+int.from_bytes(b[2:4],'big')
        if ihl<20 or len(b)<ihl or int.from_bytes(b[6:8],'big')&8191:continue
        networks.append((at,at+ihl,end,b[9],str(ipaddress.ip_address(b[12:16])),str(ipaddress.ip_address(b[16:20]))))
    for at,b in physical_nodes('ipv6_raw'):
        if len(b)<40 or b[0]>>4!=6:continue
        networks.append((at,at+40,at+40+int.from_bytes(b[4:6],'big'),b[6],str(ipaddress.ip_address(b[8:24])),str(ipaddress.ip_address(b[24:40]))))
    for name,protocol,minimum in[('tcp',6,20),('udp',17,8)]:
        for at,b in physical_nodes(name+'_raw'):
            if len(b)<minimum:continue
            owners=[n for n in networks if n[1]==at and n[2]>=at+minimum and n[3]==protocol]
            if len(owners)!=1:continue
            n=owners[0];endpoints.append((name,n[4],int.from_bytes(b[:2],'big'),n[5],int.from_bytes(b[2:4],'big')))
    return endpoints

def oracle_scope(layers,reassembled=()):
    frame=layers['frame'];endpoints=scope_endpoints(layers,bytes.fromhex(layers['frame_raw'][0]))
    for wire in reassembled:endpoints.extend(scope_endpoints(layers,wire))
    return {'section':int(frame.get('frame.section_number','1'))-1,'interface':int(frame.get('frame.interface_id','0')),'endpoints':endpoints}

def outer_ipv6_reassemblies(packets):
    groups=collections.defaultdict(list)
    for packet in packets:
        l=packet['_source']['layers'];f=l['frame'];frame=int(f['frame.number']);wire=bytes.fromhex(l['frame_raw'][0])
        for row in raw_nodes(l,'ipv6_raw'):
            b=bytes.fromhex(row[0]);at=int(row[1])
            if at<0 or len(b)<48 or wire[at:at+len(b)]!=b or b[0]>>4!=6 or b[6]!=44:continue
            # This supplemental oracle is the observed direct Fragment-header
            # carrier. No guesses for extension chains, jumbograms or holes.
            end=at+40+int.from_bytes(b[4:6],'big');flags=int.from_bytes(b[42:44],'big')
            assert end<=len(wire)and end>=at+48 and b[41]==0 and flags&6==0,'bounded original IPv6 fragment header'
            offset=(flags>>3)*8;more=bool(flags&1);data=wire[at+48:end]
            assert offset+len(data)<=65535 and(not more or len(data)%8==0),'bounded aligned IPv6 fragment payload'
            key=(int(f.get('frame.section_number','1'))-1,int(f.get('frame.interface_id','0')),b[8:40],b[40],int.from_bytes(b[44:48],'big'))
            groups[key].append((frame,offset,more,data))
    buffers=collections.defaultdict(list);proofs=[]
    for key,fragments in groups.items():
        ends={at+len(data)for _,at,more,data in fragments if not more}
        if not ends:continue
        assert len(ends)==1,'ambiguous final length in selected IPv6 fragment group'
        end=ends.pop();data=bytearray(end);covered=bytearray(end)
        for frame,at,more,part in fragments:
            assert at+len(part)<=end,'fragment crosses the observed terminal boundary'
            for i,b in enumerate(part,at):
                assert not covered[i]or data[i]==b,'conflicting original IPv6 fragments are not merged'
                data[i]=b;covered[i]=1
        if not end or not all(covered):continue
        complete=bytes(data);frames=sorted({r[0]for r in fragments})
        for frame in frames:buffers[frame].append(complete)
        proofs.append({'section':key[0],'interface':key[1],'next_header':key[3],'fragment_id':key[4],'source_frames':frames,'complete_payload_length':end,'complete_payload_sha256':sha(complete)})
    return buffers,proofs

def reference_scope_matches(e,frame):
    v=ws_scopes.get(frame)
    if not v:return False
    domain=e['domain']
    # Require the actual contiguous IP -> TCP/UDP header pair. Nested tunnel
    # addresses cannot be cross-joined with an unrelated transport's ports.
    src,sp=e['source'].rsplit(':',1);dst,dp=e['destination'].rsplit(':',1)
    expected=(e['transport'],str(ipaddress.ip_address(src.strip('[]'))),int(sp),str(ipaddress.ip_address(dst.strip('[]'))),int(dp))
    return(domain['Section'],domain['Interface'])==(v['section'],v['interface'])and expected in v['endpoints']
def tls_records(v):
    if isinstance(v,dict):
        for k,x in v.items():
            if k=="tls.record_raw":
                for r in raw_rows(x):
                    w=bytes.fromhex(r[0])
                    if len(w)>=5 and int.from_bytes(w[3:5],'big')==len(w)-5:yield w
            elif not k.endswith('_raw'):yield from tls_records(x)
    elif isinstance(v,list):
        for x in v:yield from tls_records(x)
def expanded_name(w,at):
    out=bytearray();seen=set();length=1
    while True:
        if at in seen or not 0<=at<len(w):raise ValueError('DNS name pointer loop/boundary')
        seen.add(at);n=w[at];at+=1
        if n==0:return bytes(out+b'\0')
        if n&0xc0==0xc0:
            if at>=len(w):raise ValueError('truncated DNS pointer')
            at=((n&63)<<8)|w[at];continue
        if n>63 or at+n>len(w):raise ValueError('DNS label boundary')
        length+=1+n
        if length>255:raise ValueError('DNS expanded name over 255 octets')
        out.append(n);out.extend(w[at:at+n]);at+=n

original_review=read(a.original_review) if a.original_review else None
oracle_path=a.weber_tls_oracle or a.baseline/'tls-final'/'tls-records-oracle.json'
oracle=read(oracle_path)
weber_oracle_path=oracle_path
ws_by_frame=collections.defaultdict(list);ws_by_hash=collections.defaultdict(set);ws_scopes={}
for packet in oracle:
    layers=packet['_source']['layers'];frame=int(layers['frame']['frame.number']);ws_scopes[frame]=oracle_scope(layers)
    for w in tls_records(layers):
        item={'sha256':sha(w),'length':len(w)-5};ws_by_frame[frame].append(item);ws_by_hash[item['sha256']].add(frame)
del oracle
derived_frames=collections.defaultdict(set)
provenance={v['derived_frame']:v['original_frame'] for v in read(a.baseline/'rdp-final'/'derived-provenance.json')}
derived_summary=read(a.baseline/'rdp-final'/'summary.json')
assert not derived_summary['missing'] and not derived_summary['extra']
assert digest(a.baseline/'rdp-final'/'DERIVED-rdpudp-ordered-tls.pcap')==derived_summary['derived_capture_sha256']
for packet in read(a.baseline/'rdp-final'/'derived-tls.json'):
    l=packet['_source']['layers'];original=provenance[int(l['frame']['frame.number'])]
    for w in tls_records(l):derived_frames[sha(w)].add(original)
reviews={}
for name,path,capture in [('weber',a.weber_events,a.weber_capture),('sharon',a.sharon_events,a.sharon_capture)]:
    if original_review:
        assert digest(capture)==original_review[name]['capture_sha256'],'original case inventory belongs to this exact capture'
        baseline=[dict(frame=r['frame'],field=r['field'],wireshark=r.get('original_wireshark_lengths'),yak=r.get('original_contributor_union_lengths'))for r in original_review[name]['rows']]
    else:baseline=read(a.baseline/(name+'-final')/'field-mismatches.json')
    wanted={d['frame'] for d in baseline};events=[];tls=[];frame_events=collections.defaultdict(list);by_hash=collections.defaultdict(list)
    for line in path.open(encoding='utf-8-sig'):
        if not line.startswith('{'):continue
        e=json.loads(line)
        if e['protocol']=='tls' and e['status']=='decoded':
            w=base64.b64decode(e['raw']);e['_hash']=sha(w);e['_length']=len(w)-5
            assert int.from_bytes(w[3:5],'big')==e['_length']==e['session']['Record Length']
            assert [v['value'] for v in e['field_values']['values'] if v['name']=='tls.record.length']==[e['_length']]
            if refs(e):
                assert e['source_bytes'].get('CompletedIn',{}).get('Number')==max(refs(e)), 'new explicit completion evidence required'
            tls.append(e);by_hash[e['_hash']].append(e)
        if any(n in wanted for n in refs(e)):events.append(e)
    for e in events:
        for n in refs(e):frame_events[n].append(e)
    # Weber's complete JSON oracle preserves original frame numbering. Sharon
    # has a different ordering: export its complete TLS views independently.
    if name=='sharon':
        oracle_path=a.sharon_tls_oracle or a.out/'sharon-tls-oracle.json'
        if not a.sharon_tls_oracle:
            with oracle_path.open('wb') as out,(a.out/'sharon-tls-oracle.stderr').open('wb') as err:
                subprocess.run([str(a.tshark),'-2','-n','-r',str(capture),'-o','tcp.reassemble_out_of_order:TRUE','-Y','tls.record','-T','json','-x','--no-duplicate-keys'],stdout=out,stderr=err,check=True,timeout=600)
        ws_by_frame=collections.defaultdict(list);ws_by_hash=collections.defaultdict(set);ws_scopes={}
        for packet in read(oracle_path):
            l=packet['_source']['layers'];frame=int(l['frame']['frame.number']);ws_scopes[frame]=oracle_scope(l)
            for w in tls_records(l):ws_by_frame[frame].append({'sha256':sha(w),'length':len(w)-5});ws_by_hash[sha(w)].add(frame)
    # RDPUDP can retain the physical carrier while its original TLS view is
    # absent. Export the original physical frames separately; a DERIVED TCP
    # oracle must never supply the source network endpoints or capture domain.
    scope_frames=set(wanted)
    for e in events:scope_frames.update(refs(e))
    if original_review:
        for case in original_review[name]['rows']:
            for record in case.get('records',[]):
                scope_frames.update(record['contributors']);scope_frames.update(record.get('wireshark_frames',[]));scope_frames.update(record.get('derived_oracle_original_frames',[]))
    scope_path=a.out/(name+'-physical-scope-oracle.json')
    with scope_path.open('wb')as f,scope_path.with_suffix('.stderr').open('wb')as er:
        subprocess.run([str(a.tshark),'-2','-n','-r',str(capture),'-Y','frame.number in {'+','.join(map(str,sorted(scope_frames)))+'}','-T','json','-x','--no-duplicate-keys'],stdout=f,stderr=er,check=True,timeout=600)
    scope_packets=read(scope_path);reassembled_scopes,reassembly_proofs=outer_ipv6_reassemblies(scope_packets);observed_scope_frames=set()
    for packet in scope_packets:
        layers=packet['_source']['layers'];frame=int(layers['frame']['frame.number']);observed_scope_frames.add(frame);ws_scopes[frame]=oracle_scope(layers,reassembled_scopes[frame])
    assert observed_scope_frames==scope_frames,'every requested original contributor/reference frame retains independent physical scope evidence'
    def record_proof(e):
        candidates=sorted(f for f in ws_by_hash[e['_hash']] if reference_scope_matches(e,f))
        direct=[f for f in candidates if f in refs(e)]
        if direct:return {'pdu':e['id'],'sha256':e['_hash'],'length':e['_length'],'contributors':refs(e),'byte_complete_frame':max(refs(e)),'wireshark_frames':direct,'proof':'byte-identical complete record in same flow/direction/domain'}
        if name=='weber' and any(v in ('RDPUDP','RDPUDP2') for v in e.get('carrier_layers') or []):
            candidates=sorted(f for f in derived_frames[e['_hash']] if f in refs(e) and reference_scope_matches(e,f))
            if candidates:return {'pdu':e['id'],'sha256':e['_hash'],'length':e['_length'],'contributors':refs(e),'byte_complete_frame':max(refs(e)),'derived_oracle_original_frames':candidates,'derived_capture_sha256':derived_summary['derived_capture_sha256'],'proof':'independent derived RDPUDP TLS record; original oracle carrier lost TLS dissection'}
        # A delayed batch view must have a native sibling which both shares
        # the completed record's packet and spans to that exact oracle frame.
        for frame in candidates:
            hashes={r['sha256'] for r in ws_by_frame[frame]}
            siblings=[v for v in tls if scope(v)==scope(e) and v['_hash'] in hashes and refs(v) and max(refs(v))==frame and set(refs(v))&set(refs(e))]
            if siblings:return {'pdu':e['id'],'sha256':e['_hash'],'length':e['_length'],'contributors':refs(e),'byte_complete_frame':max(refs(e)),'wireshark_frames':[frame],'spanning_sibling_pdus':[v['id'] for v in siblings],'proof':'Wireshark batch deferral, proved by byte-identical spanning sibling in same flow/direction/domain'}
        raise AssertionError(('no constrained TLS byte proof',name,e['id'],refs(e),e['_hash']))
    dns_frames=[d['frame'] for d in baseline if d['field']=='dns.qry.name'];dns_layers={}
    if dns_frames:
        out=a.out/(name+'-dns-oracle.json')
        with out.open('wb') as f,(a.out/(name+'-dns-oracle.stderr')).open('wb') as er:
            subprocess.run([str(a.tshark),'-n','-r',str(capture),'-c',str(max(dns_frames)),'-Y','frame.number in {'+','.join(map(str,dns_frames))+'}','-T','json','-x','--no-duplicate-keys'],stdout=f,stderr=er,check=True,timeout=600)
        for packet in read(out):dns_layers[int(packet['_source']['layers']['frame']['frame.number'])]=packet['_source']['layers']
    def name_labels(w):
        result=[];at=0
        while at<len(w):
            n=w[at];at+=1
            if n==0:
                assert at==len(w),'expanded name ends exactly at the root label'
                return result
            assert 1<=n<=63 and at+n<=len(w),'expanded label bounds'
            result.append(w[at:at+n]);at+=n
        raise AssertionError('expanded name lacks a root label')
    def escaped_label(label):
        return ''.join('\\'+chr(b)if b in(46,92)else chr(b)if 33<=b<=126 else'\\%03d'%b for b in label)
    def readable_label(label):
        out=[]
        for r in label.decode('utf8','surrogateescape'):
            if 0xdc80<=ord(r)<=0xdcff:out.append('\\%03d'%(ord(r)-0xdc00))
            elif unicodedata.category(r)[0]in 'LMNPS' or unicodedata.category(r)=='Zs':out.append(('\\'if r in '.\\'else'')+r)
            else:out.extend('\\%03d'%b for b in r.encode('utf8'))
        return ''.join(out)
    dns_by_id={e['id']:e for e in events if e['protocol']in('dns','mdns','llmnr')and e['status']=='decoded'}
    def identity(e):
        dns=e['session']['DNS']
        return (dns['ID'],dns['Opcode'],[(base64.b64decode(q['Name Wire']).lower(),q['Type'],q['Class'])for q in dns['Questions']])
    result=[]
    for d in baseline:
        frame=d['frame'];row={'frame':frame,'field':d['field']}
        if d['field']=='tls.record.length':
            associated=[e for e in frame_events[frame] if e['protocol']=='tls' and e['status']=='decoded']
            proofs=[record_proof(e) for e in associated]
            missing=set(d['wireshark'])-set(e['_length'] for e in associated)
            for v in ws_by_frame[frame]:
                if v['length'] not in missing:continue
                choices=[e for e in by_hash[v['sha256']] if reference_scope_matches(e,frame)]
                assert len(choices)==1,(frame,v,'ambiguous omitted native record')
                proofs.append(record_proof(choices[0]));missing.discard(v['length'])
            assert not missing,(name,frame,missing)
            kind='contributor/frame attribution with exact record bytes'
            if any('derived_oracle_original_frames' in r for r in proofs):kind='contributor attribution plus original RDPUDP oracle limitation; independent derived byte proof'
            row.update({'classification':kind,'original_wireshark_lengths':d['wireshark'],'original_contributor_union_lengths':d['yak'],'records':proofs,'parser_value_error':False})
        else:
            ns=[e for e in frame_events[frame] if e['protocol'] in ('dns','mdns','llmnr') and e['status']=='decoded'];assert len(ns)==1
            e=ns[0];w=base64.b64decode(e['raw']);w=w[2:] if e['transport']=='tcp' else w
            assert reference_scope_matches(e,frame),'DNS endpoints and capture section/interface match literal original IP/transport headers'
            assert w in bytes.fromhex(dns_layers[frame]['frame_raw'][0]),'native DNS bytes absent from original physical frame'
            q=e['session']['DNS']['Questions'][0];actual=expanded_name(w,12);assert actual==base64.b64decode(q['Name Wire'])
            dns=e['session']['DNS'];assert dns['ID']==int.from_bytes(w[:2],'big')and dns['Opcode']==(w[2]>>3)&15 and dns['Response']==bool(w[2]&128),'DNS identity header comes from the exact wire message'
            assert len(dns['Questions'])==int.from_bytes(w[4:6],'big')==1,'this reviewed name case contains exactly one wire question'
            cursor=12
            while w[cursor]:
                if w[cursor]&192==192:
                    cursor+=1;break
                cursor+=1+w[cursor]
            cursor+=1
            assert(q['Type'],q['Class'])==(int.from_bytes(w[cursor:cursor+2],'big'),int.from_bytes(w[cursor+2:cursor+4],'big')),'question type/class are part of the literal wire association identity'
            text=[v['value'] for v in e['field_values']['values'] if v['name']=='dns.qry.name'];escaped=[v['value'] for v in e['field_values']['values'] if v['name']=='dns.qry.name.escaped']
            assert escaped==[q['Name']]
            labels=name_labels(actual);expected_escaped='.'.join(map(escaped_label,labels));expected_readable='.'.join(map(readable_label,labels))
            assert escaped==[expected_escaped] and text==[expected_readable],'native display and escaped values are independently derived from literal labels'
            ws_text=list(field_strings(dns_layers[frame],'dns.qry.name'));assert len(ws_text)==1,'current oracle must expose this one question name'
            fixed=text==ws_text
            if not fixed:
                try:b''.join(labels).decode('utf8')
                except UnicodeDecodeError:pass
                else:raise AssertionError(('valid UTF-8 display disagreement cannot be classified as invalid octets',name,frame))
                assert ws_text==['.'.join(v.decode('utf8','replace')for v in labels)],'the exact replacement-text discrepancy must be independently explained'
            if e['protocol']=='mdns':
                assert e['session']['Association']=='multicast-observation'and e['transaction']==e['response_to']==0
                association='multicast observation; no request/response association asserted'
            elif e['response_to']:
                request=dns_by_id[e['response_to']]
                assert identity(request)==identity(e)and request['domain']==e['domain']and(request['source'],request['destination'])==(e['destination'],e['source'])
                assert e['transaction']==request['id']and e['session']['Matched Request']==request['session']['DNS']['Questions'][0]['Name']
                association='exact wire-question identity and reverse endpoints/domain match the emitted request'
            else:
                assert not e['session']['DNS']['Response']and e['transaction']==e['id']
                association='request owns its transaction; canonical wire-question preserved'
            row.update({'classification':'readable DNS name display fixed' if fixed else 'invalid UTF-8 retained losslessly; Wireshark replacement text differs','wire_name_sha256':sha(actual),'native_message_sha256':sha(w),'display_matches_wireshark':fixed,'escaped_name_preserved':True,'question_identity_preserves_wire':True,'association_check':association,'parser_value_error':False})
        if original_review:
            old=next(r for r in original_review[name]['rows']if(r['frame'],r['field'])==(frame,d['field']))
            assert row['classification']==old['classification'],'a classification change requires a separate explicit review'
            if d['field']=='tls.record.length':
                key=lambda r:(r['sha256'],r['length'],tuple(r['contributors']),r['byte_complete_frame'])
                assert collections.Counter(map(key,row['records']))==collections.Counter(map(key,old['records'])),'original record byte/contributor witnesses are all retained'
            else:
                assert(row['wire_name_sha256'],row['native_message_sha256'])==(old['wire_name_sha256'],old['native_message_sha256']),'original DNS byte witnesses are unchanged'
        result.append(row)
    assert len(result)==len(baseline)
    reviews[name]={'original_difference_count':len(baseline),'events_sha256':digest(path),'capture_sha256':digest(capture),'TLS_oracle_sha256':digest(weber_oracle_path if name=='weber'else oracle_path),'physical_scope_oracle_sha256':digest(scope_path),'physical_scope_frames':len(scope_frames),'independent_outer_IPv6_reassemblies':reassembly_proofs,'DNS_oracle_sha256':digest(a.out/(name+'-dns-oracle.json'))if dns_frames else None,'original_review_sha256':digest(a.original_review)if a.original_review else None,'RDPUDP_derived_capture_sha256':derived_summary['derived_capture_sha256'],'classification_counts':dict(collections.Counter(r['classification'] for r in result)),'unreviewed':0,'rows':result}
    print(json.dumps({'capture':name,'reviewed':len(result),'classes':reviews[name]['classification_counts']}),flush=True)
(a.out/'field-differences-reviewed.json').write_text(json.dumps(reviews,indent=2)+'\n',encoding='utf8')
