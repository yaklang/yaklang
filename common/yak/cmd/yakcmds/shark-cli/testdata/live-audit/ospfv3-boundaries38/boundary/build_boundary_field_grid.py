"""Independent complete-source-leaf/range grid for authored OSPF boundaries.

Wire values come only from the frozen authored original bytes. The old pure
schema mapper supplies aliases, never values. This is a Session/private-field
oracle, not GetFields success and not a complete native/CLI event expectation.
"""
import argparse,base64,copy,hashlib,importlib.util,ipaddress,json
from pathlib import Path

META_SHA='26e9f5fd15485d57e8c85bf0a69a3a252106001acfc0e53724b3a3efe2421f71'
TYPED_SHA='ae0359a175fdf9fe8350f37284a1398ca85904fce75b5153c08aec9f9c601a7d'
MAPPER_SHA='4711238e1a310627344de25116e8789d4033bf79d0ce4d09cb98906ce3971c97'
CODEC_SHA='fb45475fec822b21989d398541c51ec5d62841db4ce75d2e76737fbf1bffb662'
HEADER=[('Version',1,'uint'),('Type',1,'uint'),('Packet Length',2,'uint'),('Router ID',4,'id'),('Area ID',4,'id'),('Checksum',2,'uint'),('Instance ID',1,'uint'),('Reserved',1,'uint')]
LSA_HEADER=[('LS Age',2,'uint'),('LS Type',2,'uint'),('Link State ID',4,'id'),('Advertising Router',4,'id'),('Sequence',4,'uint'),('Checksum',2,'uint'),('Length',2,'uint')]


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def load(name,p):
    spec=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')


class Source:
    def __init__(self,case,metadata,codec):
        self.case=case;self.metadata=metadata;self.codec=codec
        frame=Path(case['original_frame']).read_bytes();assert sha(case['original_frame'])==case['original_frame_SHA256']
        self.w=frame[54:];self.leaves=[];self.checks=[];self.nodes=[];self.header_prefix=None
        self.tree={'Wire Field Ranges':{}}
    def add(self,node,path,key,at,size,kind='uint'):
        if at+size>len(self.w):return False
        raw=self.w[at:at+size]
        value=str(ipaddress.IPv4Address(raw))if kind=='id'else base64.b64encode(raw).decode()if kind=='bytes'else int.from_bytes(raw,'big')
        node[key]=value;node.setdefault('Wire Field Ranges',{})[key]=[at,at+size]
        self.leaves.append(dict(path=path+[key],JSON_value=value,kind=kind,PDU_range=[at,at+size],physical_range=[54+at,54+at+size],source_SHA256=hashlib.sha256(raw).hexdigest()))
        return True
    def sequential(self,node,path,at,spec):
        for key,size,kind in spec:
            if not self.add(node,path,key,at,size,kind):return at,False
            at+=size
        return at,True
    def checksum(self,path,verified,scope):self.checks.append(dict(path=path,verified=verified,scope=scope))
    def construct(self):
        w=self.w;t=self.tree;header={};t['Header']=header;_,complete=self.sequential(header,['Header'],0,HEADER);assert complete
        n=header['Packet Length'];typ=header['Type'];instance=header['Instance ID'];name=self.case['name'];body_reason=None
        options=int.from_bytes(w[21:24],'big')if typ==1 and len(w)>=24 else None
        capture_short=n>len(w);at=16
        if instance>=64:body_reason='AF-context';self.nodes.append(dict(path=['Unparsed Body'],owned_bytes_range=[16,len(w)],required=True))
        elif typ==1:
            node={};t['Hello']=node
            at,ok=self.sequential(node,['Hello'],16,[('Interface ID',4,'uint'),('Router Priority',1,'uint'),('Options',3,'uint'),('Hello Interval',2,'uint'),('Router Dead Interval',2,'uint'),('Designated Router',4,'id'),('Backup Designated Router',4,'id')])
            node['Neighbors']=None
            if not ok:body_reason='malformed-declared-Hello'if not capture_short else'capture-short'
        elif typ==4:
            assert self.add(t,[],'LSA Count',16,4);at=20;count=t['LSA Count'];rows=[];t['LSAs']=rows
            limit=self.case.get('raw_codec_collection_budget',1<<30)
            for index in range(count):
                if index>=limit:body_reason='budget';break
                if len(w)-at<2:body_reason='capture-short';break
                start=at;node={};path=['LSAs',len(rows)];rows.append(node)
                at,ok=self.sequential(node,path,at,LSA_HEADER)
                if not ok:
                    self.header_prefix=dict(row_index=path[-1],start=start,captured_bytes=len(w)-start,complete_scalars=[x[0]for x in LSA_HEADER if x[0]in node],no_Length=True)
                    self.checksum(path,False,'unverified-prefix-no-complete-Length-or-body');body_reason='capture-short';break
                length=node['Length']
                if length<20:
                    self.checksum(path,False,'unverified-invalid-declared-LSA-length');body_reason='invalid-LSA-length';break
                if start+length>len(w):
                    self.checksum(path,False,'unverified-LSA-body-unavailable');body_reason='capture-short';break
                end=start+length;body={};node['Body']=body;bpath=path+['Body']
                if node['LS Type']not in(0x2001,):
                    assert self.add(body,bpath,'Opaque Body',start+20,length-20,'bytes')
                    self.nodes.append(dict(path=bpath,exact_keys=['Opaque Body','Wire Field Ranges']))
                else:
                    assert length==24
                    assert self.add(body,bpath,'Flags',start+20,1)and self.add(body,bpath,'Options',start+21,3)
                    body['Links']=None;self.nodes.append(dict(path=bpath,exact_keys=['Flags','Options','Links','Wire Field Ranges']))
                valid=self.codec.fletcher(w[start:end])['valid'];self.checksum(path,valid,'complete LSA excluding LS Age; authentication unverified');at=end
                if not valid:body_reason='LSA-integrity';break
        else:body_reason='unselected-message'
        scopes=self.metadata['shared']['Checksum Scope']
        if capture_short:scope=scopes['captured_message_short'];verified=False
        elif typ==1 and options is None:scope=scopes['Options_unavailable'];verified=False
        elif options is not None and options&0x400:scope=scopes['AT_observed'];verified=False
        elif self.case.get('raw_codec_context_override',{}).get('PseudoHeaderUnavailable'):
            scope=scopes['final_destination_unavailable'];verified=False
        else:
            scope=scopes['complete_base_math'];source=read(self.case['source_facts']);verified=source['mathematical_packet_checksum_match']
        self.checksum([],verified,scope)
        if capture_short:status='incomplete'
        elif body_reason in('LSA-integrity','invalid-LSA-length','malformed-declared-Hello'):status='malformed'
        elif scope==scopes['complete_base_math']and not verified:status='malformed'
        elif body_reason=='budget':status='limited'
        else:status='context-required'
        # The actual Session can contain descriptive metadata too. Every
        # source range/leaf is exact; metadata is separately fenced below.
        return dict(source_wire_tree=t,source_leaves=self.leaves,checksum_requirements=self.checks,structural_requirements=self.nodes,
                    required_observed_LSA_nodes=len(t.get('LSAs',[])),no_nil_LSA_slots=True,partial_LSA_header=self.header_prefix,
                    body_boundary_reason=body_reason,native_refusal_status=status,packet_name={1:'Hello',4:'Link State Update'}.get(typ,'Unknown'),
                    expected_packet_bits=None if options is None else dict(AT=bool(options&0x400),LLS=bool(options&0x200)),
                    parsed_PDU_cursor=at,raw_PDU_bytes=len(w),conditional_context_case=bool(self.case.get('requires_direct_codec_invocation')))


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('source','metadata-contract','typed-contract','mapper','codec','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists()
    assert sha(a.metadata_contract)==META_SHA and sha(a.typed_contract)==TYPED_SHA and sha(a.mapper)==MAPPER_SHA and sha(a.codec)==CODEC_SHA
    assert sha(a.source/'plan.json')=='e9f5b8e4f2820bf12ca79bf7049562442f0bec7540d1a910c7dd2d11da570cd0'
    meta=read(a.metadata_contract);typed=read(a.typed_contract);mapper=load('old_pure_source_alias_mapper',a.mapper);codec=load('independent_boundary_codec',a.codec)
    cases=read(a.source/'cases.json');assert len(cases)==20;rows=[]
    for case in cases:
        constructor=Source(case,meta,codec);facts=constructor.construct()
        aliases,coverage=mapper.typed_values(facts['source_wire_tree'],typed)
        rows.append(dict(case=case['name'],capture_SHA256=case['capture_SHA256'],source_facts_SHA256=case['source_facts_SHA256'],
                         original_frame=case['original_frame'],original_frame_SHA256=case['original_frame_SHA256'],
                         direct_codec_invocation_required=bool(case.get('requires_direct_codec_invocation')),
                         source_fields=facts,expected_private_typed_aliases=aliases,typed_source_coverage=coverage,
                         partial_fields_source='Session and private field_values;GetFields remains null on error with fields_error equal Error',
                         identity_requirements=dict(flow=0,transaction=0,response_to=0,TCPFlowID=0,no_LSDB_or_neighbor_or_SA_or_parent_PDU_inheritance=True),
                         progressive_display=dict(protocol='OSPFv3',stack=['Ethernet','IPv6','OSPFv3'],physical_layers_preserved=True,no_duplicate_OSPF_layer=True)))
    a.out.mkdir();data=a.out/'source-leaf-and-range-grid.json';write(data,rows)
    plan=dict(scope=__doc__,source_plan_SHA256=sha(a.source/'plan.json'),metadata_contract_SHA256=META_SHA,typed_contract_SHA256=TYPED_SHA,
        source_constructor_SHA256=sha(__file__),source_alias_mapper_SHA256=MAPPER_SHA,independent_codec_SHA256=CODEC_SHA,
        cases=20,ordinary_capture_cases=17,direct_codec_context_or_budget_cases=3,
        source_scalar_leaves=sum(len(r['source_fields']['source_leaves'])for r in rows),
        typed_occurrences=sum(sum(len(v)for v in r['expected_private_typed_aliases'].values())for r in rows),
        data_file=str(data),data_SHA256=sha(data),candidate_output_read=False,complete_expected_native_or_CLI_objects=False,
        important_policy=['Never substitute fields for Session in partial/error cases','No nil LSA slots;complete captured prefix scalars remain visible without Length or body claims',
                          'Hard malformed Length/Fletcher stops at declared boundary;unknown opaque only is soft','No invented LSDB/connection/authentication/parent inheritance',
                          'Current grid also covers new prefix12/prefix18 controls,not present in prior15 old685 baseline'])
    write(a.out/'plan.json',plan);print(json.dumps(dict(plan=str(a.out/'plan.json'),SHA256=sha(a.out/'plan.json'),cases=20,source_leaves=plan['source_scalar_leaves'],typed_occurrences=plan['typed_occurrences'])),flush=True)


if __name__=='__main__':main()
