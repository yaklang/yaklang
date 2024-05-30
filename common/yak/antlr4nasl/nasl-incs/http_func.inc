###############################################################################
# OpenVAS Include File
# $Id: http_func.inc 14291 2019-03-18 17:18:50Z cfischer $
#
# Methods to handle HTTP
#
# Authors:
# Michel Arboi <arboi@alussinan.org>
# get_http_port added by Georges Dagousset
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
# $Revision: 14291 $
#
###############################################################################

# Enables to include the OID of the VT within the HTTP user agent of http_get_user_agent()
# and http_send_recv() for debugging purposes on the target / targets logfiles.
global_var _http_ua_include_oid;
_http_ua_include_oid = FALSE;

# Global vars to avoid that we're repeatedly asking redis for something we already know.
global_var __http_func_user_agent, __http_is_cgi_scan_disabled;

# Enables debugging output for a few functions below.
global_var _http_debug;
_http_debug = FALSE;

# Global var passed to a few functions below
global_var optimize_test_enabled;
optimize_test_enabled = get_preference( "optimize_test" );

# Global vars to avoid that we're repeatedly asking redis for something we already know.
global_var _http_is_broken_array, _http_no404_string_array, _http_has_generic_xss_array, _http_is_embedded_array;

# nb: key is the host+port or NULL (if not previously checked), value TRUE/FALSE if broken or not
_http_is_broken_array = make_array();

# nb: key is the host+port or NULL (if not previously checked), value a string or FALSE
_http_no404_string_array = make_array();

# nb: key is the host+port or NULL (if not previously checked), value TRUE/FALSE if it has a generic XSS or not
_http_has_generic_xss_array = make_array();

# nb: key is the port or NULL (if not previously checked), value TRUE/FALSE if embedded or not
_http_is_embedded_array = make_array();

# @brief Returns a HTTP User Agent string which can be used by VTs within the
#        User-Agent: header of HTTP requests.
#
# @param vt_string This is a string which is initially used to build the default UA
#        string for the case where the "http/user-agent" KB key is empty initially.
# @param dont_add_oid If set to TRUE don't add the OID of the calling VT to the
#        User-Agent string even if the _http_ua_include_oid global variable is set.
#
# @return A string containing a HTTP user agent.
#
function http_get_user_agent( vt_string, dont_add_oid ) {

  local_var vt_string, dont_add_oid;
  local_var ua_vt_string, default, ua;
  # nb: __http_func_user_agent is global var

  if( vt_string )
    ua_vt_string = vt_string;
  else
    ua_vt_string = "MVULNSCAN";

  # nb: Only used if the function is called by a VT running via openvas-nasl
  # and if global_settings.nasl wasn't called before that VT.
  if( defined_func( "vendor_version" ) )
    vendor = vendor_version();
  else
    vendor = NULL;

  if( ! isnull( vendor ) && vendor != "" )
    default = "Mozilla/5.0 [en] (X11, U; " + vendor + ")";
  else if( OPENVAS_VERSION )
    default = "Mozilla/5.0 [en] (X11, U; " + ua_vt_string + " " + OPENVAS_VERSION + ")";
  else
    default = "Mozilla/5.0 [en] (X11, U; " + ua_vt_string + ")";

  if( ! isnull( __http_func_user_agent ) ) {
    ua = string( __http_func_user_agent );
  } else {
    ua = get_kb_item( "http/user-agent" );
    if( ! isnull( ua ) ) {
      __http_func_user_agent = string( ua );
    } else {
      __http_func_user_agent = default;
      ua = default;
    }
  }

  # nb: See description of the variable for the purpose
  if( _http_ua_include_oid && ! dont_add_oid )
    ua = ereg_replace( string:ua, pattern:"(.+)$", replace:"\1 (OID:" + get_script_oid() + ")" );

  return ua;
}

# @brief Split the passed HTTP header into an array containing the header names as an array
#        key and the header values as the array value.
#
# @param h The full HTTP header to split into an array.
#
# @return An array containing the split header or NULL if no header was passed or contained invalid data.
#
function headers_split( h ) {

  local_var h;
  local_var end, array, item, subarray, ret;

  if( ! h )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#h#-#headers_split" );

  end = strstr( h, '\r\n\r\n' );
  if( end )
    h -= end;

  array = split( h, keep:FALSE );
  foreach item( array ) {
    subarray = split( item, sep:':', keep:FALSE );
    ret[tolower( subarray[0] )] = ereg_replace( pattern:"^ *", replace:"", string:subarray[1] );
  }

  return ret;
}

# @brief Converts a single hex char, e.g. '0' or 'a' into the integer representation of this
#        char. The function is used e.g. for chunk-decoding.
#
# @param num The hex char to convert.
#
# @return An integer representation of the passed char.
#
function __hex_value( num ) {

  local_var num;

  if( num == "a") return( 10 );
  if( num == "b") return( 11 );
  if( num == "c") return( 12 );
  if( num == "d") return( 13 );
  if( num == "e") return( 14 );
  if( num == "f") return( 15 );
  return( int( num ) );
}

# @brief Converts a passed hex string into the decimal representation of the string. Examples:
#        xvalue: 0x0a0b or 0x0A0B -> Return 2571
#        xvalue: 0x0a or 0x0A -> Return 10
#
# @param xvalue The hex string to convert.
#
# @return A decimal representation of the passed hex string or 0 if no string was passed.
#
function hex2dec( xvalue ) {

  local_var xvalue;
  local_var l, ret, m, i, n;

  if( ! xvalue ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#xvalue#-#hex2dec" );
    return( 0 );
  }

  xvalue = tolower( xvalue );
  if( '\r\n' >< xvalue ) {
    l = strlen( xvalue ) - 2;
  } else if( '\n' >< xvalue ) {
    l = strlen( xvalue ) - 1;
  } else {
    l = strlen( xvalue );
  }

  ret = 0;
  m = 1;
  if( l == 0 )
    return( 0 );

  # Remove the trailing spaces
  while( xvalue[l - 1] == " " && l > 0 )
    l--;

  for( i = l; i > 0; i-- ) {
    n = __hex_value( num:xvalue[i - 1] ) * m;
    ret += n;
    m = m * 16;
  }

  return int( ret );
}

# @brief Creates a HTTP GET request for "/" or a specific URL, sends
#        the request to the target, receives the response and extracts
#        the HTTP header (banner) without the body from the response.
#        If a header was received it is saved into the knowledge base.
#        If a header was requested which was requested previously the cached
#        header will be returned instead of doing a new request.
#
# @param port          The HTTP port of the target.
# @param file          The URL/file to request, defaults to "/".
# @param ignore_broken Returns the banner even if the remote host was known to be "broken".
#
# @return The header of the remote host or NULL if no response was received, no port parameter was passed or the remote port wasn't scanned.
#
function get_http_banner( port, file, ignore_broken ) {

  local_var port, file, ignore_broken;
  local_var sb, banner, req, soc, body;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_http_banner" );
    return NULL;
  }

  if( ! get_port_state( port ) )
    return NULL;

  if( ! file )
    file = "/";

  sb = strcat( "www/real_banner/", port, file );
  banner = get_kb_item( sb );
  if( banner )
    return banner;

  # nb: This is set in nasl/nasl_builtin_find_service.c of openvas-scanner as well but without a trailing "/"
  # like e.g. www/banner/80. In the case of this function we want to use the trailing "/" because the scanner
  # is saving the whole response including the body in that KB entry. This is not something we want to return
  # with this function.
  sb = strcat( "www/banner/", port, file );
  banner = get_kb_item( sb );
  if( banner )
    return banner;

  if( ! ignore_broken && http_get_is_marked_broken( port:port, host:"*" ) )
    return NULL; # TBD: Really return NULL if marked as broken? We still could use the banner version in NVTs...

  # nb: Always keep http_get() before http_open_socket() as the first could
  # fork with multiple vhosts and the child's would share the same socket
  # causing race conditions and similar.
  req = http_get( item:file, port:port );
  soc = http_open_socket( port );
  if( ! soc )
    return NULL;

  send( socket:soc, data:req );
  banner = http_recv_headers2( socket:soc );
  #body = http_recv_body( socket:soc, headers:banner ); # TBD: What's the purpose of this?
  http_close_socket( soc );

  if( banner )
    replace_kb_item( name:sb, value:banner );

  return banner;
}

# @brief Returns all HTTP ports of the target system which are saved
#        in the KB entry "Services/www" found by the scanner. If no
#        ports where found open or no "default" parameter is passed
#        (if "nodefault" isn't set to TRUE) it will exit. In the case
#        where the "unscanned_closed" scanner setting is set to "no"
#        it will return the port passed via the default parameter.
#
# @param default             The port to return if "unscanned_closed" is set to "no" and no ports have been found open.
# @param nodefault           Don't exit if no "default" parameter is passed, it still will return if no HTTP servers are detected at all.
# @param host                The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#                            parameter or get_host_name) of the remote server. If no host parameter
#                            is passed http_host_name will be called automatically and a debug
#                            message is logged into the KB (currently commented out).
# @param ignore_broken       Returns the ports even if they are known to be "broken".
# @param ignore_unscanned    Returns the ports even if they where "unscanned" previously.
# @param ignore_cgi_disabled Returns the ports even if CGI scanning is disabled.
# @param dont_use_vhosts     Allows to specify if http_host_name is called internally (which would fork on multiple vhosts).
#
# @return The open / found HTTP ports or the port passed via the default parameter in the case of no found ports
#         and "unscanned_closed" is set to "no".
#
function get_http_port( default, nodefault, host, ignore_broken, ignore_unscanned, ignore_cgi_disabled, dont_use_vhosts ) {

  local_var default, nodefault, host, ignore_broken, ignore_unscanned, ignore_cgi_disabled, dont_use_vhosts;
  local_var port;
  # nb: optimize_test_enabled is a global var

  if( ! default && ! nodefault ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#default#-#get_http_port" );
    exit( 0 );
  }

  # nb: Keep before the forking get_kb_item call from below
  if( ! ignore_cgi_disabled ) {
    if( http_is_cgi_scan_disabled() ) {
      if( optimize_test_enabled && "yes" >< optimize_test_enabled ) {
        set_kb_item( name:"nvt_cgi_scanning_disabled/" + get_script_oid(), value:get_script_oid() + "#-#get_http_port" );
      }
      exit( 0 );
    }
  }

  port = get_kb_item( "Services/www" ); # nb: This will fork on multiple open ports
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
  if( http_is_fragile_port( port:default ) )
    exit( 0 );

  if( ! dont_use_vhosts && ! host ) {
    host = http_host_name( dont_add_port:TRUE );
    #set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#get_http_port" );
  } else if( ! host ) {
    host = "*";
  }

  if( ! ignore_broken && http_get_is_marked_broken( port:default, host:host ) )
    exit( 0 );

  return default;
}

# @brief Returns all HTTP ports of the target system which are saved
#        in the KB entry "Services/www" found by the scanner. If no
#        ports where found open or no list is passed to the "default_ports"
#        parameter the function will return an empty list. In the case
#        where the "unscanned_closed" scanner setting is set to "no" it
#        will return a default set of HTTP ports (namely 80 and 443)
#        or (if passed) the ports included in the list of the default_ports
#        parameter.
#
# @note  In contrast to @ref get_http_port this function will not fork
#        and return a list instead. This should be used e.g. for
#        Detection-VTs where we want to make sure that we're not getting
#        blocked by the remote service with to many concurrent connections.
#
# @param default_list        The list of ports to return (overwriting the internal defaults) if "unscanned_closed"
#                            is set to "no" and no ports have been found open.
# @param host                The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#                            parameter or get_host_name) of the remote server. If no host parameter
#                            is passed http_host_name will be called automatically and a debug
#                            message is logged into the KB (currently commented out).
# @param ignore_broken       Returns the ports even if they are known to be "broken".
# @param ignore_unscanned    Returns the ports even if they where "unscanned" previously.
# @param ignore_cgi_disabled Returns the ports even if CGI scanning is disabled.
# @param dont_use_vhosts     Allows to specify if http_host_name is called internally (which would fork on multiple vhosts).
#
# @return The open / found HTTP ports, if none where found an empty list. In the case of no found ports and
#         "unscanned_closed" is set to "no" a set of default ports or the list passed to the default_ports
#         parameter.
#
function http_get_ports( default_list, host, ignore_broken, ignore_unscanned, ignore_cgi_disabled, dont_use_vhosts ) {

  local_var default_list, host, ignore_broken, ignore_unscanned, ignore_cgi_disabled, dont_use_vhosts;
  local_var final_port_list, check_port_list, default_ports, num_ports, ports, port;

  final_port_list = make_list();
  check_port_list = make_list();
  default_ports = make_list( 80, 443 );
  num_ports = 0;

  if( ! ignore_cgi_disabled ) {
    if( http_is_cgi_scan_disabled() ) {
      if( optimize_test_enabled && "yes" >< optimize_test_enabled ) {
        set_kb_item( name:"nvt_cgi_scanning_disabled/" + get_script_oid(), value:get_script_oid() + "#-#http_get_ports" );
      }
      return final_port_list;
    }
  }

  ports = get_kb_list( "Services/www" );
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

  if( ! dont_use_vhosts && ! host ) {
    host = http_host_name( dont_add_port:TRUE );
    #set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_get_ports" );
  } else if( ! host ) {
    host = "*";
  }

  foreach port( check_port_list ) {

    if( ! ignore_unscanned && ! get_port_state( port ) )
      continue;

    # Includes e.g. PJL ports which are printing everything
    # sent to them so don't include them here.
    if( http_is_fragile_port( port:port ) )
      continue;

    if( ! ignore_broken ) {
      if( http_get_is_marked_broken( port:port, host:host ) )
        continue;
    }

    final_port_list = make_list( final_port_list, port );
  }
  return final_port_list;
}

# @brief Deprecated / unused. Document this if the will be used again.
#
# Usage:
# banner = get_http_banner( port:port );
# if( php_ver_match( banner:banner, pattern:".*PHP/((3.*)|(4\.0.*)|(4\.1\.[01].*))"))
#   security_message( port );
#
function php_ver_match( banner, pattern ) {

  local_var banner, pattern;
  local_var line;

  if( ! banner ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#banner#-#php_ver_match" );
    return NULL;
  }

  if( ! pattern ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#pattern#-#php_ver_match" );
    return NULL;
  }

  line = egrep( pattern:"^Server:.*", string:banner, icase:TRUE );

  if( ereg( pattern:pattern, string:line, icase:TRUE ) ) {
    return TRUE;
  } else {
    line = egrep( pattern:"^X-Powered-By:.*", string:banner, icase:TRUE );
    if( ereg( pattern:pattern, string:line, icase:TRUE ) ) {
      return TRUE;
    }
  }

  return FALSE;
}

# @brief Checks if the remote web server is responding to a HTTP GET request.
#
# @param port  The port of the remote server to check.
# @param retry The amount of retries until the check is considered as failed.
#              This parameter is optional and defaults to 2 retries and is
#              limited to a max. of 5 retries.
#
# @return TRUE if the remote web server is not responding, FALSE otherwise and NULL if no port was passed.
#
function http_is_dead( port, retry ) {

  local_var port, retry;
  local_var url, req, i, soc, code, h, h2, b;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_is_dead" );
    return NULL;
  }

  if( ! retry )
    retry = 2;

  if( retry > 5 )
    retry = 5; # nb: Limit the amount of retries to a sane default

  # nb: http_head does not work against SWAT & VNC (& probably others...)
  # nb: Always keep http_get() before http_open_socket() as the first could
  # fork with multiple vhosts and the child's would share the same socket
  # causing race conditions and similar.
  url = strcat( "/OpenVASTest", rand(), ".html" );
  req = http_get( item:url, port:port );

  i = 0;
  soc = http_open_socket( port );
  while( ! soc && i++ < retry ) {
    sleep( i );
    soc = http_open_socket( port );
    if( _http_debug ) display( "DEBUG: i = ", i, "\n" );
  }

  if( ! soc )
    return TRUE;

  send( socket:soc, data:req );
  code = recv_line( socket:soc, length:1024 );
  if( code ) {
    h = http_recv_headers2( socket:soc );
    h2 = strcat( code, h );
    b = http_recv_body( socket:soc, headers:h2 );
  }
  http_close_socket( soc );
  if( ! code )
    return TRUE;

  # 500: internal server error
  # 501: not implemented = unsupported method...
  # 502: Bad gateway = upstream server sends an invalid response
  # 503: service unavailable = temporary overloading...
  # 504: gateway timeout = no timely response from upstream server
  if( ereg( pattern:"^HTTP/1\.[01] +50[234]", string:code ) )
    return TRUE;
  else
    return FALSE;
}

# @brief Requests a specified URL and checks the response if it contains a pattern for windows related files
#        (autoexec.bat, win.ini and boot.ini).
#
# @param port       The port of the remote server to check.
# @param url        The URL to check.
# @param quickcheck If set to TRUE the response is only checked for an existing HTTP 200 status code.
#
# @return TRUE if the remote web server has responded with the expected pattern, FALSE otherwise and
#         NULL in the case where no connection to the target could be established or not all
#         required parameters where passed.
#
function do_check_win_dir_trav( port, url, quickcheck ) {

  local_var port, url, quickcheck;
  local_var req, soc, cod, buf;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#do_check_win_dir_trav" );
    return NULL;
  }
  if( ! url ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#url#-#do_check_win_dir_trav" );
    return NULL;
  }

  if( _http_debug ) display( "DEBUG: check_win_dir_trav(port=", port, ", url=", url, ", quickcheck=", quickcheck, ")\n" );

  # nb: Always keep http_get() before http_open_socket() as the first could
  # fork with multiple vhosts and the child's would share the same socket
  # causing race conditions and similar.
  req = http_get( item:url, port:port );
  soc = http_open_socket( port );
  if( ! soc ) {
    if( _http_debug ) display( "DEBUG: check_win_dir_trav: cannot open socket to ", port, "\n" );
    return NULL;
  }

  send( socket:soc, data:req );
  cod = recv_line( socket:soc, length:80 );
  buf = http_recv( socket:soc, code:cod );
  http_close_socket( soc );

  if( "content-encoding: gzip" >< tolower( buf ) )
    buf = http_gunzip( buf:buf );

  if( quickcheck ) {
    if( " 200 " >< cod )
      return TRUE;
    else
      return FALSE;
  }

  if( "; for 16-bit app support" >< buf || "[boot loader]" >< buf )
    return TRUE;
  else
    return FALSE;
}

# @brief Requests a specified URL and checks the response if it contains
#        a pattern for windows related files (autoexec.bat, win.ini
#        and boot.ini). It is an extension to @ref do_check_win_dir_trav and
#        is additionally checking if the web server is responding with the
#        expected pattern on a non-existent random file.
#
# @param port       The port of the remote server to check.
# @param url        The URL to check.
# @param quickcheck If set to TRUE the response is only checked for an existing HTTP 200 status code.
#
# @return TRUE if the remote web server has responded with the expected pattern, FALSE otherwise and
#         NULL in the case where no connection to the target could be established, not all
#         required parameters where passed or if the web server is responding with the expected
#         pattern on a non-existent random file.
#
function check_win_dir_trav( port, url, quickcheck ) {
  local_var port, url, quickcheck;
  if( do_check_win_dir_trav( port:port, url:url + rand(), quickcheck:quickcheck ) )
    return NULL;
  else
    return do_check_win_dir_trav( port:port, url:url, quickcheck:quickcheck );
}

# @brief Receives the HTTP header from a specified socket.
#
# @param socket The existing socket to receive the header from.
#
# @return A string containing the HTTP header or NULL if none was
#         received or no socket parameter was passed.
#
function http_recv_headers2( socket ) {

  local_var socket;
  local_var counter, line, buf;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#http_recv_headers2" );
    return NULL;
  }

  while( TRUE ) {
    counter++;
    line = recv_line( socket:socket, length:4096 );
    buf += line;
    if( line == '\r\n' )
      break;
    if( ! strlen( line ) )
      break;
    if( strlen( line ) == 1 && line =~ '^\x0a$' )
      break;
    if( counter > 1024 )
      break;
  }

  return buf;
}

# @brief Receives the HTTP body from a specified socket.
#
# @param socket  The existing socket to receive the body from.
# @param headers A previously received header used instead of getting the header with a new request.
# @param length  The length / amount of data to receive. See the @note below for some more info.
#
# @return A string containing the HTTP body or NULL if none was
#         received or no socket parameter was passed.
#
# @note This function does not return the headers! So 'length' parameter does not include headers length, even if we
#       have to read them. If Content-length is set, "length" only allows the function to read more data, if available.
#       i.e., it is ignored most of the time.
#
function http_recv_body( socket, headers, length ) {

  local_var socket, headers, length;
  local_var h, l, cl, gzip, max, min, body, tmp, x, n;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#http_recv_body" );
    return NULL;
  }

  if( ! headers ) {
    h = http_recv_headers2( socket:socket );
  } else {
    h = headers;
  }

  l = -1;
  cl = egrep( pattern:"^Content-length: *[0-9]+", string:h, icase:TRUE );
  if( "content-encoding: gzip" >< tolower( h ) )
    gzip = TRUE;

  if( cl )
    l = int( ereg_replace( pattern:"Content-length: *([0-9]+).*", replace:"\1", string:cl, icase:TRUE ) );

  # nb: "l" = Content-Length or -1 now

  max = -1;
  min = -1;

  if( l < 0 && egrep( pattern:"^transfer-encoding: chunked", string:h, icase:TRUE ) ) {

    body = "";

    while( 1 ) {
      tmp = recv_line( socket:socket, length:4096 );
      if( ! tmp )
        l = 0;
      else
        l = hex2dec( xvalue:tmp );
      body = strcat( body, recv( socket:socket, length:l, min:l ) );
      # "\r\n"
      recv( socket:socket, length:2, min:2 );
      if( l == 0 ) {
        return( body ); # This is expected - don't put this line before the previous
      }
    }
  }

  if( length )
    max = length;

  if( l >= 0 )
    min = int( l );
  if( l >= max || min >= max )
    max = l;

  if( max < 0 ) {
    if( _http_debug ) display( "DEBUG: http_recv_body: bogus or no Content-length field, and no 'length' parameter set! Defaulting to 32 KB\n" );
    max = 32768;
  }

  if( _http_debug ) display( "DEBUG: http_recv_body: min=", min, "; max=", max, "\n" );

  if( min > 0 ) {
    x = recv( socket:socket, length:max, min:min );
  } else {
    n = recv( socket:socket, min:max, length:max );
    x = n;
    while( strlen( n ) >= max && max != 0 ) {
      n = recv( socket:socket, length:max );
      x += n;
      if( strlen( x ) > 1048576 ) {
        if( _http_debug ) display( "DEBUG: http_recv_body: read stopped after 1 MB!\n" );
        break;
      }
    }
  }

  if( gzip )
    return http_gunzip( buf:x, onlybody:FALSE );
  else
    return( x );
}

# @brief Reads all received HTTP data (header and body) from a specified socket.
#
# @param socket The existing socket to receive the data from.
# @param code   TBD: What's the purpose of this parameter?
#
# @return A string containing the HTTP data or NULL if none was
#         received or no socket parameter was passed.
#
function http_recv( socket, code ) {

  local_var socket, code;
  local_var h, l, b;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#http_recv" );
    return NULL;
  }

  if( code ) {
    h = strcat( code ); # Convert to string, just in case
    repeat {
      l = recv_line( socket:socket, length:2048 );
      h += l;
    } until( ! l || l =~ '^[\r\n]+$' ); # EOF or empty line

    if( ! l )
      return h;

  } else {
    h = http_recv_headers2( socket:socket );

    if( ! h )
      return NULL;
    else if( ! ereg( pattern:"^HTTP/.* [0-9]*", string:h ) )
      return h;

    h = strcat( h, '\r\n' );
  }

  b = http_recv_body( socket:socket, headers:h, length:0 );
  return strcat( h, b );
}

# @brief Reads all received HTTP data (header and body) from a specified socket
#        up to the specified length.
#
# @param socket     The existing socket to receive the data from.
# @param bodylength The amount/length of the data to receive (optional).
#
# @return A string containing the HTTP data or NULL if none was
#         received or no socket parameter was passed.
#
# @note The bodylength will be ignored if the Content-length field is set.
#
function http_recv_length( socket, bodylength ) {

  local_var socket, bodylength;
  local_var h, b;

  if( ! socket ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#socket#-#http_recv_length" );
    return NULL;
  }

  h = http_recv_headers2( socket:socket );
  b = http_recv_body( socket:socket, headers:h, length:bodylength );
  return strcat( h, '\r\n', b );
}

# @brief Sends a HTTP request to the target and reads all received data (header and body) from
#        the response.
#
# @param port The port of the remote server to send the data to.
# @param data The data to sent to the remote server.
#
# @return A string containing the received HTTP data or NULL if none was
#         received or no port or data parameter was passed.
#
# @note A valid HTTP request needs to be passed to the data parameter. Some basic sanity checks are
#       done by this function and it will report any invalid data into the KB for later evaluation.
#
function http_send_recv( port, data ) {

  local_var port, data;
  local_var oid, s, x, cl, conlen, r, user_agent, oid_str;
  # nb: _http_ua_include_oid and _http_debug are global vars

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_send_recv" );
    return NULL;
  }

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#http_send_recv" );
    return NULL;
  }

  # nb: We should expect at least a request like: "GET / HTTP/1.1\r\n\r\n" (length: 18).
  # But for some specific cases we're just checking for some less data.
  if( strlen( data ) < 8 || data !~ "^(DELETE|PROPFIND|PUT|GET|HEAD|POST|OPTIONS|REPORT|MKCOL|MOVE|PROPPATCH|COPY|PATCH|CONNECT|TRACE|LOCK|UNLOCK|TRACK|M-POST|CHECKOUT|CHECKIN|UNCHECKOUT|VERSION-CONTROL|BASELINE-CONTROL).*HTTP/(1\.[01]|2)" ) {
    oid = get_script_oid();
    if( oid != "1.3.6.1.4.1.25623.1.0.900522" && # secpod_ziproxy_server_detect.nasl might require the wrong request to trigger the detection
        oid != "1.3.6.1.4.1.25623.1.0.10730" )   # pre2008/raptor_detect.nasl as well
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_send_recv(): Invalid HTTP request (length < 8, invalid HTTP method or missing HTTP/ header) passed in 'data' variable." );
  }

  # http_get() won't add a Host: header if only HTTP/1.0 support was detected
  # nb: Commented out for now as quite a lot older VTs are probably passing a HTTP request without the Host: header
  #if( "Host: " >!< data && " HTTP/1.0" >!< data ) {
  #  oid = get_script_oid();
  #  if( oid != "1.3.6.1.4.1.25623.1.0.802045" ) # nb: gb_linux_rootkit_nginx_iframe_injection.nasl requires a wrong host header
  #    set_kb_item( name:"nvt_debug_misc/" + oid, value:oid + "#-#http_send_recv(): No 'Host:' header passed in 'data' variable. This might lead to issues if SNI is enabled on the remote host." );
  #}

  if( data !~ "^(DELETE|PROPFIND|PUT|GET|HEAD|POST|OPTIONS|REPORT|MKCOL|MOVE|PROPPATCH|COPY|PATCH|CONNECT|TRACE|LOCK|UNLOCK|TRACK|M-POST|CHECKOUT|CHECKIN|UNCHECKOUT|VERSION-CONTROL|BASELINE-CONTROL) (/|\.+/|https?:|\*).* HTTP" ) {
    oid = get_script_oid();
    if( oid != "1.3.6.1.4.1.25623.1.0.103293" && # gb_apache_49957.nasl requires sending a @localhost URI
        oid != "1.3.6.1.4.1.25623.1.0.17230"  && # cern_httpd_access_ctrl.nasl requires sending a \ at the start of the URI
        oid != "1.3.6.1.4.1.25623.1.0.900522" && # secpod_ziproxy_server_detect.nasl might require the wrong request to trigger the detection
        oid != "1.3.6.1.4.1.25623.1.0.10730" )   # pre2008/raptor_detect.nasl as well
      set_kb_item( name:"nvt_debug_misc/" + oid, value:oid + "#-#http_send_recv(): URL/URI of the HTTP request passed in 'data' variable doesn't start with one of the following: '/, ./, http, *'." );
  }

  # Some (mostly older) VTs are not passing the User-Agent header but we should always have one.
  if( " HTTP/1.1" >< data && ! egrep( pattern:"^User-Agent:.+", string:data, icase:TRUE ) ) {
    data = ereg_replace( string:data, pattern:'\r\n\r\n', replace:'\r\nUser-Agent: ' + http_get_user_agent() + '\r\n\r\n' );
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_send_recv(): Using a HTTP/1.1 request without a 'User-Agent:' header passed in 'data' variable. Adding it automatically to the request." );
  }

  if( _http_ua_include_oid && user_agent = egrep( pattern:"^User-Agent:.+", string:data, icase:TRUE ) ) {
    user_agent = chomp( user_agent );
    oid_str = "(OID:" + get_script_oid() + ")";
    if( oid_str >!< user_agent )
      data = str_replace( string:data, find:user_agent, replace:user_agent + " " + oid_str );
  }

  if( _http_debug ) display( "DEBUG: http_send_recv( port: ", port, ", data: ", data, " )\n" );

  s = http_open_socket( port );
  if( ! s )
    return;

  send( socket:s, data:data );
  while( x = http_recv( socket:s ) ) {
    if( "content-length: " >< tolower( x ) && "206 Partial" >!< x ) {
      cl = eregmatch( pattern:"Content-Length: ([0-9]+)", string:x, icase:TRUE );
      if( ! isnull( cl[1] ) )
        conlen = int( cl[1] );
    }

    r += x;
    if( ( conlen && conlen > 0 ) && strlen( r ) >= conlen )
      break;
  }

  http_close_socket( s );

  if( "content-encoding: gzip" >< tolower( r ) )
    return http_gunzip( buf:r );
  else
    return r;
}

# @brief Returns the detected directories (e.g. by webmirror.nasl and/or
#        DDI_Directory_Scanner.nasl) on the remote web server. Additionally
#        it includes all user specified directories via the scanner preference
#        'cgi_path'. If CGI Scanning is is disabled the function will exit.
#
# @param port The port of the remote server for which the detected directories
#             should be returned. This parameter is optional and if not passed
#             the directories of all web servers on the remote host are returned.
# @param host The hostname of the remote server (e.g. from http_host_name with the
#             dont_add_port:TRUE parameter or get_host_name) for which the detected
#             directories should be returned. This parameter is optional and if not
#             passed the directories of all web servers and all hostnames of the
#             remote host are returned.
#
# @return The directories on the remote web server as defined in the description above.
#
function cgi_dirs( port, host ) {

  local_var port, host;
  local_var kb, usercgis;
  # nb: optimize_test_enabled is a global var

  if( ! port )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#cgi_dirs" );

  if( http_is_cgi_scan_disabled() ) {
    if( optimize_test_enabled && "yes" >< optimize_test_enabled ) {
      set_kb_item( name:"nvt_cgi_scanning_disabled/" + get_script_oid(), value:get_script_oid() + "#-#cgi_dirs()" );
    }
    #return NULL; # Old behaviour
    exit( 0 ); # Throws a "nasl_array_iterator: unhandled type 57" message if cgi_dirs() is called within a foreach but not within a make_list()
  }

  # TBD: Use http_host_name(dont_add_port:TRUE) if(!host) ?
  # This would require an additional parameter to skip this if
  # the NVT dev wants to get the directories of all hosts...

  if( port && host ) {
    kb = get_kb_list( "www/" + host + "/" + port + "/content/directories" );
  } else if( port && ! host ) {
    kb = get_kb_list( "www/*/" + port + "/content/directories" );
  } else if( ! port && host ) {
    kb = get_kb_list( "www/" + host + "/*/content/directories" );
  } else {
    kb = get_kb_list( "www/*/*/content/directories" );
  }

  usercgis = get_kb_list( "/user/cgis" );
  if( isnull( usercgis ) ) usercgis = "/";
  if( isnull( kb ) ) {
    kb = make_list( usercgis, "/" );
  } else {
    kb = make_list( usercgis, kb, "/" );
  }

  return( make_list_unique( kb ) );
}

# @brief Returns if a remote web server is able to host or support hosting PHP files.
#
# @param port The port of the remote server to check.
#
# @return TRUE if the remote web server is able to host or support hosting PHP files,
#         FALSE otherwise and NULL in the case where no port parameter was passed.
#
function can_host_php( port ) {

  local_var port;
  local_var key, can_host, files, banner, srv_pattern;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#can_host_php" );
    return NULL;
  }

  # nb: We're using "yes/no" for the keys as a FALSE won't be saved into the KB.
  key = "www/" + port + "/can_host_php";
  if( can_host = get_kb_item( key ) ) {
    if( can_host == "yes" )
      return TRUE;
    else
      return FALSE;
  }

  if( get_kb_item( "www/" + port + "/PHP" ) ) {
    set_kb_item( name:key, value:"yes" );
    return TRUE;
  }

  # nb: * in host is expected as webserver hosting .php* files on one vhost
  # is most likely supporting PHP on another vhost as well.
  files = http_get_kb_file_extensions( port:port, host:"*", ext:"php*" );
  if( ! isnull( files ) ) {
    set_kb_item( name:key, value:"yes" );
    return TRUE;
  }

  banner = get_http_banner( port:port );
  if( ! banner ) {
    set_kb_item( name:key, value:"no" );
    return FALSE; # Broken Web Server
  } else {

    if( egrep( pattern:"((powered|server).*php|set-cookie:.*phpsessid|content-type: application/x-appweb-php)", string:banner, icase:TRUE ) ) {
      set_kb_item( name:key, value:"yes" );
      return TRUE;
    }

    # TODO: Find more possible web server to add
    srv_pattern  = "(apache|nginx|thttpd|aolserver|pi3web|zeus|iis|icewarp|lighttpd|";
    srv_pattern += "hiawatha|litespeed|caddy|panweb|embedthis-http|embedthis-appweb|mbedthis-appweb)";

    if( egrep( pattern:"^Server:.*" + srv_pattern, string:banner, icase:TRUE ) ) {
      set_kb_item( name:key, value:"yes" );
      return TRUE;
    }
  }

  if( egrep( pattern:"^Server:.*", string:banner, icase:TRUE ) ) {
    set_kb_item( name:key, value:"no" );
    return FALSE; # Unknown web server, should be added above if we know that it can host a PHP website
  } else {
    if( egrep( pattern:"(user-agent: loolwsd wopi|x-powered-by: express)", string:banner, icase:TRUE ) ) {
      set_kb_item( name:key, value:"no" );
      return FALSE; # We know that this can't host a PHP website
    } else {
      set_kb_item( name:key, value:"yes" );
      return TRUE; # Server: banner hidden - might be able to host a PHP website
    }
  }
}

# @brief Returns if a remote web server is able to host or support hosting .asp and/or .aspx files.
#
# @param port The port of the remote server to check.
#
# @return TRUE if the remote web server is able to host or support hosting .asp and/or .aspx files,
#         FALSE otherwise and NULL in the case where no port parameter was passed.
#
function can_host_asp( port ) {

  local_var port;
  local_var key, can_host, files, banner, srv_pattern;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#can_host_asp" );
    return NULL;
  }

  # nb: We're using "yes/no" for the keys as a FALSE won't be saved into the KB.
  key = "www/" + port + "/can_host_asp";
  if( can_host = get_kb_item( key ) ) {
    if( can_host == "yes" )
      return TRUE;
    else
      return FALSE;
  }

  # nb: * in host is expected as webserver hosting .asp* files on one vhost
  # is most likely supporting .asp/.aspx on another vhost as well.
  files = http_get_kb_file_extensions( port:port, host:"*", ext:"asp*" );
  if( ! isnull( files ) ) {
    set_kb_item( name:key, value:"yes" );
    return TRUE;
  }

  banner = get_http_banner( port:port );
  if( ! banner ) {
    set_kb_item( name:key, value:"no" );
    return FALSE; # Broken Web Server
  } else {

    if( egrep( pattern:"((powered|server).*asp|set-cookie:.*(asp\.net_sessionid|aspsessionid)|x-aspnet-version)", string:banner, icase:TRUE ) ) {
      set_kb_item( name:key, value:"yes" );
      return TRUE;
    }

    srv_pattern = "(iis|goahead-webs)"; # TODO: Find more possible web server to add

    if( egrep( pattern:"^Server:.*" + srv_pattern, string:banner, icase:TRUE ) ) {
      set_kb_item( name:key, value:"yes" );
      return TRUE;
    }
  }

  if( egrep( pattern:"^Server:.*", string:banner, icase:TRUE ) ) {
    set_kb_item( name:key, value:"no" );
    return FALSE; # Unknown web server, should be added above if we know that it can host a ASP/ASP.NET website
  } else {
    if( egrep( pattern:"(user-agent: loolwsd wopi|x-powered-by: express)", string:banner, icase:TRUE ) ) {
      set_kb_item( name:key, value:"no" );
      return FALSE; # We know that this can't host a ASP/ASP.NET website
    } else {
      set_kb_item( name:key, value:"yes" );
      return TRUE; # Server: banner hidden - might be able to host a ASP/ASP.NET website
    }
  }
}

# @brief Unzips the received "content-encoding: gzip" HTTP data with gunzip.
#
# @param buf      The data to gunzip.
# @param onlybody If set to TRUE only the HTTP body is unzipped and returned.
#
# @return A string containing the unzipped data, a string without unzipped data
#         if no "content-encoding: gzip" was found or if the data was already
#         unzipped by this function and FALSE if no buf parameter was passed.
#
function http_gunzip( buf, onlybody ) {

  local_var buf, onlybody;
  local_var lines, line, sep, header, body, h;

  if( ! buf ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#buf#-#http_gunzip" );
    return FALSE;
  }

  if( "##**##UNZIPPED##**##" >< buf )
    return buf;

  if( "content-encoding: gzip" >!< tolower( buf ) && ! onlybody )
    return buf;

  if( ! onlybody ) {

    lines = split( buf, keep:FALSE );

    foreach line( lines ) {

      if( ! sep && line !~ "^$" )
        header += line + '\n';

      if( line =~ "^$" && ! body ) {
        sep = TRUE;
        continue;
      }

      if( sep )
        body += line + '\n';
    }
  } else {
    body = buf;
  }

  if( ! body )
    return buf;

  if( body = gunzip( data:body ) ) {

    if( onlybody )
      return body + '\n\n\n##**##UNZIPPED##**##';

    h = ereg_replace( string:header, pattern:'(content-encoding:[^\r\n]+[\r\n]+)', replace:"", icase:TRUE );
    return( h + '\r\n\r\n' + body + '\n\n\n##**##UNZIPPED##**##' );
  }
  return buf;
}

# @brief Returns the hostname of the remote HTTP server. If the port parameter
#        is passed the function is also adding the port of the remote server
#        to the hostname if the port is not 80 and not 443. If no port is passed
#        the dont_add_port parameter needs to be set to TRUE.
#
# @param port          The port of the remote server.
# @param use_ip        If set to TRUE the IP of the remote server is returned, otherwise the hostname.
# @param dont_add_port If set to TRUE no port is added to the IP/hostname, otherwise the port is added.
#
# @return The hostname or IP of the remote server controlled by the described parameters.
#
function http_host_name( port, use_ip, dont_add_port ) {

  local_var port, use_ip, dont_add_port;
  local_var host;

  if( use_ip ) {
    host = get_host_ip();
  } else {
    host = get_host_name();
  }

  if( dont_add_port )
    return host;

  if( ! port )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_host_name" );

  if( port ) {
    # nb: Some IPS/WAF might block our requests if we're adding a port to the
    # Host: header so at least don't include it for 80 and 443 which complies
    # with the RFC.
    if( port != 80 && port != 443 )
      host += ':' + port;
  }

  return host;
}

# @brief Converts a passed list to a new list containing only
#        items which are not duplicated within the original list.
#
# @param _FCT_ANON_ARGS One or more strings or lists to convert into a single list.
#
# @return A list containing only items which are not duplicated within the original list
#         or an empty list if no list was passed.
#
function make_list_unique( ) {

  local_var ret, args, x, z, a, e, r;

  ret = make_list();
  args = make_list();

  foreach x( _FCT_ANON_ARGS ) {
    if( typeof( x ) == "array" ) { # e.g. return value from cgi_dirs()
      foreach z( x )
        args = make_list( args, z );
    } else {
      args = make_list( args, x );
    }
  }

  foreach a( args ) {

    e = FALSE;
    foreach r ( ret ) {
      if( a == r ) {
        e = TRUE; # entry already exist
        break;
      }
    }

    if( ! e )
      ret = make_list( ret, a ); # entry didn't exist, add entry...
  }

  return ret;
}

# @brief Checks and returns if a specified port was marked internally as "fragile".
#
# @param port The port of the remote server to check.
#
# @return TRUE if the port was marked internally as "fragile", FALSE otherwise
#         and NULL if no port parameter was specified.
#
# @note Duplicated from misc_func.inc to avoid that misc_func.inc needs to be included for every HTTP VT.
#
function http_is_fragile_port( port ) {

  local_var port;
  local_var fragile_ports, fragile_port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_is_fragile_port" );
    return NULL;
  }

  fragile_ports = get_kb_list( "Services/fragile_port" );
  if( ! fragile_ports )
    return FALSE;

  foreach fragile_port( fragile_ports ) {
    if( port == fragile_port )
      return TRUE;
  }

  return FALSE;
}

# @brief Extracts the url part of a "Location:" header from a 301/302 redirect response.
#        It makes also sure that the redirect is not pointing to a different server.
#
# Examples are e.g., for each "/foo" is returned.
# Location: https://example.com/foo (pointing to the same host)
# Location: http://example.com/foo (pointing to the same host)
# Location: //example.com/foo (pointing to the same host)
# Location: https://example.com/foo (pointing to a different host -> this needs to be ignored)
# Location: http://example.com/foo (to a different host -> this needs to be ignored)
# Location: //example.com/foo (to a different host -> this needs to be ignored)
# Location: /foo
#
# @param port  The port of the remote server.
# @param data  The data to extract the location from.
# @param debug If set to TRUE additional debugging infos on the location extraction is displayed on command line / the scanner log.
#
# @return The extracted location from the data or NULL if no "Location:" header was found, no port or data parameter
#         was passed or the redirect is pointing to a different host.
#
# @todo TBD: What to do if a redirect is pointing from e.g. 80 to 443?
#
function http_extract_location_from_redirect( port, data, debug ) {

  local_var port, data, debug;
  local_var host, ip, location, location_host, loc;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_extract_location_from_redirect" );
    return;
  }

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#http_extract_location_from_redirect" );
    return;
  }

  host = http_host_name( port:port );
  ip   = get_host_ip();

  # Exit if we don't have a location header in the data
  location = egrep( string:data, pattern:"^Location: ", icase:TRUE );
  if( ! location )
    return;

  if( debug || _http_debug ) display( 'DEBUG: Location header is: "' + location + '"\n' );

  # Grab everything after Location:
  location -= string( "\r\n" );
  location = ereg_replace( string:location, pattern:"Location: (.*)$", replace:"\1", icase:TRUE );

  if( debug || _http_debug ) display( 'DEBUG: Location header after cleanup is: "' + location + '"\n' );

  # Handle "https://", "http://" and "//" links in the location header
  if( location =~ "^(https:|http:)?//" ) {

    location_host = ereg_replace( string:location, pattern:"(https:|http:)?//([^/]*)/.*", replace:"\2", icase:TRUE );

    if( debug || _http_debug ) display( 'DEBUG: Location header is pointing to host/ip: "' + location_host + '"\n' );

    # The location header is pointing to the same hostname / ip
    if( host >< location_host || ip >< location_host ) {
      loc = ereg_replace( string:location, pattern:"(https:|http:)?//[^/]*/([^?]*)", replace:"/\2", icase:TRUE );
      if( debug || _http_debug ) display( 'DEBUG: Location header is pointing to "' + location + '" on the same host/ip. Returning the "' + loc + '" part of the location.\n' );
      return loc;
    # If its not pointing to the same hostname / ip we can't follow it
    } else {
      if( debug || _http_debug ) display( 'DEBUG: Location header is pointing to "' + location + '" NOT on the same host/ip. NOT returning this location.\n' );
      return;
    }
  # Handle redirects like "Location: /foo"
  } else {
    if( debug || _http_debug ) display( 'DEBUG: Location header is pointing to "' + location + '" on the same host/ip. Returning this location.\n' );
    return location;
  }
}

# @brief Extracts a HTTP cookie from a HTTP header.
#
# @param data    The data to extract the cookie from.
# @param pattern The regex pattern to use for extracting the cookie. Defaults to "Set-Cookie: ([^\r\n]+)".
#
# @return A string containing the extracted cookie or NULL if no cookie was found with the specified pattern
#         or if no buf parameter was passed.
#
function http_get_cookie_from_header( buf, pattern ) {

  local_var buf, pattern;
  local_var match;

  if( ! buf ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#buf#-#http_get_cookie_from_header" );
    return;
  }

  if( ! pattern )
    pattern = 'Set-Cookie: ([^\r\n]+)';

  if( match = eregmatch( pattern:pattern, string:buf ) )
    return match[ max_index( match ) -1 ];
  else
    return;
}

# @brief Creates a list of URLs from an array created by @ref http_get_kb_cgis and appends
#        the exploit request (passed via the ex parameter) to each of the URL parameter.
#
# @param cgiArray The list of URLs created by @ref http_get_kb_cgis.
# @param ex       The exploit request to append to each URL parameter.
#
# @return A list of URLs with the exploit request (passed via the ex parameter) appended to each of the
#         URL parameter or an empty list if no URL parameter was passed.
#
function http_create_exploit_req( cgiArray, ex ) {

  local_var cgiArray, ex;
  local_var urls, pseudocount, rrayval, tmpf, data, param, z, url, i;

  urls = make_array();

  pseudocount = 0;
  foreach rrayval( cgiArray ) {
    if( pseudocount >= 2 ) {
      if( "]" >< rrayval ) {
        pseudocount--;
        tmpf = ereg_replace( pattern:"\[|\]", string:rrayval, replace:"" );
        data[pseudocount] = tmpf;
      } else {
        param[pseudocount] = rrayval;
      }
    } else {
      param[pseudocount] = rrayval;
    }
    pseudocount++;
  }

  for( z = 2; z < max_index( param ); z++ ) {
    url = string( param[0], "?" );
    for( i = 2; i < max_index( param ); i++ ) {
      if( z == i ) {
        url += param[i] + "=" + ex;
      } else {
        if( data[i] ) {
          url += param[i] + "=" + data[i];
        } else {
          url += param[i] + "=";
        }
      }
      if( param[i + 1] ) {
        url += "&";
      }
    }
    urls = make_list( urls, url + "&" );
  }

  return urls;
}

# @brief Returns the Basic Authentication (Basic Auth) information from the passed data.
#
# @param data The data to extract the information from.
#
# @return An array containing the following index = value:
#
#         basic_auth = TRUE/FALSE if Basic Auth is available/found
#         realm      = A string containing the Basic Auth realm or "Undefined/Unknown" if none found
#
function http_extract_basic_auth( data ) {

  local_var data;
  local_var infos, header, realm;

  infos = make_array( "basic_auth", FALSE,
                      "realm", "Undefined/Unknown" );

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#http_extract_basic_auth" );
    return infos;
  }

  if( ! header = egrep( pattern:"^WWW-Authenticate: Basic", string:data, icase:TRUE ) )
    return infos;

  infos["basic_auth"] = TRUE;

  realm = eregmatch( pattern:"realm=(.*)", string:header, icase:TRUE );
  if( realm[1] )
    infos["realm"] = chomp( realm[1] );

  return infos;
}

# @brief Returns the info if a remote web server was internally (in the KB)
#        marked as "broken" by no404.nasl. It will also save the information
#        in an internal array to avoid that redis is queried again for the
#        same info.
#
# @param port The port of the remote server.
# @param host The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#             parameter or get_host_name) of the remote server. If no host parameter
#             is passed http_host_name will be called automatically and a debug
#             message is logged into the KB. A special "*" string can be passed
#             where the function will return TRUE if any of the remote hostnames
#             are marked as "broken".
#
# @return TRUE if the remote web server was marked as broken, FALSE otherwise and
#         NULL in the case where the port parameter wasn't passed.
#
function http_get_is_marked_broken( port, host ) {

  local_var port, host;
  local_var marked_broken_list, marked_broken, mb;
  # nb: _http_is_broken_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_get_is_marked_broken" );
    return NULL;
  }

  if( ! host ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_get_is_marked_broken" );
    host = http_host_name( dont_add_port:TRUE );
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _http_is_broken_array[host+port] ) ) {
    if( _http_is_broken_array[host+port] )
      return TRUE;
    else
      return FALSE;
  }

  if( host == "*" ) {
    marked_broken_list = get_kb_list( "www/*/" + port + "/is_broken" );
    if( ! marked_broken_list || typeof( marked_broken_list ) != "array" ) {
      marked_broken = FALSE;
    } else {
      foreach mb( marked_broken_list ) {
        if( mb ) {
          marked_broken = TRUE;
          break;
        }
      }
    }
  } else {
    marked_broken = get_kb_item( "www/" + host + "/" + port + "/is_broken" );
  }

  if( marked_broken ) {
    _http_is_broken_array[host+port] = TRUE;
  } else {
    _http_is_broken_array[host+port] = FALSE;
    marked_broken = FALSE;
  }

  return marked_broken;
}

# @brief Sets if a remote web server should be marked as "broken" to other NVTs.
#        The status is set within the following KB entry:
#        "www/" + host + "/" + port + "/is_broken"
#
# @param port The port of the remote server.
# @param host The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#             parameter or get_host_name) of the remote server.
#
# @return TRUE if the status was successfully saved within the KB and
#         NULL if no port or host parameter was passed.
#
function http_set_is_marked_broken( port, host ) {

  local_var port, host;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_set_is_marked_broken" );
    return NULL;
  }

  if( ! host ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_set_is_marked_broken" );
    return NULL;
  }

  set_kb_item( name:"www/" + host + "/" + port + "/is_broken", value:TRUE );
  _http_is_broken_array[host+port] = TRUE;
  return TRUE;
}

# @brief Returns all files identified on a remote web page and which are saved in
#        the knowledge base (by e.g. webmirror.nasl) within the following KB tree:
#        "www/" + host + "/" + port + "/content/extensions/".
#
# @param port The port of the remote server for which the detected files should
#             be returned. This parameter is optional and if not passed the
#             files of all web servers on the remote host are returned.
# @param host The hostname of the remote server (e.g. from http_host_name with the
#             dont_add_port:TRUE parameter or get_host_name) for which the detected
#             files should be returned. This parameter is optional and if not passed
#             the files of all web servers and all hostnames of the remote host are
#             returned.
# @param ext  The file extension for which the detected files should be returned.
#             A "php" passed to this parameter would e.g. return /index.php
#             and /index2.php. A wildcard like "php*" could be used as well which
#             would return /index.php and /index.php5. Similar a full wildcard like
#             "*" can be passed as well which would return all detected files. This
#             parameter is optional and if not passed the files of all web servers
#             and all hostnames of the remote host are returned.
#
# @return A list of files matching the specified parameters, NULL in the case no files
#         have matched the parameters or none have been identified at all.
#
# @note Add a dependency to at least DDI_Directory_Scanner.nasl and webmirror.nasl
#       when using this function within an NVT.
#
function http_get_kb_file_extensions( port, host, ext ) {

  local_var port, host, ext;
  local_var list, item;

  if( ! port )
    port = "*";

  if( ! host )
    host = "*";

  if( ! ext  )
    ext  = "*";

  list = get_kb_list( "www/" + host + "/" + port + "/content/extensions/" + ext );
  if( ! list || typeof( list ) != "array" )
    return NULL;

  if( max_index( list ) == 0 ) {
    foreach item( list )
      return list;

    return NULL;
  }

  return list;
}

# @brief Returns all files/directories requiring authentication on a remote web page and
#        which are saved in the knowledge base (by e.g. webmirror.nasl) within the following
#        KB tree: "www/" + host + "/" + port + "/content/auth_required".
#
# @param port The port of the remote server for which the detected files/directories
#             should be returned. This parameter is optional and if not passed the
#             files/directories of all web servers on the remote host are returned.
# @param host The hostname of the remote server (e.g. from http_host_name with the
#             dont_add_port:TRUE parameter or get_host_name) for which the detected
#             files/directories should be returned. This parameter is optional and if
#             not passed the files/directories of all web servers and all hostnames
#             of the remote host are returned.
#
# @return A list of files/directories requiring authentication, NULL in the case no files/
#         directories have matched the parameters or none have been identified at all.
#
# @note Add a dependency to at least DDI_Directory_Scanner.nasl and webmirror.nasl
#       when using this function within an NVT.
#
function http_get_kb_auth_required( port, host ) {

  local_var port, host;
  local_var list, item;

  if( ! port )
    port = "*";

  if( ! host )
    host = "*";

  list = get_kb_list( "www/" + host + "/" + port + "/content/auth_required" );
  if( ! list || typeof( list ) != "array" )
    return NULL;

  if( max_index( list ) == 0 ) {
    foreach item( list )
      return list;

    return NULL;
  }

  return list;
}

# @brief Returns all CGIs (namely scripts with a parameter) on a remote web page and
#        which are saved in the knowledge base (by e.g. webmirror.nasl) within the
#        following KB tree: "www/" + host + "/" + port + "/content/cgis/plain_cgis".
#        The returned syntax is the following:
#        /url - var [] -> This would match to http://example.com/url?var=
#        /url - var [param] -> This would match to http://example.com/url?var=param
#
# @param port The port of the remote server for which the detected CGIs should
#             be returned. This parameter is optional and if not passed the
#             CGIs of all web servers on the remote host are returned.
# @param host The hostname of the remote server (e.g. from http_host_name with the
#             dont_add_port:TRUE parameter or get_host_name) for which the detected
#             CGIs should be returned. This parameter is optional and if not passed
#             the CGIs of all web servers and all hostnames of the remote host are
#             returned.
#
# @return A list of CGIs in the syntax shown in the function description, NULL in the
#         case no CGIs have matched the parameters or none have been identified at all.
#
# @note Add a dependency to at least DDI_Directory_Scanner.nasl and webmirror.nasl
#       when using this function within an NVT.
#
function http_get_kb_cgis( port, host ) {

  local_var port, host;
  local_var list, item;

  if( ! port )
    port = "*";

  if( ! host )
    host = "*";

  list = get_kb_list( "www/" + host + "/" + port + "/content/cgis/plain_cgis" );
  if( ! list || typeof( list ) != "array" )
    return NULL;

  if( max_index( list ) == 0 ) {
    foreach item( list )
      return list;

    return NULL;
  }

  return list;
}

# @brief Returns a string which was identified by no404.nasl for a remote web server
#        returning a 200 HTTP (Found) status code for non-existing pages. The string
#        contains a pattern like e.g. "not found" which can be used by a NVT as a sign
#        that the web server hasn't returned the expected result about the existence
#        of a file/url. It will also save the information in an internal array to avoid
#        that redis is queried again for the same info.
#
# @param port The port of the remote server.
# @param host The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#             parameter or get_host_name) of the remote server. If no host parameter
#             is passed http_host_name will be called automatically and a debug
#             message is logged into the KB. A special "*" string can be passed
#             where the function will return the string of the first hostname.
#
# @return An identified string if the remote web server is returning a 200 status code
#         to non-existing pages, FALSE otherwise and NULL in the case where the port
#         parameter wasn't passed.
#
# @todo Maybe implement an additional "subdir" check as some pages might return different
#       strings based on different subdirs. This would require an update to no404.nasl as well.
#
function http_get_no404_string( port, host ) {

  local_var port, host;
  local_var no404_string, no404_string_list, _no404_string;
  # nb: _http_no404_string_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_get_no404_string" );
    return NULL;
  }

  if( ! host ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_get_no404_string" );
    host = http_host_name( dont_add_port:TRUE );
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _http_no404_string_array[host+port] ) ) {
    if( no404_string = _http_no404_string_array[host+port] )
      return no404_string;
    else
      return FALSE;
  }

  if( host == "*" ) {
    no404_string_list = get_kb_list( "www/*/" + port + "/no404_string" );
    if( ! no404_string_list || typeof( no404_string_list ) != "array" ) {
      no404_string = FALSE;
    } else {
      foreach _no404_string( no404_string_list ) {
        if( _no404_string && strlen( _no404_string > 0 ) ) {
          no404_string = _no404_string;
          break;
        }
      }
    }
  } else {
    no404_string = get_kb_item( "www/" + host + "/" + port + "/no404_string" );
  }

  if( no404_string && strlen( no404_string ) > 0 )
    _http_no404_string_array[host+port] = no404_string;
  else
    _http_no404_string_array[host+port] = FALSE;

  return no404_string;
}

# @brief Sets a specific string which should be assumed by NVTs as a sign for a
#        "file not found" if a remote web server isn't responding with a 200 HTTP
#        status code to non-existent files. The string is set within the following
#        KB entry: "www/" + host "/" + port + "/no404_string"
#
# @param port   The port of the remote server.
# @param host   The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#               parameter or get_host_name) of the remote server.
# @param string The string to set within the mentioned KB entry.
#
# @return TRUE if the string was successfully saved within the KB and
#         NULL if no port, host or string parameter was passed.
#
# @todo See the todo for @ref http_get_no404_string above.
#
function http_set_no404_string( port, host, string ) {

  local_var port, host, string;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_set_no404_string" );
    return NULL;
  }

  if( ! host ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_set_no404_string" );
    return NULL;
  }

  if( ! string ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#string#-#http_set_no404_string" );
    return NULL;
  }

  set_kb_item( name:"www/" + host + "/" + port + "/no404_string", value:string );
  return TRUE;
}

# @brief Checks if a specific passed data contains a 40x HTTP status code.
#        If no such status code was identified the KB is checked additionally
#        (by using @ref http_get_no404_string) if the remote web server is
#        responding with a 200 HTTP status code to a non-existent file/url.
#
# @param port The port of the remote server.
# @param host The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#             parameter or get_host_name) of the remote server.
# @param data The data which is checked in form of a HTTP response.
#
# @return TRUE if the passed data contains a 40x HTTP status code or a string
#         as described in @ref http_get_no404_string was identified by no404.nasl,
#         FALSE otherwise and NULL if no port, host or data parameter was passed
#         or the data parameter doesn't contain a valid HTTP response.
#
# @todo See the todo for @ref http_get_no404_string above.
#
function http_data_has_40x( port, host, data ) {

  local_var port, host, data;
  local_var no404;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_data_has_40x" );
    return NULL;
  }

  if( ! host ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_data_has_40x" );
    return NULL;
  }

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#http_data_has_40x" );
    return NULL;
  }

  if( ! ereg( string:data, pattern:"^HTTP/1\.[01] +[0-9]+" ) ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#No data containing a valid HTTP status code passed in data variable of http_data_has_40x." );
    return NULL;
  }

  if( ereg( string:data, pattern:"^HTTP/1\.[01] +40[0-9]" ) )
    return TRUE;

  no404 = http_get_no404_string( port:port, host:host );
  if( no404 && no404 >< data )
    return TRUE;
  else
    return FALSE;
}

# @brief Checks if a specific passed data contains a 200 HTTP status code.
#        If no such status code was identified the KB is checked additionally
#        (by using @ref http_get_no404_string) if the remote web server is
#        responding with a 200 HTTP status code to a non-existent file/url.
#
# @param port The port of the remote server.
# @param host The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#             parameter or get_host_name) of the remote server.
# @param data The data which is checked in form of a HTTP response.
#
# @return TRUE if the passed data contains a 200 HTTP status code or a string
#         as described in @ref http_get_no404_string was identified by no404.nasl,
#         FALSE otherwise and NULL if no port, host or data parameter was passed
#         or the data parameter doesn't contain a valid HTTP response.
#
# @todo See the todo for @ref http_get_no404_string above.
#
function http_data_has_200( port, host, data ) {

  local_var port, host, data;
  local_var no404;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_data_has_200" );
    return NULL;
  }

  if( ! host ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_data_has_200" );
    return NULL;
  }

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#http_data_has_200" );
    return NULL;
  }

  if( ! ereg( string:data, pattern:"^HTTP/1\.[01] +[0-9]+" ) ) {
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#No data containing a valid HTTP status code passed in data variable of http_data_has_200." );
    return NULL;
  }

  if( ereg( string:data, pattern:"^HTTP/1\.[01] +200" ) ) {
    no404 = http_get_no404_string( port:port, host:host );
    if( ! no404 || no404 >!< data )
      return TRUE;
  }

  return FALSE;
}

# @brief Creates and returns report containing the URL to a "vulnerable" location on the
#        remote web server.
#
# @param port     The port of the remote server.
# @param url      The URL to the "vulnerable" location which should be included in the report
# @param url_only If set to TRUE the "Vulnerable url:" text won't be added to the report
#                 and only the plain URL is returned within the report.
#
# @return A string containing the generated report or NULL if no port or url parameter
#         was passed.
#
function report_vuln_url( port, url, url_only ) {

  local_var port, url, url_only;
  local_var proto, host, report;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#report_vuln_url" );
    return NULL;
  }

  if( ! url ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#url#-#report_vuln_url" );
    return NULL;
  }

  proto = "http";
  if( get_port_transport( port ) > ENCAPS_IP )
    proto = "https";

  host = http_host_name( port:port );

  if( url_only ) {
    report = proto + "://" + host + url;
  } else {
    report = "Vulnerable url: " + proto + "://" + host + url;
  }
  return report;
}

# @brief Crafts a raw HTTP POST request (without sending it to the target) and returns it as a string.
#
# @param port               The port of the remote server.
# @param url                The URL to request, if not passed "/" will be used.
# @param data               The data which needs to be added in the body of the HTTP POST request.
# @param add_headers        Additional headers which should be added to the HTTP POST request. Needs to be an
#                           array in form of e.g. make_array( "Content-Type", "application/x-www-form-urlencoded" )
# @param accept_header      The Accept: header to use (e.g. application/json), defaults to "*/*" if not passed.
# @param user_agent         The User-Agent to use within the User-Agent: header, defaults to the result of the
#                           http_get_user_agent() function if not passed. If this parameter is given the function
#                           will add a separate X-Scanner: header field by default. This behavior can be disabled
#                           via the dont_add_xscanner parameter set to TRUE.
# @param host_header_use_ip If set to TRUE the request will contain the IP instead of the target
#                           hostname in the Host: header. This can be used for special targets not
#                           accepting the hostname in the request.
# @param dont_add_xscanner  Can be used in conjunction with the user_agent parameter. If set to TRUE the function
#                           is not adding a separate X-Scanner: header field to the request.
#
# @return A string containing the raw HTTP POST request or NULL if no port parameter was passed or
#         the data passed in the add_headers parameter isn't an array.
#
# @example req = http_post_req( port:port, url:"/", data:data, accept_header:"application/json", add_headers:make_array( "Content-Type", "application/x-www-form-urlencoded" ) );
#
function http_post_req( port, url, data, add_headers, accept_header, user_agent, host_header_use_ip, dont_add_xscanner ) {

  local_var port, url, data, add_headers, accept_header, user_agent, host_header_use_ip, dont_add_xscanner;
  local_var x_header, len, vtstrings, x_scanner_string, host, req, header;

  if( ! isnull( add_headers ) ) {
    if( typeof( add_headers ) != "array" ) {
      set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#add_headers_no-array#-#http_post_req" );
      return;
    }
  }

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_post_req" );
    return;
  }

  if( ! url )
    url = "/";

  x_header = FALSE;

  if( data )
    len = strlen( data );

  if( ! accept_header )
    accept_header = '*/*';

  if ( ! user_agent ) {
    user_agent = http_get_user_agent();
  } else if( ! dont_add_xscanner ) {
    x_header = TRUE;
    vtstrings = get_vt_strings();
    if( OPENVAS_VERSION )
      x_scanner_string = "X-Scanner: " + vtstrings["default"] + " " + OPENVAS_VERSION;
    else
      x_scanner_string = "X-Scanner: " + vtstrings["default"];
  }

  if( host_header_use_ip )
    host = http_host_name( port:port, use_ip:TRUE );
  else
    host = http_host_name( port:port );

  req = 'POST ' + url + ' HTTP/1.1\r\n' +
        'Host: ' + host + '\r\n' +
        'Pragma: no-cache\r\n' +
        'User-Agent: ' + user_agent + '\r\n' +
        'Accept-Language: en\r\n' +
        'Accept-Charset: iso-8859-1,utf-8;q=0.9,*;q=0.1\r\n' +
        'Accept: ' + accept_header + '\r\n' +
        'Accept-Encoding: identity\r\n';

  if( data )
    req += 'Content-Length: ' + len + '\r\n';

  if( x_header )
    req += x_scanner_string + '\r\n';

  if( ! isnull( add_headers ) ) {
    foreach header( keys( add_headers ) )
      req += header +': ' + add_headers[header] + '\r\n';
  }

  req += '\r\n';

  if( data )
    req += data;

  return req;
}

# @brief Crafts a raw HTTP GET request (without sending it to the target) and returns it as a string.
#
# @param port               The port of the remote server.
# @param url                The URL to request, if not passed "/" will be used.
# @param add_headers        Additional headers which should be added to the HTTP GET request. Needs to be an
#                           array in form of e.g. make_array( "Cookie", "PHPSESSID=" + sessid )
# @param accept_header      The Accept: header to use (e.g. text/html), defaults to the following if not passed:
#                           image/gif, image/x-xbitmap, image/jpeg, image/pjpeg, image/png, */*
# @param user_agent         The User-Agent to use within the User-Agent: header, defaults to the result of the
#                           http_get_user_agent() function if not passed. If this parameter is given the function
#                           will add a separate X-Scanner: header field by default. This behavior can be disabled
#                           via the dont_add_xscanner parameter set to TRUE.
# @param host_header_use_ip If set to TRUE the request will contain the IP instead of the target
#                           hostname in the Host: header. This can be used for special targets not
#                           accepting the hostname in the request.
# @param dont_add_xscanner  Can be used in conjunction with the user_agent parameter. If set to TRUE the function
#                           is not adding a separate X-Scanner: header field to the request.
#
# @return A string containing the raw HTTP GET request or NULL if no port parameter was passed or
#         the data passed in the add_headers parameter isn't an array.
#
# @example req = http_get_req( port:port, url:"/", accept_header:"text/html", add_headers:make_array( "Cookie", "PHPSESSID=" + sessid ) );
#
function http_get_req( port, url, add_headers, accept_header, user_agent, host_header_use_ip, dont_add_xscanner ) {

  local_var port, url, add_headers, accept_header, user_agent, host_header_use_ip, dont_add_xscanner;
  local_var x_header, vtstrings, x_scanner_string, host, req, header;

  if( ! isnull( add_headers ) ) {
    if( typeof( add_headers ) != "array" ) {
      set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#add_headers_no-array#-#http_get_req" );
      return;
    }
  }

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_get_req" );
    return;
  }

  if( ! url )
    url = "/";

  x_header = FALSE;

  if( ! accept_header )
    accept_header = 'image/gif, image/x-xbitmap, image/jpeg, image/pjpeg, image/png, */*';

  if ( ! user_agent ) {
    user_agent = http_get_user_agent();
  } else if( ! dont_add_xscanner ) {
    x_header = TRUE;
    vtstrings = get_vt_strings();
    if( OPENVAS_VERSION )
      x_scanner_string = "X-Scanner: " + vtstrings["default"] + " " + OPENVAS_VERSION;
    else
      x_scanner_string = "X-Scanner: " + vtstrings["default"];
  }

  if( host_header_use_ip )
    host = http_host_name( port:port, use_ip:TRUE );
  else
    host = http_host_name( port:port );

  req = 'GET ' + url + ' HTTP/1.1\r\n' +
        'Connection: Close\r\n' +
        'Host: ' + host + '\r\n' +
        'Pragma: no-cache\r\n' +
        'Cache-Control: no-cache\r\n' +
        'User-Agent: ' + user_agent + '\r\n' +
        'Accept: ' + accept_header + '\r\n' +
        'Accept-Language: en\r\n' +
        'Accept-Charset: iso-8859-1,*,utf-8\r\n';

  if( x_header )
    req += x_scanner_string + '\r\n';

  if( ! isnull( add_headers ) ) {
    foreach header( keys( add_headers ) )
      req += header +': ' + add_headers[header] + '\r\n';
  }

  req += '\r\n';

  return req;
}

# @brief Returns the info if a "generic" XSS was found on the remote web server
#        to avoid that e.g. more then one NVT is reporting the same XSS multiple
#        times. It will also save the information in an internal array to avoid
#        that redis is queried again for the same info.
#
# @param port The port of the remote server.
# @param host The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#             parameter or get_host_name) of the remote server. If no host parameter
#             is passed http_host_name will be called automatically and a debug
#             message is logged into the KB. A special "*" string can be passed
#             where the function will return TRUE if a "generic" XSS was found on
#             any of the remote hostnames.
#
# @return TRUE if the remote web server has a "generic" XSS, FALSE otherwise and
#         NULL in the case where the port parameter wasn't passed.
#
function http_get_has_generic_xss( port, host ) {

  local_var port, host;
  local_var generic_xss_list, generic_xss, gxss;
  # nb: _http_has_generic_xss_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_get_has_generic_xss" );
    return NULL;
  }

  if( ! host ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_get_has_generic_xss" );
    host = http_host_name( dont_add_port:TRUE );
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _http_has_generic_xss_array[host+port] ) ) {
    if( _http_has_generic_xss_array[host+port] )
      return TRUE;
    else
      return FALSE;
  }

  if( host == "*" ) {
    generic_xss_list = get_kb_list( "www/*/" + port + "/generic_xss" );
    if( ! generic_xss_list || typeof( generic_xss_list ) != "array" ) {
      generic_xss = FALSE;
    } else {
      foreach gxss( generic_xss_list ) {
        if( gxss ) {
          generic_xss = TRUE;
          break;
        }
      }
    }
  } else {
    generic_xss = get_kb_item( "www/" + host + "/" + port + "/generic_xss" );
  }

  if( generic_xss )
    _http_has_generic_xss_array[host+port] = TRUE;
  else
    _http_has_generic_xss_array[host+port] = FALSE;

  return generic_xss;
}

# @brief Sets the info if a "generic" XSS was found on the remote web server to
#        avoid that e.g. more then one NVT is reporting the same XSS multiple
#        times. The status is set within the following KB entry:
#        "www/" + host + "/" + port + "/generic_xss"
#
# @param port The port of the remote server
# @param host The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#             parameter or get_host_name) of the remote server.
#
# @return TRUE if the status was successfully saved within the KB and
#         NULL if no port or host parameter was passed.
#
# @todo Maybe also save the "location" of the generic XSS?
#
function http_set_has_generic_xss( port, host ) {

  local_var port, host;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_set_has_generic_xss" );
    return NULL;
  }

  if( ! host ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_set_has_generic_xss" );
    return NULL;
  }

  set_kb_item( name:"www/" + host + "/" + port + "/generic_xss", value:TRUE );
  return TRUE;
}

# @brief Returns the info if "CGI Scanning" was disabled (Settings/disable_cgi_scanning of global_settings.nasl) within the
#        scan config. To avoid querying the KB for the same info again it will be cached within an internal variable.
#
# @return TRUE if CGI scanning is disabled, FALSE otherwise.
#
function http_is_cgi_scan_disabled() {

  local_var is_cgi_scan_disabled;
  # nb: __http_is_cgi_scan_disabled is a global var

  if( ! isnull( __http_is_cgi_scan_disabled ) ) {
    is_cgi_scan_disabled = __http_is_cgi_scan_disabled;
  } else {
    is_cgi_scan_disabled = get_kb_item( "Settings/disable_cgi_scanning" );
    if( is_cgi_scan_disabled )
      __http_is_cgi_scan_disabled = TRUE;
    else
      __http_is_cgi_scan_disabled = FALSE;
    is_cgi_scan_disabled = __http_is_cgi_scan_disabled;
  }
  return is_cgi_scan_disabled;
}

# @brief Returns the info if a remote web server was internally (in the KB)
#        marked as being an embedded web server. It will also save the information
#        in an internal array to avoid that redis is queried again for the
#        same info.
#
# @param port The port of the remote server.
#
# @return TRUE if the remote web server was marked as embedded, FALSE otherwise and
#         NULL in the case where the port parameter wasn't passed.
#
function http_get_is_marked_embedded( port ) {

  local_var port;
  local_var marked_embedded_list, marked_embedded;
  # nb: _http_is_embedded_array is a global_var (see on top)

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_get_is_marked_embedded" );
    return NULL;
  }

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( _http_is_embedded_array[port] ) ) {
    if( _http_is_embedded_array[port] )
      return TRUE;
    else
      return FALSE;
  }

  marked_embedded = get_kb_item( "www/" + port + "/is_embedded" );
  if( marked_embedded ) {
    _http_is_embedded_array[port] = TRUE;
  } else {
    _http_is_embedded_array[port] = FALSE;
    marked_embedded = FALSE;
  }

  return marked_embedded;
}

# @brief Sets if a remote web server should be marked as "embedded" to other NVTs.
#        The status is set within the following KB entry:
#        "www/" + host + "/" + port + "/is_embedded"
#
# @param port The port of the remote server.
#
# @return TRUE if the status was successfully saved within the KB and
#         NULL if no port or host parameter was passed.
#
function http_set_is_marked_embedded( port ) {

  local_var port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_set_is_marked_embedded" );
    return NULL;
  }

  set_kb_item( name:"www/" + port + "/is_embedded", value:TRUE );
  _http_is_embedded_array[port] = TRUE;
  return TRUE;
}