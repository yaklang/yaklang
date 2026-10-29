"""Adapt only closed CLI auditor provenance; preserve physical/source assertions."""
from pathlib import Path
import hashlib

phase = Path(__file__).parent
old = phase/'audit_fresh_CLI39_03.py'
assert hashlib.sha256(old.read_bytes()).hexdigest() == '72488f725ee0751cb09186b62a6d18630b0649cd757fd815a249729bfef55d53'
text = old.read_text('utf8')
edits = [
    ('import base64,collections,datetime,hashlib,importlib.util,json,re,sys', 'import argparse,base64,collections,datetime,hashlib,importlib.util,json,re,sys'),
    ("actual=root/'fresh-TLS-QUIC-d677-actual39-01'", "ap=argparse.ArgumentParser(description=__doc__)\nfor k in ('actual','sdk-proof','bins','out'):ap.add_argument('--'+k,type=Path,required=True)\nap.add_argument('--expected-source',required=True)\na=ap.parse_args()\nactual=a.actual"),
    ("out=root/sys.argv[1];out.mkdir()", "out=a.out;out.mkdir()"),
    ("sdkproof=source/'SDK-d677-source39-audit01/summary.json'\nassert sha(sdkproof)=='f75f6b9b62a936e1aa3033eb1eca8c5a4cd25ba4b7b3177a2f64d8b851ae158a'", "sdkproof=a.sdk_proof\nassert read(sdkproof)['source']==a.expected_source and read(sdkproof)['failed_checks']==0"),
    ("proof['source']=='d6772965c669e288705f7e0d0e25c62972226832'", "proof['source']==a.expected_source"),
    ("build=root/'bins-38'/(kind+'-receipt.json')", "build=a.bins/(kind+'-receipt.json')"),
    ("sha(root/'bins-38'/(kind+'.exe'))", "sha(a.bins/(kind+'.exe'))"),
    ("source='d6772965c669e288705f7e0d0e25c62972226832'", "source=a.expected_source"),
]
for before, after in edits:
    assert text.count(before) == 1, before
    text = text.replace(before, after)
target = phase/'audit_fresh_CLI40.py'
with target.open('x',encoding='utf8') as f:f.write(text)
print(hashlib.sha256(target.read_bytes()).hexdigest())
