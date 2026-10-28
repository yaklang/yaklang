"""Source705 discovery-to-fresh-whole expectation freeze, no candidate copying."""
import argparse,collections,hashlib,json,sys
from pathlib import Path
from expected_direct import whole_row,whole_worker


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()


def main():
    ap=argparse.ArgumentParser()
    for n in['discovery','capture','source-parser','contract145','audit-modules','out']:ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path[:0]=[str(a.source_parser),str(a.contract145),str(a.audit_modules)]
    from classify import parent,failure,projection
    from expected import own_failure
    from pptp_parent_source_audit import stamp
    from vxlan_endpoint_projection_audit import capture_records
    summary=json.loads((a.discovery/'summary.json').read_text());metadata=next(x for x in summary['corpora']if x['corpus']=='sharon');ledger=a.discovery/'sharon/source-classification.jsonl';assert sha(ledger)==metadata['ledger_SHA256']and sha(a.capture)==metadata['capture_SHA256']
    # Only the discovery reference/clock/hash is retained; its candidate row
    # and even its source-derived values are not used by this constructor.
    refs={x['frame']:dict(wire_SHA256=x['wire_SHA256'],record=x['record'])for x in map(json.loads,ledger.open())}
    result=[];counts=collections.Counter();short=collections.Counter()
    for n,r in enumerate(capture_records(a.capture),1):
        if n not in refs:continue
        r['frame']=n;assert hashlib.sha256(r['wire']).hexdigest()==refs[n]['wire_SHA256']and{k:v for k,v in r.items()if k!='wire'}==refs[n]['record']
        loc=parent(r);bad=failure(r['wire'],loc,own_failure);row=whole_row(r,loc,bad,projection,stamp);worker=whole_worker(r,loc,bad);quick=whole_worker(r,loc,bad,True);assert worker['Fields'][1:]==quick['Fields'][1:]
        counts[bad['category']]+=1
        if loc['own_end']-loc['own_offset']<20:short[loc['own_end']-loc['own_offset']]+=1
        result.append(dict(frame=n,wire_SHA256=refs[n]['wire_SHA256'],record=refs[n]['record'],literal_location=loc,own_failure=bad,whole_expected_row=row,whole_expected_worker=worker,whole_expected_quick=quick))
    assert len(result)==len(refs)==metadata['sources'];target=a.out/'whole-expected.jsonl';target.write_text(''.join(json.dumps(x,sort_keys=True)+'\n'for x in result),encoding='utf8')
    out=dict(scope='Fresh entire CLI/worker/quick objects after discovery of affected frame numbers, derived by rereading original records only. This post-discovery source oracle is not misrepresented as pre-candidate. Old145 pre-candidate oracle remains separate and unchanged.',sources=len(result),categories=dict(counts),fixed_short_length_threshold_profiles=dict(sorted(short.items())),source_capture_SHA256=sha(a.capture),discovery_SHA256=sha(a.discovery/'summary.json'),discovery_ledger_SHA256=sha(ledger),whole_expected_SHA256=sha(target),constructor_SHA256=sha(Path(__file__).with_name('expected_direct.py')),source_parser_SHA256=sha(a.source_parser/'classify.py'),contract_full145_SHA256=sha(a.contract145/'expected.py'),driver_SHA256=sha(Path(__file__)),fields='Fixed words require2/4/8/12/14/16/18/20 captured bytes; DataOffset requires13, complete payload boundary requires valid20..60B header within own envelope. No default words, Native Raw offset search, PDU, Source1 or connection IDs.')
    p=a.out/'summary.json';p.write_text(json.dumps(out,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(p),SHA256=sha(p),sources=len(result))),flush=True)


if __name__=='__main__':main()
