"""Reuse the closed role50 console controller with three immutable preview51 cases."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
base=root/'run_Windows_SSH_role50.py'
producer=root/'windows-SSH-role-26f50-01/wrong-client-reply-producer.json'
p=json.loads(producer.read_text('utf8'))
assert p['exit_code']==0 and p['inputs_unchanged']
assert hashlib.sha256(base.read_bytes()).hexdigest()==p['inputs'][str(base)]
newplan=root/'Windows-SSH-preview-source-plan51-01/plan.json'
assert hashlib.sha256(newplan.read_bytes()).hexdigest()=='d595f04fc3afebe6381ed7f3bb847062e41c677adbf2c9bf580edc25e7f05a54'
source=base.read_text('utf8')
for before,after in[
 ('Windows-SSH-role-source-plan50-01/plan.json','Windows-SSH-preview-source-plan51-01/plan.json'),
 ('37b2eea6c83bc639ecc4f222aad82f061458e4c95b6aae1b8921a3fa95816cf7','d595f04fc3afebe6381ed7f3bb847062e41c677adbf2c9bf580edc25e7f05a54'),
 ("if c['preview_only']","if c['opaque']")]:
 assert source.count(before)==1,before
 source=source.replace(before,after)
exec(compile(source,str(Path(__file__)),'exec'),globals())
