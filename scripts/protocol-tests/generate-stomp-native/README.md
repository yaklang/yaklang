# Native STOMP corpus

`generate.py` starts an isolated RabbitMQ 4.1.0 container using the immutable
image digest in the script, enables `rabbitmq_stomp`, and captures real TCP
connections from stomp.py 8.2.0. It requires Docker, Python, tcpdump capture
permission, and tshark. The checked-in captures used macOS `lo0`; Linux users
can select `--interface lo`.

```sh
python3 -m venv /tmp/stomp-native-venv
/tmp/stomp-native-venv/bin/pip install -r scripts/protocol-tests/generate-stomp-native/requirements.txt
/tmp/stomp-native-venv/bin/python scripts/protocol-tests/generate-stomp-native/generate.py --out /tmp/stomp-native-capture
```

The output directory must be empty. Existing originals are never overwritten.
The listener binds only `127.0.0.1:46164` (overridable with `--port`). The
`fixture/fixture` credentials are disposable lab credentials deliberately
present in the original plaintext packets. The script removes its own
container and volume when finished.

There are three independent connections negotiating STOMP 1.0, 1.1 and 1.2.
Each subscribes, aborts one transaction, commits a different SEND, verifies
its echoed body/header, acknowledges it, requests receipts, unsubscribes and
disconnects. The 1.1/1.2 variants also NACK a message with requeue disabled;
1.2 sends an embedded NUL body with content-length. No packet headers,
checksums, sequence numbers, timestamps or application frames are rewritten.
Different runs naturally have different session IDs, TCP ports and hashes.

The client oracle records stomp.py's `on_send`, `on_connected`, `on_message`
and `on_receipt` callbacks and asserts application outcomes before writing
success. `on_send` contains escaped outgoing header values; `on_message`
contains independently decoded values. The Go comparison uses the echoed
server callback for the custom SEND header, rather than using the decoder
under test to manufacture an expected value. Broker version, listeners,
logs and post-session queue output provide separate server evidence. Empty
queue files mean the successful `list_queues` command returned no queues
(the test subscriptions use auto-delete).

Tshark 4.4.8 has no STOMP dissector. Its oracle is restricted to frame number,
time/length, IP/TCP endpoints, stream, sequence/ACK numbers, flags, payload
length and raw TCP payload; no application-level tshark verdict is claimed.
The checked-in native PCAPs have their own SHA-256 manifest. Synthetic Go
regressions and fuzz mutations are not native captures.

RabbitMQ applies escape validation even to its 1.0 connection and rejected a
literal `\\t` header during setup experiments. This broker behavior is not a
positive conformance oracle for legacy raw escaping; the STOMP 1.0 raw-header
case is separately and explicitly tested using synthetic specification bytes.

References and licenses:

- STOMP [1.0](https://stomp.github.io/stomp-specification-1.0.html),
  [1.1](https://stomp.github.io/stomp-specification-1.1.html), and
  [1.2](https://stomp.github.io/stomp-specification-1.2.html).
- RabbitMQ [4.1.0 source](https://github.com/rabbitmq/rabbitmq-server/tree/v4.1.0),
  MPL-2.0; [STOMP plugin documentation](https://www.rabbitmq.com/docs/stomp).
- stomp.py [8.2.0 source](https://github.com/jasonrbriggs/stomp.py/tree/abb4d8244d6640f6a7cf260dc75c106aaa078841),
  Apache-2.0.
- Generated capture/evidence files: CC0-1.0. Generator/test code follows the
  repository's license.
