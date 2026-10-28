"""Build the complete expected inspector response from literal admitted IP bytes.

The previous response is frozen before the new CLI address projection. Only a
Captured network prefix section may be inserted, with source-zero IP/header
ranges; native event metadata, every other field and all logical sources remain
literal. No offset is assigned to generated native event fields.
"""
import copy
import vxlan_endpoint_projection_audit as ep


def expected_response(record,request,before):
    expected=copy.deepcopy(before);event=None
    mapping={'protocol':'Protocol','status':'Status','profile':'Profile','completeness':'Completeness','expert':'ExpertCode','session':'Session'}
    for raw in request.get('Events')or[]:
        candidate={key:raw.get(value)for key,value in mapping.items()}
        if candidate['session']is None:candidate['session']={}
        selected=ep.contract(candidate)
        if selected is not None:event,selection=candidate,selected;break
    if event is None:return expected,[]
    headers=ep.prefix_headers(record,selection['headers'])
    if not headers:return expected,[]
    def field(text,branch=False,depth=0,span=None):
        return dict(Text=text,Branch=branch,Depth=depth,Source=0,Start=span[0]if span else 0,End=span[1]if span else 0,HasRange=span is not None)
    added=[field('Captured network prefix',branch=True)]
    for h in headers:
        added.extend([field('IPv'+str(h['version'])+' header',branch=True,span=[h['start'],h['end']]),field('Source: '+h['source'],depth=1,span=h['source_range']),field('Destination: '+h['destination'],depth=1,span=h['destination_range'])])
    assert before['Fields']and before['Fields'][0]['Text'].startswith('Frame 1:')
    expected['Fields']=before['Fields'][:1]+added+before['Fields'][1:]
    return expected,headers


def inspect(record,request,before,after):
    expected,headers=expected_response(record,request,before)
    assert ep.canonical(expected)==ep.canonical(after),'entire inspector response must equal literal admitted-prefix insertion; no other field/source/type mutation'
    return dict(literal_headers=headers,complete_response_equal_literal_expected=True,all_other_fields_and_sources_literal=True)


def negatives(cases):
    checks=[]
    def run(name,case,mutate):
        record,request,before,after=copy.deepcopy(cases[case]);inspect(record,request,before,after);mutate(after)
        try:inspect(record,request,before,after)
        except(AssertionError,KeyError):checks.append(dict(case=name,rejected=True));return
        raise AssertionError('negative accepted: '+name)
    def source(response):return next(f for f in response['Fields']if f['Text'].startswith('Source:')and f['HasRange'])
    run('confirmed IP source range moved into adjacent bytes',('I-missing',False),lambda r:source(r).__setitem__('Start',source(r)['Start']+1))
    run('confirmed IP address belongs to a virtual logical source',('I-missing',False),lambda r:source(r).__setitem__('Source',1))
    run('wrong deepest unadmitted IP displayed as captured physical endpoint',('nested-I-missing',False),lambda r:source(r).__setitem__('Text','Source: 192.0.2.1'))
    run('valid intermediate tunnel IP displayed beyond native outer-prefix scope',('nested-I-missing',False),lambda r:source(r).__setitem__('Text','Source: 198.51.100.1'))
    run('new prefix field silently loses its physical byte range',('I-missing',False),lambda r:source(r).__setitem__('HasRange',False))
    run('unrelated existing native metadata text changes',('I-missing',False),lambda r:r['Fields'][-1].__setitem__('Text','unrelated metadata changed'))
    run('old source-zero frame entry changes its integer range type',('I-missing',False),lambda r:r['Fields'][0].__setitem__('Start',0.0))
    return dict(negative_controls=len(checks),all_rejected=True,checks=checks)
