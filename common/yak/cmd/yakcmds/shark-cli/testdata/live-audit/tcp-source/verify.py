"""Portable whole705 CLI/SDK/worker verification, no field/path waivers."""
import argparse,base64,collections,concurrent.futures,copy,hashlib,json,os,subprocess,sys,time
from pathlib import Path


def sha(path):
    h=hashlib.sha256()
    with Path(path).open('rb')as f:
        for b in iter(lambda:f.read(1048576),b''):h.update(b)
    return h.hexdigest()


def rows(path):
    with Path(path).open(encoding='utf-8-sig')as f:
        for line in f:yield json.loads(line)


def same(a,b):
    if type(a)is not type(b):return False
    if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
    if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
    return a==b


def write(p,v):p.write_text(json.dumps(v,indent=2,sort_keys=True)+'\n',encoding='utf8')


def main():
    ap=argparse.ArgumentParser()
    for n in['expected','capture','native','cli','bins','audit-modules','out']:ap.add_argument('--'+n,type=Path,required=True)
    ap.add_argument('--source',required=True);a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(a.audit_modules))
    from vxlan_endpoint_projection_audit import capture_records
    s=json.loads((a.expected/'summary.json').read_text());assert s['whole_expected_SHA256']==sha(a.expected/'whole-expected.jsonl')and s['source_capture_SHA256']==sha(a.capture)
    expected={x['frame']:x for x in rows(a.expected/'whole-expected.jsonl')};assert len(expected)==s['sources']==705
    records={}
    for frame,r in enumerate(capture_records(a.capture),1):
        if frame not in expected:continue
        r['frame']=frame;x=expected[frame];assert hashlib.sha256(r['wire']).hexdigest()==x['wire_SHA256']and {k:v for k,v in r.items()if k!='wire'}==x['record'];records[frame]=r
    assert records.keys()==expected.keys()
    receipts={}
    for name in['yak','events']:
        p=a.bins/(name+'-receipt.json');r=json.loads(p.read_text());assert r['source']==a.source and r['exit_code']==0 and r['binary_SHA256']==sha(a.bins/(name+'.exe'));receipts[name]=dict(receipt_SHA256=sha(p),binary_SHA256=r['binary_SHA256'])
    producers={}
    for name,root,filename,binary in [('SDK',a.native,'events','events'),('CLI',a.cli,'yak','yak')]:
        p=root/'preservation/sharon'/(filename+'-producer.json');r=json.loads(p.read_text());target=root/'preservation/sharon'/(filename+'.jsonl');assert r['exit_code']==0 and r['source']==a.source and r['capture_SHA256']==sha(a.capture)and r['export_SHA256']==sha(target)and r['binary_SHA256']==receipts[binary]['binary_SHA256'];producers[name]=dict(producer_SHA256=sha(p),export_SHA256=sha(target))
    contributions=[];SDK_rows=0
    for event in rows(a.native/'preservation/sharon/events.jsonl'):
        SDK_rows+=1
        refs=event.get('source_bytes',{}).get('PacketRefs')or[]
        if any(ref['Number']in expected for ref in refs):contributions.append(event)
    write(a.out/'unexpected-native-contributions.json',contributions);assert not contributions,'own error-bearing sources contributed native state'
    actual={};scope=set();errors=[];CLI_rows=0
    for row in rows(a.cli/'preservation/sharon/yak.jsonl'):
        CLI_rows+=1;n=row.get('number')
        if row.get('protocol_scope')=='captured TCP header diagnostic':scope.add(n)
        if n not in expected:continue
        assert n not in actual;actual[n]=row
        if not same(row,expected[n]['whole_expected_row']):errors.append(dict(frame=n,scope='whole public CLI',expected=expected[n]['whole_expected_row'],actual=row))
    assert actual.keys()==scope==expected.keys(),'a TCP diagnostic source was added/lost, or discovered coverage changed'
    folder=a.out/'worker';folder.mkdir();env=dict(os.environ,YAK_SHARK_WORKER='1',GOMAXPROCS='1',GOMEMLIMIT='1GiB')
    def worker(frame):
        r=records[frame];request=dict(Number=frame,Data=base64.b64encode(r['wire']).decode(),Length=r['original_length'],Link=r['link'],Events=[]);inp=folder/(str(frame)+'.request.json');write(inp,request)
        started=time.time();p=subprocess.run([str(a.bins/'yak.exe'),'shark-worker'],input=inp.read_bytes(),stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,timeout=20,creationflags=0x08000000)
        stdout=folder/(str(frame)+'.stdout');stdout.write_bytes(p.stdout);stdout.with_suffix('.stderr').write_bytes(p.stderr);assert p.returncode==0
        marker=b'\x1eYAKSHARK:';assert marker in p.stdout;response=json.loads(p.stdout.split(marker)[-1]);out=folder/(str(frame)+'.response.json');write(out,response);eq=same(response,expected[frame]['whole_expected_worker'])
        record=dict(frame=frame,request_SHA256=sha(inp),response_SHA256=sha(out),stdout_SHA256=sha(stdout),stderr_SHA256=sha(stdout.with_suffix('.stderr')),exit_code=0,seconds=round(time.time()-started,3),whole_response_equal=eq)
        if eq:return record,None
        return record,dict(frame=frame,scope='whole worker',expected=expected[frame]['whole_expected_worker'],actual=response)
    worker_results=[]
    with concurrent.futures.ThreadPoolExecutor(max_workers=2)as pool:
        for result,error in pool.map(worker,sorted(expected)):
            worker_results.append(result)
            if error:errors.append(error)
    (a.out/'worker-comparisons.jsonl').write_text(''.join(json.dumps(x,sort_keys=True)+'\n'for x in worker_results),encoding='utf8');write(a.out/'differences.json',errors);assert not errors,str(a.out/'differences.json')
    controls=[]
    def mutate(name,source,target,fn):
        c=copy.deepcopy(source);fn(c);assert not same(c,target),name;controls.append(dict(name=name,rejected=True))
    full=next(n for n,x in expected.items()if x['own_failure']['complete']);short=next(n for n,x in expected.items()if x['own_failure']['category']=='fixed-header-incomplete'and x['literal_location']['own_end']-x['literal_location']['own_offset']==2)
    for name,fn in[
        ('Protocol cannot inheritTLS',lambda x:x.update(protocol='TLS')),
        ('FlowID cannot appear',lambda x:x.update(tcp_flow_id=2)),
        ('LogicalSource cannot appear',lambda x:x.update(message_source='192.0.2.1:443')),
        ('Physical endpoint cannot change',lambda x:x.update(source='192.0.2.2')),
        ('Wire integer scalar type cannotchange',lambda x:x.update(captured_length=float(x['captured_length']))),
    ]:mutate(name,actual[full],expected[full]['whole_expected_row'],fn)
    mutate('Short TCP cannot inventcaptured destinationport0',actual[short],expected[short]['whole_expected_row'],lambda x:x.update(destination_port=0))
    mutate('Short TCP cannot claimempty payload',actual[short],expected[short]['whole_expected_row'],lambda x:x.update(info=x['info'].replace('payload boundary unknown','Payload=0')))
    response=json.loads((folder/(str(short)+'.response.json')).read_text());target=expected[short]['whole_expected_worker']
    for name,fn in[
        ('Source0 field cannot be movedtoSource1',lambda x:x['Fields'][4].update(Source=1)),
        ('Source0 range cannot shift',lambda x:x['Fields'][4].update(Start=x['Fields'][4]['Start']+1)),
        ('Uncaptured destinationword cannotdefaultzero',lambda x:x['Fields'].append(dict(Depth=1,Text='Destination Port: 0',Branch=False,Start=0,End=0,HasRange=False,Source=0))),
        ('Logical byte source cannot be created',lambda x:x.update(Sources=[dict(Name='TCP',Data='AAAA')]))
    ]:mutate(name,response,target,fn)
    write(a.out/'negative-controls.json',controls)
    summary=dict(scope='Finalized original full-public CLI and SDK plus every worker response, compared as entire objects to fresh source705 expectations. No fields or paths ignored. Scope is these705 discovered direct own-TCP failures; 145 pre-candidate certificate remains separate. Quick field equality is covered by targeted Go; there is no separate external quick API.',source=a.source,sources=len(expected),whole_public_CLI_equal=len(actual),whole_worker_equal=len(worker_results),native_contributions=0,SDK_rows_scanned=SDK_rows,CLI_rows_scanned=CLI_rows,statuses=s['categories'],negative_controls=len(controls),receipts=receipts,producers=producers,expected_summary_SHA256=sha(a.expected/'summary.json'),capture_SHA256=sha(a.capture),auditor_SHA256=sha(Path(__file__)),evidence={p.name:sha(p)for p in a.out.iterdir()if p.is_file()})
    target=a.out/'summary.json';write(target,summary);print(json.dumps(dict(summary=str(target),SHA256=sha(target),whole_public_CLI=705,whole_worker=705,negative_controls=len(controls))),flush=True)


if __name__=='__main__':main()
