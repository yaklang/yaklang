"""Read hash-pinned packed trafficfixture members without Go or repo writes.

Implements the existing ZipCrypto/deflate/Zstd storage and packed chunk-recipe
reader for these two logical fixtures. Every physical and logical member is
checked against the existing index. Decompression uses the local Wireshark
Zstd DLL; no decoder/native replay or future output is used.
"""
from pathlib import Path
import argparse, ctypes, hashlib, json, struct, traceback, zipfile, zlib

def sha(data):
    return hashlib.sha256(data).hexdigest()

class Reader:
    def __init__(self, repo, dll):
        self.repo = repo
        self.base = repo / "common/bin-parser/testdata/corpus-batches"
        self.index_bytes = (self.base / "index.json").read_bytes()
        self.index = json.loads(self.index_bytes)
        self.pack = next(p for p in self.index["packs"] if p["id"] == "pr5265-consolidated-c830")
        self.raw = (self.base / self.pack["file"]).read_bytes()
        assert sha(self.raw) == self.pack["sha256"]
        self.zip = zipfile.ZipFile(self.base / self.pack["file"])
        self.cache = {}
        self.dll = ctypes.CDLL(str(dll))
        self.dll.ZSTD_decompress.argtypes = [ctypes.c_void_p, ctypes.c_size_t, ctypes.c_void_p, ctypes.c_size_t]
        self.dll.ZSTD_decompress.restype = ctypes.c_size_t
        self.dll.ZSTD_isError.argtypes = [ctypes.c_size_t]
        self.dll.ZSTD_isError.restype = ctypes.c_uint
        self.manifest = json.loads(self.physical("manifest.json"))
        self.chunks = []
        block, offset = 0, 0
        for table in self.manifest["tables"]:
            raw = self.physical(table["file"])
            assert table["first"] == len(self.chunks)
            if self.manifest["version"] == 2:
                assert len(raw) == table["count"] * 12
                chunks = list(struct.iter_unpack("<III", raw))
                self.chunks.extend(chunks)
            else:
                assert self.manifest["version"] == 3 and len(raw) == table["count"] * 2
                for (count,) in struct.iter_unpack("<H", raw):
                    assert count > 0
                    self.chunks.append((block, offset, count))
                    offset += count
                    limit = self.pack["members"][self.manifest["blocks"][block]]["bytes"]
                    assert offset <= limit
                    if offset == limit:
                        block, offset = block + 1, 0
        assert len(self.chunks) == self.manifest["chunk_count"]

    def physical(self, name):
        if name in self.cache:
            return self.cache[name]
        pin = self.pack["members"][name]
        info = self.zip.getinfo(name)
        if info.compress_type != 93:
            value = self.zip.read(name, pwd=self.pack["password"].encode())
        else:
            assert name.startswith("blocks/") and 0 < pin["bytes"] <= 4 << 20
            start = info.header_offset
            assert self.raw[start:start + 4] == b"PK\x03\x04"
            a, b = struct.unpack_from("<HH", self.raw, start + 26)
            data = self.raw[start + 30 + a + b:start + 30 + a + b + info.compress_size]
            assert info.flag_bits & 1 and len(data) == info.compress_size
            decrypt = zipfile._ZipDecrypter(self.pack["password"].encode())
            clear = decrypt(data)
            check = (info._raw_time >> 8) & 255 if info.flag_bits & 8 else (info.CRC >> 24) & 255
            assert clear[11] == check
            compressed = clear[12:]
            target = ctypes.create_string_buffer(pin["bytes"])
            source = ctypes.create_string_buffer(compressed)
            size = self.dll.ZSTD_decompress(target, pin["bytes"], source, len(compressed))
            assert not self.dll.ZSTD_isError(size) and size == pin["bytes"]
            value = target.raw[:size]
            assert zlib.crc32(value) & 0xffffffff == info.CRC
        assert len(value) == pin["bytes"] and sha(value) == pin["sha256"]
        self.cache[name] = value
        return value

    def member(self, batch_name, name):
        batch = next(b for b in self.index["batches"] if b["id"] == batch_name)
        assert batch["pack"] == self.pack["id"]
        pin = batch["members"][name]
        recipe = self.manifest["recipes"][pin["sha256"]]
        raw = self.physical(recipe["file"])[recipe["offset"]:recipe["offset"] + recipe["bytes"]]
        decoder = zlib.decompressobj(-15)
        data = decoder.decompress(raw) + decoder.flush()
        assert decoder.eof and not decoder.unused_data and not decoder.unconsumed_tail
        previous, at, values = 0, 0, []
        for _ in range(recipe["count"]):
            value, shift, begin = 0, 0, at
            while True:
                byte = data[at]
                at += 1
                value |= (byte & 127) << shift
                if byte < 128:
                    break
                shift += 7
                assert shift < 35
            assert at - begin <= 5 and (at - begin == 1 or value >= 1 << (7 * (at - begin - 1)))
            previous += (value >> 1) ^ -(value & 1)
            assert 0 <= previous < len(self.chunks)
            block, offset, count = self.chunks[previous]
            values.append(self.physical(self.manifest["blocks"][block])[offset:offset + count])
        assert at == len(data)
        output = b"".join(values)
        assert len(output) == pin["bytes"] and sha(output) == pin["sha256"]
        return output, dict(batch=batch_name, logical_member=name, logical_SHA256=pin["sha256"], bytes=pin["bytes"])

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--repo", type=Path, required=True)
    p.add_argument("--zstd", type=Path, required=True)
    p.add_argument("--out", type=Path, required=True)
    args = p.parse_args()
    args.out.mkdir(parents=True, exist_ok=False)
    try:
        reader = Reader(args.repo, args.zstd)
        members = []
        cases = []
        requested = [
            ("validation-tftp-negotiation", "controls.json"),
            ("validation-tftp-negotiation", "answers/rrq-datagram-no-stitch.json"),
            ("validation-tftp-negotiation", "cases/rrq-datagram-no-stitch.pcap"),
            ("validation-tftp-file", "tftp-file/controls.json"),
            ("validation-tftp-file", "tftp-file/UDP-no-byte-stitch.pcapng"),
        ]
        for batch, name in requested:
            raw, proof = reader.member(batch, name)
            target = args.out / batch / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(raw)
            members.append(dict(path=str(target), **proof))
            if name.endswith("controls.json"):
                doc = json.loads(raw)
                selected = [c for c in doc["cases"] if c["id"] in ("rrq-datagram-no-stitch", "UDP-no-byte-stitch")]
                assert len(selected) == 1
                cases.append(dict(batch=batch, schema=doc["schema"], case=selected[0]))
        result = dict(scope=__doc__, driver_SHA256=sha(Path(__file__).read_bytes()),
                      source_index_SHA256=sha(reader.index_bytes), pack_file=reader.pack["file"],
                      pack_SHA256=reader.pack["sha256"], Zstd_DLL=str(args.zstd),
                      Zstd_DLL_SHA256=sha(args.zstd.read_bytes()), pack_version=reader.manifest["version"],
                      extracted_members=members, complete_selected_cases=cases,
                      no_Go_native_or_future_output=True)
        path = args.out / "summary.json"
        path.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(dict(summary=str(path), SHA256=sha(path.read_bytes()), cases=[c["case"]["id"] for c in cases])))
    except Exception:
        path = args.out / "failure.json"
        path.write_text(json.dumps(dict(traceback=traceback.format_exc()), indent=2) + "\n", encoding="utf-8")
        raise

if __name__ == "__main__":
    main()
