# This script was written by Thomas Reinke <reinke@securityspace.com>
#
# Copyright (c) 2005 E-Soft Inc. http://www.securityspace.com
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License Version 2
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA  02110-1301  USA
#

# This variable is defined at inclusion time. It's declared global and is set
# to TRUE if isrpmvuln() is called for an OS version and a package name that
# match the target's configuration. It allows the callers to distinguish
# between non-installed packages and packages installed but not vulnerable.
__pkg_match = FALSE;

# Global keys to avoid that we're repeatedly asking redis for something we already know
global_var __rpm_ssh_release, __rpm_ssh_rpms, __rpm_kernel_overwrite, __rpm_kernel_uname;

# @brief Checks if a user has requested to overwrite the reporting of vulnerabilities for non-running
#        Linux-Kernels (see lsc_options.nasl) and to collect them into a separate Reporting-VT (with
#        a lower severity). If this was requested the function will check if a passed package name is
#        related to a Linux Kernel. If a kernel package is detected the function will compare the version
#        of the package with the running kernel and chose where the information is logged to.
#
# @param vuln_pkg  The vulnerable package name to check, e.g. "kernel-package".
# @param inst_pkg  The installed package name to check, e.g. "kernel-default-4.4.27-2.1".
# @param fixed_pkg The package including the fix, e.g. "kernel-default-4.4.36-5.1".
#
# @return The report string to return to the VT calling @isrpmvuln in the case the user hasn't requested
#         the overwrite of the reporting or the package isn't related to a Linux-Kernel, NULL if the user
#         has requested the overwrite of the reporting and the package is related to a Linux-Kernel and
#         the version of it doesn't match the running kernel, FALSE if no vuln_pkg, inst_pkg, inst_pkg_ver
#         or fixed_pkg parameter was passed.
#
function rpm_check_kernel_overwrite( vuln_pkg, inst_pkg, fixed_pkg ) {

  local_var vuln_pkg, inst_pkg, fixed_pkg;
  local_var overwrite_enabled, report, kernel_uname, inst_pkg_ver;
  # nb: __rpm_kernel_overwrite and __rpm_kernel_uname are global vars from above

  if( ! vuln_pkg ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#rpm_check_kernel_overwrite#-#vuln_pkg" );
    return FALSE;
  }

  if( ! inst_pkg ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#rpm_check_kernel_overwrite#-#inst_pkg" );
    return FALSE;
  }

  if( ! fixed_pkg ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#rpm_check_kernel_overwrite#-#fixed_pkg" );
    return FALSE;
  }

  report  = 'Vulnerable package: ' + vuln_pkg + '\n';
  report += 'Installed version:  ' + inst_pkg + '\n';
  report += 'Fixed version:      ' + fixed_pkg + '\n\n';

  if( vuln_pkg !~ "^kernel(-[0-9a-zA-Z_-]+)?$" )
    return report;

  # nb: This evaluates the user setting from lsc_options.nasl
  if( isnull( __rpm_kernel_overwrite ) ) {
    overwrite_enabled = get_kb_item( "ssh/login/kernel_reporting_overwrite/enabled" );
    if( ! overwrite_enabled )
      overwrite_enabled = FALSE;
    else
      overwrite_enabled = TRUE;

    __rpm_kernel_overwrite = overwrite_enabled;
  } else {
    overwrite_enabled = __rpm_kernel_overwrite;
  }

  # nb: If the overwrite is not enabled we don't need to continue and just returning the report.
  if( ! overwrite_enabled )
    return report;

  if( isnull( __rpm_kernel_uname ) ) {
    kernel_uname = get_kb_item( "ssh/login/uname" );
    if( ! kernel_uname )
      kernel_uname = FALSE;

    __rpm_kernel_uname = kernel_uname;
  } else {
    kernel_uname = __rpm_kernel_uname;
  }

  # nb: If we didn't got the uname in gather-package-list.nasl (which shouldn't happen) we can't continue and need to return the report.
  if( ! kernel_uname )
    return report;

  # nb: If the installed_ver is included in the uname we know the running kernel is vulnerable.
  # On OpenSUSE Leap 42.2 e.g.:
  # kernel_uname: Linux opensuse-leap-422 4.4.74-18.20-default #1 SMP Fri Jun 30 19:01:19 UTC 2017 (b5079b8) x86_64 x86_64 x86_64 GNU/Linux
  # inst_pkg: kernel-default-4.4.74-18.20
  # or:
  # kernel_uname: Linux opensuse-leap-422 4.4.27-2-default #1 SMP Thu Nov 3 14:59:54 UTC 2016 (5c21e7c) x86_64 x86_64 x86_64 GNU/Linux
  # inst_pkg: kernel-default-4.4.27-2.1
  #
  # On CentOS 7.4 e.g.:
  # kernel_uname: Linux centos-74 3.10.0-693.el7.x86_64 #1 SMP Tue Aug 22 21:09:27 UTC 2017 x86_64 x86_64 x86_64 GNU/Linux
  # inst_pkg: kernel-3.10.0-693.el7
  #
  # nb: As shown above the last digit(s) is/are not always included in the uname line so we're doing a hacky
  # workaround to remove this for the match below so that we can match between uname version and installed version.
  inst_pkg_ver = ereg_replace( string:inst_pkg, pattern:"\.[0-9]+$", replace:"" );

  # and finally we need to extract the version from the inst_pkg string so that we can match
  # against the uname line.
  inst_pkg_ver = eregmatch( string:inst_pkg_ver, pattern:"^" + vuln_pkg + "-(.+)" );

  if( ! inst_pkg_ver[1] )
    return report; # nb: Fallback if something went wrong during the extraction above.

  if( inst_pkg_ver[1] >< kernel_uname )
    return report;

  # Otherwise save it into the KB for separate reporting in a Consolidation-VT with a lower severity
  # nb: Don't use the vuln_pkg variable for the second set_kb_item call below as this string isn't unique.
  set_kb_item( name:"ssh/login/inactive_kernel_vulns/available", value:TRUE );
  set_kb_item( name:"ssh/login/inactive_kernel_vulns/reports/" + get_script_oid() + "/" + inst_pkg, value:report );
  return NULL;
}

# @brief returns the current release string of the target saved within the KB (ssh/login/release).
#
# @return a string containing the current release of the target or an empty string if no release
#         string exists in the KB.
#
# @todo Maybe move into a spearate (new) include if other pkg-lib-*.inc includes needs this as well.
#
function rpm_get_ssh_release() {

  local_var rls;

  if( ! isnull( __rpm_ssh_release ) ) {
    rls = string( __rpm_ssh_release );
  } else {
    rls = string( get_kb_item( "ssh/login/release" ) );
    __rpm_ssh_release = rls;
  }
  return rls;
}

# @brief returns the current installed rpms of the target collected by gather-package-list.nasl
#        and saved within the KB (ssh/login/rpms).
#
# @return a string containing the current installed rpms of the target or an empty string if no
#         rpm list exists in the KB.
#
function rpm_get_ssh_rpms() {

  local_var rpms;

  if( ! isnull( __rpm_ssh_rpms ) ) {
    rpms = string( __rpm_ssh_rpms );
  } else {
    rpms = string( get_kb_item( "ssh/login/rpms" ) );
    __rpm_ssh_rpms = rpms;
  }
  return rpms;
}

# @brief Modifies/Transforms a passed package string (either collected by collected by gather-package-list.nasl or passed by the the VT calling
#        @isrpmvuln) into various representations of the string to be used in e.g. @revcomp or in the reporting. The purpose of this function
#        is to solve a few issues when comparing package versions outlined in the comments of this function.
#
# @param inst_pkg  The "plain" package name without the appended version (e.g. kernel for an installed kernel~2.6.9~89 package) passed by
#                  the VT calling @isrpmvuln.
#
# @param fixed_pkg The "full" package string including the appended version (e.g. kernel~2.6.9~89) passed by the VT calling @isrpmvuln.
#
# @param vuln_pkg  The "full" package string including the appended version (e.g. kernel~2.6.9~89.0.20) collected by gather-package-list.nasl.
#
# @return An array containing the following array indices:
#
#         ret_array["inst_pkg"]       = The modified/transformed package string (e.g. kernel~2.6.9~89 if kernel~2.6.9~89.EL was passed)
#                                       of the installed package version collected by gather-package-list.nasl to be used in @revcomp.
#         ret_array["fixed_pkg"]      = The modified/transformed package string (e.g. kernel~2.6.9~89.0.20 if kernel~2.6.9~89.0.20.EL was passed)
#                                       of the fixed package version passed by the VT calling @isrpmvuln to be used in @revcomp.
#         ret_array["norm_inst_pkg"]  = The modified/transformed package string (e.g. kernel-2.6.9-89.EL if kernel~2.6.9~89.EL was passed)
#                                       of the installed package version collected by gather-package-list.nasl to be used in the reporting.
#         ret_array["norm_fixed_pkg"] = The modified/transformed package string (e.g. kernel-2.6.9-89.0.20.EL if kernel~2.6.9~89.0.20.EL was passed)
#                                       of the fixed package version passed by the VT calling @isrpmvuln to be used in the reporting.
#
function rpmnorm( inst_pkg, fixed_pkg, vuln_pkg ) {

  local_var inst_pkg, fixed_pkg, vuln_pkg;
  local_var ret_array, _inst_pkg, _fixed_pkg, inst_pkg_substr, fixed_pkg_substr;
  local_var _inst_pkg_check, _fixed_pkg_check, norm_inst_pkg, norm_fixed_pkg, chomp;

  ret_array  = make_array();
  _inst_pkg  = inst_pkg;
  _fixed_pkg = fixed_pkg;

  # Splitting Package name and version because the below replace condition
  # would also replace _ with .(Dot) in the package name.
  inst_pkg_substr  = substr( _inst_pkg, strlen( vuln_pkg ) + 1, strlen( _inst_pkg ) );
  fixed_pkg_substr = substr( _fixed_pkg, strlen( vuln_pkg ) + 1, strlen( _fixed_pkg ) );

  # To replace _ with .(Dot) because a version match fails in some case where version is having _(underscore)
  if( fixed_pkg_substr =~ 'el[0-9]{1}_[0-9]{1}' && inst_pkg_substr !~ 'el[0-9]{1}_[0-9]{1}' )
    fixed_pkg_substr = str_replace( string:fixed_pkg_substr, find:'_', replace:'.' );

  else if( inst_pkg_substr =~ 'el[0-9]{1}_[0-9]{1}' && fixed_pkg_substr !~ 'el[0-9]{1}_[0-9]{1}' )
    inst_pkg_substr = str_replace( string:inst_pkg_substr, find:'_', replace:'.' );

  # Combining package name and version again.
  _inst_pkg  = vuln_pkg + '~' + inst_pkg_substr;
  _fixed_pkg = vuln_pkg + '~' + fixed_pkg_substr;

  # revcomp handles almost all situations intellligently. But, it can get caught on the following two string examples:
  #
  # a="kernel~2.6.9~89.EL", b="kernel~2.6.9~89.0.20.EL"
  #
  # a="kernel~3.10.0~693.el7", b="kernel~3.10.0~693.2.2.el7"
  #
  # One would expect 'a' to be < (older than) 'b'. However, in this case revcomp differentiates the 'E' from the '0',
  # and thinks that that b is as a result older (lexicographically less than) a. This is the only situation we've seen
  # where this type of suffix causes problems. So, as a patch, we solve the problem locally BEFORE we call revcomp, by
  # stripping from both strings the trailing '.EL[0-9]*' suffix, but only if both strings have the identical suffix.
  # If we find other systems where this type of problem occurred, it may make sense to consider if we should be stripping
  # equal trailing strings from the a/b parameters within revcomp. For now, though we'll do it here.

  _inst_pkg_check  = eregmatch( pattern:"^(.*)[.]([Ee][Ll][0-9]*)$", string:_inst_pkg );
  _fixed_pkg_check = eregmatch( pattern:"^(.*)[.]([Ee][Ll][0-9]*)$", string:_fixed_pkg );
  if( ! isnull( _inst_pkg_check ) && ! isnull( _fixed_pkg_check ) ) {
    if( _inst_pkg_check[2] == _fixed_pkg_check[2] ) {
      _inst_pkg  = _inst_pkg_check[1];
      _fixed_pkg = _fixed_pkg_check[1];
    }
  }

  if( eregmatch( pattern:"\.[0-9]{1}\.el[0-9]{1}", string:_inst_pkg ) && ! eregmatch( pattern:"\.[0-9]{1}\.el[0-9]{1}", string:_fixed_pkg ) )
    _fixed_pkg = ereg_replace( pattern:"(\.el[0-9]{1})", replace:".0\1", string:_fixed_pkg );

  norm_inst_pkg = "";
  foreach comp( split( inst_pkg, sep: "~", keep:FALSE ) )
    norm_inst_pkg = string( norm_inst_pkg, "-", comp );

  norm_inst_pkg = substr( norm_inst_pkg, 1 );

  norm_fixed_pkg = "";
  foreach comp( split( fixed_pkg, sep: "~", keep:FALSE ) )
    norm_fixed_pkg = string( norm_fixed_pkg, "-", comp );

  norm_fixed_pkg = substr( norm_fixed_pkg, 1 );

  ret_array["inst_pkg"]       = _inst_pkg;
  ret_array["fixed_pkg"]      = _fixed_pkg;
  ret_array["norm_inst_pkg"]  = norm_inst_pkg;
  ret_array["norm_fixed_pkg"] = norm_fixed_pkg;

  return ret_array;
}


# @brief provides a quick way to verify that a given rpm, if it is installed, is up to date.
#
# @param pkg the rpm name (e.g. gnutls-utils) to check.
# @param rpm the version of the rpm (e.g. gnutls-utils~1.4.1~3) which contains a fix for
#            the related vulnerability.
# @param rls a release string in a form of e.g. "FC6" which needs to exactly
#            match the collected release (set by gather-package-list.nasl) so
#            hat the vulnerability is checked.
#
# @return NULL if a rpm is not installed or is installed but up to date, a string
#         containing a report about the found rpm and the current installed version of
#         it if it is installed and vulnerable.
#
function isrpmvuln( pkg, rpm, rls ) {

  local_var pkg, rpm, rls;
  local_var kbrls, rpms, pat, matches, pkg_name, pkgs, report, report1, rpminfo;

  if( ! pkg ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isrpmvuln#-#pkg" );
    return NULL;
  }

  if( ! rpm ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isrpmvuln#-#rpm" );
    return NULL;
  }

  if( ! rls ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isrpmvuln#-#rls" );
    return NULL;
  }

  kbrls = rpm_get_ssh_release();
  if( kbrls != rls )
    return NULL;

  rpms = rpm_get_ssh_rpms();
  if( ! rpms || rpms == ";" )
    return NULL;

  # Must include a leading \n or ; in the package search to prevent an
  # overly permissive search (e.g. search for 'ash' must not match 'bash')
  pat = string( "[\n;](", pkg, "~[^;]+);" );
  matches = eregmatch( pattern:pat, string:rpms );
  if( ! matches[1] )
    return NULL;

  # If a kernel package is found this loop will check for every installed version of
  # it and reports all vulnerable versions and not only the first one.
  if( matches[1] =~ "^kernel(-[0-9a-zA-Z_-]+)?~" ) {

    pkg_name = split( rpms, sep:";", keep:TRUE );

    foreach pkgs( pkg_name ) {
      if( egrep( string:pkgs, pattern:string( "^", pkg, "~" ) ) ) {
        matches = eregmatch( pattern:"^(" + pkg + "~[^;]+)", string:pkgs );
        if( ! matches[1] )
          continue;

        rpminfo = rpmnorm( inst_pkg:matches[1], fixed_pkg:rpm, vuln_pkg:pkg );
        if( revcomp( a:rpminfo["inst_pkg"], b:rpminfo["fixed_pkg"] ) < 0 ) {
          report = rpm_check_kernel_overwrite( vuln_pkg:pkg, inst_pkg:rpminfo["norm_inst_pkg"], fixed_pkg:rpminfo["norm_fixed_pkg"] );
          if( report )
            report1 += report;
        }
      }
    }
    return chomp( report1 );
  }

  rpminfo = rpmnorm( inst_pkg:matches[1], fixed_pkg:rpm, vuln_pkg:pkg );
  __pkg_match = TRUE;

  # Now proceed with the normal revision comparison
  if( revcomp( a:rpminfo["inst_pkg"], b:rpminfo["fixed_pkg"] ) < 0 ) {
    # nb: Kernel packages should be already checked above but just to be sure...
    report = rpm_check_kernel_overwrite( vuln_pkg:pkg, inst_pkg:rpminfo["norm_inst_pkg"], fixed_pkg:rpminfo["norm_fixed_pkg"] );
    return report;
  }
  return NULL;
}