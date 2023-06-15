###############################################################################
# OpenVAS Vulnerability Test
#
# Collection of functions to handle URLs
#
# Authors:
# Vlatko Kosturjak <kost@linux.hr>
# URL encode & decode functions: Chandan S(schandan@secpod.com)
# clean_url, canonical_url, basename, check_recursion_candidates and
# remove_cgi_arguments moved from webmirror.nasl and C to the respective authors.
#
# Copyright:
# Copyright (C) 2008 Vlatko Kosturjak
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
###############################################################################

global_var HEX_LOWERCASE, HEX_UPPERCASE;
HEX_LOWERCASE = 1;
HEX_UPPERCASE = 2;

global_var recur_candidates;
recur_candidates = make_array();

# function taken from amap.nasl which is GPL
function url_hex2raw( s ) {

  local_var s, i, j, ret, l;

  if( ! s ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#s#-#url_hex2raw" );

  s = chomp( s ); # remove trailing blanks, CR, LF...
  l = strlen( s );
  if( l % 2 ) display( "hex2raw: odd string: ", s, "\n" );
  for( i = 0; i < l; i += 2 ) {
    if( ord( s[i] ) >= ord( "0" ) && ord( s[i] ) <= ord( "9" ) ) {
      j = int(s[i]);
    } else {
      j = int( ( ord( s[i] ) - ord( "a" ) ) + 10 );
    }

    j *= 16;
    if( ord( s[i+1] ) >= ord( "0" ) && ord( s[i+1] ) <= ord( "9" ) ) {
      j += int(s[i+1]);
    } else {
      j += int( ( ord( s[i+1] ) - ord( "a" ) ) + 10 );
    }
    ret += raw_string( j );
  }
  return ret;
}

###############################################################
# Function Name 'urlencode'
###############################################################

function urlencode( str, uppercase, unreserved ) {

  local_var i, j, str, uppercase, estr, char_set, num_set, specl_char_set, unreserv_chars, unreserved, flag;

  if( ! str ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#str#-#urlencode" );

  char_set = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ";
  num_set = "0123456789";
  specl_char_set = "_-.!~*'()";
  unreserv_chars = char_set + num_set + specl_char_set;

  if( unreserved != NULL ) {
    unreserv_chars = unreserv_chars + unreserved;
  }

  for( i = 0; i < strlen( str ); i++ ) {
    flag = "non_word";

    # Encode on any non word character
    for( j = 0; j < strlen( unreserv_chars ); j++ ) {
      if( str[i] == unreserv_chars[j] ) {
        flag = "word";
        break;
      }
    }

    if( flag == "non_word" ) {
      if( uppercase )
        estr = estr + '%' + toupper( hexstr( str[i] ) );
      else
        estr = estr + '%' + hexstr( str[i] );
    } else {
        estr = estr + str[i];
    }
  }
  return( estr );
}

###############################################################
# Function Name 'urldecode'
###############################################################

function urldecode( estr ) {

  local_var estr, dstr, i;

  if( ! estr ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#estr#-#urldecode" );

  for( i = 0; i < strlen( estr ); i++ ) {
    if( estr[i] == '%' ) {
      dstr = dstr + url_hex2raw( s:tolower( estr[i+1] + estr[i+2] ) );
      i = i + 2;
    } else if( estr[i] == '+' ) {
      dstr = dstr + ' ';
      i = i + 1;
    } else {
      dstr = dstr + estr[i];
    }
  }
  dstr = ereg_replace( string:dstr, pattern:"<!--(.|\n)*-->", replace:"", icase:TRUE );
  return( dstr );
}

# @brief Clears a passed URL from spaces as well as single and doubled quotation marks.
#
# @param url The URL to clear.
#
# @return The passed URL with the mentioned items removed or just the URL (if no changes where done)
#         and NULL if no URL parameter was passed.
#
function clean_url( url ) {

  local_var url;
  local_var _search;

  if( ! url )
    return url;

  foreach _search( make_list( "'"," ",'"' ) ) {
    if( ! isnull( url ) ) {
      url = str_replace( string:url, find:_search, replace:"", keep:FALSE );
    }
  }
  return url;
}

# @brief Converts a passed URL into a "canonical" form by e.g. removing any dynamic parameters.
#
# @param url              The URL to evaluate, e.g. /dir1/dir2.
# @param current          The current level / directory within the evaluated webpage, e.g. /dir1.
# @param port             The port of the remote host.
# @param host             The hostname of the remote host.
# @param debug            The verbose level for debugging output, needs to be a positive integer.
# @param webmirror_called Internal parameter only, will be set by webmirror.nasl and passed to
#                         @ref check_recursion_candidates.
#
# @return A list containing the converted URL in index 0, optional any removed arguments in index 1.
#         If no url parameter, a location redirect to another host or a "#" prefixed URL was passed
#         NULL will be returned.
#
function canonical_url( url, current, port, host, debug, webmirror_called ) {

  local_var url, current, port, host, debug, webmirror_called;
  local_var location, num_dots, i;

  url = clean_url( url:url );

  if( debug > 1 ) display( "***** canonical '", url, "' (current:", current, ")\n" );

  if( strlen( url ) == 0 )
    return NULL;

  if( url[0] == "#" )
    return NULL;

  if( url == "./" || url == "." || url =~ "^\./\?" )
    return make_list( current );

  # We need to check for a possible recursion, see the function for some background notes.
  if( check_recursion_candidates( url:url, current:current, port:port, host:host, debug:debug, webmirror_called:webmirror_called ) )
    return NULL;

  if( debug > 2 ) display( "**** canonical(again) ", url, "\n" );

  if( ereg( pattern:"[a-z]*:", string:url, icase:TRUE ) ) {
    if( ereg( pattern:"^http://", string:url, icase:TRUE ) ) {
      location = ereg_replace( string:url, pattern:"http://([^/]*)/.*", replace:"\1", icase:TRUE );
      if( location != url ) {
        # TBD: location could also contain the port, e.g. Location: http://example.com:1234/url
        if( location != get_host_name() ) {
          return NULL;
        } else {
          return remove_cgi_arguments( url:ereg_replace( string:url, pattern:"http://[^/]*/([^?]*)", replace:"/\1", icase:TRUE ) );
        }
      }
    } else if( ereg( pattern:"^https://", string:url, icase:TRUE ) ) {
      location = ereg_replace( string:url, pattern:"https://([^/]*)/.*", replace:"\1", icase:TRUE );
      if( location != url ) {
        # TBD: location could also contain the port, e.g. Location: https://example.com:1234/url
        if( location != get_host_name() ) {
          return NULL;
        } else {
          return remove_cgi_arguments( url:ereg_replace( string:url, pattern:"https://[^/]*/([^?]*)", replace:"/\1", icase:TRUE ) );
        }
      }
    }
  } else {
    if( url == "//" )
      return make_list( "/" );

    if( ereg( pattern:"^//.*", string:url, icase:TRUE ) ) {
      location = ereg_replace( string:url, pattern:"//([^/]*)/.*", replace:"\1", icase:TRUE );
      if( location != url ) {
        # TBD: location could also contain the port, e.g. Location: //example.com:1234/url
        if( location == get_host_name() ) {
          return remove_cgi_arguments( url:ereg_replace( string:url, pattern:"//[^/]*/([^?]*)", replace:"/\1", icase:TRUE ) );
        }
     }
     return NULL;
    }

    if( url[0] == "/" ) {
      return remove_cgi_arguments( url:url );
    } else {
      i = 0;
      num_dots = 0;

      while( i < strlen( url ) - 2 && url[i] == "." && url[i+1] == "." && url[i+2] == "/" ) {
        num_dots++;
        url = url - "../";
        if( strlen( url ) == 0 )
          break;
      }

      while( i < strlen( url ) && url[i] == "." && url[i+1] == "/" ) {
        url = url - "./";
        if( strlen( url ) == 0 )
          break;
      }

      # Repeat again as some websites are doing stuff like <a href="./../foo"></a>
      while( i < strlen( url ) - 2 && url[i] == "." && url[i+1] == "." && url[i+2] == "/" ) {
        num_dots++;
        url = url - "../";
        if( strlen( url ) == 0 )
          break;
      }

      url = string( basename( name:current, level:num_dots ), url );
    }

    i = stridx( url, "#" );
    if( i >= 0 )
      url = substr( url, 0, i - 1 );

    if( url[0] != "/" ) {
      return remove_cgi_arguments( url:string("/", url ) );
    } else {
      return remove_cgi_arguments( url:url );
    }
  }
  return NULL;
}

# @brief Returns the "base" of a passed URL which basically means
#        for a "name" of "/foo/bar" and a level of "0" the URL "/foo"
#        will be returned.
#
# @param name  The basename to evaluate.
# @param level The current level.
#
# @return The basename for the passed name and level.
#
function basename( name, level ) {

  local_var name, level;
  local_var len, i;

  len = strlen( name );

  if( len == 0 )
    return NULL;

  for( i = len - 1; i >= 0; i-- ) {
    if( name[i] == "/" ) {
      level--;
      if( level < 0 ) {
        return( substr( name, 0, i ) );
      }
    }
  }

  # Level is too high, we return /
  return "/";
}

# @brief This function checks if the passed URL is a recursion candidate (see examples below).
#
# @param url              The URL to evaluate, e.g. /dir1/dir2.
# @param current          The current level / directory within the evaluated webpage, e.g. /dir1.
# @param port             The port of the remote host.
# @param host             The hostname of the remote host.
# @param debug            The verbose level for debugging output, needs to be a positive integer.          
# @param webmirror_called Internal parameter only, will be set by webmirror.nasl to
#                         save the recursion candidates to the KB only once.
#
# @return TRUE if the same URL was previously collected two times
#         and FALSE otherwise.
#
function check_recursion_candidates( url, current, port, host, debug, webmirror_called ) {

  local_var url, current, port, host, debug, webmirror_called;
  local_var num;
  # nb: recur_candidates is a global_var

  if( ! url )
    return FALSE;

  # A few of those are already checked in canonical_url
  # but just checking again to be sure...
  # Examples without recursions:
  # <a href="#">example1</a>
  # <a href="./example2">example2</a>
  # <a href="/example3">example3</a>
  # <a href="../example4">example4</a>
  # <a href="./../example5">example5</a>
  # <a href="https://example6">example6</a>
  # <a href="//example7">example7</a>
  if( url =~ "^(https?|\.|/|#)" ) {
    if( debug > 3 ) display( "***** Not a recursion candidate: '", url, "'\n" );
    return FALSE;
  }

  # Recursion candidates are only links to subdirs
  if( "/" >!< url )
    return FALSE;

  # e.g. if a 404 page contains a link like:
  # <link rel="icon" href="assets/img/favicon.ico" type="image/x-icon">
  # which would be a relative URL to a subfolder of the current path
  # throwing the same 404 page causing a recursion later...
  num = recur_candidates[url];
  if( num ) {
    num++;
    if( debug > 3 ) display( "***** Adding possible recursion candidate: '", url, "' (Count: ", num, ")\n" );
    recur_candidates[url] = num;
    if( num > 2 ) {
      if( debug > 3 ) display( "***** Max count ", num, " of recursion for: '", url, "' reached, skipping this URL.\n" );

      if( webmirror_called )
        set_kb_item( name:"www/" + host + "/" + port + "/content/recursion_urls", value:current + " (" + url + ")" );
      return TRUE;
    }
  } else {
    if( debug > 3 ) display( "***** Adding possible recursion candidate: '", url, "' (Count: 1)\n" );
    recur_candidates[url] = 1;
  }
  return FALSE;
}

# @brief Removes CGI arguments (e.g. like ?test=1 from /url/index.php?test=1) from a passed URL.
#
# @param url The URL to evaluate.
#
# @return A list containing the converted URL in index 0, optional any removed arguments in index 1.
#         If no url parameter was passed NULL will be returned.
#
function remove_cgi_arguments( url ) {

  local_var url;
  local_var len, idx, cgi, cgi_args, arg, args, a, b;

  if( isnull( url ) )
    return NULL;
  else if( ! url )
    return make_list( url );

  # Remove the trailing blanks
  while( url[ strlen( url ) - 1] == " " ) {
    url = substr( url, 0, strlen( url ) - 2);
  }

  if( isnull( url ) )
    return NULL;
  else if( ! url )
    return make_list( url );

  # New length after removing the trailing blanks
  len = strlen( url );

  idx = stridx( url, "?" );
  if( idx < 0 ) {
    return make_list( url );
  } else if( idx >= len - 1 ) {
    cgi = substr( url, 0, len - 2 );
    return make_list( cgi, "" );
  } else {
    if( idx > 1 ) {
      cgi = substr( url, 0, idx - 1 );
    } else {
      cgi = ".";
    }

    cgi_args = split( substr( url, idx + 1, len - 1 ), sep:"&" );

    foreach arg( make_list( cgi_args ) ) {

      arg = arg - "&";
      arg = arg - "amp;";
      a = ereg_replace( string:arg, pattern:"(.*)=.*", replace:"\1" );
      b = ereg_replace( string:arg, pattern:".*=(.*)", replace:"\1" );

      if( a != b ) {
        args = string( args, a, " [", b, "] " );
      } else {
        args = string( args, arg, " [] " );
      }
    }
    return make_list( cgi, args );
  }
}