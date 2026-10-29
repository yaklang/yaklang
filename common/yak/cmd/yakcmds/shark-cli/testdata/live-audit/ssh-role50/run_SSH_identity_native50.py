"""Recheck frozen identity49 controls with the role50 binary and strict checker03."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
archive=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/ssh-identity49/source-manifest.json'
assert hashlib.sha256(archive.read_bytes()).hexdigest()=='b7606b655feb69e3a4bb52e826674f7f30c5b52cdc5f2c8eec7748b1fabaf188'
base=root/'run_SSH_native49.py'
assert hashlib.sha256(base.read_bytes()).hexdigest()==next(x['SHA256']for x in json.loads(archive.read_text('utf8'))['files']if x['path']==base.name)
extra_checker=root/'SSH-SDK-CLI-source49-03/ssh_native_cli49_v03.py'
extra_contract=extra_checker.parent/'contract.json'
extra_source02=root/'SSH-SDK-CLI-source49-02/plan02/manifest.json'
assert hashlib.sha256(extra_contract.read_bytes()).hexdigest()=='5de2a91f843b8c3871df54f4dfce1a50c9ff7a0157e84334f5ae7549670dc0c7'
source=base.read_text('utf8')
old="str(checker),'verify','--plan',str(plan),'--events'"
assert source.count(old)==1
source=source.replace(old,"str(extra_checker),'verify','--events'")
old="plan/'runtime-command-plan.json',capture)"
assert source.count(old)==1
source=source.replace(old,"plan/'runtime-command-plan.json',capture,extra_checker,extra_contract,extra_source02)")
exec(compile(source,str(Path(__file__)),'exec'),globals())
