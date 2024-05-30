###############################################################################
# OpenVAS Include File
# $Id: pkg-lib-deb.inc 14272 2019-03-18 14:29:52Z cfischer $
#
# Check if debian package is up to date
#
# Authors:
# Thomas Reinke <reinke@securityspace.com>
#
# Copyright:
# Copyright (c) 2007 E-Soft Inc. http://www.securityspace.com
#
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
# $Revision: 14272 $
#
###############################################################################

# This variable is defined at inclusion time. It's declared global and is set
# to TRUE if isdpkgvuln() is called for an OS version and a package name that
# match the target's configuration. It allows the callers to distinguish
# between non-installed packages and packages installed but not vulnerable.
__pkg_match = FALSE;

# Global keys to avoid that we're repeatedly asking redis for something we already know
global_var __dpkg_ssh_release, __dpkg_ssh_pkgs, __dpkg_kernel_overwrite, __dpkg_kernel_uname;

# @brief Checks if a user has requested to overwrite the reporting of vulnerabilities for non-running
#        Linux-Kernels (see lsc_options.nasl) and to collect them into a separate Reporting-VT (with
#        a lower severity). If this was requested the function will check if a passed package name is
#        related to a Linux Kernel. If a kernel package is detected the function will compare the version
#        of the package with the running kernel and chose where the information is logged to.
#
# @param vuln_pkg      The vulnerable package name to check.
# @param installed_ver The installed package version to check.
# @param fixed_ver     The version of the package including the fix.
#
# @return The report string to return to the VT calling @isdpkgvuln in the case the user hasn't requested
#         the overwrite of the reporting or the package isn't related to a Linux-Kernel, NULL if the user
#         has requested the overwrite of the reporting and the package is related to a Linux-Kernel and
#         the version of it doesn't match the running kernel, FALSE if no vuln_pkg, installed_ver or
#         fixed_ver parameter was passed.
#
function dpkg_check_kernel_overwrite( vuln_pkg, installed_ver, fixed_ver ) {

  local_var vuln_pkg, installed_ver, fixed_ver;
  local_var overwrite_enabled, report, kernel_uname;
  # nb: __dpkg_kernel_overwrite and __dpkg_kernel_uname are global vars from above

  if( ! vuln_pkg ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dpkg_check_kernel_overwrite#-#vuln_pkg" );
    return FALSE;
  }

  if( ! installed_ver ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dpkg_check_kernel_overwrite#-#installed_ver" );
    return FALSE;
  }

  if( ! fixed_ver ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dpkg_check_kernel_overwrite#-#fixed_ver" );
    return FALSE;
  }

  report  = 'Vulnerable package: ' + vuln_pkg + '\n';
  report += 'Installed version:  ' + installed_ver + '\n';
  report += 'Fixed version:      ' + fixed_ver + '\n\n';

  # nb: Those are the package names on Debian and Ubuntu related to Kernel-LSC checks
  if( vuln_pkg !~ "^(linux-(cloud-tools|compiler-gcc|doc|headers|image|kbuild|manual|perf|source|support|)-|xen-linux-system-|linux-libc-dev|linux-cpupower)" )
    return report;

  # nb: This evaluates the user setting from lsc_options.nasl
  if( isnull( __dpkg_kernel_overwrite ) ) {
    overwrite_enabled = get_kb_item( "ssh/login/kernel_reporting_overwrite/enabled" );
    if( ! overwrite_enabled )
      overwrite_enabled = FALSE;
    else
      overwrite_enabled = TRUE;

    __dpkg_kernel_overwrite = overwrite_enabled;
  } else {
    overwrite_enabled = __dpkg_kernel_overwrite;
  }

  # nb: If the overwrite is not enabled we don't need to continue and just returning the report.
  if( ! overwrite_enabled )
    return report;

  if( isnull( __dpkg_kernel_uname ) ) {
    kernel_uname = get_kb_item( "ssh/login/uname" );
    if( ! kernel_uname )
      kernel_uname = FALSE;

    __dpkg_kernel_uname = kernel_uname;
  } else {
    kernel_uname = __dpkg_kernel_uname;
  }

  # nb: If we didn't got the uname in gather-package-list.nasl (which shouldn't happen) we can't continue and need to return the report.
  if( ! kernel_uname )
    return report;

  # nb: If the installed_ver is included in the uname we know the running kernel is vulnerable.
  # e.g.:
  # kernel_uname:  Linux debian-stretch 4.9.0-3-amd64 #1 SMP Debian 4.9.30-2+deb9u2 (2017-06-26) x86_64
  # installed_ver: 4.9.30-2+deb9u2
  if( installed_ver >< kernel_uname )
    return report;

  # Otherwise save it into the KB for separate reporting in a Consolidation-VT with a lower severity
  set_kb_item( name:"ssh/login/inactive_kernel_vulns/available", value:TRUE );
  set_kb_item( name:"ssh/login/inactive_kernel_vulns/reports/" + get_script_oid() + "/" + vuln_pkg, value:report );
  return NULL;
}

# @brief returns the current release string of the target saved within the KB (ssh/login/release).
#
# @return a string containing the current release of the target or an empty string if no release
#         string exists in the KB.
#
# @todo Maybe move into a spearate (new) include if other pkg-lib-*.inc includes needs this as well.
#
function dpkg_get_ssh_release() {

  local_var rls;

  if( ! isnull( __dpkg_ssh_release ) ) {
    rls = string( __dpkg_ssh_release );
  } else {
    rls = string( get_kb_item( "ssh/login/release" ) );
    __dpkg_ssh_release = rls;
  }
  return rls;
}

# @brief returns the current installed packages of the target collected by gather-package-list.nasl
#        and saved within the KB (ssh/login/packages).
#
# @return a string containing the current installed packages of the target or an empty string if no
#         package list exists in the KB.
#
function dpkg_get_ssh_pkgs() {

  local_var pkgs;

  if( ! isnull( __dpkg_ssh_pkgs ) ) {
    pkgs = string( __dpkg_ssh_pkgs );
  } else {
    pkgs = string( get_kb_item( "ssh/login/packages" ) );
    __dpkg_ssh_pkgs = pkgs;
  }
  return pkgs;
}

# @brief used for string normalization that helps us with cases were even revcomp
#        can't handle things the way we would expect. For example, consider two
#        subsequent releases of cups:
#
#        1.3.8-1lenny5
#        1.3.8-1+lenny6 (yes, this really happened, but makes the results
#                        of revcomp incorrectly determine which rev is later)
#        1.3.8-1.lenny7 (ok, this didn't for this example, but did in others)
#
#        Normalization allows us to do things like change all instances to look like:
#
#        1.3.8-1lennyX
#
#        so that we can use revcomp to accurately determine which revision is the latest.
#        We're passing in rls so that we can use it if we need to - but for now,
#        the only instances causing us problems are debian lenny.
#
# @param str the string to normalize
# @param rls the release string of the target system, currently unused in this function
#
# @return the normalized string as explained in the description or NULL if no string was
#         passed via the str parameter
#
function dpkgnorm( str, rls ) {

  local_var str, rls, str2, m;

  if( ! str ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#dpkgnorm#-#str" );
    return NULL;
  }

  str2 = str_replace( find:"-lenny", string:str, replace:"lenny" );
  str2 = str_replace( find:"+lenny", string:str2, replace:"lenny" );
  str2 = str_replace( find:".lenny", string:str2, replace:"lenny" );
  str2 = str_replace( find:"-squeeze", string:str, replace:"squeeze" );
  str2 = str_replace( find:"+squeeze", string:str2, replace:"squeeze" );
  str2 = str_replace( find:".squeeze", string:str2, replace:"squeeze" );
  str2 = str_replace( find:"-wheezy", string:str, replace:"wheezy" );
  str2 = str_replace( find:"+wheezy", string:str2, replace:"wheezy" );
  str2 = str_replace( find:".wheezy", string:str2, replace:"wheezy" );
  m = eregmatch( pattern:"^([0-9]:)?(.*)$", string:str2 );
  if( isnull( m ) )
    return str2;

  return( m[2] );
}

# @brief isdpkgvuln is intended to provide a quick way to verify that a given deb
#        package, if it is installed, is vulnerable or not.
#
# @param pkg         the package name (e.g. cups from the example in dpkgnorm) to check.
# @param ver         the version of the package (e.g. 1.3.8-1+lenny6 from the example
#                    in dpkgnorm) which contains a fix for the related vulnerability.
# @param rls         a release string in a form of e.g. "DEB6" which needs to exactly
#                    match the collected release (set by gather-package-list.nasl) so
#                    that the vulnerability is checked.
#
# @return NULL if a package is not installed or is installed but up to date, a string
#         containing a report about found package and the current installed version of
#         it if it is installed and vulnerable.
#
function isdpkgvuln( pkg, ver, rls ) {

  local_var kbrls, pkgs, pat, matches, rc, pat_a, pat_b, report;

  if( ! pkg ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isdpkgvuln#-#pkg" );
    return NULL;
  }

  if( ! ver ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isdpkgvuln#-#ver" );
    return NULL;
  }

  if( ! rls ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#isdpkgvuln#-#rls" );
    return NULL;
  }

  # Check that we have the data for this release.
  kbrls = dpkg_get_ssh_release();
  if( kbrls != rls )
    return NULL;

  pkgs = dpkg_get_ssh_pkgs();
  if( ! pkgs )
    return NULL;

  # TBD: What about other ARCH variants like arm?
  # TODO: Consider moving to dpkgnorm
  pkgs = ereg_replace( pattern:":i386", replace:"", string:pkgs );
  pkgs = ereg_replace( pattern:":amd64", replace:"", string:pkgs );

  pat = string("ii  (", pkg, ") +([0-9]:)?([^ ]+)");
  matches = eregmatch( pattern:pat, string:pkgs );
  if( isnull( matches ) )
    return NULL;

  __pkg_match = TRUE; # Package was installed.

  # Normalize our two strings for known idiosyncrasies
  pat_a = dpkgnorm( str:matches[3], rls:rls );
  pat_b = dpkgnorm( str:ver, rls:rls );
  rc = revcomp( a:pat_a, b:pat_b );
  if( rc < 0 ) {
    report = dpkg_check_kernel_overwrite( vuln_pkg:pkg, installed_ver:matches[3], fixed_ver:ver );
    return report;
  }
  return NULL;
}