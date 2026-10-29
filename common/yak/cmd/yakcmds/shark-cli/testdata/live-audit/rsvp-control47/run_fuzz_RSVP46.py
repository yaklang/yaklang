"""One bounded RSVP literal/range fuzz target with the clean-source guard."""
from pathlib import Path
source = Path(__file__).with_name('run_fuzz_membership.py').read_text('utf8')
old = "for pkg,name in [('./common/bin-parser/parser/stream_parser','FuzzMulticastMembershipWireBounds')]:"
assert source.count(old) == 1
source = source.replace(old, "for pkg,name in [('./common/bin-parser/parser/stream_parser','FuzzRSVPBoundedOccurrences')]:")
assert source.count("'40s'") == 1
source = source.replace("'40s'", "'60s'")
exec(compile(source,str(Path(__file__)),'exec'),globals())
