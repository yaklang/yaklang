"""Stage the exact four-file snapshot that passed the expanded 1772 checks."""
from pathlib import Path
base=Path(__file__).with_name('stage_SSH_wire_ranges53.py')
source=base.read_text('utf8')
for old,new in [('test-SSH-wire-ranges53-05/receipt.json','test-SSH-dedup54-02/receipt.json'),("{'pass':548}","{'pass':1772}"),('len(paths)==7','len(paths)==4'),('SSH-wire-ranges-stage53-diff-check.log','SSH-dedup-stage54-diff-check.log'),('SSH-wire-ranges-stage53-01.json','SSH-dedup-stage54-01.json'),('test_pass=548','test_pass=1772')]:
 assert source.count(old)==1
 source=source.replace(old,new)
exec(compile(source,str(Path(__file__)),'exec'),globals())
