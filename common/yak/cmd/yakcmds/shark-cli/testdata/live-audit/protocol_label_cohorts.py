"""Render literal TOP30 reference-label cohorts from a closed physical census.

One frame contributes exactly once. The Shark label distribution is preserved,
without aliases, inferred protocol families or a protocol-correctness score.
"""
import argparse
import collections
import csv
import hashlib
import json
from pathlib import Path


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def escape(label):
    return label.replace('|', '\\|').replace('\n', ' ')


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--census', type=Path, required=True)
    ap.add_argument('--out', type=Path, required=True)
    a = ap.parse_args()
    a.out.mkdir(exist_ok=False)
    census = json.loads(a.census.read_text('utf-8-sig'))
    sources = {v[k]['source'] for v in census['inputs'].values() for k in ('SDK', 'CLI')}
    assert len(sources) == 1
    source, = sources
    result = dict(scope=__doc__, source=source, census_SHA256=sha(a.census),
        auditor_SHA256=sha(__file__), alias_normalization=False, correctness_score=False,
        reference_profile='Fresh isolated TShark configuration, no explicit keylog; see original census arguments and fixed native/CLI providers.', inputs={})
    lines = [f'逐帧 TOP30 标签分布，实际生产者 `{source}`。', '',
        '每行以 Wireshark 的原始列标签分组，列出同一批原始帧在 Shark 中的真实标签分布。没有合并别名；名称、加密可见性、载体和拒绝策略的差异需要逐帧核实，不能直接算成误识别或已支持。', '',
        '参考采用新的隔离个人配置、无名称解析及显式 TCP 乱序重组，没有显式 keylog；Shark 保留原生产参数。字段、身份、流和会话的正确性不由这个计数表证明。原包、keys、完整逐帧矩阵均在仓库外。', '']
    for name, value in sorted(census['inputs'].items()):
        path = a.census.parent/name/'literal-label-pairs.csv'
        assert sha(path) == value['label_pairs_SHA256']
        pairs, refs, actual = {}, collections.Counter(), collections.Counter()
        with path.open(encoding='utf8', newline='') as f:
            reader = csv.DictReader(f)
            assert reader.fieldnames == ['Wireshark literal label', 'Shark literal label', 'physical frames']
            for row in reader:
                pair = row['Wireshark literal label'], row['Shark literal label']
                count = int(row['physical frames'])
                assert count > 0 and pair not in pairs
                pairs[pair] = count
                refs[pair[0]] += count
                actual[pair[1]] += count
        assert dict(refs) == value['reference_labels']
        assert dict(actual) == value['current_labels']
        assert sum(refs.values()) == sum(actual.values()) == value['physical_frames'] == value['source_frame_number_length_interface_clock_checks']
        top = []
        for rank, label in enumerate(sorted(refs, key=lambda k: (-refs[k], k))[:30], 1):
            distribution = sorted([(b, n) for (x, b), n in pairs.items() if x == label], key=lambda row: (-row[1], row[0]))
            assert sum(n for _, n in distribution) == refs[label]
            top.append(dict(rank=rank, reference_literal_label=label, physical_frames=refs[label],
                Shark_literal_distribution=[dict(label=b, physical_frames=n) for b, n in distribution]))
        result['inputs'][name] = dict(physical_frames=value['physical_frames'],
            native_message_records=value['native_message_records'],
            CLI_nonphysical_publications=value['CLI_nonphysical_publications'],
            capture_SHA256=value['capture_SHA256'], label_pairs_SHA256=sha(path),
            SDK=value['SDK'], CLI=value['CLI'], top30=top)
        lines += [f'## {name}', '',
            f"物理帧 {value['physical_frames']:,}；原生消息 {value['native_message_records']:,}；非物理结束发布 {value['CLI_nonphysical_publications']:,}。", '',
            '| 排名 | Wireshark 原始标签 | 帧数 | 同一批帧的 Shark 原始标签 |',
            '|---:|---|---:|---|']
        for row in top:
            text = '；'.join(escape(v['label'])+' '+format(v['physical_frames'], ',') for v in row['Shark_literal_distribution'])
            lines.append(f"| {row['rank']} | {escape(row['reference_literal_label'])} | {row['physical_frames']:,} | {text} |")
        lines.append('')
    target = a.out/'summary.json'
    target.write_text(json.dumps(result, indent=2, ensure_ascii=False)+'\n', encoding='utf8')
    (a.out/'TOP30.md').write_text('\n'.join(lines), encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), physical_frames=sum(v['physical_frames'] for v in result['inputs'].values()))))


if __name__ == '__main__':
    main()
