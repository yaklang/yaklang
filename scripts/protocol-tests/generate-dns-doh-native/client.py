"""Native dnspython/CoreDNS exchanges, independent of Yaklang's decoder.

The server's zone/configuration and client/tshark oracles remain distinct.
TLS key logs are generated only for these local, public test sessions.
"""
import argparse
import base64
import concurrent.futures
import json
from pathlib import Path
import ssl

import dns.edns
import dns.flags
import dns.message
import dns.query
import dns.rcode
import httpx


def canonical(message):
    return {
        "id": message.id,
        "qr": bool(message.flags & dns.flags.QR),
        "rcode": message.rcode(),
        "question": [
            {"name": q.name.to_text(), "type": q.rdtype, "class": q.rdclass}
            for q in message.question
        ],
        "answer": [
            {"name": rr.name.to_text(), "type": rr.rdtype, "class": rr.rdclass,
             "ttl": rr.ttl, "data": sorted(r.to_text() for r in rr)}
            for rr in message.answer
        ],
        "authority": [rr.to_text() for rr in message.authority],
        "additional": [
            {"name": rr.name.to_text(), "type": rr.rdtype, "class": rr.rdclass,
             "ttl": rr.ttl, "data": sorted(r.to_text() for r in rr)}
            for rr in message.additional
        ],
        "edns": message.edns,
        "udp_payload": message.payload,
        "edns_flags": message.ednsflags & 0xffff,
        "edns_options": [{"code": option.otype, "value": base64.b64encode(option.to_wire()).decode()}
                         for option in message.options],
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["dns", "h1", "h2"])
    parser.add_argument("out", type=Path)
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)
    queries = [("a.native.test", "A"), ("a.native.test", "AAAA"),
               ("alias.native.test", "A"), ("missing.native.test", "A"),
               ("text.native.test", "TXT"), ("native.test", "SOA")]
    rows = []
    if args.mode == "dns":
        for i, (name, kind) in enumerate(queries):
            for transport in ["udp", "tcp"]:
                q = dns.message.make_query(name, kind, use_edns=0, payload=1232)
                q.id = 0x4100 + i
                if i == 4:
                    q.use_edns(edns=0, payload=1232, options=[dns.edns.GenericOption(65001, b"independent")])
                exchange = dns.query.udp if transport == "udp" else dns.query.tcp
                response = exchange(q, "127.0.0.1", port=19553, timeout=3)
                rows.append({"transport": transport, "request": canonical(q),
                             "response": canonical(response),
                             "request_wire": base64.b64encode(q.to_wire()).decode(),
                             "response_wire": base64.b64encode(response.wire).decode()})
    else:
        context = ssl.create_default_context(cafile=str(args.out / "server.pem"))
        context.minimum_version = context.maximum_version = ssl.TLSVersion.TLSv1_2
        context.set_ciphers("ECDHE-RSA-AES128-GCM-SHA256")
        keylog = args.out / f"{args.mode}.keys"
        keylog.unlink(missing_ok=True)
        context.keylog_filename = str(keylog)
        with httpx.Client(http2=args.mode == "h2", verify=context, trust_env=False) as client:
            def exchange(index):
                name, kind = queries[index]
                q = dns.message.make_query(name, kind, use_edns=0, payload=1232)
                # RFC 8484 recommends zero; parallel streams must not use this
                # identifier as their HTTP request/response association key.
                q.id = 0
                wire = q.to_wire()
                method = "GET" if index % 2 else "POST"
                url = "https://127.0.0.1:19544/dns-query"
                if method == "GET":
                    response = client.get(url, params={"dns": base64.urlsafe_b64encode(wire).decode().rstrip("=")},
                                          headers={"accept": "application/dns-message"})
                else:
                    response = client.post(url, content=wire, headers={"content-type": "application/dns-message"})
                response.raise_for_status()
                assert response.http_version == ("HTTP/2" if args.mode == "h2" else "HTTP/1.1")
                answer = dns.message.from_wire(response.content)
                assert q.is_response(answer)
                return {"transport": f"tls-{args.mode}", "method": method,
                        "http_version": response.http_version, "http_status": response.status_code,
                        "content_type": response.headers["content-type"],
                        "request": canonical(q), "response": canonical(answer),
                        "request_wire": base64.b64encode(wire).decode(),
                        "response_wire": base64.b64encode(response.content).decode()}
            if args.mode == "h2":
                # Establish one connection first; the remaining queries use
                # independent concurrent HTTP/2 streams on that connection.
                rows.append(exchange(0))
                with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
                    rows.extend(pool.map(exchange, range(1, len(queries))))
            else:
                rows.extend(exchange(i) for i in range(len(queries)))
    for row in rows:
        name = row["request"]["question"][0]["name"]
        assert row["response"]["rcode"] == (dns.rcode.NXDOMAIN if name.startswith("missing.") else dns.rcode.NOERROR)
    (args.out / f"{args.mode}-client-oracle.json").write_text(json.dumps(rows, indent=2) + "\n")


if __name__ == "__main__":
    main()
