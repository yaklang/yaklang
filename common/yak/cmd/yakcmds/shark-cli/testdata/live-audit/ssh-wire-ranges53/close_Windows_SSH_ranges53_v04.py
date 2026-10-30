"""Bind the packet number/length row without assuming its Unicode selection glyph."""
from pathlib import Path
base=Path(__file__).with_name('close_Windows_SSH_ranges53_v03.py')
source=base.read_text('utf8')
old="packet_rows=[row for row in shots['01-protocol-fields']['Rows']if re.search('>'+str(case['frame'])+r'\\\\s+\\\\d\\\\d:\\\\d\\\\d:\\\\d\\\\d\\\\.',row)]"
new="""packet_pattern=r'(\\\\d+)\\\\s+\\\\d{2}:\\\\d{2}:\\\\d{2}\\\\.\\\\d{3}\\\\s+SSH\\\\s+\\\\S+\\\\s+\\\\S+\\\\s+(\\\\d+)\\\\s+'
 packet_rows=[row for row in shots['01-protocol-fields']['Rows']if re.search(packet_pattern,row)]"""
assert source.count(old)==1;source=source.replace(old,new)
old='assert len(packet_rows)==1'
new="assert len(packet_rows)==1\n+ match=re.search(packet_pattern,packet_rows[0]);assert int(match.group(1))==case['frame']and int(match.group(2))==len(physical)"
new=new.replace('\n+ ','\n ')
assert source.count(old)==1;source=source.replace(old,new)
old="Windows-SSH-ranges-root-closure53-03"
assert source.count(old)==1;source=source.replace(old,'Windows-SSH-ranges-root-closure53-04')
exec(compile(source,str(Path(__file__)),'exec'),globals())
