"""Negative controls for frozen whole-row comparison and narrow ID references.

Only frozen source expectations are read; no candidate CLI/native output.
These test the auditor, not the production parser.
"""
import argparse,copy,hashlib,importlib.util,json,sys
from pathlib import Path


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def load(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m


def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--plan',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args()
    assert not a.out.exists()
    p=json.loads((a.plan/'plan.json').read_text('utf8'))
    f=a.plan/'frozen';sys.path.insert(0,str(f));driver=load('cli_frozen_driver',f/'cli_source_plan.py')
    remap=load('cli_frozen_reference_schema',f/'cli_pdu_references.py')
    assert driver.case_key(r'preservation\sharon')==driver.case_key('preservation/sharon')=='preservation/sharon'
    assert driver.case_key(r'matrix\actual')==driver.case_key('matrix/actual')=='matrix/actual'
    for bad in('/root','../peer','preservation/../peer','C:\\root'):
        try:driver.case_key(bad)
        except AssertionError:pass
        else:raise AssertionError(('unsafe case path',bad))
    source_rows=json.loads((a.plan/'807-complete-source-rows.json').read_text('utf8'))
    assert len(source_rows)==807 and sha(a.plan/'807-complete-source-rows.json')==p['expected_source_rows_SHA256']
    reports=[]
    for entry in source_rows:
        row=entry['expected_whole_row_placeholder_ID1']
        assert driver.same(row,copy.deepcopy(row))
    base=copy.deepcopy(source_rows[0]['expected_whole_row_placeholder_ID1'])
    def negative(name,change,target=None):
        original=base if target is None else target
        altered=copy.deepcopy(original);change(altered)
        assert not driver.same(original,altered),name
        reports.append(dict(control=name,rejected=True))
    negative('physical-frame-must-not-remap',lambda x:x.__setitem__('number',x['number']+1))
    negative('capture-domain-is-literal',lambda x:x['capture_domain'].__setitem__('Interface',9999))
    negative('owned-byte-source-physical-contributor-is-literal',lambda x:x['pdus'][0]['byte_source']['PacketRefs'][0].__setitem__('Number',9999))
    negative('invented-flow-association',lambda x:x.__setitem__('conversation_id',1))
    negative('wrong-visibility',lambda x:x.__setitem__('visibility','headers'))
    negative('legacy-incomplete-marker-must-disappear-only-in-proved-rows',lambda x:x.__setitem__('info',x['info']+' [incomplete/undecoded]'))
    negative('incorrect-version-label',lambda x:x.__setitem__('protocol','OSPFv2'))
    negative('extra-summary-key-in-normal-PDU',lambda x:x['pdus'][0].__setitem__('summary','invented summary'))
    negative('changed-fieldmap-scalar',lambda x:x['pdus'][0]['fields']['ospf.version'].__setitem__(0,2))
    def alter_bits(x):
        item=next(v for v in x['pdus'][0]['field_values']['values']if v['name']=='ospf.version')
        item['exact_value']=str(item['value'])
    negative('32bit-or-less-dedicated-alias-must-not-gain-exact_value',alter_bits)
    def alter_range(x):
        item=next(v for v in x['pdus'][0]['field_values']['values']if 'pdu_range'in v)
        item['pdu_range']['offset']+=1
    negative('typed-source-range-is-exact',alter_range)
    def alter_origin(x):
        item=next(v for v in x['pdus'][0]['field_values']['values']if v['origin']=='pdu')
        item['origin']='event'
    negative('native-alias-origin-is-pdu',alter_origin)
    negative('scalar-number-does-not-become-string',lambda x:x['pdus'][0]['fields']['ospf.version'].__setitem__(0,'3'))
    ah=next(r['expected_whole_row_placeholder_ID1']for r in source_rows if 'IPSecAH'in r['expected_whole_row_placeholder_ID1']['layers'])
    negative('observed-AH-header-must-remain',lambda x:x['layers'].remove('IPSecAH'),ah)
    negative('display-chain-must-not-duplicate-OSPF',lambda x:x['protocol_stack'].append('OSPFv3'))
    duplicate=None;alias=None
    for entry in source_rows:
        row=entry['expected_whole_row_placeholder_ID1']
        for name,values in row['pdus'][0]['fields'].items():
            if name.startswith('ospf')and len(values)>1:
                duplicate=row;alias=name;break
        if duplicate is not None:break
    assert duplicate is not None
    negative('repeated-alias-occurrence-must-not-be-deduplicated',lambda x:x['pdus'][0]['fields'][alias].pop(),duplicate)
    # A PDU ID value may numerically coincide with a wire/frame/length value.
    # The explicit reference schema must still leave those other values alone.
    frame=base['number'];length=base['pdus'][0]['fields']['pdu.length'][0]
    mapping={1:1000001,frame:2000001,length:3000001}
    assert len(mapping)==3
    changed=remap.remap_row(base,mapping)
    assert changed['number']==frame and changed['layers']==base['layers']
    b=base['pdus'][0];c=changed['pdus'][0]
    assert c['id']==1000001 and c['field_values']['pdu']==1000001
    assert c['fields']['pdu.id']==[1000001]
    assert c['fields']['pdu.length']==[length] and c['fields']['pdu.completed_in']==[frame]
    assert c['fields']['ospf.packet_length']==b['fields']['ospf.packet_length']
    assert c['fields']['flow.id']==[0] and c['byte_source']['PacketRefs']==b['byte_source']['PacketRefs']
    assert c['byte_source']['CompletedIn']==b['byte_source']['CompletedIn']
    assert c['field_values']['source']==b['field_values']['source']
    assert driver.same(base,remap.remap_row(base,{1:1}))
    report=dict(scope=__doc__,plan_SHA256=sha(a.plan/'plan.json'),driver_SHA256=sha(f/'cli_source_plan.py'),
        selftest_SHA256=sha(__file__),complete_source_rows=807,negative_controls=reports,
        negative_controls_rejected=len(reports),narrow_reference_collision_control=True,
        candidate_output_read=False,production_parser_tested=False,Windows_relative_native_case_key_control=True)
    a.out.mkdir();path=a.out/'summary.json';path.write_text(json.dumps(report,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(summary=str(path),SHA256=sha(path),negative_controls_rejected=len(reports),complete_source_rows=807)),flush=True)


if __name__=='__main__':main()
