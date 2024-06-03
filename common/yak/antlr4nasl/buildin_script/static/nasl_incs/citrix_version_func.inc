###############################################################################
# OpenVAS Include File
# $Id: citrix_version_func.inc 13333 2019-01-28 15:37:29Z cfischer $
#
# Functions for Citrix Version Checks
#
# Authors:
# Michael Meyer <michael.meyer@greenbone.net>
#
# Copyright:
# Copyright (c) 2017 Greenbone Networks GmbH
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
#
# $Revision: 13333 $
#
###############################################################################

function report_if_citrix_xenserver_is_vulnerable( version, hotfixes, patches )
{
  local_var version, hotfixes, patches;
  local_var a, av, fixes, patch;

  if( ! version || ! patches || ! hotfixes )
  {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version_patches_hotfixes#-#report_if_citrix_xenserver_is_vulnerable" );
    return;
  }

  if( ! is_array( patches ) )
  {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#patches_no_array#-#report_if_citrix_xenserver_is_vulnerable" );
    return;
  }

  a = eregmatch( pattern:"^([0-9]\.[0-9]\.[0-9])", string:version );
  if( isnull( a[1] ) )
    return;

  av = a[1];

  if( ! is_array( patches[ av ] ) )
    return;

  if( "No hotfixes installed" >< hotfixes )
    report_missing_xenserver_patch( version:version, fix:patches[av] );

  fixes = make_list();

  foreach patch ( patches[ av ] )
  {
    if( ( "ECC" >< hotfixes && "ECC" >!< patch ) || ( "ECC" >< patch && "ECC" >!< hotfixes ) )
      continue;

    if( patch >!< hotfixes )
      fixes = make_list( fixes, patch );
  }

  if( is_array( fixes ) )
    report_missing_xenserver_patch( version:version, fix:fixes );

  return;
}

function report_missing_xenserver_patch( version, fix )
{
  local_var version, fix;
  local_var report;

  report  = 'Installed version: ' + version + '\n';
  report += 'Missing hotfix:    ' + join( list:fix, sep:" / " );
  security_message( port:0, data:report );
  exit( 0 );
}