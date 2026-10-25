"""Retain and classify every changed SDK row; never normalize raw exports.

Usage: l2tp3_export_review.py PREVIOUS_JSONL CURRENT_JSONL OUTPUT_JSON
The source exports remain external. Fragment-close event order is compared as
a multiset only for the explicitly retained diagnostics; raw order/IDs remain
unchanged in the original files.
"""
import base64
import collections
import hashlib
import itertools
import json
from pathlib import Path
import sys

old, new, output = map(Path, sys.argv[1:4])
counts = collections.Counter()
before, after = collections.Counter(), collections.Counter()
unexpected, evidence = [], []
for x, y in itertools.zip_longest(old.open(encoding='utf-8-sig'), new.open(encoding='utf-8-sig')):
    if x == y:
        continue
    if not x or not y:
        unexpected.append('event cardinality changed')
        continue
    a, b = json.loads(x), json.loads(y)
    changed = {k for k in a.keys() | b.keys() if a.get(k) != b.get(k)}
    av, bv = a.get('field_values', {}), b.get('field_values', {})
    avalues, bvalues = av.get('values', []), bv.get('values', [])
    if changed == {'field_values'}:
        delta_a = [v for v in avalues if v not in bvalues]
        delta_b = [v for v in bvalues if v not in avalues]
        if len(delta_a) == len(delta_b) == 1 and delta_a[0]['name'] == delta_b[0]['name'] == 'pdu.length' and delta_a[0]['value'] == 0 and delta_b[0]['value'] == len(base64.b64decode(b['raw'])):
            counts['observed_refused_wire_length_corrected_from_zero'] += 1
            evidence.append({'pdu': b['id'], 'refs': b['source_bytes']['PacketRefs'], 'observed_bytes': delta_b[0]['value']})
            continue
    if changed <= {'field_values', 'domain', 'source_bytes'} and a['protocol'] == b['protocol'] == 'ip' and a.get('error') == b.get('error') and a['status'] == b['status'] == 'incomplete':
        source_delta = {k for k in av.keys() | bv.keys() if av.get(k) != bv.get(k)}
        if source_delta == {'source'}:
            key = lambda e: json.dumps([e['source_bytes'], e['domain'], e.get('error')], sort_keys=True)
            before[key(a)] += 1
            after[key(b)] += 1
            counts['fragment_close_order_changed_raw_rows_retained'] += 1
            continue
    if a['protocol'] == '' and b['protocol'] == 'l2tp' and a['status'] == b['status'] == 'incomplete' and b['profile'] == 'l2tp3-bounded-wire-envelope' and a['raw'] == b['raw'] and a['source_bytes'] == b['source_bytes'] and not b.get('session') and not b.get('fields') and 'carrier capture is truncated' in b.get('error', ''):
        counts['existing_incomplete_UDP_now_has_specific_L2TP_refusal'] += 1
        continue
    unexpected.append({'pdu': b.get('id'), 'changed': sorted(changed)})
result = {'previous_sha256': hashlib.sha256(old.read_bytes()).hexdigest(), 'current_sha256': hashlib.sha256(new.read_bytes()).hexdigest(), 'changed_rows': sum(counts.values()), 'classifications': dict(counts), 'fragment_diagnostic_multisets_equal': before == after, 'unexpected': unexpected, 'observed_length_evidence': evidence}
output.write_text(json.dumps(result, indent=2)+'\n', encoding='utf-8')
print(json.dumps({k: v for k, v in result.items() if k not in {'previous_sha256', 'current_sha256', 'observed_length_evidence'}}))
assert before == after and not unexpected
