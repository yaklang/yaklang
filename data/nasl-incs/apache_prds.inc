# Copyright (C) 2022 Greenbone Networks GmbH
# Some text descriptions might be excerpted from (a) referenced
# source(s), and are Copyright (C) by the respective right holder(s).
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

# @brief Returns a list of known Apache Commons components.
#
# @return A list of known Apache Commons components.
#
function apache_commons_comp_list() {

  local_var comp_list;

  comp_list = make_list(

    # nb: The list should be always sorted in a way that e.g. "collections4" is included before
    # "collections" or "commons-geometry-examples-io" before "commons-geometry-examples". Otherwise
    # the detection VTs using this in e.g. a regex would extract the wrong component name as the
    # less specific one.

    # A list can be found on the index page on https://commons.apache.org/ or in more detail at:
    # https://search.maven.org/search?q=g:org.apache.commons
    #
    # nb:
    # - For a few like "commons-collections4" the previous versions like "commons-collections3" and
    #   the more generic "commons-collections" (seems at least on Debian a few exists without that
    #   numbering) have been added manually
    # - The detection is dropping e.g. the "4" completely as the NVD is only using the CPE without
    #   these (which makes sense because the "branch" is reflectedin the version). If this needs to
    #   be kept for some specific components these needs to be handled separately in the detection
    #   like done for "rdf-rdf4j" (make sure to check this when adding new strings below!!!)
    "build-plugin",
    "cli",
    "collections4",
    "collections3",
    "collections2",
    "collections1",
    "collections",
    "compress",
    "configuration2",
    "configuration1",
    "configuration",
    "crypto",
    "csv",
    "daemon",
    "dbcp2",
    "dbcp1",
    "dbcp",
    "digester3",
    "digester2",
    "digester1",
    "digester",
    "email",
    "exec",
    "geometry-core",
    "geometry-enclosing",
    "geometry-euclidean",
    "geometry-examples-io",
    "geometry-examples-jmh",
    "geometry-examples-tutorials",
    "geometry-examples",
    "geometry-hull",
    "geometry-io-core",
    "geometry-io-euclidean",
    "geometry-parent",
    "geometry-spherical",
    "imaging",
    "io",
    "jci-core",
    "jci-eclipse",
    "jci-examples",
    "jci-fam",
    "jci-groovy",
    "jci-janino",
    "jci-javac",
    "jci-rhino",
    "jci",
    "jcs3-core",
    "jcs3-jcache",
    "jcs3-jcache-extras",
    "jcs3-jcache-openjpa",
    "jcs3-jcache-tck",
    "jcs3",
    "jcs2-core",
    "jcs2-jcache",
    "jcs2-jcache-extras",
    "jcs2-jcache-openjpa",
    "jcs2-jcache-tck",
    "jcs2",
    "jcs1-core",
    "jcs1-jcache",
    "jcs1-jcache-extras",
    "jcs1-jcache-openjpa",
    "jcs1-jcache-tck",
    "jcs1",
    "jcs-core",
    "jcs-dist",
    "jcs-jcache",
    "jcs-jcache-extras",
    "jcs-jcache-openjpa",
    "jcs-jcache-tck",
    "jcs",
    "jelly",
    "jexl3",
    "jexl2",
    "jexl1",
    "jexl",
    "lang3",
    "lang2",
    "lang1",
    "lang",
    "logging-adapters",
    "logging-api",
    "logging",
    "math3",
    "math2",
    "math1",
    "math",
    "numbers-angle",
    "numbers-arrays",
    "numbers-bom",
    "numbers-combinatorics",
    "numbers-complex",
    "numbers-core",
    "numbers-examples-jmh",
    "numbers-examples",
    "numbers-field",
    "numbers-fraction",
    "numbers-gamma",
    "numbers-parent",
    "numbers-primes",
    "numbers-quaternion",
    "numbers-rootfinder",
    "parent",
    "pool2",
    "pool1",
    "pool",
    "proxy",
    "rdf-api",
    "rdf-integration-tests",
    "rdf-jena",
    "rdf-jsonld-java",
    "rdf-parent",
    "rdf-rdf4j",
    "rdf-simple",
    "release-plugin",
    "rng-bom",
    "rng-client-api",
    "rng-core",
    "rng-examples-jmh",
    "rng-examples-jpms-app",
    "rng-examples-jpms-lib",
    "rng-examples-jpms",
    "rng-examples-quadrature",
    "rng-examples-sampling",
    "rng-examples-stress",
    "rng-examples",
    "rng-jmh",
    "rng-parent",
    "rng-sampling",
    "rng-simple",
    "sandbox-parent",
    "skin",
    "text",
    "vfs2-distribution",
    "vfs2-examples",
    "vfs2-jackrabbit1",
    "vfs2-jackrabbit2",
    "vfs2-jackrabbit",
    "vfs2-project",
    "vfs2",
    "vfs1-distribution",
    "vfs1-examples",
    "vfs1-jackrabbit1",
    "vfs1-jackrabbit2",
    "vfs1-jackrabbit",
    "vfs1-project",
    "vfs1",
    "vfs-distribution",
    "vfs-examples",
    "vfs-jackrabbit1",
    "vfs-jackrabbit2",
    "vfs-jackrabbit",
    "vfs-project",
    "vfs",
    "weaver-antlib-parent",
    "weaver-antlib-test",
    "weaver-antlib",
    "weaver-base",
    "weaver-build-tools",
    "weaver-example",
    "weaver-maven-plugin",
    "weaver-modules-parent",
    "weaver-normalizer-example",
    "weaver-normalizer-parent",
    "weaver-normalizer",
    "weaver-parent",
    "weaver-privilizer-api",
    "weaver-privilizer-parent",
    "weaver-privilizer",
    "weaver-processor" );

  comp_list = make_list_unique( comp_list );

  return comp_list;
}
