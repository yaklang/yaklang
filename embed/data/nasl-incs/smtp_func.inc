###############################################################################
# OpenVAS Include File
# $Id: smtp_func.inc 13821 2019-02-21 19:42:54Z cfischer $
#
# Support functions for handling SMTP
#
# Authors:
# Michel Arboi <arboi@alussinan.org>
#
# Copyright:
# Copyright (C) 2002 Michel Arboi <arboi@alussinan.org>
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2,
# as published by the Free Software Foundation
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
#
# $Revision: 13821 $
#
###############################################################################

# Global vars to avoid that we're repeatedly asking redis for something we already know.
global_var _smtp_is_broken_array, _smtp_is_wrapped_array, __smtp_helo;

# nb: key is the port or NULL (if not previously checked), value TRUE/FALSE if broken/wrapped or not
_smtp_is_broken_array = make_array();
_smtp_is_wrapped_array = make_array();

# Enables debugging output for a few functions below.
global_var __smtp_debug;
__smtp_debug = FALSE;

# @brief Closes a passed TCP socket of a SMTP server by first sending a QUIT message to the service.
#
# @param socket     The socket to close.
# @param check_data The (mandatory) data which was previously received by other functions. Based on this
#                   data (e.g. if it contains a 3 ASCII Digits code or FALSE of @ref smtp_recv_line) the
#                   function decides if the final QUIT command should be sent.
#
# @return NULL
#
function smtp_close( socket, check_data ) {

  local_var socket, check_data;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#smtp_close" );
    return;
  }

  if( check_data =~ "^[0-9]{3}[ -]" || check_data == FALSE ) {
    send( socket:socket, data:'QUIT\r\n' );
    smtp_recv_line( socket:socket );
  }
  close( socket );
}

# @brief Opens a connection to a SMTP server and returns the opened socket. Allows to optionally sending
#        additional data within an HELO/EHLO request.
#
# @param port      The port of the remote SMTP server.
# @param data      An (optional) string which is sent within an HELO/EHLO request to the server. If no
#                  string is passed an opened socket will be returned directly. If the string is
#                  passed the function will verify if the server has replied with an SMTP status
#                  code in the range of 2-3xx.
# @param send_helo If set to TRUE (the default) the passed data string will be sent within an HELO
#                  request.
# @param send_ehlo If set to TRUE the passed data string will be sent within an EHLO
#                  request.
# @param code      An (optional) string or regex which is used to verify the received data against.
#                  If this parameter isn't passed the default regex of ^[2-3][0-9]{2}[ -] is assumed.
#                  If e.g. a code of 220 is passed the following regex is used: ^220[ -].
#
# @note If send_helo and send_ehlo is set to FALSE or no data parameter was passed the function will
#       return the opened socket directly without sending the HELO/EHLO request to the target. Also
#       make sure that you don't set both send_helo and send_ehlo to TRUE. In this case the function
#       can't decide for you which request you want to sent and will return with a NULL.
#
# @return The opened socket or NULL if no connection could be opened, no data was
#         received, no port parameter or a wrong mix of parameters was passed.
#
function smtp_open( port, data, send_helo, send_ehlo, code ) {

  local_var port, data, send_helo, send_ehlo, code;
  local_var soc, res, req;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#smtp_open" );
    return NULL;
  }

  if( send_helo == TRUE && send_ehlo == TRUE ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#smtp_open: send_helo and send_ehlo set to TRUE, the function can't decide which one you want to use and will return with NULL" );
    return NULL;
  }

  if( smtp_get_is_marked_broken( port:port ) ) {
    if( __smtp_debug ) display( "SMTP DEBUG (smtp_open): SMTP service is marked as broken.\n" );
    return NULL;
  }

  soc = open_sock_tcp( port );
  if ( ! soc ) {
    if( __smtp_debug ) display( "SMTP DEBUG (smtp_open): Can't open connection to port ", port, "\n" );
    return NULL;
  }

  res = smtp_recv_banner( socket:soc );
  if( ! res ) {
    if( __smtp_debug ) display( "SMTP DEBUG (smtp_open): No initial 220 banner received from SMTP service.\n" );
    smtp_close( socket:soc, check_data:res );
    return NULL;
  }

  # set some sane defaults based on what's passed via the function parameters
  if( isnull( send_helo ) && ! send_ehlo )
    send_helo = TRUE;

  if( isnull( send_helo ) && isnull( send_ehlo ) ) {
    send_helo = TRUE;
    send_ehlo = FALSE;
  }

  if( isnull( data ) || ( send_helo == FALSE && send_ehlo == FALSE ) ) {
    if( __smtp_debug ) display( "SMTP DEBUG (smtp_open): Successful returning socket without sending a HELO/EHLO first.\n" );
    return soc;
  }

  if( ! send_helo && send_ehlo )
    req = strcat( 'EHLO ', data, '\r\n' );
  else
    req = strcat( 'HELO ', data, '\r\n' );

  if( ! code )
    code = "[2-3][0-9]{2}";

  send( socket:soc, data:req );
  res = smtp_recv_line( socket:soc, code:code );
  if( ! res ) {
    if( __smtp_debug ) display( "SMTP DEBUG (smtp_open): No data received from smtp_recv_line or response didn't matched the regex pattern: ", code, ".\n" );
    smtp_close( socket:soc, check_data:res );
    return NULL;
  }

  if( __smtp_debug ) display( "SMTP DEBUG (smtp_open): Successful returning socket after sending a HELO/EHLO first.\n" );
  return soc;
}

function smtp_send_socket( socket, from, to, body ) {

  local_var socket, from, to, body, buff;

  if( ! socket ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#smtp_send_socket" );
  if( ! from ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#from#-#smtp_send_socket" );
  if( ! to ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#to#-#smtp_send_socket" );
  if( ! body ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#body#-#smtp_send_socket" );

  # display(string("smtp_send_socket from=", from, " to=", to, "\n"));
  # Just to be sure
  send( socket:socket, data:string( "RSET\r\n" ) );
  buff = recv_line( socket:socket, length:2048 );
  # Here, we might test the return code
  if( from !~ ' *<.*> *') from = strcat( '<', from, '>' );
  send( socket:socket, data:string( "MAIL FROM: ", from, "\r\n" ) );
  buff = recv_line( socket:socket, length:2048 );
  if( ! ereg( pattern:"^2[0-9][0-9] ", string:buff ) ) return( 0 );

  if( to !~ ' *<.*> *') to = strcat( '<', to, '>' );
  send( socket:socket, data:string( "RCPT TO: ", to, "\r\n" ) );
  buff = recv_line( socket:socket, length: 2048 );
  if( ! ereg( pattern:"^2[0-9][0-9] ", string:buff ) ) return( 0 );

  send( socket:socket, data:string( "DATA\r\n" ) );
  buff = recv_line( socket:socket, length:2048 );
  if( ! ereg( pattern:"^3[0-9][0-9] ", string:buff ) ) return( 0 );

  # Make sure that every line ends up with \r\n
  # This is not useful yet, as only two scripts send data to the SMTP server
  #body = ereg_replace(string: body, pattern: string("([^\r])\n"), replace: string("\\1\r\n"));
  send( socket:socket, data:body );
  send( socket:socket, data:string( "\r\n.\r\n" ) );
  buff = recv_line( socket:socket, length:2048 );
  if( ! ereg( pattern:"^2[0-9][0-9] ", string:buff ) ) return( 0 );
  return( 1 );
}

# @brief Sends an e-mail to a SMTP server.
#
# @param The port of the SMTP server.
# @param The "from" field used in the e-mail.
# @param The "to" field used in the e-mail.
# @param The body to send in the e-mail.
#
# @return The received response of the SMTP server or NULL if it was not possible
#         to open a connection, not all required parameters where passed, the remote
#         SMTP server doesn't accept us or is no SMTP server at all.
#
function smtp_send_port( port, from, to, body ) {

  local_var port, from, to, body;
  local_var socket, ret;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#smtp_send_port" );
    return NULL;
  }

  if( ! from ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#from#-#smtp_send_port" );
    return NULL;
  }

  if( ! to ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#to#-#smtp_send_port" );
    return NULL;
  }

  if( ! body ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#body#-#smtp_send_port" );
    return NULL;
  }

  socket = smtp_open( port:port, helo:smtp_get_helo_from_kb( port:port ) );
  if( ! socket )
    return NULL;

  ret = smtp_send_socket( socket:socket, from:from, to:to, body:body );
  smtp_close( socket:socket, check_data:ret );
  return( ret );
}

function smtp_from_header() {

  local_var fromaddr;

  fromaddr = get_kb_item( "SMTP/headers/From" );
  if( ! fromaddr ) fromaddr = "openvas@example.com";
  return( fromaddr );
}

function smtp_to_header() {

  local_var toaddr;

  toaddr = get_kb_item( "SMTP/headers/To" );
  if( ! toaddr ) toaddr = string( "postmaster@[", get_host_ip(), "]" );
  return( toaddr );
}

# @brief Opens a connection to a SMTP server and receives the returned banner. If a banner was received it
#        is saved into the knowledge base. If a banner for a port was requested which was requested
#        previously the cached banner will be returned instead of doing a new connection. This function
#        is also sending additional ESMTP commands to the SMTP server and saving the responses 
#        to the KB for later use in the fingerprinting of the remote SMTP server by other VTs.
#
# @param port The port of the remote SMTP server.
#
# @return The banner of the remote host, NULL if no response was received or the remote port wasn't scanned
#         and FALSE if no port parameter was passed.
#
function get_smtp_banner( port ) {

  local_var port;
  local_var banner, soc, ehlo, is_tls, auth_string, auths, auth, help, noop, rset, quit;
  local_var commands, command, _command, first;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_smtp_banner" );
    return FALSE;
  }

  # nb: nasl_builtin_find_service.c is setting smtp/banner but we want to catch the various responses
  # below as well so we're using the uppercase SMTP here.
  banner = get_kb_item( "SMTP/banner/" + port );
  if( banner )
    return( banner );

  if( ! get_port_state( port ) )
    return NULL;

  if( smtp_get_is_marked_broken( port:port ) )
    return NULL;

  if( smtp_get_is_marked_wrapped( port:port ) )
    return NULL;

  # nb: Don't use smtp_open() as we want to grab the banner below.
  soc = open_sock_tcp( port );
  if( ! soc ) {
    smtp_set_is_marked_broken( port:port );
    return NULL;
  }

  # nb: smtp_recv_line returns NULL if the service hasn't responded at all
  # and FALSE if we haven't received a 3 ASCII Digits code from the service.
  # This allows us to differ between "broken" and "wrapped" below.
  banner = smtp_recv_line( socket:soc, retry:1 );
  if( isnull( banner ) ) {
    close( soc );
    smtp_set_is_marked_wrapped( port:port );
    return NULL;
  }

  # nb: See description about the FALSE value above. The second one is an
  # additional safeguard to make sure that we're getting the expected response.
  if( banner == FALSE || banner !~ "^[0-9]{3}[ -].+" ) {
    close( soc );
    # Doesn't look like SMTP...
    smtp_set_is_marked_broken( port:port );
    return NULL;
  }

  banner = chomp( banner );
  replace_kb_item( name:"SMTP/banner/" + port, value:banner );

  # nb: Don't use smtp_open as we want to use the banner below as well.
  send( socket:soc, data:'EHLO ' + smtp_get_helo_from_kb( port:port ) + '\r\n' );
  ehlo = smtp_recv_line( socket:soc );
  ehlo = chomp( ehlo );
  if( ehlo ) {

    set_kb_item( name:"smtp/fingerprints/" + port + "/ehlo_banner", value:ehlo );

    if( get_port_transport( port ) > ENCAPS_IP ) {
      is_tls = TRUE;
      set_kb_item( name:"smtp/fingerprints/" + port + "/tls_ehlo_banner", value:ehlo );
    } else {
      is_tls = FALSE;
      set_kb_item( name:"smtp/fingerprints/" + port + "/nontls_ehlo_banner", value:ehlo );
    }

    commands = split( ehlo, keep:FALSE );
    first = 0;
    foreach command( commands ) {

      # nb: The first command includes the remote hostname so we're ignoring it here.
      first++;
      if( first == 1 )
        continue;

      _command = eregmatch( string:command, pattern:"^250[ -](.+)" );
      if( _command[1] ) {

        set_kb_item( name:"smtp/fingerprints/" + port + "/commandlist", value:_command[1] );

        if( is_tls )
          set_kb_item( name:"smtp/fingerprints/" + port + "/tls_commandlist", value:_command[1] );
        else
          set_kb_item( name:"smtp/fingerprints/" + port + "/nontls_commandlist", value:_command[1] );
      }
    }

    auth_string = egrep( string:ehlo, pattern:"^250[ -]AUTH .+" );
    auth_string = chomp( auth_string );
    if( auth_string ) {

      set_kb_item( name:"smtp/auth_methods/available", value:TRUE );

      auth_string = substr( auth_string, 9 );
      auths = split( auth_string, sep:" ", keep:FALSE );
      foreach auth( auths ) {
        set_kb_item( name:"smtp/fingerprints/" + port + "/authlist", value:auth );
        if( is_tls )
          set_kb_item( name:"smtp/fingerprints/" + port + "/tls_authlist", value:auth );
        else
          set_kb_item( name:"smtp/fingerprints/" + port + "/nontls_authlist", value:auth );
      }
    }
  }

  send( socket:soc, data:'HELP\r\n' );
  help = smtp_recv_line( socket:soc );
  help = chomp( help );
  if( help )
    set_kb_item( name:"smtp/fingerprints/" + port + "/help_banner", value:help );

  send( socket:soc, data:'NOOP\r\n' );
  noop = smtp_recv_line( socket:soc );
  noop = chomp( noop );
  if( noop )
    set_kb_item( name:"smtp/fingerprints/" + port + "/noop_banner", value:noop );

  send( socket:soc, data:'RSET\r\n' );
  rset = smtp_recv_line( socket:soc );
  rset = chomp( rset );
  if( rset )
    set_kb_item( name:"smtp/fingerprints/" + port + "/rset_banner", value:rset );

  send( socket:soc, data:'QUIT\r\n' );
  quit = smtp_recv_line( socket:soc );
  quit = chomp( quit );
  if( quit )
    set_kb_item( name:"smtp/fingerprints/" + port + "/quit_banner", value:quit );

  # nb: Don't use smtp_close() as we want to get the QUIT banner above.
  close( soc );
  return( banner );
}

# @brief Receives data from a SMTP server over the given socket. The function will verify if
#        the received data is related to SMTP responses.
#
# @param socket The socket to the SMTP server.
# @param code   An (optional) string or regex which is used to verify the received data against.
#               If this parameter isn't passed the default regex of '^[0-9]{3}[ -].+' is assumed.
#               If e.g. a code of 220 is passed the following regex is used: '^220[ -].+'.
# @param retry  An (optional) amount of retries which should be used for the initial connection
#               if no data was received. If this parameter isn't passed no retry will be done.
# @param last   An (optional) parameter to specify (via a passed TRUE/FALSE) if only the last
#               data matching the mentioned regex in the 'code' parameter should be returned.
#               If this parameter isn't passed all data received is returned.
#
# @return The received data or NULL if the received data is not matching against the regex, no
#         data was received at all or no socket parameter was passed.
#
function smtp_recv_line( socket, code, retry, last ) {

  local_var socket, code, retry, last;
  local_var pat, r, n, ret;
  # nb: __smtp_debug is a global var to enable some debug output

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#smtp_recv_line" );
    return NULL;
  }

  if( isnull( retry ) )
    retry = 0;

  if( isnull( last ) )
    last = FALSE;

  if( isnull( code ) )
    pat = "^[0-9]{3}[ -].+";
  else
    pat = strcat("^", code, "[ -].+");

  r = recv_line( socket:socket, length:4096 );

  n = 0;
  while( ! r && n++ < retry ) {
    r = recv_line( socket:socket, length:4096 );
  }

  if( __smtp_debug && r ) display( "SMTP DEBUG (smtp_recv_line): Initial response = ", r, "\n" );

  n = 0;
  ret = r;
  if( strlen( r ) < 4 || r !~ "^[0-9]{3}[ -].+" )
    return NULL;

  # nb: We want to differ between NULL and FALSE in the return values here.
  if( ! ereg( pattern:pat, string:r ) )
    return FALSE;

  while( ereg( pattern:pat, string:r ) ) {
    n++;
    r = recv_line( socket:socket, length:4096 );
    if( strlen( r ) == 0 )
      break;

    if( n > 512 )
      return FALSE;

    if( last )
      ret = r;
    else
      ret = strcat( ret, r );
  }
  if( __smtp_debug && r ) display( "SMTP DEBUG (smtp_recv_line): Final return = ", ret, "\n" );
  return ret;
}

# @brief Receives the initial banner of a SMTP server over the given socket. The function will verify if
#        the received data is related to SMTP responses and is matching a 220 return code.
#
# @param socket The socket to the SMTP server.
# @param retry  An (optional) amount of retries which should be used for the initial connection
#               if no data was received. If this parameter isn't passed no retry will be done.
#
# @return The received data or NULL if the received data is not matching a 220 return code, no
#         data was received at all or no socket parameter was passed.
#
function smtp_recv_banner( socket, retry ) {

  local_var socket, retry;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#smtp_recv_banner" );
    return NULL;
  }

  return smtp_recv_line( socket:socket, retry:retry, code:"220" );
}

# @brief Returns all SMTP ports of the target system which are saved
#        in the KB entry "Services/smtp" found by the scanner. If no
#        ports where found open or no "default" parameter is passed
#        (if "nodefault" isn't set to TRUE) it will exit. In the case
#        where the "unscanned_closed" scanner setting is set to "no"
#        it will return the port passed via the default parameter.
#
# @note This function will fork on multiple open ports.
#
# @param default          The port to return if "unscanned_closed" is set to "no" and no ports have been found open.
# @param nodefault        Don't exit if no "default" parameter is passed, it still will return if no SMTP servers are detected at all.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found SMTP ports or the port passed via the default parameter in the case of no found ports
#         and "unscanned_closed" is set to "no".
#
function get_smtp_port( default, nodefault, ignore_broken, ignore_unscanned ) {

  local_var default, nodefault, ignore_broken, ignore_unscanned;
  local_var port;

  if( ! default && ! nodefault ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_smtp_port" );
    exit( 0 );
  }

  port = get_kb_item( "Services/smtp" );
  if( port )
    default = port;

  # nb: If this is the case we haven't found an open port, default wasn't passed
  # and nodefault was set to TRUE. We can't continue in this case.
  if( ! default )
    exit( 0 );

  if( ! ignore_unscanned && ! get_port_state( default ) )
    exit( 0 );

  # Includes e.g. PJL ports which are printing everything
  # sent to them so exit for such a port here
  if( smtp_is_fragile_port( port:default ) )
    exit( 0 );

  if( ! ignore_broken && smtp_get_is_marked_broken( port:default ) )
    exit( 0 );

  return default;
}

# @brief Returns all SMTP ports of the target system which are saved
#        in the KB entry "Services/smtp" found by the scanner. If no
#        ports where found open or no list is passed to the "default_ports"
#        parameter the function will return an empty list. In the case
#        where the "unscanned_closed" scanner setting is set to "no" it
#        will return a default set of SMTP ports (namely 25, 465 and 587)
#        or (if passed) the ports included in the list of the default_ports
#        parameter.
#
# @note  In contrast to @ref get_smtp_port this function will not fork
#        and return a list instead. This should be used e.g. for
#        Detection-VTs where we want to make sure that we're not getting
#        blocked by the remote service with to many concurrent connections.
#
# @param default_list     The list of ports to return (overwriting the internal defaults) if "unscanned_closed"
#                         is set to "no" and no ports have been found open.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found SMTP ports, if none where found an empty list. In the case of no found ports and
#         "unscanned_closed" is set to "no" a set of default ports or the list passed to the default_ports
#         parameter.
#
function smtp_get_ports( default_list, ignore_broken, ignore_unscanned ) {

  local_var default_list, ignore_broken, ignore_unscanned;
  local_var final_port_list, check_port_list, default_ports, num_ports, ports, port;

  final_port_list = make_list();
  check_port_list = make_list();
  default_ports = make_list( 25, 465, 587 );
  num_ports = 0;

  ports = get_kb_list( "Services/smtp" );
  if( ports && typeof( ports ) == "array" ) {
    foreach port( ports ) {
      num_ports++;
      check_port_list = make_list( check_port_list, port );
    }
  }

  if( num_ports == 0 ) {
    if( default_list && typeof( default_list ) == "array" )
      check_port_list = default_list;
    else
      check_port_list = default_ports;
  }

  foreach port( check_port_list ) {

    if( ! ignore_unscanned && ! get_port_state( port ) )
      continue;

    # Includes e.g. PJL ports which are printing everything
    # sent to them so don't include them here.
    if( smtp_is_fragile_port( port:port ) )
      continue;

    if( ! ignore_broken && smtp_get_is_marked_broken( port:port ) )
      continue;

    final_port_list = make_list( final_port_list, port );
  }
  return final_port_list;
}

# @brief Returns the info if a remote SMTP server was internally (in the KB)
#        marked as "broken". It will also save the information in an internal
#        array to avoid that redis is queried again for the same info.
#
# @param port The port of the remote SMTP server.
#
# @return TRUE if the remote SMTP server was marked as broken, FALSE otherwise and
#         NULL in the case where the port parameter wasn't passed.
#
function smtp_get_is_marked_broken( port ) {

  local_var port;
  local_var marked_broken_list, marked_broken;
  # nb: _smtp_is_broken_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#smtp_get_is_marked_broken" );
    return NULL;
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _smtp_is_broken_array[port] ) ) {
    if( _smtp_is_broken_array[port] )
      return TRUE;
    else
      return FALSE;
  }

  marked_broken = get_kb_item( "smtp/" + port + "/is_broken" );
  if( marked_broken ) {
    _smtp_is_broken_array[port] = TRUE;
  } else {
    _smtp_is_broken_array[port] = FALSE;
    marked_broken = FALSE;
  }
  return marked_broken;
}

# @brief Sets if a remote SMTP server should be marked as "broken" to other NVTs.
#        The status is set within the following KB entries:
#        "smtp/" + port + "/is_broken" and "smtp/is_broken".
#
# @param port The port of the remote SMTP server.
#
# @return TRUE if the status was successfully saved within the KB and
#         NULL if no port parameter was passed.
#
function smtp_set_is_marked_broken( port ) {

  local_var port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#smtp_set_is_marked_broken" );
    return NULL;
  }

  set_kb_item( name:"smtp/is_broken", value:TRUE );
  set_kb_item( name:"smtp/" + port + "/is_broken", value:TRUE );
  _smtp_is_broken_array[port] = TRUE;
  return TRUE;
}

# @brief Returns the info if a remote SMTP server was internally (in the KB)
#        marked as "wrapped". It will also save the information in an internal
#        array to avoid that redis is queried again for the same info.
#
# @param port The port of the remote SMTP server.
#
# @return TRUE if the remote SMTP server was marked as wrapped, FALSE otherwise and
#         NULL in the case where the port parameter wasn't passed.
#
function smtp_get_is_marked_wrapped( port ) {

  local_var port;
  local_var marked_wrapped_list, marked_wrapped;
  # nb: _smtp_is_wrapped_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#smtp_get_is_marked_wrapped" );
    return NULL;
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _smtp_is_wrapped_array[port] ) ) {
    if( _smtp_is_wrapped_array[port] )
      return TRUE;
    else
      return FALSE;
  }

  marked_wrapped = get_kb_item( "smtp/" + port + "/is_wrapped" );
  if( marked_wrapped ) {
    _smtp_is_wrapped_array[port] = TRUE;
  } else {
    _smtp_is_wrapped_array[port] = FALSE;
    marked_wrapped = FALSE;
  }
  return marked_wrapped;
}

# @brief Sets if a remote SMTP server should be marked as "wrapped" to other NVTs.
#        The status is set within the following KB entries:
#        "smtp/" + port + "/is_wrapped" and "smtp/is_wrapped".
#
# @param port The port of the remote SMTP server.
#
# @return TRUE if the status was successfully saved within the KB and
#         NULL if no port parameter was passed.
#
function smtp_set_is_marked_wrapped( port ) {

  local_var port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#smtp_set_is_marked_wrapped" );
    return NULL;
  }

  set_kb_item( name:"smtp/is_wrapped", value:TRUE );
  set_kb_item( name:"smtp/" + port + "/is_wrapped", value:TRUE );
  _smtp_is_wrapped_array[port] = TRUE;
  return TRUE;
}

# @brief Returns the "third party" domain the user has configured within smtp_settings.nasl. To avoid querying
#        the KB for the same info again it will be cached within an internal variable.
#
# @return A string containing the "third party" domain the user has configured within smtp_setting.nasl
#         or "example.com" in the case where the default was kept in smtp_settings.nasl or the VT wasn't
#         included in the scan configuration.
#
function get_3rdparty_domain() {

  local_var domain;
  # nb: __3rdparty_domain is a global var

  if( ! isnull( __3rdparty_domain ) ) {
    domain = string( __3rdparty_domain );
  } else {
    domain = get_kb_item( "Settings/third_party_domain" );
    if( ! isnull( domain ) ) {
      __3rdparty_domain = string( domain );
    } else {
      domain = "example.com";
      __3rdparty_domain = domain;
    }
  }
  return domain;
}

# @brief Gets the SMTP HELO domain saved within the KB in smtp/port/accepted_helo_name.
#        If no SMTP HELO was found in the KB the function will fallback
#        to the returned value of @ref get_3rdparty_domain.
#
# @param port The port of the remote SMTP server to get the HELO from.
#
# @note Add a dependency to check_smtp_helo.nasl if you want to use this function.
#
# @return The SMTP HELO domain or NULL in the case where the port parameter wasn't passed.
#
function smtp_get_helo_from_kb( port ) {

  local_var port, helo;
  # nb: __smtp_helo is a global var

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#smtp_get_helo_from_kb" );
    return NULL;
  }

  if( ! isnull( __smtp_helo ) ) {
    helo = string( __smtp_helo );
  } else {
    helo = get_kb_item( "smtp/"+ port + "/accepted_helo_name" );
    if( ! isnull( helo ) ) {
      __smtp_helo = string( helo );
    } else {
      helo = get_3rdparty_domain();
      __smtp_helo = helo;
    }
  }
  return helo;
}

# Duplicated from misc_func.inc
function smtp_is_fragile_port( port ) {

  local_var port, fragile_ports, fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#smtp_is_fragile_port" );
    return;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports ) return;

  foreach fragile_port( fragile_ports ) {
    if( port == fragile_port ) return TRUE;
  }

  return;
}