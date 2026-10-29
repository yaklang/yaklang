"""Independent MLD wire facts, not future native events or listener state.

RFC9777/3810 sections5.1,5.2,8.1 supply format and timer arithmetic. Each
field records original-byte and logical-ICMP coordinates. A cross-family or
unknown-extent body interpretation is explicitly not canonical MLD admission.
"""
import hashlib,ipaddress


def response_milliseconds(code):
    assert type(code)is int and 0<=code<=65535
    return code if code<32768 else (4096+(code%4096))*2**(((code//4096)%8)+3)


def query_seconds(code):
    assert type(code)is int and 0<=code<=255
    return code if code<128 else (16+(code%16))*2**(((code//16)%8)+3)


def inspect(w,physical_start,native_start,declared_extent):
    assert len(w)>=4 and w[0]in(130,131,132,143)
    result=dict(type=w[0],code=w[1],available_ICMP_bytes=len(w),declared_ICMP_bytes=declared_extent,
                fields=[],records=[],body_complete=False,wire_checksum_verified=False,listener_state_inferred=False)
    def add(name,at,size,kind='unsigned',alias=None,derived=None):
        if at+size>len(w):return None
        raw=w[at:at+size]
        value=str(ipaddress.IPv6Address(raw))if kind=='address'else raw.hex()if kind=='bytes'else int.from_bytes(raw,'big')
        if derived is not None:value=derived(value)
        field=dict(semantic=name,kind=kind,value=value,ICMP_range=[at,at+size],
                   physical_range=[physical_start+at,physical_start+at+size],
                   native_PDU_range=[physical_start-native_start+at,physical_start-native_start+at+size],
                   source_SHA256=hashlib.sha256(raw).hexdigest(),derived=derived is not None)
        if alias:field['public_alias']=alias
        result['fields'].append(field);return value
    def need(end,reason):
        if end<=len(w):return False
        result.update(next_field_required_ICMP_end=end,missing_reason=reason);return True
    def finish(at):
        assert at<=len(w);result['body_complete']=True;result['parsed_ICMP_end']=at
        if at<len(w):
            add('Additional Data: uninterpreted',at,len(w)-at,'bytes')
            result['additional_bytes']=len(w)-at
        else:result['additional_bytes']=0
        result['declared_envelope_complete']=declared_extent is not None and declared_extent==len(w)
        return result
    add('ICMPv6 Type',0,1,alias='icmpv6.type');add('ICMPv6 Code',1,1,alias='icmpv6.code')
    add('ICMPv6 Checksum',2,2,alias='icmpv6.checksum')
    typ=w[0]
    if typ==143:
        result['wire_body_family']='MLDv2 report format'
        add('Report Flags: raw',4,2)
        n=add('Address Record Count',6,2,alias='icmpv6.mldr.nb_mcast_records')
        if need(8,'report fixed header'):return result
        at=8
        for index in range(n):
            rec=dict(index=index,ICMP_start=at,complete=False);result['records'].append(rec)
            if at>=len(w):need(at+20,'record fixed header');return result
            rec['type']=add('Record Type',at,1,alias='icmpv6.mldr.mar.record_type')
            aux=add('Auxiliary Data Length: 32-bit words',at+1,1,alias='icmpv6.mldr.mar.aux_data_len')
            ns=add('Record Source Count',at+2,2,alias='icmpv6.mldr.mar.nb_sources')
            add('Record Multicast Address',at+4,16,'address',alias='icmpv6.mldr.mar.multicast_address')
            if need(at+20,'record fixed header'):return result
            rec.update(auxiliary_words=aux,source_count=ns,declared_bytes=20+16*ns+4*aux,
                       sources_ICMP_range=[at+20,at+20+16*ns],
                       auxiliary_ICMP_range=[at+20+16*ns,at+20+16*ns+4*aux])
            at+=20
            for _ in range(ns):
                if need(at+16,'record source address'):return result
                add('Record Source Address',at,16,'address',alias='icmpv6.mldr.mar.source_address');at+=16
            if need(at+4*aux,'record auxiliary bytes'):return result
            if aux:add('Record Auxiliary Data: uninterpreted',at,4*aux,'bytes',alias='icmpv6.mldr.mar.auxiliary_data')
            at+=4*aux;rec['ICMP_end']=at;rec['complete']=True
        return finish(at)
    if typ in(131,132):
        result['wire_body_family']='MLDv1 report format'if typ==131 else'MLDv1 done format'
        add('Unused Delay Word',4,2);add('Reserved',6,2)
        add('Multicast Address',8,16,'address',alias='icmpv6.mld.multicast_address')
        if need(24,'MLDv1 report/done fixed header'):return result
        return finish(24)
    # Version comes from declared extent, never from captured-length alone.
    version=1 if declared_extent==24 else 2 if declared_extent is not None and declared_extent>=28 else None
    result['wire_body_family']='query format';result['declared_query_version']=version
    add('Response Code or Delay: raw word',4,2,alias='icmpv6.mld.maximum_response_code.raw')
    if version==1:add('Maximum Response Delay: milliseconds',4,2,alias='icmpv6.mld.maximum_response_delay')
    if version==2:add('Maximum Response Delay: milliseconds',4,2,alias='icmpv6.mld.maximum_response_code',derived=response_milliseconds)
    add('Reserved',6,2);add('Multicast Address',8,16,'address',alias='icmpv6.mld.multicast_address')
    if need(24,'query fixed prefix'):return result
    if version is None:
        result['version_refusal']='declared extent unknown or neither24 nor>=28; no inferred timer/version'
        return result
    if version==1:return finish(24)
    add('Query Flags: raw byte',24,1,alias='icmpv6.mld.flag')
    add('Suppress Router-side Processing',24,1,alias='icmpv6.mld.flag.s',derived=lambda x:bool(x&8))
    add('Querier Robustness Variable: raw',24,1,alias='icmpv6.mld.flag.qrv',derived=lambda x:x%8)
    add('Query Extension Flags: raw',24,1,alias='icmpv6.mld.flag.reserved',derived=lambda x:x//16)
    add('Querier Query Interval Code: raw',25,1,alias='icmpv6.mld.qqi.raw')
    add('Querier Query Interval: seconds',25,1,alias='icmpv6.mld.qqi',derived=query_seconds)
    ns=add('Query Source Count',26,2,alias='icmpv6.mld.nb_sources')
    if need(28,'MLDv2 query fixed header'):return result
    result['source_count']=ns;result['declared_body_bytes']=28+16*ns;at=28
    for _ in range(ns):
        if need(at+16,'query source address'):return result
        add('Query Source Address',at,16,'address',alias='icmpv6.mld.source_address');at+=16
    return finish(at)
