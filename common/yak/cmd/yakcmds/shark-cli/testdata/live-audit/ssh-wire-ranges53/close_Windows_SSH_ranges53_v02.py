"""Account for a literal field click already switching to the PDU before b cycles."""
from pathlib import Path
base=Path(__file__).with_name('close_Windows_SSH_ranges53.py')
source=base.read_text('utf8')
old="assert module.pane(shots['02-byte-sources'])[0]==physical\n assert module.pane(shots['02-first-byte-source'])[0]==raw"
new="""if case['expected_PDU_span']:
  assert module.pane(shots['02-first-byte-source'])[0]==physical
  assert module.pane(shots['02-byte-sources'])[0]==raw
 else:
  assert module.pane(shots['02-first-byte-source'])[0]==raw
  assert module.pane(shots['02-byte-sources'])[0]==physical"""
assert source.count(old)==1;source=source.replace(old,new)
old="out=P/'Windows-SSH-ranges-root-closure53-01'"
assert source.count(old)==1;source=source.replace(old,"out=P/'Windows-SSH-ranges-root-closure53-02'")
old="(out/'summary.json').write_text"
assert source.count(old)==1
source=source.replace(old,"result['checker_history']={'first_version':str(base),'first_SHA256':sha(base),'first_failed':'Assumed b cycle begins from captured frame, although selecting a literal already chose the logical PDU. Actual source bytes and all highlight spans are unchanged.','source_facts_unchanged':True}\n"+old)
exec(compile(source,str(Path(__file__)),'exec'),globals())
