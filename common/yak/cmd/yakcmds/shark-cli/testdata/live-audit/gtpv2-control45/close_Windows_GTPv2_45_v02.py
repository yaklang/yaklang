"""A waitShot alias inherits the exact completed candidate's acknowledgement."""
from pathlib import Path
source=Path(__file__).with_name('close_Windows_GTPv2_45.py').read_text('utf8')
old="assert(folder/(name+'.json.ready')).is_file(),'snapshot completion acknowledgement missing'"
assert source.count(old)==1
new="assert(folder/(name+'.json.ready')).is_file() or any(sha(q)==sha(folder/(name+'.json')) and q.with_name(q.name+'.ready').is_file() for q in folder.glob(name+'-wait-*.json')),'neither direct nor exact waitShot candidate acknowledgement present'"
source=source.replace(old,new).replace('Windows-GTPv2-root-closure45-01','Windows-GTPv2-root-closure45-02')
exec(compile(source,str(Path(__file__)),'exec'),globals())
