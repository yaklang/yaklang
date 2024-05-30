###############################################################################
# OpenVAS Include File
# $Id: version_func.inc 12405 2018-11-19 08:42:37Z cfischer $
#
# Helper functions for various version related tests
#
# Authors:
# Carsten Koch-Mauthe <c.koch-mauthe at dn-systems.de>
#
# Copyright:
# Copyright (C) 2008 Greenbone Networks GmbH
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
# $Revision: 12405 $
#
###############################################################################

# XXX: the version tests should be eventually consolidated with
# the methods from revisions-lib.inc.

function get_string_version( text, ver_pattern ) {

  local_var text, ver_pattern, loc_version;

  if( ! text ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#text#-#get_string_version" );

  if( isnull( ver_pattern) ) {
    ver_pattern = "([0-9\.]+)"; # Standard Version Pattern for most cases
  }

  loc_version = eregmatch( pattern:ver_pattern, string:text );
  if( ! isnull( loc_version ) )
    loc_version[max_index( loc_version )] = text;
  return loc_version;
}

function version_is_less( version, test_version, icase, debug ) {

  local_var version, test_version, icase, debug;

  if( ! version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#version_is_less" );
  if( ! test_version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#test_version#-#version_is_less" );
  return( version_test( version:version, test_version:test_version, less:TRUE, icase:icase, debug:debug ) );
}

function version_is_equal( version, test_version, icase, debug ) {

  local_var version, test_version, icase, debug;

  if( ! version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#version_is_equal" );
  if( ! test_version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#test_version#-#version_is_equal" );
  return( version_test( version:version, test_version:test_version, less:FALSE, icase:icase, debug:debug ) );
}

function version_test( version, test_version, less, icase, debug ) {

  # TODO: Implement 1.0.0a > 1.0.0alpha/beta/rc

  local_var ver_ary, test_ary, ver_num, ver_sep, i, r, s, char_found, test_prerelease, ver_prerelease;
  local_var version, test_version, less, icase, debug;

  if( isnull( version ) || version == "" || version == 0 || version == '0' || version == "unknown" || version == "Unknown" || version == "." ) return FALSE;

  if( isnull( icase ) ) icase = TRUE;
  if( icase ) {
    version = tolower( version );
    test_version = tolower( test_version );
  }

  if( debug ) display('DEBUG: Initial version: ' + version + ' and test_version: ' + test_version + '\n');

  # set / use a standard separator for the version and test_version parameter
  ver_sep = "."; # Set Standard Separator

  # Replace commonly used version separators (e.g. 1.0 rc1, or 1.0-rc1) with the standard separator.
  # nb: The "\.\." replacement is a fallback for NVTs passing doubled dots to the version comparison functions.
  # Such a doubled point would e.g. report "1.0..99" to be a lower version then 1.0.1
  ##For Java NVTs updated regex, 'update_' to be replaced by '.'. 1.8.0_151 , 1.8.0update_51, 1.7.0update1
  version = ereg_replace( pattern:"( |\-|\.\.|_|update_)", string:version, replace:"." );

  ##Next few lines for Java only
  ##For case 1.7.0update1 --> 1.7.0.update.1
  if(version =~ "[0-9]update[0-9]"){
    version = ereg_replace( pattern:"update", string:version, replace:".update.");
  }
  ##Few Java NVTs are doing this, removing e.g. '-b11'
  version = ereg_replace( pattern:"-b[0-9][0-9]", string:version, replace:"");

  test_version = ereg_replace( pattern:"( |\-|\.\.|_)", string:test_version, replace:"." );

  if( debug ) display('DEBUG: version: ' + version + ' and test_version: ' + test_version + ' after applying standard separator\n');

  # Some versions have e.g. 1.0.0g or 1.0.0beta1 so add a dot between them
  version = ereg_replace( pattern:"([0-9])([A-Za-z])", string:version, replace:"\1.\2" );
  test_version = ereg_replace( pattern:"([0-9])([A-Za-z])", string:test_version, replace:"\1.\2" );

  if( debug ) display('DEBUG: version: ' + version + ' and test_version: ' + test_version + ' after adding dots\n');

  # Split the versions into an array based on the ver_sep above
  ver_ary = split( version, sep:ver_sep, keep:FALSE );
  test_ary = split( test_version, sep:ver_sep, keep:FALSE );

  while( max_index( ver_ary ) < max_index( test_ary ) ) {
    ver_ary[max_index( ver_ary )] = "0";
  }

  while( max_index( ver_ary ) > max_index( test_ary ) ) {
    test_ary[max_index( test_ary )] = "0";
  }

  foreach i( keys( test_ary ) ) {

    r = eregmatch( pattern:"([0-9]+)", string:test_ary[i] );
    s = eregmatch( pattern:"([A-Za-z])", string:test_ary[i] );
    test_prerelease = eregmatch( pattern:"(rc|alpha|beta)([0-9]+)", string:test_ary[i], icase:TRUE );

    if( isnull( s ) ) {
      test_ary[i] = int( r[0] ) * 128; # Changed to * 128 so if a char follows a number the number is more significant
      char_found = FALSE;
    } else {
      if( isnull( test_prerelease ) ) {
        test_ary[i] = ( int( r[0] ) * 128 ) + ord( s[0] );
      } else {
        test_ary[i] = test_prerelease[2];
      }
      char_found = TRUE;
    }

    r = eregmatch( pattern:"([0-9]+)", string:ver_ary[i] );
    s = eregmatch( pattern:"([A-Za-z])", string:ver_ary[i] );
    ver_prerelease = eregmatch( pattern:"(rc|alpha|beta)([0-9]+)", string:ver_ary[i], icase:TRUE );

    if( isnull( s ) ) {
      ver_ary[i] = int( r[0] ) * 128;
    } else if( char_found ) {
      if( isnull( ver_prerelease ) ) {
        ver_ary[i] = ( int(r[0] ) * 128 ) + ord( s[0] );
      } else {
        ver_ary[i] = ver_prerelease[2];
      }
    } else {
      if( isnull( r ) ) {
        ver_ary[i] = ord( s[0] );
      } else {
        if( ! less ) return FALSE;    # If char found in test_version and no char in version it is not equal
        ver_ary[i] = int(r[0]) * 128; # No chars if test_version has no chars on this position else 1.1.1a is > 1.1.2
      }
    }

    if( less ) {
      if( ver_ary[i] < test_ary[i] ) return TRUE;
      if( ver_ary[i] > test_ary[i] ) return FALSE;
    } else {
      if( ver_ary[i] != test_ary[i] ) return FALSE;
    }
  }
  if( less )
    return FALSE;
  else
    return TRUE;
}

function version_is_less_equal( version, test_version, icase, debug ) {

  local_var version, test_version, icase, debug;

  if( ! version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#version_is_less_equal" );
  if( ! test_version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#test_version#-#version_is_less_equal" );
  if( version_is_equal( version:version, test_version:test_version, icase:icase, debug:debug ) ) return TRUE;
  if( version_is_less( version:version, test_version:test_version, icase:icase, debug:debug ) ) return TRUE;
  return FALSE;
}

function version_is_greater_equal( version, test_version, icase, debug ) {

  local_var version, test_version, icase, debug;

  if( ! version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#version_is_greater_equal" );
  if( ! test_version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#test_version#-#version_is_greater_equal" );
  if( version_is_equal( version:version, test_version:test_version, icase:icase, debug:debug ) ) return TRUE;
  if( version_is_less( version:test_version, test_version:version, icase:icase, debug:debug ) ) return TRUE;
  return FALSE;
}

function version_is_greater( version, test_version, icase, debug ) {

  local_var version, test_version, icase, debug;

  if( ! version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#version_is_greater" );
  if( ! test_version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#test_version#-#version_is_greater" );
  if( version_is_less( version:test_version, test_version:version, icase:icase, debug:debug ) ) return TRUE;
  return FALSE;
}

function version_in_range( version, test_version, test_version2, icase, debug ) {

  local_var version, test_version, test_version2, icase, debug;

  if( ! version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#version_in_range" );
  if( ! test_version ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#test_version#-#version_in_range" );
  if( ! test_version2 ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#test_version2#-#version_in_range" );
  if( version_is_greater_equal( version:version, test_version:test_version, icase:icase, debug:debug ) ) {
    if( version_is_less_equal( version:version, test_version:test_version2, icase:icase, debug:debug ) ) return TRUE;
  }
  return FALSE;
}

# @brief Checks if the given version passed via the 'version' parameter is in the range of the
#        versions given via the 'test_version_lo' (lower version) and 'test_version_up' parameters
#        (upper version).
#
# @note This function works 'exclusive' by default for the version passed via the 'test_version_up'
#       parameter, means that it is excluded when determining if the passed version (via the
#       'version' parameter) is included within the tested range. This default was chosen because
#       most common vendor advisories are giving a starting range of affected versions as the lowest
#       version and the first fixed version.
#
# @param version         The version to be tested.
# @param test_version_lo The "lower" version to be tested against.
# @param test_version_up The "upper" version to be tested against.
# @param lower_exclusive If set to TRUE (default is FALSE) the version passed via the
#                        'test_version_lo' parameter is excluded when determining if the passed
#                        version (via the 'version' parameter) is included within the tested range.
# @param upper_exclusive If set to TRUE (default is TRUE) the version passed via the
#                        'test_version_up' parameter is excluded when determining if the passed
#                        version (via the 'version' parameter) is included within the tested range.
# @param icase           TRUE (the default) if the check should be case insensitive, FALSE if otherwise.
# @param debug           If set to TRUE additional debug logging about the version comparison is done
#                        on the command line.
#
# @return TRUE if the version passed via the 'version' parameter is in the range of the versions
#         given via the 'test_version_lo' (lower version, inclusive by default) and
#         'test_version_up' parameters (upper version, exclusive by default), FALSE otherwise.
#
function version_in_range_exclusive( version, test_version_lo, test_version_up, lower_exclusive, upper_exclusive, icase, debug ) {

  local_var version, test_version_lo, test_version_up, lower_exclusive, upper_exclusive, icase, debug;

  if( ! version )
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#version_in_range_exclusive" );

  if( ! test_version_lo )
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#test_version_lo#-#version_in_range_exclusive" );

  if( ! test_version_up )
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#test_version_up#-#version_in_range_exclusive" );

  if( version =~ "^\s+" || version =~ "\s+$" )
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#version_in_range_exclusive: Trailing / leading space passed to 'version' parameter which might show an unexpected behavior." );

  if( test_version_lo =~ "^\s+" || test_version_lo =~ "\s+$" )
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#version_in_range_exclusive: Trailing / leading space passed to 'test_version_lo' parameter which might show an unexpected behavior." );

  if( test_version_up =~ "^\s+" || test_version_up =~ "\s+$" )
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#version_in_range_exclusive: Trailing / leading space passed to 'test_version_up' parameter which might show an unexpected behavior." );

  # nb: Set some defaults according to the function description.
  if( isnull( lower_exclusive ) )
    lower_exclusive = FALSE;

  if( isnull( upper_exclusive ) )
    upper_exclusive = TRUE;

  if( lower_exclusive && upper_exclusive ) {
    if( version_is_greater( version:version, test_version:test_version_lo, icase:icase, debug:debug ) ) {
      if( version_is_less( version:version, test_version:test_version_up, icase:icase, debug:debug ) )
        return TRUE;
    }
  }

  else if( lower_exclusive ) {
    if( version_is_greater( version:version, test_version:test_version_lo, icase:icase, debug:debug ) ) {
      if( version_is_less_equal( version:version, test_version:test_version_up, icase:icase, debug:debug ) )
        return TRUE;
    }
  }

  else if( upper_exclusive ) {
    if( version_is_greater_equal( version:version, test_version:test_version_lo, icase:icase, debug:debug ) ) {
      if( version_is_less( version:version, test_version:test_version_up, icase:icase, debug:debug ) )
        return TRUE;
    }
  }

  # nb: Basically what version_in_range() is doing.
  else {
    if( version_is_greater_equal( version:version, test_version:test_version_lo, icase:icase, debug:debug ) ) {
      if( version_is_less_equal( version:version, test_version:test_version_up, icase:icase, debug:debug ) )
        return TRUE;
    }
  }

  return FALSE;
}

function get_version_from_kb( port, app ) {

  local_var port, app, version, matches, vers;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_version_from_kb" );
  if( ! app ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#app#-#get_version_from_kb" );

  if( isnull( port ) || isnull( app ) ) return FALSE;

  if( ! version = get_kb_item( "www/" + port + "/" + app ) ) return FALSE;
  if( ! matches = eregmatch( string:version, pattern:"^(.+) under (/.*)$" ) ) return FALSE;

  vers = matches[1];

  if( isnull( vers ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#vers#-#get_version_from_kb" );
    return FALSE;
  }

  if( vers == "unknown" || vers == "Unknown" ) return FALSE;

  return string( vers );
}

function get_dir_from_kb( port, app ) {

  local_var port, app, version, matches, dir;

  if( ! port ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_dir_from_kb" );
  if( ! app ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#app#-#get_dir_from_kb" );

  if( isnull( port ) || isnull( app ) ) return FALSE;

  if( ! version = get_kb_item( "www/" + port + "/" + app ) ) return FALSE;
  if( ! matches = eregmatch( string:version, pattern:"^(.+) under (/.*)$" ) ) return FALSE;

  dir = matches[2];

  if( isnull( dir ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dir#-#get_dir_from_kb" );
    return FALSE;
  }
  return string( dir );
}

# add  "script_dependencies("smb_nativelanman.nasl");" to your NVT to make this function work
function get_samba_version() {

  local_var lanman, version;

  if( ! lanman = get_kb_item( "SMB/NativeLanManager" ) ) return FALSE;
  if( "Samba" >!< lanman ) return FALSE;

  if( ! version = eregmatch( pattern:"Samba ([0-9.]+)", string:lanman ) ) return FALSE;
  if( isnull( version[1] ) ) return FALSE;

  return version[1];
}

## This function will return the version of the given file
## nb: If you use this you need to include secpod_smb_func.inc
function fetch_file_version( sysPath, file_name ) {

  local_var sysPath, file_name;
  local_var kbsysPath, kb_proxy_file, kb_proxy, share, file, sysVer;

  if( ! sysPath )   set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#sysPath#-#fetch_file_version" );
  if( ! file_name ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#file_name#-#fetch_file_version" );

  ## Query KB with specified file name first.
  ## Convert file name and sysPath to lower case.
  file_name = tolower( file_name );
  kbsysPath = tolower( sysPath ); # sysPath is currently not a local_var

  ## Construct KB string with file name.
  kb_proxy_file = "SMB//fetch_file_version//" + kbsysPath + "//" + file_name;

  ## Try to get the file version if already exists in KB
  kb_proxy = get_kb_item( kb_proxy_file );
  if( ! isnull( kb_proxy ) || kb_proxy ) {
    # Set to unknown to avoid that we're querying the version again if we already know that the file doesn't exist
    if( kb_proxy == "unknown" )
      return FALSE;
    else
      return kb_proxy;
  }

  share = ereg_replace( pattern:"([A-Za-z]):.*", replace:"\1$", string:sysPath );
  file  = ereg_replace( pattern:"[A-Za-z]:(.*)", replace:"\1", string:sysPath + "\" + file_name );

  sysVer = GetVer( file:file, share:share ); # from secpod_smb_func.inc
  if( ! sysVer || isnull( sysVer ) || sysVer == "" || sysVer == 0 || sysVer == '0' ) {
    set_kb_item( name:kb_proxy_file, value:"unknown" );
    return FALSE;
  }

  ## set KB string with file name if its not exists already
  set_kb_item( name:kb_proxy_file, value:sysVer );

  return sysVer;
}

########################################################################
#
# Get the product version from file
#
########################################################################
function get_version( dllPath, string, offs ) {

  local_var dllPath, string, offs;
  local_var name, login, pass, domain, port, share, file, soc, r, prot, uid, tid, fid, prVersion;

  if( ! dllPath ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dllPath#-#get_version" );
    return FALSE;
  }

  name    =  kb_smb_name();
  login   =  kb_smb_login();
  pass    =  kb_smb_password();
  domain  =  kb_smb_domain();
  port    =  kb_smb_transport();

  share = ereg_replace(pattern:"([A-Z]):.*", replace:"\1$", string:dllPath);
  file =  ereg_replace(pattern:"[A-Z]:(.*)", replace:"\1", string:dllPath);

  soc = open_sock_tcp(port);
  if(!soc){
    exit(0);
  }

  r = smb_session_request(soc:soc, remote:name);
  if(!r){
    close(soc);
    exit(0);
  }

  prot = smb_neg_prot(soc:soc);
  if(!prot){
    close(soc);
    exit(0);
  }

  r = smb_session_setup(soc:soc, login:login, password:pass, domain:domain, prot:prot);
  if(!r){
    close(soc);
    exit(0);
  }

  uid = session_extract_uid(reply:r);
  if(!uid){
    close(soc);
    exit(0);
  }

  r = smb_tconx(soc:soc, name:name, uid:uid, share:share);
  if(!r){
    close(soc);
    exit(0);
  }

  tid = tconx_extract_tid(reply:r);
  if(!tid){
    close(soc);
    exit(0);
  }

  fid = OpenAndX(socket:soc, uid:uid, tid:tid, file:file);
  if(!fid){
    close(soc);
    exit(0);
  }

  prVersion = GetVersion(socket:soc, uid:uid, tid:tid, fid:fid, verstr:string, offset:offs);
  if( ! prVersion || isnull( prVersion ) || prVersion == "" || prVersion == 0 || prVersion == '0' ) {
    close(soc);
    return FALSE;
  }

  close(soc);
  return prVersion;
}

function report_fixed_ver( installed_version, installed_build, installed_patch, vulnerable_range, file_checked,
                           file_version, fixed_version, fixed_build, fixed_patch, extra, install_path, install_url ) {

  local_var installed_version, installed_build, installed_patch, vulnerable_range, file_checked;
  local_var file_version, fixed_version, fixed_build, fixed_patch, extra, install_path, install_url, report;

  if( ! installed_version && ! file_checked ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#installed_version/file_checked#-#report_fixed_ver" );
  if( ! fixed_version && ! vulnerable_range ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#fixed_version/vulnerable_range#-#report_fixed_ver" );

  if( installed_version ) report += 'Installed version: ' + installed_version + '\n';
  if( file_checked )      report += 'File checked:      ' + file_checked + '\n';
  if( file_version )      report += 'File version:      ' + file_version + '\n';
  if( vulnerable_range )  report += 'Vulnerable range:  ' + vulnerable_range + '\n';
  if( fixed_version )     report += 'Fixed version:     ' + fixed_version + '\n';
  if( installed_build )   report += 'Installed build:   ' + installed_build + '\n';
  if( installed_patch )   report += 'Installed patch:   ' + installed_patch + '\n';
  if( fixed_build )       report += 'Fixed build:       ' + fixed_build + '\n';
  if( fixed_patch )       report += 'Fixed patch:       ' + fixed_patch + '\n';
  if( install_path ) {
                          report += 'Installation\n';
                          report += 'path / port:       ' + install_path + '\n';
  }
  if( install_url )       report += 'Installation URL:  ' + install_url + '\n';
  if( extra )             report += '\n' + extra + '\n';

  return report;
}
