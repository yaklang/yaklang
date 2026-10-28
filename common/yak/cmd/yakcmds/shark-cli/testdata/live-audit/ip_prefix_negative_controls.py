"""Corrupt external prefix evidence and require semantic rejection.

The receipts of intentionally changed SDK/reference exports are recalculated so
the controls exercise value, type, source and graph checks beyond file hashes.
Original capture files, exports and positive proofs are never overwritten.
"""
import argparse,copy,hashlib,json
from pathlib import Path
from ip_short_header_source_audit import audit as source_audit
from ip_short_header_wire_audit import audit as wire_audit
from ip_prefix_protocol_preservation_audit import audit as protocol_audit
from ip_prefix_reference_preservation_audit import audit as reference_audit


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def load(p):return json.loads(p.read_text('utf8'))
def rows(p):return[json.loads(l)for l in p.open(encoding='utf8')]
def save(p,value,lines=False):
    p.write_text(''.join(json.dumps(v)+'\n'for v in value)if lines else json.dumps(value),encoding='utf8');return p
def find(tree,key,predicate=lambda v:True):
    if isinstance(tree,dict):
        if key in tree and predicate(tree[key]):return tree,key
        children=tree.values()
    elif isinstance(tree,list):children=tree
    else:return None
    for child in children:
        result=find(child,key,predicate)
        if result is not None:return result
    return None
def change(tree,key,value,predicate=lambda v:True):
    found=find(tree,key,predicate);assert found is not None,('missing mutation witness',key)
    parent,k=found;parent[k]=value(parent[k]);return True


def run(root,out):
    for p in[root,out]:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    out.mkdir(exist_ok=False);rejections=[];positives=[]
    def reject(group,name,invoke):
        try:invoke()
        except(AssertionError,KeyError,ValueError,TypeError,IndexError)as e:rejections.append(dict(group=group,control=name,rejected=True,reason=type(e).__name__+': '+str(e)[:400]))
        else:raise AssertionError(('forged evidence accepted',group,name))
    case=root/'controls-02/original-five';sdk=root/'control-parent-01/original-five';ref=root/'control-replay-01/original-five'
    args=[case/'observation.pcapng',sdk/'events.jsonl',sdk/'yak.jsonl',ref/'oracle.json',ref/'labels.tsv']
    source_audit(*args);positives.append('source')
    plan=[
        ('boolean-observed-version',lambda e,c:e[0]['session'].update({'Observed IP Version':True})),
        ('wrong-required-bytes',lambda e,c:e[0]['session'].update({'Required Header Bytes':40})),
        ('wrong-available-bytes',lambda e,c:e[0]['session'].update({'Available Header Bytes':5})),
        ('false-complete-fields',lambda e,c:e[0].update(fields={'TLS':{'Version':771}})),
        ('false-TCP-generation',lambda e,c:e[0].update(flow=1,tcp_flow_id=1)),
        ('wrong-source-interface',lambda e,c:e[0]['domain'].update(Interface=1)),
        ('duplicate-contribution',lambda e,c:e.append(copy.deepcopy(e[0]))),
        ('child-header-splice',lambda e,c:e[0]['session']['Observed Header Layers'].extend(['TCP','TLS'])),
        ('CLI-confirmed-TLS',lambda e,c:c[0].update(protocol='TLS')),
        ('CLI-stale-connection',lambda e,c:c[0].update(connection_protocol='TLS',tcp_flow_id=1)),
        ('CLI-complete-status',lambda e,c:c[0].update(protocol_status='decoded')),
        ('CLI-child-stack',lambda e,c:c[0]['protocol_stack'].append('TLS')),
    ]
    for name,mutate in plan:
        d=out/('source-'+name);d.mkdir();e=rows(args[1]);c=rows(args[2]);mutate(e,c)
        reject('source',name,lambda:source_audit(args[0],save(d/'events.jsonl',e,True),save(d/'cli.jsonl',c,True),*args[3:]))
    case=root/'controls-02/QUIC-short-future';manifest=case/'manifest.json';capture=case/'observation.pcapng';wire=root/'control-proofs-09/QUIC-short-future-wire.json'
    wire_audit(capture,manifest);positives.append('wire')
    doc=load(manifest);prefix=next(i for i,e in enumerate(doc['ledger'])if e['change']['kind']=='incomplete-fixed-header-observation')
    plan=[
        ('wrong-source-time',lambda d:d['ledger'][0].update(source_time_ns=d['ledger'][0]['source_time_ns']+1)),
        ('wrong-observed-time',lambda d:d['ledger'][prefix].update(time_ns=d['ledger'][prefix]['time_ns']+1)),
        ('boolean-original-frame',lambda d:d['ledger'][0].update(source_frame=True)),
        ('duplicate-original-owner',lambda d:d['ledger'][1].update(source_frame=d['ledger'][0]['source_frame'])),
        ('wrong-source-domain',lambda d:d['ledger'][0].update(source_interface=1)),
        ('wrong-prefix-interval',lambda d:d['ledger'][prefix]['change'].update(available_fixed_header_bytes=0)),
        ('forged-clock-interpretation',lambda d:d['ledger'][prefix]['change'].update(timestamp_interpretation='literal PCAP epoch zero')),
        ('missing-original-record',lambda d:d['ledger'].pop()),
    ]
    for name,mutate in plan:
        d=out/('wire-'+name);d.mkdir();value=copy.deepcopy(doc);mutate(value);p=save(d/'manifest.json',value)
        reject('wire',name,lambda:wire_audit(capture,p))
    after=root/'control-parent-01/QUIC-short-future';before=root.parent/'yak-shark-IP-admission-20261028/native-preservation-04/owned/fresh-QUIC'
    protocol_audit(before/'events.jsonl',before/'events-producer.json',after/'events.jsonl',after/'events-producer.json',manifest,wire);positives.append('native-graph')
    original=rows(after/'events.jsonl');other_id=next(e['id']for e in original if e['protocol']=='quic')
    def parent_splice(e):change(e,'Parent PDU',lambda v:other_id if v!=other_id else other_id+1,lambda v:type(v)is int and v>0)
    plan=[
        ('wire-stream-ID',lambda e:change(e,'Stream ID',lambda v:v+4,lambda v:type(v)is int)),
        ('wire-stream-offset',lambda e:change(e,'Offset',lambda v:v+1,lambda v:type(v)is int)),
        ('wire-packet-number',lambda e:change(e,'Packet Number',lambda v:v+1,lambda v:type(v)is int)),
        ('false-authentication',lambda e:change(e,'Authentication Verified',lambda v:False,lambda v:v is True)),
        ('generated-parent-splice',parent_splice),
        ('generated-request-splice',lambda e:change(e,'Request PDU ID',lambda v:other_id,lambda v:type(v)is int and v>0 and v!=other_id)),
        ('boolean-frame-reference',lambda e:change(e,'Number',lambda v:True,lambda v:type(v)is int and v>0)),
        ('false-native-flow',lambda e:next(v for v in e if v['protocol']!='ip').update(flow=999999)),
        ('missing-message',lambda e:e.pop()),
    ]
    for name,mutate in plan:
        d=out/('graph-'+name);d.mkdir();e=copy.deepcopy(original);mutate(e);p=save(d/'events.jsonl',e,True);receipt=load(after/'events-producer.json');receipt['export_SHA256']=sha(p);rp=save(d/'producer.json',receipt)
        reject('native-graph',name,lambda:protocol_audit(before/'events.jsonl',before/'events-producer.json',p,rp,manifest,wire))
    baseline=root/'owned-baseline-reference-01/QUIC';current=root/'control-replay-01/QUIC-short-future';schema=root/'reference-field-schema-01.json'
    reference_audit(baseline/'oracle.json',current/'oracle.json',manifest,wire,baseline/'producer.json',current/'oracle-producer.json',schema);positives.append('reference')
    reference=load(current/'oracle.json');framefields=set(load(schema)['frame_reference_fields'])
    def wire_descriptor(v,index,value):
        parent,key=find(v,'ip.version_raw');descriptor=parent[key];assert isinstance(descriptor,list)and len(descriptor)==6;descriptor[index]=value(descriptor[index])
    def generated(v):
        def walk(node):
            if isinstance(node,dict):
                for k,value in node.items():
                    if k in framefields and k+'_raw'in node and type(value)is str and value.isdecimal()and int(value)>0:
                        node[k]=str(prefix+1);return True
                    if k in framefields and k+'_raw'in node and isinstance(value,list)and value and all(type(x)is str and x.isdecimal()for x in value):
                        value[0]=str(prefix+1);return True
                return any(walk(x)for x in node.values())
            if isinstance(node,list):return any(walk(x)for x in node)
            return False
        assert walk(v)
    plan=[
        ('wire-version-scalar',lambda v:change(v,'ip.version',lambda x:'6')),
        ('raw-offset',lambda v:wire_descriptor(v,1,lambda x:x+1)),
        ('raw-mask',lambda v:wire_descriptor(v,3,lambda x:x^1)),
        ('raw-data-source-index',lambda v:wire_descriptor(v,5,lambda x:x+1)),
        ('raw-boolean-offset',lambda v:wire_descriptor(v,1,lambda x:True)),
        ('absolute-TCP-sequence-scalar',lambda v:change(v,'tcp.seq_raw',lambda x:str(int(x)+1),lambda x:type(x)is str and x.isdecimal())),
        ('generated-reference-to-prefix',generated),
        ('missing-original-frame',lambda v:v.pop()),
    ]
    for name,mutate in plan:
        rb,rc,rm,rw=baseline,current,manifest,wire
        if name=='absolute-TCP-sequence-scalar':
            rb=root/'owned-baseline-reference-01/TLS';rc=root/'control-replay-01/TLS-short-future';rm=root/'controls-02/TLS-short-future/manifest.json';rw=root/'control-proofs-09/TLS-short-future-wire.json'
        d=out/('reference-'+name);d.mkdir();value=load(rc/'oracle.json');mutate(value);p=save(d/'oracle.json',value);receipt=load(rc/'oracle-producer.json');receipt['export_SHA256']=sha(p);rp=save(d/'producer.json',receipt)
        reject('reference',name,lambda:reference_audit(rb/'oracle.json',p,rm,rw,rb/'producer.json',rp,schema))
    proof=dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),positive_groups=positives,rejected=len(rejections),controls=rejections,semantic_export_receipts_rehashed=True)
    save(out/'proof.json',proof);print(json.dumps(dict(rejected=len(rejections),positive_groups=positives)),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--evidence-root',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();run(a.evidence_root,a.out)
