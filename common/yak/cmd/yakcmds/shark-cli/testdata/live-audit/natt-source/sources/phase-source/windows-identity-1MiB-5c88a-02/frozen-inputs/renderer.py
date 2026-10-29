"""Render unmodified Windows console cells/attributes recorded by ConPTY QA.
This is a native console buffer snapshot, not a redesigned UI mockup.
Usage: render_console.py NATIVE_BUFFER_JSON OUTPUT_PNG
"""
import json,sys
from pathlib import Path
from PIL import Image,ImageDraw,ImageFont
s=json.loads(Path(sys.argv[1]).read_text(encoding='utf8'))
font=ImageFont.truetype('C:/Windows/Fonts/consola.ttf',16);cw=font.getlength('0');ch=21;pad=10
palette=['#0b111b','#0037da','#13a10e','#3a96dd','#c50f1f','#881798','#c19c00','#cccccc','#767676','#3b78ff','#16c60c','#61d6d6','#e74856','#b4009e','#f9f1a5','#f2f2f2']
im=Image.new('RGB',(int(s['Width']*cw+2*pad),s['Height']*ch+2*pad),palette[0]);d=ImageDraw.Draw(im)
for y,row in enumerate(s['Rows']):
    for x,c in enumerate(row):
        a=s['Attributes'][y][x];px,py=pad+x*cw,pad+y*ch
        d.rectangle((px,py,px+cw+1,py+ch),fill=palette[(a>>4)&15]);d.text((px,py-1),c,font=font,fill=palette[a&15])
im.save(sys.argv[2])
