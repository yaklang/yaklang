###############################################################################
# OpenVAS Include File
# $Id: http_keepalive.inc 13843 2019-02-25 10:11:55Z cfischer $
#
# Methods to support http keep alive handling
#
# Authors:
# Renaud Deraison
#
# Copyright:
# Copyright (C) 2005 Renaud Deraison
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

# Note that the file "http_func.inc" must also be included when
# using this file.

global_var __ka_socket, __ka_port, __ka_enabled, __ka_last_request;

__ka_socket = 0;
__ka_port   = 0;
__ka_enabled = -1;
__ka_last_request = "";

# Enables debugging output for a few functions below
global_var _http_ka_debug;
_http_ka_debug = FALSE;

# Enables to include the OID of the VT within the HTTP user agent of http_keepalive_send_recv()
# for debugging purposes on the target / targets logfiles.
global_var _http_ua_include_oid;
_http_ua_include_oid = FALSE;

# Global var passed to a few functions below
global_var optimize_test_enabled;
optimize_test_enabled = get_preference( "optimize_test" );

# @brief Determined on the passed header if the keep-alive socket of the
#        connection needs to be close and re-opened.
#
# @param headers The headers to evaluate.
#
# @return NULL
#
function http_keepalive_check_connection( headers ) {

  local_var headers;
  local_var tmp;

  if( ! headers ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#headers#-#http_keepalive_check_connection" );
    return NULL;
  }

  tmp = egrep( pattern:"^Connection: Close", string:headers, icase:TRUE );
  if( tmp ) {
    if( __ka_socket ) {
      http_close_socket( __ka_socket );
    }
    __ka_socket = http_open_socket( __ka_port );
  }
  return NULL;
}

# @brief Opens a new socket to the specified port and sets it
#        internallly as the port/socket to be used for keep-alive connections.
#
# @param port The port to open a connection to.
#
# @return NULL
#
function enable_keepalive( port ) {

  local_var port;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#enable_keepalive" );
    return NULL;
  }

  __ka_enabled = 1;
  __ka_port    = port;
  __ka_socket  = http_open_socket( port );
  return NULL;
}

# @brief Determine if the remote web server is keep-alive enabled or not.
#
# @param port The HTTP port of the target.
#
# @return 1 if keep alive is enabled, 0 otherwise and -2 if no port was passed or no socket could be opened to the target host.
#
function http_keepalive_enabled( port ) {

  local_var port;
  local_var kb, useragent, host, req, soc, r;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_keepalive_enabled" );
    return -2;
  }

  kb = get_kb_item( strcat( "www/", port, "/keepalive" ) );

  if( kb == "yes" ) {
    enable_keepalive( port:port );
    return( 1 );
  } else if( kb == "no" ) {
    return( 0 );
  }

  useragent = http_get_user_agent();

  # nb: Always keep http_host_name() before http_open_socket() as the first
  # could fork with multiple vhosts and the child's would share the same
  # socket causing race conditions and similar.
  host = http_host_name( port:port );

  req = strcat( 'GET / HTTP/1.1\r\n',
                'Connection: Keep-Alive\r\n',
                'Host: ', host, '\r\n',
                'Pragma: no-cache\r\n',
                'User-Agent: ', useragent, '\r\n\r\n' );

  soc = http_open_socket( port );
  if( ! soc )
    return -2;

  send( socket:soc, data:req );
  r = http_recv( socket:soc );

  # Apache
  if( egrep( pattern:"^Keep-Alive:.*", string:r, icase:TRUE ) ) {
    http_close_socket( soc );
    set_kb_item( name:strcat( "www/", port, "/keepalive" ), value:"yes" );
    enable_keepalive( port:port );
    return( 1 );
  } else {
    # IIS
    send( socket:soc, data:req );
    r = http_recv( socket:soc );
    http_close_socket( soc );
    if( strlen( r ) ) {
      set_kb_item( name:strcat( "www/", port, "/keepalive" ), value:"yes" );
      enable_keepalive( port:port );
      return( 1 );
    }
  }
  set_kb_item( name:strcat( "www/", port, "/keepalive" ), value:"no" );
  return( 0 );
}

# @brief This function is akin to @ref http_recv_body except that if the last request
#        was a HEAD, we bail out (whereas @ref http_recv will timeout).
#
# @param headers A previously received header used for evaluation within this function.
# @param bodyonly If set to TRUE only the HTTP body is returned. TBD: The function is called "recv_body" some this parameter doesn't make that much sense in this context...
#
# @return The received data as a string.
#
function http_keepalive_recv_body( headers, bodyonly ) {

  local_var headers, bodyonly;
  local_var killme, length, tmp, gzip, body;
  # nb: _http_ka_debug, __ka_port, __ka_last_request and __ka_socket are global vars

  if( ! headers )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#headers#-#http_keepalive_recv_body" );

  killme = 0;
  length = -1;

  if( ereg( pattern:"^HEAD.*HTTP/.*", string:__ka_last_request ) ) {
    # HEAD does not return a body
    http_keepalive_check_connection( headers:headers );
    if( bodyonly ) {
      return( "" );
    } else {
      return( headers );
    }
  }

  if( "content-length: " >< tolower( headers ) ) {
    tmp = egrep( string:headers, pattern:"^Content-Length: *[0-9]+", icase:TRUE );
    if( tmp )
      length = int( ereg_replace( string:tmp, pattern:"^Content-Length: *([0-9]*) *", replace:"\1", icase:TRUE ) );
  }

  if( "content-encoding: gzip" >< tolower( headers ) )
    gzip = TRUE;

  if( ( length < 0 ) && ( egrep( pattern:"transfer-encoding: chunked", string:headers, icase:TRUE ) ) ) {
    while( 1 ) {
      tmp = recv_line( socket:__ka_socket, length:4096 );
      if( ! tmp )
        length = 0;
      else
        length = hex2dec( xvalue:tmp );
      if( length > 1048576 ) {
        length = 1048576;
        killme = 1;
      }
      body = strcat( body, recv( socket:__ka_socket, length:length, min:length ) );
      # "\r\n"
      recv( socket:__ka_socket, length:2, min:2 );
      if( strlen( body ) > 1048576 )
        killme = 1;

      if( length == 0 || killme ) {
        http_keepalive_check_connection( headers:headers );
        # This is expected - don't put this line before the previous
        if( gzip )
          body = http_gunzip( buf:body, onlybody:TRUE );

        if( bodyonly ) {
          return( body );
        } else {
          return( strcat( headers, '\r\n', body ) );
        }
      }
    }
  }

  if( length >= 0 ) {
    # Don't receive more than 1 MB
    if( length > 1048576 ) length = 1048576;

    body = recv( socket:__ka_socket, length:length, min:length );
  } else {
    # If we don't have the length, we close the connection to make sure
    # the next request won't mix up the replies.

    if( _http_ka_debug ) display( "DEBUG: ERROR - Keep Alive, but no length!!!\n", __ka_last_request );
    body = recv( socket:__ka_socket, length:16384, min:0 );
    if( body =~ '<html[^>]*>' && body !~ '</html>' ) { # case insensitive
      repeat {
        tmp = recv( socket:__ka_socket, length:16384 );
        body += tmp;
      }
      until( ! tmp || body =~ "</html>" );
      if( _http_ka_debug && body !~ "</html>" ) display( "DEBUG: http_keepalive_recv_body: incomplete body?\n------------\n", body, "\n------------\n" );
    }
    http_close_socket( __ka_socket );
    __ka_socket = http_open_socket( __ka_port );
  }

  http_keepalive_check_connection( headers:headers );

  if( gzip )
    body = http_gunzip( buf:body, onlybody:TRUE );

  if( bodyonly )
    return( body );
  else
    return( strcat( headers, '\r\n', body ) );
}

# @brief Closes the opened keep-alive socket on exit.
#
# @return NULL
#
function on_exit() {
  if( __ka_socket ) {
    http_close_socket( __ka_socket );
    __ka_socket = 0;
  }
  return NULL;
}

if( 0 ) on_exit(); #TBD: What's the purpose of this?

# @brief This is our "public" Keep-Alive function. It sends <data> to the remote
#        host on port <port>, and returns the result, or NULL if no connection
#        could be established.
#
# @param port                   The port of the remote server to connect to.
# @param data                   The data to sent to the remote server.
# @param bodyonly               If set to TRUE only the HTTP body will be returned.
# @param fetch404               If set to TRUE the function will also return the body of HTTP 404 error pages.
# @param content_type_body_only Allows to fetch pages with e.g. only Content-Type: text/html by passing a regex like "^Content-Type: text/(xml|html)"
# @param headersonly            If set to TRUE only the HTTP header will be returned.
#
# @return A string containing the received data or NULL if no connection could be esablished or no port or data parameter was passed.
#
function http_keepalive_send_recv( port, data, bodyonly, fetch404, content_type_body_only, headersonly ) {

  local_var port, data, bodyonly, fetch404, content_type_body_only, headersonly;
  local_var oid, soc, headers, body, id, n, lendata, user_agent, oid_str;
  # nb: _http_ua_include_oid, _http_ka_debug, __ka_enabled and __ka_socket are global vars

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_keepalive_send_recv" );
    return NULL;
  }

  if( ! data ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#data#-#http_keepalive_send_recv" );
    return NULL;
  }

  # nb: We should expect at least a request like: "GET / HTTP/1.1\r\n\r\n" (length: 18).
  # But for some specific cases we're just checking for some less data.
  if( strlen( data ) < 8 || data !~ "^(DELETE|PROPFIND|PUT|GET|HEAD|POST|OPTIONS|REPORT|MKCOL|MOVE|PROPPATCH|COPY|PATCH|CONNECT|TRACE|LOCK|UNLOCK|TRACK|M-POST|CHECKOUT|CHECKIN|UNCHECKOUT|VERSION-CONTROL|BASELINE-CONTROL).*HTTP/(1\.[01]|2)" ) {
    oid = get_script_oid();
    if( oid != "1.3.6.1.4.1.25623.1.0.900522" && # secpod_ziproxy_server_detect.nasl might require the wrong request to trigger the detection
        oid != "1.3.6.1.4.1.25623.1.0.10730" )   # pre2008/raptor_detect.nasl as well
    set_kb_item( name:"nvt_debug_misc/" + oid, value:oid + "#-#http_keepalive_send_recv(): Invalid HTTP request (length < 8, invalid HTTP method or missing HTTP/ header) passed in 'data' variable." );
  }

  # http_get() won't add a Host: header if only HTTP/1.0 support was detected
  if( "Host: " >!< data && " HTTP/1.0" >!< data ) {
    oid = get_script_oid();
    if( oid != "1.3.6.1.4.1.25623.1.0.802045" ) # nb: gb_linux_rootkit_nginx_iframe_injection.nasl requires a wrong host header
      set_kb_item( name:"nvt_debug_misc/" + oid, value:oid + "#-#http_keepalive_send_recv(): No 'Host:' header passed in 'data' variable. This might lead to issues if SNI is enabled on the remote host." );
  }

  if( data !~ "^(DELETE|PROPFIND|PUT|GET|HEAD|POST|OPTIONS|REPORT|MKCOL|MOVE|PROPPATCH|COPY|PATCH|CONNECT|TRACE|LOCK|UNLOCK|TRACK|M-POST|CHECKOUT|CHECKIN|UNCHECKOUT|VERSION-CONTROL|BASELINE-CONTROL) (/|\.+/|https?:|\*).* HTTP" ) {
    oid = get_script_oid();
    if( oid != "1.3.6.1.4.1.25623.1.0.103293" && # gb_apache_49957.nasl requires sending a @localhost URI
        oid != "1.3.6.1.4.1.25623.1.0.17230"  && # cern_httpd_access_ctrl.nasl requires sending a \ at the start of the URI
        oid != "1.3.6.1.4.1.25623.1.0.900522" && # secpod_ziproxy_server_detect.nasl might require the wrong request to trigger the detection
        oid != "1.3.6.1.4.1.25623.1.0.10730" )   # pre2008/raptor_detect.nasl as well
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_keepalive_send_recv(): URL/URI of the HTTP request passed in 'data' variable doesn't start with one of the following: '/, ./, http, *'." );
  }

  # Some (mostly older) VTs are not passing the User-Agent header but we should always have one.
  if( " HTTP/1.1" >< data && ! egrep( pattern:"^User-Agent:.+", string:data, icase:TRUE ) ) {
    data = ereg_replace( string:data, pattern:'\r\n\r\n', replace:'\r\nUser-Agent: ' + http_get_user_agent() + '\r\n\r\n' );
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_keepalive_send_recv(): Using a HTTP/1.1 request without a 'User-Agent:' header passed in 'data' variable. Adding it automatically to the request." );
  }

  if( _http_ua_include_oid && user_agent = egrep( pattern:"^User-Agent:.+", string:data, icase:TRUE ) ) {
    user_agent = chomp( user_agent );
    oid_str = "(OID:" + get_script_oid() + ")";
    if( oid_str >!< user_agent )
      data = str_replace( string:data, find:user_agent, replace:user_agent + " " + oid_str );
  }

  if( _http_ka_debug ) display( "DEBUG: http_keepalive_send_recv( port: ", port, ", data: ", data, ", bodyonly: ", bodyonly, " )\n" );

  if( __ka_enabled == -1 )
    __ka_enabled = http_keepalive_enabled( port:port );

  if( __ka_enabled == -2 )
    return NULL;

  if( __ka_enabled == 0 ) {
    soc = http_open_socket( port );
    if( ! soc )
      return NULL;

    if( send( socket:soc, data:data ) <= 0 ) {
      http_close_socket( soc );
      return NULL;
    }

    headers = http_recv_headers2( socket:soc );
    # If the headers are not HTTP compliant, just return right away
    if( headers && ! ereg( pattern:"^HTTP/.* [0-9]+", string:headers ) ) {
      http_close_socket( soc );
      return headers;
    }

    if( content_type_body_only ) {
      if( "content-type: " >< tolower( headers ) ) {
        if( ! egrep( pattern:content_type_body_only, string:headers, icase:TRUE ) ) {
          http_close_socket( soc );
          return headers;
        }
      }
    }

    if( headers && ( ! ereg( pattern:"^HTTP/.* 404", string:headers ) || fetch404 == TRUE ) )
      body = http_recv_body( socket:soc, headers:headers, length:0 );

    http_close_socket( soc );

    if( "content-encoding: gzip" >< tolower( headers ) )
      body = http_gunzip( buf:body, onlybody:TRUE );

    if( headersonly )
      return( headers );

    if( bodyonly )
      return( body );
    else
      return( strcat( headers, '\r\n', body ) );
  }

  if( ( port != __ka_port ) || ( ! __ka_socket ) ) {
    if( __ka_socket ) http_close_socket( __ka_socket );
    __ka_port = port;
    __ka_socket = http_open_socket( port );
    if( ! __ka_socket )
      return NULL;
  }

  id = stridx( data, '\r\n\r\n' );
  # Old behavior: data = str_replace( string:data, find:"Connection: Close", replace:"Connection: Keep-Alive", count:1 );
  # https://tools.ietf.org/html/rfc7230#section-6.1 -> Connection options are case-insensitive.
  # Some VT devs might add a "Connection: close" to the header data passed to this function so doing an ereg_replace instead
  # nb: Be aware that this ereg_replace is also replacing any occurrence of "Connection: Close" somewhere in the body of the request data.
  # TODO: Add a Connection: Keep-Alive if no Connection: Close is included in the passed request?
  data = ereg_replace( string:data, pattern:"Connection: Close", replace:"Connection: Keep-Alive", icase:TRUE );
  __ka_last_request = data;
  n = send( socket:__ka_socket, data:data );
  lendata = strlen( data );
  if( n >= lendata )
    headers = http_recv_headers2( socket:__ka_socket );

  if( ! headers ) {
    http_close_socket( __ka_socket );
    __ka_socket = http_open_socket( __ka_port );
    if( __ka_socket == 0 ) return NULL;
    if( send( socket:__ka_socket, data:data ) < lendata ) {
      http_close_socket( __ka_socket );
      __ka_socket = NULL;
      return NULL;
    }
    headers = http_recv_headers2( socket:__ka_socket );
  }

  if( headersonly )
    return( headers );

  # Allows to fetch pages with e.g. only Content-Type: text/html by passing
  # a regex like "^Content-Type: text/(xml|html)"
  if( content_type_body_only ) {
    if( "content-type: " >< tolower( headers ) ) {
      if( ! egrep( pattern:content_type_body_only, string:headers, icase:TRUE ) ) return headers;
    }
  }
  return http_keepalive_recv_body( headers:headers, bodyonly:bodyonly );
}

# @brief Requests a specified URL and checks the response if it contains a pattern for windows related files
#        (autoexec.bat, win.ini and boot.ini). In contrast to @ref check_win_dir_trav this function is requesting
#        the data over an keep-alive enabled connection.
#
# @param port The port of the remote server to check.
# @param url  The URL to check.
#
# @return TRUE if the remote web server has responded with the expected pattern, FALSE otherwise and
#         NULL in the case where no connection to the target could be established or not all
#         required parameters where passed.
#
function check_win_dir_trav_ka( port, url ) {

  local_var port, url;
  local_var req, buf;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#check_win_dir_trav_ka" );
    return NULL;
  }

  if( ! url ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#url#-#check_win_dir_trav_ka" );
    return NULL;
  }

  req = http_get( item:url, port:port );
  buf = http_keepalive_send_recv( port:port, data:req );

  if( "; for 16-bit app support" >< buf || "[boot loader]" >< buf )
    return TRUE;
  else
    return FALSE;
}

# @brief Creates a HTTP GET request with enabled Keep-Alive Connection for a
#        specified item (e.g. a script), sends the request to the target and checks
#        if the requested item is available.
#
# @param port The HTTP port of the target.
# @param item The item to request.
# @param host The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#             parameter or get_host_name) of the remote server. If no host parameter
#             is passed http_host_name will be called automatically and a debug
#             message is logged into the KB (currently commented out).
#
# @return TRUE if the requested item exists on the remote web server, FALSE otherwise
#         and NULL in the cases where wrong data is passed to the function or where the
#         server is respondig with a 200 status code to not existing files.
#
function is_cgi_installed_ka( port, item, host ) {

  local_var port, item, host;
  local_var dirs, slash, no404, dir, req, res;
  # nb: optimize_test_enabled is a global var from above

  if( ! item ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#item#-#is_cgi_installed_ka" );
    return NULL;
  }

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#is_cgi_installed_ka" );
    return NULL;
  }

  if( http_is_cgi_scan_disabled() ) {
    if( optimize_test_enabled && "yes" >< optimize_test_enabled ) {
      set_kb_item( name:"nvt_cgi_scanning_disabled/" + get_script_oid(), value:get_script_oid() + "#-#is_cgi_installed_ka()" );
    }
    exit( 0 );
  }

  if( item[0] != "/" ) {
    dirs = cgi_dirs( port:port );
    slash = "/";
  } else {
    dirs = make_list("");
    slash = "";
  }

  if( ! host ) {
    host = http_host_name( dont_add_port:TRUE );
    #set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#is_cgi_installed_ka" );
  }

  no404 = http_get_no404_string( port:port, host:host );
  if( strlen( no404 ) >= 1 )
    return NULL;

  foreach dir( dirs ) {

    if( dir == "/" ) dir = "";

    # some server return a 404 for / but sometimes not for all subdirs. so check again...
    req = http_get( item:dir + slash + item + '_' + rand(), port:port );
    res = http_keepalive_send_recv( port:port, data:req );
    if( res !~ "^HTTP/1\.[01] 404" )
      continue;

    req = http_get( item:dir + slash + item, port:port );
    res = http_keepalive_send_recv( port:port, data:req );
    if( isnull( res ) )
      continue;

    if( res =~ "^HTTP/1\.[01] +200 +" && "Proxy-Agent: IWSS" >!< r ) {
      if( no404 && tolower( no404 ) >< tolower( res ) ) {
        continue;
      } else {
        return TRUE;
      }
    }
  }
  return FALSE;
}

# @brief Creates a HTTP GET request with enabled Keep-Alive Connection for a
#        specified item (e.g. a script), sends the request to the target and
#        returns the response. It will also follow up to 32 times a received redirect.
#
# @param port          The HTTP port of the target.
# @param url           The URL to request.
# @param max_redirects The count of redirects to follow, defaults to 32 if not passed.
#
# @return The response to the HTTP GET request or NULL if no response was received or the
#         max count of redirects was reached.
#
function get_http_page( port, url, max_redirects ) {

  local_var port, url, max_redirects;
  local_var count, seen_loc, i, req, res, redirect, location;
  # nb: optimize_test_enabled is a global var from above

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_http_page" );
    return NULL;
  }

  if( ! url ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#url#-#get_http_page" );
    return NULL;
  }

  if( http_is_cgi_scan_disabled() ) {
    if( optimize_test_enabled && "yes" >< optimize_test_enabled ) {
      set_kb_item( name:"nvt_cgi_scanning_disabled/" + get_script_oid(), value:get_script_oid() + "#-#get_http_page()" );
    }
    exit( 0 );
  }

  if( isnull( max_redirects ) ) {
    count = 32;
  } else if( max_redirects <= 0 ) {
    count = 1;
  } else {
    count = max_redirects + 1;
  }

  seen_loc = make_list();
  for( i = 0; i < count; i ++ ) { # Limited iterations to avoid traps
    seen_loc[url] = TRUE;
    req = http_get( port:port, item:url );
    res = http_keepalive_send_recv( port:port, data:req );
    if( ! res )
      return NULL;

    if( res =~ "^HTTP/1\.[01] +30[0-9] .*" ) {
      redirect = eregmatch( pattern:'\r\nLocation: *([^ \t\r\n]+)[ \t]*[\r\n]+', string:res, icase:TRUE );
      if( isnull( redirect ) )
        return NULL; # Big problem

      location = redirect[1];
      if( seen_loc[location] )
        return NULL;

      seen_loc[location] = TRUE;
    } else if( res =~ "^HTTP/1\.[01] +200 " ) {
      res = strstr( res, '\r\n\r\n' );
      res = substr( res, 4 );
      return res;
    } else { # Code 4xx or 5xx
      return NULL;
    }
  }
  # Loop?
  return NULL;
}

# @brief Creates a HTTP GET request for the specified URL, sends
#        the request to the target and receives the response.
#        If a response was received it is saved into the knowledge base.
#        If an URL was requested which was requested previously the cached
#        response will be returned instead of doing a new request.
#
# @param port The HTTP port of the target.
# @param item The URL/item to request.
# @param host The hostname (e.g. from http_host_name with the dont_add_port:TRUE
#             parameter or get_host_name) of the remote server. If no host parameter
#             is passed http_host_name will be called automatically and a debug
#             message is logged into the KB (currently commented out).
#
# @return The response to the HTTP GET request or NULL if no response was received.
#
# @note  The global_var __ka_last_request can be used to access the "req"
#        crafted by this function.
#
function http_get_cache( port, item, host ) {

  local_var port, item, host;
  local_var res, req;
  # nb: __ka_last_request is a global var
  # nb: optimize_test_enabled is a global var from above

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_get_cache" );
    return NULL;
  }

  if( ! item ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#item#-#http_get_cache" );
    return NULL;
  }

  if( http_is_cgi_scan_disabled() ) {
    if( optimize_test_enabled && "yes" >< optimize_test_enabled ) {
      set_kb_item( name:"nvt_cgi_scanning_disabled/" + get_script_oid(), value:get_script_oid() + "#-#http_get_cache()" );
    }
    exit( 0 );
  }

  if( ! host ) {
    host = http_host_name( dont_add_port:TRUE );
    #set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#host#-#http_get_cache" );
  }

  res = get_kb_item( "Cache/" + host + "/" + port + "/URL_" + item );
  if( res )
    return res;

  req = http_get( port:port, item:item );
  res = http_keepalive_send_recv( port:port, data:req, bodyonly:FALSE );
  if( isnull( res  ) || res == "" || ! res )
    return NULL;

  __ka_last_request = req;

  # Only save a valid HTTP response into the KB. We also
  # don't want so save the following HTTP status codes
  # into our cache which might be just temporary:
  # Internal Server Errors (5xx)
  # Too Many Requests (429)
  # Request Timeout (408)
  if( res !~ "^HTTP/1\.[01] 50[0-9]" &&
      res !~ "^HTTP/1\.[01] 51[01]" &&
      res !~ "^HTTP/1\.[01] 408" &&
      res !~ "^HTTP/1\.[01] 429" &&
      res =~ "^HTTP/1\.[01]" ) {
    replace_kb_item( name:"Cache/" + host + "/" + port + "/URL_" + item, value:res );
  }
  return res;
}

# @brief Creates a HTTP GET request with enabled Keep-Alive Connection for a
#        specified item (e.g. a script), sends the request to the target and checks
#        if one or more specific patterns are included in the response. If the pattern
#        was found a security_message() is reported and the script exit afterwards.
#
# @param default_port  The default HTTP port to assume if no open ports was found on the target host.
# @param extra_dirs    Additional directories to check which are added to the found directories of @ref cgi_dirs. If empty only @ref cgi_dirs is used.
# @param unique_dir    The directory which should be checked, if this parameter is passed extra_dirs will be ignored.
# @param check_request The request / item to sent in the HTTP GET request.
# @param extra_check   A pattern to check additionally to the check_result in form of a regex.
# @param check_result  The expected result to the response in form of a regex where a security_message() is reported.
# @param command       The command which was used in the check_request parameter.
# @param port          The HTTP port to check.
#
# @return NULL
#
function http_check_remote_code( default_port, extra_dirs, unique_dir, check_request, extra_check, check_result, command, port ) {

  local_var default_port, extra_dirs, unique_dir, check_request, extra_check, check_result, command, port;
  local_var list, dir, req, buf, txt_result, extra, txt_desc;
  # nb: optimize_test_enabled is a global var from above

  if( ! check_request ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#check_request#-#http_check_remote_code" );
    return NULL;
  }

  if( ! check_result ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#check_result#-#http_check_remote_code" );
    return NULL;
  }

  if( ! command )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#command#-#http_check_remote_code" );

  if( http_is_cgi_scan_disabled() ) {
    if( optimize_test_enabled && "yes" >< optimize_test_enabled ) {
      set_kb_item( name:"nvt_cgi_scanning_disabled/" + get_script_oid(), value:get_script_oid() + "#-#http_check_remote_code()" );
    }
    exit( 0 );
  }

  if( ! port ) {
    if( default_port ) {
      port = get_http_port( default:default_port );
    } else {
      port = get_http_port( default:80 );
    }
  } else {
    if( ! get_port_state( port ) )
      exit( 0 );
  }

  if( unique_dir ) {
    list = make_list( unique_dir );
  } else {
    if( ! isnull( extra_dirs ) ) {
      list = make_list_unique( cgi_dirs( port:port ), extra_dirs );
    } else {
      list = make_list( cgi_dirs( port:port ) );
    }
  }

  foreach dir( list ) {

    if( dir == "/" ) dir = "";

    req = string( dir, check_request );
    req = http_get( item:req, port:port );
    buf = http_keepalive_send_recv( port:port, data:req );
    if( ! buf )
      exit( 0 ); #TBD: Really exit?

    txt_result = egrep( pattern:check_result, string:buf );
    if( extra_check ) {
      extra = FALSE;
      if( egrep( pattern:extra_check, string:buf ) ) {
        extra = TRUE;
      }
    } else {
      extra = TRUE;
    }

    if( txt_result && extra ) {
      txt_desc = 'It was possible to execute the command "' + command + '" on the remote host, which produces the following output:\n\n' + txt_result;
      security_message( port:port, data:txt_desc );
      exit( 0 );
    }
  }
}

# @brief Checks a specific URL on the remote server if the response is matching / not matching the specified data.
#
# @param port          The HTTP port of the target.
# @param url           The URL/item to request.
# @param pattern       A regex pattern which needs to be included in the response of the target for the function to return TRUE.
# @param check_header  If set to TRUE the function is checking if it has received a HTTP 200 status code additionally to the regex pattern.
# @param debug         If set to TRUE the received data is displayed on the command line or in the scanners logfile.
# @param extra_check   A string or list containing extra pattern which needs to be included in the response. If multiple items are passed via
#                      a list all of these needs to be included in the response for the function to return TRUE.
# @param hostname      The hostname to connect to if called on command-line / via openvas-nasl
# @param cookie        The cookie to send within the request.
#
# @param check_nomatch A string or list containing extra pattern which shouldn't be included in the response. If multiple items are passed via
#                      a list all of these shouldn't be included in the response for the function to return TRUE.
#
# @param icase         If set to TRUE (default if not passed) the pattern, extra_check and check_nomatch are evaluated case insensitive, otherwise if FALSE is passed.
# @param usecache      If set to TRUE the response will be cached within the internal Redis KB.
#
# @return TRUE If the requested data is matching / not matching the specified pattern, FALSE if not or no data was received and NULL if no url, port or pattern parameter was passed.
#
# @note The global_var __ka_last_request can be used to access the "req" crafted by this VT.
#
function http_vuln_check( port, url, pattern, check_header, debug, extra_check, hostname, cookie, check_nomatch, icase, usecache ) {

  local_var port, url, pattern, check_header, debug, extra_check, hostname, cookie, check_nomatch, icase, usecache;
  local_var buf, req, ec, nm;
  # nb: __ka_last_request is a global var

  if( ! url ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#url#-#http_vuln_check" );
    return NULL;
  }

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_vuln_check" );
    return NULL;
  }

  if( isnull( pattern ) || pattern == "" ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#pattern#-#http_vuln_check" );
    return NULL;
  }

  if( isnull( icase ) )
    icase = TRUE;

  if( usecache ) {
    buf = http_get_cache( item:url, port:port );
    if( COMMAND_LINE && hostname )
      set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_vuln_check: Setting nocache to TRUE and passing hostname on command line isn't supported." );
    if( cookie && ! isnull( cookie ) )
      set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_vuln_check: Setting nocache to TRUE and passing a cookie isn't supported." );
  } else {

    req = http_get( item:url, port:port );

    if( COMMAND_LINE ) { # set hostname for vhosts on command line if hostname parameter was given.
      if( hostname ) {
        req = ereg_replace( string:req, pattern:"Host:.*" + get_host_ip(), replace:"Host: " + hostname );
      }
    }

    if( cookie && ! isnull( cookie ) )
      req = ereg_replace( string:req, pattern:'\r\n\r\n', replace:'\r\nCookie: ' + cookie + '\r\n\r\n' );

    buf = http_keepalive_send_recv( port:port, data:req );
    __ka_last_request = req;
  }

  if( isnull( buf  ) || buf == "" || ! buf )
    return FALSE;

  if( debug ) {
    display( "\nContent:\n", buf, "\n" );
  }

  if( check_header == TRUE ) {
    if( ! ereg( pattern:"^HTTP/[0-9]\.[0-9] 200.*", string:buf ) ) {
      return FALSE;
    }
  }

  ## Extra check for Strings to match in the buf
  if( extra_check ) {
    if( typeof( extra_check ) != "array" ) {
      extra_check = make_list( extra_check );
    }

    foreach ec( extra_check ) {
      if( ! egrep( pattern:ec, string:buf, icase:icase ) ) {
        return FALSE;
      }
    }
  }

  ## Check for Strings that should not match in buf
  if( check_nomatch ) {
    if( typeof( check_nomatch ) != "array" ) {
      check_nomatch = make_list( check_nomatch );
    }

    foreach nm( check_nomatch ) {
      if( egrep( pattern:nm, string:buf, icase:icase ) ) {
        return FALSE;
      }
    }
  }

  if( egrep( pattern:pattern, string:buf, icase:icase ) )
    return buf;
  else
    return FALSE;
}

# @brief Creates a HTTP GET request for the specified URL, sends
#        the request to the target and receives the response.
#
# @param port     The HTTP port of the target.
# @param url      The URL to request.
# @param bodyonly If TRUE only the body of the response is returned (not compatible with the nocache parameter set to FALSE).
# @param nocache  If TRUE the response is not saved into the knowledge base.
#
# @return The response to the HTTP GET request or NULL if no response was received.
#
function http_ka_recv_buf( port, url, bodyonly, nocache ) {

  local_var port, url, bodyonly, nocache;
  local_var res, req;

  if( ! port ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#http_ka_recv_buf" );
    return NULL;
  }

  if( ! url ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#url#-#http_ka_recv_buf" );
    return NULL;
  }

  if( ! nocache ) {
    if( ! egrep( pattern:"(\?|&|#|;|\[|\]|=)", string:url ) ) {
      if( bodyonly ) set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_ka_recv_buf: nocache set to FALSE but bodyonly set to TRUE, response will contain the body." );
      res = http_get_cache( item:url, port:port );
      return res;
    }
  }

  req = http_get( item:url, port:port );
  res = http_keepalive_send_recv( port:port, data:req, bodyonly:bodyonly );
  return res;
}