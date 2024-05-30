# Copyright (C) 2021 Greenbone Networks GmbH
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

# Texts are kept so output of scripts doesn't change in a delta
strengthtext["strong"]  = "'Strong'";
strengthtext["medium"]  = "'Medium'";
strengthtext["weak"]    = "'Weak'";
strengthtext["null"]    = "'Null'";
strengthtext["anon"]    = "'Anonymous'";
strengthtext["nonweak"] = "'Non Weak'";

# From ssl_funcs.inc
global_var version_string, version_kb_string_mapping, SSL_v2, SSL_v3, TLS_10, TLS_11, TLS_12, TLS_13;

# From ssl_funcs.inc
version_kb_string_mapping[SSL_v2] = "sslv2";
version_kb_string_mapping[SSL_v3] = "sslv3";
version_kb_string_mapping[TLS_10] = "tlsv1";
version_kb_string_mapping[TLS_11] = "tlsv1_1";
version_kb_string_mapping[TLS_12] = "tlsv1_2";
version_kb_string_mapping[TLS_13] = "tlsv1_3";

# From ssl_funcs.inc
version_string[SSL_v2] = "SSLv2";
version_string[SSL_v3] = "SSLv3";
version_string[TLS_10] = "TLSv1.0";
version_string[TLS_11] = "TLSv1.1";
version_string[TLS_12] = "TLSv1.2";
version_string[TLS_13] = "TLSv1.3";

# @brief Inner iterator for a cipherlist of given strength and protocol
#
# @param port The SSL/TLS port to get the list from
# @param version A valid SSL/TLS protocol version
# @param strength One of strong, medium, weak, null, anon
# @param negative Print output if no match for this level was found (optional)
#
# @return A string of found ciphers or NULL if none found
#
function print_cipherlist( port, version, strength, negative ) {

  local_var port, version, strength, negative;
  local_var cipherlistname, cipherlist, cipher, report;

  if( isnull( port ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#print_cipherlist" );
    return NULL;
  }

  if( isnull( version ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#print_cipherlist" );
    return NULL;
  }

  if( isnull( strength ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#strength#-#print_cipherlist" );
    return NULL;
  }

  report = NULL;

  cipherlistname = "secpod_ssl_ciphers/" + version_kb_string_mapping[version] + "/" + port + "/" +
                   strength + "_ciphers";
  cipherlist = get_kb_list( cipherlistname );
  if( ! isnull( cipherlist ) ) {
    report += strengthtext[strength] + ' cipher suites accepted by this service via the ' +
              version_string[version] + ' protocol:\n\n';

    # Sort to not report changes on delta reports if just the order is different
    cipherlist = sort( cipherlist );
    foreach cipher( cipherlist )
      report += cipher + '\n';
    report += '\n';
  } else {
    if( negative ) {
      report += 'No ' + strengthtext[strength] + ' cipher suites accepted by this service via the ' +
                version_string[version] + ' protocol.\n\n';
    }
  }

  return report;
}

# @brief Middle iterator over the requested strength(s) for a given protocol
#
# @param port The SSL/TLS port to get the list from
# @param version A valid SSL/TLS protocol version
# @param strengths Any one or more of strong, medium, weak, null, anon
# @param negative Print output if no match for this level was found
#
# @return A string of found ciphers or NULL if none found
#
function print_cipherlist_version( port, version, strengths, negative ) {

  local_var port, version, strengths, negative;
  local_var report;

  if( isnull( port ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#print_cipherlist_version" );
    return NULL;
  }

  if( isnull( version ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#print_cipherlist_version" );
    return NULL;
  }

  if( isnull( strengths ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#strengths#-#print_cipherlist_version" );
    return NULL;
  }

  report = NULL;

  # nb: since arrays can be reordered, this is unrolled
  if( "strong" >< strengths )
    report += print_cipherlist( port:port, version:version, strength:"strong", negative:negative );

  if( "medium" >< strengths )
    report += print_cipherlist( port:port, version:version, strength:"medium", negative:negative );

  if( "weak" >< strengths )
    report += print_cipherlist( port:port, version:version, strength:"weak", negative:negative );

  if( "null" >< strengths )
    report += print_cipherlist( port:port, version:version, strength:"null", negative:negative );

  if( "anon" >< strengths )
    report += print_cipherlist( port:port, version:version, strength:"anon", negative:negative );

  if( "nonweak" >< strengths )
    report += print_cipherlist( port:port, version:version, strength:"nonweak", negative:negative );

  return report;
}

# @brief Outer iterator over the requested SSL/TLS versions for a given protocol
#
# @param port The SSL/TLS port to get the list from
# @param versions Any one or more SSL/TLS protocol version(s)
# @param strengths Any one or more of strong, medium, weak, null, anon
# @param negative Print output if no match for this version/level was found (optional)
#
# @return A string of found ciphers or NULL if none found
#
function print_cipherlists( port, strengths, negative ) {

  local_var port, strengths, negative;
  local_var report, sup_ssl;

  if( isnull( port ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#print_cipherlists" );
    return NULL;
  }

  if( isnull( strengths ) ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#strengths#-#print_cipherlists" );
    return NULL;
  }

  report = NULL;

  if( ! sup_ssl = get_kb_item( "tls/supported/" + port ) )
    return NULL;

  # nb: since arrays can be reordered, this is unrolled
  # special handling for "nonweak" as all SSLv2 ciphers are weak
  if( "nonweak" >!< strengths ) {
    if( "SSLv2" >< sup_ssl )
      report += print_cipherlist( port:port, version:SSL_v2, strengths:strengths );
  }

  if( "SSLv3" >< sup_ssl )
    report += print_cipherlist_version( port:port, version:SSL_v3, strengths:strengths, negative:negative );

  if( "TLSv1.0" >< sup_ssl )
    report += print_cipherlist_version( port:port, version:TLS_10, strengths:strengths, negative:negative );

  if( "TLSv1.1" >< sup_ssl )
    report += print_cipherlist_version( port:port, version:TLS_11, strengths:strengths, negative:negative );

  if( "TLSv1.2" >< sup_ssl )
    report += print_cipherlist_version( port:port, version:TLS_12, strengths:strengths, negative:negative );

  if( "TLSv1.3" >< sup_ssl )
    report += print_cipherlist_version( port:port, version:TLS_13, strengths:strengths, negative:negative );

  return report;
}
