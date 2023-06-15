# Copyright (C) 2021 Greenbone Networks GmbH
#
# SPDX-License-Identifier: GPL-2.0-or-later
#
# This program is free software; you can redistribute it and/or
# modify it under the terms of the GNU General Public License
# as published by the Free Software Foundation; either version 2
# of the License, or (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.

# nb: Normally defined in byte_func.inc but openvas-nasl-lint will report
# those as not declared in this include.
# Little-endian byte order
BYTE_ORDER_LITTLE_ENDIAN = 1;
# Big-endian byte order
BYTE_ORDER_BIG_ENDIAN = 2;

# @brief Creates a DTLS Client Hello packet
#
# @param version Version to use (currently supported DTLS10 (DTLS 1.0)) (default: DTLS10)
# @param random Client random to send to the server (32 bytes)
# @param seq_num Sequence number to use
# @param cookie (optional) Cookie sent by the server
#
# @return Created DTLS Client Hello packet if success otherwise NULL
#
function dtls_create_client_hello(version, random, seq_num, cookie)
{
  local_var version, random, seq_num, cookie;
  local_var dtls_header, handshake_proto, fragment;
  local_var data_len, data, cookie_len;

  if (isnull(version))
    version = "DTLS10";

  if (isnull(random)) {
    set_kb_item(name: "vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#random#-#dtls_create_client_hello");
    return NULL;
  }

  if (isnull(seq_num)) {
    set_kb_item(name: "vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#seq_num#-#dtls_create_client_hello");
    return NULL;
  }

  set_byte_order(BYTE_ORDER_BIG_ENDIAN);

  if (version == "DTLS10")
    version = raw_string(0xfe, 0xff);                     # DTLS 1.0
#  else if (version == "DTLS12")
#    version = raw_string(0xfe, 0xfd);                     # DTLS 1.2
  else {
    set_kb_item(name: "vt_debug_misc/" + get_script_oid(), value: get_script_oid() + "#-#dtls_create_client_hello#-#Unsupported DTLS Version");
    return NULL;
  }

  if (isnull(cookie) || cookie == "")
    cookie_len = 0;
  else
    cookie_len = strlen(cookie);

  fragment = raw_string(version,                   # Version
                        random,                    # Client Random
                        0x00,                      # Session ID length
                        mkbyte(cookie_len));       # Cookie length

  if (!isnull(cookie) || cookie != "")
    fragment = raw_string(fragment, cookie);

  fragment = raw_string(fragment,
                        0x00, 0x10,                # Cipher Suites length
                        sslv3_tls_raw_ciphers["TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA"],
                        sslv3_tls_raw_ciphers["TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA"],
                        sslv3_tls_raw_ciphers["TLS_DHE_RSA_WITH_AES_256_CBC_SHA"],
                        sslv3_tls_raw_ciphers["TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA"],
                        sslv3_tls_raw_ciphers["TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA"],
                        sslv3_tls_raw_ciphers["TLS_DHE_RSA_WITH_AES_128_CBC_SHA"],
                        sslv3_tls_raw_ciphers["TLS_RSA_WITH_AES_256_CBC_SHA"],
                        sslv3_tls_raw_ciphers["TLS_RSA_WITH_AES_128_CBC_SHA"],
                        0x01,                      # Compression Method length
                        0x00,                      # Compression Method
                        0x00, 0x24,                # Extension length
                        0x00, 0x0b,                # Type: ec_point_formats
                        0x00, 0x04,                # Length
                        0x03,                      # EC point formats length
                        0x00,                      # EC point format: uncompressed
                        ec_point_formats["ansiX962_compressed_prime"],
                        ec_point_formats["ansiX962_compressed_char2"],
                        0x00, 0x0a,                # Type: supported groups
                        0x00, 0x0c,                # Length
                        0x00, 0x0a,                # Supported Groups List Length
                        elliptic_curves["x25519"],
                        elliptic_curves["secp256r1"],
                        elliptic_curves["x448"],
                        elliptic_curves["secp521r1"],
                        elliptic_curves["secp521r1"],
                        0x00, 0x23,                # Type: session ticket
                        0x00, 0x00,                # Length
                        0x00, 0x16,                # Type: encrypt_then_mac
                        0x00, 0x00,                # Length
                        0x00, 0x17,                # extended_master_secret
                        0x00, 0x00);               # Length

  handshake_proto = raw_string(0x01,                      # Handshake Type (Client Hello)
                               0x00, mkword(strlen(fragment)),
                               mkword(seq_num),           # Message Sequenze
                               0x00, 0x00, 0x00,          # Fragment Offset
                               0x00, mkword(strlen(fragment))); # Fragment Length

  data = handshake_proto + fragment;
  data_len = strlen(data);

  dtls_header = raw_string(0x16,                          # Content-Type (Handshake)
                           version,                       # Version
                           0x00, 0x00,                    # Epoch
                           0x00, 0x00, 0x00, 0x00, 0x00,  # Sequence Number
                           mkbyte(seq_num),
                           mkword(data_len));             # Length of data

  return dtls_header + data;
}

# @brief Does the DTLS Client Hello handshake part. The client sends a "Client Hello"
#        packet with a client random to the server. The server will send a
#        "Hello Verify Request" back with a cookie. The client sends the same
#        client hello request with the received cookie again. The server will
#        respond with a "Hello Server".
#
# @param socket UDP socket to use
#
# @return Returns the next sequence number to use, or -1 in case of an alert was received
#         or NULL in case of an error.
#
function dtls_client_hello(socket)
{
  local_var socket;
  local_var seq_num, version, rand, cookie_len, hello1, hello2;
  local_var cookie, recv;

  if (isnull(socket)) {
    set_kb_item(name: "vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#dtls_client_hello");
    return NULL;
  }

  set_byte_order(BYTE_ORDER_BIG_ENDIAN);

  seq_num = 0;
  version = "DTLS10";
  rand = dec2hex(num:unixtime()) + rand_str(length:28);
  cookie_len = 0;

  hello1 = dtls_create_client_hello(version: version, random: rand, seq_num: seq_num);
  if (isnull(hello1))
    return NULL;

  send(socket: socket, data: hello1);
  recv = recv(socket: socket, length: 1024, min: 14);

  if (!recv || strlen(recv) < 13)
    return NULL;
  # Check for "Alert"
  if (ord(recv[13]) == 21)
    return -1;
  # Check for "Hello Verify Request" (0x03)
  if (strlen(recv) < 27 || ord(recv[13]) != 3)
    return NULL;

  cookie_len = ord(recv[27]);

  if (strlen(recv) < 27 + cookie_len)
    return NULL;

  cookie = substr(recv, 28, 28 + cookie_len - 1);

  seq_num += 1;

  hello2 = dtls_create_client_hello(version: version, random: rand, seq_num: seq_num, cookie: cookie);
  if (isnull(hello2))
    return NULL;

  send(socket: socket, data: hello2);
  recv = recv(socket: socket, length: 1024);

  if (!recv || strlen(recv) < 13)
    return NULL;
  # Check for "Alert"
  if (ord(recv[13]) == 21)
    return -1;
  # Check for "Server Hello" (0x02)
  if (!recv || strlen(recv) < 13 || ord(recv[13]) != 2)
    return NULL;

  return seq_num + 1;
}

# @brief Sends an "Close Notify" Alert message to the server (indicates a closing
#        of the "connection". Some servers might just ignore this.
#
# @param socket UDP socket to use
# @param seq_num Sequence number to use
#
function dtls_send_alert(socket, seq_num)
{
  local_var socket, sec_num, version, msg;

  if (isnull(socket)) {
    set_kb_item(name: "vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#dtls_send_alert");
    return NULL;
  }

  if (isnull(seq_num)) {
    set_kb_item(name: "vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#seq_num#-#dtls_send_alert");
    return NULL;
  }

  version = raw_string(0xfe, 0xff);                  # DTLS 1.0

  msg = raw_string(0x15,
                   version,
                   0x00, 0x00,                       # Epoch
                   0x00, 0x00, 0x00, 0x00, 0x00,     # Sequence number
                   mkbyte(seq_num),
                   0x00, 0x07,                       # Length
                   0x02,                             # Level (Fatal)
                   0x00,                             # Description (Close Notify)
                   crap(length: 5));

  send(socket: socket, data: msg);
  recv(socket: socket, length: 14);

  return;
}
