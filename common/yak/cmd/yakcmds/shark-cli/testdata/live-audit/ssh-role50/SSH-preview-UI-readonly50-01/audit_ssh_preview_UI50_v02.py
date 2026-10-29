from pathlib import Path
import hashlib,json,re,struct
P=Path(r'C:/tmp/yak-shark-ESP-SSH-UI-20261028')
OUT=P/'SSH-preview-UI-readonly50-01'
REPO=Path(r'C:/Users/V/.codex/worktrees/shark-protocol-infrastructure/yaklang')
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def obj(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def need(test,msg):
    if not test:raise AssertionError(msg)
def read_pcap_frame(p,number):
    w=p.read_bytes(); magic=w[:4]
    endian={b'\xd4\xc3\xb2\xa1':'<',b'\xa1\xb2\xc3\xd4':'>',b'\x4d\x3c\xb2\xa1':'<',b'\xa1\xb2\x3c\x4d':'>'}[magic]
    a=24
    for n in range(1,number+1):
        sec,frac,cap,orig=struct.unpack_from(endian+'IIII',w,a); a+=16
        raw=w[a:a+cap];need(len(raw)==cap,'short pcap record');a+=cap
    return raw,cap,orig

def hex_cells(path,expected,per_row=16):
    x=obj(path); by_offset={}
    for row in x['Rows']:
        if '││' not in row:continue
        right=row.split('││',1)[1]
        m=re.search(r'\b([0-9a-f]{6})  ',right)
        if not m:continue
        offset=int(m[1],16);a=m.end()
        for i in range(per_row):
            token=right[a+i*3+i//8:a+i*3+i//8+2]
            if re.fullmatch('[0-9a-f]{2}',token):
                pos=offset+i;need(pos not in by_offset,'repeated byte position');by_offset[pos]=int(token,16)
    need(sorted(by_offset)==list(range(len(expected))),'whole visible source length/coverage')
    actual=bytes(by_offset[i] for i in range(len(expected)))
    need(actual==expected,'whole visible source bytes')
    return {'path':str(path),'cells_SHA256':sha(path),'visible_bytes':len(actual),'visible_bytes_SHA256':hashlib.sha256(actual).hexdigest()}

base=P/'windows-SSH-role-26f50-01';snap=base/'protected-preview'
producer=base/'protected-preview-producer.json';pr=obj(producer)
need(pr['source']=='26f01f4baa5fd39b88deadf901257a70bc5c1dfe','source');need(pr['exit_code']==0 and pr['inputs_unchanged'],'closed producer')
for p,h in pr['inputs'].items():need(sha(p)==h,'producer input hash '+p)
for name,h in pr['snapshots'].items():
    need(sha(snap/(name+'.json'))==h['cells_SHA256'],'cells receipt '+name)
    need(sha(snap/(name+'.png'))==h['PNG_SHA256'],'PNG receipt '+name)
cap=P/'SSH-role-source50-01/plan02/ssh-role-source50-authored9.pcap'
raw_path=P/'SSH-role-source50-01/plan02/raw/01-curve-correct-server-first-reverse-address-frame013.bin'
frame,caplen,origlen=read_pcap_frame(cap,13);wire=raw_path.read_bytes()
need(caplen==origlen==246 and len(wire)==192,'independent frame lengths')
need(frame[54:]==wire,'physical IP/TCP payload source')
need(sha(cap)=='22f3a36ed8dce4fa4cead19aeba75a9c4632df0a5f9eaf433c62d22e2adf906a','frozen capture')
need(sha(raw_path)=='2d60b3f8fa1956095a9591714467da0abb8a1bf4e7f213620c91f74882066709','frozen opaque chunk')
policy=obj(P/'SSH-role-API-CLI-source50-02/plan02/opaque-preview-policy.json')
checks=[]
for path in sorted(snap.glob('*.json')):
    if path.name=='restored.json':continue
    x=obj(path)
    for line in x.get('Rows',[]):
        if 'Frame 13:' in line:
            need('Frame 13: 246 bytes on wire, 246 captured' in line,'frame physical scope '+path.name)
            checks.append(path.name)
physical=hex_cells(snap/'02-byte-sources.json',frame)
preview=hex_cells(snap/'02-first-byte-source.json',wire[:64])
need(any('Captured frame' in s for s in obj(snap/'02-byte-sources.json')['Rows']),'physical byte label')
need(any('SSH · reassembled PDU' in s for s in obj(snap/'02-first-byte-source.json')['Rows']),'actual misleading preview title retained as failure evidence')
files=['common/yak/cmd/yakcmds/shark-cli/protocol_events.go','common/yak/cmd/yakcmds/shark-cli/tui.go','common/yak/cmd/yakcmds/shark-cli/field_bytes.go','common/yak/cmd/yakcmds/shark-cli/decode.go','common/yak/cmd/yakcmds/shark-cli/decode_worker.go','common/yak/cmd/yakcmds/shark-cli/conversation.go','common/pcapx/pcaputil/bin_parser.go']
summary={
 'scope':'offline source/cells/receipt audit; no Go, SDK, TShark or UI process started',
 'source':'26f01f4baa5fd39b88deadf901257a70bc5c1dfe','producer':{'path':str(producer),'SHA256':sha(producer)},
 'inputs':[{'path':str(cap),'SHA256':sha(cap)},{'path':str(raw_path),'SHA256':sha(raw_path)},{'path':str(P/'SSH-role-API-CLI-source50-02/plan02/opaque-preview-policy.json'),'SHA256':sha(P/'SSH-role-API-CLI-source50-02/plan02/opaque-preview-policy.json')}],
 'frame':{'number':13,'physical_original_bytes':origlen,'physical_captured_bytes':caplen,'IP_TCP_payload_offset':54,'observed_opaque_stream_chunk_bytes':len(wire),'retained_preview_bytes':64,'not_retained_by_SDK_bytes':128,'not_retained_tail_SHA256':hashlib.sha256(wire[64:]).hexdigest(),'not_a_proven_encrypted_SSH_packet_boundary':True},
 'whole_cells_byte_checks':[physical,preview],
 'frame_length_lines_verified':len(checks),'frame_length_line_files':checks,
 'finding':{'reported_64_on_wire_64_captured':'not reproduced in any saved frame13 cells or exact 02-byte-sources.png; all physical frame lines are246/246','actual_confirmed_defect':'opaque64-byte preview is named SSH reassembled PDU; even a short fully retained encrypted stream chunk has no SSH packet-boundary proof','b_key_scope':'tui.go523..526 only changes bytesSource and selection offsets; no synthetic capturedPacket','minimal_fix_sites':['protocol_events.go766 event heading: protectedSSHEvent must say opaque/encrypted stream chunk, not PDU','protocol_events.go790..796 byte source: protectedSSHEvent use opaque stream bytes; when Length>len(Raw) expose retained/observed count; preserve source bytes','decode.go440 / field_bytes.go186..204 / tui.go523..526 physical-frame data should remain unchanged'],
 'suggested_negative_controls':['physical frame 246/246 and complete physical bytes remain identical after b-cycle','retained preview64 exactly equals source opaque raw[0:64], never physical frame[0:64]','retained48 of observed48 after NEWKEYS still must not be called a proven PDU','retained64 of observed192 must show explicit preview counts and no complete-PDU wording','known clear SSH packet remains PDU and nested field-source offsets unchanged','other malformed/role refusals must not be reclassified encrypted by source length alone']},
 'source_files':[{'path':str(REPO/f),'SHA256':sha(REPO/f)} for f in files],
 'screenshots_inspected':[{'path':str(snap/'02-byte-sources.png'),'SHA256':sha(snap/'02-byte-sources.png')}],
 'historical_evidence_preserved':True,'script_SHA256':sha(__file__)
}
(OUT/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'summary':str(OUT/'summary.json'),'SHA256':sha(OUT/'summary.json'),'physical_bytes':len(frame),'retained_preview_bytes':64,'physical_frame_lines':len(checks),'actual_confirmed_defects':1},ensure_ascii=False))