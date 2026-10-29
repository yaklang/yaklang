"""Source-only IPv6 checksum/envelope observations, not native verdicts.

The selected carriers contain no active Routing header, AH or non-atomic
fragment. No key, authenticated peer or listener state is established.
"""
import argparse,hashlib,ipaddress,json
from pathlib import Path


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--source',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args()
    assert not a.out.exists();rows=[]
    for corpus in('sharon','weber'):
        for f in json.loads((a.source/corpus/'source-facts.json').read_text('utf8')):
            if f['parent']['cross_family']:continue
            raw=Path(f['original_frame_file']).read_bytes();h=f['parent']['offset'];start=f['source_profile']['offset'];end=f['source_profile']['end']
            assert hashlib.sha256(raw).hexdigest()==f['original_frame_SHA256']
            assert not any(x['kind']=='IPv6 extension'and x['type']in(43,44,51)for x in f['source_profile']['headers'])
            wire=raw[start:end];data=raw[h+8:h+40]+len(wire).to_bytes(4,'big')+bytes([0,0,0,58])+wire
            if len(data)%2:data+=b'\0'
            total=sum(int.from_bytes(data[i:i+2],'big')for i in range(0,len(data),2))
            while total>>16:total=(total&65535)+(total>>16)
            source=ipaddress.IPv6Address(raw[h+8:h+24])
            allowed=source.is_link_local or(f['source']['type']==143 and source.is_unspecified)
            multicast=[x for x in f['source']['fields']if x['semantic']=='Record Multicast Address']
            rows.append(dict(corpus=corpus,frame=f['frame'],source_ICMP_range=[start,end],
                source_ICMP_SHA256=hashlib.sha256(wire).hexdigest(),mathematical_IPv6_ICMP_checksum_match=total==65535,
                IPv6_source_shape_allowed=allowed,HopLimit1=f['parent']['hop_limit']==1,RouterAlert0=f['parent']['Router_Alert_values']==[0],
                record_addresses_multicast=all(ipaddress.IPv6Address(x['value']).is_multicast for x in multicast),body_complete=f['source']['body_complete']))
    assert len(rows)==220;a.out.mkdir()
    checks=a.out/'checks.json';checks.write_text(json.dumps(rows,indent=2)+'\n',encoding='utf8')
    report=dict(scope=__doc__,source_records=220,all_checksum_matches=all(x['mathematical_IPv6_ICMP_checksum_match']for x in rows),
        all_source_shapes_allowed=all(x['IPv6_source_shape_allowed']for x in rows),all_HopLimit1=all(x['HopLimit1']for x in rows),
        all_RouterAlert0=all(x['RouterAlert0']for x in rows),all_record_addresses_multicast=all(x['record_addresses_multicast']for x in rows),
        checks_SHA256=sha(checks),source_summary_SHA256=sha(a.source/'summary.json'),driver_SHA256=sha(__file__))
    result=a.out/'summary.json';result.write_text(json.dumps(report,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(summary=str(result),SHA256=sha(result),source_records=220,all_source_conditions=all(report[k]for k in report if k.startswith('all_')))),flush=True)


if __name__=='__main__':main()
