"""Save primary Intserv wire specifications outside the repository for the next bounded audit."""
from pathlib import Path
import hashlib, json, urllib.request

root = Path(__file__).parent
out = root / 'RSVP-Intserv-primary46-01'
out.mkdir(exist_ok=False)
rows = []
for number in (2210,2215,2212):
    url = f'https://www.rfc-editor.org/rfc/rfc{number}.txt'
    with urllib.request.urlopen(url, timeout=40) as response:
        data = response.read(4*1024*1024)
        final_url = response.url
    assert data and len(data) < 4*1024*1024
    path = out / f'rfc{number}.txt'
    path.write_bytes(data)
    rows.append(dict(number=number, URL=url, final_URL=final_url, path=str(path), bytes=len(data), SHA256=hashlib.sha256(data).hexdigest()))
(out/'manifest.json').write_text(json.dumps(dict(scope=__doc__, documents=rows, controller_SHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest()),indent=2)+'\n',encoding='utf8')
print(json.dumps(rows))
