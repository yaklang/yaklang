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

global_var ENCRYPTION_ALGORITHMS, AUTHENTICATION_METHODS, HASH_ALGORITHMS, GROUP_DESCRIPTIONS, EXCHANGE_TYPES_RAW, PROTOCOL_IDS;
global_var EXCHANGE_TYPES, PAYLOADS, PAYLOADS_RAW, VERSIONS, VERSIONS_RAW, ID_TYPES, ID_TYPES_RAW;

ENCRYPTION_ALGORITHMS["des"] = raw_string( 0x80, 0x01, 0x00, 0x01 );
ENCRYPTION_ALGORITHMS["3des"] = raw_string( 0x80, 0x01, 0x00, 0x05 );
ENCRYPTION_ALGORITHMS["cast"] = raw_string( 0x80, 0x01, 0x00, 0x06 );
ENCRYPTION_ALGORITHMS["aes/128"] = make_list( raw_string( 0x80, 0x01, 0x00, 0x07 ), raw_string( 0x80, 0x0E, 0x00, 0x80 ) );
ENCRYPTION_ALGORITHMS["aes/192"] = make_list( raw_string( 0x80, 0x01, 0x00, 0x07 ), raw_string( 0x80, 0x0E, 0x00, 0xC0 ) );
ENCRYPTION_ALGORITHMS["aes/256"] = make_list( raw_string( 0x80, 0x01, 0x00, 0x07 ), raw_string( 0x80, 0x0E, 0x01, 0x00 ) );

AUTHENTICATION_METHODS["psk"] = raw_string( 0x80, 0x03, 0x00, 0x01 );
AUTHENTICATION_METHODS["rsa"] = raw_string( 0x80, 0x03, 0x00, 0x03 );
AUTHENTICATION_METHODS["ECDSA"] = raw_string( 0x80, 0x03, 0x00, 0x08 );
AUTHENTICATION_METHODS["Hybrid"] = raw_string( 0x80, 0x03, 0xFA, 0xDD );
AUTHENTICATION_METHODS["XAUTH"] = raw_string( 0x80, 0x03, 0xFD, 0xE9 );

HASH_ALGORITHMS["md5"] = raw_string( 0x80, 0x02, 0x00, 0x01 );
HASH_ALGORITHMS["sha1"] = raw_string( 0x80, 0x02, 0x00, 0x02 );
HASH_ALGORITHMS["sha2-256"] = raw_string( 0x80, 0x02, 0x00, 0x04 );
HASH_ALGORITHMS["sha2-384"] = raw_string( 0x80, 0x02, 0x00, 0x05 );
HASH_ALGORITHMS["sha2-512"] = raw_string( 0x80, 0x02, 0x00, 0x06 );

GROUP_DESCRIPTIONS["768"] = raw_string( 0x80, 0x04, 0x00, 0x01 );
GROUP_DESCRIPTIONS["1024"] = raw_string( 0x80, 0x04, 0x00, 0x02 );
GROUP_DESCRIPTIONS["1536"] = raw_string( 0x80, 0x04, 0x00, 0x05 );
GROUP_DESCRIPTIONS["2048"] = raw_string( 0x80, 0x04, 0x00, 0x0E );

EXCHANGE_TYPES[raw_string( 0x02 )] = "Identity Protection (Main Mode)";
EXCHANGE_TYPES[raw_string( 0x04 )] = "Aggressive";
EXCHANGE_TYPES[raw_string( 0x05 )] = "Informational";

EXCHANGE_TYPES_RAW["Identity Protection (Main Mode)"] = raw_string( 0x02 );
EXCHANGE_TYPES_RAW["Aggressive"] = raw_string( 0x04 );
EXCHANGE_TYPES_RAW["Informational"] = raw_string( 0x05 );

PROTOCOL_IDS["tcp"] = raw_string( 0x06 );
PROTOCOL_IDS["udp"] = raw_string( 0x11 );

PAYLOADS[raw_string( 0x00 )] = "NONE";
PAYLOADS[raw_string( 0x01 )] = "Security Association";
PAYLOADS[raw_string( 0x02 )] = "Proposal";
PAYLOADS[raw_string( 0x03 )] = "Transform";
PAYLOADS[raw_string( 0x04 )] = "Key Exchange";
PAYLOADS[raw_string( 0x05 )] = "Identification";
PAYLOADS[raw_string( 0x08 )] = "Hash";
PAYLOADS[raw_string( 0x0A )] = "Nonce";
PAYLOADS[raw_string( 0x0B )] = "Notification";
PAYLOADS[raw_string( 0x0D )] = "Vendor ID";

PAYLOADS_RAW["NONE"] = raw_string( 0x00 );
PAYLOADS_RAW["Security Association"] = raw_string( 0x01 );
PAYLOADS_RAW["Proposal"] = raw_string( 0x02 );
PAYLOADS_RAW["Transform"] = raw_string( 0x03 );
PAYLOADS_RAW["Key Exchange"] = raw_string( 0x04 );
PAYLOADS_RAW["Identification"] = raw_string( 0x05 );
PAYLOADS_RAW["Hash"] = raw_string( 0x08 );
PAYLOADS_RAW["Nonce"] = raw_string( 0x0A );
PAYLOADS_RAW["Notification"] = raw_string( 0x0B );
PAYLOADS_RAW["Vendor ID"] = raw_string( 0x0D );

VERSIONS[raw_string( 0x10 )] = "1.0";
VERSIONS[raw_string( 0x20 )] = "2.0";

VERSIONS_RAW["1.0"] = raw_string( 0x10 );
VERSIONS_RAW["2.0"] = raw_string( 0x20 );

ID_TYPES[raw_string( 0x03 )] = "USER_FQDN";

ID_TYPES_RAW["USER_FQDN"] = raw_string( 0x03 );

# @brief Creates a raw "transforms" packet based on the current supported transforms.
#
# @param enable_short_list If set to TRUE a shorter list of default transforms are generated.
#
# @return A list containing:
#         - A raw "transforms" packet of the current supported transforms in list index 0.
#         - The total number of created transforms in list index 1.
#
function isakmp_create_transforms_packet_from_list( enable_short_list ) {

  local_var enable_short_list;
  local_var supported_auth_method_list, supported_encryption_algo_list, supported_hash_algo_list, supported_group_desc_list;
  local_var max_transforms, current_transform, packet, supported_auth, supported_encryption, supported_hash, supported_group;

  # The default list as used by ike-scan. Some services don't respond to the "full" list below
  # so a shorter list can be tried as well.
  if( enable_short_list ) {
    supported_auth_method_list = make_list( "psk" );
    supported_encryption_algo_list = make_list( "des", "3des" );
    supported_hash_algo_list = make_list( "md5", "sha1" );
    supported_group_desc_list = make_list( "768", "1024" );
  } else {
    supported_auth_method_list = make_list( "psk", "rsa", "Hybrid", "XAUTH" );
    supported_encryption_algo_list = make_list( "des", "3des", "aes/128", "aes/192", "aes/256" );
    supported_hash_algo_list = make_list( "md5", "sha1" );
    supported_group_desc_list = make_list( "768", "1024", "1536" );
  }

  max_transforms = max_index( supported_auth_method_list ) * max_index( supported_encryption_algo_list ) * max_index( supported_hash_algo_list ) * max_index( supported_group_desc_list );
  current_transform = 0;

  foreach supported_auth_method( supported_auth_method_list ) {
    foreach supported_encryption_algo( supported_encryption_algo_list ) {
      foreach supported_hash_algo( supported_hash_algo_list ) {
        foreach supported_group_desc( supported_group_desc_list ) {
          current_transform++;
          packet += isakmp_create_transforms_packet_single( encryption_algo:supported_encryption_algo, auth_method:supported_auth_method, hash_algo:supported_hash_algo, group_desc:supported_group_desc, max_transforms:max_transforms, current_transform:current_transform );
        }
      }
    }
  }

  return make_list( packet, max_transforms );
}

# @brief Creates a single raw "transforms" packet based on the given data.
#
# @param encryption_algo   The Encryption-Algorithm to use. Currently supported: "des", "3des",
#                          "aes/128", "aes/192", "aes/256"
# @param auth_method       The Authentication-Method to use. Currently supported: "psk", "rsa",
#                          "Hybrid", "XAUTH"
# @param hash_algo         The Hash-Algorithm to use. Currently supported: "md5", "sha1"
# @param group_desc        The Group-Description to use. Currently supported:  "768", "1024", "1536"
# @param max_transforms    The total number of transforms to create. This is used to add a final
#                          "NONE" Next Payload to the created package to define that no additional
#                          transforms follow.
# @param current_transform The current number of transforms performed / created (needs to be calculated
#                          / passed by the calling function / VT).
#
# @return A single raw "transforms" packet based on the given data or NULL if not all required
#         parameters or an unsupported encryption method, auth type, hash algorithm or group was
#         passed.
#
function isakmp_create_transforms_packet_single( encryption_algo, auth_method, hash_algo, group_desc, max_transforms, current_transform ) {

  local_var encryption_algo, auth_method, hash_algo, group_desc, max_transforms, current_transform;
  local_var trans_length, encryption_algo_info, enc, key_length, next_payload, packet;

  if( ! encryption_algo ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_transforms_packet_single#-#encryption_algo" );
    return NULL;
  }

  if( ! auth_method ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_transforms_packet_single#-#auth_method" );
    return NULL;
  }

  if( ! hash_algo ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_transforms_packet_single#-#hash_algo" );
    return NULL;
  }

  if( ! group_desc ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_transforms_packet_single#-#group_desc" );
    return NULL;
  }

  if( encryption_algo !~ "^(des|3des|aes/128|aes/192|aes/256)$" ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_transforms_packet_single(): Unsupported Encryption-Algorithm '" + encryption_algo + "' given to 'encryption_algo' parameter. Currently supported: des, 3des, aes/128, aes/192, aes/256" );
    return NULL;
  }

  if( auth_method !~ "^(psk|rsa|Hybrid|XAUTH)$" ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_transforms_packet_single(): Unsupported Authentication-Method '" + auth_method + "' given to 'auth_method' parameter. Currently supported: psk, rsa, Hybrid, XAUTH" );
    return NULL;
  }

  if( hash_algo !~ "^(md5|sha1)$" ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_transforms_packet_single(): Unsupported Hash-Algorithm '" + hash_algo + "' given to 'hash_algo' parameter. Currently supported: md5, sha1" );
    return NULL;
  }

  if( group_desc !~ "^(768|1024|1536)$" ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_transforms_packet_single(): Unsupported Group-Description '" + group_desc + "' given to 'group_desc' parameter. Currently supported: 768, 1024, 1536" );
    return NULL;
  }

  # AES requires an extra field for the key length
  if( encryption_algo =~ "^aes/" ) {
    trans_length = raw_string( 0x00, 0x28 );
    encryption_algo_info = ENCRYPTION_ALGORITHMS[encryption_algo];
    encryption_algo_raw = encryption_algo_info[0];
    key_length = encryption_algo_info[1];
  } else {
    trans_length = raw_string( 0x00, 0x24 );
    encryption_algo_raw = ENCRYPTION_ALGORITHMS[encryption_algo];
    key_length = "";
  }

  if( current_transform == max_transforms )
    next_payload = PAYLOADS_RAW["NONE"];          # Next payload: NONE / No Next Payload (0)
  else
    next_payload = PAYLOADS_RAW["Transform"];     # Next payload: Transform (3)

  # Payload: Transform (3)
  packet  = next_payload;                         # Next payload
  packet += raw_string( 0x00 );                   # Reserved
  packet += trans_length;                         # Payload length
  packet += mkbyte( current_transform );          # Transform number
  packet += raw_string( 0x01 );                   # Transform ID: KEY_IKE (1)
  packet += raw_string( 0x00, 0x00 );             # Reserved
  packet += encryption_algo_raw;                  # Encryption-Algorithm
  packet += HASH_ALGORITHMS[hash_algo];           # Hash-Algorithm
  packet += AUTHENTICATION_METHODS[auth_method];  # Authentication-Method
  packet += GROUP_DESCRIPTIONS[group_desc];       # Group-Description

  if( key_length )
    packet += key_length;                         # Key-Length (Only set for AES Encryption-Algorithm)

  packet += raw_string( 0x80, 0x0b, 0x00, 0x01 ); # Life-Type: Seconds
  packet += raw_string( 0x00, 0x0c, 0x00, 0x04,
                        0x00, 0x00, 0x70, 0x80 ); # Life-Duration: 28800

  return packet;
}

# @brief Creates an Aggressive Mode package
#
# @param port               The port for the connection to the remote service. Defaults to 500 if
#                           not given.
# @param ipproto            The IP protocol (tcp or udp) for the connection to the remote service.
#                           Defaults to udp if not given.
# @param dhgroup            The Diffie Hellman Group for the connection to the remote service.
#                           Currently supported values are 1,2,5,14,15,16,17,18 (MODP only).
#                           Defaults to 2 if not given.
# @param aggressive_mode_id The Aggressive mode ID for the connection to the remote service.
#                           Defaults to "vpngroup" if not given.

function isakmp_generate_aggressive_package( port, ipproto, dhgroup, aggressive_mode_id ) {

  local_var port, ipproto, dhgroup, aggressive_mode_id;
  local_var key_length, package;

  if( ipproto && ipproto !~ "^(tcp|udp)$" ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_generate_aggressive_package(): Unsupported protocol '" + ipproto + "' given to 'ipproto' parameter. Currently supported: udp, tcp" );
    return NULL;
  }

  if( dhgroup && dhgroup !~ "^(1|2|5|14|15|16|17|18)$" ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_generate_aggressive_package(): Unsupported Diffie Hellman Group '" + dhgroup + "' given to 'dhgroup' parameter. Currently supported:1,2,5,14,15,16,17,18 (MODP only)" );
    return NULL;
  }

  if( ! port )
    port = 500;

  if( ! ipproto )
    ipproto = "udp";

  if( ! dhgroup )
    dhgroup = 2;

  if( isnull( aggressive_mode_id ) )
    aggressive_mode_id = "vpngroup";

  # TODO: Check other DH groups
  if( dhgroup == 1 )
    key_length = 96;
  else if( dhgroup == 2 )
    key_length = 128;
  else if( dhgroup == 5 )
    key_length = 192;

  # Payload: Key Exchange (4)
  packet  = PAYLOADS_RAW["Nonce"];                                                                      # Next payload: Nonce (10)
  packet += raw_string( 0x00 );                                                                         # Reserved
  packet += mkword( key_length + 4 );                                                                   # Payload length
  packet += raw_string( rand_str( length:key_length, charset:"abcdefghiklmnopqrstuvwxyz0123456789" ) ); # Key Exchange Data

  # Payload: Nonce (10)
  packet += PAYLOADS_RAW["Identification"];                                                             # Next payload: Identification (5)
  packet += raw_string( 0x00 );                                                                         # Reserved
  packet += mkword( 20 + 4 );                                                                           # Payload length
  packet += raw_string( rand_str( length:20, charset:"abcdefghiklmnopqrstuvwxyz0123456789" ) );         # Nonce DATA

  # Payload: Identification (5)
  packet += PAYLOADS_RAW["NONE"];                                                                       # Next payload: NONE / No Next Payload (0)
  packet += raw_string( 0x00 );                                                                         # Reserved
  packet += mkword( strlen( aggressive_mode_id ) + 4 + 4 );                                             # Payload length
  packet += ID_TYPES_RAW["USER_FQDN"];                                                                  # ID type: USER_FQDN
  packet += PROTOCOL_IDS[ipproto];                                                                      # Protocol ID (UDP or TCP)
  packet += mkword( port );                                                                             # Port
  packet += aggressive_mode_id;                                                                         # Identification Data

  return packet;
}

# @brief Creates an IKE / ISAKMP request packet.
#
# @param port               The port for the connection to the remote service. Only used for the
#                           aggressive mode exchange_type. Defaults to 500 if not given.
# @param ipproto            The IP protocol (tcp or udp) for the connection to the remote service.
#                           Only used for the aggressive mode exchange_type. Defaults to udp if not
#                           given.
# @param exchange_type      The exchange type for the connection to the remote service. Currently
#                           supported values:
#                           Identity Protection (Main Mode) = Main mode (2)
#                           Aggressive = Aggressive mode (4)
#                           Defaults to Main mode if not given.
#
# @param transforms         The transforms for the connection to the remote service generated by
#                           @ref isakmp_create_transforms_packet.
# @param transforms_num     The number of transforms passed in the transforms parameter.
# @param dhgroup            The Diffie Hellman Group for the connection to the remote service. Only
#                           used for the aggressive mode exchange_type. Currently supported values
#                           are 1,2,5,14,15,16,17,18 (MODP only). Defaults to 2 if not given.
# @param aggressive_mode_id The Aggressive mode ID for the connection to the remote service.
#                           Defaults to "vpngroup" if not given.
# @param initiator_spi      The Initiator SPI string to be used in the request. Will be
#                           automatically created if not given.
#
# @return The generated IKE/ISAKMP request packet as a raw string or NULL if no transforms parameter
#         (created by @ref isakmp_create_transforms_packet) or wrong / unsupported values for the
#         ipproto, exchange_type or dhgroup parameters were given.
#
function isakmp_create_request_packet( port, ipproto, exchange_type, transforms, transforms_num, dhgroup, aggressive_mode_id, initiator_spi ) {

  local_var port, ipproto, exchange_type, transforms, transforms_num, dhgroup, aggressive_mode_id, initiator_spi;
  local_var agressive_packet, sa_next_payload, main_packet;

  if( ! transforms ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_request_packet#-#transforms" );
    return NULL;
  }

  if( isnull( transforms_num ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_request_packet#-#transforms_num" );
    return NULL;
  }

  if( ipproto && ipproto !~ "^(tcp|udp)$" ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_request_packet(): Unsupported protocol '" + ipproto + "' given to 'ipproto' parameter. Currently supported: udp, tcp" );
    return NULL;
  }

  if( exchange_type && exchange_type !~ "^(Identity Protection \(Main Mode\)|Aggressive)$" ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_request_packet(): Unsupported exchange type '" + exchange_type + "' given to 'exchange_type' parameter. Currently supported: Identity Protection (Main Mode), Aggressive" );
    return NULL;
  }

  if( dhgroup && dhgroup !~ "^(1|2|5|14|15|16|17|18)$" ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#isakmp_create_request_packet(): Unsupported Diffie Hellman Group '" + dhgroup + "' given to 'dhgroup' parameter. Currently supported:1,2,5,14,15,16,17,18 (MODP only)" );
    return NULL;
  }

  if( ! port )
    port = 500;

  if( ! ipproto )
    ipproto = "udp";

  if( ! exchange_type )
    exchange_type = "Identity Protection (Main Mode)";

  if( ! dhgroup )
    dhgroup = 2;

  if( exchange_type == "Aggressive" ) {

    if( isnull( aggressive_mode_id ) )
      aggressive_mode_id = "vpngroup";

    aggressive_packet = isakmp_generate_aggressive_package( port:port, ipproto:ipproto, dhgroup:dhgroup, aggressive_mode_id:aggressive_mode_id );
    sa_next_payload = PAYLOADS_RAW["Key Exchange"];                                                 # Next payload: Key Exchange (4)
  } else {
    aggressive_packet = "";
    sa_next_payload = PAYLOADS_RAW["NONE"];                                                         # Next payload: NONE / No Next Payload (0)
  }

  if( ! initiator_spi )
    initiator_spi = rand_str( length:8, charset:"abcdefghiklmnopqrstuvwxyz0123456789" );

  # Main header
  main_packet  = raw_string( initiator_spi );                                                       # Initiator SPI
  main_packet += raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 );                      # Responder SPI
  main_packet += PAYLOADS_RAW["Security Association"];                                              # Next payload: Security Association (1)
  main_packet += VERSIONS_RAW["1.0"];                                                               # Version
  main_packet += EXCHANGE_TYPES_RAW[exchange_type];                                                 # Exchange type (Aggressive mode: Aggressive (4), Main mode: Identity Protection (Main Mode) (2))
  main_packet += raw_string( 0x00 );                                                                # Flags: (Encryption: Not encrypted, Commit: No commit, Authentication: No authentication)
  main_packet += raw_string( 0x00, 0x00, 0x00, 0x00 );                                              # Message ID
  main_packet += mkdword( 48 + strlen( transforms ) + strlen( aggressive_packet ) );                # Length

  # Payload: Security Association (1)
  main_packet += sa_next_payload;                                                                   # Next payload (Aggressive mode: Key Exchange, Main mode: None)
  main_packet += raw_string( 0x00 );                                                                # Reserved
  main_packet += mkword( 20 + strlen( transforms ) );                                               # Payload length
  main_packet += raw_string( 0x00, 0x00, 0x00, 0x01 );                                              # Domain of interpretation: IPSEC (1)
  main_packet += raw_string( 0x00, 0x00, 0x00, 0x01 );                                              # Situation: 00000001 (1 = Identity Only: True, 0. = Secrecy: False, 0.. = Integrity: False)

  # Payload: Proposal (2)
  main_packet += PAYLOADS_RAW["NONE"];                                                              # Next payload: NONE / No Next Payload (0)
  main_packet += raw_string( 0x00 );                                                                # Reserved
  main_packet += mkword( 8 + strlen( transforms ) );                                                # Payload length
  main_packet += raw_string( 0x01 );                                                                # Proposal number: 1
  main_packet += raw_string( 0x01 );                                                                # Protocol ID: ISAKMP (1)
  main_packet += raw_string( 0x00 );                                                                # SPI Size: 0
  main_packet += mkbyte( transforms_num );                                                          # Proposal transforms

  # Payload: Transform (3)
  main_packet += transforms;                                                                        # Transform payload

  if( exchange_type == "Aggressive" )
    main_packet += aggressive_packet;

  return main_packet;
}