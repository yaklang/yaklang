# Copyright (C) 2002 Michel Arboi (rpc_get_port), 2011 SecPod (rpc_construct_packet)
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

# Fast replacement for getrpcport() which uses the libc
function rpc_get_port( program, protocol, portmap ) {

  local_var program, protocol, portmap;
  local_var a, b, c, d, p_a, p_b, p_c, p_d, pt_a, pt_b, pt_c, pt_d;
  local_var req, port, broken, len, soc, r;


  if( isnull( program ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#program#-#rpc_get_port" );
    return( 0 );
  }

  if( isnull( protocol ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#protocol#-#rpc_get_port" );
    return( 0 );
  }

  a = rand() % 255;
  b = rand() % 255;
  c = rand() % 255;
  d = rand() % 255;

  p_a = program / 16777216;
  p_a = p_a % 256;

  p_b = program / 65356;
  p_b = p_b % 256;

  p_c = program / 256;
  p_c = p_c % 256;

  p_d = program % 256;

  pt_a = protocol / 16777216;
  pt_a = pt_a % 256;

  pt_b = protocol / 65535;
  pt_b = pt_b % 256;

  pt_c = protocol / 256;
  pt_c = pt_c % 256;

  pt_d = protocol % 256;

  req = raw_string( a, b, c, d,               # XID
                    0x00, 0x00, 0x00, 0x00,   # Msg type: call
                    0x00, 0x00, 0x00, 0x02,   # RPC Version
                    0x00, 0x01, 0x86, 0xA0,   # Program
                    0x00, 0x00, 0x00, 0x02,   # Program version
                    0x00, 0x00, 0x00, 0x03,   # Procedure
                    0x00, 0x00, 0x00, 0x00,   # Credentials - flavor
                    0x00, 0x00, 0x00, 0x00,   # Credentials - length
                    0x00, 0x00, 0x00, 0x00,   # Verifier - Flavor
                    0x00, 0x00, 0x00, 0x00,   # Verifier - Length
                    p_a, p_b, p_c, p_d,       # Program
                    0xFF, 0xFF, 0xFF, 0xFF ); # Version (any)

  if( protocol == IPPROTO_TCP )
    req += raw_string( 0x00, 0x00, 0x00, 0x06 );
  else
    req += raw_string( pt_a, pt_b, pt_c, pt_d );

  req += raw_string( 0x00, 0x00, 0x00, 0x00 ); # Port

  if( protocol == IPPROTO_TCP ) {
    req = mkdword( strlen( req ) ) + req;
    req = raw_string( 0x80 | ord( req[ 0 ]) ) + substr( req, 1, strlen( req ) );
  }

  if( isnull( portmap ) ) {
    port = int( get_kb_item( "rpc/portmap" ) );
    if( port == 0 )
      port = 111;
  } else {
    port = portmap;
  }

  broken = get_kb_item( "/tmp/rpc/noportmap/" + port );
  if( broken )
    return( 0 );

  if( protocol == IPPROTO_TCP ) {
    len = 32;
    soc = open_sock_tcp( port );
  } else {
    len = 28;
    soc = open_sock_udp( port );
  }

  if( ! soc )
    return;

  send( socket:soc, data:req );
  r = recv( socket:soc, length:len );

  close( soc );

  if( ! r ) {
    set_kb_item( name:"/tmp/rpc/noportmap/" + port, value:TRUE );
    return( 0 );
  }

  if( strlen( r ) != len ) {
    return( 0 );
  } else {
    port = getdword( blob:raw_string( r[ len -  4 ], r[ len -  3 ], r[ len - 2 ], r[ len - 1 ] ) );

    if( protocol == IPPROTO_TCP ) {
      if( get_tcp_port_state( port ) ) {
        return( port );
      } else {
        return( 0 );
      }
    } else {
      if( get_udp_port_state( port ) ) {
        return( port );
      } else {
        return( 0 );
      }
    }
  }
}

## This function will construct a rpc packet
function rpc_construct_packet( program, prog_ver, procedure, data, udp, credentials, verifier ) {

  local_var program, prog_ver, procedure, data, udp, credentials, verifier;
  local_var xid, header, cred_data, verifier_data, rpc_packet, data_len, frag_header;

  if( isnull( program ) )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#program#-#rpc_construct_packet" );

  if( isnull( prog_ver ) )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#prog_ver#-#rpc_construct_packet" );

  if( isnull( procedure ) )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#procedure#-#rpc_construct_packet" );

  # Add 0 If credentials are not passed
  if( isnull( credentials ) ) {
    credentials[0] = 0;
    credentials[1] = 0;
  }

  # Add 0 If Verified are not passed
  if( isnull( verifier ) ) {
    verifier[0] = 0;
    verifier[1] = 0;
  }

  # Random XID
  xid = rand();

  # Construct complete RPC Rstat Request
  header  = mkdword( xid );                          # XID
  header += mkdword( 0 );                            # Message Type: Call (0)
  header += mkdword( 2 );                            # RPC Version: 2
  header += mkdword( program );                      # Program
  header += mkdword( prog_ver );                     # Program Version
  header += mkdword( procedure );                    # Procedure

  # Credentials
  cred_data = mkdword( credentials[0] );             # Flavor
  cred_data += mkdword( strlen( credentials[1] ) );  # Length

  # Verifier
  verifier_data = mkdword( verifier[0] );            # Flavor
  verifier_data += mkdword( strlen( verifier[1] ) ); # Length

  rpc_packet = header + cred_data + verifier_data + data;

  # Add Fragment header if it's not UDP protocol
  if( udp != "udp" || udp == FALSE ) {
    # Fragment Length
    data_len = strlen( header + cred_data + verifier_data + data );

    # Fragment Header
    frag_header  = mkbyte( 0x80 );         # Last Fragment
    frag_header  += mkbyte( 0 );
    frag_header  += mkdword( data_len );   # Fragment Length
    rpc_packet = frag_header + rpc_packet;
  }

  return( rpc_packet );
}
