"""Check fixed packet-row identity while deep fields scroll their own pane."""
from pathlib import Path
base=Path(__file__).with_name('close_Windows_SSH_ranges53.py')
second=Path(__file__).with_name('close_Windows_SSH_ranges53_v02.py')
source=base.read_text('utf8')
old="assert module.pane(shots['02-byte-sources'])[0]==physical\n assert module.pane(shots['02-first-byte-source'])[0]==raw"
new="""if case['expected_PDU_span']:
  assert module.pane(shots['02-first-byte-source'])[0]==physical
  assert module.pane(shots['02-byte-sources'])[0]==raw
 else:
  assert module.pane(shots['02-first-byte-source'])[0]==raw
  assert module.pane(shots['02-byte-sources'])[0]==physical"""
assert source.count(old)==1;source=source.replace(old,new)
marker="for name in('01-protocol-fields','01-selected-field','02-first-byte-source','02-byte-sources'):"
assert source.count(marker)==1
prefix="""initial_text='\\n'.join(shots['01-protocol-fields']['Rows'])
 assert f"Frame {case['frame']}: {len(physical)} bytes on wire, {len(physical)} captured"in initial_text
 packet_rows=[row for row in shots['01-protocol-fields']['Rows']if re.search('>'+str(case['frame'])+r'\\s+\\d\\d:\\d\\d:\\d\\d\\.',row)]
 assert len(packet_rows)==1
 """
source=source.replace(marker,prefix+marker)
old='  assert f"Frame {case[\'frame\']}: {len(physical)} bytes on wire, {len(physical)} captured"in text'
assert source.count(old)==1;source=source.replace(old,"  assert packet_rows[0]in shots[name]['Rows']")
old="out=P/'Windows-SSH-ranges-root-closure53-01'"
assert source.count(old)==1;source=source.replace(old,"out=P/'Windows-SSH-ranges-root-closure53-03'")
old="(out/'summary.json').write_text"
assert source.count(old)==1
history="result['checker_history']={'first_source':str(base),'first_SHA256':sha(base),'second_source':str(second),'second_SHA256':sha(second),'first_failed':'b cycle starts from the already selected literal PDU, not always captured frame','second_failed':'deep field navigation scrolls the left Frame info out of view; fixed packet row and both entire byte sources remain exact','source_facts_unchanged':True}\n"
source=source.replace(old,history+old)
exec(compile(source,str(Path(__file__)),'exec'),globals())
