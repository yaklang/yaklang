"""Build complete OSPFv2 message/LSA/authentication/LLS source grids from old census.

Only original captured bytes are decoded. No candidate/native output is read;
this source grid is not a complete expected native event or a router state DB.
"""
import argparse,collections,hashlib,importlib.util,json,shutil,traceback
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('census','codec','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir();assert shutil.disk_usage('C:/').free>20*2**30
    assert sha(a.census/'summary.json')=='ae983a9c7c699b7b629f8e6208d8226a5b59394ae3d3eb3185e6fa6dea3047bb';inventory=read(a.census/'summary.json')
    spec=importlib.util.spec_from_file_location('independent_OSPFv2_codec',a.codec);codec=importlib.util.module_from_spec(spec);spec.loader.exec_module(codec)
    binding={str(p):sha(p)for p in[Path(__file__),a.codec,a.census/'summary.json']};summary=[];failure=[];allrows=[]
    for entry in inventory['captures']:
        if not entry['declared_IPv4_OSPF']:continue
        folder=a.census/('input-%02d'%entry['input']);factspath=folder/'source-facts.json';assert sha(factspath)==entry['facts_SHA256'];facts=read(factspath);binding[str(factspath)]=sha(factspath)
        counts=collections.Counter();msg=collections.Counter();auth=collections.Counter();lsatypes=collections.Counter();headers=collections.Counter();links=collections.Counter();digestlens=collections.Counter();llssizes=collections.Counter();llsauth=collections.Counter();tlvs=collections.Counter();ordering=[];rows=[]
        for original in facts:
            frame=Path(original['original_frame']);assert sha(frame)==original['original_frame_SHA256'];binding[str(frame)]=sha(frame);raw=frame.read_bytes();start,end=original['carrier']['payload_range'];w=raw[start:end]
            try:
                decoded=codec.decode(w,start);tree=decoded['fields'];h=tree['Header'];kind=h['Type'];au=h['Authentication Type'];msg[str(kind)]+=1;auth[str(au)]+=1;counts['source_packets']+=1;counts['source_scalar_or_opaque_leaves']+=len(decoded['source_leaves'])
                if not original['carrier']['parent_complete']:counts['incomplete_enclosing_IPv4']+=1
                if au in(0,1):counts['base_packet_checksum_valid'if tree['Packet Checksum Verification']['verified']else'base_packet_checksum_bad']+=1
                elif au==2:
                    counts['base_packet_checksum_omitted']+=1;counts['crypto_header_checksum_nonzero']+=h['Checksum']!=0;digestlens[str(tree['Authentication']['Authentication Data Length'])]+=1
                full=tree.get('LSAs',[])
                for row in full:
                    lsatypes[str(row['LS Type'])]+=1;counts['full_LSA_Fletcher_valid'if row['Checksum Verification']['valid']else'full_LSA_Fletcher_bad']+=1
                    if row['LS Type']==1:
                        for link in row['Body']['Links']:links[str(link['Link Type'])]+=1;counts['Router_optional_TOS_metrics']+=len(link['TOS Metrics'])
                for row in tree.get('LSA Headers',tree.get('Database Description',{}).get('LSA Headers',[])):headers[str(row['LS Type'])]+=1
                if'LLS'in tree:
                    lls=tree['LLS'];llsauth[str(au)]+=1;llssizes[str(lls['Length Words'])]+=1;counts['LLS_blocks']+=1
                    counts['LLS_violations']+=len(lls['Violations'])
                    if au==2:counts['LLS_checksum_omitted']+=1
                    else:counts['LLS_checksum_valid'if lls['Checksum Verification']['verified']else'LLS_checksum_bad']+=1
                    for row in lls['TLVs']:
                        tlvs[str(row['Type'])]+=1
                        if row['Type']==2:counts['LLS_CA_sequence_match']+=bool(row['Sequence Matches OSPF Header']);counts['LLS_CA_sequence_mismatch']+=not row['Sequence Matches OSPF Header']
                    if au==2:
                        ordering.append(dict(frame=original['frame'],OSPFn=[0,h['Packet Length']],authentication_digest=tree['Authentication']['Wire Field Ranges']['Authentication Digest'],LLS=lls['Observed Extent'],LLS_CA_sequence_matches=all(row['Sequence Matches OSPF Header']for row in lls['TLVs']if row['Type']==2),digest_and_LLS_never_part_of_base_body=True))
                if'Unexplained Tail'in tree:counts['unexplained_tail_bytes']+=tree['Unexplained Tail']['bytes']
                row=dict(input=entry['input'],capture=entry['capture'],capture_SHA256=entry['capture_SHA256'],frame=original['frame'],domain=original['domain'],time_ns=original['time_ns'],original_frame=str(frame),original_frame_SHA256=original['original_frame_SHA256'],physical_captured_length=original['captured_length'],physical_original_length=original['original_length'],carrier=original['carrier'],PDU_range=[start,end],PDU_SHA256=hashlib.sha256(w).hexdigest(),source=decoded)
                rows.append(row);allrows.append(row)
            except Exception as error:failure.append(dict(input=entry['input'],frame=original['frame'],original_frame_SHA256=original['original_frame_SHA256'],error=repr(error),traceback=traceback.format_exc()))
        out=a.out/('input-%02d-source-grid.json'%entry['input']);write(out,rows)
        summary.append(dict(input=entry['input'],capture=entry['capture'],capture_SHA256=entry['capture_SHA256'],source_grid=str(out),source_grid_SHA256=sha(out),source_counts=dict(counts),message_types=dict(msg),authentication_types=dict(auth),full_LSA_types=dict(lsatypes),advertised_header_types=dict(headers),Router_link_types=dict(links),cryptographic_digest_lengths=dict(digestlens),LLS_word_lengths=dict(llssizes),LLS_auth_types=dict(llsauth),LLS_TLV_types=dict(tlvs),source_crypto_digest_before_LLS_ordering=ordering))
    assert binding=={p:sha(p)for p in binding}
    report=dict(scope=__doc__,census_summary_SHA256=sha(a.census/'summary.json'),source_codec_SHA256=sha(a.codec),source_constructor_SHA256=sha(__file__),source_selected_rows=556,source_successful_rows=len(allrows),source_scalar_or_opaque_leaves=sum(len(r['source']['source_leaves'])for r in allrows),source_failures=len(failure),failures=failure,captures=summary,inputs=binding,inputs_unchanged=True,no_candidate_or_native_output_read=True,authentication_SA_replay_LSDB_neighbor_or_installed_route_verified=False,real_LSA3_LSA4_LSA7_and_AuType1_not_present=True,IPv4_OSPF_fragment_not_in_complete556_is_separate_scope=True)
    write(a.out/'summary.json',report);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),source_rows=len(allrows),leaves=report['source_scalar_or_opaque_leaves'],failures=len(failure),counts=[r['source_counts']for r in summary])),flush=True)
    if failure:raise SystemExit(1)
if __name__=='__main__':main()
