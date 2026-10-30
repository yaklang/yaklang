"""Inventory old50 SSH event/CLI schema only; never read candidate53."""
from pathlib import Path
import base64,collections,hashlib,importlib.util,json,re
P=Path(__file__).resolve().parent.parent;D=Path(__file__).resolve().parent
N=P/'runtime-public-native-50';C=P/'runtime-public-CLI-50'
REPO=Path(r'C:/Users/V/.codex/worktrees/shark-protocol-infrastructure/yaklang')
f=REPO/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/public_native_literal_continuity.py'
spec=importlib.util.spec_from_file_location('old_literal_receipts53',f);util=importlib.util.module_from_spec(spec);spec.loader.exec_module(util)
assert util.sha(f)=='cea420bb3cc200cbddd8c023cb9ea25f4a756a2f857f606cbf77f440edfa955d'
ns=util.read(N/'summary.json');cs=util.read(C/'summary.json');assert ns['source']==cs['source']=='26f01f4baa5fd39b88deadf901257a70bc5c1dfe'
files=[];categories=collections.Counter();cli_shapes=collections.Counter();by_file={};unknown=[]
for path in sorted(N.glob('*/*/events-producer.json')):
    r=util.read(path);assert r in ns['outputs']and r['exit_code']==0
    file=path.with_name('events.jsonl');counts=collections.Counter();h=hashlib.sha256();rows=0
    for i,line in enumerate(file.open('rb'),1):
        h.update(line);rows+=1
        if not re.search(br'"protocol"\s*:\s*"ssh"',line):continue
        e=json.loads(line)
        if e['protocol']!='ssh':continue
        a=e.get('fields');s=e.get('session')or{};raw=base64.b64decode(e['raw'])if e.get('raw')else b''
        if not raw:kind='emptyRaw diagnostic'
        elif isinstance(a,dict)and'Identification'in a:kind='identification'
        elif isinstance(a,dict)and'Text'in a:kind='pre-identification'
        elif isinstance(a,dict)and'Payload'in a and'Packet Length'in a:kind='complete packet fields'
        elif a is None:kind='refused/opaque/incomplete no fields'
        else:kind='uncovered field shape';unknown.append({'file':str(file.relative_to(N)),'line':i,'keys':list(a)if isinstance(a,dict)else str(type(a))})
        categories[kind]+=1;counts[kind]+=1
    assert h.hexdigest()==r['export_SHA256'],'old closed export'
    rel=str(path.parent.relative_to(N));by_file[rel]=dict(counts)
    files.append({'case':rel,'rows':rows,'SSH':dict(counts),'export_SHA256':h.hexdigest(),'receipt_SHA256':util.sha(path)})
for path in sorted(C.glob('*/*/yak-producer.json')):
    r=util.read(path);assert r in cs['outputs']and r['exit_code']==0
    f=path.with_name('yak.jsonl');h=hashlib.sha256()
    for line in f.open('rb'):
        h.update(line)
        if not re.search(br'"protocol"\s*:\s*"ssh"',line):continue
        c=json.loads(line)
        for key in('pdus','published_pdus'):
            for e in(c.get(key)or[]):
                if e.get('protocol')=='ssh':cli_shapes[key+':'+','.join(sorted(e))]+=1
    assert h.hexdigest()==r['export_SHA256']
result={'source':'26f01f4baa5fd39b88deadf901257a70bc5c1dfe','scope':'old50 schema census only; not candidate evidence or independent PCAP truth','native_categories':dict(categories),'CLI_ssh_compact_shapes':dict(cli_shapes),'native_cases':files,'uncovered':unknown,'no_candidate53_read':True,'script_SHA256':util.sha(__file__)}
(D/'old50-inventory.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'inventory_SHA256':util.sha(D/'old50-inventory.json'),'native_categories':dict(categories),'CLI_ssh_compact_shapes':dict(cli_shapes),'unknown_shapes':len(unknown)}))