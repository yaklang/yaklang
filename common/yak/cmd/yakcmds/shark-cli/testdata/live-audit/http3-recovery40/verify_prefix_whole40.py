"""Source-bound whole-object correction; no ID mapping or ignored fields."""
from pathlib import Path
import argparse, copy, hashlib, json

phase = Path(__file__).parent
sha = lambda p: hashlib.sha256(Path(p).read_bytes()).hexdigest()
read = lambda p: json.loads(Path(p).read_text('utf-8-sig'))
rows = lambda p: [json.loads(s) for s in Path(p).read_text('utf-8-sig').splitlines()]
ap = argparse.ArgumentParser(description=__doc__)
ap.add_argument('--actual', type=Path, required=True)
ap.add_argument('--expected-source', required=True)
ap.add_argument('--out', type=Path, required=True)
a = ap.parse_args(); a.out.mkdir()
planpath = phase/'HTTP3-prefix-identity-source-plan40-01/whole-object-contract03.json'
assert sha(planpath) == '2bc54ef1300d071b7afabc96a83fcc4c2ca89ea07ee09a1c42e9498132df3b36'
plan = read(planpath); slots = {x['frame']:x for x in plan['slots']}
baseline = phase/'fresh-TLS-QUIC-d677-actual39-01'
bound = {str(planpath):sha(planpath), str(Path(__file__)):sha(__file__)}
differences = []; checks = []
assert read(a.actual/'summary.json')['source'] == a.expected_source
for kind in ('TLS','QUIC'):
    left = baseline/kind/'events.jsonl'; right = a.actual/kind/'events.jsonl'
    for p in (left,right):bound[str(p)] = sha(p)
    old, new = rows(left), rows(right); assert len(old) == len(new)
    changes = 0
    for index,(before,after) in enumerate(zip(old,new)):
        expected = copy.deepcopy(before)
        refs = [r['Number'] for r in before['source_bytes']['PacketRefs']]
        source = slots.get(refs[0]) if len(refs)==1 and kind=='QUIC' else None
        if source:
            assert before['source_bytes']['SHA256'] == source['packet_SHA256']
            assert before['protocol']=='quic' and before['session']['ALPN']=='h3'
            assert before['session']['Authentication Verified'] is True
            expected.update(plan['PDU_labels']); expected['session'].update(plan['new_session_values'])
            streams=[f for f in expected['session']['Frames'] if f['Frame Type']=='STREAM']
            assert len(streams)==1 and streams[0]['Stream ID']==12 and streams[0]['Delivered Bytes']==0
            assert not streams[0].get('Request PDU ID') and not before['transaction'] and not before['response_to']
            streams[0].update(plan['new_STREAM_values'])
            for v in expected['field_values']['values']:
                if v['name']=='protocol':assert v['value']=='quic';v['value']='http3'
            changes += 1
        if expected != after:
            differences.append(dict(capture=kind,index=index,PDU=before['id'],physical=refs))
    assert changes == (2 if kind=='QUIC' else 0)
    checks.append(dict(capture=kind,whole_events=len(old),source_constructed_corrections=changes))
assert all(sha(p)==h for p,h in bound.items())
target = a.out/'summary.json'
with target.open('x',encoding='utf8') as f:
    json.dump(dict(scope=__doc__,source=a.expected_source,checks=checks,unplanned_differences=differences,
        ID_mapping=False,ignored_fields=False,inputs=bound,inputs_unchanged=True),f,indent=2);f.write('\n')
print(json.dumps(dict(summary=str(target),SHA256=sha(target),unplanned_differences=len(differences))))
assert not differences
