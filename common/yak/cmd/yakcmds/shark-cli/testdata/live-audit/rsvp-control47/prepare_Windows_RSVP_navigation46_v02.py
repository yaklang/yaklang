"""Move the Resv cursor up sixteen real rows; keep original source values and ranges unchanged."""
from pathlib import Path
source=Path(__file__).with_name('prepare_Windows_RSVP_navigation46.py').read_text('utf8')
for old,new in [("out=root/'Windows-RSVP-source-plan46-02'","out=root/'Windows-RSVP-source-plan46-03'"),("(64,96)","(64,80)")]:
 assert source.count(old)==1
 source=source.replace(old,new)
exec(compile(source,str(Path(__file__)),'exec'),globals())
