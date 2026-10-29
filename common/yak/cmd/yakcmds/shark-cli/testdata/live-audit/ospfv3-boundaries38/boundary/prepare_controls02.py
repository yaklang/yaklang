"""Small authored OSPFv3 boundary controls, independent of candidate output.

These are synthetic one-record captures, never real traffic. The immutable
807-source RFC codec supplies full unknown-opaque/known-sibling source facts.
No native/CLI event, generated PDU ID or complete expected object is created.
"""
import argparse,hashlib,importlib.util,ipaddress,json,struct
from pathlib import Path

SOURCE='fe80::100';DESTINATION='ff02::5'
CODEC_SHA='fb45475fec822b21989d398541c51ec5d62841db4ce75d2e76737fbf1bffb662'


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def sha_bytes(b):return hashlib.sha256(b).hexdigest()
def load(p):
    assert sha(p)==CODEC_SHA
    spec=importlib.util.spec_from_file_location('independent_OSPF_source',p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def write(p,x):
    with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')


def lsa(codec,typ,body,identifier):
    w=bytearray(struct.pack('!HHIIIHH',1,typ,identifier,0xc6336401,0x80000001,0,20+len(body))+body)
    # Solve Fletcher's two checksum octets after excluding LS Age. This is
    # verified by the separately written source codec, not native production.
    c0=c1=0
    for value in w[2:]:c0=(c0+value)%255;c1=(c1+c0)%255
    x=((len(w)-17)*c0-c1)%255;x=x or 255;y=(-c0-x)%255;y=y or 255
    w[16:18]=bytes([x,y]);assert codec.fletcher(bytes(w))['valid'];return bytes(w)


def ospf(codec,typ,body,instance=0,checksum='base',tail=b''):
    w=bytearray(struct.pack('!BBHIIHBB',3,typ,16+len(body),0xc6336401,0,0,instance,0)+body)
    pseudo=ipaddress.IPv6Address(SOURCE).packed+ipaddress.IPv6Address(DESTINATION).packed+len(w).to_bytes(4,'big')+bytes([0,0,0,89])
    if checksum=='base':w[12:14]=codec.checksum(pseudo+bytes(w)).to_bytes(2,'big')
    elif checksum=='nonzero-unverified-AT':w[12:14]=bytes([0x12,0x34])
    else:assert checksum=='zero-unverified-AT'
    return bytes(w)+tail


def frame(w):
    ethernet=bytes.fromhex('33330000000502000000000186dd')
    ipv6=struct.pack('!IHBB',0x60000000,len(w),89,1)+ipaddress.IPv6Address(SOURCE).packed+ipaddress.IPv6Address(DESTINATION).packed
    return ethernet+ipv6+w


def pcap(w,original):
    return struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1)+struct.pack('<IIII',1700000000,0,len(w),original)+w


def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--codec',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args()
    assert not a.out.exists();codec=load(a.codec);a.out.mkdir();cases=[]
    unknown=lsa(codec,0xa123,bytes([1,35,69,103]),1)
    unknown_u0=lsa(codec,0x2123,bytes([1,35,69,103]),1)
    known=lsa(codec,0x2001,bytes([0,0,0,19]),2)
    update=lambda first:ospf(codec,4,(2).to_bytes(4,'big')+first+known)
    base=update(unknown)
    hello=lambda options:struct.pack('!IB',1,1)+options.to_bytes(3,'big')+struct.pack('!HHII',10,40,0,0)
    ll=bytearray(struct.pack('!HH',0,1));ll[:2]=codec.checksum(bytes(ll)).to_bytes(2,'big');ll=bytes(ll)
    bad_ll=bytearray(ll);bad_ll[0]^=1
    # RFC7166 crypto type1 header plus a dummy32-byte digest. It is not a
    # secret, valid HMAC, configured SA, authenticated packet or replay proof.
    at=struct.pack('!HHHHQ',1,48,0,1,1)+bytes(32)
    def add(name,wire,purpose,properties,cut=None,codec_context=None,budget=None):
        full=frame(wire);captured=full if cut is None else full[:54+cut]
        assert len(full)<=256 and len(captured)<=256
        d=a.out/name;d.mkdir();capture=d/'authored.pcap';capture.write_bytes(pcap(captured,len(full)))
        original=d/'authored-frame.bin';original.write_bytes(captured)
        n=int.from_bytes(wire[2:4],'big');available=captured[54:];complete=len(available)>=n
        pseudo=ipaddress.IPv6Address(SOURCE).packed+ipaddress.IPv6Address(DESTINATION).packed+n.to_bytes(4,'big')+bytes([0,0,0,89])
        math_match=codec.checksum(pseudo+available[:n])==0 if complete else None
        facts=dict(authored_not_real_capture=True,source=SOURCE,destination=DESTINATION,physical_PDU_start=54,
            captured_PDU_bytes=len(available),declared_OSPF_bytes=n,physical_original_bytes=len(full),physical_captured_bytes=len(captured),
            capture_short=len(captured)<len(full),complete_declared_OSPF_bytes=complete,mathematical_packet_checksum_match=math_match,
            mathematical_packet_checksum_applicability='conditional base IPv6/OSPF sum;omitted if AT observed',
            declared_parent_bytes=len(wire),outside_OSPF_length_bytes=max(0,len(available)-n),authentication_verified=False)
        if complete and available[14]<64:
            try:facts['independent_source_codec']=codec.message(available,54,SOURCE,DESTINATION)
            except AssertionError as e:facts['source_structure_refusal']=repr(e)
        else:
            facts['body_interpretation']='Opaque when AF context unknown or declared packet incomplete;no IPv6 prefix inference'
        sourcefile=d/'source-facts.json';write(sourcefile,facts)
        row=dict(name=name,classification='authored boundary control,not live/real traffic',capture=str(capture),capture_SHA256=sha(capture),
            original_frame=str(original),original_frame_SHA256=sha(original),source_facts=str(sourcefile),source_facts_SHA256=sha(sourcefile),
            physical_records=1,physical_original_bytes=len(full),physical_captured_bytes=len(captured),purpose=purpose,
            required_properties=properties,complete_expected_native_or_CLI_object=False)
        if codec_context is not None:row['raw_codec_context_override']=codec_context
        if budget is not None:row['raw_codec_collection_budget']=budget
        if budget is not None or codec_context is not None:row['requires_direct_codec_invocation']=True
        cases.append(row)
    add('unknown-u1-then-known',base,'A complete unknown U1 LSA must not hide its exact-length known sibling',
        ['both source LSA headers and known Router body remain inspectable','unknown opaque body remains unsupported semantics','packet and both full-LSA checksum mathematical matches retained','no LSDB/flood/neighbor/authentication state inferred'])
    add('unknown-u0-then-known',update(unknown_u0),'U0 changes RFC unknown flooding scope handling,not byte-boundary admission',
        ['both bounded LSAs retained','U0 does not make opaque unknown body malformed','no inferred actual link-local flood/LSDB action'])
    bad=bytearray(unknown);bad[-1]^=1
    add('unknown-bad-Fletcher',update(bytes(bad)),'Isolate bad unknown Fletcher with a recomputed good outer packet checksum',
        ['packet checksum matches','first complete LSA Fletcher fails and is not masked as merely unsupported','hard Fletcher failure may stop further semantic parsing at its declared boundary','never scan for a later plausible LSA'])
    bad=bytearray(base);bad[12]^=1
    add('bad-packet-sum-unsupported-body',bytes(bad),'A packet-sum failure must survive soft unknown-body status',
        ['packet checksum mismatch explicitly checked/reported','complete LSA Fletcher matches are independent','soft unsupported must not hide the packet checksum failure'])
    add('unknown-AF-instance64',ospf(codec,4,(2).to_bytes(4,'big')+unknown+known,instance=64),'AF unavailable is separate from checksum context',
        ['AF body remains context-required/opaque','complete declared packet mathematical checksum can be checked independently','do not call supplied pseudo-header absent'])
    add('AT-Hello-dummy-digest',ospf(codec,1,hello(0x413),checksum='zero-unverified-AT',tail=at),'Observed AT omits header checksum verification',
        ['AT observed','checksum omitted rather than failed','dummy digest never authenticated','no secret/SA/replay state'])
    add('AT-Hello-no-trailer',ospf(codec,1,hello(0x413),checksum='zero-unverified-AT'),'AT flag without observed trailer must remain explicit context',
        ['AT observed with no tail','no base checksum-failure verdict under AT','missing/authentication context remains explicit'])
    add('LLS-only-good',ospf(codec,1,hello(0x213),tail=ll),'LLS-only packet checksum remains independently applicable',
        ['OSPFn excludes4-byte LLS block','base packet checksum matches','LLS semantics may remain context-required','no authentication claim'])
    add('LLS-only-bad-block-sum',ospf(codec,1,hello(0x213),tail=bytes(bad_ll)),'Bad LLS checksum does not invalidate the OSPF packet checksum/body',
        ['OSPFn checksum still matches','bad LLS block is separately unverified/discardable','Hello fields not hidden by LLS checksum failure'])
    add('AT-plus-LLS-dummy-digest',ospf(codec,1,hello(0x613),checksum='nonzero-unverified-AT',tail=struct.pack('!HH',0,1)+at),'AT follows LLS and omits both checksum checks',
        ['AT follows4-byte LLS block','nonzero OSPF checksum is not a bad-checksum verdict under AT','zero LLS checksum is not independently rejected under AT','dummy digest never authenticated'])
    add('Hello-Options-unavailable-full-declaration',ospf(codec,1,hello(0x13)[:6]),'Malformed full declared Hello lacks complete Options bytes',
        ['Options unavailable distinct from missing IPv6 pseudo-header','do not treat default options0 as observed ATclear','body remains refused'])
    add('capture-short-before-Options',ospf(codec,1,hello(0x13)),'Real caplen/origlen truncation before Options completion',
        ['captured22 of36 OSPF bytes','packet checksum not checked due missing declared tail','Options incomplete reason preserved'],cut=22)
    add('capture-short-after-unknown',base,'Actual record truncated after first complete unknown LSA',
        ['captured44 of68 OSPF bytes','first LSA full Fletcher independent','no whole packet checksum validation or invented known sibling'],cut=44)
    invalid=bytearray(unknown);invalid[18:20]=(19).to_bytes(2,'big')
    add('invalid-declared-LSA-length19',update(bytes(invalid)),'An impossible LSA boundary must not trigger resynchronization',
        ['outer packet mathematical checksum matches','LSA declared length19 rejected','do not label later authored marker bytes as a validated sibling via scanning'])
    add('budget1-bad-packet-sum',bytes(bad),'Confirmed wire-integrity failure coexists with collection-budget refusal',
        ['direct codec error retains budget identity and explicit integrity identity','capture/native integrity handling must not silently treat this as a pure resource refusal','bad packet checksum remains visible in fields/error','soft unsupported context must not mask integrity'],budget=1)
    add('budget1-before-known',base,'Resource refusal is separate from unsupported body and packet checksum',
        ['budget1 remains budget refusal','full wire packet checksum mathematical result independent','do not relabel budget as context or decoded'],budget=1)
    add('pseudo-destination-unavailable',base,'Explicit direct-codec context override,not an ingress claim',
        ['unavailable final pseudo-header destination reason','do not verify protocol checksum with guessed destination','raw source mathematical checksum may match but override prevents native verdict'],codec_context=dict(PseudoHeaderUnavailable=True))
    add('unknown-message-type6',ospf(codec,6,bytes(8)),'Unsupported message semantics does not imply missing supplied pseudo-header',
        ['unselected packet type stays explicit','complete bounded base checksum mathematical result independent','no invented body/neighbor state'])
    write(a.out/'cases.json',cases)
    plan=dict(scope=__doc__,source_constructor_SHA256=sha(__file__),independent_codec=str(a.codec),independent_codec_SHA256=CODEC_SHA,
        authored_controls=len(cases),physical_records=len(cases),maximum_original_frame_bytes=max(x['physical_original_bytes']for x in cases),
        total_capture_file_bytes=sum(Path(x['capture']).stat().st_size for x in cases),cases_SHA256=sha(a.out/'cases.json'),
        runtime_candidate_output_read=False,whole_native_or_CLI_expectations_generated=False,Go_run=False,
        reference_plan=dict(executable='Existing pinned TShark4.6.9',arguments=['-n','-2','-r','<each authored.pcap>','-T','pdml'],
            keylog_used=False,scope='Record literal fields,checksum presentation and known-sibling source positions;retain reference omissions/disagreements. AT digest has no valid key.',
            compare_to='independent original authored bytes and frozen RFC source codec,never candidate output'),
        normative_primary_references=['https://www.rfc-editor.org/rfc/rfc5340.html','https://www.rfc-editor.org/rfc/rfc7166.html#section-4.2','https://www.rfc-editor.org/rfc/rfc5613.html#section-2.2'],
        baseline_execution_pending='Root will run old685 native/CLI after this source plan closes;direct budget/context cases require explicit codec invocation,not packet-label equivalence')
    write(a.out/'plan.json',plan)
    print(json.dumps(dict(plan=str(a.out/'plan.json'),SHA256=sha(a.out/'plan.json'),controls=len(cases),max_frame_bytes=plan['maximum_original_frame_bytes'],total_pcap_bytes=plan['total_capture_file_bytes'])),flush=True)


if __name__=='__main__':main()
