###############################################################################
# OpenVAS Include File
# $Id: ftp_func.inc 13843 2019-02-25 10:11:55Z cfischer $
#
# Methods for handling FTP
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
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
#
# $Revision: 13843 $
#
###############################################################################

# Enables debugging output for a few functions below
global_var _ftp_func_debug;
_ftp_func_debug = FALSE;

# Global vars to avoid that we're repeatedly asking redis for something we already know.
global_var _ftp_is_broken_array, __ftp_kb_login, __ftp_kb_pass;

# nb: key is the port or NULL (if not previously checked), value TRUE/FALSE if broken or not
_ftp_is_broken_array = make_array();

# @brief Tries to open a TCP connection to a FTP server. If the remote server is a FTP server and the expected
#        response was received the opened socket is returned.
#
# @param port The port of the remote FTP server.
#
# @return The opened socket or NULL if no connection could be opened, no or unexpected data was
#         received or no port parameter was passed.
#
function ftp_open_socket( port ) {

  local_var port;
  local_var soc, banner;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#ftp_open_socket" );
    return NULL;
  }

  if( ftp_get_is_marked_broken( port:port ) )
    return NULL;

  soc = open_sock_tcp( port );
  if( ! soc )
    return NULL;

  banner = ftp_recv_line( socket:soc );
  if( ! ftp_verify_banner( data:banner ) ) {
    close( soc );
    return NULL;
  }
  return soc;
}

# @brief Closes a passed TCP socket of a FTP server by first sending a QUIT message to the service.
#
# @param socket The socket to close.
#
# @return NULL
#
function ftp_close( socket ) {

  local_var socket;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ftp_close" );
    return;
  }

  send( socket:socket, data:'QUIT\r\n' );
  close( socket );
}

# @brief Returns the banner of a remote FTP service. This function will also mark
#        the remote service as "broken" if no banner was received at all.
#        If a banner was received it is saved into the knowledge base.
#        If a banner was requested for a specific port which was requested previously
#        the cached banner will be returned instead of doing a new request.
#
# @param port The port of the remote FTP service to connect to.
#
# @return The banner of the remote host, NULL if no response was received or the remote port wasn't scanned
#         and FALSE if no port parameter was passed.
#
function get_ftp_banner( port ) {

  local_var port;
  local_var banner, soc, csid_cmd, csid_banner, help_cmd, help_banner, syst_cmd, syst_banner;
  local_var stat_cmd, stat_banner, creds, user, pass, quit_cmd, quit_banner;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_ftp_banner" );
    return FALSE;
  }

  # nb: nasl_builtin_find_service.c is setting ftp/banner but we want to catch the CSID/HELP response
  # as well so we're using the uppercase FTP here.
  banner = get_kb_item( "FTP/banner/" + port );
  if( banner )
    return( banner );

  if( ! get_port_state( port ) )
    return NULL;

  if( ftp_get_is_marked_broken( port:port ) )
    return NULL;

  soc = open_sock_tcp( port );
  if( ! soc ) {
    ftp_set_is_marked_broken( port:port );
    return NULL;
  }

  banner = ftp_recv_line( socket:soc, retry:1 );
  if( ! ftp_verify_banner( data:banner ) ) {
    close( soc );
    ftp_set_is_marked_broken( port:port );
    return NULL;
  }

  banner = chomp( banner );

  # Client / Server Identification (CSID)
  # https://tools.ietf.org/html/draft-peterson-streamlined-ftp-command-extensions-10#section-6
  # Some servers like Serv-U implement this and report the Server-Type and Version even if the Banner is changed to something generic
  # Example Client Request: CSID Name=FTP Voyager; Version=15.0.0.2;
  # Example Server Response: 200 Name=Serv-U; Version=7.0.0.5; OS=Windows XP; OSVer=5.1.2600; CaseSensitive=0; DirSep=/;
  # Example Server Response when not logged in: 530 Not logged in.
  csid_cmd = "CSID Name=OpenVAS; Version=" + OPENVAS_VERSION + ";";
  csid_banner = get_ftp_cmd_banner( port:port, socket:soc, cmd:csid_cmd );

  if( csid_banner )
    set_kb_item( name:"ftp/fingerprints/" + port + "/csid_banner_noauth", value:csid_banner );

  if( egrep( pattern:"^200 .+", string:csid_banner ) )
    banner += '\n' + csid_banner;

  help_cmd = "HELP";
  help_banner = get_ftp_cmd_banner( port:port, socket:soc, cmd:help_cmd );

  if( help_banner )
    set_kb_item( name:"ftp/fingerprints/" + port + "/help_banner_noauth", value:help_banner );

  # https://cr.yp.to/ftp/syst.html
  syst_cmd = "SYST";
  syst_banner = get_ftp_cmd_banner( port:port, socket:soc, cmd:syst_cmd );

  if( syst_banner )
    set_kb_item( name:"ftp/fingerprints/" + port + "/syst_banner_noauth", value:syst_banner );

  stat_cmd = "STAT";
  stat_banner = get_ftp_cmd_banner( port:port, socket:soc, cmd:stat_cmd );

  if( stat_banner )
    set_kb_item( name:"ftp/fingerprints/" + port + "/stat_banner_noauth", value:stat_banner );

  if( egrep( pattern:"^530 .+", string:csid_banner ) || egrep( pattern:"^530 .+", string:help_banner ) ||
      egrep( pattern:"^530 .+", string:syst_banner ) || egrep( pattern:"^530 .+", string:stat_banner ) ) {

    creds = ftp_get_kb_creds();
    user  = creds["login"];
    pass  = creds["pass"];

    if( ftp_authenticate( socket:soc, user:user, pass:pass, skip_banner:TRUE ) ) {

      # nb: Don't use get_ftp_cmd_banner() as it would return the previously collected banner from the KB.
      csid_banner = ftp_send_cmd( socket:soc, cmd:csid_cmd );
      csid_banner = chomp( csid_banner );

      if( egrep( pattern:"^200 .+", string:csid_banner ) ) {
        banner += '\n' + csid_banner;
        replace_kb_item( name:"ftp/cmd/" + csid_cmd + "_banner/" + port, value:csid_banner );
      }

      if( csid_banner )
        set_kb_item( name:"ftp/fingerprints/" + port + "/csid_banner_authed", value:csid_banner );

      # Same for get_ftp_cmd_banner()
      help_banner = ftp_send_cmd( socket:soc, cmd:help_cmd );
      help_banner = chomp( help_banner );

      if( egrep( pattern:"^214 .+", string:help_banner ) )
        replace_kb_item( name:"ftp/cmd/" + help_cmd + "_banner/" + port, value:help_banner );

      if( help_banner )
        set_kb_item( name:"ftp/fingerprints/" + port + "/help_banner_authed", value:help_banner );

      # Same for get_ftp_cmd_banner()
      syst_banner = ftp_send_cmd( socket:soc, cmd:syst_cmd );
      syst_banner = chomp( syst_banner );

      if( egrep( pattern:"^215 .+", string:syst_banner ) )
        replace_kb_item( name:"ftp/cmd/" + syst_cmd + "_banner/" + port, value:syst_banner );

      if( syst_banner )
        set_kb_item( name:"ftp/fingerprints/" + port + "/syst_banner_authed", value:syst_banner );

      # Same for get_ftp_cmd_banner()
      stat_banner = ftp_send_cmd( socket:soc, cmd:stat_cmd );
      stat_banner = chomp( stat_banner );

      if( egrep( pattern:"^211 .+", string:stat_banner ) )
        replace_kb_item( name:"ftp/cmd/" + stat_cmd + "_banner/" + port, value:stat_banner );

      if( stat_banner )
        set_kb_item( name:"ftp/fingerprints/" + port + "/stat_banner_authed", value:stat_banner );
    }
  }

  replace_kb_item( name:"FTP/banner/" + port, value:banner );

  # nb: Don't use ftp_close_socket as we want to catch the QUIT banner as well.
  quit_cmd = "QUIT";
  quit_banner = get_ftp_cmd_banner( port:port, socket:soc, cmd:quit_cmd );
  if( quit_banner )
    set_kb_item( name:"ftp/fingerprints/" + port + "/quit_banner", value:quit_banner );

  close( soc );
  return banner;
}

# @brief Returns the answer of a remote FTP service to a specific command. This function will
#        also mark the remote service as "broken" if no answer was received at all.
#        If a response to a command was received it is saved into the knowledge base.
#        If a command was requested for a specific port which was requested previously
#        the cached response will be returned instead of doing a new request.
#
# @param port   The port (mandatory) of the remote FTP service to connect to.
# @param socket An existing socket (optional) to be used to send the command in the case no
#               existing response is cached within the KB. Please note that a custom passed
#               socket won't by closed by this function and needs to be closed within the calling VT.
# @param cmd    The command to send to the remote FTP service (e.g. HELP).
#
# @return The response of the remote host to the specific command, NULL if no response was received
#         or the remote port wasn't scanned and FALSE if no port or cmd parameter was passed.
#
function get_ftp_cmd_banner( port, socket, cmd ) {

  local_var port, socket, cmd;
  local_var banner, socket_no_close;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_ftp_cmd_banner" );
    return FALSE;
  }

  if( ! cmd ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cmd#-#get_ftp_cmd_banner" );
    return FALSE;
  }

  banner = get_kb_item( "ftp/cmd/" + cmd + "_banner/" + port );
  if( banner )
    return( banner );

  if( ! get_port_state( port ) )
    return NULL;

  if( ftp_get_is_marked_broken( port:port ) )
    return NULL;

  if( ! socket ) {
    socket = open_sock_tcp( port );
    if( ! socket ) {
      ftp_set_is_marked_broken( port:port );
      return NULL;
    }
    banner = ftp_recv_line( socket:socket );
    if( ! ftp_verify_banner( data:banner ) ) {
      close( socket );
      return NULL;
    }
  } else {
    socket_no_close = TRUE;
  }

  banner = ftp_send_cmd( socket:socket, cmd:cmd );
  if( ! socket_no_close )
    ftp_close( socket:socket );

  if( ! ftp_verify_banner( data:banner ) )
    return NULL;

  banner = chomp( banner );

  replace_kb_item( name:"ftp/cmd/" + cmd + "_banner/" + port, value:banner );
  return banner;
}

function ftp_send_cmd( socket, cmd ) {

  local_var socket, cmd;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ftp_send_cmd" );
    return;
  }

  if( ! cmd ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cmd#-#ftp_send_cmd" );
    return;
  }

  send( socket:socket, data:cmd + '\r\n' );
  return ftp_recv_line( socket:socket );
}

function ftp_recv_line( socket, retry ) {

  local_var n, r, res, t1, t2, socket, retry;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ftp_recv_line" );
    return;
  }

  r = recv_line( socket:socket, length:65535 );
  n = 0;
  if( _ftp_func_debug ) t1 = unixtime();
  while( strlen( r ) == 0 && n++ < retry )
    r = recv_line( socket:socket, length:65535 );
    if( _ftp_func_debug ) {
      t2 = unixtime();
      if( n > 0 && t2 - t1 > 1 ) display( 'ftp_recv_line: retried ', n, ' time( s ) = ', t2-t1, 's. Increase read timeout!\n' );
    }
    if( strlen( r ) < 4 )
      return r;

  n = 0;
  res = r;
  # Some FTP servers have a broken banner on several lines with a leading space
  while( ( strlen( r ) > 3 && r[3] == "-" ) || ( strlen( r ) >= 3 && r[0] == ' ' ) ) {
    n ++;
    r = recv_line( socket:socket, length:65535 );
    if( n > 255 )
      return;
    res += r;
  }
  return res;
}

function ftp_recv_listing( socket ) {

  local_var n, r, buf, socket;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ftp_recv_listing" );
    return;
  }

  n = 0;
  r = recv_line( socket:socket, length:65535 );
  buf = r;
  while( strlen( r ) ) {
    n ++;
    if( n > 4096 )
      return;
    r = recv_line( socket:socket, length:65535 );
    buf += r;
  }
  return buf;
}

function ftp_recv_data( socket, line ) {

  local_var buf, bytes, min, socket, line;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ftp_recv_data" );
    return;
  }

  if( line != NULL ) {
    bytes = ereg_replace( pattern:"^150.*\(([0-9]*) .*\)", string:line, replace:"\1" );
  }

  if( ! bytes ) {
    bytes = 8192;
    min = 1;
  } else {
    min = int( bytes );
    bytes = int( bytes);
  }

  return recv( socket:socket, min:bytes, length:bytes );
}

# @brief Logs into the remote FTP server
#
# @param socket      The already opened TCP socket of the remote FTP service to connect to.
# @param user        The username for the login.
# @param pass        The password for the login.
# @param skip_banner Specifies (optional) if the function should "grab" the initial banner
#                    of the service before sending the login. This can be set to TRUE if
#                    e.g. the calling VT has already did that step.
#
# @return TRUE if the login was successful, FALSE otherwise and NULL in the case no socket
#              parameter was passed.
#
function ftp_authenticate( socket, user, pass, skip_banner ) {

  local_var socket, user, pass, skip_banner;
  local_var oid, r;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ftp_authenticate" );
    return NULL;
  }

  # nb: Don't return NULL for both as some VTs required an empty user and/or pass to be passed.
  if( ! user ) {
    oid = get_script_oid();
    if( oid != "1.3.6.1.4.1.25623.1.0.14707" ) # typsoftftp_empty_username_dos.nasl needs an empty username
      set_kb_item( name:"nvt_debug_empty/" + oid, value:oid + "#-#user#-#ftp_authenticate" );
  }

  if( ! pass ) {
    oid = get_script_oid();
    if( oid != "1.3.6.1.4.1.25623.1.0.11160" ) # ftp_administrator.nasl needs an empty password
      set_kb_item( name:"nvt_debug_empty/" + oid, value:oid + "#-#pass#-#ftp_authenticate" );
  }

  if( ! skip_banner )
    r = ftp_recv_line( socket:socket ); # Receive the banner

  send( socket:socket, data:'USER ' + user + '\r\n' );
  r = ftp_recv_line( socket:socket );
  if( r[0] != '3' && r[0] != '2' )
    return FALSE;

  send( socket:socket, data:'PASS ' + pass + '\r\n' );
  r = ftp_recv_line( socket:socket );
  if( r[0] != '2' ) {
    return FALSE;
  } else {
    return TRUE;
  }
}

#
# Returns the port opened on the remote side through a 'PASV' command
#
function ftp_pasv( socket ) {

  local_var r, port, array, socket;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#ftp_pasv" );
    return;
  }

  send( socket:socket, data:'PASV\r\n' );
  r = ftp_recv_line( socket:socket );
  if( r[0] != '2' ) return FALSE;
  port = egrep( pattern:"^227 .* \([0-9]+,[0-9]+,[0-9]+,[0-9]+,[0-9]+,[0-9]+\)", string:r );
  if( ! port ) return FALSE;
  array = eregmatch( pattern:"^227 .* \([0-9]+,[0-9]+,[0-9]+,[0-9]+,([0-9]+),([0-9]+)\)", string:port );
  if( isnull( array ) ) return FALSE;
  port = int( array[1] ) * 256 + int( array[2] );
  return port;
}

# @brief Returns all FTP ports of the target system which are saved
#        in the KB entry "Services/ftp" found by the scanner. If no
#        ports where found open or no "default" parameter is passed
#        (if "nodefault" isn't set to TRUE) it will exit. In the case
#        where the "unscanned_closed" scanner setting is set to "no"
#        it will return the port passed via the default parameter.
#
# @note This function will fork on multiple open ports.
#
# @param default          The port to return if "unscanned_closed" is set to "no" and no ports have been found open.
# @param nodefault        Don't exit if no "default" parameter is passed, it still will return if no FTP servers are detected at all.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found FTP ports or the port passed via the default parameter in the case of no found ports
#         and "unscanned_closed" is set to "no".
#
function get_ftp_port( default, nodefault, ignore_broken, ignore_unscanned ) {

  local_var default, nodefault, ignore_broken, ignore_unscanned;
  local_var port;

  if( ! default && ! nodefault ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_ftp_port" );
    exit( 0 );
  }

  port = get_kb_item( "Services/ftp" );
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
  if( ftp_is_fragile_port( port:default ) )
    exit( 0 );

  if( ! ignore_broken && ftp_get_is_marked_broken( port:default ) )
    exit( 0 );

  return default;
}

# @brief Returns all FTP ports of the target system which are saved
#        in the KB entry "Services/ftp" found by the scanner. If no
#        ports where found open or no list is passed to the "default_ports"
#        parameter the function will return an empty list. In the case
#        where the "unscanned_closed" scanner setting is set to "no" it
#        will return a default set of FTP ports (namely 21 and 990)
#        or (if passed) the ports included in the list of the default_ports
#        parameter.
#
# @note  In contrast to @ref get_ftp_port this function will not fork
#        and return a list instead. This should be used e.g. for
#        Detection-VTs where we want to make sure that we're not getting
#        blocked by the remote service with to many concurrent connections.
#
# @param default_list     The list of ports to return (overwriting the internal defaults) if "unscanned_closed"
#                         is set to "no" and no ports have been found open.
# @param ignore_broken    Returns the ports even if they are known to be "broken".
# @param ignore_unscanned Returns the ports even if they where "unscanned" previously.
#
# @return The open / found FTP ports, if none where found an empty list. In the case of no found ports and
#         "unscanned_closed" is set to "no" a set of default ports or the list passed to the default_ports
#         parameter.
#
function ftp_get_ports( default_list, ignore_broken, ignore_unscanned ) {

  local_var default_list, ignore_broken, ignore_unscanned;
  local_var final_port_list, check_port_list, default_ports, num_ports, ports, port;

  final_port_list = make_list();
  check_port_list = make_list();
  default_ports = make_list( 21, 990 );
  num_ports = 0;

  ports = get_kb_list( "Services/ftp" );
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
    if( ftp_is_fragile_port( port:port ) )
      continue;

    if( ! ignore_broken && ftp_get_is_marked_broken( port:port ) )
      continue;

    final_port_list = make_list( final_port_list, port );
  }
  return final_port_list;
}

# @brief Returns an array containing the username/password credentials pair for a FTP login.
#        To avoid querying the KB for the same info again it will be cached within an internal variable.
#
# @param default_login The (optional) default login which should be used if no valid credentials where
#                      found in the KB.
# @param default_pass  The (optional) default password which should be used if no valid credentials where
#                      found in the KB.
#
# @return An array containing the following array indices:
#
#         ret_array["login"] = The login defined via the "FTP account" preference of logins.nasl,
#                              "anonymous" in the case the VT wasn't included in the scan config or
#                              the preference has been cleared.
#         ret_array["pass"]  = The password defined via the "FTP password (sent in clear)" preference of
#                              logins.nasl, "anonymous@example.com" in the case the VT wasn't included in
#                              the scan config or the preference has been cleared.
#
# @note Please include secpod_ftp_anonymous.nasl (or any VT pulling this dependency in) into your
#       script_dependencies so that an detected anonymous account is taken into account.
#
function ftp_get_kb_creds( default_login, default_pass ) {

  local_var default_login, default_pass;
  local_var login, pass, ret_array;
  # nb: __ftp_kb_login and __ftp_kb_pass are global vars

  if( ! isnull( __ftp_kb_login ) && ! default_login ) {
    login = string( __ftp_kb_login );
  } else {
    login = get_kb_item( "ftp/login" );
    if( isnull( login ) ) {
      if( default_login )
        login = default_login;
      else
        login = "anonymous";
    }
    if( ! default_login )
      __ftp_kb_login = string( login );
  }

  if( ! isnull( __ftp_kb_pass ) && ! default_pass ) {
    pass = string( __ftp_kb_pass );
  } else {
    pass = get_kb_item( "ftp/password" );
    if( isnull( pass ) ) {
      if( default_pass )
        pass = default_pass;
      else
        pass = "anonymous@example.com";
    }
    if( ! default_pass )
      __ftp_kb_pass = string( pass );
  }

  ret_array["login"] = login;
  ret_array["pass"]  = pass;
  return ret_array;
}

# @brief Returns the info if a remote FTP server was internally (in the KB)
#        marked as "broken". It will also save the information in an internal
#        array to avoid that redis is queried again for the same info.
#
# @param port The port of the remote FTP server.
#
# @return TRUE if the remote FTP server was marked as broken, FALSE otherwise and
#         NULL in the case where the port parameter wasn't passed.
#
function ftp_get_is_marked_broken( port ) {

  local_var port;
  local_var marked_broken_list, marked_broken;
  # nb: _ftp_is_broken_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#ftp_get_is_marked_broken" );
    return NULL;
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _ftp_is_broken_array[port] ) ) {
    if( _ftp_is_broken_array[port] )
      return TRUE;
    else
      return FALSE;
  }

  marked_broken = get_kb_item( "ftp/" + port + "/broken" );
  if( marked_broken ) {
    _ftp_is_broken_array[port] = TRUE;
  } else {
    _ftp_is_broken_array[port] = FALSE;
    marked_broken = FALSE;
  }
  return marked_broken;
}

# @brief Sets if a remote FTP server should be marked as "broken" to other NVTs.
#        The status is set within the following KB entries:
#        "ftp/" + port + "/broken" and "ftp/broken".
#
# @param port The port of the remote FTP server.
#
# @return TRUE if the status was successfully saved within the KB and
#         NULL if no port parameter was passed.
#
function ftp_set_is_marked_broken( port ) {

  local_var port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#ftp_set_is_marked_broken" );
    return NULL;
  }

  set_kb_item( name:"ftp/is_broken", value:TRUE );
  set_kb_item( name:"ftp/" + port + "/broken", value:TRUE );
  _ftp_is_broken_array[port] = TRUE;
  return TRUE;
}

# @brief Verify if the received/passed data is related to a FTP connection / server.
#
# @param data The data to be verified.
#
# @return TRUE if the passed data is related to a FTP connection / server, FALSE otherwise.
#
function ftp_verify_banner( data ) {

  local_var data;

  data = chomp( data );
  if( ! data )
    return FALSE;

  if( data =~ "^[0-9]{3}[ -].+" || "FTP server ready" >< data || "FTPd " >< data || "FTPD" >< data )
    return TRUE;

  return FALSE;
}

# Duplicated from misc_func.inc
function ftp_is_fragile_port( port ) {

  local_var port, fragile_ports, fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#ftp_is_fragile_port" );
    return;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports ) return;

  foreach fragile_port( fragile_ports ) {
    if( port == fragile_port ) return TRUE;
  }

  return;
}