"""Independent RFC 2710/3810 wire facts; no native fields, guesses or peer state.

Ranges address the caller's exact ICMPv6 envelope. Additional bytes are preserved
as uninterpreted data; auxiliary lengths are 32-bit words. A valid wire shape
does not validate checksum, Router Alert, hop limit or multicast membership.
"""
import ipaddress


def inspect(w):
    assert len(w)>=4 and w[0] in (130,131,132,143)
    out=dict(type=w[0],code=w[1],available=len(w),fields=[],records=[],status='complete')
    def add(name,at,size,kind='uint',mask=None,shift=0,derive=None):
        if at+size>len(w):return None
        raw=w[at:at+size]
        value=str(ipaddress.IPv6Address(raw)) if kind=='ipv6' else raw.hex() if kind=='bytes' else int.from_bytes(raw,'big')
        if kind=='ipv6' and ipaddress.IPv6Address(raw).ipv4_mapped is not None:value='::ffff:'+str(ipaddress.IPv6Address(raw).ipv4_mapped)
        encoded=value
        if derive=='mrc' and encoded>=32768:value=((encoded&4095)|4096)<<(((encoded>>12)&7)+3)
        if derive=='qqic' and encoded>=128:value=((encoded&15)|16)<<(((encoded>>4)&7)+3)
        if mask is not None:value=(value&mask)>>shift
        item=dict(name=name,offset=at,length=size,value=value,wire_hex=raw.hex())
        if derive is not None:item.update(wire_value=encoded,derivation='RFC3810 '+derive,unit='milliseconds'if derive=='mrc'else'seconds')
        if mask is not None:item.update(mask=mask,shift=shift)
        out['fields'].append(item)
        return value
    def need(end,reason):
        if end<=len(w):return False
        out.update(status='incomplete',required=end,reason=reason)
        return True
    typ=w[0]
    if typ==143:
        out['version']=2
        n=add('icmpv6.mldr.nb_mcast_records',6,2)
        if need(8,'MLDv2 report fixed header'):return out
        at=8
        for index in range(n):
            rec=dict(index=index+1,offset=at)
            if at>=len(w):
                need(at+20,'MLDv2 address record fixed header');return out
            rec['type']=add('icmpv6.mldr.mar.record_type',at,1)
            aux=add('icmpv6.mldr.mar.aux_data_len',at+1,1)
            ns=add('icmpv6.mldr.mar.nb_sources',at+2,2)
            rec['multicast_address']=add('icmpv6.mldr.mar.multicast_address',at+4,16,'ipv6')
            out['records'].append(rec)
            if need(at+20,'MLDv2 address record fixed header'):return out
            rec.update(declared_sources=ns,aux_words=aux,sources=[])
            at+=20
            for _ in range(ns):
                if need(at+16,'MLDv2 source address'):return out
                rec['sources'].append(add('icmpv6.mldr.mar.source_address',at,16,'ipv6'));at+=16
            if need(at+aux*4,'MLDv2 auxiliary data'):return out
            if aux:rec['auxiliary_data']=add('icmpv6.mldr.mar.auxiliary_data',at,aux*4,'bytes')
            at+=aux*4
        out['consumed']=at
    else:
        add('icmpv6.mld.multicast_address',8,16,'ipv6')
        if typ in (131,132):
            out['version']=1
            if need(24,'MLDv1 report/done fixed header'):return out
            out['consumed']=24
        elif len(w)<=24:
            add('icmpv6.mld.maximum_response_delay',4,2)
            if need(24,'MLD query fixed header'):return out
            out.update(version=1,consumed=24)
        else:
            out['version']=2
            add('icmpv6.mld.maximum_response_code',4,2,derive='mrc')
            flags=add('icmpv6.mld.flag',24,1)
            if flags is not None:
                add('icmpv6.mld.flag.s',24,1,mask=8,shift=3)
                add('icmpv6.mld.flag.qrv',24,1,mask=7)
                add('icmpv6.mld.flag.reserved',24,1,mask=240,shift=4)
            add('icmpv6.mld.qqi',25,1,derive='qqic')
            ns=add('icmpv6.mld.nb_sources',26,2)
            if need(28,'MLDv2 query fixed header'):return out
            at=28
            for _ in range(ns):
                if need(at+16,'MLDv2 query source address'):return out
                add('icmpv6.mld.source_address',at,16,'ipv6');at+=16
            out['consumed']=at
    out['additional_data_hex']=w[out['consumed']:].hex()
    return out
