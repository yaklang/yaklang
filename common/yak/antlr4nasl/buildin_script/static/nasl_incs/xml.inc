###############################################################################
# OpenVAS Include File
# $Id: xml.inc 11825 2018-10-10 14:07:01Z cfischer $
#
# XML generation functions
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
#
# $Revision: 11825 $
#
###############################################################################

# @brief Returns a XML "opening" tag and adds passed additional attributes to the tag.
#
# @param tag        the tag to use as a XML tag, e.g. test to create a tag like <test>
# @param attributes the attributes to add to the tag (optional), this needs to be passed as an array
#                   which was created in the form of e.g. attributes['attributename'] = 'attributevalue';
#
# @return A string containing a XML representation of the passed parameters or NULL if
#         an empty tag parameter was passed or no array was passed to the attributes parameter.
#
#         Example:
#         str = "tagname";
#         attr_array['attributename'] = 'attributevalue';
#         xml_open_tag(tag:str, attributes:attr_array);
#
#         Result:
#         <tagname attributename="attributevalue">
#
function xml_open_tag(tag, attributes) {

  local_var tag, attributes, res, attr;

  if (isnull(tag)) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#tag#-#xml_open_tag");
    return NULL;
  }

  if (attributes && typeof(attributes) != "array") {
    set_kb_item(name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#xml_open_tag: attributes parameter not empty but no array passed.");
    return NULL;
  }

  res = '';

  res += '<' + tag;

  if (!isnull(attributes)) {
    foreach attr (keys(attributes)) {
      res += ' ' + attr + '="' + xml_escape(str:attributes[attr]) + '"';
    }
  }
  res += '>';
  return res;
}

# @brief Returns a XML "closing" tag.
#
# @param tag the tag to use as a XML tag, e.g. test to create a tag like </test>
#
# @return A string containing a XML representation of the passed parameters or
#         NULL if an empty tag parameter was passed.
#
#         Example: xml_close_tag(tag:"tagname");
#         Result: </tagname>
#
function xml_close_tag(tag) {

  local_var tag;

  if (isnull(tag)) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#tag#-#xml_close_tag");
    return NULL;
  }

  return '</' + tag + '>';
}

# @brief Returns a complete XML representation of various passed data in one single string "line".
#
# @param tag        the tag to use as a XML tag, e.g. test to use a tag like <test></test>
# @param attributes the attributes to add to the tag (optional), this needs to be passed as an array
#                   which was created in the form of e.g. attributes['attributename'] = 'attributevalue';
# @param value      the value to add within the XML tag (optional), e.g. <test>value</test>
#
# @return A string containing a XML representation of the passed parameters or NULL if
#         an empty tag parameter was passed.
#
#         Example:
#         tagstr = "tagname";
#         tagval = "tagvalue";
#         attr_array['attributename'] = 'attributevalue';
#         xml_tagline(tag:tagstr, attributes:attr_array, value:tagval);
#
#         Result:
#         <tagname attributename="attributevalue">tagvalue</tagname>
#
function xml_tagline(tag, attributes, value) {

  local_var tag, attributes, value;

  if (isnull(tag)) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#tag#-#xml_tagline");
    return NULL;
  }

  if (attributes && typeof(attributes) != "array") {
    set_kb_item(name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#xml_tagline: attributes parameter not empty but no array passed.");
    return NULL;
  }

  return xml_open_tag(tag:tag, attributes:attributes) + xml_escape(str:value) + xml_close_tag(tag:tag);
}

# @brief Returns the XML representation of an opened XML comment.
#
# @return A string containing the XML representation of an opened XML comment.
#
function xml_open_comment() {
  return '<!--';
}

# @brief Returns the XML representation of a closed XML comment.
#
# @return A string containing the XML representation of a closed XML comment.
#
function xml_close_comment() {
  return '-->';
}

# @brief Returns the XML representation of a XML newline.
#
# @return A string containing the XML representation of a XML newline.
#
function xml_newline() {
  return '\n';
}

# @brief Evaluates a passed string for reserved XML (e.g. &, < or >) chars which needs to be escaped
#        and returns representation of the passed string with those chars escaped.
#
# @param str the string to evaluate
#
# @return A string containing the representation of the passed string with escaped chars or NULL
#         if an empty str parameter was passed.
#
function xml_escape(str) {

  local_var str, escape_table, str_escaped, str, max_idx, i;

  if(isnull(str)) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#str#-#xml_escape");
    return NULL;
  }

  escape_table = make_array(
                      "&", "&amp;",
                      "'", "&apos;",
                      '"', "&quot;",
                      "<", "&lt;",
                      ">", "&gt;");

  str_escaped = "";

  max_idx = strlen(str);

  for (i = 0; i < max_idx; i++) {
    if (!isnull(escape_table[str[i]]))
      str_escaped += escape_table[str[i]];
    else
      str_escaped += str[i];
  }
  return str_escaped;
}