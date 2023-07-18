###############################################################################
# OpenVAS Include File
# $Id: lexmark_printers.inc 7288 2017-09-27 07:21:24Z cfischer $
#
# Authors:
# Michael Meyer <michael.meyer@greenbone.net>
#
# Copyright:
# Copyright (C) 2013 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or
# modify it under the terms of the GNU General Public License
# as published by the Free Software Foundation; either version 2
# of the License, or (at your option) any later version
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
# $Revision: 7288 $
#
###############################################################################

function get_lexmark_detect_urls() {

  local_var lexmark_detect_urls;

  lexmark_detect_urls = make_array();
  lexmark_detect_urls['<title>Lexmark ([^<]+)</title>'] = '/';
  lexmark_detect_urls['<title>Lexmark&#032;([^<]+)</title>'] = '/';

  return lexmark_detect_urls;
}

function check_lexmark_default_login( model, port ) {

  local_var model, port, lex, host, req, buf;

  lex = lexmark_login_details( model:model );
  if( ! lex ) return FALSE;

  if( lex['no_default_auth'] ) {
    if( lex['req_type'] == "GET" ) {
      if( lex['req_auth'] == "BASIC" ) {

        host = http_host_name( port:port );
        req = string( "GET ", lex['login_url']," HTTP/1.1\r\n",
                      "Host: ", host, "\r\n" );
        buf = http_send_recv( port:port, data:req + '\r\n', bodyonly:FALSE );
        lexmark_error_exit( buf:buf );

        if( ereg( pattern:lex['http_status'], string:buf ) ) {
          if( eregmatch( pattern:lex['login_success'], string:buf ) ) {
            return 2;
          }
        }
      }
    }
  }
}

function lexmark_login_details( model ) {

  local_var model, lexmark_printer;

  lexmark_printer = make_array();

  if(model == "T644"   ||
     model == "E450dn" ||
     model == "T642"   ||
     model == "119") {

    lexmark_printer['req_type']  = "GET";
    lexmark_printer['login_url'] = '/cgi-bin/dynamic/config/secure/changepass.html';
    lexmark_printer['req_auth']  = "BASIC";
    lexmark_printer['no_default_auth'] = "TRUE";

    lexmark_printer['http_status'] = 'HTTP/1.. 200';
    lexmark_printer['login_success'] = 'NAME="vac.255.GENPASSWORD"';

    return lexmark_printer;
  }

  else if(model == "T630" ||
          model == "C510" ||
          model == "T620" ||
          model == "T522" ||
          model == "T520") {

    lexmark_printer['req_type']  = "GET";
    lexmark_printer['login_url'] = '/port_0/config/security';
    lexmark_printer['req_auth']  = "BASIC";
    lexmark_printer['no_default_auth'] = "TRUE";

    lexmark_printer['http_status'] = 'HTTP/1.. 200';
    lexmark_printer['login_success'] = 'NAME="GENPASSWORD"';

    return lexmark_printer;
  }

  else if(model == "X954"    ||
          model == "X952"    ||
          model == "X656de"  ||
          model == "X792"    ||
          model == "T650"    ||
          model == "X464de"  ||
          model == "C925"    ||
          model == "X463de"  ||
          model == "C734"    ||
          model == "E460dn"  ||
          model == "E462dtn" ||
          model == "T652"    ||
          model == "C792"    ||
          model == "MX410de" ||
          model == "T654"    ||
          model == "C925"    ||
          model == "X746"    ||
          model == "C736"    ||
          model == "X738de"  ||
          model == "X864de")  {

    lexmark_printer['req_type']  = "GET";
    lexmark_printer['login_url'] = '/cgi-bin/dynamic/printer/config/secure/authsetup.html';
    lexmark_printer['req_auth']  = "BASIC";
    lexmark_printer['no_default_auth'] = "TRUE";

    lexmark_printer['http_status'] = 'HTTP/1.. 200';
    lexmark_printer['login_success'] = 'select name="auth_type" id="auth_type"';

    return lexmark_printer;
  }

  else if(model == "E260dn"         ||
          model == "X342n"          ||
          model == "X203n"          ||
          model == "X204n"          ||
          model == "C543"           ||
          model == "X364dn"         ||
          model == "E250dn"         ||
          model == "X264dn"         ||
          model == "S510 Series"    ||
          model == "C544"           ||
          model == "Pro5500 Series" ||
          model == "E360dn"         ||
          model == "X543"           ||
          model == "S310 Series"    ||
          model == "X544"           ||
          model == "C540") {

    lexmark_printer['req_type']  = "GET";
    lexmark_printer['login_url'] = '/cgi-bin/dynamic/config/gen/changepass.html';
    lexmark_printer['req_auth']  = "BASIC";
    lexmark_printer['no_default_auth'] = "TRUE";

    lexmark_printer['http_status'] = 'HTTP/1.. 200';
    lexmark_printer['login_success'] = 'NAME="vac.255.GENPASSWORD"';

    return lexmark_printer;
  }

  else if(model == "C935"  ||
          model == "X642e" ||
          model == "C782"  ||
          model == "X945e") {

    lexmark_printer['req_type']  = "GET";
    lexmark_printer['login_url'] = '/cgi-bin/dynamicpf/pf/config/gen/changepass.html';
    lexmark_printer['req_auth']  = "BASIC";
    lexmark_printer['no_default_auth'] = "TRUE";

    lexmark_printer['http_status'] = 'HTTP/1.. 200';
    lexmark_printer['login_success'] = 'NAME="vac.255.GENPASSWORD"';

    return lexmark_printer;
  }

  else if(model == "T656") {

    lexmark_printer['req_type']  = "GET";
    lexmark_printer['login_url'] = '/cgi-bin/dynamic/printer/config/secure/auth/backuppwd.html';
    lexmark_printer['req_auth']  = "BASIC";
    lexmark_printer['no_default_auth'] = "TRUE";

    lexmark_printer['http_status'] = 'HTTP/1.. 200';
    lexmark_printer['login_success'] = 'id="useFallbackPassword"';

    return lexmark_printer;
  }
}

function lexmark_error_exit( buf ) {

  local_var buf;

  if( ! buf || ereg( pattern:"HTTP/1.(0|1) (404|401|500)", string:buf ) ) exit( 0 );
  return TRUE;
}
