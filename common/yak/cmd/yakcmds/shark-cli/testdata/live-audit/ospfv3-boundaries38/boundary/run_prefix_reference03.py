"""Reference only two new authored captured LSA-header prefixes.

Source-required complete scalar prefixes remain the oracle even if Wireshark
omits them. Raw header matches and reference omissions are recorded separately.
No candidate output is read and no native producer is executed.
"""
import argparse,hashlib,json,os,subprocess,time,xml.etree.ElementTree as ET
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('plan','grid','tshark','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();assert sha(a.plan/'plan.json')=='e9f5b8e4f2820bf12ca79bf7049562442f0bec7540d1a910c7dd2d11da570cd0'
    assert sha(a.grid/'plan.json')=='8b77033c3bd933124f15b0af9c80e3823b86c64510fe6cf9dec3c64c9ed4280d';gp=read(a.grid/'plan.json');assert sha(gp['data_file'])==gp['data_SHA256']
    grid={x['case']:x for x in read(gp['data_file'])};plan=read(a.plan/'plan.json');assert sha(a.plan/'cases.json')==plan['cases_SHA256']
    cases=[x for x in read(a.plan/'cases.json')if x['name'].startswith('capture-short-next-LSA-prefix')];assert len(cases)==2;a.out.mkdir()
    binary_sha=sha(a.tshark);results=[];scalar_checks=[]
    for c in cases:
        for k in('capture','original_frame','source_facts'):assert sha(c[k])==c[k+'_SHA256']
        d=a.out/c['name'];d.mkdir();cfg=d/'fresh-wireshark-config';cfg.mkdir();env=dict(os.environ,WIRESHARK_CONFIG_DIR=str(cfg));env.pop('SSLKEYLOGFILE',None)
        args=[str(a.tshark),'-n','-2','-r',c['capture'],'-T','pdml'];start=time.monotonic()
        with(d/'wireshark.pdml').open('xb')as out,(d/'wireshark.stderr.txt').open('xb')as err:proc=subprocess.run(args,stdout=out,stderr=err,env=env,creationflags=0x08000000,timeout=30)
        assert proc.returncode==0
        packets=ET.parse(d/'wireshark.pdml').getroot().findall('packet');assert len(packets)==1
        fields=[f for f in packets[0].iter('field')if f.get('name','').startswith('ospf')];wire=Path(c['original_frame']).read_bytes()
        expected=[l for l in grid[c['name']]['source_fields']['source_leaves']if l['path'][:2]==['LSAs',1]]
        header=[]
        for name,at,size in(('ospf.version',54,1),('ospf.msg',55,1),('ospf.packet_length',56,2),('ospf.checksum',66,2)):
            got=[f for f in fields if f.get('name')==name and int(f.get('pos','-1'))==at and int(f.get('size','-1'))==size]
            assert len(got)==1 and bytes.fromhex(got[0].get('value',''))==wire[at:at+size];header.append(name)
        for leaf in expected:
            at,end=leaf['physical_range'];got=[f for f in fields if int(f.get('pos','-1'))==at and int(f.get('size','-1'))==end-at and f.get('value')and bytes.fromhex(f.get('value'))==wire[at:end]]
            scalar_checks.append(dict(case=c['name'],source_path=leaf['path'],physical_range=[at,end],source_value=leaf['JSON_value'],matching_reference_fields=[f.get('name')for f in got],observed_in_reference=bool(got)))
        current=dict(case=c['name'],arguments=args,TShark_SHA256=binary_sha,capture_SHA256=c['capture_SHA256'],source_facts_SHA256=c['source_facts_SHA256'],exit_code=proc.returncode,seconds=round(time.monotonic()-start,3),PDML_SHA256=sha(d/'wireshark.pdml'),stderr_SHA256=sha(d/'wireshark.stderr.txt'),source_header_prefix_scalars=len(expected),raw_packet_header_checks=len(header),authored_not_real_traffic=True,source_missing_Length=True,source_missing_body=True)
        write(d/'receipt.json',current);results.append(current)
        assert sha(a.tshark)==binary_sha
        for k in('capture','original_frame','source_facts'):assert sha(c[k])==c[k+'_SHA256']
    report=dict(scope=__doc__,source_plan_SHA256=sha(a.plan/'plan.json'),field_plan_SHA256=sha(a.grid/'plan.json'),inputs=2,physical_records=2,raw_packet_header_checks=8,source_prefix_scalar_checks=len(scalar_checks),prefix_scalars_shown=sum(x['observed_in_reference']for x in scalar_checks),prefix_scalar_reference_omissions=sum(not x['observed_in_reference']for x in scalar_checks),checks=scalar_checks,results=results,driver_SHA256=sha(__file__),candidate_output_read=False,native_executed=False)
    write(a.out/'summary.json',report);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),source_prefix_scalars=len(scalar_checks),shown=report['prefix_scalars_shown'],omitted=report['prefix_scalar_reference_omissions'])),flush=True)
if __name__=='__main__':main()
