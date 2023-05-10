###############################################################################
# OpenVAS Include File
# $Id: hp_printers.inc 11035 2018-08-17 10:40:39Z ckuersteiner $
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
# $Revision: 11035 $
#
###############################################################################

function get_hp_detect_urls() {

  local_var hp_detect_urls;
  hp_detect_urls = make_array();
  hp_detect_urls['/info_deviceStatus.html']                             = '<title>HP (LaserJet [^ ]+) Series'; # HP LaserJet P2015 a.o.
  hp_detect_urls['/hp/device/']                                         =  '<title> HP Color i(LaserJet [^\n ]+)'; # HP Color LaserJet 2840 a.o.
  hp_detect_urls['/info_deviceStatus.html?tab=Status']                  = '<title>HP Color (LaserJet [^& ]+)'; # HP Color LaserJet CP2025n a.o.
  hp_detect_urls['/info_deviceStatus.html?']                            = '<title>HP (LaserJet [^&]+)'; # HP LaserJet 400 M401n a.o.
  hp_detect_urls['/index_top.htm']                                      =  string("(HP (LaserJet [^\n ]+)|HP Color (LaserJet [^\n ]+)|(DesignJet [^ ]+)|(psc [^ ]+) series)"); # LaserJet 3600 a.o.
  hp_detect_urls['/index.htm?cat=info&page=printerInfo']                =  string("((Officejet Pro [^& ]+)|(Officejet [^\n ]+))"); # K5400 a.o.
  hp_detect_urls['/hp/device/InternalPages/Index?id=ConfigurationPage'] = '<strong id="ProductName">HP ?([^&<]+)'; # HP Color LaserJet Flow MFP 680 a.o.
  hp_detect_urls['/']                                                   = '<strong class="product">HP ?([^&<]+)'; # HP Color LaserJet M651 a.o.
  hp_detect_urls['/hp/jetdirect/index.html']                            = '<h1>HP (Latex [0-9]+)'; # HP Latex 315 a.o.
  # HP PageWide Pro 477 a.o.
  hp_detect_urls['/DevMgmt/ProductConfigDyn.xml']                       = '<dd:MakeAndModel>HP (PageWide Pro [^<]+)';
  # HP PageWide 352dw a.o
  hp_detect_urls['/DevMgmt/ProductConfigDyn.xml']                       = '<dd:MakeAndModel>HP (PageWide [^<]+)';
  # HP Deskjet 2540 a.o.
  hp_detect_urls['/DevMgmt/ProductConfigDyn.xml']                       = '<dd:MakeAndModel>(Deskjet [^<]+)';
  # HP ENVY 120 series a.o.
  hp_detect_urls['/DevMgmt/ProductConfigDyn.xml']                       = '<dd:MakeAndModel>(Envy [^<]+)';
  # HP Ink Tank Wireless 410 series a.o.
  hp_detect_urls['/DevMgmt/ProductConfigDyn.xml']                       = '<dd:MakeAndModel>(Ink Tank [^<]+)';
  # HP Photosmart 5510 series a.o.
  hp_detect_urls['/DevMgmt/ProductConfigDyn.xml']                       = '<dd:MakeAndModel>(Photosmart [^<]+)';
  # HP Smart Tank Wireless 450 series a.o.
  hp_detect_urls['/DevMgmt/ProductConfigDyn.xml']                       = '<dd:MakeAndModel>HP (Smart Tank [^<]+)';

  # Detection string from dont_print_on_printers.nasl. TBD: Include here?
  # if( "<title>Hewlett Packard</title>" >< banner || egrep( pattern:"<title>.*LaserJet.*</title>", string:banner, icase:TRUE ) || "HP Officejet" >< banner || "server: hp-chai" >< tolower( banner ) || ( "Server: Virata-EmWeb/" >< banner && ( "HP" >< banner || "printer" >< banner ) ) ) {

  return hp_detect_urls;
}

function check_hp_default_login( model, port ) {

  local_var model, port, HP, host, req, buf;

  HP = hp_login_details( model:model );
  if( ! HP ) return FALSE;

  if( HP['no_default_auth'] ) {
    if( HP['req_type'] == "GET" ) {
      if( HP['req_auth'] == "BASIC" ) {

        host = http_host_name( port:port );
        req = string( "GET ", HP['login_url']," HTTP/1.1\r\n",
                      "Host: ", host, "\r\n" );
        buf = http_send_recv( port:port, data:req + '\r\n', bodyonly:FALSE );
        hp_error_exit( buf:buf );

        if( HP['http_status'] >< buf ) {
          if( eregmatch( pattern:HP['login_success'], string:buf ) ) {
            return 2;
          }
        }
      }
    }
  }
}

function hp_login_details( model ) {

  local_var model, hp_printer;

  hp_printer = make_array();

  if(model == "LaserJet 100 colorMFP M175nw" ||
     model == "LaserJet P2015"               ||
     model == "LaserJet CP2025n"             ||
     model == "LaserJet CP1515n"             ||
     model == "LaserJet M1522nf MFP"         ||
     model == "LaserJet M1536dnf MFP"        ||
     model == "LaserJet M2727nf MFP"         ||
     model == "LaserJet 2605dtn"             ||
     model == "LaserJet 2605dn"              ||
     model == "LaserJet 3055"                ||
     model == "LaserJet CM1312nfi"           ||
     model == "LaserJet CM2320fxi"           ||
     model == "LaserJet CM1415fn"            ||
     model == "LaserJet CM2320nf"            ||
     model == "LaserJet 400 color M451dn"    ||
     model == "LaserJet 400 M401n"           ||
     model == "LaserJet 400 M401dn") {

    hp_printer['req_type'] = "GET";
    hp_printer['login_url'] = '/set_config_networkIP.html';
    hp_printer['req_auth'] = "BASIC";
    hp_printer['no_default_auth'] = "TRUE";

    hp_printer['http_status'] = 'HTTP/1.1 200';
    hp_printer['login_success'] = 'label for="IPAddr1">';

    return hp_printer;
  }

  else if(model == "LaserJet 2840") {

    hp_printer['req_type'] = "GET";
    hp_printer['login_url'] = '/hp/device/set_config_networkIP.html';
    hp_printer['req_auth'] = "BASIC";
    hp_printer['no_default_auth'] = "TRUE";

    hp_printer['http_status'] = 'HTTP/1.1 200';
    hp_printer['login_success'] = 'This table contains the Network IP heading';

    return hp_printer;
  }

  else if(model == "LaserJet 5100"   ||
          model == "LaserJet 5000"   ||
          model == "LaserJet 4000"   ||
          model == "LaserJet 4050"   ||
          model == "LaserJet 3800"   ||
          model == "LaserJet 3700"   ||
          model == "LaserJet 3600"   ||
          model == "LaserJet 3500"   ||
          model == "LaserJet 2200"   ||
          model == "LaserJet 3550"   ||
          model == "LaserJet 3030"   ||
          model == "LaserJet 2300"   ||
          model == "LaserJet 8150"   ||
          model == "LaserJet 5800"   ||
          model == "Designjet 800PS" ||
          model == "DesignJet 800"   ||
          model == "LaserJet 2550"   ||
          model == "LaserJet 1320"   ||
          model == "Designjet 500"   ||
          model == "DesignJet 1055CM") {

    hp_printer['req_type'] = "GET";
    hp_printer['login_url'] = '/tcp_param.htm';
    hp_printer['req_auth'] = "BASIC";
    hp_printer['no_default_auth'] = "TRUE";

    hp_printer['http_status'] = 'HTTP/1.1 200';
    hp_printer['login_success'] = 'This table displays the TCP/IP configuration items';

    return hp_printer;
  }

  else if(model == "LaserJet 4250"    ||
          model == "LaserJet 4600"    ||
          model == "LaserJet 4650"    ||
          model == "designjet 5500ps" ||
          model == "LaserJet 4200"    ||
          model == "LaserJet 4730mfp" ||
          model == "LaserJet 4730"    ||
          model == "LaserJet CM4730"  ||
          model == "LaserJet P2055dn" ||
          model == "Color LaserJet M651" ||
          model == "LaserJet 2430") {

    hp_printer['req_type'] = "GET";
    hp_printer['login_url'] = '/hp/jetdirect/tcp_param.htm';
    hp_printer['req_auth'] = "BASIC";
    hp_printer['no_default_auth'] = "TRUE";

    hp_printer['http_status'] = 'HTTP/1.1 200';
    hp_printer['login_success'] = 'This table displays the TCP/IP configuration items';

    return hp_printer;
  }

  else if(model == "psc 2500") {

    hp_printer['req_type'] = "GET";
    hp_printer['login_url'] = '/uvl_gen_settings.htm';
    hp_printer['req_auth'] = "BASIC";
    hp_printer['no_default_auth'] = "TRUE";

    hp_printer['http_status'] = 'HTTP/1.1 200';
    hp_printer['login_success'] = 'This table is used to display the network protocol links/tabs';

    return hp_printer;
  }

  else if(model == "Officejet Pro K5400"  ||
          model == "Officejet Pro K5400"  ||
          model == "Officejet Pro L7600"  ||
          model == "Officejet Pro 8500"   ||
          model == "Officejet 7000 E809a") {

    hp_printer['req_type'] = "GET";
    hp_printer['login_url'] = '/index.htm?cat=network&page=wiredIfc&adaptorID=0';
    hp_printer['req_auth'] = "BASIC";
    hp_printer['no_default_auth'] = "TRUE";

    hp_printer['http_status'] = 'HTTP/1.1 200';
    hp_printer['login_success'] = 'id="alt_dnsserver1"';

    return hp_printer;
  }

  else if(model == "Officejet 4500") {

    hp_printer['req_type'] = "GET";
    hp_printer['login_url'] = '/index.htm?cat=network&page=wirelessIfc';
    hp_printer['req_auth'] = "BASIC";
    hp_printer['no_default_auth'] = "TRUE";

    hp_printer['http_status'] = 'HTTP/1.1 200';
    hp_printer['login_success'] = 'id="alt_dnsserver1"';

    return hp_printer;
  }

  else if(model == "Officejet Pro 8600") {

    hp_printer['req_type'] = "GET";
    hp_printer['login_url'] = '/webApps/Wired/ipV4.htf';
    hp_printer['req_auth'] = "BASIC";
    hp_printer['no_default_auth'] = "TRUE";

    hp_printer['http_status'] = 'HTTP/1.1 200';
    hp_printer['login_success'] = '<?hp te.includeSubPage';

    return hp_printer;
  }
}

function hp_error_exit( buf ) {

  local_var buf;

  if( ! buf || ereg( pattern:"HTTP/1.(0|1) (404|401|500)", string:buf ) ) exit( 0 );
  return TRUE;
}
