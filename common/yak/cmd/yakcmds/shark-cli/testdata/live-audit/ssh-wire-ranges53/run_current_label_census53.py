"""Run the existing literal census with explicitly isolated reference environment."""
from pathlib import Path
import hashlib
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
base=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/membership-source/closed_protocol_label_audit.py'
assert hashlib.sha256(base.read_bytes()).hexdigest()=='ebdc7e8665deae7ab37af0fdd3521755fc7ad370389ad656ea9cc061ff85fd50'
source=base.read_text('utf8')
old='env = dict(os.environ, WIRESHARK_CONFIG_DIR=str(config))'
assert source.count(old)==1
source=source.replace(old,'''env = dict(os.environ, APPDATA=str(config), WIRESHARK_CONFIG_DIR=str(config))
    for ambient in ('SSLKEYLOGFILE', 'WIRESHARK_PLUGIN_DIR', 'WIRESHARK_EXTCAP_DIR'):
        env.pop(ambient, None)''')
exec(compile(source,str(Path(__file__)),'exec'),globals())
