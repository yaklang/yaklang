###############################################################################
# OpenVAS Vulnerability Test
# $Id: nmap.inc 4487 2016-11-14 07:41:39Z cfi $
#
# NSE registration library for the nmap_net system
#
# Authors:
# Henri Doreau <henri.doreau@greenbone.net>
#
# Copyright:
# Copyright (C) 2011 Greenbone Networks GmbH, http://www.greenbone.net
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
#
# $Revision: 4487 $
#
###############################################################################


function nmap_nse_register(script, args, ports) {
    set_kb_item(name:"NmapNSE/scripts", value:script);
    set_kb_item(name:"NmapNSE/registered/" + script, value:"1");
    if (!isnull(args)) {
        foreach argname (keys(args)) {
            set_kb_item(name:"NmapNSE/arguments", value:argname + "=" + args[argname]);
        }
    }
    if (!isnull(ports)) {
        set_kb_item(name:"NmapNSE/ports", value:ports);
    }
}

function nmap_nse_get_results(script) {
    res = make_array();

    # Gather portscript results
    ports = get_kb_list("NmapNSE/results/" + script);
    if (!isnull(ports)) {
        foreach port (ports) {
            output = get_kb_item("NmapNSE/results/" + script + "/" + port);
            if (!isnull(output)) {
                res[port] = output;
            }
        }
    }

    # Gather hostscript results
    output = get_kb_item("NmapNSE/results/hostscripts/" + script);
    if (!isnull(output)) {
        # port 0 is used as "general/tcp"
        res[0] = output;
    }

    return res;
}

