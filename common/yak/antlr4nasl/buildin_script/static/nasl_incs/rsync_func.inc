###############################################################################
# OpenVAS Include File
# $Id: rsync_func.inc 7922 2017-11-28 10:06:28Z cfischer $
#
# Functions for Rsync
#
# Authors:
# Michael Meyer
#
# Copyright:
# Copyright (c) 2015 Greenbone Networks GmbH
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
#
# $Revision: 7922 $
#
###############################################################################

# to use this .inc you also have to include:
#
# misc_func.inc
# byte_func.inc

function rsync_connect( port ) {

  local_var soc, banner, port, i, buf;

  if( ! port ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#rsync_connect" );
    return FALSE;
  }

  soc = open_sock_tcp( port );
  if( ! soc ) return NULL;

  banner = recv_line( socket:soc, length:8096 );
  if( isnull( banner ) ) {
    close( soc );
    return FALSE;
  }

  replace_kb_item( name:"rsync/" + port + "/banner", value:chomp( banner ) );
  send( socket:soc, data:'@RSYNCD: 29.0\n' );

  # Ignore MOTD
  while( 1 ) {
    buf = recv_line( socket:soc, length:8096 );
    if( ! buf || strstr( buf, '@ERROR' ) )
      break;
  }

  return soc;
}

function get_module_list( soc ) {

  local_var soc, port, num, line, ret;

  if( ! soc ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#soc#-#get_module_list" );
    return FALSE;
  }

  num = 0;
  send( socket:soc, data:'#list\r\n' );
  while( 1 ) {
    line = recv_line( socket:soc, length:8096, timeout:1 );
    if( ! line || strstr( line, '@RSYNCD' ) )
      break;

   ret[num++] = line;
  }

  return ret;
}

function authentication_required( module, port ) {

  local_var port, module;

  if( ! port ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#authentication_required" );
    return FALSE;
  }

  soc = rsync_connect( port:port );
  if( ! soc ) return 'unknown';

  send( socket:soc, data:string( module + '\r\n' ) );
  line = recv_line( socket:soc, length:8096 );

  close( soc );

  if( strstr( line, '@RSYNCD: OK' ) ) {
    return 'no';
  } else if( strstr( line, '@RSYNCD: AUTHREQD' ) ) {
    return 'yes';
  } else {
    return 'unknown';
  }
}

function get_file( soc, module, file ) {

  local_var soc, module, file, buf, ret, flen, file_end, plen, pfile, s, b;

  if( isnull( soc ) ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#soc#-#get_file" );
    return FALSE;
  }

  if( isnull( module ) ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#module#-#get_file" );
    return FALSE;
  }

  if( isnull( file ) ) {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file#-#get_file" );
    return FALSE;
  }

  set_byte_order( BYTE_ORDER_LITTLE_ENDIAN );

  send( socket:soc, data:module + '\n' );
  buf = recv_line( socket:soc, length:4096 );

  if( "@RSYNCD: OK" >!< buf ) return FALSE;

  send( socket:soc, data:'--server\n--sender\n-L\n.\n' + module +'/' + file + '\n\n' );
  buf = recv( socket:soc, length:4096 );

  send( socket:soc, data: raw_string( 0,0,0,0 ) );
  buf = recv( socket:soc, length:4096 );

  if( ! buf || buf == "" ) return FALSE;

  s = hex2raw( s:'0000000000a000000000000000000000000000000000' );

  send( socket:soc, data:s );
  while( b = recv( socket:soc, length:1024 ) )
    ret += b;

  plen  = strlen( s ) + 4;
  pfile = strlen( s ) + 8;

  if( isnull( ret ) || strlen( ret ) < ( pfile + 4 ) ) return FALSE;

  flen = getdword( blob:ret, pos:plen );
  file_end = pfile + ( flen - 1 );

  if( strlen( ret ) < file_end ) return FALSE;

  return substr( ret, pfile, file_end );
}

function get_rsync_port( default ) {

  local_var default, port;

  if( ! default ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_rsync_port" );

  port = get_kb_item( "Services/rsync" );
  if( port ) default = port;

  # Includes e.g. PJL ports which are printing everything
  # sent to them so exit for such a port here
  if( rsync_is_fragile_port( port:default ) ) exit( 0 );

  if( ! get_port_state( default ) ) exit( 0 );

  return default;
}

# Duplicated from misc_func.inc
function rsync_is_fragile_port( port ) {

  local_var port, fragile_ports, fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#rsync_is_fragile_port" );
    return;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports ) return;

  foreach fragile_port( fragile_ports ) {
    if( port == fragile_port ) return TRUE;
  }

  return;
}