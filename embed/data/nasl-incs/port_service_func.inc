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

# @brief Returns the info if the given port is marked internally as "fragile".
#
# @param port The port to check if it is marked as fragile.
#
# @return TRUE if the given port is marked as fragile, FALSE otherwise and
#         NULL if no port parameter was given.
#
function port_is_marked_fragile( port ) {

  local_var port;
  local_var fragile_ports, _fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#port_is_marked_fragile" );
    return;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports )
    return;

  foreach _fragile_port( fragile_ports ) {
    if( port == _fragile_port )
      return TRUE;
  }
  return;
}

# @brief Returns all during the portscan phase found / open TCP ports (saved
#        internally in the KB key "TCP/PORTS").
#
# @note This function will fork on multiple open / found ports.
#
# @note Add a dependency to secpod_open_tcp_ports.nasl if you want to use this function.
#
# @return The found TCP port. If no open port was found the function will exit.
#
function tcp_get_all_port() {

  local_var port;

  port = get_kb_item( "TCP/PORTS" );
  if( ! port )
    exit( 0 );

  # Includes e.g. PJL ports which are printing everything
  # sent to them so exit for such a port here
  if( port_is_marked_fragile( port:port ) )
    exit( 0 );

  if( ! get_port_state( port ) )
    exit( 0 );

  return port;
}

# @brief Returns all during the portscan phase found / open TCP ports (saved
#        internally in the KB key "TCP/PORTS") as a list.
#
# @note In contrast to @ref tcp_get_all_port this function will not fork and is
#       returning a list instead. This should be used e.g. for Detection-VTs
#       where we want to make sure that we're not getting blocked by the remote
#       service with to many concurrent connections or if a fork for each port
#       is not desired.
#
# @note Add a dependency to secpod_open_tcp_ports.nasl if you want to use this function.
#
# @return A list of all found / open TCP ports or an empty list if no open port was found.
#
function tcp_get_all_ports() {

  local_var ports, kb_ports, _port;

  ports = make_list();
  kb_ports = get_kb_list( "TCP/PORTS" );

  foreach _port( kb_ports ) {

    # Includes e.g. PJL ports which are printing everything
    # sent to them so continue for such a port here
    if( port_is_marked_fragile( port:_port ) )
      continue;

    if( ! get_port_state( _port ) )
      continue;

    ports = make_list( ports, _port );
  }
  return ports;
}

# @brief Returns the first found known to be open TCP port on the remote system. This function
#        replaces the nasl built-in @ref get_host_open_port where you can't control that it
#        doesn't return a "fragile" port.
#
# @note Add a dependency to secpod_open_tcp_ports.nasl if you want to use this function.
#
# @return The first found known to be open TCP Port. If none has been found the function will exit.
#
function tcp_get_first_open_port() {

  local_var ports;

  ports = tcp_get_all_ports();
  if( isnull( ports[0] ) )
    exit( 0 );
  else
    return ports[0]; # TBD: Bring some randomness into this?
}

# @brief Returns all during the portscan phase found / open UDP ports (saved
#        internally in the KB key "UDP/PORTS").
#
# @note This function will fork on multiple open / found ports.
#
# @note Add a dependency to gb_open_udp_ports.nasl if you want to use this function.
#
# @return The found UDP port. If no open port was found the function will exit.
#
function udp_get_all_port() {

  local_var port;

  port = get_kb_item( "UDP/PORTS" );
  if( ! port )
    exit( 0 );

  if( ! get_udp_port_state( port ) )
    exit( 0 );

  return port;
}

# @brief Returns all during the portscan phase found / open UDP ports (saved
#        internally in the KB key "UDP/PORTS") as a list.
#
# @note In contrast to @ref udp_get_all_port this function will not fork and is
#       returning a list instead. This should be used e.g. for Detection-VTs
#       where we want to make sure that we're not getting blocked by the remote
#       service with to many concurrent connections or if a fork for each port
#       is not desired.
#
# @note Add a dependency to gb_open_udp_ports.nasl if you want to use this function.
#
# @return A list of all found / open UDP ports or an empty list if no open port was found.
#
function udp_get_all_ports() {

  local_var ports, kb_ports, _port;

  ports = make_list();
  kb_ports = get_kb_list( "UDP/PORTS" );

  foreach _port( kb_ports ) {

    if( ! get_udp_port_state( _port ) )
      continue;

    ports = make_list( ports, _port );
  }
  return ports;
}
