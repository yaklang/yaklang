"""Verify selected ESP detail and bytes while preserving truthful global partial evidence.

Version02 corrects only the old verifier's global detail-unavailable ban. The
old failed proof is preserved; no wire, SPI, field value or range expectation
is obtained from the actual console output.
"""
from pathlib import Path
import argparse,base64,hashlib,json,re,traceback
ROOT=Path(__file__).resolve().parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
def pane(state):
    data=bytearray();cells=[]
    for y,row in enumerate(state['Rows']):
        if '││' not in row:continue
        begin=row.index('││')+2
        match=re.match(r'\s*([0-9a-f]{6})  ',row[begin:])
        if not match:continue
        assert int(match[1],16)==len(data),('hex row offset',match[1],len(data))
        x=begin+match.start(1)
        for i in range(16):
            column=x+8+i*3+i//8;word=row[column:column+2]
            if not re.fullmatch('[0-9a-f]{2}',word):break
            data.append(int(word,16));cells.append((y,column))
    return bytes(data),cells
def verify(folder,case):
    required={'00-capture-ready','00-filter-ready','01-protocol-fields','01-adaptive-fields-before',
        '01-adaptive-bytes-click','01-adaptive-fields-click','01-selected-field','02-first-byte-source','02-byte-sources',
        '03-conversation-messages','04-association-evidence','05-narrow','06-restored'}
    snapshots={}
    for p in sorted(folder.glob('*.json')):
        value=read(p)
        if 'Rows' not in value:continue
        assert len(value['Rows'])==len(value['Attributes'])==value['Height']
        assert all(len(r)==len(a)==value['Width'] for r,a in zip(value['Rows'],value['Attributes']))
        snapshots[p.stem]=value
    assert required<=snapshots.keys(),('missing canonical snapshots',required-snapshots.keys())
    extra=snapshots.keys()-required
    assert all(re.fullmatch(r'(00-capture-ready|00-filter-ready|01-protocol-fields)-wait-[0-9]{3}',n) for n in extra)
    for n in ['00-capture-ready','00-filter-ready','01-protocol-fields']:assert n+'-wait-000' in snapshots
    first=snapshots['01-protocol-fields'];text='\n'.join(first['Rows'])
    assert ' EOF ' in first['Rows'][0]
    assert case['query'] in first['Rows'][1] and '▏' not in first['Rows'][1]
    assert re.search(r'buffer\s+20000\s*/\s*20000',text)
    assert re.search(r'· 1 visible(?:\s|╮)',text)
    assert any(re.search(r'›46978\s+\S+\s+ESP(?:\s|$)',r) for r in first['Rows'])
    assert '2 ESP FIELDS' in text and '#46978' in text
    assert 'ESP · PDU #37903' in text
    assert 'Ethernet › IPv4 › UDP › ESP' in text
    # Global history coverage is separate from the selected retained PDU. The
    # real footer correctly reports other missing packet details, and must
    # remain visible. Restrict missing-detail assertions to the selected pane.
    header=next(i for i,r in enumerate(first['Rows']) if '2 ESP FIELDS' in r)
    right=next((r.index('││')+1 for r in first['Rows'][header+1:first['Height']-3] if '││' in r),first['Width']//2)
    field_text='\n'.join(r[:right] for r in first['Rows'][header:first['Height']-3])
    for line in ['SPI: 4102537167','Sequence: 2','Authentication Verified: false','Algorithm Context Available: false']:
        assert line in field_text,('missing selected retained actual field',line)
    assert 'Protocol detail absent or evicted' not in field_text and 'detail unavailable' not in field_text
    footer='\n'.join(first['Rows'][first['Height']-3:])
    assert 'Field filter partial: incomplete field evidence' in footer
    assert 'protocol detail unavailable in retained history' in footer
    assert 'PDU #0' not in text
    raw=Path(case['physical_frame_file']).read_bytes();wire=base64.b64decode(case['full_native_expectation']['raw'],validate=True)
    assert len(raw)==138 and len(wire)==96 and raw[42:138]==wire
    first_bytes,_=pane(first);assert first_bytes==raw,('first Source0 bytes',len(first_bytes))
    selected=snapshots['01-selected-field'];selected_text='\n'.join(selected['Rows'])
    assert 'SPI: 4102537167' in selected_text and '3 ESP · captured PDU' in selected_text
    assert '0x0–0x3 · 4 bytes' in selected_text
    chosen,positions=pane(selected);assert chosen==wire and len(positions)==96
    attrs=[selected['Attributes'][y][x] for y,x in positions]
    selected_style=attrs[0]
    assert len(set(attrs[:4]))==1 and all(v!=selected_style for v in attrs[4:]),'SPI highlights must cover exactly PDU0..4'
    for y,x in positions[:4]:assert selected['Attributes'][y][x+1]==selected_style
    cycled0,_=pane(snapshots['02-first-byte-source']);cycled1,_=pane(snapshots['02-byte-sources'])
    assert cycled0==raw and cycled1==wire,'After selecting Source1 SPI, b cycles to Source0 then back to Source1'
    restored=read(folder/'restored.json');assert restored=={'error':'<nil>','input_cp':936,'output_cp':936}
    return dict(scope=__doc__,fresh_snapshots=len(snapshots),canonical_snapshots=sorted(required),
        physical_frame=46978,PDU=37903,selected_field='SPI',default_packet_cache=20000,default_history_mib=64,
        actual_detail_present=True,physical_Source0_bytes=138,PDU_Source1_bytes=96,
        physical_Source0_SHA256=hashlib.sha256(raw).hexdigest(),PDU_Source1_SHA256=hashlib.sha256(wire).hexdigest(),
        SPI_PDU_range=[0,4],SPI_physical_range=[42,46],actual_SPI_highlight_exact=True,
        sequence_PDU_range=[4,8],sequence_physical_range=[46,50],sequence_display_value=2,
        sequence_actual_highlight_not_claimed=True,SA_or_authentication_not_claimed=True,
        selected_ESP_detail_available=True,truthful_global_history_partial_footer_preserved=True,
        restored=restored,cell_SHA256={p.name:sha(p) for p in folder.glob('*.json') if p.stem in snapshots})
def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--runtime',type=Path,required=True);p.add_argument('--out',type=Path,required=True)
    a=p.parse_args();a.out.mkdir(exist_ok=False)
    try:
        case=read(ROOT/'cases.json')['ESP-NATT-Weber46978'];value=verify(a.runtime/'ESP-NATT-Weber46978',case)
        value['auditor_SHA256']=sha(__file__);value['case_file_SHA256']=sha(ROOT/'cases.json')
        write(a.out/'summary.json',value)
        print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'))))
    except Exception:
        write(a.out/'failure.json',dict(failure=traceback.format_exc(),no_success_certificate=True));raise
if __name__=='__main__':main()
