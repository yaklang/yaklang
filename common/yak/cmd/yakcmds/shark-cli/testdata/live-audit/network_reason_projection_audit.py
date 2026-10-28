"""Certify a CLI-only removal of redundant native network error markers.

Every JSON record, physical fact, protocol identity, stack, source annotation,
publication and connection field stays literal. Only one generic suffix may be
removed when the preceding export already carries a precise native refusal.
"""
import collections,hashlib,itertools,json
from pathlib import Path


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'))
def audit(before,after):
    changes=[];counts=collections.Counter();marker=' [incomplete/undecoded]'
    with before.open(encoding='utf8')as a,after.open(encoding='utf8')as b:
        for previous,current in itertools.zip_longest(a,b):
            assert previous is not None and current is not None,'no added or missing CLI record'
            left,right=json.loads(previous),json.loads(current);counts[left.get('record_type','physical')]+=1
            if canonical(left)==canonical(right):continue
            assert left.get('record_type')!='protocol_publication'
            assert left['visibility']in['truncated-network-header','unselected-network-profile','invalid-network-header']
            assert left['protocol']in['IPv4','IPv6','IP']and left['info'].count(marker)==1
            expected=dict(left);expected['info']=left['info'].replace(marker,'',1)
            assert canonical(expected)==canonical(right),'only an explained native network reason may lose its redundant generic marker'
            changes.append(dict(frame=right['number'],visibility=right['visibility']))
    assert counts
    return dict(scope=__doc__,before_CLI_SHA256=sha(before),CLI_SHA256=sha(after),auditor_SHA256=sha(Path(__file__)),record_counts=dict(counts),precise_reason_changes=len(changes),changes=changes,every_other_value_type_and_connection_literal=True)
