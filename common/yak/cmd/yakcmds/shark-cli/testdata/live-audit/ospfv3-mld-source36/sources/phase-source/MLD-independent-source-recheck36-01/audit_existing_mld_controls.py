"""Independent source/PDML checks for existing advanced MLD control captures.

The frozen UI cases file selects existing capture paths and hashes only. No
expected byte span, protocol field or candidate output is copied from it.
Authored examples and owned live traffic are explicitly separate from the237
historical public observations. This does not run the production decoder.
"""
import argparse,collections,hashlib,importlib.util,json,os,subprocess,sys,time,xml.etree.ElementTree as ET
from pathlib import Path
from mld_source_layout import inspect
from verify_mld_source_fields import ws_name,ws_value,same


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def load(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def write(p,value):
    with Path(p).open('x',encoding='utf8')as f:json.dump(value,f,indent=2);f.write('\n')


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('case-source','capture-helper','carrier-helper','tshark','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir()
    sys.path.insert(0,str(a.capture_helper.parent));reader=load('existing_mld_capture_reader',a.capture_helper);carrier=load('existing_mld_carrier',a.carrier_helper)
    cases=json.loads(a.case_source.read_text('utf8'));selected={k:v for k,v in cases.items()if'MLD'in k}
    assert len(selected)==5;proofs=[];comparisons=[];differences=[]
    for case,meta in selected.items():
        cap=Path(meta['capture']);assert sha(cap)==meta['capture_SHA256']
        d=a.out/case;d.mkdir();frames=d/'original-frames';frames.mkdir();source=[]
        for frame,record in enumerate(reader.capture_records(cap),1):
            try:loc=carrier.locate(record)
            except AssertionError:continue
            if loc['protocol']!='icmpv6':continue
            wire=record['wire'];body=wire[loc['offset']:loc['end']]
            if len(body)<4 or body[0]not in(130,131,132,143):continue
            parent=[h for h in loc['headers']if h['kind']in('IPv4','IPv6')][-1]
            if parent['kind']=='IPv6':declared=parent['offset']+40+parent['declared_payload']-loc['offset']
            else:declared=parent['offset']+parent['declared_length']-loc['offset']if parent['declared_length']>=loc['offset']-parent['offset']else None
            file=frames/f'frame-{frame}.bin';file.write_bytes(wire)
            source.append(dict(frame=frame,record={k:v for k,v in record.items()if k!='wire'},source_profile=loc,
                               original_frame_file=str(file),original_frame_SHA256=sha(file),
                               source=inspect(body,loc['offset'],loc['raw_start'],declared)))
        assert source,'existing input contains no source-selected MLD body'
        source_file=d/'source-facts.json';write(source_file,source)
        args=[str(a.tshark),'-n','-2','-r',str(cap),'-Y','frame.number in {'+', '.join(str(x['frame'])for x in source)+'}','-T','pdml']
        ws=d/'wireshark.pdml';stderr=d/'wireshark.stderr.txt';cfg=d/'fresh-wireshark-config';cfg.mkdir()
        env=os.environ.copy();env['WIRESHARK_CONFIG_DIR']=str(cfg);env.pop('SSLKEYLOGFILE',None);start=time.monotonic()
        with ws.open('xb')as out,stderr.open('xb')as err:proc=subprocess.run(args,stdout=out,stderr=err,env=env,creationflags=0x08000000)
        assert proc.returncode==0 and sha(cap)==meta['capture_SHA256']
        packets={int(p.find(".//field[@name='frame.number']").get('show')):p for p in ET.parse(ws).getroot()}
        assert set(packets)=={x['frame']for x in source}
        for r in source:
            proto=[p for p in packets[r['frame']].iter('proto')if p.get('name')=='icmpv6'and int(p.get('pos'))==r['source_profile']['offset']]
            for field in r['source']['fields']:
                alias=field.get('public_alias')
                if not alias:continue
                first,last=field['physical_range']
                matching=[x for x in proto[0].iter('field')if x.get('name')==ws_name(alias)and int(x.get('pos','-1'))==first and int(x.get('size','-1'))==last-first]if len(proto)==1 else[]
                check=dict(case=case,frame=r['frame'],field=alias,physical_range=[first,last],matches=len(matching),equal=False)
                if len(matching)==1:
                    try:check['equal']=same(ws_value(field,matching[0]),field['value'])
                    except Exception as e:check['error']=repr(e)
                comparisons.append(check)
                if not check['equal']:differences.append(check)
        proof=dict(case=case,classification='owned live OS-generated traffic'if case.startswith('live-')else'literal public-frame excerpt'if case.startswith('public-')else'authored control',
                   capture=str(cap),capture_SHA256=meta['capture_SHA256'],source_messages=len(source),
                   body_complete=sum(r['source']['body_complete']for r in source),capture_short=sum(r['record']['captured_length']<r['record']['original_length']for r in source),
                   source_facts_SHA256=sha(source_file),arguments=args,binary_SHA256=sha(a.tshark),exit_code=0,
                   seconds=round(time.monotonic()-start,3),PDML_SHA256=sha(ws),stderr_SHA256=sha(stderr))
        write(d/'receipt.json',proof);proofs.append(proof)
    for name,items in(('comparisons.jsonl',comparisons),('differences.jsonl',differences)):
        (a.out/name).write_text(''.join(json.dumps(x,sort_keys=True)+'\n'for x in items),encoding='utf8')
    report=dict(scope=__doc__,inputs=5,source_messages=sum(r['source_messages']for r in proofs),comparisons=len(comparisons),differences=len(differences),
        case_selection_file_SHA256=sha(a.case_source),source_constructor_SHA256=sha(Path(__file__).with_name('mld_source_layout.py')),
        reference_semantics_helper_SHA256=sha(Path(__file__).with_name('verify_mld_source_fields.py')),driver_SHA256=sha(__file__),
        proofs=proofs,comparisons_SHA256=sha(a.out/'comparisons.jsonl'),differences_SHA256=sha(a.out/'differences.jsonl'),
        production_decoder_run=False,no_future_whole_expected_objects=True)
    write(a.out/'summary.json',report)
    print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),inputs=5,source_messages=report['source_messages'],comparisons=len(comparisons),differences=len(differences))),flush=True)


if __name__=='__main__':main()
