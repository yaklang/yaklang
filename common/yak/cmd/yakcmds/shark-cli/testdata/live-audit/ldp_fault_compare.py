"""Run LDP fault controls against TShark and the shared native TCP receiver.

Usage: ldp_fault_compare.py EXTERNAL_FAULT_DIRECTORY TSHARK EVENTS
The generator's manifest declares all byte/sequence changes; this verifier
never changes expected bytes to match a decoder. Original differences remain.
"""
import base64,collections,hashlib,json,subprocess,sys
from pathlib import Path

root,tshark,events=map(Path,sys.argv[1:4])
if any((x/'.git').exists() for x in [root.resolve(),*root.resolve().parents]):raise SystemExit('Raw output must stay outside Git')
manifest=json.loads((root/'manifest.json').read_text())
audit=Path(__file__).with_name('ldp_session_audit.py')
def run(args,path,allow=False):
    with path.open('wb') as stdout,path.with_suffix(path.suffix+'.stderr').open('wb') as stderr:
        p=subprocess.run(list(map(str,args)),stdout=stdout,stderr=stderr,timeout=180)
    if not allow and p.returncode:raise RuntimeError((path.name,p.returncode))
    return p.returncode
def digest(e):return hashlib.sha256(base64.b64decode(e.get('raw') or '')).hexdigest()
aggregate={};baseline=[];basecounts=collections.Counter()
for case in manifest['cases']:
    name=case['name'];cap=root/case['file'];out=root/name;out.mkdir(exist_ok=True)
    assert hashlib.sha256(cap.read_bytes()).hexdigest()==case['sha256']
    run([tshark,'-2','-n','-r',cap,'-o','tcp.reassemble_out_of_order:TRUE','-Y','ldp','-T','json','-x','--no-duplicate-keys'],out/'oracle.json')
    run([events,'replay',cap,'-','--ignore-embedded-secrets'],out/'events.jsonl')
    code=run([sys.executable,'-X','utf8',audit,out/'oracle.json',out/'events.jsonl',out/'comparison.json'],out/'audit.log',True)
    es=[json.loads(s) for s in (out/'events.jsonl').read_text(encoding='utf-8-sig').splitlines()]
    es=[e for e in es if e['protocol']=='ldp' and e['transport']=='tcp']
    decoded=[e for e in es if e['status']=='decoded']
    observed=collections.Counter(digest(e) for e in decoded)
    if name=='baseline':baseline=decoded;basecounts=observed
    twice=collections.Counter({k:v*2 for k,v in basecounts.items()})
    allowed=twice if name=='same-tuple-new-generation' else basecounts
    assert not observed-allowed,(name,'invented or duplicated logical PDU')
    if name in ('baseline','split-reordered','split-duplicate','snaplen-retransmitted'):
        assert observed==basecounts,(name,'lost complete PDU despite complete source/retransmission')
        assert code==0,(name,'Wireshark complete PDU/field differences remain')
    if name in ('split-loss','snaplen'):
        assert code==0,(name,'remaining complete PDU/field differences')
        target_source=max(baseline,key=lambda e:len(base64.b64decode(e['raw'])))['source']
        reverse_expected=collections.Counter(digest(e) for e in baseline if e['source']!=target_source)
        reverse_observed=collections.Counter(digest(e) for e in decoded if e['source']!=target_source)
        assert reverse_observed==reverse_expected,(name,'intact reverse direction was lost')
    if name=='same-tuple-new-generation':
        assert observed==twice,'generation replay did not preserve both original PDU occurrences'
        assert len({e['flow'] for e in decoded})==2,'same tuple must have two observed TCP generations'
        assert code==0,'generation oracle mismatch'
        for flow in {e['flow'] for e in decoded}:
            first=next(e for e in decoded if e['flow']==flow)
            assert 'Observed Maximum PDU Length' not in first['session'],'new generation inherited old negotiation'
    comparison=json.loads((out/'comparison.json').read_text())
    aggregate[name]={'frames':case['frames'],'native_complete_pdus':len(decoded),'native_flows':len({e['flow'] for e in decoded}),
        'native_status':dict(collections.Counter(e['status'] for e in es)),'same_logical_pdus_as_baseline':observed==basecounts,
        'oracle_views':comparison['oracle_views'],'wire_matches':comparison['matched_wire_pdus'],'checks':comparison['checks_total'],
        'oracle_prefix_views':len(comparison['oracle_prefix_views']),'unmatched_oracle_complete_pdus':comparison['missing'],
        'field_or_identity_differences':comparison['field_or_identity_differences'],'unmatched_native_pdus':comparison['unmatched_native_pdus'],
        'comparison_return_code':code,'capture_sha256':case['sha256']}
    print(json.dumps({'case':name,'native':len(decoded),'wire_matches':comparison['matched_wire_pdus'],'missing':len(comparison['missing']),'field_differences':len(comparison['field_or_identity_differences'])}),flush=True)
(root/'fault-summary.json').write_text(json.dumps({'kind':manifest['kind'],'source_sha256':manifest['source_sha256'],'cases':aggregate},indent=2)+'\n',encoding='utf8')
