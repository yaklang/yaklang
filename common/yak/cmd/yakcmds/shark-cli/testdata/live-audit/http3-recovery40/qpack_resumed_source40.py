"""Independent QPACK blocked-section source; no Yak event or label input."""
from pathlib import Path
import argparse, hashlib, json, sys

sys.path.insert(0, r'C:\tmp\yak-shark-recovery-20261024\aioquic-libs')
import pylsqpack

ap = argparse.ArgumentParser(description=__doc__)
ap.add_argument('--out', type=Path, required=True)
a = ap.parse_args()
a.out.mkdir()
block = bytes.fromhex('03811011d1d7')
encoder = bytes.fromhex('3fbd01c00f7777772e6578616d706c652e636f6dc10c2f73616d706c652f70617468')
decoder = pylsqpack.Decoder(220, 1)
try:
    decoder.feed_header(0, block)
except pylsqpack.StreamBlocked:
    pass
else:
    raise AssertionError('source field section must await two encoder insertions')
assert decoder.feed_encoder(encoder[:7]) == []
assert decoder.feed_encoder(encoder[7:]) == [0]
ack, headers = decoder.resume_header(0)
expected = [(b':authority', b'www.example.com'), (b':path', b'/sample/path'),
            (b':method', b'GET'), (b':scheme', b'https')]
assert headers == expected
sha = lambda b: hashlib.sha256(b).hexdigest()
result = dict(scope=__doc__, controller_SHA256=sha(Path(__file__).read_bytes()),
              block_SHA256=sha(block), encoder_SHA256=sha(encoder),
              independently_decoded_headers=[dict(name=k.decode(), value=v.decode()) for k, v in headers],
              header_count=len(headers), method_occurrences=1,
              partial_encoder_does_not_unblock=True,
              reference='https://www.rfc-editor.org/rfc/rfc9204.html#appendix-B.2',
              boundary='PDU completing decoding owns newly available method; original block wire ownership remains unchanged; no method inherited by later duplicates')
target = a.out / 'source.json'
target.write_text(json.dumps(result, indent=2) + '\n', encoding='utf8')
print(json.dumps(dict(summary=str(target), SHA256=sha(target.read_bytes()), header_count=4)))
