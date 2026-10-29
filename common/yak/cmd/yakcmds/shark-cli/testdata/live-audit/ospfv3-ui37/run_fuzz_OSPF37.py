"""One bounded OSPFv3 wire/range fuzz target; unchanged audited runner policy."""
from pathlib import Path
source=Path(__file__).with_name('run_fuzz_membership.py').read_text('utf8')
before="for pkg,name in [('./common/bin-parser/parser/stream_parser','FuzzMulticastMembershipWireBounds')]:"
assert source.count(before)==1
source=source.replace(before,"for pkg,name in [('./common/bin-parser/parser/stream_parser','FuzzOSPFv3BoundedOccurrences')]:")
assert source.count("'40s'")==1
source=source.replace("'40s'","'60s'")
exec(compile(source,str(Path(__file__)),'exec'),globals())
