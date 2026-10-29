"""Verify closed boundary native/CLI fields from independent source grid.

All partial/error leaves come from Session and private field_values. The
existing GetFields null/error contract is preserved. This is a strict source
field/range/identity/progression proof, not a full unrelated-event certificate.
"""
import argparse,base64,collections,hashlib,json,traceback
from pathlib import Path


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def rows(p):return [json.loads(x)for x in Path(p).open(encoding='utf-8-sig')]
def same(a,b):
    if type(a)is not type(b):return False
    if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
    if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
    return a==b
def node_at(root,path):
    for key in path:root=root[key]
    return root
def range_grid(value,path=()):
    result={}
    if isinstance(value,dict):
        for k,v in value.get('Wire Field Ranges',{}).items():result[path+(k,)]=v
        for k,v in value.items():
            if k!='Wire Field Ranges':result.update(range_grid(v,path+(k,)))
    elif isinstance(value,list):
        for i,v in enumerate(value):result.update(range_grid(v,path+(i,)))
    return result


def verify_session(frozen,session,meta):
    f=frozen['source_fields'];expected=f['source_wire_tree']
    assert isinstance(session,dict)
    assert set(session['Header'])==set(expected['Header']),'no unobserved Header scalar'
    for leaf in f['source_leaves']:
        assert same(node_at(session,leaf['path']),leaf['JSON_value']),('source scalar',leaf['path'])
    assert same(range_grid(session),range_grid(expected)),'every actual wire-range leaf must equal independent source grid'
    for name in('LSAs','Hello'):
        if name not in expected:assert name not in session,('body guessed beyond admitted source context',name)
    if 'LSAs'in expected:
        assert isinstance(session['LSAs'],list)and len(session['LSAs'])==len(expected['LSAs'])
        assert all(isinstance(v,dict)for v in session['LSAs']),'nil is not an observed LSA'
        for old,new in zip(expected['LSAs'],session['LSAs']):
            assert set(new)==set(old)|{'Checksum Verified','Checksum Scope'},'no unobserved LSA scalar or missing observed prefix'
            if 'Body'not in old:assert 'Body'not in new,'no Body without complete observed Length/boundary'
    if 'Hello'in expected:assert set(session['Hello'])==set(expected['Hello']),'no unobserved Hello scalar'
    for spec in f['structural_requirements']:
        if 'exact_keys'in spec:assert set(node_at(session,spec['path']))==set(spec['exact_keys'])
        if spec.get('required'):
            value=node_at(session,spec['path']);raw=Path(frozen['original_frame']).read_bytes()[54:];start,end=spec['owned_bytes_range']
            assert base64.b64decode(value)==raw[start:end],'owned unparsed bytes must remain observable without invented semantics'
    for path,node in walk(expected):
        for key,value in node.items():
            if value is None:assert node_at(session,list(path)+[key])is None,('zero source collection must remain nil/null',path,key)
    for check in f['checksum_requirements']:
        node=node_at(session,check['path']);assert type(node['Checksum Verified'])is bool and node['Checksum Verified']==check['verified']
        if check['scope'].startswith('unverified-'):
            assert node['Checksum Scope'].startswith('unverified:'),'partial/invalid LSA cannot claim checksum validation'
        else:assert node['Checksum Scope']==check['scope'],('checksum applicability',check['path'])
    assert session['Authentication Verified']is False and session['Packet Name']==f['packet_name']
    assert session['Observation Scope']=='bounded wire advertisements only; no LSDB, verified neighbor or installed route'
    bits=f['expected_packet_bits']
    if bits is not None and(bits['AT']or bits['LLS']):
        assert session['Authentication Trailer Bit']is bits['AT']and session['LLS Bit']is bits['LLS']
        raw=Path(frozen['original_frame']).read_bytes()[54:];n=int.from_bytes(raw[2:4],'big')
        assert base64.b64decode(session['Trailing Bytes'])==raw[n:]
        if bits['LLS']and not bits['AT']:assert session['LLS Scope']==meta['shared']['LLS Scope']
    if 'Unparsed Body'in session:
        raw=Path(frozen['original_frame']).read_bytes()[54:];end=min(len(raw),int.from_bytes(raw[2:4],'big'))
        assert base64.b64decode(session['Unparsed Body'])==raw[f['parsed_PDU_cursor']:end]
    return len(f['source_leaves'])


def verify_aliases(frozen,values):
    actual=collections.defaultdict(list);generic=collections.defaultdict(list)
    for item in values:
        if item['name'].startswith('ospf'):actual[item['name']].append(item)
        else:generic[item['name']].append(item)
    assert set(actual)==set(frozen['expected_private_typed_aliases'])
    for name,expected in frozen['expected_private_typed_aliases'].items():assert same(actual[name],expected),('private alias/range/duplicates',name)
    return actual,generic


def walk(value,path=()):
    if isinstance(value,dict):
        yield path,value
        for k,v in value.items():
            if k!='Wire Field Ranges':yield from walk(v,path+(k,))
    elif isinstance(value,list):
        for i,v in enumerate(value):yield from walk(v,path+(i,))


def verify_event(frozen,event,meta):
    f=frozen['source_fields'];session=event['session'];count=verify_session(frozen,session,meta)
    expected_status=f['native_refusal_status'];assert event['status']==expected_status,(event['status'],expected_status)
    expert=meta['native']['status_to_expert'][expected_status]
    assert event['expert']==expert and event['completeness']==expected_status
    assert event['fields']is None and event['fields_error']==event['error'],'GetFields error API is not Session'
    assert isinstance(event['error'],str)and event['error'].startswith(expert+': ')
    assert event['protocol']=='ospf'and event['transport']=='network'and event['profile']=='ospfv3-bounded-control'
    assert event['entry']=='OSPFv3ControlFields'and event['admission']==meta['native']['admission_on_refusal']
    for key in('flow','transaction','response_to'):assert type(event[key])is int and event[key]==0,('inherited association',key)
    assert event.get('tcp_flow_id',0)==0 and event['source']=='fe80::100'and event['destination']=='ff02::5'
    assert event['id']>0 and type(event['id'])is int
    raw=Path(frozen['original_frame']).read_bytes()[54:]
    assert base64.b64decode(event['raw'])==raw
    source=event['source_bytes'];assert source['Kind']=='captured'and source['SHA256']==hashlib.sha256(raw).hexdigest()
    assert source['ParentPDU']==0 and not source['ParentPDUs']
    assert source['PacketRefs']==[dict(Number=1,Domain=dict(Section=0,Interface=0,Encapsulation=''))]
    snapshot=event['field_values'];assert snapshot['schema_version']==1 and snapshot['pdu']==event['id']and snapshot['source']==source
    assert snapshot['completeness']==snapshot_completeness_policy(snapshot,expected_status)
    assert not snapshot.get('truncated'),'all source aliases are within the field-value cap'
    actual,generic=verify_aliases(frozen,snapshot['values'])
    for name,value in(('flow.id',0),('pdu.id',event['id']),('pdu.length',len(raw))):
        assert len(generic[name])==1 and type(generic[name][0]['value'])is int and generic[name][0]['value']==value
        assert generic[name][0]['exact_value']==str(value)
    assert [x['value']for x in generic['protocol']]==['ospf']and [x['value']for x in generic['status']]==[expected_status]
    allowed={'destination','source','ip.addr','protocol','status','flow.id','pdu.id','pdu.length','pdu.completed_in','pdu.response_to'}
    assert set(generic)<=allowed,'no foreign protocol fields or inherited association aliases'
    if 'pdu.response_to'in generic:assert [x['value']for x in generic['pdu.response_to']]==[0]
    return count,sum(len(v)for v in actual.values())


def snapshot_completeness_policy(snapshot,event_completeness):
    # Existing public API separates field coverage from the event wire outcome.
    # This validates the exported flag policy without inventing its private flag.
    incomplete=snapshot.get('incomplete',False)
    assert type(incomplete)is bool
    return 'limited'if incomplete and event_completeness!='evicted'else event_completeness


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('grid','metadata-contract','actual','producer-controller','out'):ap.add_argument('--'+n,type=Path,required=True)
    ap.add_argument('--source',required=True);a=ap.parse_args();assert not a.out.exists();a.out.mkdir()
    try:
        plan=read(a.grid/'plan.json');assert sha(a.grid/'plan.json')=='8b77033c3bd933124f15b0af9c80e3823b86c64510fe6cf9dec3c64c9ed4280d'
        assert sha(a.metadata_contract)==plan['metadata_contract_SHA256'];meta=read(a.metadata_contract)
        data=Path(plan['data_file']);assert sha(data)==plan['data_SHA256'];grid={r['case']:r for r in read(data)}
        summary=read(a.actual/'summary.json');assert summary['source']==a.source and summary['authored_not_real_traffic']is True
        source_plans={
            '8603f16a3d23a7bb4c62741bb2b3bb623822757e71f273b110d25f91932bf21e':'frozen-source01',
            '83aa407034d0f21f8ffab5217f6a6b990bbf7e2d2cddb678ff8b2d25d23cac19':'frozen-source02',
            'e9f5b8e4f2820bf12ca79bf7049562442f0bec7540d1a910c7dd2d11da570cd0':'frozen-source03'}
        source_root=a.grid.parent/source_plans[summary['source_plan_SHA256']]
        assert sha(source_root/'plan.json')==summary['source_plan_SHA256']
        sp=read(source_root/'plan.json');assert sha(source_root/'cases.json')==sp['cases_SHA256']
        cases={c['name']:c for c in read(source_root/'cases.json')}
        ordinary={n:c for n,c in cases.items()if not c.get('requires_direct_codec_invocation')}
        direct=sorted(n for n,c in cases.items()if c.get('requires_direct_codec_invocation'))
        assert summary['physical_cases']==len(ordinary) and len(summary['executions'])==2*len(ordinary)
        assert sorted(summary['skipped_direct_cases'])==direct
        assert collections.Counter(r['case']for r in summary['executions'])==collections.Counter({n:2 for n in ordinary})
        assert all((a.actual/n).is_dir()for n in ordinary),'runtime cannot silently omit a claimed ordinary case'
        bound={str(p):sha(p)for p in[Path(__file__),a.producer_controller,a.metadata_contract,a.grid/'plan.json',data,a.actual/'summary.json',source_root/'plan.json',source_root/'cases.json']}
        for c in cases.values():
            for k in('capture','original_frame','source_facts'):
                assert sha(c[k])==c[k+'_SHA256'];bound[c[k]]=c[k+'_SHA256']
        for name,c in ordinary.items():
            assert name in grid and all(grid[name][k+'_SHA256']==c[k+'_SHA256']for k in('capture','original_frame','source_facts'))
        results=[];failures=[];skipped=[];totals=[0,0]
        for name,frozen in grid.items():
            if frozen['direct_codec_invocation_required']:skipped.append(name);continue
            if name not in ordinary:continue
            folder=a.actual/name
            try:
                for kind,filename in(('events','events.jsonl'),('yak','yak.jsonl')):
                    receipt=read(folder/(kind+'-receipt.json'));export=folder/filename
                    assert receipt in summary['executions']and receipt['source']==a.source and receipt['exit_code']==0
                    assert receipt['capture_SHA256']==frozen['capture_SHA256']and receipt['export_SHA256']==sha(export)
                    assert receipt['source_facts_SHA256']==frozen['source_facts_SHA256'] and receipt['source_plan_SHA256']==summary['source_plan_SHA256']
                    assert receipt['controller_SHA256']==sha(a.producer_controller)
                    binary=Path(receipt['arguments'][0]);build_path=binary.parent/(kind+'-receipt.json');build=read(build_path)
                    assert sha(binary)==receipt['binary_SHA256']==build['binary_SHA256']
                    assert sha(build_path)==receipt['build_receipt_SHA256'] and build['source']==a.source and build['exit_code']==0
                    capture=cases[name]['capture']
                    expected_args=[str(binary),'replay',capture,'-','--ignore-embedded-secrets']if kind=='events'else[str(binary),'shark','-r',capture,'--json','--ignore-embedded-secrets']
                    assert receipt['arguments']==expected_args and receipt['direct_codec_overrides_claimed']is False
                    for p in(folder/(kind+'-receipt.json'),export,binary,build_path):bound[str(p)]=sha(p)
                events=rows(folder/'events.jsonl');assert len(events)==1
                count,typed=verify_event(frozen,events[0],meta);packet=rows(folder/'yak.jsonl');assert len(packet)==1;row=packet[0]
                assert row['number']==1 and row['protocol']=='OSPFv3'and row['protocol_status']==frozen['source_fields']['native_refusal_status']
                layers=row['layers'];assert layers[:2]==['Ethernet','IPv6']and layers.count('OSPF')==1
                assert all(l in('Ethernet','IPv6','OSPF','DecodeFailure')for l in layers)and layers.count('DecodeFailure')<=1
                assert row['protocol_stack']==['OSPFv3'if l=='OSPF'else l for l in layers],'replace OSPF once;preserve actual diagnostic chain'
                if frozen['source_fields']['native_refusal_status']=='context-required':assert 'DecodeFailure'not in layers,'soft unsupported semantics is not damaged captured wire'
                assert not row.get('conversation_id')and not row.get('tcp_flow_id')and not row.get('connection_protocol')
                assert 'YAML dissector incomplete'not in row['info']and row['source']=='fe80::100'and row['destination']=='ff02::5'
                assert len(row['pdus'])==1 and row['pdus'][0]['id']==events[0]['id']
                assert same(row['pdus'][0]['field_values'],events[0]['field_values'])
                totals[0]+=count;totals[1]+=typed
                results.append(dict(case=name,source_leaves=count,private_typed_occurrences=typed,source_fields_ranges_checksums_identity_progression_checked=True,events_SHA256=sha(folder/'events.jsonl'),CLI_SHA256=sha(folder/'yak.jsonl')))
            except Exception as e:failures.append(dict(case=name,error=repr(e),traceback=traceback.format_exc()))
        assert len(results)+len(failures)==len(ordinary)
        after={p:sha(p)for p in bound};assert bound==after,'a frozen source,producer or export changed during verification'
        (a.out/'inputs-before.json').write_text(json.dumps(bound,indent=2)+'\n',encoding='utf8')
        (a.out/'inputs-after.json').write_text(json.dumps(after,indent=2)+'\n',encoding='utf8')
        report=dict(scope=__doc__,source=a.source,source_grid_plan_SHA256=sha(a.grid/'plan.json'),metadata_contract_SHA256=sha(a.metadata_contract),
            actual_summary_SHA256=sha(a.actual/'summary.json'),checked_cases=len(results)+len(failures),passed_cases=len(results),failed_cases=len(failures),
            source_leaves=totals[0],private_typed_occurrences=totals[1],results=results,failures=failures,direct_cases_not_tested=skipped,
            candidate_values_used_for_expectations=False,whole_export_certificate=False,driver_SHA256=sha(__file__),
            actual_source_plan_SHA256=summary['source_plan_SHA256'],ordinary_cases_in_source_grid_not_in_this_producer=sorted(set(grid)-set(cases)),
            inputs_before_SHA256=sha(a.out/'inputs-before.json'),inputs_after_SHA256=sha(a.out/'inputs-after.json'))
        (a.out/'summary.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf8')
        print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),passed=len(results),failed=len(failures))),flush=True)
        if failures:raise SystemExit(1)
    except Exception:
        (a.out/'controller-failure.json').write_text(json.dumps(dict(error=traceback.format_exc()),indent=2)+'\n',encoding='utf8');raise


if __name__=='__main__':main()
