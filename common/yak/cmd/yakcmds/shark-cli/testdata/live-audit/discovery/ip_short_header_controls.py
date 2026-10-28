"""Copy original short headers and derive declared prefix-observation controls.

No network fault injection is claimed. Each owned control keeps every original
socket capture record exact and in order, then inserts one explicitly declared
capture prefix with a future or literal epoch-zero time. Transport bytes are
unavailable in that additional record, never replaced by fabricated traffic.
"""
import argparse,hashlib,json,struct,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from capture_clock_wire_audit import physical


def digest(wire):return hashlib.sha256(wire).hexdigest()
def block(kind,body):
    assert len(body)%4==0
    size=len(body)+12;return struct.pack('<II',kind,size)+body+struct.pack('<I',size)


def pcapng(path,link,records):
    with path.open('xb')as out:
        out.write(block(0x0a0d0d0a,struct.pack('<IHHq',0x1a2b3c4d,1,0,-1)))
        out.write(block(1,struct.pack('<HHI',link,0,65535)+struct.pack('<HH',9,1)+b'\x09\0\0\0'+b'\0'*4))
        for record in records:
            wire=record['wire'];stamp=record['time_ns'];size=len(wire)
            out.write(block(6,struct.pack('<IIIII',0,stamp>>32,stamp&0xffffffff,size,record['original_length'])+wire+b'\0'*((-size)%4)))


def network_offset(record):
    wire=record['wire']
    if record['link']==0:
        assert wire[:4]==b'\x02\0\0\0';return 4
    assert record['link']==1 and wire[12:14]==b'\x08\0';return 14


def run(public,inventory,tls,quic,coap,out):
    for p in[public,inventory,tls,quic,coap,out]:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    out.mkdir(parents=True,exist_ok=False);source=physical(public);selected=[v for v in json.loads(inventory.read_text('utf8'))['rows']if v['category']=='short Ethernet IPv4 fixed header'];assert len(selected)==5
    def save(name,capture,observations,ledger,scope):
        dest=out/name;dest.mkdir();links={v['link']for v in observations};assert len(links)==1
        pcapng(dest/'observation.pcapng',next(iter(links)),observations)
        manifest=dict(scope=__doc__,generator_SHA256=digest(Path(__file__).read_bytes()),case_scope=scope,source_capture=str(capture.resolve()),source_capture_SHA256=digest(capture.read_bytes()),capture_SHA256=digest((dest/'observation.pcapng').read_bytes()),ledger=ledger,original_interface_mapping='source interface explicitly mapped to output section/interface zero',physical_records=len(observations),all_original_primary_records_preserved=scope!='original-record-filter')
        (dest/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(case=name,records=len(observations))),flush=True)
    observations=[];ledger=[]
    for entry in sorted(selected,key=lambda v:v['frame']):
        record=source[entry['frame']-1];wire=record['wire'];assert digest(wire)==entry['wire_SHA256']and 14<len(wire)<34 and wire[12:14]==b'\x08\0'
        observations.append(dict(record,original_length=len(wire)));ledger.append(dict(number=len(ledger)+1,source_frame=entry['frame'],source_section=record['section'],source_interface=record['interface'],source_wire_SHA256=digest(wire),source_time_ns=record['time_ns'],time_ns=record['time_ns'],change=dict(kind='exact-filtered-record')))
    save('original-five',public,observations,ledger,'original-record-filter')
    for kind,capture in[('TLS',tls),('QUIC',quic),('CoAP',coap)]:
        primary=physical(capture);assert len({v['link']for v in primary})==1
        usable=[]
        for n,r in enumerate(primary,1):
            at=network_offset(r)
            if len(r['wire'])>at+40 and r['wire'][at]>>4==4:usable.append((n,at))
        assert usable;selected,at=usable[len(usable)//2];original=primary[selected-1]
        for mode,available,shift in[('short-future',4,3600_000_000_000),('absent-future',0,3600_000_000_000),('short-zero-epoch',4,None)]:
            observations=[];ledger=[]
            for n,record in enumerate(primary,1):
                wire=record['wire'];observations.append(dict(record,original_length=len(wire)))
                ledger.append(dict(number=len(ledger)+1,source_frame=n,source_section=record['section'],source_interface=record['interface'],source_wire_SHA256=digest(wire),source_time_ns=record['time_ns'],time_ns=record['time_ns'],change=dict(kind='exact')))
                if n!=selected:continue
                stamp=0 if shift is None else original['time_ns']+shift;prefix=wire[:at+available]
                observations.append(dict(record,wire=prefix,time_ns=stamp,original_length=len(wire)))
                ledger.append(dict(number=len(ledger)+1,source_frame=n,source_section=record['section'],source_interface=record['interface'],source_wire_SHA256=digest(wire),source_time_ns=record['time_ns'],time_ns=stamp,change=dict(kind='incomplete-fixed-header-observation',IP_header_offset=at,available_fixed_header_bytes=available,required_fixed_header_bytes=20,original_wire_length=len(wire),observation_clock_offset_ns=stamp-original['time_ns'],timestamp_interpretation='literal PCAP epoch zero'if shift is None else'literal future timestamp')))
            save(kind+'-'+mode,capture,observations,ledger,'all-original-records-plus-one-declared-prefix')


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['public','inventory','tls','quic','coap','out']:ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();run(a.public,a.inventory,a.tls,a.quic,a.coap,a.out)
