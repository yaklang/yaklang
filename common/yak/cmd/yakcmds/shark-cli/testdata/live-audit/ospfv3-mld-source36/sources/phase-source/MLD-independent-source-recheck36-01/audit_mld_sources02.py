"""Revisit237 historical observations using original capture bytes and RFC facts.

Historical rows choose the source frame set only. Values and ranges are read
from the original records, independently of old or candidate native fields.
Current a82 output, if requested, is diagnostic comparison, never an oracle.
No complete future native or CLI expected object is generated.
"""
import argparse,collections,hashlib,importlib.util,json,os,subprocess,sys,time,urllib.request
from pathlib import Path
from mld_source_layout import inspect,response_milliseconds,query_seconds


def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb')as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def write(p,x):
    with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2,ensure_ascii=False);f.write('\n')
def load(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def counter(rows,name):return dict(collections.Counter(str(r[name])for r in rows))


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('old-inventory','capture-weber','capture-sharon','capture-helper','carrier-helper','native-before','tshark','out'):
        ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir()
    sys.path.insert(0,str(a.capture_helper.parent));reader=load('mld_capture_reader',a.capture_helper)
    carrier=load('mld_literal_parent_layout',a.carrier_helper)
    old_summary=read(a.old_inventory/'summary.json');assert sha(a.old_inventory/'summary.json')=='3c865ba3c7d54267585ce4efdb7a457d5ba23a27ad54df82420105e13a7ef6c2'
    overall=[];bindings=[];source_files=[]
    for corpus,cap in(('sharon',a.capture_sharon),('weber',a.capture_weber)):
        old_file=a.old_inventory/(corpus+'-source.jsonl');meta=next(x for x in old_summary['corpora']if x['corpus']==corpus)
        assert sha(old_file)==meta['ledger_SHA256']
        historical={r['frame']:r for r in map(json.loads,old_file.open(encoding='utf8'))}
        assert len(historical)==meta['source_bound']
        capture_sha=sha(cap);dest=a.out/corpus;dest.mkdir();frames=dest/'original-frames';frames.mkdir()
        records={};facts=[]
        for frame,record in enumerate(reader.capture_records(cap),1):
            if frame not in historical:continue
            old=historical[frame];raw=record['wire'];assert sha_bytes(raw)==old['wire_SHA256'] and capture_sha==old['capture_SHA256']
            for key,value in old['record'].items():
                if key!='frame':assert record[key]==value
            location=carrier.locate(record)
            assert location==old['source_profile']
            networks=[x for x in location['headers']if x['kind']in('IPv4','IPv6')];parent=networks[-1]
            at=location['offset'];end=location['end'];body=raw[at:end]
            if parent['kind']=='IPv6':declared=parent['offset']+40+parent['declared_payload']-at
            else:declared=parent['offset']+parent['declared_length']-at if parent['declared_length']>=at-parent['offset']else None
            source=inspect(body,at,location['raw_start'],declared)
            target=frames/f'frame-{frame}.bin';target.write_bytes(raw)
            parent_facts=dict(kind=parent['kind'],offset=parent['offset'],source=parent['source'],destination=parent['destination'],
                declared_ICMP_extent=declared,declared_envelope_complete=declared==len(body),cross_family=parent['kind']!='IPv6',
                capture_record_short=record['captured_length']<record['original_length'])
            if parent['kind']=='IPv6':
                start=parent['offset'];parent_facts['hop_limit']=raw[start+7]
                parent_facts['Router_Alert_values']=[]
                for h in location['headers']:
                    if h['kind']!='IPv6 extension'or h['type']!=0:continue
                    off=h['offset']+2;limit=h['offset']+h['length']
                    while off<limit:
                        kind=raw[off]
                        if kind==0:off+=1;continue
                        assert off+2<=limit;size=raw[off+1];assert off+2+size<=limit
                        if kind==5 and size==2:parent_facts['Router_Alert_values'].append(int.from_bytes(raw[off+2:off+4],'big'))
                        off+=2+size
            else:parent_facts['IPv4_Total_Length']=parent['declared_length']
            row=dict(corpus=corpus,frame=frame,capture_SHA256=capture_sha,original_frame_file=str(target),original_frame_SHA256=sha_bytes(raw),
                     record={k:v for k,v in record.items()if k!='wire'},source_profile=location,parent=parent_facts,source=source,
                     original_04_body_status=old['facts']['status'])
            facts.append(row);records[frame]=row
        assert set(records)==set(historical)
        datafile=dest/'source-facts.json';write(datafile,facts)
        # This independent timer arithmetic is also checked exhaustively at
        # each encoded word, not merely on a few captured/synthetic examples.
        args=[str(a.tshark),'-n','-2','-r',str(cap),'-Y','frame.number in {'+', '.join(map(str,sorted(records)))+'}','-T','pdml']
        ws=dest/'wireshark.pdml';err=dest/'wireshark.stderr.txt';cfg=dest/'fresh-wireshark-config';cfg.mkdir()
        env=os.environ.copy();env['WIRESHARK_CONFIG_DIR']=str(cfg);env.pop('SSLKEYLOGFILE',None)
        start=time.monotonic()
        with ws.open('xb')as out,err.open('xb')as stderr:
            proc=subprocess.run(args,stdout=out,stderr=stderr,env=env,creationflags=0x08000000)
        assert proc.returncode==0 and sha(cap)==capture_sha
        write(dest/'wireshark-receipt.json',dict(arguments=args,binary_SHA256=sha(a.tshark),capture_SHA256=capture_sha,
             exit_code=0,elapsed_seconds=round(time.monotonic()-start,3),PDML_SHA256=sha(ws),stderr_SHA256=sha(err),
             source_facts_SHA256=sha(datafile),selected_physical_records=len(records),no_explicit_keys=True))
        # Diagnostic-only current checkpoint. Never call inspect on Raw.
        native=a.native_before/'preservation'/corpus/'events.jsonl';producer=native.with_name('events-producer.json');nm=read(producer)
        assert nm['source']=='a82acd0fff97ae50dfc1bbb403c16af359d7bf8f'and nm['exit_code']==0 and sha(native)==nm['export_SHA256']
        current=[]
        for line in native.open(encoding='utf8'):
            event=json.loads(line);refs=event['source_bytes'].get('PacketRefs')or[]
            selected={r['Number']for r in refs}&set(records)
            if not selected:continue
            for frame in sorted(selected):
                current.append(dict(frame=frame,id=event['id'],protocol=event['protocol'],profile=event['profile'],
                    status=event['status'],completeness=event['completeness'],expert=event['expert'],summary=event['summary'],
                    source_bytes=event['source_bytes'],fields=event['fields'],field_values=event['field_values']))
        write(dest/'current-a82-diagnostic-only.json',current)
        overall.extend(facts)
        bindings.append(dict(corpus=corpus,source_count=len(facts),historical_body_complete=meta['source_statuses'].get('complete',0),
            historical_body_incomplete=meta['source_statuses'].get('incomplete',0),cross_family=sum(r['parent']['cross_family']for r in facts),
            actual_IPv6=sum(not r['parent']['cross_family']for r in facts),source_types=counter([r['source']for r in facts],'type'),
            source_file=str(datafile),source_SHA256=sha(datafile),current_native_producer_SHA256=sha(producer),
            current_native_diagnostic_file=str(dest/'current-a82-diagnostic-only.json'),current_native_diagnostic_SHA256=sha(dest/'current-a82-diagnostic-only.json'),
            current_protocols=counter(current,'protocol'),current_profiles=counter(current,'profile')))
    for code in range(65536):
        raw=response_milliseconds(code)
        alternate=code if code<0x8000 else ((code&0xfff)|0x1000)<<(((code>>12)&7)+3)
        assert raw==alternate and 0<=raw<=8387584
    for code in range(256):
        raw=query_seconds(code);alternate=code if code<128 else ((code&15)|16)<<(((code>>4)&7)+3)
        assert raw==alternate and 0<=raw<=31744
    references=[];refs=a.out/'normative-primary-references';refs.mkdir()
    for number in(9777,3810,2710,3590):
        url=f'https://www.rfc-editor.org/rfc/rfc{number}.txt';data=urllib.request.urlopen(url,timeout=30).read()
        file=refs/f'rfc{number}.txt';file.write_bytes(data)
        references.append(dict(URL=url,file=str(file),SHA256=sha(file)))
    report=dict(scope=__doc__,historical_observations=237,source_records=len(overall),sources=bindings,
        canonical_IPv6_observations=sum(not r['parent']['cross_family']for r in overall),
        historical_body_incomplete_all_cross_family=all(r['parent']['cross_family']for r in overall if r['original_04_body_status']=='incomplete'),
        timer_arithmetic_cross_checked_16bit_words=65536,timer_arithmetic_cross_checked_8bit_codes=256,
        normative_references=references,constructor_SHA256=sha(Path(__file__).with_name('mld_source_layout.py')),
        driver_SHA256=sha(__file__),capture_helper_SHA256=sha(a.capture_helper),carrier_helper_SHA256=sha(a.carrier_helper),
        no_future_whole_expected_objects=True,no_Go_run=True,no_repository_changes=True,
        old_inventory_SHA256=sha(a.old_inventory/'summary.json'))
    write(a.out/'summary.json',report)
    print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),source_records=len(overall),source_counts=bindings)),flush=True)


def sha_bytes(raw):return hashlib.sha256(raw).hexdigest()
if __name__=='__main__':main()
