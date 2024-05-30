# Copyright (C) 2019 Greenbone Networks GmbH
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

# Various functions and comments based on the Bluekeep Metasploit module cve_2019_0708_bluekeep.rb

# nb: Normally defined in byte_func.inc but openvas-nasl-lint will report
# those as not declared in this include.
# Little-endian byte order
BYTE_ORDER_LITTLE_ENDIAN = 1;
# Big-endian byte order
BYTE_ORDER_BIG_ENDIAN    = 2;

# @brief Builds a Virtual Channel PDU request
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/6c074267-1b32-4ceb-9496-2eb941a23e6b
# Virtual Channel PDU 2.2.6.1
#
# @param flags The channel control flags.
# @param data Variable-length data to be processed by the static virtual channel protocol handler.
#
# @return virtual channel PDU (channelPduHeader + virtualChannelData)
#
function rdp_build_virtual_channel_pdu_request( flags, data ) {

  local_var flags, data;
  local_var data_len;

  if( isnull( flags ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#flags#-#rdp_build_virtual_channel_pdu_request" );
    return FALSE;
  }

  if( isnull( data ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#rdp_build_virtual_channel_pdu_request" );
    return FALSE;
  }

  data_len = strlen( data );

  set_byte_order( BYTE_ORDER_LITTLE_ENDIAN );
  req = mkdword( data_len ) + # length
        mkdword( flags ) +    # flags
        data;

  return req;
}

# @brief Builds a TPKT Header and X.224 Data (DT) TPDU part of a RDP request
#        based on the length of the data to be send.
#        - TPKT Header, as specified in https://go.microsoft.com/fwlink/?LinkId=90541 section 8
#        - X.224 Class 0 Data TPDU, as specified in https://go.microsoft.com/fwlink/?LinkId=90588 section 13.7
#
# @param data Data to be used in a request.
#
# @return tpktHeader and x224Data part of a RDP request as a raw string.
#
function rdp_build_data_tpdu( data ) {

  local_var data;
  local_var tpkt_length, req;

  if( isnull( data ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#rdp_build_data_tpdu" );
    return FALSE;
  }

  tpkt_length = strlen( data ) + 7;

  set_byte_order( BYTE_ORDER_BIG_ENDIAN );
  req = raw_string( 0x03, 0x00 ) +      # TPKT Header version 03, reserved 0
        mkword( tpkt_length ) +         # TPKT length
        raw_string( 0x02, 0xf0, 0x80 ); # X.224 Data TPDU (2 bytes: 0xf0 = Data TPDU, 0x80 = end of transmission)

  return req;
}

# @brief Creates and returns a RDP connection request packet (Client X.224 Connection Request PDU).
#
# @param use_cookie If set to TRUE a "Cookie: mstshash=" cookie will be
#                   added to the request. This is required if the Windows
#                   System has "Allow Connections Only From Computers Running
#                   Remote Desktop With Network Level Authentication (More Secure)."
#                   enabled.
#
# @return The created connection request packet as a raw string.
#
# @todo See PROTOCOL_RDP
#
function rdp_create_pdu_negotiation_request( use_cookie ) {

  local_var use_cookie;
  local_var vt_strings, cookie, len_cookie, full_length, req;

  if( use_cookie ) {

    vt_strings = get_vt_strings();
    cookie = "Cookie: mstshash=" + vt_strings["lowercase_rand"];
    len_cookie = strlen( cookie );

    # The full length of the packet
    full_length = 4 +          # TPKT Header length
                  7 +          # X.224 packet length
                  len_cookie + # Length of the cookie created above
                  2 +          # Cookie terminator
                  8;           # Length of RDP_NEG_REQ

    # High-level:  https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/18a27ef9-6f9a-4501-b000-94b1fe3c2c10
    # packet dump: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/e78db616-689f-4b8a-8a99-525f7a433ee2
    req = raw_string( 0x03 ) +                  # TPKT Header: version = 3
          raw_string( 0x00 ) +                  # TPKT Header: Reserved = 0
          raw_string( 0x00 ) +                  # TPKT Header: Packet length - high part
          raw_string( full_length ) +           # TPKT Header: Packet length - low part (length for the whole packet)
          raw_string( full_length - 5 ) +       # X.224: Length indicator (full length of the packet minus TPKT header length and minus this field)
          raw_string( 0xe0 ) +                  # X.224: Type (high nibble) = 0xe = CR TPDU; credit (low nibble) = 0
          raw_string( 0x00, 0x00 ) +            # X.224: Destination reference = 0
          raw_string( 0x00, 0x00 ) +            # X.224: Source reference = 0
          raw_string( 0x00 ) +                  # X.224: Class and options = 0
          cookie +                              # The mstshash cookie
          raw_string( 0x0d, 0x0a ) +            # Cookie terminator sequence (\r\n)
          raw_string( 0x01 ) +                  # RDP_NEG_REQ::type (TYPE_RDP_NEG_REQ)
          raw_string( 0x00 ) +                  # RDP_NEG_REQ::flags (0)
          raw_string( 0x08, 0x00 ) +            # RDP_NEG_REQ::length (8 bytes)
          raw_string( 0x00, 0x00, 0x00, 0x00 ); # RDP_NEG_REQ: Requested protocols (PROTOCOL_RDP) # TODO: Make this dynamic for TLS via a passed function parameter
  } else {
    # found in amap (https://github.com/BlackArch/amap/blob/master/appdefs.trig#L60) and mapped to the
    # packet info in the microsoft docs above.
    req = raw_string( 0x03 ) +       # TPKT Header: version = 3
          raw_string( 0x00 ) +       # TPKT Header: Reserved = 0
          raw_string( 0x00 ) +       # TPKT Header: Packet length - high part
          raw_string( 0x0b ) +       # TPKT Header: Packet length - low part (11 bytes for the whole packet)
          raw_string( 0x06 ) +       # X.224: Length indicator (6 bytes for the next raw strings)
          raw_string( 0xe0 ) +       # X.224: Type (high nibble) = 0xe = CR TPDU; credit (low nibble) = 0
          raw_string( 0x00, 0x00 ) + # X.224: Destination reference = 0
          raw_string( 0x00, 0x00 ) + # X.224: Source reference = 0
          raw_string( 0x00 );        # X.224: Class and options = 0
  }

  return req;
}

# @brief Client MCS Connect Initial PDU with GCC Conference Create Request
#
# @param client_name The clientName to send in the request, defaults to "rdesktop"
#
# @return The MCS Connect Initial PDU request as a raw string
#
function rdp_create_pdu_connect_initial_request( client_name ) {

  local_var client_name;
  local_var len, req;

  if( isnull( client_name ) )
    client_name = "rdesktop";

  len = strlen( client_name );
  if( strlen( len ) > 15)
    client_name = ascii2unicode( data:substr( client_name, 0, 14 ) );
  else
    client_name = ascii2unicode( data:substr( client_name, 0, len ) );

  client_name += crap( length:32 - strlen( client_name ), data:raw_string( 0x00 ) );

  # High-level:  https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/db6713ee-1c0e-4064-a3b3-0fac30b4037b
  # packet dump: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/2610fcc7-3df4-4166-85bb-2c7ae21f6151

  req = raw_string( 0x7f, 0x65 ) +                     # T.125 Connect-Initial
        raw_string( 0x82, 0x01, 0xbe ) +               # Length
        raw_string( 0x04, 0x01, 0x01 ) +               # CallingDomainSelector: 1
        raw_string( 0x04, 0x01, 0x01 ) +               # CalledDomainSelector: 1
        raw_string( 0x01, 0x01, 0xff ) +               # UpwaredFlag: True
        # Connect-Initial: Target Parameters
        raw_string( 0x30, 0x20) +                      # TargetParamenters
        raw_string( 0x02, 0x02, 0x00, 0x22 ) +         # MaxChannelIds: 34
        raw_string( 0x02, 0x02, 0x00, 0x02 ) +         # MaxUserIDs: 2
        raw_string( 0x02, 0x02, 0x00, 0x00 ) +         # MaxTokenIds: 0
        raw_string( 0x02, 0x02, 0x00, 0x01 ) +         # NumPriorities: 1
        raw_string( 0x02, 0x02, 0x00, 0x00 ) +         # MinThroughput: 0
        raw_string( 0x02, 0x02, 0x00, 0x01 ) +         # MaxHeight: 1
        raw_string( 0x02, 0x02, 0xff, 0xff ) +         # MaxMCSPDUSize: 65535
        raw_string( 0x02, 0x02, 0x00, 0x02 ) +         # ProtocolVersion: 2
        # Connect-Intial: Minimum Parameters
        raw_string( 0x30, 0x20 ) +                     # MinimumParameters
        raw_string( 0x02, 0x02, 0x00, 0x01 ) +         # MaxChannelIds: 1
        raw_string( 0x02, 0x02, 0x00, 0x01 ) +         # MaxUserIDs: 1
        raw_string( 0x02, 0x02, 0x00, 0x01 ) +         # MaxTokenIds: 1
        raw_string( 0x02, 0x02, 0x00, 0x01 ) +         # NumPriorities: 1
        raw_string( 0x02, 0x02, 0x00, 0x00 ) +         # MinThroughput: 0
        raw_string( 0x02, 0x02, 0x00, 0x01 ) +         # MaxHeight: 1
        raw_string( 0x02, 0x02, 0x04, 0x20 ) +         # MaxMCSPDUSize: 1056
        raw_string( 0x02, 0x02, 0x00, 0x02 ) +         # ProtocolVersion: 2
        # Connect-Initial: Maximum Parameters
        raw_string( 0x30, 0x20 ) +                     # MaximumParameters
        raw_string( 0x02, 0x02, 0xff, 0xff ) +         # MaxChannelIds: 65535
        raw_string( 0x02, 0x02, 0xfc, 0x17 ) +         # MaxUserIDs: 64535
        raw_string( 0x02, 0x02, 0xff, 0xff ) +         # MaxTokenIds: 65535
        raw_string( 0x02, 0x02, 0x00, 0x01 ) +         # NumPriorities: 1
        raw_string( 0x02, 0x02, 0x00, 0x00 ) +         # MinThroughput: 0
        raw_string( 0x02, 0x02, 0x00, 0x01 ) +         # MaxHeight: 1
        raw_string( 0x02, 0x02, 0xff, 0xff ) +         # MaxMCSPDUSize: 65535
        raw_string( 0x02, 0x02, 0x00, 0x02 ) +         # ProtocolVersion: 2
        # Connect-Initial: UserData
        raw_string( 0x04, 0x82, 0x01, 0x4b ) +         # UserData, length 331
        # T.124 GCC Connection Data (ConnectData)- PER Encoding used
        raw_string( 0x00, 0x05 ) +                     # Object length
        raw_string( 0x00, 0x14, 0x7c, 0x00, 0x01 ) +   # object: OID 0.0.20.124.0.1 = Generic Conference Control
        raw_string( 0x81, 0x42 ) +                     # Length: 322 (Connect PDU)
        raw_string( 0x00, 0x08, 0x00, 0x10 ) +         # T.124 Connect PDU, Conference name 1
        raw_string( 0x00, 0x01, 0xc0, 0x00 ) +
        raw_string( 0x44, 0x75, 0x63, 0x61 ) +         # h221NonStandard: 'Duca' (client-to-server H.221 key)
        raw_string( 0x81, 0x34 ) +                     # Length: 308 (T.124 UserData section)
        # Client MCS Section - 2.2.1.3
        raw_string( 0x01, 0xc0 ) +                     # clientCoreData (TS_UD_CS_CORE) header - 2.2.1.3.2
        raw_string( 0xd8, 0x00 ) +                     # Length: 216 (includes header)
        raw_string( 0x04, 0x00, 0x08, 0x00 ) +         # Version: 8.4 (RDP 5.0 -> 8.1)
        raw_string( 0x20, 0x03 ) +                     # desktopWidth: 800
        raw_string( 0x58, 0x02 ) +                     # desktopHeigth: 600
        raw_string( 0x01, 0xca ) +                     # colorDepth: 8 bpp
        raw_string( 0x03, 0xaa ) +                     # SASSequence: 43523
        raw_string( 0x09, 0x04, 0x00, 0x00 ) +         # keyboardLayout: 1033 (English US)
        raw_string( 0x28, 0x0a, 0x00, 0x00 ) +         # clientBuild: 2600
        client_name +                                  # clientName (32 bytes)
        raw_string( 0x04, 0x00, 0x00, 0x00 ) +         # keyboardType: 4 (IBMEnhanced 101 or 102)
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +         # keyboadSubtype: 0
        raw_string( 0x0c, 0x00, 0x00, 0x00 ) +         # keyboardFunctionKey: 12
        # imeFileName (64 bytes)
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x01, 0xca ) +                     # postBeta2ColorDepth: 8 bpp
        raw_string( 0x01, 0x00 ) +                     # clientProductID: 1
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +         # serialNumber: 0
        raw_string( 0x18, 0x00 ) +                     # highColorDepth: 24 bpp
        raw_string( 0x07, 0x00 ) +                     # supportedColorDepths: flag (24 bpp | 16 bpp | 15 bpp )
        raw_string( 0x01, 0x00 ) +                     # earlyCapabilityFlags: 1 (RNS_UD_CS_SUPPORT_ERRINFO_PDU)
        # clientDigProductID (64 bytes)
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00 ) +                           # connectionType: 0
        raw_string( 0x00 ) +                           # pad1octet
        # serverSelectedProtocol - After negotiating TLS or CredSSP this value must
        # match the selectedProtocol value from the server's Negotiate Connection
        # confirm PDU that was sent before encryption was started.
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x04, 0xc0 ) +                     # clientClusterdata (TS_UD_CS_CLUSTER) header - 2.2.1.3.5
        raw_string( 0x0c, 0x00 ) +                     # Length: 12 (includes header)
        raw_string( 0x09, 0x00, 0x00, 0x00 ) +         # flags (REDIRECTION_SUPPORTED | REDIRECTION_VERSION3)
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +         # RedirectedSessionID
        raw_string( 0x02, 0xc0 ) +                     # clientSecuritydata (TS_UD_CS_SEC) header - 2.2.1.3.3
        raw_string( 0x0c, 0x00 ) +                     # Length: 12 (includes header)
        raw_string( 0x03, 0x00, 0x00, 0x00 ) +         # encryptionMethods: 3 (40 bit | 128 bit)
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +         # extEncryptionMethods (French locale only)
        raw_string( 0x03, 0xc0 ) +                     # clientNetworkData (TS_UD_CS_NET) - 2.2.1.3.4
        raw_string( 0x44, 0x00 ) +                     # Length: 68 (includes header)
        raw_string( 0x05, 0x00, 0x00, 0x00 ) +         # channelCount: 5
        # Channels definitions consist of a name (8 bytes) and options flags
        # (4 bytes). Names are up to 7 ANSI characters with null termination.
        raw_string( 0x63, 0x6c, 0x69, 0x70 ) +         # 'cliprdr'
        raw_string( 0x72, 0x64, 0x72, 0x00 ) +
        raw_string( 0xc0, 0xa0, 0x00, 0x00 ) +         # flags
        raw_string( 0x4d, 0x53, 0x5f, 0x54 ) +         # MS_T120'
        raw_string( 0x31, 0x32, 0x30, 0x00 ) +
        raw_string( 0x80, 0x80, 0x00, 0x00 ) +         # flags
        raw_string( 0x72, 0x64, 0x70, 0x73 ) +         # 'rdpsnd'
        raw_string( 0x6e, 0x64, 0x00, 0x00 ) +
        raw_string( 0xc0, 0x00, 0x00, 0x00 ) +         # flags
        raw_string( 0x73, 0x6e, 0x64, 0x64 ) +         # 'snddbg'
        raw_string( 0x62, 0x67, 0x00, 0x00 ) +
        raw_string( 0xc0, 0x00, 0x00, 0x00 ) +         # flags
        raw_string( 0x72, 0x64, 0x70, 0x64 ) +         # rdpdr'
        raw_string( 0x72, 0x00, 0x00, 0x00 ) +
        raw_string( 0x80, 0x80, 0x00, 0x00 );          # flags

  req = rdp_build_data_tpdu( data:req ) + req;

  return req;
}

# @brief Client MCS Erect Domain Request PDU
#
# @return The MCS Erect Domain Request PDU request as a raw string
#
function rdp_create_pdu_erect_domain_request() {

  local_var req;

  # High-level:   https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/04c60697-0d9a-4afd-a0cd-2cc133151a9c
  # packet dump:  https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/7afba26d-52a5-4153-b1df-e21eca3b1b4f

  req = raw_string( 0x04 ) +       # T.125 ErectDomainRequest
        raw_string( 0x01, 0x00 ) + # subHeight - length 1, value 0
        raw_string( 0x01, 0x00 );  # subInterval - length 1, value 0

  req = rdp_build_data_tpdu( data:req ) + req;

  return req;
}

# @brief Client MCS Attach User Request PDU
#
# @return The MCS Attach User Request PDU request as a raw string
#
function rdp_create_pdu_attach_user_request() {

  local_var req;

  # High-level:  https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/f5d6a541-9b36-4100-b78f-18710f39f247
  # packet dump: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/5125dd86-1a99-46cd-bcae-d1c3c083eeb0

  req = raw_string( 0x28 ); # T.125 AttachUserRequest

  req = rdp_build_data_tpdu( data:req ) + req;

  return req;
}

# @brief Client MCS Channel Join Request PDU
#
# @param user1 User
# @param channel_id Channel ID
# @param debug
#
# @return The created Channel join request packet as a raw string.
#
function rdp_create_pdu_channel_request( user1, channel_id, debug ) {

  local_var user1, channel_id, debug;
  local_var req;

  if( isnull( user1 ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#user1#-#rdp_create_pdu_channel_request" );
    return FALSE;
  }

  if( isnull( channel_id ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#channel_id#-#rdp_create_pdu_channel_request" );
    return FALSE;
  }

  # High-level:  https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/64564639-3b2d-4d2c-ae77-1105b4cc011b
  # packet dump: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/388bac6b-39ff-4d85-8b72-4d11506e489b

  if( debug ) display( "-- rdp_create_pdu_channel_request --\nChosen userid:\n", hexdump( ddata:user1 ), "\nChosen channel_id: ", channel_id );

  req = raw_string( 0x38 ) + # ChannelJoin request
        user1 + dec2hex( num:channel_id );

  req = rdp_build_data_tpdu( data:req ) + req;

  return req;
}

# @brief Client Security Exchange PDU - 2.2.1.10
#
# @param client_rand Client random which will be encrypted with the public key of the server
#
# @param public_exponent Public exponent of the server
#
# @param modulus Modulus from the server
#
# @param bitlen Bitlength of the public key
#
# @return The created client security exchange packet with the encrypted client random
#
function rdp_create_pdu_security_exchange( client_rand, public_exponent, modulus, bitlen ) {

  local_var client_rand, public_exponent, modulus, bitlen;
  local_var enc, userdata_len, userdata_len_low, userdata_len_high, flags;
  local_var pkt_len, reverse_bitlen, req;

  if( isnull( client_rand ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#client_rand#-#rdp_create_pdu_security_exchange" );
    return FALSE;
  }

  if( isnull( public_exponent ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#public_exponent#-#rdp_create_pdu_security_exchange" );
    return FALSE;
  }

  if( isnull( modulus ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#modulus#-#rdp_create_pdu_security_exchange" );
    return FALSE;
  }

  if( isnull( bitlen ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#bitlen#-#rdp_create_pdu_security_exchange" );
    return FALSE;
  }

  # High-level:  https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/9cde84cd-5055-475a-ac8b-704db419b66f
  # packet dump: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/b6075470-bbdd-465a-b6d9-ef15941ae358

  # client_rand needs to be reversed first for the RSA function
  client_rand = reverse_blob( blob:client_rand );

  # Note: Change 'pad: "FALSE"' to 'pad: FALSE' once GVM 9 is retired!
  enc = rsa_public_encrypt( data:client_rand, e:public_exponent, n:modulus, pad:"FALSE" );
  enc = reverse_blob( blob:enc );

  bitlen += 8; # Pad with size of TS_SECURITY_PACKET header

  userdata_len = bitlen + 8;
  userdata_len_low = dec2hex( num:userdata_len & 0xff );
  userdata_len_high = userdata_len / 256;
  flags = dec2hex( num:0x80 | userdata_len_high );
  pkt_len = userdata_len + 15;

  set_byte_order( BYTE_ORDER_BIG_ENDIAN );
  reverse_bitlen = reverse_blob( blob:mkdword( bitlen ) );

  req = raw_string( 0x64 ) +       # T.125 sendDataRequest
        raw_string( 0x00, 0x08 ) + # Initiator UserID
        raw_string( 0x03, 0xeb ) + # ChannelID = 1003
        raw_string( 0x70 ) +       # dataPriority = high, segmentation = begin | end
        flags +
        userdata_len_low +         # UserData length
        raw_string( 0x01, 0x00 ) + # securityHeader flags
        raw_string( 0x00, 0x00 ) + # securityHeader flagsHi
        reverse_bitlen +           # securityPkt length
        enc +                      # encrypted client random
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ); # 8 bytes rear padding (always present)

  req = rdp_build_data_tpdu( data:req ) + req;

  return req;
}

# @brief Client Info PDU
#
# @param username Username to connect with
# @param domain_name Domain to connect with
# @param ip_addr The client IP address
#
# @return The created Client Info PDU as a raw string
#
function rdp_create_pdu_client_info_request( user_name, domain_name, ip_addr ) {

  local_var user_name, domain_name, ip_addr;
  local_var vt_strings, username_len, domain_name_len, ip_addr_len, req;

  # High-level:  https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/772d618e-b7d6-4cd0-b735-fa08af558f9d
  # packet dump: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/ac6dc9ab-6f32-471e-8374-f80caab50069

  if( ! user_name ) {
    vt_strings = get_vt_strings();
    user_name = vt_strings["ping_string"];
  }

  user_name = ascii2unicode( data:user_name );
  user_name = substr( user_name, 0, 20 );
  user_name_len = strlen( user_name );

  if( ! domain_name ) {
    vt_strings = get_vt_strings();
    domain_name = vt_strings["ping_string"];
  }

  domain_name = ascii2unicode( data:domain_name );
  domain_name = substr( domain_name, 0, 24 );
  domain_name_len = strlen( domain_name );

  if( ! ip_addr )
    ip_addr = this_host();

  ip_addr = ascii2unicode( data:ip_addr ) + raw_string( 0x00, 0x00 );
  ip_addr_len = strlen( ip_addr );

  set_byte_order( BYTE_ORDER_LITTLE_ENDIAN );
  req = raw_string( 0x00, 0x00, 0x00, 0x00 ) +                   # CodePage
        raw_string( 0x33, 0x01, 0x00, 0x00 ) +                   # flags - INFO_MOUSE, INFO_DISABLECTRLALTDEL, INFO_UNICODE, INFO_MAXIMIZESHELL, INFO_ENABLEWINDOWSKEY
        mkword( domain_name_len ) +                              # cbDomain (length value) - EXCLUDES null terminator
        mkword( user_name_len ) +                                # cbUserName (length value) - EXCLUDES null terminator
        raw_string( 0x00, 0x00 ) +                               # cbPassword (length value)
        raw_string( 0x00, 0x00 ) +                               # cbAlternateShell (length value)
        raw_string( 0x00, 0x00 ) +                               # cbWorkingDir (length value)
        raw_string( domain_name ) +                              # Domain
        raw_string( 0x00, 0x00 ) +                               # Domain null terminator, EXCLUDED from value of cbDomain
        raw_string( user_name ) +                                # UserName
        raw_string( 0x00, 0x00 ) +                               # UserName null terminator, EXCLUDED FROM value of cbUserName
        raw_string( 0x00, 0x00 ) +                               # Password - empty
        raw_string( 0x00, 0x00 ) +                               # AlternateShell - empty
        raw_string( 0x00, 0x00 ) +                               # WorkingDir - empty
        # TS_EXTENDED_INFO_PACKET - 2.2.1.11.1.1.1
        raw_string( 0x02, 0x00 ) +                               # clientAddressFamily - AF_INET # TODO: detect and set dynamically
        mkword( ip_addr_len ) +                                  # cbClientAddress (length value) - INCLUDES terminator ... for reasons.
        raw_string( ip_addr ) +                                  # clientAddress (unicode + null terminator (unicode)
        raw_string( 0x3c, 0x00 ) +                               # cbClientDir (length value): 60
        # clientDir - 'C:\WINNT\System32\mstscax.dll' + null terminator
        raw_string( 0x3c, 0x00, 0x43, 0x00, 0x3a, 0x00, 0x5c, 0x00, 0x57, 0x00, 0x49, 0x00, 0x4e, 0x00, 0x4e, 0x00,
                    0x54, 0x00, 0x5c, 0x00, 0x53, 0x00, 0x79, 0x00, 0x73, 0x00, 0x74, 0x00, 0x65, 0x00, 0x6d, 0x00,
                    0x33, 0x00, 0x32, 0x00, 0x5c, 0x00, 0x6d, 0x00, 0x73, 0x00, 0x74, 0x00, 0x73, 0x00, 0x63, 0x00,
                    0x61, 0x00, 0x78, 0x00, 0x2e, 0x00, 0x64, 0x00, 0x6c, 0x00, 0x6c, 0x00, 0x00, 0x00 ) +
        # clientTimeZone - TS_TIME_ZONE struct - 172 bytes
        # These are the default values for rdesktop
        raw_string( 0xa4, 0x01, 0x00, 0x00 ) + # Bias
        # StandardName - 'GTB,normaltid'
        raw_string( 0x47, 0x00, 0x54, 0x00, 0x42, 0x00, 0x2c, 0x00, 0x20, 0x00, 0x6e, 0x00, 0x6f, 0x00, 0x72, 0x00,
                    0x6d, 0x00, 0x61, 0x00, 0x6c, 0x00, 0x74, 0x00, 0x69, 0x00, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00,
                    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                    0x00, 0x00, 0x0a, 0x00, 0x00, 0x00, 0x05, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) + # StandardDate - Oct 5
        raw_string( 0x00, 0x00, 0x00, 0x00 ) + # StandardBias
        # DaylightName - 'GTB,sommartid'
        raw_string( 0x47, 0x00, 0x54, 0x00, 0x42, 0x00, 0x2c, 0x00, 0x20, 0x00, 0x73, 0x00, 0x6f, 0x00, 0x6d, 0x00,
                    0x6d, 0x00, 0x61, 0x00, 0x72, 0x00, 0x74, 0x00, 0x69, 0x00, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00,
                    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                    0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x05, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) + # DaylightDate - Mar 3
        raw_string( 0xc4, 0xff, 0xff, 0xff ) + # DaylightBias
        raw_string( 0x00, 0x00, 0x00, 0x00 ) + # clientSessionId
        raw_string( 0x27, 0x00, 0x00, 0x00 ) + # performanceFlags
        raw_string( 0x00, 0x00 );              # cbAutoReconnectCookie

  return req;
}

# @brief Build a Share Control Header
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/73d01865-2eae-407f-9b2c-87e31daac471
# Share Control Header - TS_SHARECONTROLHEADER - 2.2.8.1.1.1.1
#
# @param type PDU type
# @param data Data to add to the header
#
# @return Share Control Header created with data append as a raw string
#
function rdp_build_share_control_header( type, data ) {

  local_var type, data;
  local_var total_len, req;

  total_len = strlen( data ) + 6;

  set_byte_order( BYTE_ORDER_LITTLE_ENDIAN );
  req = mkword( total_len ) +      # totalLength - includes all headers
        mkword( type ) +           # pduType - flags 16 bit, unsigned
        raw_string( 0xf1, 0x03 ) + # PDUSource: 0x03f1 = 1009
        data;

  return req;
}

# @brief Build a Share Data Header
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/4b5d4c0d-a657-41e9-9c69-d58632f46d31
# Share Data Header - TS_SHAREDATAHEADER - 2.2.8.1.1.1.2
#
# @param type PDU type
# @param data Data to add to the header
#
# @return Share Data Header created with data append as a raw string
#
function rdp_build_share_data_header( type, data ) {

  local_var type, data;
  local_var uncompressed_len, req;

  uncompressed_len = strlen( data ) + 4;

  set_byte_order( BYTE_ORDER_LITTLE_ENDIAN );
  req = raw_string( 0xea, 0x03, 0x01, 0x00 ) + # shareId: 66538
        raw_string( 0x00 ) +                   # pad1
        raw_string( 0x01 ) +                   # streamID: 1
        mkword( uncompressed_len ) +           # uncompressedLength - 16 bit, unsigned int
        dec2hex( num:type ) +                  # pduType2 - 8 bit, unsigned int - 2.2.8.1.1.2
        raw_string( 0x00 ) +                   # compressedType: 0
        raw_string( 0x00, 0x00 ) +             # compressedLength: 0
        data;

  return req;
}

# @brief Build a Client Control (Cooperate) PDU
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/9d1e1e21-d8b4-4bfd-9caf-4b72ee91a713
# Control Cooperate - TC_CONTROL_PDU 2.2.1.15
#
# @return The Client Control (Cooperate) PDU request as a raw string
#
function rdp_build_pdu_client_control_cooperate() {

  local_var pdu, data_header;

  pdu = raw_string( 0x04, 0x00 ) +            # action: 4 - CTRLACTION_COOPERATE
        raw_string( 0x00, 0x00 ) +            # grantId: 0
        raw_string( 0x00, 0x00, 0x00, 0x00 ); # controlId: 0

  # pduType2 = 0x14 = 20 - PDUTYPE2_CONTROL
  data_header = rdp_build_share_data_header( type:0x14, data:pdu );

  # type = 0x17 = TS_PROTOCOL_VERSION | PDUTYPE_DATAPDU
  return rdp_build_share_control_header( type:0x17, data:data_header );
}

# @brief Build a Client Control (Request Control) PDU
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/4f94e123-970b-4242-8cf6-39820d8e3d35
# Control Request - TC_CONTROL_PDU 2.2.1.16
#
# @return The Client Control (Request Control) PDU request as a raw string
#
function rdp_create_pdu_client_control_request() {

  local_var pdu, data_header;

  pdu = raw_string( 0x01, 0x00 ) +            # action: 1 - CTRLACTION_REQUEST_CONTROL
        raw_string( 0x00, 0x00 ) +            # grantId: 0
        raw_string( 0x00, 0x00, 0x00, 0x00 ); # controlId: 0

  # pduType2 = 0x14 = 20 - PDUTYPE2_CONTROL
  data_header = rdp_build_share_data_header( type:0x14, data:pdu );

  # type = 0x17 = TS_PROTOCOL_VERSION | PDUTYPE_DATAPDU
  return rdp_build_share_control_header( type:0x17, data:data_header );
}

# @brief Build a Font List PDU
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/7067da0d-e318-4464-88e8-b11509cf0bd9
# Client Font List - TS_FONT_LIST_PDU - 2.2.1.18
#
# @return The Font List PDU request as a raw string
#
function rdp_create_pdu_client_font_list_request() {

  local_var pdu, data_header;

  pdu = raw_string( 0x00, 0x00 ) + # numberFonts: 0
        raw_string( 0x00, 0x00 ) + # totalNumberFonts: 0
        raw_string( 0x03, 0x00 ) + # listFlags: 3 (FONTLIST_FIRST | FONTLIST_LAST)
        raw_string( 0x32, 0x00 );  # entrySize: 50

  # pduType2 = 0x27 = 29 -  PDUTYPE2_FONTLIST
  data_header = rdp_build_share_data_header( type:0x27, data:pdu );

  # type = 0x17 = TS_PROTOCOL_VERSION | PDUTYPE_DATAPDU
  return rdp_build_share_control_header( type:0x17, data:data_header );
}

# @brief Build a Server Synchronize PDU
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/5186005a-36f5-4f5d-8c06-968f28e2d992
# Client Synchronize - TS_SYNCHRONIZE_PDU - 2.2.1.19 /  2.2.14.1
#
# @param target_user The MCS channel ID of the target user
#
# @return The Server Synchronize PDU request as a raw string
#
function rdp_create_pdu_client_synchronize_request( target_user ) {

  local_var target_user;
  local_var pdu, data_header;

  if( isnull( target_user ) )
    target_user = 0;

  set_byte_order( BYTE_ORDER_LITTLE_ENDIAN );
  pdu = raw_string( 0x01, 0x00 ) + # messageType: 1 SYNCMSGTYPE_SYNC
        mkword( target_user );     # targetUser, 16 bit, unsigned.

  # pduType2 = 0x1f = 31 - PDUTYPE2_SCYNCHRONIZE
  data_header = rdp_build_share_data_header( type:0x1f, data:pdu );

  # type = 0x17 = TS_PROTOCOL_VERSION | PDUTYPE_DATAPDU
  return rdp_build_share_control_header( type:0x17, data:data_header );
}

# @brief Build Client Confirm Active PDU
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/4e9722c3-ad83-43f5-af5a-529f73d88b48
# Confirm Active PDU Data - TS_CONFIRM_ACTIVE_PDU - 2.2.1.13.2.1
#
# @return The Confirm Active PDU request as a raw string
#
function rdp_create_pdu_client_confirm_active_request() {

  local_var pdu;

  # High-level:  https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/4c3c2710-0bf0-4c54-8e69-aff40ffcde66
  # packet dump: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/54765b0a-39d4-4746-92c6-8914934023da

  pdu = raw_string( 0xea, 0x03, 0x01, 0x00 ) +             # shareId: 66538
        raw_string( 0xea, 0x03 ) +                         # originatorId
        raw_string( 0x06, 0x00 ) +                         # lengthSourceDescriptor: 6
        raw_string( 0x8e, 0x01 ) +                         # lengthCombinedCapabilities: 398
        raw_string( 0x4d, 0x53, 0x54, 0x53, 0x43, 0x00 ) + # SourceDescriptor: 'MSTSC'
        raw_string( 0x0e, 0x00 ) +                         # numberCapabilities: 14
        raw_string( 0x00, 0x00 ) +                         # pad2Octets
        raw_string( 0x01, 0x00 ) +                         # capabilitySetType: 1 - TS_GENERAL_CAPABILITYSET
        raw_string( 0x18, 0x00 ) +                         # lengthCapability: 24
        raw_string( 0x01, 0x00, 0x03, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x0d, 0x04, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x02, 0x00 ) +                         # capabilitySetType: 2 - TS_BITMAP_CAPABILITYSET
        raw_string( 0x1c, 0x00 ) +                         # lengthCapability: 28
        raw_string( 0x10, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x20, 0x03, 0x58, 0x02, 0x00, 0x00, 0x01, 0x00 ) +
        raw_string( 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00 ) +
        raw_string( 0x03, 0x00 ) +                         # capabilitySetType: 3 - TS_ORDER_CAPABILITYSET
        raw_string( 0x58, 0x00 ) +                         # lengthCapability: 88
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x14, 0x00, 0x00, 0x00, 0x01, 0x00, 0x47, 0x01, 0x2a, 0x00 ) +
        raw_string( 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0x01, 0x01, 0x00, 0x01, 0x01, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0x01, 0x00, 0x00, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0xa1, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x84, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0xe4, 0x04, 0x00, 0x00, 0x13, 0x00, 0x28, 0x00, 0x00, 0x00, 0x00, 0x03, 0x78, 0x00, 0x00, 0x00 ) +
        raw_string( 0x78, 0x00, 0x00, 0x00, 0x50, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x08, 0x00 ) +                         # capabilitySetType: 8 - TS_POINTER_CAPABILITYSET
        raw_string( 0x0a, 0x00 ) +                         # lengthCapability: 10
        raw_string( 0x01, 0x00, 0x14, 0x00, 0x14, 0x00 ) +
        raw_string( 0x0a, 0x00 ) +                         # capabilitySetType: 10 - TS_COLORTABLE_CAPABILITYSET
        raw_string( 0x08, 0x00 ) +                         # lengthCapability: 8
        raw_string( 0x06, 0x00, 0x00, 0x00 ) +
        raw_string( 0x07, 0x00 ) +                         # capabilitySetType: 7 - TSWINDOWACTIVATION_CAPABILITYSET
        raw_string( 0x0c, 0x00 ) +                         # lengthCapability: 12
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x05, 0x00 ) +                         # capabilitySetType: 5 - TS_CONTROL_CAPABILITYSET
        raw_string( 0x0c, 0x00 ) +                         # lengthCapability: 12
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x02, 0x00 ) +
        raw_string( 0x09, 0x00 ) +                         # capabilitySetType: 9 - TS_SHARE_CAPABILITYSET
        raw_string( 0x08, 0x00 ) +                         # lengthCapability: 8
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x0f, 0x00 ) +                         # capabilitySetType: 15 - TS_BRUSH_CAPABILITYSET
        raw_string( 0x08, 0x00 ) +                         # lengthCapability: 8
        raw_string( 0x01, 0x00, 0x00, 0x00 ) +
        raw_string( 0x0d, 0x00 ) +                         # capabilitySetType: 13 - TS_INPUT_CAPABILITYSET
        raw_string( 0x58, 0x00 ) +                         # lengthCapability: 88
        raw_string( 0x01, 0x00, 0x00, 0x00, 0x09, 0x04, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x0c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x00, 0x00, 0x00, 0x00 ) +
        raw_string( 0x0c, 0x00 ) +                         # capabilitySetType: 12 - TS_SOUND_CAPABILITYSET
        raw_string( 0x08, 0x00 ) +                         # lengthCapability: 8
        raw_string( 0x01, 0x00, 0x00, 0x00 ) +
        raw_string( 0x0e, 0x00 ) +                         # capabilitySetType: 14 - TS_FONT_CAPABILITYSET
        raw_string( 0x08, 0x00 ) +                         # lengthCapability: 8
        raw_string( 0x01, 0x00, 0x00, 0x00 ) +
        raw_string( 0x10, 0x00 ) +                         # capabilitySetType: 16 - TS_GLYPHCAChE_CAPABILITYSET
        raw_string( 0x34, 0x00 ) +                         # lengthCapability: 52
        raw_string( 0xfe, 0x00, 0x04, 0x00, 0xfe, 0x00, 0x04, 0x00, 0xfe, 0x00, 0x08, 0x00, 0xfe, 0x00, 0x08, 0x00 ) +
        raw_string( 0xfe, 0x00, 0x10, 0x00, 0xfe, 0x00, 0x20, 0x00, 0xfe, 0x00, 0x40, 0x00, 0xfe, 0x00, 0x80, 0x00 ) +
        raw_string( 0xfe, 0x00, 0x00, 0x01, 0x40, 0x00, 0x00, 0x08, 0x00, 0x01, 0x00, 0x01, 0x02, 0x00, 0x00, 0x00 );

  # type = 0x13 = TS_PROTOCOL_VERSION | PDUTYPE_CONFIRMACTIVEPDU
  return rdp_build_share_control_header( type:0x13, data:pdu );
}

# @brief Build a Synchronize Event request
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/ff7f06f8-0dcf-4c8d-be1f-596ae60c4396
# Client Input Event Data - TS_INPUT_PDU_DATA - 2.2.8.1.1.3.1
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/6c5d0ef9-4653-4d69-9ba9-09ba3acd660f
# Synchronize Event (TS_SYNC_EVENT) - 2.2.8.1.1.3.1.1.5
#
# @return The Synchronize Event request as a raw string
#
function rdp_create_pdu_client_input_event_sychronize_request() {

  local_var pdu, data_header;

  pdu = raw_string( 0x01, 0x00 ) +             # numEvents: 1
        raw_string( 0x00, 0x00 ) +             # pad2Octets
        raw_string( 0x00, 0x00, 0x00, 0x00 ) + # eventTime
        raw_string( 0x00, 0x00 ) +             # messageType: 0 - INPUT_EVENT_SYNC
        # TS_SYNC_EVENT 202.8.1.1.3.1.1.5
        raw_string( 0x00, 0x00 ) +             # pad2Octets
        raw_string( 0x00, 0x00, 0x00, 0x00 );  # toggleFlags

  # pduType2 = 0x1c = 28 - PDUTYPE2_INPUT
  data_header = rdp_build_share_data_header( type:0x1c, data:pdu );

  # type = 0x17 = TS_PROTOCOL_VERSION | PDUTYPE_DATAPDU
  return rdp_build_share_control_header( type:0x17, data:data_header );
}

# @brief Server MCS Connect Response PDU with GCC Conference Create Response
#
# @param data Data to parse
# @param debug
#
# @return An array containing the extracted data, NULL if no data could be extracted, FALSE in case of an error.
#
#         ret_array["server_random"]    = server random (used for RC4 key generation)
#         ret_array["public_exponent"]  = public exponend (used for RSA key generation)
#         ret_array["modulus"]          = modulus (used for RSA key generation)
#         ret_array["rsa_magic"]        = RSA signature (e.g. RSA1)
#         ret_array["bitlen"]           = RSA key length (e.g. 256)
#
function rdp_parse_serverdata( data, debug ) {

  local_var data, debug;
  local_var len, rdp_package, pos, header_type, header_length, security_header;
  local_var server_random, public_exponent, rsa_magic, bitlen, modulus, ret_array;

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#rdp_parse_serverdata" );
    return FALSE;
  }

  # High-Level:  https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/927de44c-7fe8-4206-a14f-e5517dc24b1c
  # packet dump: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/d23f7725-876c-48d4-9e41-8288896a19d3

  len = strlen( data );
  # TODO: Define a min. length where we should jump out, the 256 is just an assumed number.
  # Have seen 341 bytes so far...
  if( len < 256 )
    return NULL;

  # nb: The RDP packet we're looking for here starts at offset 73
  rdp_package = substr( data, 73, len );
  pos = 0;

  while( pos < strlen( rdp_package ) ) {

    header_type = rdp_package[pos] + rdp_package[pos + 1];
    if( debug ) display( "Header-Type: ", hexdump( ddata:header_type ) );

    # nb: Length is little-endian
    header_length = getword( blob:rdp_package[pos + 3] + rdp_package[pos + 2] );
    if( debug ) display( "Header-Length: ", header_length );

    if( header_type == raw_string( 0x02, 0x0c ) ) {

      security_header = TRUE;

      if( debug ) display( "Security header received" );

      server_random = substr( rdp_package, pos + 20, pos + 51 );
      if( debug ) display( "Server Random:\n", hexdump( ddata:server_random ) );

      public_exponent = substr( rdp_package, pos + 84, pos + 87 );
      if( debug ) display( "Server Exponent:\n", hexdump( ddata:public_exponent ) );

      rsa_magic = substr( rdp_package, pos + 68, pos + 71 );
      if( debug ) display( "RSA magic:\n", hexdump( ddata:rsa_magic ) );

      if( rsa_magic != "RSA1" ) {
        if( debug ) display( "Unsupported RSA magic received, only RSA1 is currently supported." );
        return NULL;
      }

      bitlen = getdword( blob:rdp_package[pos + 75] + rdp_package[pos + 74] + rdp_package[pos + 73] + rdp_package[pos + 72] ) - 8;
      if( debug ) display( "RSA bitlen:\n", bitlen );

      modulus = substr( rdp_package, pos + 88, pos + 87 + bitlen );
      if( debug ) display( "Server Modulus:\n", hexdump( ddata:modulus ) );
    }
    pos += header_length;
  }

  if( ! security_header )
    return NULL;

  ret_array["server_random"] = server_random;
  ret_array["public_exponent"] = reverse_blob( blob:public_exponent );
  ret_array["modulus"] = reverse_blob( blob:modulus );
  ret_array["rsa_magic"] = rsa_magic;
  ret_array["bitlen"] = bitlen;

  return ret_array;
}

# @brief Creates a packet with RC4 encrypted data including needed headers, checksum etc.
#
# @param data Original plaintext data in the packet
# @param channel_id The Channel ID to use
# @param client_info (optional) TRUE if packet is a Client Info Request, otherwise FALSE
# @param rdp_sec (optional) TRUE if Standard RDP Security should be used, otherwise FALSe
# @param enc_data The RC4 encrypted data
# @param hmackey The calculated MAC checksum of the data to send
#
# @return RC4 encrypted packet as a raw string
#
function rdp_build_pkt( data, channel_id, client_info, rdp_sec, enc_data, hmackey ) {

  local_var data, channel_id, client_info, rdp_sec, enc_data, hmackey;
  local_var flags, pdu, user_data_len, udl_with_flags, req;

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#rdp_build_pkt" );
    return FALSE;
  }

  if( ! enc_data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#enc_data#-#rdp_build_pkt" );
    return FALSE;
  }

  if( ! hmackey ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#hmackey#-#rdp_build_pkt" );
    return FALSE;
  }

  if( isnull( client_info ) )
    client_info = FALSE;

  if( isnull( rdp_sec ) )
    rdp_sec = TRUE; # nb: Assume Standard RDP Security

  flags = 0;

  if( rdp_sec )
    flags = flags | 8; # Set SEC_ENCRYPT, 0b1000

  if( client_info )
    flags = flags | 64; # if client_info # Set SEC_INFO_PKT, 0b1000000

  set_byte_order( BYTE_ORDER_LITTLE_ENDIAN );
  flags = mkword( flags );

  if ( ! channel_id )
    channel_id = raw_string( 0x03, 0xeb );

  pdu = "";

  # TS_SECURITY_HEADER - 2.2.8.1.1.2.1
  # Send when the packet is encrypted w/ Standard RDP Security and in all Client Info PDUs
  if( client_info || rdp_sec ) {
    pdu += flags;                    # flags  "\x48\x00" = SEC_INFO_PKT | SEC_ENCRYPT
    pdu += raw_string( 0x00, 0x00 ); # flagsHi
  }

  if( rdp_sec ) {
    pdu += substr( rdp_hmac( mac_salt_key:hmackey, data_content:data ), 0, 7 );

    # Encrypt the payload with RDP Standard Encryption
    # TODO: NASL rc4_encrypt currently can't encrypt the data for our purposes so
    # the data needs to be dissected in the VT calling this function.
    pdu += enc_data;
  } else {
    pdu += data;
  }

  user_data_len = strlen( pdu );
  udl_with_flag = dec2hex( num:0x8000 | user_data_len );

  req = raw_string( 0x64 ) +       # sendDataRequest
        raw_string( 0x00, 0x08 ) + # intiator userId .. TODO: for a functional client this isn't static
        channel_id +               # channelId
        raw_string( 0x70 ) +       # dataPriority
        udl_with_flag +
        pdu;

  req = rdp_build_data_tpdu( data:req ) + req;

  return req;
}

# @brief MAC checksum calculation of data with a given salt
#
# @param mac_salt_key MAC salt key
# @param data_content Data to calculate the MAC on
#
# @return The calculated MAC checksum as a raw string
#
function rdp_hmac( mac_salt_key, data_content ) {

  # High-level: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/7c61b54e-f6cd-4819-a59a-daf200f6bf94

  local_var mac_salt_key, data_content;
  local_var pad1, pad2, data_len, sha1, md5;

  # Pad1 = 0x36 repeated 40 times to give 320 bits
  # Pad2 = 0x5C repeated 48 times to give 384 bits
  pad1 = crap( data:raw_string( 0x36 ), length:40 );
  pad2 = crap( data:raw_string( 0x5c ), length:48 );

  set_byte_order( BYTE_ORDER_LITTLE_ENDIAN );
  data_len = mkdword( strlen( data_content ) );

  # SHAComponent = SHA(MACKeyN + Pad1 + DataLength + Data)
  sha1 = SHA1( mac_salt_key + pad1 + data_len + data_content );

  # MACSignature = First64Bits(MD5(MACKeyN + Pad2 + SHAComponent))
  md5 = MD5( mac_salt_key + pad2 + sha1 );

  return md5;
}

# @brief Creates a salted hash from the client and server random as used for initial RC4 key generation.
#
# @param s_bytes S component
# @param i_bytes I component
# @param client_rand Client random
# @param server_rand Server random
#
# @return The calculated salted hash
#
function rdp_salted_hash( s_bytes, i_bytes, client_rand, server_rand ) {

  # High-level: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/705f9542-b0e3-48be-b9a5-cf2ee582607f
  # SaltedHash(S, I) = MD5(S + SHA(I + S + ClientRandom + ServerRandom))

  local_var s_bytes, i_bytes, client_rand, server_rand;
  local_var input, sha1;

  input = i_bytes + s_bytes + client_rand + server_rand;

  sha1 = SHA1( input );

  input = s_bytes + sha1;

  return MD5( input );
}

# @brief Creates the final hash used in the initial RC4 key generation.
#
# @param k K component
# @param client_rand Client random
# @param server_rand Server random
#
# @return The calculated final hash
#
function rdp_final_hash( k, client_rand, server_rand ) {

  local_var k, client_rand, server_rand;

  # FinalHash(K) = MD5(K + ClientRandom + ServerRandom)

  return MD5( k + client_rand + server_rand );
}

# @brief Calculates the RC4 keys from the server_random and client_random
#
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/705f9542-b0e3-48be-b9a5-cf2ee582607f
#
# @param client_random Client random
# @param server_random Server random
# @param debug
#
# @return Array containing the calculated keys. FALSE in case of an error.
#
#         ret_array["initial_client_encryptkey_128"] = Initial RC4 key for encryption
#         ret_array["initial_client_decryptkey_128"] = Initial RC4 key for decryption
#         ret_array["mac_key"]                       = Key for MAC checksum generation
#         ret_array["sess_key_blob"]                 = SessionBlob from which the keys are derived (TODO: might not be used external)
#
function rdp_calculate_rc4_keys( client_rand, server_rand, debug ) {

  local_var client_rand, server_rand, debug;
  local_var pre_master_secret, master_secret, sess_key_blob;
  local_var initial_client_decryptkey_128, initial_client_encryptkey_128, mac_key, ret_array;

  if( ! client_rand ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#client_rand#-#rdp_calculate_rc4_keys" );
    return FALSE;
  }

  if( ! server_rand ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#server_rand#-#rdp_calculate_rc4_keys" );
    return FALSE;
  }

  # preMasterSecret = First192Bits(ClientRandom) + First192Bits(ServerRandom)
  pre_master_secret = substr( client_rand, 0, 23 ) + substr( server_rand, 0, 23 );

  # PreMasterHash(I) = SaltedHash(preMasterSecret, I)
  # MasterSecret = PreMasterHash(0x41) + PreMasterHash(0x4242) + PreMasterHash(0x434343)
  master_secret = rdp_salted_hash( s_bytes:pre_master_secret, i_bytes:"A", client_rand:client_rand, server_rand:server_rand ) +
                  rdp_salted_hash( s_bytes:pre_master_secret, i_bytes:"BB", client_rand:client_rand, server_rand:server_rand ) +
                  rdp_salted_hash( s_bytes:pre_master_secret, i_bytes:"CCC", client_rand:client_rand, server_rand:server_rand );

  # MasterHash(I) = SaltedHash(MasterSecret, I)
  # SessionKeyBlob = MasterHash(0x58) + MasterHash(0x5959) + MasterHash(0x5A5A5A)
  sess_key_blob = rdp_salted_hash( s_bytes:master_secret, i_bytes:"X", client_rand:client_rand, server_rand:server_rand ) +
                  rdp_salted_hash( s_bytes:master_secret, i_bytes:"YY", client_rand:client_rand, server_rand:server_rand ) +
                  rdp_salted_hash( s_bytes:master_secret, i_bytes:"ZZZ", client_rand:client_rand, server_rand:server_rand );

  # InitialClientDecryptKey128 = FinalHash(Second128Bits(SessionKeyBlob))
  initial_client_decryptkey_128 = rdp_final_hash( k:substr( sess_key_blob, 16, 31 ), client_rand:client_rand, server_rand:server_rand );

  # InitialClientEncryptKey128 = FinalHash(Third128Bits(SessionKeyBlob))
  initial_client_encryptkey_128 = rdp_final_hash( k:substr( sess_key_blob, 32, 47 ), client_rand:client_rand, server_rand:server_rand );

  mac_key = substr( sess_key_blob, 0, 15 );

  ret_array["initial_client_encryptkey_128"] = initial_client_encryptkey_128;
  ret_array["initial_client_decryptkey_128"] = initial_client_decryptkey_128;
  ret_array["mac_key"] = mac_key;
  ret_array["sess_key_blob"] = sess_key_blob;

  if( debug ) {
    display( "-- rdp_calculate_rc4_keys --" );
    display( "RC4_ENC_KEY:\n", hexstr( ret_array["initial_client_encryptkey_128"] ) );
    display( "RC4_DEC_KEY:\n", hexstr( ret_array["initial_client_decryptkey_128"] ) );
    display( "HMAC_KEY:\n", hexstr( ret_array["mac_key"] ) );
    display( "SESS_BLOB\n", hexstr( ret_array["sess_key_blob"] ), "\n" );
  }

  return ret_array;
}

# @brief Creates a RDP client random used for the Basic Settings Exchange phase of the RDP Connection Sequence
# https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/5c763306-c9c0-483d-8674-31ed881de118
#
# @return 32-byte pseudorandom value
#
function rdp_create_client_random() {

  local_var i, client_rand;

  # https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/5c763306-c9c0-483d-8674-31ed881de118

  for( i = 0; i < 32; i++ )
    client_rand += raw_string( rand() % 256 );

  return client_rand;
}

# @brief Sends data
#        This adds mainly some debug messages to the plain send() function
#
# @param socket The socket to send data over
# @param data The data to send
# @param debug TRUE if debug messages should be printed
# @param debug_req_name The function name which uses this function to print in case of debug is TRUE
#
# @return FALSE in case of an error
#
function rdp_send( socket, data, debug, debug_req_name ) {

  local_var socket, data, debug, debug_req_name;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#rdp_send_recv" );
    return FALSE;
  }

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#rdp_send_recv" );
    return FALSE;
  }

  if( debug ) {
    display( "-- rdp_send --" );
    if( debug_req_name )
      display( "Sending ", debug_req_name );
    display( "Sending packet length: ", strlen( data ) );
    display( "Sending packet:\n", hexdump( ddata:data ), "\n" );
  }

  send( socket:socket, data:data );
}

# @brief Sends and receives RDP data over a given socket
#
# @param socket The socket to send data over
# @param data The data to send
# @param debug TRUE if debug messages should be printed
# @param debug_req_name The function name which uses this function to print in case of debug is TRUE
#
# @return The received data from the server
#
function rdp_send_recv( socket, data, debug, debug_req_name ) {

  local_var socket, data, debug, debug_req_name;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#rdp_send_recv" );
    return FALSE;
  }

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#rdp_send_recv" );
    return FALSE;
  }

  rdp_send( socket:socket, data:data, debug:debug, debug_req_name:debug_req_name );

  return rdp_recv( socket:socket, debug:debug, debug_req_name:debug_req_name );
}

# @brief Receives RDP data from a given socket
#
# @param socket The socket to send data over
# @param debug TRUE if debug messages should be printed
# @param debug_req_name The function name which uses this function to print in case of debug is TRUE
#
# @return The received data from the server
#
function rdp_recv( socket, debug, debug_req_name ) {

  local_var socket, debug, debug_req_name;
  local_var header, hd_len, buf, len;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#rdp_send_recv" );
    return FALSE;
  }

  header = recv( socket:socket, length:4 );
  hd_len = strlen( header );
  if( hd_len != 4 || hexstr( header ) !~ "^0300" ) {
    if( debug ) {
      display( "-- rdp_recv --" );
      if( debug_req_name )
        display( "Received malformed header response to ", debug_req_name, "\n" );
      else
        display( "Received malformed header response\n" );
      if( hd_len > 0 ) {
        display( "Received header length: ", hd_len );
        display( "Received header packet:\n", hexdump( ddata:header ), "\n" );
      } else {
        display( "Received empty header\n" );
      }
    }
    return FALSE;
  }

  len = getword( blob:header, pos:2 );
  buf = recv( socket:socket, length:len - 4 );

  buf = header + buf;

  if( debug ) {
    display("-- rdp_recv --");
    if( debug_req_name )
      display( "Received response to ", debug_req_name );
    display( "Received packet length: ", len );
    display( "Received packet:\n", hexdump( ddata:buf ), "\n" );
  }

  return buf;
}
