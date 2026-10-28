"""Check fresh OS membership packets against the frozen literal field oracle."""
import argparse, base64, copy, hashlib, json, os, subprocess, sys
from pathlib import Path


def sha(p):
    return hashlib.sha256(Path(p).read_bytes()).hexdigest()


def same(a, b):
    if type(a) is not type(b): return False
    if isinstance(a, dict): return a.keys() == b.keys() and all(same(a[k], b[k]) for k in a)
    if isinstance(a, list): return len(a) == len(b) and all(same(x,y) for x,y in zip(a,b))
    return a == b


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ('bins', 'source-proof', 'capture', 'audit-modules', 'out'):
        ap.add_argument('--'+name, type=Path, required=True)
    a = ap.parse_args(); a.out.mkdir(exist_ok=False)
    sys.path.insert(0, str(a.audit_modules))
    from vxlan_endpoint_projection_audit import capture_records
    meta = json.loads((a.source_proof/'summary.json').read_text())
    ledger = a.source_proof/'source-expectations.jsonl'
    assert meta['source_ledger_SHA256'] == sha(ledger) and meta['capture_SHA256'] == sha(a.capture)
    frozen = {x['frame']:x for x in map(json.loads, ledger.open())}
    records = list(capture_records(a.capture))
    assert len(records) == len(frozen) == 6
    build = json.loads((a.bins/'events-receipt.json').read_text())
    binary = a.bins/'events.exe'
    assert build['exit_code'] == 0 and build['binary_SHA256'] == sha(binary)
    output = a.out/'events.jsonl'
    args = [str(binary), 'replay', str(a.capture), '-', '--ignore-embedded-secrets']
    with output.open('xb') as stream, (a.out/'events.stderr').open('xb') as err:
        p = subprocess.run(args, stdout=stream, stderr=err, env=dict(os.environ, GOMAXPROCS='1', GOMEMLIMIT='1GiB'), creationflags=0x08000000)
    assert p.returncode == 0
    events = [json.loads(line) for line in output.open()]

    def validate(actual):
        assert len(actual) == 6
        seen, ids, results = set(), set(), []
        for e in actual:
            refs = e['source_bytes']['PacketRefs']
            assert len(refs) == 1 and type(refs[0]['Number']) is int
            frame = refs[0]['Number']
            assert frame in frozen and frame not in seen; seen.add(frame)
            assert type(e['id']) is int and e['id'] > 0 and e['id'] not in ids; ids.add(e['id'])
            r, expected = records[frame-1], frozen[frame]
            assert same({k:v for k,v in r.items() if k != 'wire'}, expected['record'])
            assert hashlib.sha256(r['wire']).hexdigest() == expected['wire_SHA256']
            assert e['protocol'] == expected['location']['protocol']
            assert e['transport'] == 'network' and e['flow'] == e['transaction'] == e['response_to'] == 0 and 'tcp_flow_id' not in e
            domain = dict(Section=r['section'], Interface=r['interface'], Encapsulation='')
            assert same(e['domain'], domain) and same(refs, [dict(Number=frame, Domain=domain)])
            assert e['source_bytes']['Kind'] == 'captured' and e['source_bytes']['ParentPDU'] == 0 and e['source_bytes']['ParentPDUs'] is None
            assert same(e['source_bytes']['CompletedIn'], refs[0])
            assert base64.b64decode(e['raw'], validate=True) == r['wire']
            assert e['source_bytes']['SHA256'] == hashlib.sha256(r['wire']).hexdigest()
            codec = expected['codec_expected']
            fields = copy.deepcopy(codec['fields'])
            def shift(value):
                if isinstance(value, dict):
                    for key, child in value.items():
                        if key == 'Wire Field Ranges':
                            for span in child.values():
                                span[0] += expected['location']['offset']; span[1] += expected['location']['offset']
                        else: shift(child)
                elif isinstance(value, list):
                    for child in value: shift(child)
            shift(fields)
            assert same(e['fields'], fields) and same(e['session'], fields)
            assert e['status'] == codec['status'] == 'decoded' and e['completeness'] == 'message' and e['expert'] == '' and 'error' not in e
            assert e['summary'] == codec['summary']
            values = [v for v in codec['values'] if v['name']]
            if e['protocol'] == 'icmpv6':
                at = expected['location']['offset']
                values += [dict(name='icmpv6.type', value=r['wire'][at], offset=0, length=1), dict(name='icmpv6.code', value=r['wire'][at+1], offset=1, length=1)]
            wanted = [dict(name=v['name'], value=v['value'], origin='pdu', pdu_range=dict(offset=expected['location']['offset']+v['offset'], length=v['length'])) for v in sorted(values, key=lambda v:v['name'])]
            got = [v for v in e['field_values']['values'] if v['name'].startswith(('igmp.', 'icmpv6.'))]
            assert same(wanted, got)
            assert e['field_values']['pdu'] == e['id'] and same(e['field_values']['source'], e['source_bytes'])
            results.append(dict(frame=frame, protocol=e['protocol'], status=e['status'], typed_values=len(got), source_SHA256=expected['wire_SHA256']))
        assert seen == frozen.keys()
        return results

    checks = validate(events)
    negative = []
    for name, mutate in [
        ('domain', lambda e:e['domain'].update(Interface=e['domain']['Interface']+1)),
        ('record-count', lambda e:e['session']['IGMP'].update(**{'Group Record Count': 7})),
        ('raw-source', lambda e:e.update(raw='AA==')),
        ('invented-flow', lambda e:e.update(flow=1)),
        ('typed-range', lambda e:next(v for v in e['field_values']['values'] if v['name']=='igmp.maddr')['pdu_range'].update(offset=0)),
    ]:
        forged = copy.deepcopy(events); mutate(forged[0])
        try: validate(forged)
        except AssertionError: negative.append(dict(name=name, rejected=True))
        else: raise AssertionError('negative accepted: '+name)
    result = dict(scope=__doc__, source=build['source'], binary_SHA256=sha(binary), build_receipt_SHA256=sha(a.bins/'events-receipt.json'),
                  source_proof_SHA256=sha(a.source_proof/'summary.json'), capture_SHA256=sha(a.capture),
                  arguments=args, exit_code=p.returncode, export_SHA256=sha(output), checks=checks, typed_values=sum(v['typed_values'] for v in checks), negative_controls=negative,
                  auditor_SHA256=sha(__file__), limits='Entire source/semantic fields/session and every membership alias/range verified. Other generic event metadata is checked only as explicitly listed; this is not a new blanket whole-export certificate.')
    (a.out/'summary.json').write_text(json.dumps(result, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(dict(source=result['source'], records=len(checks), typed_values=result['typed_values'], negative_controls=len(negative), summary_SHA256=sha(a.out/'summary.json'))), flush=True)


if __name__ == '__main__': main()
