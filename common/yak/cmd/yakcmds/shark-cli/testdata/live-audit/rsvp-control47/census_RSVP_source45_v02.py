"""Add the frozen capture reader's sibling import path; source logic unchanged."""
from pathlib import Path
source=Path(__file__).with_name('census_RSVP_source45.py').read_text('utf8')
old="rp=Path(doc['capture_reader']);assert sha(rp)==doc['capture_reader_SHA256'];spec="
assert source.count(old)==1
source=source.replace(old,"rp=Path(doc['capture_reader']);assert sha(rp)==doc['capture_reader_SHA256'];sys.path.insert(0,str(rp.parent));spec=")
exec(compile(source,str(Path(__file__)),'exec'),globals())
