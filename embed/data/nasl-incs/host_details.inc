###############################################################################
# OpenVAS Include File
#
# Centralized and organized host information base
#
# Authors:
# Henri Doreau <henri.doreau@greenbone.net>
#
# Copyright:
# Copyright (c) 2011 Greenbone Networks GmbH, http://www.greenbone.net
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

# Used in get_app_port/get_app_location to return a default
# location if none was previously detected and optimize_test
# is set to "no".
global_var optimize_test_enabled;
optimize_test_enabled = get_preference( "optimize_test" );

# Those are for debugging purposes (see get_app_version() and get_app_location())
global_var _get_app_version_used, _get_app_location_used;

##
# This script exports a function to store host information in a structured
# way.
#
# <host>
#   <detail>
#     <name>OS</name>
#     <value>Debian</value>
#     <method>
#       <nvt>1.2.3.4.5.6</nvt>
#       <description>NVT description</description>
#     </method>
#   </detail>
#   <detail>
#     <name>OS</name>
#     <value>cpe:/o:debian_linux:debian:5.0</value>
#     <method>
#       <nvt>1.2.3.4</nvt>
#       <description>another NVT description</description>
#     </method>
#   </detail>
# </host>
#
#
#
# For this example, we would use these KB entries:
# ------------------------------------------------
#
# HostDetails                     => "OS"
# HostDetails/NVT                 => "1.2.3.4.5.6", "1.2.3.4"
# --
# HostDetails/NVT/1.2.3.4.5.6     => "NVT Description"
# HostDetails/NVT/1.2.3.4.5.6/OS  => "Debian"
# --
# HostDetails/NVT/1.2.3.4         => "another NVT description"
# HostDetails/NVT/1.2.3.4/OS      => "cpe:/o:debian_linux:debian:5.0"
##

# Best OS detection sources, sorted by decreasing accuracy (best is first).
OS_CPE_SRC = make_list(
    "1.3.6.1.4.1.25623.1.0.103220", # gb_greenbone_os_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105383", # gb_ami_megarac_sp_web_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103633", # gb_ros_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103628", # gb_apple_mobile_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103418", # gb_vmware_esx_web_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103417", # gb_vmware_esx_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.96206",  # gb_ssh_cisco_ios_get_version.nasl
    "1.3.6.1.4.1.25623.1.0.105544", # gb_cisco_cucmim_version.nasl
    "1.3.6.1.4.1.25623.1.0.105540", # gb_cisco_cucm_version.nasl
    "1.3.6.1.4.1.25623.1.0.105690", # gb_cisco_nx_os_version.nasl
    "1.3.6.1.4.1.25623.1.0.105068", # gb_cyclades_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105313", # gb_fortios_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105440", # gb_cisco_esa_version.nasl
    "1.3.6.1.4.1.25623.1.0.105441", # gb_cisco_wsa_version.nasl
    "1.3.6.1.4.1.25623.1.0.105433", # gb_cisco_csma_version.nasl
    "1.3.6.1.4.1.25623.1.0.105533", # gb_cisco_ip_phone_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105532", # gb_cisco_ios_xr_version.nasl
    "1.3.6.1.4.1.25623.1.0.96200",  # gb_ssh_junos_get_version.nasl
    "1.3.6.1.4.1.25623.1.0.105263", # gb_palo_alto_panOS_version.nasl
    "1.3.6.1.4.1.25623.1.0.105266", # gb_screenos_version.nasl
    "1.3.6.1.4.1.25623.1.0.106413", # gb_extremeos_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108566", # gb_tippingpoint_sms_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.106513", # gb_cisco_asa_version_snmp.nasl
    "1.3.6.1.4.1.25623.1.0.105222", # gb_cisco_asa_version.nasl
    "1.3.6.1.4.1.25623.1.0.105033", # gb_cisco_asa_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106494", # gb_arista_eos_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108307", # gb_netgear_prosafe_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.108311", # gb_hirschmann_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.810608", # gb_mikrotik_router_routeros_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.105144", # gb_xenserver_version.nasl
    "1.3.6.1.4.1.25623.1.0.105659", # gb_cisco_ios_xe_version.nasl
    "1.3.6.1.4.1.25623.1.0.105155", # gb_mcafee_email_gateway_version.nasl
    "1.3.6.1.4.1.25623.1.0.140058", # gb_brocade_netiron_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108335", # gb_brocade_fabricos_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.105244", # gb_arubaos_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106864", # gb_cyberoam_umt_ngfw_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106875", # gb_aerohive_hiveos_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103875", # gb_qnap_nas_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103786", # gb_synology_dsm_detect.nasl
    "1.3.6.1.4.1.25623.1.0.142078", # gb_drobo_nas_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.108557", # gb_euleros_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106096", # gb_simatic_s7_version.nasl
    "1.3.6.1.4.1.25623.1.0.140738", # gb_simatic_cp_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.140748", # gb_simatic_scalance_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.140811", # gb_siemens_ruggedcom_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.20285",  # ilo_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106078", # gb_watchguard_fireware_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108338", # gb_vibnode_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.108347", # gb_hyperip_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.103910", # gb_avm_fritz_box_detect.nasl
    "1.3.6.1.4.1.25623.1.0.810234", # gb_dlink_dap_detect.nasl
    "1.3.6.1.4.1.25623.1.0.812377", # gb_dlink_dsl_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106015", # gb_dlink_dns_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103689", # gb_dlink_dir_detect.nasl
    "1.3.6.1.4.1.25623.1.0.113293", # gb_dlink_dwr_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108488", # gb_wd_mycloud_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.108491", # gb_intelbras_ncloud_devices_http_detect.nasl
    "1.3.6.1.4.1.25623.1.0.141923", # gb_netapp_data_ontap_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.141736", # gb_ricoh_iwb_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108494", # gb_codesys_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.141684", # gb_simatic_hmi_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.141766", # gb_wago_plc_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.141772", # gb_rockwell_micrologix_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.142010", # gb_rockwell_powermonitor_http_detect.nasl
    "1.3.6.1.4.1.25623.1.0.114075", # gb_beward_ip_cameras_detect_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.114095", # gb_zavio_ip_cameras_detect.nasl
    "1.3.6.1.4.1.25623.1.0.114097", # gb_tp_link_ip_cameras_detect.nasl
    "1.3.6.1.4.1.25623.1.0.114099", # gb_pearl_ip_cameras_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105789", # gb_riverbed_steelcentral_version.nasl
    "1.3.6.1.4.1.25623.1.0.106845", # gb_riverbed_steelhead_ssh_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106844", # gb_riverbed_steelhead_http_detect.nasl
    "1.3.6.1.4.1.25623.1.0.96207",  # gb_windows_cpe_detect.nasl
    "1.3.6.1.4.1.25623.1.0.50282",  # gather-package-list.nasl
    "1.3.6.1.4.1.25623.1.0.105614", # gb_cisco_pis_version.nasl
    "1.3.6.1.4.1.25623.1.0.140454", # gb_checkpoint_fw_version.nasl
    "1.3.6.1.4.1.25623.1.0.103621", # gb_smb_windows_detect.nasl
    "1.3.6.1.4.1.25623.1.0.112309", # gb_nec_communication_platforms_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105586", # gb_ssh_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.104000", # nmap_net.nasl
    "1.3.6.1.4.1.25623.1.0.103809", # gb_junos_snmp_version.nasl
    "1.3.6.1.4.1.25623.1.0.105271", # gb_citrix_netscaler_version.nasl
    "1.3.6.1.4.1.25623.1.0.103429", # gb_snmp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108014", # gb_dns_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.105355", # gb_ftp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.102011", # smb_nativelanman.nasl
    "1.3.6.1.4.1.25623.1.0.103979", # gb_ucs_detect.nasl
    "1.3.6.1.4.1.25623.1.0.111067", # sw_http_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.111068", # sw_mail_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.111069", # sw_telnet_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108192", # gb_mysql_mariadb_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.100292", # apcnisd_detect.nasl
    "1.3.6.1.4.1.25623.1.0.10884",  # ntp_open.nasl
    "1.3.6.1.4.1.25623.1.0.101013", # remote-detect-MDNS.nasl
    "1.3.6.1.4.1.25623.1.0.10144",  # mssqlserver_detect.nasl
    "1.3.6.1.4.1.25623.1.0.140000", # gb_apple_tv_version.nasl
    "1.3.6.1.4.1.25623.1.0.105899", # gb_apple_tv_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108200", # gb_upnp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108201", # gb_sip_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.140096", # gb_check_mk_agent_detect.nasl
    "1.3.6.1.4.1.25623.1.0.100062", # ms_rdp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105330", # gb_apache_activemq_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108044", # dcetest.nasl
    "1.3.6.1.4.1.25623.1.0.108284", # gb_hnap_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108565", # gb_ident_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108342", # gb_pihole_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105112", # gb_dropbear_ssh_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108451", # gb_rtsp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108455", # gb_nntp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108447", # gb_android_adb_detect.nasl nb: Down here because an ADB service could be run on an Android emulator as well...
    "1.3.6.1.4.1.25623.1.0.10150",  # netbios_name_get.nasl
    "1.3.6.1.4.1.25623.1.0.108021", # gb_nmap_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.102002"  # os_fingerprint.nasl
);

OS_TXT_SRC = make_list(
    "1.3.6.1.4.1.25623.1.0.103220", # gb_greenbone_os_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105383", # gb_ami_megarac_sp_web_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103633", # gb_ros_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103628", # gb_apple_mobile_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103418", # gb_vmware_esx_web_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103417", # gb_vmware_esx_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.96206",  # gb_ssh_cisco_ios_get_version.nasl
    "1.3.6.1.4.1.25623.1.0.105544", # gb_cisco_cucmim_version.nasl
    "1.3.6.1.4.1.25623.1.0.105540", # gb_cisco_cucm_version.nasl
    "1.3.6.1.4.1.25623.1.0.105690", # gb_cisco_nx_os_version.nasl
    "1.3.6.1.4.1.25623.1.0.105068", # gb_cyclades_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105313", # gb_fortios_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105440", # gb_cisco_esa_version.nasl
    "1.3.6.1.4.1.25623.1.0.105441", # gb_cisco_wsa_version.nasl
    "1.3.6.1.4.1.25623.1.0.105433", # gb_cisco_csma_version.nasl
    "1.3.6.1.4.1.25623.1.0.105533", # gb_cisco_ip_phone_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105532", # gb_cisco_ios_xr_version.nasl
    "1.3.6.1.4.1.25623.1.0.96200",  # gb_ssh_junos_get_version.nasl
    "1.3.6.1.4.1.25623.1.0.105263", # gb_palo_alto_panOS_version.nasl
    "1.3.6.1.4.1.25623.1.0.105266", # gb_screenos_version.nasl
    "1.3.6.1.4.1.25623.1.0.106413", # gb_extremeos_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108566", # gb_tippingpoint_sms_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.106513", # gb_cisco_asa_version_snmp.nasl
    "1.3.6.1.4.1.25623.1.0.105222", # gb_cisco_asa_version.nasl
    "1.3.6.1.4.1.25623.1.0.105033", # gb_cisco_asa_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106494", # gb_arista_eos_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106875", # gb_aerohive_hiveos_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103875", # gb_qnap_nas_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103786", # gb_synology_dsm_detect.nasl
    "1.3.6.1.4.1.25623.1.0.142078", # gb_drobo_nas_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.108557", # gb_euleros_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106096", # gb_simatic_s7_version.nasl
    "1.3.6.1.4.1.25623.1.0.140738", # gb_simatic_cp_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.140748", # gb_simatic_scalance_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.140811", # gb_siemens_ruggedcom_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.20285",  # ilo_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106078", # gb_watchguard_fireware_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108307", # gb_netgear_prosafe_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.108311", # gb_hirschmann_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.810608", # gb_mikrotik_router_routeros_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.105144", # gb_xenserver_version.nasl
    "1.3.6.1.4.1.25623.1.0.105659", # gb_cisco_ios_xe_version.nasl
    "1.3.6.1.4.1.25623.1.0.105155", # gb_mcafee_email_gateway_version.nasl
    "1.3.6.1.4.1.25623.1.0.140058", # gb_brocade_netiron_snmp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108335", # gb_brocade_fabricos_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.105244", # gb_arubaos_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106864", # gb_cyberoam_umt_ngfw_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108338", # gb_vibnode_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.108347", # gb_hyperip_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.103910", # gb_avm_fritz_box_detect.nasl
    "1.3.6.1.4.1.25623.1.0.810234", # gb_dlink_dap_detect.nasl
    "1.3.6.1.4.1.25623.1.0.812377", # gb_dlink_dsl_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106015", # gb_dlink_dns_detect.nasl
    "1.3.6.1.4.1.25623.1.0.103689", # gb_dlink_dir_detect.nasl
    "1.3.6.1.4.1.25623.1.0.113293", # gb_dlink_dwr_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108488", # gb_wd_mycloud_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.108491", # gb_intelbras_ncloud_devices_http_detect.nasl
    "1.3.6.1.4.1.25623.1.0.141923", # gb_netapp_data_ontap_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.141736", # gb_ricoh_iwb_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108494", # gb_codesys_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.141684", # gb_simatic_hmi_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.141766", # gb_wago_plc_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.141772", # gb_rockwell_micrologix_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.142010", # gb_rockwell_powermonitor_http_detect.nasl
    "1.3.6.1.4.1.25623.1.0.114075", # gb_beward_ip_cameras_detect_consolidation.nasl
    "1.3.6.1.4.1.25623.1.0.114095", # gb_zavio_ip_cameras_detect.nasl
    "1.3.6.1.4.1.25623.1.0.114097", # gb_tp_link_ip_cameras_detect.nasl
    "1.3.6.1.4.1.25623.1.0.114099", # gb_pearl_ip_cameras_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105789", # gb_riverbed_steelcentral_version.nasl
    "1.3.6.1.4.1.25623.1.0.106845", # gb_riverbed_steelhead_ssh_detect.nasl
    "1.3.6.1.4.1.25623.1.0.106844", # gb_riverbed_steelhead_http_detect.nasl
    "1.3.6.1.4.1.25623.1.0.96207",  # gb_windows_cpe_detect.nasl
    "1.3.6.1.4.1.25623.1.0.50282",  # gather-package-list.nasl
    "1.3.6.1.4.1.25623.1.0.105614", # gb_cisco_pis_version.nasl
    "1.3.6.1.4.1.25623.1.0.140454", # gb_checkpoint_fw_version.nasl
    "1.3.6.1.4.1.25623.1.0.103621", # gb_smb_windows_detect.nasl
    "1.3.6.1.4.1.25623.1.0.112309", # gb_nec_communication_platforms_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105586", # gb_ssh_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.104000", # nmap_net.nasl
    "1.3.6.1.4.1.25623.1.0.105271", # gb_citrix_netscaler_version.nasl
    "1.3.6.1.4.1.25623.1.0.103809", # gb_junos_snmp_version.nasl
    "1.3.6.1.4.1.25623.1.0.103429", # gb_snmp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108014", # gb_dns_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.105355", # gb_ftp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.102011", # smb_nativelanman.nasl
    "1.3.6.1.4.1.25623.1.0.103979", # gb_ucs_detect.nasl
    "1.3.6.1.4.1.25623.1.0.111067", # sw_http_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.111068", # sw_mail_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.111069", # sw_telnet_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108192", # gb_mysql_mariadb_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.100292", # apcnisd_detect.nasl
    "1.3.6.1.4.1.25623.1.0.10884",  # ntp_open.nasl
    "1.3.6.1.4.1.25623.1.0.101013", # remote-detect-MDNS.nasl
    "1.3.6.1.4.1.25623.1.0.10144",  # mssqlserver_detect.nasl
    "1.3.6.1.4.1.25623.1.0.140000", # gb_apple_tv_version.nasl
    "1.3.6.1.4.1.25623.1.0.105899", # gb_apple_tv_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108200", # gb_upnp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108201", # gb_sip_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.140096", # gb_check_mk_agent_detect.nasl
    "1.3.6.1.4.1.25623.1.0.100062", # ms_rdp_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105330", # gb_apache_activemq_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108044", # dcetest.nasl
    "1.3.6.1.4.1.25623.1.0.108284", # gb_hnap_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108565", # gb_ident_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108342", # gb_pihole_detect.nasl
    "1.3.6.1.4.1.25623.1.0.105112", # gb_dropbear_ssh_detect.nasl
    "1.3.6.1.4.1.25623.1.0.108451", # gb_rtsp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108455", # gb_nntp_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.108447", # gb_android_adb_detect.nasl nb: Down here because an ADB service could be run on an Android emulator as well...
    "1.3.6.1.4.1.25623.1.0.10150",  # netbios_name_get.nasl
    "1.3.6.1.4.1.25623.1.0.108021", # gb_nmap_os_detection.nasl
    "1.3.6.1.4.1.25623.1.0.102002"  # os_fingerprint.nasl
);

# @brief Register the specified info as a "HostDetail" into the knowledge base for use by other VTs.
#
# @param name  The name of the HostDetail to register,
# @param value The value to assign to the specified HostDetail name,
# @param desc  A text description of the registered HostDetail,
#
# @return NULL
#
function register_host_detail( name, value, desc ) {

  local_var name, value, desc, nvt, tokens;

  if( ! name ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#name#-#register_host_detail" );
    return;
  }

  if( ! value ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#value#-#register_host_detail" );

  nvt = get_script_oid();
  if( ! nvt )
    return;

  if( name != "port" ) {
    set_kb_item( name:"HostDetails", value:name );
    set_kb_item( name:"HostDetails/NVT", value:nvt );
    if( ! isnull( desc ) )
      replace_kb_item( name:"HostDetails/NVT/" + nvt, value:desc );
  }

  if( value )
    set_kb_item( name:"HostDetails/NVT/" + nvt + "/" + name, value:value );

  # gather-package-list.nasl sets precise OS CPE keys that we use to efficiently schedule LSCs.
  if( name == "OS" && "cpe:/o:" >< value && nvt == "1.3.6.1.4.1.25623.1.0.50282" ) {
    tokens = split(value, sep:":", keep:FALSE);

    if( tokens && tokens[0] && tokens[1] && tokens[2] && tokens[3] )
      set_kb_item( name:string( "HostDetails/OS/", tokens[0], ":", tokens[1], ":", tokens[2], ":", tokens[3]), value:TRUE );
  }
}

# @brief Registers a specific product as an "App" HostDetails to the scanner/manager for later
#        use by other VTs and internal functions.
#
# @param cpe      The cpe to assign to the registered product.
# @param location The location to assign to the registered product, e.g. 80/tcp or /path/to/folder.
# @param port     The port to assign to the registered product, e.g. 80 or 443.
# @param proto    The protocol to assign to the registered product, e.g. "tcp" or "udp". Defaults to "tcp" if none given.
# @param service  The service to assign to the registered product, e.g. "www", "mysql" or "local".
#
# @return NULL
#
function register_product( cpe, location, port, proto, service ) {

  local_var cpe, location, port, proto, service, kb_e, base_cpe;

  register_host_detail( name:"App", value:cpe );
  register_host_detail( name:cpe, value:location );
  if( port )
    register_host_detail( name:"port", value:port );

  # store an entry under the "base" cpe, so we could fork later for each entry under a cpe
  if( ! base_cpe = get_base_cpe( cpe:cpe ) ) {
    set_kb_item( name:"nvt_debug_cpe_syntax/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#register_product" );
    base_cpe = cpe;
  }

  if( ! proto )
    proto = "tcp";

  if( service )
    kb_e = get_script_oid() + '#-#' + cpe + '#-#' + location + '#-#' + port + '#-#' + proto + '#-#' + service;
  else
    kb_e = get_script_oid() + '#-#' + cpe + '#-#' + location + '#-#' + port + '#-#' + proto;

  set_kb_item( name:'get_app/' + base_cpe, value:kb_e );

  if( ! isnull( port ) ) {
    if( port == 0 ) {
      set_kb_item( name:'get_app/ports/' + base_cpe, value:"local" ); # For products identified locally. We currently can't set a value of 0 here
    } else {
      set_kb_item( name:'get_app/ports/' + base_cpe, value:port );
    }

    set_kb_item( name:'get_app/protocols/' + base_cpe, value:proto );

    if( service )
      set_kb_item( name:'get_app/services/' + port + "/" + base_cpe, value:service );
  }
}

# @brief Returns a list of HostDetails matching the specified key, e.g. host_details_list( key:"OS" ).
#
# @param key The key to return the matching HostDetails, e.g. "OS" or "App".
#
# @return A list containing the matching HostDetails or NULL if no host details exists.
#
function host_details_list( key ) {
  local_var key;
  return get_kb_list( "HostDetails/NVT/*/" + key );
}

# @brief Register the specified info as a "HostDetail" in a XML format directly to the scanner/manager (as a "Host_Details" proto log_message).
#
# @param name  The name of the HostDetail to register.
# @param value The value to assign to the specified HostDetail name.
# @param nvt   The name of the VT registering the HostDetails.
# @param desc  A text description of the registered HostDetail.
#
# @return NULL
#
function report_host_detail_single( name, value, nvt, desc ) {

  local_var name, value, nvt, desc, report;

  if( ! name ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#name#-#report_host_detail_single" );
  if( ! value ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#value#-#report_host_detail_single" );
  if( ! nvt ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#nvt#-#report_host_detail_single" );

  report = '';

  report += xml_open_tag( tag:'host' );
  report += xml_open_tag( tag:'detail' );
  report += xml_tagline( tag:'name', value:name );
  report += xml_tagline( tag:'value', value:value );
  report += xml_open_tag( tag:'source' );
  report += xml_tagline( tag:'type', value:"nvt" );
  report += xml_tagline( tag:'name', value:nvt );

  if( ! isnull( desc ) )
    report += xml_tagline( tag:'description', value:desc );
  else
    report += '<description/>';

  report += xml_close_tag( tag:'source' );
  report += xml_close_tag( tag:'detail' );
  report += xml_close_tag( tag:'host' );

  log_message( proto:"Host_Details", data:report );
}

# @brief Iterate over the host details and report them individually. Additionally report best CPE and TXT available concerning the remote OS.
#
# @return NULL
#
function report_host_details() {

  local_var names, nvts, nvt, desc, name, details, item;

  # get the names of the host details we gathered
  names = get_kb_list( "HostDetails" );

  # list of concerned NVTs
  nvts = get_kb_list( "HostDetails/NVT" );
  foreach nvt( nvts ) {

    # description of the current NVT
    desc = get_kb_item( "HostDetails/NVT/" + nvt );

    # iterate over the registered entries...
    foreach name( names ) {
      details = get_kb_list( "HostDetails/NVT/" + nvt + "/" + name );

      # add a section in case the current NVT has some value for this entry
      foreach item( details ) {
        if( ! isnull( item ) ) {
          report_host_detail_single( name:name, value:item, nvt:nvt, desc:desc );
        }
      }
    }
  }

  # Report best OS entries
  report_best_os_cpe();
  report_best_os_txt();
}

# @brief Returns the info if the operating system of the remote host is matching a specified pattern.
#
# @param _FCT_ANON_ARGS[0] The pattern to match against the remote operating system.
#
# @return "yes" if the remote operating system is matching the specified ospattern, "no" not matching or
#         "unknown" in other cases where it wasn't possible to determine the status of the OS.
#
function host_runs() {

  local_var ospattern, best_os;

  ospattern = _FCT_ANON_ARGS[0];
  if( ! ospattern )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#_FCT_ANON_ARGS[0]#-#host_runs" );

  #TBD/TODO: What if "best_os_txt" is "better" then "best_os_cpe"?
  best_os = best_os_cpe();
  if( isnull( best_os ) ) {
    best_os = best_os_txt();
    if( isnull( best_os ) )
      return "unknown";
  }

  #nb: update this to include possible cpes and txt from ssh_detect.nasl, gb_ssh_os_detection.nasl, gather-package-list.nasl and other *_os_detection.nasl
  if( "linux" >< tolower( ospattern ) )
    ospattern = '(' + ospattern + '|apc:aos|ubuntu|debian|bsd|opensuse|fedora|suse|red ?hat|centos|mandriva|mandrake|mageia|slackware|connectiva|amazon|oracle|d-link(:| )d|euleros|' +
                'turbolinux|gentoo|cisco|apple|fortios|junos|screenos|univention|esx|xen|pan-?os|qnap|synology|greenbone|hyperip|simatic|ruggedcom|vibnode|avm(:| )fritz|my_?cloud|' +
                'intelbras(:| )ncloud|ricoh:iwb|nucleus(_| )rtos|vxworks|chip-rtos|simatic(_| )hmi|ibm(:| )os(/|_)400|novell|rockwell ?automation(:| )(micrologix|powermonitor)|beward|' +
                'tippingpoint(_| )operating(_| )system|data(_| )ontap|(zavio|tp-link|pearl)(:| )ip(_| )camera|riverbed(_| )optimization(_| )system)';

  if( eregmatch( pattern:ospattern, string:best_os, icase:TRUE ) )
    return "yes";

  return "no";
}

# @brief Returns the CPE of the "best" determined operating system running on the remote host.
#
# @return A string containing the CPE of the "best" OS, NULL if it wasn't possible to determine the best one.
#
function best_os_cpe() {

  local_var oid, res, entry;

  foreach oid( OS_CPE_SRC ) {
    res = get_kb_list( "HostDetails/NVT/" + oid + "/OS" );
    if( ! isnull( res ) ) {
      res = make_list( res );
      foreach entry( res ) {
        # both CPE and free-form entries can be registered under the "OS" banner
        if( "cpe:/" >< entry )
          return entry;
      }
    }
  }
  return NULL;
}

# @brief Returns the name (text form) of the "best" determined operating system running on the remote host.
#
# @return A string containing the name of the "best" OS, NULL if it wasn't possible to determine the best one.
#
function best_os_txt() {

  local_var oid, res, entry;

  foreach oid( OS_TXT_SRC ) {
    res = get_kb_list( "HostDetails/NVT/" + oid + "/OS" );
    if( ! isnull( res ) ) {
      res = make_list( res );
      foreach entry( res ) {
        # both CPE and free-form entries can be registered under the "OS" banner
        if( "cpe:/" >!< entry )
          return entry;
      }
    }
  }
  return NULL;
}

# @brief Registers the CPE of the "best" determined operating system running on the remote host as a HostDetail.
#
# @return NULL
#
function report_best_os_cpe() {

  local_var best_os, desc;

  best_os = get_kb_item( "HostDetails/OS/BestMatch" );
  if( best_os ) {
    desc = get_kb_item( "HostDetails/OS/BestMatch/Details" );
    desc = split( desc, sep:";" );
    report_host_detail_single( name:"best_os_cpe", value:best_os, nvt:desc[0], desc:desc[1] );
    return;
  }
}

# @brief Registers the name (text form) of the "best" determined operating system running on the remote host as a HostDetail.
#
# @return NULL
#
function report_best_os_txt() {

  local_var oid, res, best_os, desc;

  foreach oid( OS_TXT_SRC ) {
    res = get_kb_list( "HostDetails/NVT/" + oid + "/OS" );

    if( ! isnull( res ) ) {
      res = make_list( res );

      foreach best_os( res ) {

        # discard CPE entries
        if( "cpe:/" >< best_os )
          continue;

        # in case we have several entries from this script...
        if( os_conflict( oslist:res, bestos:best_os ) )
          best_os = best_os + ' [possible conflict]';

        desc = get_kb_item( "HostDetails/NVT/" + oid );
        report_host_detail_single( name:"best_os_txt", value:best_os, nvt:oid, desc:desc );
        return;
      }
    }
  }
}

# @brief Returns a list of CPEs containing all the registered entries for applications on the remote host.
#
# @return A list of containing all the registered entries for applications on the remote host, an empty list
#         if no application where registered.
#
function host_details_cpes() {

  local_var res, details_list, item;

  res = make_list();

  # get all the registered entries for applications
  details_list = host_details_list( key:"App" );

  if( ! isnull( details_list ) )
    foreach item( details_list )
      if( "cpe:/" >< item )
        res = make_list( res, item );

  # get the best CPE we have for operating system (if any)
  item = best_os_cpe();
  if( ! isnull( item ) )
    res = make_list( res, item );

  return res;
}

# @brief Evaluates if the passed list contains at least one element of the same type (cpe or text).
#        TODO: Not really sure what the functions is actually doing / responsible for...
#
# @param oslist A list of operating system CPEs to match against.
# @param bestos The CPE of the current "Best OS" registered.
#
# @return TRUE if the list contains at least one element of the same type (cpe or text), FALSE otherwise.
#
function os_conflict( oslist, bestos ) {

  local_var oslist, bestos, is_cpe, item;

  is_cpe = FALSE;

  if( "cpe:/" >< bestos )
    is_cpe = TRUE;

  foreach item( oslist ) {
    if( item == bestos )
      continue;

    # return true if the list contains at least one element of the same type (cpe or text)
    if( ( "cpe:/" >< item ) == is_cpe )
      return TRUE;
  }
  return FALSE;
}

# @brief Unused function. TODO: What is the function doing? Do we need to keep it?
#
# @param cpe
# @param loc
#
# @return
#
function get_app( cpe, loc ) {

  local_var cpe, loc;
  local_var host_details, host_detail, host_values, res, port, location, locations, l;

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app" );
    return;
  }

  host_details = get_kb_list( "HostDetails/NVT/*" );
  if( ! host_details )
    return NULL;

  foreach host_detail( keys( host_details ) ) {

    if( cpe >< host_detail ) {

      host_values = split( host_detail, sep:"/", keep:FALSE );

      if( isnull( host_values[2] ) || isnull( host_values[4] ) || "cpe:" >!< host_values[3] )
        return NULL;

      res = make_array();

      res["OID"] = host_values[2];
      res["CPE"] = 'cpe:/' +  host_values[4];

      port = get_kb_item( "HostDetails/NVT/" + res["OID"] + "/port" ); # nb: this could fork

      if( port )
        res["port"] = port;

      # TODO: This looks broken. Why is the "loc" ignored here and the same (but only the first entry) of
      # a list below is used instead?
      if( loc ) {
        location = get_kb_item( "HostDetails/NVT/" + res["OID"] + "/" + res["CPE"] ); # nb: this could fork
        res["LOCATION"] = location;
      } else {
        locations = get_kb_list( "HostDetails/NVT/" + res["OID"] + "/" + res["CPE"] );
        l = make_list( locations );
        if( max_index( l ) == 1 )
          location = l[0];
      }

      # Store link between scripts.
      register_host_detail( name:"detected_by", value:res["OID"] );
      register_host_detail( name:"detected_at", value:location );

      return res;
    }
  }
  return NULL;
}

# register_product( cpe:'cpe:/a:foo:bar:1.1.0', location:"/foo", port: 80);
# register_product( cpe:'cpe:/a:foo:bar:1.2.0', location:"/bar", port: 80);
# register_product( cpe:'cpe:/a:foo:bar:1.3.0', location:"/foobar", port: 8080);
#                                                                                                                           ,--[1.1.0]
#                                                 ,--[ 80 ] -> get_app_version( cpe:'cpe:/a:foo:bar', port:80  ) -> fork -> |
#                                                 |                                                                         `--[1.2.0]
# get_app_port( cpe:'cpe:/a:foo:bar' ) -> fork -> |
#                                                 `--[8080] -> get_app_version( cpe:'cpe:/a:foo:bar', port:8080 ) -> [1.3.0]
# Without port:
#
#                                                    ,--[1.1.0]
# get_app_version( cpe:'cpe:/a:foo:bar' ) -> fork -> |
#                                                    |--[1.2.0]
#                                                    |
#                                                    `--[1.3.0]
#
#
#                                                                                                                            ,--[/foo]
#                                                 ,--[ 80 ] -> get_app_location( cpe:'cpe:/a:foo:bar', port:80  ) -> fork -> |
#                                                 |                                                                          `--[/bar]
# get_app_port( cpe:'cpe:/a:foo:bar' ) -> fork -> |
#                                                 `--[8080] -> get_app_location( cpe:'cpe:/a:foo:bar', port:8080 ) -> [/foobar]
#
# Without port:
#                                                     ,--[/foo]
# get_app_location( cpe:'cpe:/a:foo:bar' ) -> fork -> |
#                                                     |--[/bar]
#                                                     |
#                                                     `--[/foobar]
#
#
#                                                                                                                                        ,-> array -> ( [version:1.1.0] [location:/foo] )
#                                                 ,--[ 80 ] -> get_app_version_and_location( cpe:'cpe:/a:foo:bar', port:80  ) -> fork -> |
#                                                 |                                                                                      `-> array -> ( [version:1.2.0] [location:bar] )
# get_app_port( cpe:'cpe:/a:foo:bar' ) -> fork -> |
#                                                 `--[8080] -> get_app_version_and_location( cpe:'cpe:/a:foo:bar', port:8080 ) -> array -> ( [version:1.3.0] [location:/foobar] )
# Without port:
#
#                                                                 ,-> array -> ( [version:1.1.0] [location:/foo] )
# get_app_version_and_location( cpe:'cpe:/a:foo:bar' ) -> fork -> |
#                                                                 |-> array -> ( [version:1.2.0] [location:/bar] )
#                                                                 |
#                                                                 `-> array -> ( [version:1.3.0] [location:/foobar] )
#

# @brief Returns the version of an application registered for the remote host within the KB matching the specified data.
#
# @param cpe           The CPE to get the version for.
# @param port          The port of the application to get the version for, e.g. "80".
# @param service       The service of the application to get the version for, e.g. "www", "mysql" or "local".
# @param nofork        If the function shouldn't fork and only return the first registered version (and not all available).
# @param version_regex A regex to restrict the returned version to. E.g. if a version "1.2" was registered but a
#                      regex like "^[0-9]+\.[0-9]+\.[0-9]+" is specified the function won't return the version.
#
# @return A version of the application matching the specified data, NULL in the case no CPE was passed or no matching info was found.
#
# @note *NEVER* use get_app_version() and get_app_location() together in a single NVT. If both information is required,
#       use get_app_version_and_location() or get_app_full().
#
function get_app_version( cpe, port, service, nofork, version_regex ) {

  local_var cpe, port, service, nofork, version_regex;

  # To report if get_app_version() and get_app_location() is used together in one single NVT
  _get_app_version_used = TRUE;
  if( _get_app_location_used )
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#get_app_version() used together with get_app_location() in one single NVT. Please use get_app_version_and_location instead." );

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_version" );
    return;
  }

  if( typeof( cpe ) == "array" )
    return get_single_app_versions_from_list( cpe_list:cpe, port:port );
  else
    return get_app_details( cpe:cpe, type:"version", port:port, service:service, nofork:nofork, version_regex:version_regex );
}

# @brief Returns the highest app version registered in the KB for the specified CPE.
#
# @param cpe The CPE to use to gather the highest app version for.
#
# @return The highest version of the application matching the specified CPE, NULL in the case no CPE was passed or no matching info was found.
#
# @note If you use this you need to include revisions-lib.inc.
#
function get_highest_app_version( cpe ) {

  local_var cpe, ad, ret_version, version;

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_highest_app_version" );
    return;
  }

  ad = get_app_details( cpe:cpe, type:"all_versions" );
  if( ! ad )
    return;

  ret_version = FALSE;

  foreach version( ad ) {
    if( ! ret_version ) {
      ret_version = version;
      continue;
    }

    # from revisions-lib.inc
    if( revcomp( a:version, b:ret_version ) == 1 )
      ret_version = version;
  }
  return ret_version;
}

# @brief Returns the location of an application registered for the remote host within the KB matching the specified data.
#
# @param cpe       The CPE to get the location for.
# @param port      The port of the application to get the location for, e.g. "80".
# @param nofork    If the function shouldn't fork and only return the first registered location (and not all available).
# @param skip_port If set to TRUE allows to get all locations independent from the port.
# @param default   The default location to return in the case "optimize_test" is set to "no" and no location was found
#                  for the given CPE and/or port.
#
# @return A location of the application matching the specified data, NULL in the case no CPE was passed or no matching info was found.
#         The return value is either a list of locations if nofork is set to TRUE or a string containing the list.
#
# @note *NEVER* use get_app_version() and get_app_location() together in a single NVT. If both information is required,
#       use get_app_version_and_location() or get_app_full().
#
function get_app_location( cpe, port, nofork, skip_port, default ) {

  local_var cpe, port, nofork, skip_port, default;

  # To report if get_app_version() and get_app_location() is used together in one single NVT
  _get_app_location_used = TRUE;
  if( _get_app_version_used )
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#get_app_version() used together with get_app_location() in one single NVT. Please use get_app_version_and_location instead." );

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_location" );
    return;
  }

  if( isnull( port ) && ! nofork && ! skip_port )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#port#-#get_app_location" );

  return get_app_details( cpe:cpe, type:"location", port:port, nofork:nofork, default:default );
}

# @brief Returns all ports (in the form of an array) of a (first found) app from a list of passed CPEs.
#
# @note This function will fork on multiple found ports.
#
# @param cpe_list A list of CPEs to evaluate.
# @param service  A service to limit the returned ports to, e.g. "www" or "mysql".
# @param default  The default port to return in the case "optimize_test" is set to "no" and no port was found
#                 for the given CPE.
#
# @return An array containing the port and CPE matching one of the specified CPEs, NULL in the case no CPE was
#         passed or no matching info was found. The array indices are the following:
#
#         ret_array["port"] = the found port
#         ret_array["cpe"]  = the CPE assigned to this specific port
#
function get_single_app_ports_from_list( cpe_list, service, default ) {

  local_var cpe_list, service, default;
  local_var cpe, base_cpe, port, services, ret_arr;

  if( ! cpe_list ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe_list#-#get_single_app_ports_from_list" );
    return;
  }

  foreach cpe( cpe_list ) {

    if( ! base_cpe = get_base_cpe( cpe:cpe ) )
      continue;

    if( isnull( port = get_kb_item( "get_app/ports/" + base_cpe ) ) )
      continue;

    if( service ) {
      services = get_kb_item( "get_app/services/" + port + "/" + base_cpe ); # this could fork
      if( port =~ '^[0-9]{1,5}$' && services >< service ) {
        ret_arr['cpe']  = cpe;
        ret_arr['port'] = port;
        return ret_arr;
      }
    } else {
      if( port =~ '^[0-9]{1,5}$' ) {
        ret_arr['cpe']  = cpe;
        ret_arr['port'] = port;
        return ret_arr;
      }
    }
  }

  if( default && optimize_test_enabled && "no" >< optimize_test_enabled ) {
    foreach cpe( cpe_list ) {
      ret_arr['cpe']  = cpe;
      ret_arr['port'] = default;
      return ret_arr;
    }
  } else {
    return;
  }
}

# @brief Returns all ports (in the form of an array) of an app from a list of passed CPEs.
#
# @note This function will fork on multiple found applications.
#
# @param cpe_list A list of CPEs to evaluate.
# @param service  A service to limit the returned ports to, e.g. "www" or "mysql".
# @param default  The default port to return in the case "optimize_test" is set to "no" and no port was found
#                 for the given CPE.
#
# @return An array containing the port and CPE matching one of the specified CPEs, NULL in the case no CPE was
#         passed or no matching info was found. The array indices are the following:
#
#         ret_array["port"] = the found port
#         ret_array["cpe"]  = the CPE assigned to this specific port
#
function get_all_app_ports_from_list( cpe_list, service, default ) {

  local_var cpe_list, service, default;
  local_var cpe, base_cpe, _ports, _port, services, _service, infos, _infos, ret_arr;

  if( ! cpe_list ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe_list#-#get_all_app_ports_from_list" );
    return;
  }

  foreach cpe( cpe_list ) {

    if( ! base_cpe = get_base_cpe( cpe:cpe ) )
      continue;

    if( isnull( _ports = get_kb_list( "get_app/ports/" + base_cpe ) ) )
      continue;

    foreach _port( _ports ) {
      if( service ) {
        services = get_kb_list( "get_app/services/" + _port + "/" + base_cpe );
        foreach _service( services ) {
          if( _port =~ '^[0-9]{1,5}$' && _service >< service )
            set_kb_item( name:"get_app/" + get_script_oid() + "/port_cpe", value:cpe + "#-#" + _port );
        }
      } else {
        if( _port =~ '^[0-9]{1,5}$' )
          set_kb_item( name:"get_app/" + get_script_oid() + "/port_cpe", value:cpe + "#-#" + _port );
      }
    }
  }

  infos = get_kb_item( "get_app/" + get_script_oid() + "/port_cpe" );
  if( infos ) {
    _infos = split( infos, sep:'#-#', keep:FALSE );
    ret_arr['cpe']  = _infos[0];
    ret_arr['port'] = _infos[1];
    return ret_arr;
  }

  if( default && optimize_test_enabled && "no" >< optimize_test_enabled ) {
    foreach cpe( cpe_list ) {
      ret_arr['cpe']  = cpe;
      ret_arr['port'] = default;
      return ret_arr;
    }
  } else {
    return;
  }
}

# @brief Returns all versions of a (first found) app from a list of passed CPEs.
#
# @note This function will fork on multiple found versions.
#
# @param cpe_list A list of CPEs to evaluate.
# @param port     The port of the application to get the version for, e.g. "80".
#
# @return A version matching one of the specified CPEs, NULL in the case no CPE was passed or no matching info was found.
#
function get_single_app_versions_from_list( cpe_list, port ) {

  local_var cpe_list, port, cpe, ap;

  if( ! cpe_list ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe_list#-#get_single_app_versions_from_list" );
    return;
  }

  foreach cpe( cpe_list ) {
    ap = get_app_details( cpe:cpe, type:"version", port:port );
    if( ap )
      return ap;
  }
  return;
}

# @brief Returns port of an app matching the specified data.
#
# @param cpe     Either a CPE passed as a string or an list of CPEs to evaluate.
# @param service A service to limit the returned ports to, e.g. "www" or "mysql".
# @param all     If a list of CPEs is passed via the cpe parameter this allows to specify if only
#                the first found port (all empty or FALSE) or all found ports (all = TRUE) is returned.
# @param default The default port to return in the case "optimize_test" is set to "no" and no port was found
#                for the given CPE.
#
# @return A port matching one of the specified CPEs and data, NULL in the case no CPE was passed or no matching info was found.
#
function get_app_port( cpe, service, all, default ) {

  local_var cpe, service, all, default;
  local_var infos, base_cpe, port, services;

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_port" );
    return;
  }

  # TODO: Evaluate all VTs passing a list and migrate them to call the functions
  # directly. Afterwards remove this part as the function shouldn't take different
  # inputs (strin and array) in the cpe parameter at all.
  if( typeof( cpe ) == "array" ) {
    if( all ) {
      infos = get_all_app_ports_from_list( cpe_list:cpe, service:service, default:default );
      return infos["port"];
    } else {
      infos = get_single_app_ports_from_list( cpe_list:cpe, service:service, default:default );
      return infos["port"];
    }
  } else {
    if( all )
      set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#get_app_port(): all parameter set to TRUE but no array within the cpe parameter." );
  }

  if( ! base_cpe = get_base_cpe( cpe:cpe ) ) {
    set_kb_item( name:"nvt_debug_cpe_syntax/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_port" );
    return;
  }

  port = get_kb_item( "get_app/ports/" + base_cpe ); # this could fork
  if( port == "local" )
    port = 0; # nb: See function register_product

  if( service ) {
    services = get_kb_item( "get_app/services/" + port + "/" + base_cpe ); # nb: this could fork
    if( port =~ '^[0-9]{1,5}$' && services >< service )
      return port;
  } else {
    if( port =~ '^[0-9]{1,5}$' )
      return port;
  }

  if( default && optimize_test_enabled && "no" >< optimize_test_enabled )
    return default;
  else
    return;
}

# @brief Returns the version and location of an application registered for the remote host within the KB matching the specified data.
#
# @param cpe              The CPE to get the version and location for.
# @param port             The port of the application to get the version and location for, e.g. "80".
# @param exit_no_version  If the function should exit if no version was found. Note: The function still return if a version but no location was found.
# @param version_regex    A regex to restrict the returned version to. E.g. if a version "1.2" was registered but a
#                         regex like "^[0-9]+\.[0-9]+\.[0-9]+" is specified the function won't return the version.
# @param default_location The default location to return in the case "optimize_test" is set to "no" and no location was found for the given CPE.
#
# @return An array containing the info about the application matching the specified data in a form of the following array keys:
#
#         ['version']
#         ['location']
#
#         or NULL in the case no CPE was passed or no matching info was found.
#
# @note get_app_version_and_location() should be used instead of get_app_version() and get_app_location() together in a single NVT if both information is required.
#
function get_app_version_and_location( cpe, port, exit_no_version, version_regex, default_location ) {

  local_var cpe, port, exit_no_version, version_regex, default_location;
  local_var app_details;

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_version_and_location" );
    return;
  }

  app_details = get_app_details( cpe:cpe, type:"version_location", port:port, exit_no_version:exit_no_version, version_regex:version_regex, default_location:default_location );
  # nb: This is used here as get_app_details() might return earlier with NULL without even
  # reaching the exit_no_version check in the "version_location" part.
  if( isnull( app_details ) && exit_no_version )
    exit( 0 );

  # nb: This is for the "optimize_test" = "no" case where app_details above might contain an empty version index
  if( app_details && ! app_details["version"] && exit_no_version )
    exit( 0 );

  return app_details;
}

# @brief Returns the location and protocol of an application registered for the remote host within the KB matching the specified data.
#
# @param cpe              The CPE to get the location and protocol for.
# @param port             The port of the application to get the location and protocol for, e.g. "80".
# @param default_location The default location to return in the case "optimize_test" is set to "no" and no location was found for the given CPE.
# @param default_proto    The default protocol to return in the case "optimize_test" is set to "no" and no protocol was found for the given CPE.
#
# @return An array containing the info about the application matching the specified data in a form of the following array keys:
#
#         ['location']
#         ['proto']
#
#         or NULL in the case no CPE was passed or no matching info was found.
#
function get_app_location_and_proto( cpe, port, default_location, default_proto ) {

  local_var cpe, port, default_location, default_proto;

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_location_and_proto" );
    return;
  }

  return get_app_details( cpe:cpe, type:"location_proto", port:port, default_location:default_location, default_proto:default_proto );
}

# @brief Returns the version and protocol of an application registered for the remote host within the KB matching the specified data.
#
# @param cpe             The CPE to get the version and protocol for.
# @param port            The port of the application to get the version and protocol for, e.g. "80".
# @param exit_no_version If the function should exit if no version was found. Note: The function still return if a version but no protocol was found.
# @param default_proto   The default protocol to return in the case "optimize_test" is set to "no" and no protocol was found for the given CPE.
#
# @return An array containing the info about the application matching the specified data in a form of the following array keys:
#
#         ['version']
#         ['proto']
#
#         or NULL in the case no CPE was passed or no matching info was found.
#
function get_app_version_and_proto( cpe, port, exit_no_version, default_proto ) {

  local_var cpe, port, exit_no_version, default_proto;
  local_var app_details;

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_version_and_proto" );
    return;
  }

  app_details = get_app_details( cpe:cpe, type:"version_proto", port:port, exit_no_version:exit_no_version, default_proto:default_proto );
  # nb: This is used here as get_app_details() might return earlier with NULL without even
  # reaching the exit_no_version check in the "version_proto" part.
  if( isnull( app_details ) && exit_no_version )
    exit( 0 );

  # nb: This is for the "optimize_test" = "no" case where app_details above might contain an empty version index
  if( app_details && ! app_details["version"] && exit_no_version )
    exit( 0 );

  return app_details;
}

# @brief Returns all known information like location, port, version, protocol and service for the remote host within the KB matching the specified data.
#
# @param cpe             The CPE to get the information for.
# @param port            The port of the application to get the information for, e.g. "80".
#
# @return An array containing the info about the application matching the specified data in a form of the following array keys:
#
#         ['version']
#         ['location']
#         ['full_cpe']
#         ['port']
#         ['proto']
#         ['service']
#         ['detected_by']
#
#         or NULL in the case no CPE was passed or no matching info was found.
#
function get_app_full( cpe, port ) {

  local_var cpe, port;

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_full" );
    return;
  }

  return get_app_details( cpe:cpe, type:"full", port:port );
}

# @brief Returns the port from a CPE prefix / vendor part. For example "cpe:/h:qnap:ts-x53" is the full CPE
#        and the function is accepting e.g. "cpe:/h:qnap" to get the port for all vendor devices.
#
# @note This function might fork on multiple found ports.
#
# @param cpe            The CPE prefix / vendor part (e.g. cpe:/h:qnap) to get the port for.
# @param default        The default port to return in the case "optimize_test" is set to "no" and no port was found
#                       for the given CPE prefix / vendor part.
# @param service        A service to limit the returned ports to, e.g. "www" or "mysql".
# @param first_cpe_only If set to TRUE only the ports of the first found CPE are returned. Note that the function is
#                       still forking if multiple ports are found for the same CPE.
#
# @return An array containing the port and CPE matching the specified CPE prefix / vendor part, NULL in the case
#         no CPE prefix / vendor part was passed or no matching info was found. The array indices are the following:
#
#         ret_array["port"] = the found port
#         ret_array["cpe"]  = the CPE assigned to this specific port
#
function get_app_port_from_cpe_prefix( cpe, default, service, first_cpe_only ) {

  local_var cpe, default, service, first_cpe_only;
  local_var data, _cpelist, entry, parts, _cpe;

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_port_from_cpe_prefix" );
    return;
  }

  data = get_kb_list( "get_app/ports/" + cpe + '*' );
  if( ! data )
    return;

  _cpelist = make_list();

  foreach entry( keys( data ) ) {

    if( cpe >!< entry )
      continue;

    parts = split( entry, sep:"/", keep:FALSE );

    if( max_index( parts ) < 4 )
      continue;

    if( parts[2] != 'cpe:' )
      continue;

    _cpe = parts[2] + '/' + parts[3];

    if( cpe >!< _cpe )
      continue;

    _cpelist = make_list( _cpelist, _cpe );
  }

  if( first_cpe_only )
    return get_single_app_ports_from_list( cpe_list:_cpelist, service:service, default:default );
  else
    return get_all_app_ports_from_list( cpe_list:_cpelist, service:service, default:default );
}

# @brief Internal function not to be used directly, please use the other wrappers
#        within this include calling this function.
#
function get_app_details( cpe, type, port, exit_no_version, service, nofork, version_regex, default, default_location, default_proto ) {

  local_var cpe, type, port, exit_no_version, service, nofork, version_regex, default, default_location, default_proto;
  local_var base_cpe, version_list, ap, dt, t, t_cpe, app_versions, app_locations;
  local_var t_oid, t_loc, app_details, x, x_oid, x_cpe, x_loc, x_port, x_proto, x_service, ret_arr;

  if( ! base_cpe = get_base_cpe( cpe:cpe ) ) {
    set_kb_item( name:"nvt_debug_cpe_syntax/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_app_details" );
    return;
  }

  # "1.3.6.1.4.1.25623.1.0.xxxxx#-#cpe:/a:foo:bar:1.1.0#-#/foo#-#80" or
  # "1.3.6.1.4.1.25623.1.0.xxxxx#-#cpe:/a:foo:bar:1.1.0#-#/foo#-#80#-#www"
  if( type == "all_versions" ) {

    version_list = make_list();

    ap = get_kb_list( "get_app/" + base_cpe );
    if( ! ap )
      return;

    foreach dt( ap ) {

      t = split( dt, sep:'#-#', keep:FALSE );
      t_cpe = t[1];

      if( ! t_cpe )
        continue;

      if( ! app_versions = get_version_from_cpe( cpe:t_cpe ) )
        continue;

      version_list = make_list( version_list, app_versions );
    }

    register_host_detail( name:"detected_by", value:t[0] );
    if( ! isnull( t[2] ) )
      register_host_detail( name:"detected_at", value:t[2] );

    return version_list;

  } else if( nofork && type == "version" ) { # nofork is currently only valid for get_app_version() and get_app_location

    ap = get_kb_list( "get_app/" + base_cpe );
    if( ! ap )
      return;

    foreach dt( ap ) {

      t = split( dt, sep:'#-#', keep:FALSE );
      t_cpe = t[1];

      if( ! t_cpe )
        continue;

      if( ! app_versions = get_version_from_cpe( cpe:t_cpe ) )
        continue;

      if( version_regex && ! ereg( pattern:version_regex, string:app_versions ) )
        continue;

      register_host_detail( name:"detected_by", value:t[0] );
      if( ! isnull( t[2] ) )
        register_host_detail( name:"detected_at", value:t[2] );

      return app_versions;
    }
    return; # no version...
  } else if( nofork && type == "location" ) { # nofork is currently only valid for get_app_version() and get_app_location

    ap = get_kb_list( "get_app/" + base_cpe );
    if( ! ap && default && optimize_test_enabled && "no" >< optimize_test_enabled )
      return make_list( default );
    else if( ! ap )
      return;

    app_locations = make_list();

    foreach dt( ap ) {

      t = split( dt, sep:'#-#', keep:FALSE );
      t_oid = t[0];
      t_loc = t[2];
      if( isnull( t_oid ) && isnull( t_loc ) )
        continue;

      register_host_detail( name:"detected_by", value:t_oid );
      if( ! isnull( t[2] ) )
        register_host_detail( name:"detected_at", value:t_loc );

      app_locations = make_list( app_locations, t_loc );
    }
    return app_locations;
  } else {
    app_details = get_kb_item( "get_app/" + base_cpe ); # this could fork
  }

  # nb: This is kept here as we don't get anny "app_details" from above if
  # no app was detected priviously and the "default" is used.
  if( ! app_details && optimize_test_enabled && "no" >< optimize_test_enabled ) {

    if( type == "location" && default ) {
      return default;
    } else if( type == "version_location" && default_location ) {
      ret_arr['version']  = '';
      ret_arr['location'] = default_location;
      return ret_arr;
    } else if( type == "version_proto" && default_proto ) {
      ret_arr['version'] = '';
      ret_arr['proto']   = default_proto;
      return ret_arr;
    } else if( type == "location_proto" && ( default_proto || default_location ) ) {
      if( default_proto )
        ret_arr['proto'] = default_proto;
      else
        ret_arr['proto'] = 'tcp';

      if( default_location )
        ret_arr['location'] = default_location;
      else
        ret_arr['location'] = '';

      return ret_arr;
    } else {
      return;
    }
  } else if( ! app_details ) {
    return;
  }

  x = split( app_details, sep:'#-#', keep:FALSE );

  x_oid       = x[0];
  x_cpe       = x[1];
  x_loc       = x[2];
  x_port      = x[3];
  x_proto     = x[4];
  if( x[5] )
    x_service = x[5];

  if( service && x_service != service )
    return;

  if( ! isnull( port ) && int( x_port ) != int( port ) )
    return;

  if( type == "version" ) {

    if( ! app_versions = get_version_from_cpe( cpe:x_cpe ) )
      return;

    if( version_regex && ! ereg( pattern:version_regex, string:app_versions ) )
      return;

    register_host_detail( name:"detected_by", value:x_oid );
    if( x_loc )
      register_host_detail( name:"detected_at", value:x_loc );

    return app_versions;
  }

  if( type == "location" ) {

    if( x_loc ) {
      register_host_detail( name:"detected_by", value:x_oid );
      if( x_loc )
        register_host_detail( name:"detected_at", value:x_loc );

      return x_loc;
    }
    return;
  }

  if( type == "version_location" ) {

    app_versions = get_version_from_cpe( cpe:x_cpe );

    if( ! app_versions && exit_no_version == TRUE )
      exit( 0 );

    if( ! app_versions && ! x_loc )
      return;

    if( app_versions ) {

      if( version_regex && ! ereg( pattern:version_regex, string:app_versions ) )
        return;

      ret_arr['version'] = app_versions;
    } else {
      ret_arr['version'] = '';
    }

    if( x_loc )
      ret_arr['location'] = x_loc;
    else
      ret_arr['location'] = '';

    register_host_detail( name:"detected_by", value:x_oid );
    if( x_loc )
      register_host_detail( name:"detected_at", value:x_loc );

    return ret_arr;
  }

  if( type == "version_proto" ) {

    app_versions = get_version_from_cpe( cpe:x_cpe );

    if( ! app_versions && exit_no_version == TRUE )
      exit( 0 );

    if( ! app_versions && ! x_proto )
      return;

    if( app_versions )
      ret_arr['version'] = app_versions;
    else
      ret_arr['version'] = '';

    if( x_proto )
      ret_arr['proto'] = x_proto;
    else
      ret_arr['proto'] = 'tcp';

    register_host_detail( name:"detected_by", value:x_oid );
    if( x_proto && x_port )
      register_host_detail( name:"detected_at", value:x_port + "/" + x_proto );

    return ret_arr;
  }

  if( type == "location_proto" ) {

    if( x_proto )
      ret_arr['proto'] = x_proto;
    else
      ret_arr['proto'] = 'tcp';

    if( x_loc )
      ret_arr['location'] = x_loc;
    else
      ret_arr['location'] = '';

    register_host_detail( name:"detected_by", value:x_oid );
    if( x_loc )
      register_host_detail( name:"detected_at", value:x_loc );

    return ret_arr;
  }

  if( type == "full" ) {

    app_versions = get_version_from_cpe( cpe:x_cpe );

    ret_arr['version']     = app_versions;
    ret_arr['location']    = x_loc;
    ret_arr['full_cpe']    = x_cpe;
    ret_arr['port']        = x_port;
    ret_arr['proto']       = x_proto;
    if( x_service )
      ret_arr['service']   = x_service;
    ret_arr['detected_by'] = x_oid;

    register_host_detail( name:"detected_by", value:x_oid );
    if( x_loc )
      register_host_detail( name:"detected_at", value:x_loc );

    return ret_arr;
  }
  return;
}

# @brief Returns the version from a specific CPE.
#
# @param cpe The CPE to get the version from, e.g.:
#
#            cpe:/a:vendor:product:1.2.3 -> 1.2.3 returned
#            cpe:/a:vendor:product:1.2.3:p1 -> 1.2.3p1 returned
#
# @return A string containing a version of the specific CPE, or NULL in the case no CPE was passed or no matching info was found.
#
# @note The function currently supports an (unfortunately wrong) used CPE syntax like cpe:/a:vendor:product:x64:1.2.3
#       and is returning the version from it as well.
#
function get_version_from_cpe( cpe ) {

  local_var cpe, tokens, token_val, app_versions;

  if( isnull ( cpe ) ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#get_version_from_cpe" );
    return;
  }

  tokens = split( cpe, sep:":", keep:FALSE );

  # Possible CPEs are e.g.
  # cpe:/a:extplorer:extplorer:2.1.0:RC5
  # cpe:/o:canonical:ubuntu_linux:12.04:-:lts
  # cpe:/o:microsoft:windows_7:-:-:x64
  # nb: Some are also setting a CPE with a wrong syntax like cpe:/a:vendor:product:x64:1.1
  # which should be cpe:/a:vendor:product:1.1:-:x64

  # Test-cases:
  # CPE: cpe:/a:foo0:bar0:x64:1.2.3 , expected version: 1.2.3, returned version: 1.2.3
  # CPE: cpe:/a:foo1:bar1:x64:1.2.3:RC1 , expected version: 1.2.3RC1 , returned version: 1.2.3RC1
  # CPE: cpe:/a:foo2:bar2:1.2.3 , expected version: 1.2.3, returned version: 1.2.3
  # CPE: cpe:/a:foo3:bar3:1.2.3:RC2 , expected version: 1.2.3RC2, returned version: 1.2.3RC2
  # CPE: cpe:/a:foo4:bar4:1.2.3:RC3:x64 , expected version: 1.2.3RC3, returned version: 1.2.3RC3
  # CPE: cpe:/a:foo5:bar5:1.2.3:RC4:x32 , expected version: 1.2.3RC4, returned version: 1.2.3RC4
  # CPE: cpe:/a:foo6:bar6:1.2.3:-:x64 , expected version: 1.2.3, returned version: 1.2.3
  # CPE: cpe:/a:foo7:bar7:1.2.3:-:x32 , expected version: 1.2.3, returned version: 1.2.3
  # CPE: cpe:/a:foo8:bar8:1.2.3::x64 , expected version: 1.2.3, returned version: 1.2.3
  # CPE: cpe:/a:foo9:bar9:1.2.3::x32 , expected version: 1.2.3, returned version: 1.2.3
  # CPE: cpe:/a:foo10:bar10:-:-:x64 , expected version: NULL, returned version:
  # CPE: cpe:/a:foo11:bar11:-:-:x32 , expected version: NULL, returned version:
  # CPE: cpe:/a:foo12:bar12:::x64 , expected version: NULL, returned version:
  # CPE: cpe:/a:foo13:bar13:::x32 , expected version: NULL, returned version:
  # CPE: cpe:/a:foo14:bar14::sp2:x64 , expected version: sp2, returned version: sp2
  # CPE: cpe:/a:foo15:bar15::sp2:x32 , expected version: sp2, returned version: sp2
  # CPE: cpe:/a:foo16:bar16:-:sp2:x64 , expected version: sp2, returned version: sp2
  # CPE: cpe:/a:foo17:bar17:-:sp2:x32 , expected version: sp2, returned version: sp2

  if( ! isnull( tokens[4] ) ) {

    # catch the wrong CPE syntax cpe:/a:foo0:bar0:x64:1.2.3 described above
    if( tokens[4] == "x64" )
      token_val = 1;
    else
      token_val = 0;

    # handle cpe:/a:foo3:bar3:1.2.3:RC2
    if( ! isnull( tokens[5 + token_val] ) && tokens[5 + token_val] != "-" && tokens[5 + token_val] != "" ) {

      if( tokens[4 + token_val] != "-" && tokens[4 + token_val] != "" )
        app_versions = tokens[4 + token_val] + "" + tokens[5 + token_val]; # handle cpe:/a:foo3:bar3:1.2.3:RC2
      else
        app_versions = tokens[5 + token_val]; # handle cpe:/a:foo10:bar10:-:sp2

    } else {
      # handle cpe:/a:foo2:bar2:1.2.3
      if( tokens[4 + token_val] != "-" && tokens[4 + token_val] != "" )
        app_versions = tokens[4 + token_val];
    }
  }

  if( ! app_versions )
    return;
  else
    return app_versions;
}

# @brief Returns the "base" CPE of a passed CPE without any version version and similar, e.g.
#
#        cpe:/a:vendor:product:1.2.3 -> cpe:/a:vendor:product returned
#        cpe:/a:vendor:product:1.2.3:p1 -> cpe:/a:vendor:product returned
#
# @return A string containing a base of a specific CPE, or NULL in the case no CPE was passed or the CPE has a wrong syntax.
#
function get_base_cpe( cpe ) {

  local_var cpe, bc, base_cpe;

  if( ! cpe )
    return;

  bc = split( cpe, sep:':', keep:FALSE );
  if( ! bc || max_index( bc ) < 4 )
    return;

  if( bc[0] != 'cpe' || bc[1] !~ '/(a|o|h)$' )
    return;

  base_cpe = bc[0] + ':' + bc[1] + ':' + bc[2] + ':' + bc[3];

  return base_cpe;
}

# @brief Builds a detection report for an application with the passed data to be used by VTs in e.g. a log_message().
#
# @param app          The name of the application, e.g. "Vendor Productname".
# @param version      The version of the application.
# @param install      The install location of the application, e.g. 80/tcp or /path/to/folder.
# @param regpath      The path to a windows registry entry of the application.
# @param cpe          The CPE of the application
# @param concluded    The info including where the version was detected from.
# @param concludedUrl The URL where the version was detected from.
# @param extra        Any additional information which should be appended to the report.
# @param skip_version If no version was passed to the "version" parameter an "unknown" version is assumed.
#                     Setting this parameter to "TRUE" won't add any version information at all.
#
# @return A string containing a detection report for the passed data.

function build_detection_report( app, version, install, regpath, cpe, concluded, concludedUrl, extra, skip_version ) {

  local_var app, version, install, regpath, cpe, concluded, concludedUrl, extra, skip_version, report;

  if( isnull( app ) ) set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#app#-#build_detection_report" );

  # report = 'Detected ' + app + '\n';

  if( ! skip_version ) {
    if( ! version )
      version = 'unknown';
   # report += 'Version:  ' + version + '\n';
  }

 # report += 'Location: ' + install + '\n';

  if( strlen( cpe ) > 0 )
   # report += 'CPE:      '+ cpe + '\n';

  if( strlen( regpath ) > 0 )
   # report += 'Registry path: '+ regpath + '\n';

  if( strlen( concluded ) > 0 && tolower( concluded ) != "unknown" )
   # report += 'Concluded from version/product identification result:' + concluded + '\n';

  if( strlen( concludedUrl ) > 0 )
   # report += 'Concluded from version/product identification location:' + concludedUrl + '\n';

  if( strlen( extra ) > 0 )
   # report += 'Extra information:' + extra + '\n';

  return report;
}

# @brief Registers and reports an detected operating system of the remote host.
#
# @param os          The name of the OS to register.
# @param version     The version of the OS to register.
# @param cpe         The CPE of the OS to register
# @param banner_type The type of the banner where the OS was detected from, e.g "HTTP Banner".
# @param port        The port of the OS to register.
# @param proto       The protocol of the OS to register. Defaults to "tcp" if not passed.
# @param banner      The banner of the OS where the OS was detected from.
# @param desc        The description of the VT (e.g. script_name()) registering the OS
# @param runs_key    The "runs_key" which should be used to register the OS as, currently supported: "windows" or "unixoide".
#
# @return NULL
#
function register_and_report_os( os, version, cpe, banner_type, port, proto, banner, desc, runs_key ) {

  local_var os, version, cpe, banner_type, port, proto, banner, desc, runs_key;
  local_var oid, report;

  oid = get_script_oid();
  if( ! proto )
    proto = "tcp";

  if( ! desc )
    set_kb_item( name:"nvt_debug_empty/" + oid, value:oid + "#-#desc#-#register_and_report_os" );

  if( version ) {
    if( cpe ) cpe += ":" + version;
    if( os ) os += " " + version;
  }

  if( cpe )
    register_host_detail( name:"OS", value:cpe, desc:desc );
  else
    cpe = "N/A";

  if( os )
    register_host_detail( name:"OS", value:os, desc:desc );
  else
    os = "N/A";

  report = os;

  if( version )
   # report += 'Version: ' + version + '\n';

  #report += 'CPE: ' + cpe + '\n';
  #report += 'Found by KB: ' + oid + ' (' + desc + ')' + '\n';

  if( banner_type ) {

   # report += 'Concluded from ' + banner_type + '\n';

    if( port && proto != "icmp" )
      report += " on port " + port + "/" + proto;

    if( banner ) {
      banner = chomp( banner ); # Few banners have newlines at the end so removing these here
      report += ': ' + banner;
    }
  }

  # Keep down here as we only need this defaults for the set_kb_item
  if( ! port )
    port = "0";

  set_kb_item( name:"os_detection_report/reports/" + oid + "/" + port + "/" + proto, value:report );

  # For setting the "Host/runs_windows" or "Host/runs_unixoide" key in os_detection.nasl
  if( ! isnull( runs_key ) ) {
    set_kb_item( name:"os_detection_report/host_runs/" + oid + "/" + port + "/" + proto, value:runs_key );
  } else {
    set_kb_item( name:"os_detection_report/host_runs/" + oid + "/" + port + "/" + proto, value:"unknown" );
  }
}

# @brief Registers an unknown banner in the KB which could contain info to detect an operating
#        system for later reporting within gb_unknown_os_service_reporting.nasl
#
# @param banner            the banner to register. Required.
# @param banner_type_name  short description of the banner (e.g. HTTP banner) which is included
#                          in the later reporting. Required.
# @param banner_type_short short keyword (e.g. http_banner) which is used internally as a keyword
#                          for the KB key where the information is saved at. Required.
# @param port              port (optional) where the banner was identified at, defaults to "0" if empty.
# @param proto             protocol (optional, tcp or udp) where the banner was idenfitied at, defaults
#                          to "tcp" if empty.
#
# @return NULL
#
function register_unknown_os_banner( banner, banner_type_name, banner_type_short, port, proto ) {

  local_var banner, banner_type_name, banner_type_short, port, proto;
  local_var oid;

  oid = get_script_oid();

  if( ! banner ) {
    set_kb_item( name:"nvt_debug_empty/" + oid, value:oid + "#-#banner#-#register_unknown_os_banner" );
    return;
  }

  if( ! banner_type_name ) {
    set_kb_item( name:"nvt_debug_empty/" + oid, value:oid + "#-#banner_type_name#-#register_unknown_os_banner" );
    return;
  }

  if( ! banner_type_short ) {
    set_kb_item( name:"nvt_debug_empty/" + oid, value:oid + "#-#banner_type_short#-#register_unknown_os_banner" );
    return;
  }

  banner = chomp( banner ); # Few banners have newlines at the end so removing these here

  # Don't report known banner were we know that it doesn't contain info about an OS here
  if( banner == "login:" ||
      egrep( pattern:"^MikroTik ", string:banner ) || # Already covered by gb_mikrotik_router_routeros_consolidation.nasl
      egrep( pattern:"^[0-9.]+$", string:banner ) ) { # Bind9 and MySQL. Both are often only sending e.g. 9.4.2
    return;
  }

  if( ! proto )
    proto = "tcp";

  if( ! port )
    port = "0";

  set_kb_item( name:"os_detection_report/unknown_os_banner/" + oid + "/" + port + "/" + proto + "/" + banner_type_short + "/banner", value:banner );
  set_kb_item( name:"os_detection_report/unknown_os_banner/" + oid + "/" + port + "/" + proto + "/" + banner_type_short + "/type_full", value:banner_type_name );
  set_kb_item( name:"unknown_os_or_service/available", value:TRUE ); # Used in gb_unknown_os_service_reporting.nasl
}
