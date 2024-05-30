# Copyright (C) 2020 Greenbone Networks GmbH
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

# @brief Closes an passed socket and exit afterwards
#
# @param _FCT_ANON_ARGS[0] The socket to close as an unnamed argument.
#
function close_sock_and_exit() {

  local_var soc;

  soc = _FCT_ANON_ARGS[0];

  if( soc )
    close( soc );

  exit( 0 );
}

# @brief Sends a specific data string (if passed) and receives the answer
#        of the remote service.
#
# @param port   The port of the remote service. Either the port OR the soc parameter
#               needs to be passed, not both. If the port parameter was passed the
#               function will close the opened socket on its own.
# @param soc    An already opened socket to the remote service. Either the soc OR the
#               soc parameter needs to be passed, not both.
# @param data   An (optional) data string which should be send to the remote service.
# @param proto  The protocol used to connect to the service if the port parameter is passed.
#               Defaults to 'tcp' if not passed.
# @param length The amount of bytes which should be received from the remote service.
#               Defaults to '1024' if not passed.
#
# @return The received data or NULL if no connection to the remote service could be opened
#         or not all parameters where passed correctly.
#
function socket_send_recv( port, soc, data, proto, length ) {

  local_var port, soc, data, proto, length;
  local_var nosock, recv;

  if( ! port && ! soc ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port/soc#-#socket_send_recv" );
    return;
  }

  if( proto && proto != "udp" && proto != "tcp" ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#socket_send_recv: proto parameter passed but doesn't contain match 'tcp' or 'udp'" );
    return;
  }

  if( port && soc ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#socket_send_recv: port and soc parameter passed, please chose only one" );
    return;
  }

  if( ! soc )
    nosock = TRUE;

  if( port && nosock ) {
    if( ! proto )
      proto = "tcp";

    if( proto == "udp" ) {
      if( ! get_udp_port_state( port ) )
        return;
      soc = open_sock_udp( port );
    } else {
      if( ! get_tcp_port_state( port ) )
        return;
      soc = open_sock_tcp( port );
    }

    if( ! soc )
      return;
  }

  if( data )
    send( socket:soc, data:data );

  if( ! length )
    length = 1024;

  recv = recv( socket:soc, length:length );

  # close socket only if it was opened by this function
  if( nosock )
    close( soc );

  return chomp( recv );
}
