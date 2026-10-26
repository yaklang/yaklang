"""Refresh raw display-label counts from complete, constrained export evidence.
Counts are physical frames or logical native records as explicitly named.
Equal labels are not proof of correct parsing, and unequal labels are not proof
of a missing protocol. No alias normalization or decrypted-view substitution.
"""
import argparse, hashlib, json
from pathlib import Path

ap = argparse.ArgumentParser(description=__doc__)
for name in ['evidence-root', 'reference-root', 'json-out', 'markdown-out']:
    ap.add_argument('--' + name, type=Path, required=True)
ap.add_argument('--source', required=True)
a = ap.parse_args()

def load(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

out = {'scope': __doc__, 'source': a.source, 'generator_sha256': sha(Path(__file__)), 'inputs': {}}
for name in ['weber', 'sharon']:
    root = a.evidence_root / name
    reference_path = a.reference_root / (name + '-summary.json')
    reference = load(reference_path)
    sdk, cli = (load(root / (kind + '-invariants.json')) for kind in ['SDK', 'CLI'])
    header = load(root / 'header-audit-final.json')
    assert header['capture_sha256'] == reference['manifest']['capture_sha256']
    assert not sdk['other_export_differences'] and not cli['other_export_differences']
    assert cli['records'] == reference['frames']
    ws, yak = reference['ws_labels'], cli['after_protocol_counts']
    assert sum(ws.values()) == sum(yak.values()) == cli['records']
    labels = sorted(set(ws) | set(yak), key=lambda key: (-ws.get(key, 0), key))
    out['inputs'][name] = {
        'capture_sha256': header['capture_sha256'], 'physical_frames': cli['records'],
        'native_message_records': sdk['records'], 'reference_labels': ws, 'current_labels': yak,
        'native_message_protocols': sdk['after_protocol_counts'],
        'raw_label_differences': [{'label': key, 'reference_frames': ws.get(key, 0),
                                  'same_label_native_frames': yak.get(key, 0),
                                  'same_label_delta': yak.get(key, 0) - ws.get(key, 0)} for key in labels],
        'reference_summary_sha256': sha(reference_path), 'SDK_export_sha256': sdk['after_sha256'],
        'CLI_export_sha256': cli['after_sha256'],
        'SDK_invariants_sha256': sha(root / 'SDK-invariants.json'),
        'CLI_invariants_sha256': sha(root / 'CLI-invariants.json'),
        'header_certificate_sha256': sha(root / 'header-audit-final.json'),
    }
a.json_out.parent.mkdir(parents=True, exist_ok=True)
a.json_out.write_text(json.dumps(out, indent=2) + '\n', encoding='utf8')
lines = [f'当前显示标签计数，源码 `{a.source}`。输入原包不入库。', '',
         '这里逐字保留 Wireshark 和 Shark 的列标签，帧数与原生消息数分开。'
         '同名计数不代表解析正确；大小写、子协议、分片及拒绝策略也会产生标签差异。'
         '字段、流、重组和来源的验证见主报告及各协议证书。', '']
for name, value in out['inputs'].items():
    lines += [f'## {name}', '', f"物理帧 {value['physical_frames']:,}；原生消息 {value['native_message_records']:,}。", '',
              '| 排名 | WS 原始标签 | WS 帧数 | Shark 完全同名帧数 | 同名差值 |', '|---:|---|---:|---:|---:|']
    for index, row in enumerate(value['raw_label_differences'][:30], 1):
        label = row['label'].replace('|', '\\|').replace('\n', ' ')
        lines.append(f"| {index} | {label} | {row['reference_frames']:,} | {row['same_label_native_frames']:,} | {row['same_label_delta']:+,} |")
    lines.append('')
a.markdown_out.parent.mkdir(parents=True, exist_ok=True)
a.markdown_out.write_text('\n'.join(lines), encoding='utf8')
print(json.dumps({'source': a.source, 'aggregate_sha256': sha(a.json_out),
                  'physical_frames': {name: value['physical_frames'] for name, value in out['inputs'].items()}}))
