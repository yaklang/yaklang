"""Close literal TShark reference for frozen small authored OSPF controls.

No native/CLI producer is executed or read. Reference omissions and semantic
checksum presentation are retained. A displayed AT dummy digest is never an
authenticated packet, and reference default budgets do not test codec limits.
"""
import argparse,hashlib,json,os,subprocess,time,xml.etree.ElementTree as ET
from pathlib import Path


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,x):
    with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('plan','tshark','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();plan=read(a.plan/'plan.json')
    assert sha(a.plan/'plan.json')=='83aa407034d0f21f8ffab5217f6a6b990bbf7e2d2cddb678ff8b2d25d23cac19'
    assert sha(a.plan/'cases.json')==plan['cases_SHA256'];cases=read(a.plan/'cases.json');assert len(cases)==18
    a.out.mkdir();results=[];checks=[];differences=[];binary_sha=sha(a.tshark)
    for case in cases:
        capture=Path(case['capture']);frame=Path(case['original_frame']);facts=Path(case['source_facts'])
        for path,key in((capture,'capture_SHA256'),(frame,'original_frame_SHA256'),(facts,'source_facts_SHA256')):assert sha(path)==case[key]
        source=read(facts);wire=frame.read_bytes();d=a.out/case['name'];d.mkdir();cfg=d/'fresh-wireshark-config';cfg.mkdir()
        env=os.environ.copy();env['WIRESHARK_CONFIG_DIR']=str(cfg);env.pop('SSLKEYLOGFILE',None)
        args=[str(a.tshark),'-n','-2','-r',str(capture),'-T','pdml'];pdml=d/'wireshark.pdml';stderr=d/'wireshark.stderr.txt';start=time.monotonic()
        with pdml.open('xb')as out,stderr.open('xb')as err:proc=subprocess.run(args,stdout=out,stderr=err,env=env,creationflags=0x08000000)
        assert proc.returncode==0 and sha(capture)==case['capture_SHA256']and sha(a.tshark)==binary_sha
        packets=ET.parse(pdml).getroot().findall('packet');assert len(packets)==1
        packet=packets[0];protocols=[p for p in packet.iter('proto')if p.get('name')=='ospf'and int(p.get('pos','-1'))==54]
        fields=list(protocols[0].iter('field'))if len(protocols)==1 else[]
        for alias,at,size in(('ospf.version',54,1),('ospf.msg',55,1),('ospf.packet_length',56,2),('ospf.checksum',66,2)):
            matching=[f for f in fields if f.get('name')==alias and int(f.get('pos','-1'))==at and int(f.get('size','-1'))==size]
            good=len(matching)==1 and bytes.fromhex(matching[0].get('value',''))==wire[at:at+size]
            row=dict(case=case['name'],kind='packet-header-raw-source',field=alias,physical_range=[at,at+size],matches=len(matching),equal=good)
            checks.append(row)
            if not good:differences.append(row)
        oracle=source.get('independent_source_codec',{})
        for index,lsa in enumerate(oracle.get('full_LSAs',[])):
            h=lsa['header_fields']['LS_type'];start,end=h['physical_range']
            matching=[f for f in fields if f.get('name')=='ospf.v3.lsa'and int(f.get('pos','-1'))==start and int(f.get('size','-1'))==end-start]
            good=len(matching)==1 and bytes.fromhex(matching[0].get('value',''))==wire[start:end]
            row=dict(case=case['name'],kind='full-LSA-header-raw-source',index=index,source_type=h['value'],physical_range=[start,end],matches=len(matching),equal=good,
                     independent_source_Fletcher_match=lsa['full_LSA_checksum']['valid'])
            checks.append(row)
            if not good:differences.append(row)
        literal=[dict(name=f.get('name'),pos=f.get('pos'),size=f.get('size'),show=f.get('show'),showname=f.get('showname'))
                 for f in packet.iter('field')if f.get('name','').startswith('ospf')and('checksum'in f.get('name','')or f.get('name')in('ospf.v3.lsa','ospf.options.at','ospf.options.l'))]
        summary=dict(case=case['name'],capture_SHA256=case['capture_SHA256'],source_facts_SHA256=case['source_facts_SHA256'],arguments=args,
            TShark_SHA256=binary_sha,exit_code=0,seconds=round(time.monotonic()-start,3),PDML_SHA256=sha(pdml),stderr_SHA256=sha(stderr),
            packet_count=1,literal_checksum_and_LSA_presentation=literal,source_math_packet_match=source['mathematical_packet_checksum_match'],
            direct_codec_context_or_budget_not_tested=bool(case.get('requires_direct_codec_invocation')),
            dummy_AT_not_authentication=True,authored_not_real_traffic=True)
        write(d/'receipt.json',summary);results.append(summary)
    for name,data in(('comparisons.jsonl',checks),('differences.jsonl',differences)):
        (a.out/name).write_text(''.join(json.dumps(x,sort_keys=True)+'\n'for x in data),encoding='utf8')
    report=dict(scope=__doc__,source_plan_SHA256=sha(a.plan/'plan.json'),inputs=18,packets=18,checks=len(checks),differences=len(differences),
        maximum_frame_bytes=142,total_pcap_bytes=2704,all_TShark_exit0=True,results=results,
        comparisons_SHA256=sha(a.out/'comparisons.jsonl'),differences_SHA256=sha(a.out/'differences.jsonl'),driver_SHA256=sha(__file__),
        native_or_CLI_producer_run=False,candidate_values_read=False,reference_differences_preserved=True)
    write(a.out/'summary.json',report)
    print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),inputs=18,checks=len(checks),differences=len(differences))),flush=True)


if __name__=='__main__':main()
