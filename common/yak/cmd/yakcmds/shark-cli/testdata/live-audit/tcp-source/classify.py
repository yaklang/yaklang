"""Classify every newly displayed TCP own-header failure using original wire.

This is a complete own-header/Source0 projection check plus an explicit
inherited-metadata inventory, not a whole-file/native-ID preservation waiver.
The separate frozen145 verifier remains limited to its original full fixed
header profile. No candidate field supplies an offset or protocol decision.
"""
import argparse,collections,hashlib,ipaddress,json,sys
from pathlib import Path


def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb')as f:
        for b in iter(lambda:f.read(1048576),b''):h.update(b)
    return h.hexdigest()


def rows(p):
    with Path(p).open(encoding='utf-8-sig')as f:
        for line in f:yield json.loads(line)


def same(a,b):
    if type(a)is not type(b):return False
    if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
    if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
    return a==b


def address(w):
    a=ipaddress.ip_address(w)
    return str(a.ipv4_mapped)if a.version==6 and a.ipv4_mapped is not None else str(a)


def parent(r):
    w=r['wire'];link=r['link'];at=0;names=[];domain='';prefix=[]
    def add(label,n):
        nonlocal at
        assert n<=len(w)-at;prefix.append(dict(label=label,start=at,end=at+n));names.append(label);at+=n
    if link==1:add('Ethernet',14);kind=int.from_bytes(w[12:14],'big')
    elif link==113:add('Linux SLL',16);kind=int.from_bytes(w[14:16],'big')
    elif link==276:add('Linux SLL2',20);kind=int.from_bytes(w[:2],'big')
    elif link in[0,108]:
        add('Loopback',4);family=int.from_bytes(w[:4],'little')
        if family>255:family=int.from_bytes(w[:4],'big')
        assert family in[2,10,24,28,30];kind=0x800 if family==2 else 0x86dd
    elif link in[228,229]:kind=0x800 if link==228 else 0x86dd
    elif link==101:
        assert w and w[0]>>4 in[4,6];kind=0x800 if w[0]>>4==4 else 0x86dd
    else:raise AssertionError('unselected link profile')
    count=0
    while kind in[0x8100,0x88a8]:
        assert count<8 and at+4<=len(w);domain+='/vlan:'+str(int.from_bytes(w[at:at+2],'big')&4095);kind=int.from_bytes(w[at+2:at+4],'big');add('Dot1Q',4);count+=1
    start=at;limit=len(w);zero=False
    if kind==0x800:
        assert at+20<=limit and w[at]>>4==4;size=(w[at]&15)*4;total=int.from_bytes(w[at+2:at+4],'big');assert size>=20 and at+size<=limit and(total==0 or total>=size)and w[at+9]==6 and not int.from_bytes(w[at+6:at+8],'big')&0x3fff
        i=at+20
        while i<at+size:
            k=w[i]
            if k==0:break
            if k==1:i+=1;continue
            assert i+2<=at+size and w[i+1]>=2 and i+w[i+1]<=at+size;i+=w[i+1]
        zero=total==0
        if total:limit=min(limit,at+total)
        src,dst=address(w[at+12:at+16]),address(w[at+16:at+20]);label='IPv4';source_range=[at+12,at+16];dest_range=[at+16,at+20];at+=size
    else:
        assert kind==0x86dd and at+40<=limit and w[at]>>4==6 and w[at+6]==6;size=int.from_bytes(w[at+4:at+6],'big');assert size>0;limit=min(limit,at+40+size);src,dst=address(w[at+8:at+24]),address(w[at+24:at+40]);label='IPv6';source_range=[at+8,at+24];dest_range=[at+24,at+40];at+=40
    assert at<=limit;prefix.append(dict(label=label,start=start,end=at));return dict(own_offset=at,own_end=limit,names=names+[label,'TCP'],domain=domain,source=src,destination=dst,network_prefix=prefix,source_range=source_range,destination_range=dest_range,zero_total_legacy=zero)


def failure(w,loc,own_failure):
    at,end=loc['own_offset'],loc['own_end'];b=w[at:end]
    if len(b)<20:
        assert b,'zero-length payload without TCP decoder error is not part of this certificate'
        return dict(status='incomplete',reason=f'TCP fixed header incomplete ({len(b)}/20 bytes)',complete=False,required=(b[12]>>4)*4 if len(b)>=13 else 20,category='fixed-header-incomplete')
    f=own_failure(w,loc);assert f,'valid TCP or child-only error cannot be selected'
    category='invalid-data-offset'if f['required']<20 else'header-options-incomplete'if not f['complete']else'MPTCP-dimension'if f['reason'].startswith('MPTCP')else'option-TLV-bounds'
    return dict(f,category=category)


def projection(r,loc,f,stamp):
    w=r['wire'];b=w[loc['own_offset']:loc['own_end']];info=f['reason']
    info+=f"; captured TCP payload={len(b)-f['required']} bytes (header boundary observed)"if f['complete']else'; payload boundary unknown'
    if loc['zero_total_legacy']:info+='; IPv4 total length zero: legacy captured-prefix observation'
    if r['original_length']>r['captured_length']:info+=f" [truncated capture: {r['captured_length']}/{r['original_length']} bytes]"
    v=dict(capture_domain=dict(Section=r['section'],Interface=r['interface'],Encapsulation=loc['domain']),number=r['frame'],time=stamp(r['time_ns']),length=r['original_length'],captured_length=r['captured_length'],protocol='TCP',transport='tcp',source=loc['source'],destination=loc['destination'],layers=loc['names'],protocol_stack=loc['names'],protocol_status=f['status'],protocol_scope='captured TCP header diagnostic',visibility='headers',protocol_evidence='captured TCP header failure; segment not admitted to a connection or application',info=info)
    for name,at in [('source_port',0),('destination_port',2)]:
        if len(b)>=at+2:
            n=int.from_bytes(b[at:at+2],'big')
            if n:v[name]=n
    absent=['physical_source','physical_destination','tcp_flow_id','tcp_flow_domain','tcp_flow_state','message_source','message_destination','message_transport','message_domain','connection_protocol','connection_stack','connection_evidence','stream_id','conversation_id','transport_view']
    absent+= [k for k in['source_port','destination_port']if k not in v]
    return v,absent


def main():
    ap=argparse.ArgumentParser()
    for n in['before','after','capture-weber','capture-sharon','expected-module','audit-modules','out']:ap.add_argument('--'+n,type=Path,required=True)
    ap.add_argument('--source',required=True);a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path[:0]=[str(a.expected_module),str(a.audit_modules)]
    from expected import own_failure
    from pptp_parent_source_audit import stamp
    from vxlan_endpoint_projection_audit import capture_records
    results=[]
    for name,cap in [('weber',a.capture_weber),('sharon',a.capture_sharon)]:
        folder=a.out/name;folder.mkdir();newfile=a.after/'preservation'/name/'yak.jsonl';oldfile=a.before/'preservation'/name/'yak.jsonl';p=a.after/'preservation'/name/'yak-producer.json';producer=json.loads(p.read_text());assert producer['source']==a.source and producer['exit_code']==0 and producer['capture_SHA256']==sha(cap)and producer['export_SHA256']==sha(newfile)
        selected={x['number']:x for x in rows(newfile)if x.get('protocol_scope')=='captured TCP header diagnostic'};old={x['number']:x for x in rows(oldfile)if x.get('number')in selected};assert old.keys()==selected.keys()
        ledger=[];errors=[];kinds=collections.Counter();short=collections.Counter();metadata=collections.Counter();statuses=collections.Counter();before_protocols=collections.Counter()
        for n,r in enumerate(capture_records(cap),1):
            if n not in selected:continue
            r['frame']=n;row=selected[n]
            try:loc=parent(r);f=failure(r['wire'],loc,own_failure);wanted,absent=projection(r,loc,f,stamp)
            except AssertionError as e:errors.append(dict(frame=n,reason='literal source cannot establish selected ownTCP failure: '+str(e)));continue
            assert all(k in row and same(row[k],v)for k,v in wanted.items()),(name,n,'source-owned projection changed')
            assert not any(k in row for k in absent),(name,n,'connection/source field leaked into rejected TCP')
            extra={k:v for k,v in row.items()if k not in wanted};old_extra={k:v for k,v in old[n].items()if k not in wanted}
            # These fields are explicitly inventoried and remain the root
            # bridge's responsibility; they are never silently normalized.
            extra_changes=[k for k in sorted(extra.keys()|old_extra.keys())if k not in extra or k not in old_extra or not same(extra[k],old_extra[k])]
            for k in extra_changes:metadata[k]+=1
            kinds[f['category']]+=1;statuses[f['status']]+=1;before_protocols[old[n]['protocol']]+=1
            if len(r['wire'][loc['own_offset']:loc['own_end']])<20:short[loc['own_end']-loc['own_offset']]+=1
            ledger.append(dict(frame=n,wire_SHA256=hashlib.sha256(r['wire']).hexdigest(),record={k:v for k,v in r.items()if k!='wire'},literal_location=loc,own_failure=f,source_owned_projection=wanted,absent_unsafe_fields=absent,source_owned_projection_exact=True,inherited_metadata_changed_keys=extra_changes,inherited_metadata_before=old_extra,inherited_metadata_after=extra,whole_before_row=old[n],whole_after_row=row))
        assert len(ledger)+len(errors)==len(selected)
        (folder/'source-classification.jsonl').write_text(''.join(json.dumps(x,sort_keys=True)+'\n'for x in ledger),encoding='utf8');(folder/'uncovered.jsonl').write_text(''.join(json.dumps(x,sort_keys=True)+'\n'for x in errors),encoding='utf8')
        result=dict(corpus=name,sources=len(selected),source_bound=len(ledger),uncovered=len(errors),categories=dict(kinds),statuses=dict(statuses),short_fixed_header_lengths=dict(sorted(short.items())),old_protocols=dict(before_protocols),inherited_metadata_changes_explicit=dict(metadata),capture_SHA256=sha(cap),before_CLI_SHA256=sha(oldfile),after_CLI_SHA256=sha(newfile),producer_SHA256=sha(p),ledger_SHA256=sha(folder/'source-classification.jsonl'),uncovered_SHA256=sha(folder/'uncovered.jsonl'));results.append(result);print(json.dumps(result),flush=True)
    summary=dict(scope=__doc__,source=a.source,corpora=results,sources=sum(x['sources']for x in results),source_bound=sum(x['source_bound']for x in results),uncovered=sum(x['uncovered']for x in results),auditor_SHA256=sha(Path(__file__)),contract_constructor_SHA256=sha(a.expected_module/'expected.py'),note='Inherited native/PDU fields are explicit values and diff keys in the ledger; they are not claimed whole-preserved until the root generated-ID/source bridge validates them.')
    p=a.out/'summary.json';p.write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(p),SHA256=sha(p))),flush=True)


if __name__=='__main__':main()
