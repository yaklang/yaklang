###############################################################################
# OpenVAS Vulnerability Test Library functions
# $Id$
#
# WMI Client Functions for Windows Policies
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


##############################################################################
# CLASS: RSOP (Resultant Set Of Policy)
##############################################################################

function wmi_rsop_accesstoken(handle)
{
  query = 'Select * from RSOP_UserPrivilegeRight where precedence=1';

  rsopList = wmi_query_rsop(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< rsopList) || !rsopList){
    return(0);
  }

  return rsopList;
}

function wmi_rsop_auditpolicy(handle, select)
{
  if(select){
    query = 'Select ' + select + ' from RSOP_AuditPolicy where precedence=1';
  }else{
    query = 'Select * from RSOP_AuditPolicy where precedence=1';
  }

  rsopList = wmi_query_rsop(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< rsopList) || !rsopList){
    return(0);
  }

  return rsopList;
}

function wmi_rsop_policysetting(handle)
{
  query = 'Select * from RSOP_PolicySetting where precedence=1';

  rsopList = wmi_query_rsop(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< rsopList) || !rsopList){
    return(0);
  }

  return rsopList;
}

function wmi_rsop_userprivilegeright(handle, select){
  if(select){
    query = 'select ' + select + ' from RSOP_UserPrivilegeRight where precedence=1';
  }else{
    query = 'select * from RSOP_UserPrivilegeRight where precedence=1';
  }

  rsopList = wmi_query_rsop(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< rsopList) || !rsopList){
    return(0);
  }

  return rsopList;
}

function wmi_rsop_lockoutpolicy(handle)
{
  query = 'Select * from RSOP_SecuritySettingBoolean where precedence=1';

  rsopList = wmi_query_rsop(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< rsopList) || !rsopList){
    return(0);
  }

  return rsopList;
}

function wmi_rsop_passwdpolicy(handle)
{
  query = 'Select * from RSOP_SecuritySettingNumeric where precedence=1';

  rsopList = wmi_query_rsop(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< rsopList) || !rsopList){
    return(0);
  }

  return rsopList;
}
