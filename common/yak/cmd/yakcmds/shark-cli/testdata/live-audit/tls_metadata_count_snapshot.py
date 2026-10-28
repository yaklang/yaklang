"""Count physical labels and native messages from source-certified TLS exports.

Reference labels are explicitly reused only for the same immutable capture.
Metadata counts and label equality do not establish protocol conformance.
Nonphysical end publications are counted separately, never as captured frames.
"""
import argparse
import collections
import copy
import json
from pathlib import Path

from tls_public_extension_preservation_audit import rows, sha


def snapshot(prior_path, proof_path, json_out, markdown_out, endpoint_proof_path=None):
    prior=json.loads(prior_path.read_text('utf-8-sig'))
    proof=json.loads(proof_path.read_text('utf-8-sig'))
    assert proof['cases']==37 and proof['generated_ID_translation']is False
    assert proof['all_old_scalar_types_values_arrays_and_alias_order_preserved']is True
    endpoint=None
    cli_source=proof['source']
    if endpoint_proof_path is not None:
        endpoint=json.loads(endpoint_proof_path.read_text('utf8'))
        assert endpoint['native_source']==proof['source'] and endpoint['cases']==37
        assert endpoint['metadata_bridge_SHA256']==sha(proof_path)
        assert endpoint['all_rows_equal_complete_literal_expected_projection']is True
        assert endpoint['all_other_values_types_IDs_domains_and_PDUs_literal']is True
        cli_source=endpoint['CLI_source']
    result=dict(scope=__doc__,source=proof['source'],generator_sha256=sha(Path(__file__)),
                CLI_source=cli_source,prior_count_snapshot_sha256=sha(prior_path),extension_preservation_proof_sha256=sha(proof_path),inputs={})
    if endpoint is not None:
        result['endpoint_projection_proof_sha256']=sha(endpoint_proof_path)
    for name in ['weber','sharon']:
        old=prior['inputs'][name]
        matches=[r for r in proof['results'] if r['capture_SHA256']==old['capture_sha256']]
        assert len(matches)==1
        current=matches[0]
        sdk_producer,cli_producer=current['SDK_producer'],current['CLI_producer']
        sdk=Path(current['SDK_export_path'])
        cli=Path(current['CLI_export_path'])
        final_cli_hash=current['CLI_SHA256']
        if endpoint is not None:
            ep=[r for r in endpoint['results']if r['capture_SHA256']==old['capture_sha256']]
            assert len(ep)==1
            ep=ep[0]
            assert ep['before_CLI_SHA256']==current['CLI_SHA256']and ep['SDK_SHA256']==current['SDK_SHA256']
            assert ep['CLI_producer']['source']==cli_source
            cli=Path(ep['after_CLI']);final_cli_hash=ep['CLI_SHA256']
        assert sha(Path(sdk_producer['capture']))==old['capture_sha256']
        assert sha(sdk)==sdk_producer['export_SHA256']==current['SDK_SHA256']
        assert sha(cli)==final_cli_hash
        if endpoint is None:
            assert final_cli_hash==cli_producer['export_SHA256']
        messages=collections.Counter();labels=collections.Counter();contexts=collections.Counter();handshakes=collections.Counter()
        for event in rows(sdk):
            messages[event['protocol']]+=1
            session=event.get('session')or{}
            if session.get('Session Establishment'):
                contexts[event['protocol']]+=1
            for h in session.get('Handshake Messages')or session.get('TLS Handshakes')or[]:
                if any(k in h for k in ['Session ID','PSK Offer','PSK Selection','New Session Ticket']):
                    handshakes[str(h['Type'])]+=1
        publications=0
        for row in rows(cli):
            if 'number'in row:
                assert type(row['number'])is int
                labels[row['protocol']]+=1
            else:
                assert row['record_type']=='protocol_publication'
                publications+=1
        assert sum(labels.values())==old['physical_frames']==current['CLI_physical_packets']
        assert sum(messages.values())==old['native_message_records']==current['SDK_events']
        assert dict(messages)==old['native_message_protocols']
        assert dict(labels)==old['current_labels'],'review changed physical protocol labels before publishing'
        value=copy.deepcopy(old)
        value.update(SDK_export_sha256=current['SDK_SHA256'],CLI_export_sha256=final_cli_hash,
                     SDK_invariants_sha256=sha(proof_path),CLI_invariants_sha256=sha(endpoint_proof_path)if endpoint is not None else sha(proof_path),
                     reference_counts_reused_for_same_immutable_capture=True,exact_capture_digest_rechecked=True,
                     SDK_source=proof['source'],CLI_source=cli_source,CLI_nonphysical_publications=publications,
                     metadata_context_records=dict(contexts),metadata_handshake_messages=dict(handshakes))
        if endpoint is not None:
            value['CLI_prior_metadata_invariants_sha256']=sha(proof_path)
        result['inputs'][name]=value
    json_out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
    lines=[f"当前原生源码 `{proof['source']}`；Windows CLI 源码 `{cli_source}`。原包不入库；TLS 会话证据、VXLAN 准入与实际页面见 [本轮摘要](TLS-metadata-VXLAN-summary.json)。",'',
           '逐字保留 Wireshark 与 Shark 的显示标签；物理帧、结束发布和原生消息分开计数。同名差值不是解析错误数。',
           'SDK 新增 TLS 观察并修正 1 条 VXLAN 来源；CLI 另修正 4 条源端口误域，以及父头拒绝时的 IP 地址展示。其余字段、数组、类型、连接号、PDU 和来源引用按完整预期逐值保留。源验证和缺口单列。',
           '参考标签计数明确复用相同不可变捕获；新增 metadata 的源验证与未覆盖范围单列，完整 TOP30/所有高级功能尚未达到 Wireshark 全量等价。','']
    for name,value in result['inputs'].items():
        lines += [f'## {name}','',f"物理帧 {value['physical_frames']:,}；SDK 记录 {value['native_message_records']:,}；非物理结束发布 {value['CLI_nonphysical_publications']:,}。",'',
                  '| 排名 | WS 原始标签 | WS 帧数 | Shark 完全同名帧数 | 同名差值 |','|---:|---|---:|---:|---:|']
        for i,row in enumerate(value['raw_label_differences'][:30],1):
            label=row['label'].replace('|','\\|').replace('\n',' ')
            lines.append(f"| {i} | {label} | {row['reference_frames']:,} | {row['same_label_native_frames']:,} | {row['same_label_delta']:+,} |")
        lines.append('')
    markdown_out.write_text('\n'.join(lines),encoding='utf8')
    return result


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for key in ['prior','proof','json-out','markdown-out']:
        ap.add_argument('--'+key,type=Path,required=True)
    ap.add_argument('--endpoint-proof',type=Path)
    a=ap.parse_args()
    result=snapshot(a.prior,a.proof,a.json_out,a.markdown_out,a.endpoint_proof)
    print(json.dumps({'source':result['source'],'counts_SHA256':sha(a.json_out),
        'physical_frames':{k:v['physical_frames']for k,v in result['inputs'].items()},
        'SDK_records':{k:v['native_message_records']for k,v in result['inputs'].items()}}),flush=True)
