# Copyright (C) 2023 Greenbone Networks GmbH
# Some text descriptions might be excerpted from (a) referenced
# source(s), and are Copyright (C) by the respective right holder(s).
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

# - General info:
#   - https://www.ietf.org/rfc/rfc2608.html (Service Location Protocol, Version 2)
#   - https://www.ietf.org/rfc/rfc2165.html (Service Location Protocol, Version 1)
#   - https://devel.aanet.ru/protocols/protocol/slp.htm
# - Currently only version 2 supported
# - All length fields in SLP messages are in network byte order

global_var SLP_VERSIONS_RAW, SLP_VERSIONS_RAW_NAME_MAPPING;
global_var SLP_ERRORS_RAW, SLP_ERRORS_RAW_NAME_MAPPING;
global_var SLP_MESSAGES_RAW, SLP_MESSAGES_RAW_NAME_MAPPING;
global_var SLP_EXPECTED_RESPONSES;

SLP_VERSIONS_RAW["1"] = raw_string( 0x01 );
SLP_VERSIONS_RAW["2"] = raw_string( 0x02 );

SLP_VERSIONS_RAW_NAME_MAPPING[raw_string( 0x01 )] = "1";
SLP_VERSIONS_RAW_NAME_MAPPING[raw_string( 0x02 )] = "2";

# https://www.ietf.org/rfc/rfc2608.html#section-7 (7. Errors)
SLP_ERRORS_RAW["LANGUAGE_NOT_SUPPORTED"] = raw_string( 0x01 );
SLP_ERRORS_RAW["PARSE_ERROR"]            = raw_string( 0x02 );
SLP_ERRORS_RAW["INVALID_REGISTRATION"]   = raw_string( 0x03 );
SLP_ERRORS_RAW["SCOPE_NOT_SUPPORTED"]    = raw_string( 0x04 );
SLP_ERRORS_RAW["AUTHENTICATION_UNKNOWN"] = raw_string( 0x05 );
SLP_ERRORS_RAW["AUTHENTICATION_ABSENT"]  = raw_string( 0x06 );
SLP_ERRORS_RAW["AUTHENTICATION_FAILED"]  = raw_string( 0x07 );
SLP_ERRORS_RAW["VER_NOT_SUPPORTED"]      = raw_string( 0x09 );
SLP_ERRORS_RAW["INTERNAL_ERROR"]         = raw_string( 0x0a );
SLP_ERRORS_RAW["DA_BUSY_NOW"]            = raw_string( 0x0b );
SLP_ERRORS_RAW["OPTION_NOT_UNDERSTOOD"]  = raw_string( 0x0c );
SLP_ERRORS_RAW["INVALID_UPDATE"]         = raw_string( 0x0d );
SLP_ERRORS_RAW["MSG_NOT_SUPPORTED"]      = raw_string( 0x0e );
SLP_ERRORS_RAW["REFRESH_REJECTED"]       = raw_string( 0x0f );

SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x01 )] = "LANGUAGE_NOT_SUPPORTED";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x02 )] = "PARSE_ERROR";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x03 )] = "INVALID_REGISTRATION";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x04 )] = "SCOPE_NOT_SUPPORTED";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x05 )] = "AUTHENTICATION_UNKNOWN";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x06 )] = "AUTHENTICATION_ABSENT";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x07 )] = "AUTHENTICATION_FAILED";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x09 )] = "VER_NOT_SUPPORTED";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x0a )] = "INTERNAL_ERROR";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x0b )] = "DA_BUSY_NOW";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x0c )] = "OPTION_NOT_UNDERSTOOD";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x0d )] = "INVALID_UPDATE";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x0e )] = "MSG_NOT_SUPPORTED";
SLP_ERRORS_RAW_NAME_MAPPING[raw_string( 0x0f )] = "REFRESH_REJECTED";

# https://www.ietf.org/rfc/rfc2608.html#section-8 (8. Required SLP Messages)
SLP_MESSAGES_RAW["SrvRqst"]     = raw_string( 0x01 ); # Service Request
SLP_MESSAGES_RAW["SrvRply"]     = raw_string( 0x02 ); # Service Reply
SLP_MESSAGES_RAW["SrvReg"]      = raw_string( 0x03 ); # Service Registration
SLP_MESSAGES_RAW["SrvDeReg"]    = raw_string( 0x04 ); # Service Deregister
SLP_MESSAGES_RAW["SrvAck"]      = raw_string( 0x05 ); # Service Acknowledge
SLP_MESSAGES_RAW["AttrRqst"]    = raw_string( 0x06 ); # Attribute Request
SLP_MESSAGES_RAW["AttrRply"]    = raw_string( 0x07 ); # Attribute Reply
SLP_MESSAGES_RAW["DAAdvert"]    = raw_string( 0x08 ); # DA Advertisement
SLP_MESSAGES_RAW["SrvTypeRqst"] = raw_string( 0x09 ); # Service Type Request
SLP_MESSAGES_RAW["SrvTypeRply"] = raw_string( 0x0a ); # Service Type Reply
SLP_MESSAGES_RAW["SAAdvert"]    = raw_string( 0x0b ); # SA Advertisement

# nb: The names might be used in some calling VTs so make sure to check them when updating them here
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x01 )] = "SrvRqst (Service Request)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x02 )] = "SrvRply (Service Reply)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x03 )] = "SrvReg (Service Registration)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x04 )] = "SrvDeReg (Service Deregister)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x05 )] = "SrvAck (Service Acknowledge)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x06 )] = "AttrRqst (Attribute Request)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x07 )] = "AttrRply (Attribute Reply)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x08 )] = "DAAdvert (DA Advertisement)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x09 )] = "SrvTypeRqst (Service Type Request)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x0a )] = "SrvTypeRply (Service Type Reply)";
SLP_MESSAGES_RAW_NAME_MAPPING[raw_string( 0x0b )] = "SAAdvert (SA Advertisement)";

# The expected responses to our requests.

# From the RFC: Normally, a SrvRqst elicits a SrvRply. There are two exceptions: If the
# <service-type> is set to "service:directory-agent", DAs respond to the SrvRqst with a DAAdvert
# (see Section 8.5.)  If set to "service:service-agent", SAs respond with a SAAdvert (see Section
# 8.6.)
SLP_EXPECTED_RESPONSES[SLP_MESSAGES_RAW["SrvRqst"]] = make_list( SLP_MESSAGES_RAW["SrvRply"], SLP_MESSAGES_RAW["DAAdvert"], SLP_MESSAGES_RAW["SAAdvert"] );

# @brief Parses a given SLP message and returns extracted info in a structured way.
#
# @param data  The SLP data / message to parse
# @param debug If set to TRUE additional debug output is printed like e.g. the type of the received
#              message, info if unexpected data is received or similar.
#
# @return An array containing the parsed data or NULL if not all required parameters are given or no
#         valid SLP response / not response at all was received. The array indices are the following:
#
#         ret_array["version_raw"]    = The SLP version extracted from the data in raw format
#         ret_array["version_string"] = The SLP version extracted from the data in human readable format
#         ret_array["func_id_raw"]    = The Function-ID extracted from the data in raw format
#         ret_array["func_id_string"] = The Function-ID extracted from the data in human readable format
#         ret_array["xid"]            = The XID (unique value) extracted from the data as an int
#
function slp_parse_response( data, debug ) {

  local_var data, debug;
  local_var version, version_string, func_id, func_id_string, xid, ret_array;

  if( ! data ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#slp_parse_response" );
    return NULL;
  }

  if( debug ) display( "---[ Given data / message ]---", "\n", hexdump( ddata:data ) );

  # nb: https://www.ietf.org/rfc/rfc2608.html#section-8 describes that the SLP header should also
  # include the Language Tag but for our purposes it should be enough to check until the XID.
  if( strlen( data ) < 12 ) {
    if( debug ) display( "---[ Too short (expected length: 12+) data given ]---" );
    return NULL;
  }

  version = data[0];
  version_string = SLP_VERSIONS_RAW_NAME_MAPPING[version];
  if( ! version_string || version_string != "2" ) {
    if( debug ) display( "---[ Unsupported SLP version found in passed data: '0x", hexstr( version ), "'. Currently supported: 0x02 (2) ]---" );
    return NULL;
  }

  func_id = data[1];
  func_id_string = SLP_MESSAGES_RAW_NAME_MAPPING[func_id];
  if( ! func_id_string ) {
    if( debug ) display( "---[ Unknown / unsupported SLP Function-ID found in passed data: '0x", hexstr( func_id ), "'. See SLP_MESSAGES_RAW_NAME_MAPPING array in this include for the list of supported IDs ]---" );
    return NULL;
  }

  xid = getword( blob:data, pos:10 );

  ret_array = make_array();
  ret_array["version_raw"]    = version;
  ret_array["version_string"] = version_string;
  ret_array["func_id_raw"]    = func_id;
  ret_array["func_id_string"] = func_id_string;
  ret_array["xid"]            = xid;

  if( debug ) display( "---[ Data extracted from passed data ]---", "\n", "SLP version: ", version_string, "\n", "Function-ID: ", func_id_string, "\n", "XID:         ", xid );

  return ret_array;
}

# @brief Creates a SLP version 2 message with the given Function-ID and the expected content.
#
# @param func_id  The Function-ID to create a SLP message for. Currently supported:
#                 SrvRqst (Service Request) with ID 0x01
# @param xid      The XID (unique value) to be included in the request. Length: 16 bit
# @param msg_exts An array containing the extensions like e.g. Service Type, Scope List and similar
#                 to be added to the request. If not given defaults for different Function-IDs are
#                 assumed. Example to pass such an extension for the SrvRqst message type is:
#                 msg_exts = make_array( "service_type_list", "service:service-agent",
#                                        "scope_list", "default",
#                                        "lang_tag", "en" );
#                 slp_create_message( func_id:SLP_MESSAGES_RAW["SrvRqst"], msg_exts:msg_exts, xid:xid );
# @param debug    If set to TRUE additional debug output is printed like e.g. the crafted packet.
#
# @return A "raw" packet of the crafted SLP message or NULL if not all required parameters or an
#         unsupported Function-ID was given.
#
function slp_create_message( func_id, xid, msg_exts, debug ) {

  local_var func_id, xid, msg_exts, debug;
  local_var xid_type, pkt, service_type_list, service_type_list_len, scope_list, scope_list_len;
  local_var data, lang_tag, lang_tag_len, pkt_len;

  if( ! func_id ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#func_id#-#slp_create_message" );
    return NULL;
  }

  if( func_id != SLP_MESSAGES_RAW["SrvRqst"] ) {
    if( debug ) display( "---[ Unsupported SLP Function-ID passed via 'func_id' parameter: '0x", hexstr( func_id ), "'. Currently supported: 0x01 (SrvRqst (Service Request)) ]---" );
    return NULL;
  }

  # nb: isnull() because we might also want to allow an XID of "0"
  if( isnull( xid ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#xid#-#slp_create_message" );
    return NULL;
  }

  xid_type = typeof( xid );
  if( xid_type != "int" ) {
    set_kb_item(name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#slp_create_message(): Invalid value passed to 'xid' parameter. Expected type: 'int', given type: '" + xid_type + "'.");
    return NULL;
  }

  pkt = SLP_VERSIONS_RAW["2"]; # Version 2 (8 bits)

  if( SLP_MESSAGES_RAW["SrvRqst"] ) {
    pkt += SLP_MESSAGES_RAW["SrvRqst"]; # Function: Service Request (8 bits)

    if( msg_exts["service_type_list"] )
      service_type_list = msg_exts["service_type_list"];
    else
      service_type_list = "service:service-agent";
    service_type_list_len = strlen( service_type_list );

    if( msg_exts["scope_list"] )
      scope_list = msg_exts["scope_list"];
    else
      scope_list = "default";
    scope_list_len = strlen( scope_list );

    data = raw_string( 0x00, 0x00,                      # Previous Response List Length: 0
                       mkword( service_type_list_len ), # Service Type Length (16 bits)
                       service_type_list,               # Service Type List
                       mkword( scope_list_len ),        # Scope List Length (16 bits)
                       scope_list,                      # Scope List
                       0x00, 0x00,                      # Predicate Length
                       0x00, 0x00 );                    # SLP SPI Length
  }

  # nb: We shouldn't reach this because we're already checking the Function-ID but it was still
  # added as another cross-check / fallback.
  else {
    return NULL;
  }

  pkt += raw_string( 0x00, 0x00, 0x00 ); # Packet length (24 bits), this will be overwritten later once the whole packet was "crafted".

  # nb: We might want to make these configurable as well...
  pkt += raw_string( 0x20 );             # Flags: Multicast requested (16 bits)
  pkt += raw_string( 0x00 );             # Reserved (16 bits)
  pkt += raw_string( 0x00, 0x00, 0x00 ); # Next extension offset: 0 (24 bits)
  pkt += mkword( xid );                  # XID (16 bits)

  if( msg_exts["lang_tag"] )
    lang_tag = msg_exts["lang_tag"];     # Lang Tag (16 bits)
  else
    lang_tag = "en";
  lang_tag_len = strlen( lang_tag );     # Lang Tag Len (16 bits)

  pkt += mkword( lang_tag_len ) + lang_tag;
  pkt += data;

  # Only at this point we know the whole packet size and can overwrite the previous defined packet length
  # TBD: We might want to handle this "more" clever in the future
  pkt_len = strlen( pkt );
  pkt = insstr( pkt, mkword( pkt_len ), 3, 4 );

  if( debug ) display( "---[ Crafted SLP message ]---", "\n", hexdump( ddata:pkt ) );

  return pkt;
}
