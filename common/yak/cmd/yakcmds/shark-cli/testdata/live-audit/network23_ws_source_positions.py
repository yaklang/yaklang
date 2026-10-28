"""Compare23 network aliases with TShark at proven original-record coordinates.

Source-location ledgers and typed plans predate the candidate SDK output. This
auditor rereads original captures and never discovers offsets by searching Raw.
It records literal Wireshark omissions and disagreements without waiving them.
Inherited invalid EtherIP/cross-family admission is not certified by this check.
"""
import argparse, base64, collections, copy, hashlib, ipaddress, json, subprocess, sys, time
from pathlib import Path


def sha(path):
    h=hashlib.sha256()
    with Path(path).open('rb') as f:
        for b in iter(lambda:f.read(1048576),b''):h.update(b)
    return h.hexdigest()


def rows(path):
    with Path(path).open(encoding='utf-8-sig') as f:
        for line in f:yield json.loads(line)


def same(x,y):
    if type(x) is not type(y):return False
    if isinstance(x,dict):return x.keys()==y.keys() and all(same(x[k],y[k]) for k in x)
    if isinstance(x,list):return len(x)==len(y) and all(same(a,b) for a,b in zip(x,y))
    return x==y


def json_array(path):
    """Streaming standard JSON array; memory bounded by one exported packet."""
    d=json.JSONDecoder();buf='';at=0;started=False;ended=False
    with Path(path).open(encoding='utf8') as f:
        eof=False
        while True:
            while at<len(buf) and buf[at].isspace():at+=1
            if not started and at<len(buf):assert buf[at]=='[';at+=1;started=True;continue
            while at<len(buf) and (buf[at].isspace() or buf[at]==','):at+=1
            if started and at<len(buf) and buf[at]==']':at+=1;ended=True;break
            try:value,end=d.raw_decode(buf,at)
            except json.JSONDecodeError:
                assert not eof,'truncated JSON array'
                buf=buf[at:]+f.read(1048576);at=0
                if len(buf)==0:eof=True
                continue
            yield value;at=end
        assert started and ended and not (buf[at:]+f.read()).strip()


def raw_variants(value):
    if isinstance(value,list):
        if len(value)>=3 and type(value[0]) is str and type(value[1]) is int and type(value[2]) is int:return [value]
        return [x for part in value for x in raw_variants(part)]
    return []


def walk(value):
    if isinstance(value,dict):
        yield value
        for v in value.values():yield from walk(v)
    elif isinstance(value,list):
        for v in value:yield from walk(v)


def own_nodes(layers,protocol,offset):
    key=('arp.opcode' if protocol=='arp' else protocol+'.type')+'_raw'
    wanted=offset+(6 if protocol=='arp' else 0)
    return [x for x in walk(layers) if key in x and any(r[1]==wanted for r in raw_variants(x[key]))]


def ws_scalar(name,text,spec):
    assert type(text) is str
    typ=spec[name][1]
    if typ=='unsigned':return int(text,16) if text.lower().startswith('0x') else int(text,10)
    if typ=='address':
        ip=ipaddress.ip_address(text)
        return '::ffff:'+str(ip.ipv4_mapped) if ip.version==6 and ip.ipv4_mapped else str(ip)
    return text.lower()


def field_result(node,item,absolute,wire,spec):
    name=item['name'];length=item['pdu_range']['length'];found=[]
    for v in walk(node):
        if name+'_raw' not in v:continue
        raw=raw_variants(v[name+'_raw']);scalars=v.get(name,[])
        scalars=scalars if isinstance(scalars,list) else [scalars]
        for index,r in enumerate(raw):
            text=scalars[index] if index<len(scalars) else None
            found.append(dict(raw=r,text=text))
    here=[x for x in found if x['raw'][1]==absolute]
    if not here:return dict(result='missing_at_own_position',other_occurrences=found)
    checks=[]
    for x in here:
        r=x['raw'];wire_hex=wire[absolute:absolute+length].hex()
        try:value=ws_scalar(name,x['text'],spec)
        except (AssertionError,ValueError,TypeError):value=None
        checks.append(dict(offset=r[1],length=r[2],wire_hex=r[0],text=x['text'],value=value,
                           exact=(r[2]==length and r[0].replace(':','').lower()==wire_hex and same(value,item['value']))))
    if len(checks)!=1:return dict(result='ambiguous_at_own_position',candidates=checks)
    return dict(result='equal' if checks[0]['exact'] else 'different_at_own_position',**checks[0])


def frame_check(packet,record):
    from pptp_parent_source_audit import instant
    layers=packet['_source']['layers'];f=layers['frame'];raw=raw_variants(layers['frame_raw'])
    assert len(raw)==1 and raw[0][1:3]==[0,len(record['wire'])]
    assert bytes.fromhex(raw[0][0])==record['wire']
    assert int(f['frame.number'])==record['frame']
    assert int(f['frame.cap_len'])==record['captured_length'] and int(f['frame.len'])==record['original_length']
    # TShark sections are one-based, PCAPNG interfaces are section-local zero-based.
    assert int(f.get('frame.section_number','1'))==record['section']+1
    assert int(f.get('frame.interface_id','0'))==record['interface']
    assert instant(f['frame.time_utc'])==record['time_ns']
    return layers


def runs_filter(frames):
    runs=[]
    for n in sorted(frames):
        if runs and n==runs[-1][1]+1:runs[-1][1]=n
        else:runs.append([n,n])
    return 'frame.number in { '+', '.join(str(a) if a==b else str(a)+'..'+str(b) for a,b in runs)+' }'


def negatives(sample,record,item,node,spec):
    cases=[]
    def expect_bad(name,fn):
        try:ok=fn()
        except (AssertionError,ValueError,KeyError,TypeError):ok=False
        assert not ok,'negative accepted: '+name
        cases.append(dict(name=name,rejected=True))
    protocol=sample['protocol'];loc=sample['location'];absolute=loc['raw_start']+item['pdu_range']['offset'];alias=item['name'];rawkey=alias+'_raw'
    # Actual matched source/node witnesses are mutated, not expected count constants.
    container=next(x for x in walk(node) if rawkey in x and any(r[1]==absolute for r in raw_variants(x[rawkey])))
    c=copy.deepcopy(container);raw_variants(c[rawkey])[0][1]+=1
    expect_bad('Wireshark own field offset shifted',lambda:field_result(c,item,absolute,record['wire'],spec)['result']=='equal')
    c=copy.deepcopy(container);raw_variants(c[rawkey])[0][2]+=1
    expect_bad('Wireshark own field length expanded',lambda:field_result(c,item,absolute,record['wire'],spec)['result']=='equal')
    c=copy.deepcopy(container);r=raw_variants(c[rawkey])[0];r[0]=('00' if r[0][:2]!='00' else '01')+r[0][2:]
    expect_bad('Wireshark raw value changed with displayed scalar preserved',lambda:field_result(c,item,absolute,record['wire'],spec)['result']=='equal')
    c=copy.deepcopy(container);c[alias]='255' if item['value']!=255 else '254'
    expect_bad('Wireshark displayed scalar changed with bytes preserved',lambda:field_result(c,item,absolute,record['wire'],spec)['result']=='equal')
    c=copy.deepcopy(node);marker=('arp.opcode' if protocol=='arp' else protocol+'.type')+'_raw'
    for r in raw_variants(c[marker]):r[1]+=8
    expect_bad('Quoted or sibling header at another offset cannot be own header',lambda:bool(own_nodes(c,protocol,loc['own_offset'])))
    c=copy.deepcopy(item);c['pdu_range']['offset']+=1
    expect_bad('Source1 field range drift',lambda:field_result(node,c,loc['raw_start']+c['pdu_range']['offset'],record['wire'],spec)['result']=='equal')
    c=copy.deepcopy(item);c['value']=not item['value'] if type(item['value']) is int else 'forged'
    expect_bad('Typed value or scalar type forged',lambda:field_result(node,c,absolute,record['wire'],spec)['result']=='equal')
    c=dict(record);c['wire']=record['wire'][:absolute]+bytes([record['wire'][absolute]^1])+record['wire'][absolute+1:]
    expect_bad('Original wire changed beneath unchanged TShark field',lambda:field_result(node,item,absolute,c['wire'],spec)['result']=='equal')
    return cases


def main():
    ap=argparse.ArgumentParser()
    for n in ['corpus','capture','native','plan','icmp-source','old-fields','carrier-bound','carrier-aliases','constructor','audit-modules','tshark','out']:
        ap.add_argument('--'+n,required=True,type=str if n=='corpus' else Path)
    ap.add_argument('--reuse-export',type=Path)
    a=ap.parse_args();assert a.corpus in ['weber','sharon'];a.out.mkdir(exist_ok=False)
    sys.path[:0]=[str(a.constructor),str(a.audit_modules)]
    from network23_values import project,SPEC
    from vxlan_endpoint_projection_audit import capture_records
    plans={r['id']:r for r in rows(a.plan/a.corpus/'snapshots.jsonl')}
    observations={}
    for x in rows(a.icmp_source/a.corpus/'expected.jsonl'):
        l=x['source_profile'];p=plans[x['id']]
        observations[x['id']]=dict(id=x['id'],frame=x['frame'],protocol='icmp',location=dict(raw_start=l['raw_start'],raw_end=l['raw_end'],own_offset=l['offset'],own_end=l['end']),
                                     wire_SHA256=x['record']['wire_SHA256'],scope=l['scope'],items=p['expected_added_values'],snapshot=p['whole_proposed_snapshot'])
    for x in rows(a.old_fields/a.corpus/'source-fields.jsonl'):
        if x['protocol'] not in ['arp','icmpv6']:continue
        p=plans[x['id']]
        observations[x['id']]=dict(id=x['id'],frame=x['frame'],protocol=x['protocol'],location=dict(raw_start=0,raw_end=None,own_offset=x['own_offset'],own_end=x['own_offset']+x['own_extent']),
                                     wire_SHA256=x['wire_SHA256'],scope=x['scope'],items=p['expected_added_values'],snapshot=p['whole_proposed_snapshot'])
    assert observations.keys()==plans.keys()
    extra={x['id']:x for x in rows(a.carrier_aliases/'expected.jsonl') if x['corpus']==a.corpus}
    for x in rows(a.carrier_bound/'bound.jsonl'):
        if x['corpus']!=a.corpus:continue
        l=x['location'];q=extra[x['id']];assert x['id'] not in observations
        observations[x['id']]=dict(id=x['id'],frame=x['frame'],protocol=x['protocol'],location=dict(raw_start=l['raw_start'],raw_end=l['raw_end'],own_offset=l['offset'],own_end=l['end']),
                                     wire_SHA256=x['wire_SHA256'],scope='; '.join(l['profiles']),items=q['added_values'],snapshot=q['expected_new_event']['field_values'])
    byframe=collections.defaultdict(list)
    for x in observations.values():byframe[x['frame']].append(x)
    records={}
    for frame,record in enumerate(capture_records(a.capture),1):
        if frame not in byframe:continue
        record['frame']=frame;records[frame]=record
        for x in byframe[frame]:
            assert hashlib.sha256(record['wire']).hexdigest()==x['wire_SHA256']
            if x['location']['raw_end'] is None:x['location']['raw_end']=len(record['wire'])
            assert same(project(x['protocol'],record['wire'],x['location']),x['items'])
    assert records.keys()==byframe.keys()
    seen=set()
    for e in rows(a.native/'preservation'/a.corpus/'events.jsonl'):
        if e['id'] not in observations:continue
        x=observations[e['id']];record=records[x['frame']];l=x['location']
        assert e['id'] not in seen;seen.add(e['id'])
        assert e['protocol']==x['protocol'] and same(e['field_values'],x['snapshot'])
        assert base64.b64decode(e['raw'])==record['wire'][l['raw_start']:l['raw_end']]
        assert e['source_bytes']['SHA256']==hashlib.sha256(base64.b64decode(e['raw'])).hexdigest()
        assert e['source_bytes']['PacketRefs'][0]['Number']==record['frame']
        assert e['domain']['Section']==record['section'] and e['domain']['Interface']==record['interface']
        for ref in e['source_bytes']['PacketRefs']:
            assert ref['Number']==record['frame'] and ref['Domain']['Section']==record['section'] and ref['Domain']['Interface']==record['interface']
    assert seen==observations.keys()
    display_filter=runs_filter(byframe)
    (a.out/'display-filter.txt').write_text(display_filter,encoding='utf8')
    target=a.out/'wireshark.json';err=a.out/'wireshark.stderr.txt'
    args=[str(a.tshark),'-n','-r',str(a.capture),'-Y',display_filter,'-T','json','-x','--no-duplicate-keys','-j','frame arp icmp icmpv6 _ws.expert']
    start=time.time()
    if a.reuse_export:
        target=a.reuse_export
        receipt=dict(reused_export=str(target),export_SHA256=sha(target),original_receipt_SHA256=sha(target.with_name('wireshark-receipt.json')))
    else:
        with target.open('wb') as output,err.open('wb') as error:
            p=subprocess.run(args,stdin=subprocess.DEVNULL,stdout=output,stderr=error)
        assert p.returncode==0
        receipt=dict(arguments=args,exit_code=p.returncode,seconds=round(time.time()-start,3),export_SHA256=sha(target),stderr_SHA256=sha(err))
    (a.out/'wireshark-receipt.json').write_text(json.dumps(receipt,indent=2,sort_keys=True)+'\n',encoding='utf8')
    counts=collections.Counter();alias_counts=collections.defaultdict(collections.Counter);seenframes=set();negative_cases=[];findings=[];sourcecounts=collections.Counter()
    with (a.out/'source-position-results.jsonl').open('w',encoding='utf8') as output:
        for packet in json_array(target):
            f=int(packet['_source']['layers']['frame']['frame.number']);assert f in records and f not in seenframes;seenframes.add(f)
            layers=frame_check(packet,records[f])
            for x in byframe[f]:
                loc=x['location'];nodes=own_nodes(layers,x['protocol'],loc['own_offset']);fields=[]
                for item in x['items']:
                    absolute=loc['raw_start']+item['pdu_range']['offset']
                    r=field_result(nodes[0],item,absolute,records[f]['wire'],SPEC) if len(nodes)==1 else dict(result='own_protocol_missing' if not nodes else 'own_protocol_ambiguous')
                    field=dict(name=item['name'],literal=item['value'],absolute_offset=absolute,expected_length=item['pdu_range']['length'],Source1_range=item['pdu_range'],**r)
                    fields.append(field);counts[r['result']]+=1;alias_counts[item['name']][r['result']]+=1
                diagnostic=[v for n in walk(layers) for k,v in n.items() if k in ['_ws.expert.message','_ws.expert.severity','_ws.malformed']]
                row=dict(id=x['id'],frame=f,protocol=x['protocol'],scope=x['scope'],wire_SHA256=x['wire_SHA256'],physical_source={k:records[f][k] for k in ['section','interface','link','time_ns','captured_length','original_length']},location=loc,own_WS_nodes=len(nodes),fields=fields,diagnostics=diagnostic)
                output.write(json.dumps(row,sort_keys=True)+'\n');sourcecounts['sources']+=1;sourcecounts[x['protocol']]+=1
                if any(q['result']!='equal' for q in fields):findings.append(row)
                if not negative_cases and len(nodes)==1 and all(q['result']=='equal' for q in fields):negative_cases=negatives(x,records[f],x['items'][0],nodes[0],SPEC)
    assert seenframes==records.keys() and sourcecounts['sources']==len(observations)
    assert sum(counts.values())==sum(len(x['items']) for x in observations.values()) and negative_cases
    (a.out/'differences.jsonl').write_text(''.join(json.dumps(x,sort_keys=True)+'\n' for x in findings),encoding='utf8')
    (a.out/'negative-controls.json').write_text(json.dumps(negative_cases,indent=2,sort_keys=True)+'\n',encoding='utf8')
    summary=dict(scope='Literal original-source/native typed value and range vs TShark own absolute field position. Different/missing values are inventoried, not waived. No parity claim for invalid parent admission.',
                 corpus=a.corpus,source='5403ffe55b219702af97a3422dd6d59996aadfbe',sources=len(observations),frames=len(records),source_families=dict(sourcecounts),typed_values=sum(counts.values()),results=dict(counts),alias_results={k:dict(v) for k,v in sorted(alias_counts.items())},sources_with_differences=len(findings),negative_controls=len(negative_cases),
                 capture_SHA256=sha(a.capture),TShark_SHA256=sha(a.tshark),SDK_SHA256=sha(a.native/'preservation'/a.corpus/'events.jsonl'),producer_SHA256=sha(a.native/'preservation'/a.corpus/'events-producer.json'),auditor_SHA256=sha(Path(__file__)),constructor_SHA256=sha(a.constructor/'network23_values.py'),reader_SHA256=sha(a.audit_modules/'vxlan_endpoint_projection_audit.py'),
                 evidence={str(p):sha(p) for p in [a.plan/'summary.json',a.icmp_source/'summary.json',a.old_fields/'summary.json',a.carrier_bound/'summary.json',a.carrier_aliases/'summary.json',a.out/'wireshark-receipt.json',a.out/'source-position-results.jsonl',a.out/'differences.jsonl',a.out/'negative-controls.json']})
    s=a.out/'summary.json';s.write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(s),SHA256=sha(s),sources=len(observations),results=dict(counts),sources_with_differences=len(findings))),flush=True)


if __name__=='__main__':main()
