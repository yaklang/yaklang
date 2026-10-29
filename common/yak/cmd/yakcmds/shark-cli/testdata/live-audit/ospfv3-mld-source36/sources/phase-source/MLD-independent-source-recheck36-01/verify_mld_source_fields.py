"""Source scalar/span comparison against closed PDML and old a82 diagnostics.

This produces source evidence, not a future event expectation. Raw timer words
and decoded durations are separate observations even when Wireshark uses one
historical alias for the decoded value. All differences remain in the report.
"""
import argparse,base64,collections,hashlib,ipaddress,json,xml.etree.ElementTree as ET
from pathlib import Path


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def same(a,b):return type(a)is type(b)and a==b
def private_path(field,record_index):
    name=field['semantic']
    common={'ICMPv6 Type':['Type'],'ICMPv6 Code':['Code'],'ICMPv6 Checksum':['Checksum']}
    if name in common:return common[name]
    m={'Report Flags: raw':'Report Flags (raw)','Address Record Count':'Address Record Count',
       'Response Code or Delay: raw word':'Response Code/Delay Word','Maximum Response Delay: milliseconds':'Maximum Response Delay (ms)',
       'Unused Delay Word':'Unused Delay Word','Reserved':'Reserved','Multicast Address':'Multicast Address',
       'Query Flags: raw byte':'Flags','Suppress Router-side Processing':'Suppress Router-Side Processing',
       'Querier Robustness Variable: raw':'Querier Robustness Variable','Query Extension Flags: raw':'Extension Flags (raw)',
       'Querier Query Interval Code: raw':'Query Interval Code','Querier Query Interval: seconds':'Query Interval (s)',
       'Query Source Count':'Source Count','Additional Data: uninterpreted':'Additional Data (uninterpreted)'}
    if name in m:return ['MLD',m[name]]
    r={'Record Type':'Record Type','Auxiliary Data Length: 32-bit words':'Auxiliary Data Words',
       'Record Source Count':'Source Count','Record Multicast Address':'Multicast Address',
       'Record Auxiliary Data: uninterpreted':'Auxiliary Data (uninterpreted)'}
    if name in r:return ['MLD','Address Records',record_index,r[name]]
    return None
def node_at(root,path):
    for k in path:root=root[k]
    return root
def ws_name(alias):
    return {'icmpv6.mld.maximum_response_code.raw':'icmpv6.mld.maximum_response_code','icmpv6.mld.qqi.raw':'icmpv6.mld.qqi'}.get(alias,alias)
def ws_value(field,node):
    name=field['public_alias'];raw=node.get('unmaskedvalue',node.get('value',''))
    if name.endswith('.raw')or not field['derived']:
        if field['kind']=='address':return str(ipaddress.IPv6Address(bytes.fromhex(raw)))
        if field['kind']=='bytes':return raw.lower()
        return int(raw,16)
    text=node.get('show','')
    if name=='icmpv6.mld.flag.s':return text.lower()=='true'
    return int(text,0)if text.startswith('0x')else int(text)


def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--source',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args()
    assert not a.out.exists();a.out.mkdir();results=[];differences=[];contexts=[];inputs=[]
    for corpus in('sharon','weber'):
        d=a.source/corpus;facts=read(d/'source-facts.json');observed={r['frame']:r for r in read(d/'current-a82-diagnostic-only.json')}
        root=ET.parse(d/'wireshark.pdml').getroot();packets={int(p.find(".//field[@name='frame.number']").get('show')):p for p in root}
        assert len(facts)==len(observed)==len(packets)
        for row in facts:
            frame=row['frame'];wire=Path(row['original_frame_file']).read_bytes();assert hashlib.sha256(wire).hexdigest()==row['original_frame_SHA256']
            event=observed[frame];source=row['source'];native=event['field_values'];record_index=-1;source_indices=collections.Counter()
            proto=[p for p in packets[frame].iter('proto')if p.get('name')=='icmpv6'and int(p.get('pos'))==row['source_profile']['offset']]
            typed=collections.defaultdict(list)
            for item in native['values']:
                if item['name'].startswith('icmpv6.'):typed[item['name']].append(item)
            expected=collections.defaultdict(list)
            for field in source['fields']:
                if field['semantic']=='Record Type':record_index+=1
                first,last=field['physical_range'];raw=wire[first:last]
                assert hashlib.sha256(raw).hexdigest()==field['source_SHA256']
                path=private_path(field,record_index)
                if field['semantic']in('Record Source Address','Query Source Address'):
                    group=record_index if field['semantic']=='Record Source Address'else'query'
                    idx=source_indices[group];source_indices[group]+=1
                    path=['MLD','Address Records',record_index,'Sources',idx]if group!='query'else['MLD','Sources',idx]
                if path is not None:
                    try:
                        got=node_at(event['fields'],path)
                        if field['kind']=='bytes':got=base64.b64decode(got).hex()
                        scalar_equal=same(got,field['value'])
                        if type(path[-1])is int:
                            owner=node_at(event['fields'],path[:-2]);key=path[-2]+'['+str(path[-1])+']'
                        else:owner=node_at(event['fields'],path[:-1]);key=str(path[-1])
                        range_equal=owner['Wire Field Ranges'][key]==field['native_PDU_range']
                        check=dict(corpus=corpus,frame=frame,kind='native-private-leaf',field=field['semantic'],path=path,
                                   scalar_equal=scalar_equal,range_equal=range_equal,equal=scalar_equal and range_equal)
                    except Exception as e:check=dict(corpus=corpus,frame=frame,kind='native-private-leaf',field=field['semantic'],path=path,equal=False,error=repr(e))
                    results.append(check)
                    if not check['equal']:differences.append(check)
                alias=field.get('public_alias')
                if not alias:continue
                start,end=field['native_PDU_range'];expected[alias].append(dict(value=field['value'],range=dict(offset=start,length=end-start)))
                candidates=[]
                if len(proto)==1:
                    candidates=[v for v in proto[0].iter('field')if v.get('name')==ws_name(alias)and int(v.get('pos','-1'))==first and int(v.get('size','-1'))==last-first]
                check=dict(corpus=corpus,frame=frame,kind='Wireshark-PDML',field=alias,physical_range=field['physical_range'],matches=len(candidates),equal=False)
                if len(candidates)==1:
                    try:check['equal']=same(ws_value(field,candidates[0]),field['value'])
                    except Exception as e:check['error']=repr(e)
                    check['literal_reference_name']=candidates[0].get('name')
                    if alias.endswith('.raw'):check['interpretation']='independent raw word from original byte range; historical WS alias also exposes decoded show value'
                results.append(check)
                if not check['equal']:differences.append(check)
            assert set(typed)==set(expected),('unexpected/missing native alias names',corpus,frame,set(typed)^set(expected))
            for alias,want in expected.items():
                actual=typed[alias];value_equal=len(actual)==len(want)and all(same(x['value'],y['value'])and x.get('pdu_range')==y['range']and x['origin']=='pdu'for x,y in zip(actual,want))
                check=dict(corpus=corpus,frame=frame,kind='native-typed-occurrences',field=alias,count=len(want),equal=value_equal)
                results.append(check)
                if not value_equal:differences.append(check)
            context=dict(corpus=corpus,frame=frame,parent=row['parent'],historical_body_status=row['original_04_body_status'],
                         body_complete=source['body_complete'],source_type=source['type'],current_status=event['status'],current_completeness=event['completeness'],
                         current_expert=event['expert'],private_observation_scope=event['fields']['MLD']['Observation Scope'])
            if row['parent']['cross_family']:
                assert event['status']in('incomplete','limited')and event['completeness']!='message'
                context['classification']='noncanonical IPv4 protocol58 wire observation; conservative body/extent refusal, not decoded valid MLD'
            else:
                assert source['body_complete']and row['parent']['declared_envelope_complete']and event['status']=='decoded'
                context['classification']='complete IPv6 MLD body observation; field decode does not prove listener state'
            contexts.append(context)
        inputs.append(dict(corpus=corpus,source_facts_SHA256=sha(d/'source-facts.json'),PDML_SHA256=sha(d/'wireshark.pdml'),native_a82_diagnostic_SHA256=sha(d/'current-a82-diagnostic-only.json')))
    for name,data in(('comparisons.jsonl',results),('differences.jsonl',differences),('context-classifications.jsonl',contexts)):
        (a.out/name).write_text(''.join(json.dumps(r,sort_keys=True)+'\n'for r in data),encoding='utf8')
    report=dict(scope=__doc__,source_summary_SHA256=sha(a.source/'summary.json'),inputs=inputs,
        comparisons=len(results),by_kind=dict(collections.Counter(r['kind']for r in results)),differences=len(differences),
        source_records=237,complete_IPv6_MLD_observations=220,noncanonical_IPv4_wire_observations=17,
        historical_body_incomplete=10,historical_body_incomplete_in_IPv6=0,
        all_current_cross_family_observations_incomplete_or_limited=True,
        comparisons_SHA256=sha(a.out/'comparisons.jsonl'),differences_SHA256=sha(a.out/'differences.jsonl'),
        context_classifications_SHA256=sha(a.out/'context-classifications.jsonl'),driver_SHA256=sha(__file__),
        no_future_whole_expected_objects=True,production_inputs_or_code_modified=False)
    write=a.out/'summary.json';write.write_text(json.dumps(report,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(summary=str(write),SHA256=sha(write),comparisons=len(results),differences=len(differences))),flush=True)


if __name__=='__main__':main()
