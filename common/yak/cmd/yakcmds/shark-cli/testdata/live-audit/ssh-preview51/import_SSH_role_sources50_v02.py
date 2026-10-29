"""Resume only the hash-verified partial23-source import; shared deps need no manifest."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
base=root/'import_SSH_role_sources50.py'
source=base.read_text('utf8')
old='assert not dest.exists();dest.mkdir()'
assert source.count(old)==1
source=source.replace(old,"""if dest.exists():
 allowed={(dest/row['path']).resolve()for row in inv['files']}
 assert all(q.resolve()in allowed for q in dest.rglob('*')if q.is_file()),'unexpected partial archive contents'
else:dest.mkdir()""")
old="target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(raw);assert sha(target)==f['SHA256']"
assert source.count(old)==1
source=source.replace(old,"""target.parent.mkdir(parents=True,exist_ok=True)
 if target.exists():assert sha(target)==f['SHA256'],'partial source differs; never overwrite'
 else:target.write_bytes(raw)
 assert sha(target)==f['SHA256']""")
old=" assert sha(repo/d['manifest_path'])==d['manifest_SHA256']"
assert source.count(old)==1
source=source.replace(old," if 'manifest_path'in d:assert sha(repo/d['manifest_path'])==d['manifest_SHA256']")
source=source.replace("SSH-role-import50-01.json","SSH-role-import50-02.json")
failure=dict(scope='Preserve first importer schema failure, not a packet/codec failure.',
 failed_controller=str(base),controller_SHA256=hashlib.sha256(base.read_bytes()).hexdigest(),
 reason='A shared repository dependency has SHA256/path but no optional manifest_path.',
 exact_partial_sources_retained=23,no_files_removed_or_moved=True)
with(root/'SSH-role-import50-01.failure.json').open('x',encoding='utf8')as f:json.dump(failure,f,indent=2);f.write('\n')
exec(compile(source,str(Path(__file__)),'exec'),globals())
