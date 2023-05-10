###############################################################################
#
# Functions and Variables for SAP
# $Id: sap.inc 10011 2018-05-30 01:12:59Z ckuersteiner $
#
# Authors:
# Christian Kuersteiner <christian.kuersteiner@greenbone.net>
#
# Copyright:
# Copyright (c) 2018 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation; either version 2 of the License, or
# (at your option) any later version.
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
# $Revision: 10011 $
#
###############################################################################

# Default SAP credentials
# Mainly from metasploits sap_default.txt
sap_default_credentials = make_list("SAP*/06071992",
                                    "SAP*/PASS",
                                    "DDIC/19920706",
                                    "DDIC/Welcome01",
                                    "SAPCPIC/ADMIN",
                                    "EARLYWATCH/SUPPORT",
                                    "TMSADM/PASSWORD",
                                    "TMSADM/ADMIN",
                                    "TMSADM/$1Pawd2&",
                                    "ADMIN/welcome",
                                    "ADSUSER/ch4ngeme",
                                    "ADS_AGENT/ch4ngeme",
                                    "DEVELOPER/ch4ngeme",
                                    "J2EE_ADMIN/ch4ngeme",
                                    "SAPJSF/ch4ngeme",
                                    "SAPR3/SAP",
                                    "CTB_ADMIN/sap123",
                                    "XMI_DEMO/sap123",
                                    "IDEADM/admin",
                                    "SMD_ADMIN/init1234",
                                    "SMD_BI_RFC/init1234",
                                    "SMD_RFC/init1234",
                                    "SOLMAN_ADMIN/init1234",
                                    "SOLMAN_BTC/init1234",
                                    "SAPSUPPORT/init1234",
                                    "CONTENTSERV/init1234",
                                    "SMD_AGT/init1234");

# Default Clients
sap_default_clients = make_list("000", "001", "066");
