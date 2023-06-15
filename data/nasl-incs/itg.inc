# Copyright (C) 2014 Greenbone Networks GmbH
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


# @brief reporting function for IT-Grundschutz Kataloge (old IT-Grundschutz)
#
# @param itg_id the id if the requirement
#
# @return NULL
#
function itg_send_details(itg_id) {

  local_var itg_id;
  local_var result, desc, report;

  if(!itg_id) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#itg_id#-#itg_send_details");
    return;
  }

  result = get_kb_item(string(itg_id + "/result"));
  desc = get_kb_item(string(itg_id + "/desc"));

  report = 'Ergebnis:\t' + result +
           '\nDetails:\t' + desc + '\n\n';

  log_message(port:0, proto:"IT-Grundschutz", data:report);
}

# @brief deprecated reporting function (for new IT-grundschutz). Can be removed, once
#        each requirement has own VT (e.g. SYS.2.2.3.A1)
#
# @param oid oid from the policy control (with list result)
# @param requirement default value as stated in policy
# @param status_spec default value (enabled or disabled)
#
# @return array with results, title and fixtext
#
function itg_report_list(oid, requirement, status_spec) {

  local_var oid, requirement, status_spec;
  local_var results, title, result, not_compliant;

  results = get_kb_list("1.3.6.1.4.1.25623.1.0." + oid + "/RESULT/*");
  title = get_kb_item("1.3.6.1.4.1.25623.1.0." + oid + "/NAME");

  if(!results) {
    result["Status"] = "Fehler";
  } else {
    foreach id(keys(results)) {
      if(results[id] != requirement) {
        not_compliant = TRUE;
      }
    }
    if(not_compliant) {
      result["Status"] = "Nicht erfuellt";
    } else {
      result["Status"] = "Erfuellt";
    }
  }
  result['Titel'] = '"' + title + '" sollte ' + status_spec + ' werden';
  return result;
}

# @brief reporting for one policy control
#
# @param oid oid from the policy control (with one result, not list)
# @param requirement default value as stated in policy
# @param status_spec default value (enabled or disabled)
#
# @return array with results, title and fixtext
#
function itg_report_item(oid, requirement, status_spec) {

  local_var oid, requirement, status_spec;
  local_var results, title, result;

  results = get_kb_item("1.3.6.1.4.1.25623.1.0." + oid + "/RESULT");
  title = get_kb_item("1.3.6.1.4.1.25623.1.0." + oid + "/NAME");

  if(!results) {
    result['Status'] = 'Fehler';
  } else if(results != requirement) {
    result['Status'] = 'Erfuellt';
  } else {
    result['Status'] = 'Nicht erfuellt';
  }
  result['Titel'] = '"' + title + '" sollte ' + status_spec + ' werden';
  return result;
}

# @brief reporting function for new IT-Grundschutz
#
# @param title name of the requirement
# @param status compliance status
# @param details results from policy controls
#
# @return NULL
#
function itg_report(title, status, details) {

  local_var title, status, details;
  local_var tag, report;

  if(tolower(status) == "erfuellt") {
    tag = "OK";
  } else if(tolower(status) == "nicht erfuellt") {
    tag = "FAIL";
  } else if("diese Vorgabe muss manuell ueberprueft werden" >< tolower(status)) {
    tag = "NI";
  } else if("nicht zutreffend" >< tolower(status)) {
    tag = "NS";
  } else{
    tag = "ERR";
  }

  report  = 'Titel: ' + title;
  report += '\nStatus: ' + toupper(status);
  report += '\nTag: ' + tag;
  report += '\n________________\n';
  report += '\nDetails:' + details;

  log_message(port:0, proto:'IT-Grundschutz/Requirement', data:report);
}

# @brief checks if level of IT-Grundschutz requirement matches
#        given level so if it should be started
#
# @param level the level of the IT-Grundschutz requirement
#
# @return bool TRUE if requirement should be started, FALSE if not
#
function itg_start_requirement(level) {

  local_var level;
  local_var ITGLevel, selected_level;

  ITGLevel = get_kb_item("GSHB/level");
  if(ITGLevel == "Basis") {
    selected_level = 0;
  } else if(ITGLevel == "Standard") {
    selected_level = 1;
  } else {
    selected_level = 2;
  }

  if(level == "Basis") {
    level = 0;
  }else if(level == "Standard") {
    level = 1;
  }else{
    level = 2;
  }

  if(level <= selected_level) {
    return TRUE;
  }
  return FALSE;
}

# @brief creates the report for each Policy Control for the ITG Anforderung
#
# @param oid_list list of all Policy Controls included
#
# @return array with keys itg_detail_report, which is the detailed report for each Policy Control.
#         desc is the description as set for summary vt, compliant is the compliance status.
#
function itg_get_policy_control_result(oid_list){
  local_var oid_list, oid, result, title, default, compliant, status, itg_detail_report, desc, ret, compliance_status;

  compliance_status = TRUE;

  foreach oid (oid_list){
    result = get_kb_item(oid + "/RESULT");
    title = get_kb_item(oid + "/NAME");
    default = get_kb_item(oid + "/DEFAULT");
    compliant = get_kb_item(oid + "/COMPLIANT");

    if(tolower(compliant) != "yes"){
      compliance_status = FALSE;
      status = "Nicht erfuellt";
    }else{
      status = "Erfuellt";
    }

    itg_detail_report += '\n\nTest: ' + '\t\t' + title + " (OID: " + oid + ')\n';
    itg_detail_report += "Ergebnis: " + '\t' + result + '\n';
    itg_detail_report += "Vorgabe: " + '\t' + default + '\n';
    itg_detail_report += "Umgesetzt: " + '\t' + toupper(compliant);

    desc += '\n' + title + ": " + status;
  }

  ret = make_array("itg_detail_report", itg_detail_report, "desc", desc, "compliant", compliance_status);
  return(ret);
}