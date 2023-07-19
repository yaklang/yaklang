###############################################################################
# OpenVAS Vulnerability Test Library functions
# $Id$
#
# WMI Client Functions for Process
#
# Authors:
# Chandan S <schandan@secpod.com>
#
# Copyright:
# Copyright (c) 2009 Greenbone Networks GmbH, http://www.greenbone.net
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
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
###############################################################################


###############################################################################
# CLASS: Win32_Process Functions
#
# - wmi_proc_prop() - Returns List of all process with Properties.
# - wmi_proc_all()      - Returns Name of all process.
# - wmi_proc_path() - Returns installed path of the process.
#
# If procName = "svchost.exe", returns details of the Process
#
##############################################################################

function wmi_proc_prop(handle, procName)
{
  if(!procName){
    query = "Select * from Win32_Process";
  }
  else {
    query = 'Select * from Win32_Process Where Name = ' +
             raw_string(0x22) + procName + raw_string(0x22);
  }

  procList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< procList) || !procList){
    return(0);
  }

  return wmi_misc_split_res(List:procList);
}

function wmi_proc_all(handle, procName)
{
  if(!procName){
    query = "Select Caption from Win32_Process";
  }
  else {
    query = 'Select Caption from Win32_Process Where Name = ' +
             raw_string(0x22) + procName +raw_string(0x22);
  }

  procList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< procName) || !procList){
    return(0);
  }

  procList = ereg_replace(pattern:"\|[0-9a-zA-Z]+", string:procList, replace:"");
  return procList;
}

function wmi_proc_path(handle, procName)
{
  if(!procName){
    query = "Select ExecutablePath from Win32_Process";
  }
  else {
    query = 'Select ExecutablePath from Win32_Process Where Name = ' +
             raw_string(0x22) + procName + raw_string(0x22);
  }

  procList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< procList) || !procList){
    return(0);
  }

  procList = ereg_replace(pattern:"\|[0-9a-zA-Z]+", string:procList, replace:"");
  return procList;
}
