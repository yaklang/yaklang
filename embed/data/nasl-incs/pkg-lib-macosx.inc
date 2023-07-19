###################################################################
# OpenVAS Include File
# $Id: pkg-lib-macosx.inc 12537 2018-11-26 16:48:50Z cfischer $
#
# Mac OS X LSC helper library
#
# Copyright (C) 2010 LSS <http://www.lss.hr>
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public
# License along with this program. If not, see
# <http://www.gnu.org/licenses/>.
#
# $Revision: 12537 $
#
###################################################################

# Global keys to avoid that we're repeatedly asking redis for something we already know
global_var __macos_ssh_osxname, __macos_ssh_osxversion;

# @brief returns the current Mac OS release name of the target collected by gather-package-list.nasl
#        and saved within the KB (ssh/login/osx_name).
#
# @note Add a dependency to gather-package-list.nasl if you want to use this function.
#
# @return a string containing the current Mac OS release name of the target or an empty string if no release
#         string exists in the KB.
#
function kb_get_ssh_osxname() {

  local_var name;

  if( ! isnull( __macos_ssh_osxname ) ) {
    name = string( __macos_ssh_osxname );
  } else {
    name = string( get_kb_item( "ssh/login/osx_name" ) );
    __macos_ssh_osxname = name;
  }
  return name;
}

# @brief returns the current installed Mac OS version of the target collected by gather-package-list.nasl
#        and saved within the KB (ssh/login/osx_version).
#
# @note Add a dependency to gather-package-list.nasl if you want to use this function.
#
# @return a string containing the current installed Mac OS version of the target or an empty string if no
#         version exists in the KB.
#
function kb_get_ssh_osxversion() {

  local_var version;

  if( ! isnull( __macos_ssh_osxversion ) ) {
    version = string( __macos_ssh_osxversion );
  } else {
    version = string( get_kb_item( "ssh/login/osx_version" ) );
    __macos_ssh_osxversion = version;
  }
  return version;
}

# @brief Checks if the target is a Mac OS (ssh/login/osx_name) and that the
#        release version (ssh/login/osx_version) is matching the provided regex.
#
# @param vers_regex The regex which is used to check the release version against
# @param name_regex The regex (optional) which is used to check the release name against.
#                   If no regex is provided "Mac OS X" is used by default.
#
# @note Add a dependency to gather-package-list.nasl if you want to use this function.
#
# @return TRUE if the target OS is a Mac OS and the release version is matching
#         the provided regex, FALSE otherwise and NULL if not all required parameters
#         are passed.
#
function kb_check_macos_release( vers_regex, name_regex ) {

  local_var vers_regex, name_regex;
  local_var os_name, os_ver;

  if( ! vers_regex ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#vers_regex#-#check_ssh_macos_release" );
    return;
  }

  if( ! name_regex )
    name_regex = "Mac OS X";

  os_name = kb_get_ssh_osxname();
  if( ! os_name || ! egrep( string:os_name, pattern:name_regex ) ) # nb: !~ can't handle variables so egrep is used here
    return FALSE;

  os_ver = kb_get_ssh_osxversion();
  if( ! os_ver || ! egrep( string:os_ver, pattern:vers_regex ) ) # nb: !~ can't handle variables so egrep is used here
    return FALSE;
  else
    return TRUE;
}

# Mac OS X update pkg name consists of two parts:
# the fixed part and the diff part
# This function looks up all the pkgs in the list
# that match the fixed part, and checks if the diff part
# in the list is greater that the given one.
#

function isosxpkgvuln(fixed, diff) {
    list=get_kb_item("ssh/login/osx_pkgs");
    if (!list) exit (0);
    max_diff=0;
    list=split(list);
    foreach i (keys(list)) if (fixed >< chomp(list[i])) {
        s=(substr(chomp(list[i]), strlen(fixed), strlen(fixed)+strlen(diff)-1));
        if (s>max_diff) {max_diff = s;}
    }
    if (diff>max_diff) return 1; else return 0;
}

# Checks if pkg name exists in pkg list #
function pkg_in_list(name, version) {
    list=get_kb_item("ssh/login/osx_pkgs");
    if (!list) exit (0);
    list=split(list);
    foreach i (keys(list)) if (name == chomp(list[i])) {
        if (version == 0 || version == chomp(list[i+1])) return 1;
    }
    return 0;
}

# Returns the release (major) version of Mac OS X #
function osx_rls_name(rls) {
    r=eregmatch(pattern:"[a-z A-Z]+[0-9]+\.[0-9]+", string:rls);
    return r[0];
}

# Returns the full (minor) version of Mac OS X #
function osx_ver(ver) {
    v=eregmatch(pattern:"[0-9.]+", string:ver);
    return v[0];
}

# Checks if the release is less than the ones in the list #
function rlsnotsupported(rls, list) {
    min = eregmatch (pattern:"[0-9]+.[0-9]+", string:list[0]);
    foreach ver (list) {
        r = eregmatch(pattern:"[0-9]+.[0-9]+", string:ver);
        if (min[0] > r[0]) { r=min; display (min);}
    }
    rls = eregmatch(pattern:"[0-9]+.[0-9]+", string:rls);
    if (rls[0] < min[0]) return 1; else return 0;
}
