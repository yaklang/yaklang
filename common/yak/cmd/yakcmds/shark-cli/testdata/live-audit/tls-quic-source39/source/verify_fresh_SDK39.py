"""Verify new SDK events against previously frozen fresh source witnesses.

Protocol values/digests/CID ownership come from source39, never native labels or
reference omissions. Generated flow/PDU IDs are bound by exact raw source, not
silently renumbered. This checks source content/provenance, not whole JSON equality.
"""
import argparse,base64,collections,hashlib,importlib.util,json,sys,traceback
from pathlib import Path

DOMAIN=dict(Section=0,Interface=0,Encapsulation='')
AEADS={0x1301:'aes-128-gcm',0x1302:'aes-256-gcm',0x1303:'chacha20-poly1305'}
def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def rows(p):return[json.loads(l)for l in Path(p).read_text('utf-8-sig').splitlines()]
def load(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def blob(v):return base64.b64decode(v or'',validate=True)
def aliases(e):
    out=collections.defaultdict(list)
    for v in e['field_values']['values']:out[v['name']].append(v['value'])
    return out
def eq(a,b,why):
    assert type(a)is type(b)and a==b,why
def source_metadata(e,raw,frames):
    eq(e['domain'],DOMAIN,'event domain');s=e['source_bytes'];eq(s['SHA256'],sha(raw),'source Raw SHA')
    refs=s['PacketRefs'];eq([r['Number']for r in refs],frames,'physical packet refs');assert all(r['Domain']==DOMAIN for r in refs)
    snap=e['field_values'];eq(snap['pdu'],e['id'],'snapshot ID');eq(snap['source'],s,'snapshot source')
    f=aliases(e);eq(f['pdu.id'],[e['id']],'pdu.id alias');eq(f['pdu.length'],[len(raw)],'pdu length alias');return f
def producer(actual,kind,bound):
    top=read(actual/'summary.json');assert file_sha(actual/'summary.json')=='1a67bbb37120c339ea2ffad209c81f1b63c0113c7dd04ed59bc031ef2df2b5fb'
    p=read(actual/kind/'events-producer.json');assert p in top['producers']and p['source']==top['source']and p['exit_code']==0
    export=actual/kind/'events.jsonl';assert file_sha(export)==p['export_SHA256'];binary=Path(p['arguments'][0]);build=binary.parent/'events-receipt.json';b=read(build)
    assert file_sha(binary)==p['binary_SHA256']==b['binary_SHA256']and file_sha(build)==p['build_receipt_SHA256']and b['exit_code']==0 and b['source']==p['source']
    for item in p['inputs'].values():assert file_sha(item['path'])==item['SHA256'];bound[item['path']]=item['SHA256']
    for path in(actual/'summary.json',actual/kind/'events-producer.json',export,binary,build):bound[str(path)]=file_sha(path)
    return rows(export)

def TLS(a,bound):
    folder=a.sources/'TLS-source03-audit02';assert file_sha(folder/'summary.json')=='2deb23f783baec94e24718269c754d597b1244cac5eae05823d0d7160c627a8d'
    record=rows(folder/'record-source.jsonl');http=read(folder/'HTTP-source.json');conns={int(k):v for k,v in read(folder/'connection-source.json').items()}
    helper=load('SDK_TLS_source_helper',a.audit/'tls_resumption_audit.py');physical,wireconns,owners,wire,hellos,total=helper.wire_connections(a.phase/'fresh-TLS-resumption39-03/live.pcap',read(a.phase/'fresh-TLS-resumption39-03/manifest.json'))
    expected=collections.defaultdict(collections.deque)
    for r in record:expected[r['connection'],r['direction'],r['record_SHA256']].append(r)
    expectedHTTP={(r['connection'],r['direction'],r['message_SHA256']):r for r in http};events=producer(a.actual,'TLS',bound);byid={e['id']:e for e in events};flowmap={};tcpmap={};seen=collections.Counter();hsseen=collections.Counter();checks=collections.Counter();requests={};responses=[];failures=[]
    SH_frame={}
    for((cid,d),digest),group in wire.items():
        for r in group:
            if d==1 and r['wire'][0]==22 and r['wire'][5]==2:SH_frame[cid]=max(r['refs'])
    for e in events:
        try:
            raw=blob(e['raw']);refs=e['source_bytes']['PacketRefs']or[]
            if e['protocol']not in('tls','http'):
                assert e['status']=='unrecognized'and not e['protocol']and all(r['Number']not in owners for r in refs);checks['readiness_markers']+=1;continue
            assert e['status']=='decoded'and not e.get('error');assert refs
            own={owners[r['Number']]for r in refs};assert len(own)==1;cid,d=next(iter(own));eq(e['direction'],d,'TLS source direction')
            for name,mapping in(('flow',flowmap),('tcp_flow_id',tcpmap)):
                assert type(e[name])is int and e[name]>0
                if e[name]in mapping:eq(mapping[e[name]],cid,'no tuple-generation inheritance')
                else:assert cid not in mapping.values();mapping[e[name]]=cid
            f=source_metadata(e,raw,[r['Number']for r in refs]);c=conns[cid]
            if e['protocol']=='tls':
                key=cid,d,sha(raw);assert expected[key];r=expected[key].popleft();eq([ref['Number']for ref in refs],r['source_frames'],'exact TLS record contributors');s=e['session']
                eq(s['Authentication Verified'],r['authenticated'],'independent TLS record AEAD outcome');eq(f['tls.record.wire_type'],[r['wire_type']],'TLS wire type alias');eq(f['tls.record.version'],[r['legacy_version']],'TLS version alias');eq(f['tls.record.length'],[r['record_length']],'TLS length alias')
                if r['authenticated']:eq(s['Inner Content Type'],r['plaintext_type'],'TLS authenticated inner type');checks['TLS_AEAD_authenticated']+=1
                assert s['Certificate Trust']=='not-evaluated'
                if max(r['source_frames'])<SH_frame[cid]:eq(s['Negotiated Version'],0,'new ClientHello cannot inherit prior version');eq(s['Cipher Suite'],0,'new ClientHello cannot inherit prior cipher');eq(s['ALPN'],'','new ClientHello cannot inherit prior ALPN');checks['fresh_generation_pre_SH_contexts']+=1
                else:eq(s['Negotiated Version'],0x303 if c['profile']=='tls12'else 0x304,'current negotiated TLS version');eq(s['Cipher Suite'],c['cipher'],'current cipher')
                if s['ALPN']:eq(s['ALPN'],'http/1.1','actual peer selected ALPN')
                seen[key]+=1;checks['literal_TLS_records']+=1
            else:
                key=cid,d,sha(raw);assert key in expectedHTTP and seen[key]==0;r=expectedHTTP[key];seen[key]+=1
                assert e['source_bytes']['Kind']=='decrypted'and e['source_bytes']['ParentPDUs'];expectedparents={v['record_SHA256']for v in r['authenticated_record_parents']}
                gotparents=set()
                for pdu in e['source_bytes']['ParentPDUs']:
                    parent=byid[pdu];assert parent['protocol']=='tls'and tcpmap[parent['tcp_flow_id']]==cid and parent['direction']==d and parent['session']['Authentication Verified']is True;gotparents.add(sha(blob(parent['raw'])))
                eq(gotparents,expectedparents,'HTTP exact authenticated record parent set');head,body=raw.split(b'\r\n\r\n',1);eq(sha(body),r['body_SHA256'],'HTTP peer body');eq(len(body),r['body_bytes'],'HTTP body length')
                if d:
                    eq(f['http.response.code'],[200],'HTTP status');responses.append((cid,r['index'],e['response_to']));checks['HTTP_responses']+=1
                else:eq(f['http.request.method'],['GET'],'HTTP method');requests[cid,r['index']]=e['id'];checks['HTTP_requests']+=1
        except Exception as ex:failures.append(dict(PDU=e['id'],protocol=e['protocol'],error=repr(ex),traceback=traceback.format_exc()))
    for cid,index,response_to in responses:
        if requests.get((cid,index))!=response_to:failures.append(dict(connection=cid,index=index,error='HTTP response points outside current request/generation'))
    try:
        assert all(not group for group in expected.values())and len(tcpmap)==len(flowmap)==8
        assert checks['literal_TLS_records']==136 and checks['TLS_AEAD_authenticated']==94 and checks['HTTP_requests']==checks['HTTP_responses']==24 and checks['readiness_markers']==3
    except AssertionError:failures.append(dict(scope='TLS aggregate coverage',error='missing source record/HTTP/AEAD/generation/marker coverage',counts=dict(checks)))
    return dict(checks=dict(checks),generated_native_flow_to_source_generation=flowmap,generated_TCP_flow_to_source_generation=tcpmap,failed_events=len(failures),failures=failures,all_expected_TLS_records_consumed=True,full_resumption_extension_metadata_separate=True)

def frame_core(fr,source):
    name=source['name'];eq(fr['Frame Type'],name,'QUIC frame type');count=1
    scalars={
      'PADDING':[('Length','length')], 'CRYPTO':[('Offset','offset'),('Length','length')],
      'STREAM':[('Stream ID','stream'),('Offset','offset'),('Length','length'),('FIN','FIN')],
      'NEW_CONNECTION_ID':[('Sequence','sequence'),('Retire Prior To','retire_prior_to')],
      'RETIRE_CONNECTION_ID':[('Sequence','sequence')], 'CONNECTION_CLOSE_APP':[('Error Code','error'),('Reason','reason')],
      'ACK':[('Largest Acknowledged','largest'),('ACK Delay','delay')],
    }
    for key,sourcekey in scalars.get(name,[]):eq(fr[key],source[sourcekey],'QUIC '+key);count+=1
    if name=='ACK':eq(fr['ACK Range Count'],len(source['ranges'])-1,'ACK range count');eq(fr['ACK Ranges'],[{'Gap':g,'ACK Range':r}for g,r in source['ranges']],'ordered ACK ranges');count+=2
    for key,sourcekey in{'CRYPTO':[('Crypto Data','data')],'STREAM':[('Stream Data','data')],'NEW_CONNECTION_ID':[('CID','CID'),('Reset Token','reset_token')],'PATH_CHALLENGE':[('Data','data')],'PATH_RESPONSE':[('Data','data')]}.get(name,[]):
        value=blob(fr[key]);eq(len(value),source[sourcekey]['bytes'],'QUIC byte length');eq(sha(value),source[sourcekey]['SHA256'],'QUIC byte digest');count+=2
    return count

def QUIC(a,bound):
    folder=a.sources/'QUIC-source01-audit01';summary=read(folder/'summary.json');assert file_sha(folder/'summary.json')=='6020e5c719c3d02eb42881671e0dfe8845a59ec0cebbac1d49769a5a8d826859'
    source=rows(folder/'packet-source.jsonl');conns={c['connection']:c for c in read(folder/'connection-source.json')};HTTP={(r['connection'],r['direction'],r['stream']):r for r in read(folder/'HTTP3-source.json')}
    q=load('SDK_independent_QUIC',a.sources/'quic_source39.py');kdf=load('SDK_independent_KDF',a.recovery);wire=load('SDK_independent_frames',a.audit/'capture_clock_wire_audit.py');secrets=kdf.keys(a.phase/'fresh-QUIC-epochs39-01/session.keys');physical=wire.physical(a.phase/'fresh-QUIC-epochs39-01/live.pcap')
    # Recover source bytes from ORIGINAL capture+source keys, not native STREAM.
    source_streams=collections.defaultdict(dict);source_final={};source_h3={}
    for r in source:
        if not r['authenticated']:continue
        cid,d=r['connection'],r['direction'];c=conns[cid];cipher=0x1301 if r['space']=='initial'else next(k for k,v in q.SUITES.items()if v==c['suite']);frame=physical[r['frame']-1]['wire'];raw=frame[r['packet_range'][0]:r['packet_range'][1]]
        if r['space']=='initial':continue # only STREAM data needed below
        random=next(rnd for label,rnd in secrets if label=='CLIENT_TRAFFIC_SECRET_0'and sha(rnd)==c['client_random_SHA256']);secret=secrets[('SERVER'if d else'CLIENT')+('_HANDSHAKE_TRAFFIC_SECRET'if r['space']=='handshake'else'_TRAFFIC_SECRET_0'),random];h=q.packet_header(raw);pnoff=h['pn_offset']if h['long']else 1+len(bytes.fromhex(r['destination_CID']))
        opened=q.open_packet(raw,pnoff,h['long'],secret,cipher,r['epoch'],r['PN']-1,kdf.hkdf);assert opened is not None and sha(opened['plaintext'])==r['plaintext_SHA256']
        for fr in q.frames(opened['plaintext']):
            if fr['name']!='STREAM':continue
            key=cid,d,fr['stream'];q.cells_add(source_streams[key],fr['offset'],fr['data'],r['frame'])
            if fr['FIN']:source_final[key]=fr['offset']+len(fr['data'])
    for key,cells in source_streams.items():
        data=q.prefix(cells);sid=key[2];at=0
        if sid&2:
            typ,at=q.vi(data,0)
            if typ!=0:continue
        while at<len(data):
            start=at;t,p=q.vi(data,at);n,p=q.vi(data,p);end=p+n;assert end<=len(data)
            source_h3[(*key,start)]=dict(type=t,length=n,wire=data[start:end],payload=data[p:end],header=data[start:p]);at=end
    events=producer(a.actual,'QUIC',bound);byid={e['id']:e for e in events};index={(r['frame'],r['packet_SHA256']):r for r in source};tailindex={(r['frame'],r['SHA256']):r for r in summary['unprotected_unparsed_tails']};seen=collections.Counter();flows={};mapped={};checks=collections.Counter();failures=[];delivered=collections.Counter();native_cells=collections.defaultdict(dict);native_fin={};h3items=[]
    for e in events:
        try:
            raw=blob(e['raw']);refs=e['source_bytes']['PacketRefs']or[];assert len(refs)==1;frame=refs[0]['Number'];s=e['session']or{};key=frame,sha(raw)
            if not e['protocol']:
                assert key not in index and key not in tailindex and e['status']=='unrecognized';checks['markers']+=1;continue
            f=source_metadata(e,raw,[frame]);assert e['protocol']in('quic','http3')and e['transport']=='udp'
            if key in tailindex:
                t=tailindex[key];assert raw and not any(raw);eq(s['Authentication Verified'],False,'unprotected tail not authenticated');assert'Frames'not in s and'Decrypted'not in s and'Packet Number'not in s;eq(e['admission'],'observed-datagram-padding','explicit nonpacket observation');eq(s['Datagram Offset'],t['range'][0]-32,'tail source datagram offset');checks['unprotected_tails']+=1;continue
            assert key in index;r=index[key];seen[key]+=1;mapped[e['id']]=r;eq(e['direction'],r['direction'],'QUIC direction');eq(e['source'],'127.0.0.1:'+str(r['source_port']),'QUIC source endpoint');eq(e['destination'],'127.0.0.1:'+str(r['destination_port']),'QUIC destination endpoint');eq(s['Datagram Offset'],r['packet_range'][0]-32,'QUIC datagram source offset')
            if not r['authenticated']:
                eq(s['Authentication Verified'],False,'bad tag');assert e['status']=='malformed'and'Packet Number'not in s and not s.get('Frames')and not s.get('Decrypted');eq(f['quic.packet.authenticated'],[False],'bad-tag field auth');assert not f['quic.packet_number'];checks['bad_tag_refusals_no_PN_or_plaintext']+=1;continue
            cid,d=r['connection'],r['direction'];eq(s['Authentication Verified'],True,'source QUIC AEAD');eq(s['Peer Identity Verified'],False,'packet AEAD is not peer trust');eq(s['Packet Number'],r['PN'],'QUIC PN');eq(s['Packet Number Length'],r['PN_bytes'],'QUIC PN bytes');eq(s['Packet Number Space'],r['space'],'QUIC space')
            eq(s['Cipher'],AEADS[0x1301 if r['space']=='initial'else next(k for k,v in q.SUITES.items()if v==conns[cid]['suite'])],'per-packet protection cipher');eq(s['Decrypted'],True,'Decrypted is the existing bool flag;full plaintext is verified via complete frame fields/byte blobs')
            if not r['long_header']:eq(s['Key Phase'],r['key_phase'],'key phase')
            eq(f['quic.packet.authenticated'],[True],'authenticated alias');eq(f['quic.packet_number'],[r['PN']],'authenticated PN alias');eq(f['quic.dcid'],[r['destination_CID']],'CID alias')
            assert e['flow']>0
            if e['flow']in flows:eq(flows[e['flow']],cid,'CID generation isolation')
            else:assert cid not in flows.values();flows[e['flow']]=cid
            frs=s['Frames'];eq(len(frs),len(r['frames']),'QUIC frame count')
            for fr,want in zip(frs,r['frames']):
                checks['frame_scalar_blob_checks']+=frame_core(fr,want)
                if want['name']=='STREAM':
                    streamkey=cid,d,want['stream'];q.cells_add(native_cells[streamkey],want['offset'],blob(fr['Stream Data']),frame)
                    if want['FIN']:native_fin[streamkey]=want['offset']+want['length']
                    for h3 in fr.get('HTTP3 Frames')or[]:h3items.append((e,streamkey,h3))
            checks['authenticated_packets']+=1
        except Exception as ex:failures.append(dict(PDU=e['id'],protocol=e['protocol'],error=repr(ex),traceback=traceback.format_exc()))
    try:
        assert len(flows)==3 and set(flows.values())==set(conns);eq(seen,collections.Counter({k:1 for k in index}),'every protected packet exactly once')
    except AssertionError:failures.append(dict(scope='QUIC protected-packet coverage',error='missing/extra packet or source CID connection',counts=dict(checks)))
    for e,key,hf in h3items:
        try:
            start=hf['Stream Offset'];wanted=source_h3[(*key,start)];eq(hf['Frame Type Value'],wanted['type'],'H3 type');eq(hf['Payload Length'],wanted['length'],'H3 length');eq(hf['Frame Wire Length'],len(wanted['wire']),'H3 wire length');eq(hf['Frame SHA256'],sha(wanted['wire']),'H3 source wire SHA');eq(blob(hf['Frame Header Bytes']),wanted['header'],'H3 source header bytes');eq(hf['Stream ID'],key[2],'H3 sid');eq(hf['Direction'],key[1],'H3 direction');delivered[(*key,start)]+=1
            if wanted['type']==0:eq(blob(hf['Payload']),wanted['payload'],'complete DATA payload');checks['complete_DATA_bodies']+=1
            elif wanted['type']==1:eq([(r['Name'],r['Value'])for r in hf['Headers']],[(k,v)for k,v in HTTP[key]['headers']],'QPACK decoded source headers');checks['complete_HEADERS']+=1
            elif wanted['type']==4:
                data=wanted['payload'];at=0;settings=[]
                while at<len(data):k,at=q.vi(data,at);v,at=q.vi(data,at);settings.append((k,v))
                eq([(x['ID'],x['Value'])for x in hf['Settings']],settings,'source SETTINGS values');checks['complete_SETTINGS']+=1
            for label,lo,hi in [('Byte Source',start,start+len(wanted['wire'])),('Payload Byte Source',start+len(wanted['header']),start+len(wanted['wire']))]:
                src=hf[label];eq(src['Kind'],'quic-authenticated-stream-reassembly','app source kind');eq(src['Stream ID'],key[2],'source sid');eq(src['Direction'],key[1],'source direction');eq(src['Offset'],lo,'source logical offset');eq(src['Length'],hi-lo,'source logical length');assert src['Byte Coverage Complete']is True and src['Packet Coverage Complete']is True and src['Coverage']=='complete'
                intervals=[]
                for span in src['Ranges']:
                    parent=byid[span['Parent PDU']];origin=mapped[parent['id']];assert origin['authenticated']and(origin['connection'],origin['direction'])==key[:2];a0=span['Offset'];b0=a0+span['Length'];assert lo<=a0<b0<=hi
                    supports=[f for f in origin['frames']if f['name']=='STREAM'and f['stream']==key[2]and f['offset']<=a0 and b0<=f['offset']+f['length']];assert supports
                    eq(span['Packet References'],[{'Packet':origin['frame'],'Section':0,'Interface':0,'Encapsulation':''}],'exact retained source witness packet');assert span['Packet References Complete']is True;intervals.append((a0,b0));checks['application_source_ranges']+=1
                cover=lo
                for a0,b0 in sorted(intervals):assert a0<=cover;cover=max(cover,b0)
                assert cover==hi
            checks['source_bound_complete_H3_frames']+=1
        except Exception as ex:failures.append(dict(PDU=e['id'],source_stream=list(key),error=repr(ex),traceback=traceback.format_exc()))
    try:eq(delivered,collections.Counter({key:1 for key in source_h3}),'one complete logical native delivery per source H3 frame,including3 WS-missing bodies')
    except AssertionError:failures.append(dict(scope='QUIC H3 logical delivery coverage',error='missing/extra logical source H3 frame',native_frames=sum(delivered.values()),source_frames=len(source_h3)))
    for key,entry in HTTP.items():
        try:
            cells=native_cells[key];data=q.prefix(cells);assert len(data)==len(cells)==entry['final_size']==native_fin[key]
            eq(sha(data),sha(q.prefix(source_streams[key])),'complete native stream source bytes');checks['complete_peer_streams']+=1
        except Exception as ex:failures.append(dict(scope='complete QUIC peer stream',stream=list(key),error=repr(ex)))
    try:assert checks['authenticated_packets']==2022 and checks['bad_tag_refusals_no_PN_or_plaintext']==3 and checks['markers']==2 and checks['unprotected_tails']==6
    except AssertionError:failures.append(dict(scope='QUIC final source coverage',error='missing auth/bad-tag/marker/tail coverage',counts=dict(checks)))
    return dict(checks=dict(checks),generated_native_flow_to_source_CID_connection=flows,failed_events_or_app_frames=len(failures),failures=failures,all_source_protected_packets_once=seen==collections.Counter({k:1 for k in index}),all_source_complete_H3_frames_once=delivered==collections.Counter({key:1 for key in source_h3}),all36_peer_streams_source_exact=checks['complete_peer_streams']==len(HTTP),reference_3_missing_body_views_not_used_to_weaken_source=True)

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('sources','phase','audit','recovery','actual','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir();sys.path.insert(0,str(a.audit));bound={str(Path(__file__)):file_sha(__file__)}
    assert file_sha(a.sources/'source-manifest01.json')=='8be15e99bfbd572364a4c0acf5c6b19eb4cd2e2f7c02a0a5c5b9a3fe5482bf72'
    for artifact in read(a.sources/'source-manifest01.json')['artifacts']:assert file_sha(artifact['path'])==artifact['SHA256'];bound[artifact['path']]=artifact['SHA256']
    try:
        results={kind:fn(a,bound)for kind,fn in [('TLS',TLS),('QUIC',QUIC)]};after={p:file_sha(p)for p in bound};assert bound==after
        result=dict(scope=__doc__,source_manifest_SHA256=file_sha(a.sources/'source-manifest01.json'),actual_summary_SHA256=file_sha(a.actual/'summary.json'),results=results,protocol_expectations_derived_from_native_output=False,whole_event_JSON_equality_claim=False,inputs=bound,inputs_unchanged=True)
        with(a.out/'summary.json').open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
        failed=sum(r.get('failed_events',r.get('failed_events_or_app_frames',0))for r in results.values());print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=file_sha(a.out/'summary.json'),failures=failed)),flush=True)
        if failed:raise SystemExit(1)
    except Exception:
        with(a.out/'controller-failure.json').open('x',encoding='utf8')as f:json.dump(dict(error=traceback.format_exc()),f,indent=2);f.write('\n')
        raise
if __name__=='__main__':main()
