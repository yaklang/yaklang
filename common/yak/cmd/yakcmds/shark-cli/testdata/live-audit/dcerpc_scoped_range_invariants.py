"""Certify only independently wire-validated nested RPC range additions.

Every complete SDK record, its identity, ordering, field/type, connection,
diagnostic and source remains exact after removing only the newly audited
context/result-local Wire Field Ranges metadata. There is no ID normalization.
"""
import argparse
import copy
import itertools
import json
from pathlib import Path

from mail_rtsp_export_invariants import canonical,rows,sha


def audit(before,after,wire_certificate):
    certificate=json.loads(wire_certificate.read_text(encoding='utf8'))
    assert certificate['require_scoped_field_ranges']is True,'mandatory full original-wire range proof'
    assert certificate['events_sha256']==sha(after),'certificate binds the exact complete current SDK export'
    proven={e['PDU']for e in certificate['compared_PDUs']}
    count=0;changes=[];metadata_maps=0
    for old,new in itertools.zip_longest(rows(before),rows(after)):
        assert old is not None and new is not None,'every original SDK record remains ordered'
        count+=1
        if canonical(old)==canonical(new):continue
        assert old['protocol']==new['protocol']=='dcerpc'and old['id']==new['id']in proven,'only an independently bound original RPC record may gain ranges'
        expected=copy.deepcopy(new);added=0
        for name in['Presentation Contexts','Presentation Context Results']:
            current=(expected.get('session')or{}).get(name)or[]
            original=(old.get('session')or{}).get(name)or[]
            assert len(current)==len(original),'all original semantic ordinals remain present'
            for a,b in zip(original,current):
                assert 'Wire Field Ranges'not in a,'baseline predates context-local range metadata'
                if 'Wire Field Ranges'in b:
                    b.pop('Wire Field Ranges');added+=1
        assert added and canonical(old)==canonical(expected),'only the exact audited range metadata changes'
        metadata_maps+=added;changes.append(dict(PDU=new['id'],added_maps=added))
    assert count==certificate['SDK_records_with_unique_positive_IDs'],'complete input coverage'
    return dict(scope=__doc__,before_sha256=sha(before),after_sha256=sha(after),wire_certificate_sha256=sha(wire_certificate),complete_records_compared=count,records_with_scoped_range_additions=len(changes),scoped_range_maps_added=metadata_maps,changed=changes,all_original_values_types_IDs_order_flows_and_sources_exact=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['before','after','wire-certificate','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    proof=audit(a.before,a.after,a.wire_certificate)
    a.out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
    print(json.dumps({k:proof[k]for k in['complete_records_compared','records_with_scoped_range_additions','scoped_range_maps_added','all_original_values_types_IDs_order_flows_and_sources_exact']}))
