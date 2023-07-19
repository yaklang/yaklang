###############################################################################
# Functions for Cisco IOS
#
# Authors:
# Michael Meyer
#
# Copyright:
# Copyright (c) 2016 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software
# Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301
# USA.
###############################################################################

function iosver_2_iosxe_ver( iosver )
{
  # http://www.cisco.com/c/en/us/td/docs/routers/asr1000/release/notes/asr1k_rn_rel_notes/asr1k_rn_intro.html
  # http://blog.router-switch.com/2014/08/cisco-ios-xe-version-and-ios-version/
  # http://www.cisco.com/c/en/us/support/docs/ios-nx-os-software/ios-xe-1611/200419-IOS-XE-to-IOS-mapping-and-Feature-set-no.html#anc4
  # http://www.cisco.com/c/en/us/products/collateral/ios-nx-os-software/ios-xe-3sg/QA_C67-622903.html
  # [...]

  if( iosver == "12.2(33)XNF2" )    xever = "2.6.2";
  if( iosver == "12.2(33)XNF1" )    xever = "2.6.1";
  if( iosver == "12.2(33)XNF" )     xever = "2.6.0";
  if( iosver == "12.2(33)XNE2" )    xever = "2.5.2";
  if( iosver == "12.2(33)XNE1" )    xever = "2.5.1";
  if( iosver == "12.2(33)XNE" )     xever = "2.5.0";
  if( iosver == "12.2(33)XND4" )    xever = "2.4.4";
  if( iosver == "12.2(33)XND3" )    xever = "2.4.3";
  if( iosver == "12.2(33)XND2t" )   xever = "2.4.2t";
  if( iosver == "12.2(33)XND2" )    xever = "2.4.2";
  if( iosver == "12.2(33)XND1" )    xever = "2.4.1";
  if( iosver == "12.2(33)XND" )     xever = "2.4.0";
  if( iosver == "12.2(33)XNC2" )    xever = "2.3.2";
  if( iosver == "12.2(33)XNC1t" )   xever = "2.3.1t";
  if( iosver == "12.2(33)XNCt" )    xever = "2.3.0t";
  if( iosver == "12.2(33)XNC" )     xever = "2.3.0";
  if( iosver == "12.2(33)XNB3" )    xever = "2.2.3";
  if( iosver == "12.2(33)XNB2" )    xever = "2.2.2";
  if( iosver == "12.2(33)XNB1" )    xever = "2.2.1";
  if( iosver == "12.2(33)XNB" )     xever = "2.2.o";
  if( iosver == "12.2(33)XNA2" )    xever = "2.1.2";
  if( iosver == "12.2(33)XNA1" )    xever = "2.1.1";
  if( iosver == "12.2(33)XNA" )     xever = "2.1.0";
  if( iosver == "12.2(33)XN" )      xever = "2.1.0";
  if( iosver == "15.0(1)S" )        xever = "3.1.0S";
  if( iosver == "15.0(1)XO" )       xever = "3.1.0SG";
  if( iosver == "15.0(2)SG" )       xever = "3.2.0SG";
  if( iosver == "15.0(2)SG1" )      xever = "3.2.1SG";
  if( iosver == "15.0(2)SG2" )      xever = "3.2.2SG";
  if( iosver == "15.0(2)SG3" )      xever = "3.2.3SG";
  if( iosver == "15.0(2)SG4" )      xever = "3.2.4SG";
  if( iosver == "15.0(2)SG5" )      xever = "3.2.5SG";
  if( iosver == "15.0(2)SG6" )      xever = "3.2.6SG";
  if( iosver == "15.0(2)SG7" )      xever = "3.2.7SG";
  if( iosver == "15.0(2)SG8" )      xever = "3.2.8SG";
  if( iosver == "15.0(2)SG9" )      xever = "3.2.9SG";
  if( iosver == "15.0(2)SG10" )     xever = "3.2.10SG";
  if( iosver == "15.0(1)XO1" )      xever = "3.1.1SG";
  if( iosver == "15.1(0)EZ5" )      xever = "3.3.5SE";
  if( iosver == "15.2(3)E3" )       xever = "3.7.3E";
  if( iosver == "15.2(3)E2" )       xever = "3.7.2E";
  if( iosver == "15.2(3)E1" )       xever = "3.7.1E";
  if( iosver == "15.2(3)E" )        xever = "3.7.0E";
  if( iosver == "15.2(2)E4" )       xever = "3.6.4E";
  if( iosver == "15.2(2)E3" )       xever = "3.6.3E";
  if( iosver == "15.2(2)E2" )       xever = "3.6.2aE";
  if( iosver == "15.2(2)E1" )       xever = "3.6.1E";
  if( iosver == "15.2(2)E" )        xever = "3.6.0E";
  if( iosver == "15.0(1)EZ5" )      xever = "3.3.5SE";
  if( iosver == "15.0(1)EZ4" )      xever = "3.3.4SE";
  if( iosver == "15.0(1)EZ3" )      xever = "3.3.3SE";
  if( iosver == "15.0(1)EZ2" )      xever = "3.3.2SE";
  if( iosver == "15.0(1)EZ1" )      xever = "3.3.1SE";
  if( iosver == "15.0(2)SQB" )      xever = "3.3.0SQ";
  if( iosver == "15.0(2)SQB1" )     xever = "3.3.1SQ";
  if( iosver == "15.0(2)SQC" )      xever = "3.4.0SQ";
  if( iosver == "15.0(2)SQC1" )     xever = "3.4.1SQ";
  if( iosver == "15.0(2)SQD" )      xever = "3.5.0SQ";
  if( iosver == "15.0(2)SQD1" )     xever = "3.5.1SQ";
  if( iosver == "15.0(2)SQD2" )     xever = "3.5.2SQ";
  if( iosver == "15.1(2)S" )        xever = "3.3.0S";
  if( iosver == "15.0(1)EZ" )       xever = "3.3.0SE";
  if( iosver == "15.1(1)SG" )       xever = "3.3.0SG";
  if( iosver == "15.1(1)SG1" )      xever = "3.3.1SG";
  if( iosver == "15.1(1)XO" )       xever = "3.3.0XO";
  if( iosver == "15.1(1)XO1" )      xever = "3.3.1XO";
  if( iosver == "15.1(2)SG" )       xever = "3.4.0SG";
  if( iosver == "15.2(1)E" )        xever = "3.5.0E";
  if( iosver == "15.0(1)EX3" )      xever = "3.2.3SE";
  if( iosver == "15.0(1)EX2" )      xever = "3.2.2SE";
  if( iosver == "15.0(1)EX1" )      xever = "3.2.1SE";
  if( iosver == "15.0(1)EX" )       xever = "3.2.0SE";
  if( iosver == "15.0(1)S1" )       xever = "3.1.1S";
  if( iosver == "15.0(1)S2" )       xever = "3.1.2S";
  if( iosver == "15.0(1)S3" )       xever = "3.1.3S";
  if( iosver == "15.0(1)S4" )       xever = "3.1.4S";
  if( iosver == "15.0(1)S4a" )      xever = "3.1.4aS";
  if( iosver == "15.1(1)S" )        xever = "3.2.0S";
  if( iosver == "15.1(1)S1" )       xever = "3.2.1S";
  if( iosver == "15.1(1)S2" )       xever = "3.2.2S";
  if( iosver == "15.1(2)S" )        xever = "3.3.0S";
  if( iosver == "15.1(2)S1" )       xever = "3.3.1S";
  if( iosver == "15.1(2)S2" )       xever = "3.3.2S";
  if( iosver == "15.1(3)S" )        xever = "3.4.0S";
  if( iosver == "15.1(3)S0a" )      xever = "3.4.0aS";
  if( iosver == "15.1(3)S1" )       xever = "3.4.1S";
  if( iosver == "15.1(3)S2" )       xever = "3.4.2S";
  if( iosver == "15.1(3)S3" )       xever = "3.4.3S";
  if( iosver == "15.1(3)S4" )       xever = "3.4.4S";
  if( iosver == "15.1(3)S5" )       xever = "3.4.5S";
  if( iosver == "15.1(3)S6" )       xever = "3.4.6S";
  if( iosver == "15.1(3)S7" )       xever = "3.4.7S";
  if( iosver == "15.1(2)SG" )       xever = "3.4.0SG";
  if( iosver == "15.1(3)SG1" )      xever = "3.4.1SG";
  if( iosver == "15.1(3)SG2" )      xever = "3.4.2SG";
  if( iosver == "15.1(3)SG4" )      xever = "3.4.4SG";
  if( iosver == "15.1(3)SG5" )      xever = "3.4.5SG";
  if( iosver == "15.1(3)SG6" )      xever = "3.4.6SG";
  if( iosver == "15.1(3)SG7" )      xever = "3.4.7SG";
  if( iosver == "15.2(1)S" )        xever = "3.5.0S";
  if( iosver == "15.2(1)S1" )       xever = "3.5.1S";
  if( iosver == "15.2(1)S2" )       xever = "3.5.2S";
  if( iosver == "15.2(2)S" )        xever = "3.6.0S";
  if( iosver == "15.2(2)S1" )       xever = "3.6.1S";
  if( iosver == "15.2(2)S2" )       xever = "3.6.2S";
  if( iosver == "15.2(4)S" )        xever = "3.7.0S";
  if( iosver == "15.2(4)S1" )       xever = "3.7.1S";
  if( iosver == "15.3(3)S1xb" )     xever = "3.10.1xbS";
  if( iosver == "15.2(4)S2" )       xever = "3.7.2S";
  if( iosver == "15.2(4)S3" )       xever = "3.7.3S";
  if( iosver == "15.2(4)S4" )       xever = "3.7.4S";
  if( iosver == "15.2(4)S5" )       xever = "3.7.5S";
  if( iosver == "15.2(4)S6" )       xever = "3.7.6S";
  if( iosver == "15.2(4)E" )        xever = "3.8.0E";
  if( iosver == "15.3(1)S" )        xever = "3.8.0S";
  if( iosver == "15.3(1)S1" )       xever = "3.8.1S";
  if( iosver == "15.3(1)S2" )       xever = "3.8.2S";
  if( iosver == "15.3(2)S" )        xever = "3.9.0S";
  if( iosver == "15.3(2)S1" )       xever = "3.9.1S";
  if( iosver == "15.3(2)S2" )       xever = "3.9.2S";
  if( iosver == "15.3(3)S" )        xever = "3.10.0S";
  if( iosver == "15.3(3)S1" )       xever = "3.10.1S";
  if( iosver == "15.3(3)S2" )       xever = "3.10.2S";
  if( iosver == "15.3(3)S3" )       xever = "3.10.3S";
  if( iosver == "15.3(3)S4" )       xever = "3.10.4S";
  if( iosver == "15.3(3)S5" )       xever = "3.10.5S";
  if( iosver == "15.3(3)S6" )       xever = "3.10.6S";
  if( iosver == "15.3(3)S7" )       xever = "3.10.7S";
  if( iosver == "15.4(1)S" )        xever = "3.11.0S";
  if( iosver == "15.4(1)S1" )       xever = "3.11.1S";
  if( iosver == "15.4(1)S2" )       xever = "3.11.2S";
  if( iosver == "15.4(1)S3" )       xever = "3.11.3S";
  if( iosver == "15.4(2)S" )        xever = "3.12.0S";
  if( iosver == "15.4(2)S1" )       xever = "3.12.1S";
  if( iosver == "15.4(2)S2" )       xever = "3.12.2S";
  if( iosver == "15.4(3)S" )        xever = "3.13.0S";
  if( iosver == "15.4(3)S1" )       xever = "3.13.1S";
  if( iosver == "15.4(3)S2" )       xever = "3.13.2S";
  if( iosver == "15.4(3)S3" )       xever = "3.13.3S";
  if( iosver == "15.4(3)S4" )       xever = "3.13.4S";
  if( iosver == "15.4(3)S5" )       xever = "3.13.5S";
  if( iosver == "15.5(1)S" )        xever = "3.14.0S";
  if( iosver == "15.5(1)S1" )       xever = "3.14.1S";
  if( iosver == "15.5(1)S2" )       xever = "3.14.2S";
  if( iosver == "15.5(1)S3" )       xever = "3.14.3S";
  if( iosver == "15.5(1)S4" )       xever = "3.14.4S";
  if( iosver == "15.5(2)S" )        xever = "3.15.0S";
  if( iosver == "15.5(2)S1" )       xever = "3.15.1S";
  if( iosver == "15.5(2)S2" )       xever = "3.15.2S";
  if( iosver == "15.5(2)S3" )       xever = "3.15.3S";
  if( iosver == "15.5(3)S" )        xever = "3.16.0S";
  if( iosver == "15.5(3)S1" )       xever = "3.16.1S";
  if( iosver == "15.5(3)S1a" )      xever = "3.16.1aS";
  if( iosver == "15.5(3)Sc" )       xever = "3.16.0cS";
  if( iosver == "15.5(3)S2" )       xever = "3.16.2S";
  if( iosver == "15.5(3)S2a" )      xever = "3.16.2aS";
  if( iosver == "15.5(3)S3")        xever = "3.16.3S";
  if( iosver == "15.6(1)S" )        xever = "3.17.0S";
  if( iosver == "15.6(1)S1" )       xever = "3.17.1S";
  if( iosver == "15.6(2)S" )        xever = "3.18.0S";

  if( xever )
    return xever;
  else
  {
    v_s = split( iosver, sep:".", keep:FALSE );
    x = 0;

    foreach v ( v_s )
    {  # e.g. 03.05.0E
      _v = ereg_replace( pattern:"^0([0-9]+*)", replace:"\1", string:v );
      if( _v == "" ) _v = "0";

      if( x == 0 )
      {
        _version = _v;
        x++;
      }
      else
        _version += '.' + _v;
    }

    if( eregmatch( pattern:"([0-9]+)\.([0-9]+)\.([0-9]+)([a-zA-Z]+)", string:_version ) ) return _version;

    s = eregmatch( pattern:"([0-9]+)\.([0-9]+)\.([0-9]+)\.([a-zA-Z]+)", string:_version );
    if( ! isnull( s[1] ) ) return s[1] + "." + s[2] + "." + s[3] + s[4];

  }

  return iosver;
}

