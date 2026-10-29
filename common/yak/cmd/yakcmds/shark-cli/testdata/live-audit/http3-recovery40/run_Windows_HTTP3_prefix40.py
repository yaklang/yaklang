"""Actual default-budget Windows console with independent wire-prefix/highlight checks."""
from pathlib import Path
import argparse, hashlib, importlib.util, json, os, re, subprocess, sys, time, traceback

phase=Path(__file__).parent
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
read=lambda p:json.loads(Path(p).read_text('utf-8-sig'))
def write(p,v):Path(p).write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
ap=argparse.ArgumentParser(description=__doc__)
for n in ('out','bins'):ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--expected-source',required=True)
a=ap.parse_args();a.out.mkdir()
try:
    casefile=phase/'Windows-HTTP3-prefix-source-plan40-01/case.json';c=read(casefile)
    build=a.bins/'yak-receipt.json';b=read(build);binary=a.bins/'yak.exe'
    assert b['source']==a.expected_source and b['exit_code']==0 and sha(binary)==b['binary_SHA256']
    inputs={n:Path(c[n])for n in ('capture','keys','helper','renderer','physical_file','PDU_file')}
    inputs.update(case=casefile,binary=binary,build=build,controller=Path(__file__))
    before={k:dict(path=str(p),SHA256=sha(p),bytes=p.stat().st_size)for k,p in inputs.items()}
    for n,key in [('capture','capture_SHA256'),('keys','keylog_SHA256'),('helper','helper_SHA256'),('renderer','renderer_SHA256'),('physical_file','physical_SHA256'),('PDU_file','PDU_SHA256')]:assert before[n]['SHA256']==c[key]
    write(a.out/'inputs-before.json',before)
    folder=a.out/'HTTP3-prefix-missing';args=[c['helper'],str(folder),str(binary),c['capture'],c['keys'],c['query'],'DCID','--ignore-embedded-secrets']
    env={k:v for k,v in os.environ.items()if not k.startswith('SHARK_QA_')}
    env.update(GOMAXPROCS='1',GOMEMLIMIT='1GiB',SHARK_QA_EXPECT_PROTOCOL='HTTP/3',SHARK_QA_EXPECT_QUERY=c['query'],
               SHARK_QA_EXPECT_PACKETS='2018',SHARK_QA_EXPECT_ADAPTIVE='1',SHARK_QA_FIELD_DOWN='0',SHARK_QA_SELECT_FIELD='DCID')
    start=time.monotonic();r=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,timeout=180,creationflags=0x08000000)
    (a.out/'stdout.log').write_bytes(r.stdout);(a.out/'stderr.log').write_bytes(r.stderr)
    after={k:dict(path=str(p),SHA256=sha(p),bytes=p.stat().st_size)for k,p in inputs.items()};write(a.out/'inputs-after.json',after)
    assert before==after
    proof=dict(source=a.expected_source,exit_code=r.returncode,seconds=round(time.monotonic()-start,3),arguments=args,
        inputs_unchanged=True,default_history_mib=64,default_packet_cache=20000,budget_increase=False,
        source_case_SHA256=sha(casefile),visual_review_pending=True,snapshots={})
    shots={}
    for f in folder.glob('*.json'):
        s=read(f)
        if 'Rows'not in s:continue
        assert len(s['Rows'])==len(s['Attributes'])==s['Height'] and all(len(row)==len(at)==s['Width']for row,at in zip(s['Rows'],s['Attributes']))
        shots[f.stem]=s;image=f.with_suffix('.png')
        render=subprocess.run([sys.executable,'-B',c['renderer'],str(f),str(image)],capture_output=True,creationflags=0x08000000);assert render.returncode==0
        proof['snapshots'][f.stem]=dict(cells_SHA256=sha(f),PNG_SHA256=sha(image))
    write(a.out/'proof.json',proof);assert r.returncode==0,'actual helper failed; outputs retained'
    spec=importlib.util.spec_from_file_location('prefix_console_wire_reader',phase/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
    first=shots['01-protocol-fields'];text='\n'.join(first['Rows'])
    assert c['query']in first['Rows'][1] and ' EOF 'in first['Rows'][0]
    assert re.search(r'buffer\s+2018\s*/\s*20000',text) and re.search(r'· 1 visible(?:\s|╮)',text)
    assert any(re.search(r'›1011\s+\S+\s+HTTP/3(?:\s|$)',row)for row in first['Rows'])
    assert ' › '.join(c['progressive_stack'])in text and c['required_summary']in text
    raw=Path(c['physical_file']).read_bytes();wire=Path(c['PDU_file']).read_bytes()
    observed,_=m.pane(first);assert len(observed)>40 and raw.startswith(observed)
    selected=shots['01-selected-field'];observed,pos=m.pane(selected);assert len(observed)>9 and wire.startswith(observed)
    attrs=[selected['Attributes'][y][x]for y,x in pos];lo,hi=c['field_range'];style=attrs[lo]
    assert all(v==style for v in attrs[lo:hi]) and all(v!=style for i,v in enumerate(attrs)if not lo<=i<hi)
    assert observed[lo:hi].hex()==c['field_wire_hex']
    assert read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
    proof.update(passed=True,source_verification=dict(frame=1011,PDU=1021,protocol='HTTP/3',
        prefix_missing_visible=True,progressive_stack=c['progressive_stack'],DCID_PDU_range=[1,9],DCID_physical_range=c['field_physical_range'],
        exact_visible_physical_prefix=True,exact_visible_PDU_prefix=True,entire_sources_visible_claim=False,
        selected_8_bytes_exact=True,mouse_tabs_resize_and_CP936_restored=True))
    write(a.out/'proof.json',proof);print(json.dumps(dict(proof=str(a.out/'proof.json'),SHA256=sha(a.out/'proof.json'),passed=True)))
except Exception:
    write(a.out/'failure.json',dict(error=traceback.format_exc(),no_success_certificate=True));raise
