"""Auto-loaded idna codec shim for benchmark server subprocesses."""
import codecs as _c
try:
    _c.lookup("idna")
except LookupError:
    def _e(i, errors='strict'):
        return ((i.encode('ascii', errors) if isinstance(i, str) else i), len(i))
    def _d(i, errors='strict'):
        return ((i.decode('ascii', errors) if isinstance(i, (bytes, bytearray)) else i), len(i))
    _c.register(lambda n: _c.CodecInfo(name='idna', encode=_e, decode=_d, incrementalencoder=_c.IncrementalEncoder, incrementaldecoder=_c.IncrementalDecoder, streamwriter=_c.StreamWriter, streamreader=_c.StreamReader) if n == 'idna' else None)
