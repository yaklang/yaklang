###############################################################################
# Functions for F5 LSCs
#
# Authors:
# Michael Meyer
#
# Copyright:
# Copyright (c) 2016 Greenbone Networks GmbH
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
###############################################################################

# Global vars to avoid that we're repeatedly asking redis for something we already know.
global_var __f5_valid_products, __f5_kb_hotfix;

function f5_hotfix_from_kb() {

  local_var hotfix;

  # Used to not query redis with the get_kb_item below
  # to save some requests.
  if( ! isnull( __f5_kb_hotfix ) ) {
    return __f5_kb_hotfix;
  } else {
    if( ! hotfix = get_kb_item( "f5/big_ip/hotfix" ) ) {
      hotfix = int( 0 );
    } else {
      hotfix = int( hotfix );
      __f5_kb_hotfix = hotfix;
    }
  }
  return hotfix;
}

function f5_valid_products() {

  local_var valid_products;

  if( ! isnull( __f5_valid_products ) ) {
    return __f5_valid_products;
  } else {
    valid_products = make_list( "LTM", "AAM", "AFM", "APM", "ASM", "GTM", "PEM", "PSM", "WOM", "AVR", "WAM", "LC" );
    __f5_valid_products = valid_products;
    return valid_products;
  }
}

function is_f5_vulnerable( ca, version )
{
  local_var ca, version;
  local_var active_modules, is_vulnerable, product, av_array, affected, unaffected, af, a, affected_modules, unaffected_modules;

  if( ! is_array( ca ) )
    return;

  if( ! version )
    return;

  if( ! active_modules = get_kb_item( "f5/big_ip/active_modules" ) )
    return;

  is_vulnerable = FALSE;

  foreach product ( keys( ca ) )
  {
    if( ! in_array( search:product, array:f5_valid_products() ) )
      continue;

    if( product >!< active_modules )
      continue;

    av_array = ca[ product ];

    affected = av_array[ 'affected' ];
    unaffected = av_array[ 'unaffected' ];

    if( ! strlen( affected ) )
      return;

    if( strlen( unaffected ) )
    {
      if( f5_is_unaffected( version:version, unaffected:unaffected ) )
        return;
    }

    af = split( affected, sep:";", keep:FALSE );
    if( ! is_array( af ) )
      continue;

    foreach a ( af )
    {
      if( check_f5_version( a:a, version:version ) )
      {
        affected_modules += '\t' + product + ' (' + clean_version( v:a ) + ')\n';
        unaffected_modules += '\t' + product +': ' + clean_version( v:unaffected ) + '\n';
        is_vulnerable = TRUE;
        break;
      }
    }
  }

  if( is_vulnerable )
    return build_f5_report( affected_modules:affected_modules,
                            unaffected_modules:unaffected_modules,
                            version:version );

  return;
}

# affected   -> 11.0.0-11.5.3
# unaffected -> 11.4.1_HF10
function f5_is_unaffected( version, unaffected )
{
  local_var version, unaffected;
  local_var ua, uav, v_h, c_version, c_hotfix, both, first, last, first_hotfix, last_hotfix;

  if( ! version )
    return;

  if( ! unaffected )
    return;

  ua = split( unaffected, sep:";", keep:FALSE );
  if( ! is_array( ua ) )
    return;

  foreach uav ( ua )
  {
    if( "_HF" >< uav && "-" >!< uav )
    {
      v_h = eregmatch( pattern:'([0-9.]+)_HF([0-9]+)', string:uav );

      c_version = v_h[1];
      c_hotfix  = v_h[2];

      if( isnull( c_version ) || isnull( c_hotfix ) )
        return;

      if( version == c_version )
      {
        if( f5_hotfix_from_kb() >= int( c_hotfix ) )
          return TRUE;
      }
    }
    else
    {
      if( "-" >< uav )
      {
        both = split( uav, sep:'-', keep:FALSE );
        if( isnull( both[0] ) || isnull( both[1] ) )
          return;

        first = both[0];
        last  = both[1];

        first_hotfix = int( 0 );
        last_hotfix  = int( 0 );

        if( "_HF" >< first )
        {
          v_h = eregmatch( pattern:'([0-9.]+)_HF([0-9]+)', string:first );
          if( ! is_array( v_h ) )
            return;

          first = v_h[1];
          first_hotfix = v_h[2];
        }

        if( "_HF" >< last )
        {
          v_h = eregmatch( pattern:'([0-9.]+)_HF([0-9]+)', string:last );
          if( ! is_array( v_h ) )
            return;

          last = v_h[1];
          last_hotfix = v_h[2];
        }

        first += '.' + first_hotfix;
        last  += '.' + last_hotfix;

        if( version_in_range( version:version + '.' + f5_hotfix_from_kb(), test_version:first, test_version2:last ) )
          return TRUE;
      }
      else
      {
        if( version == uav )
          return TRUE;
      }
    }
  }
  return;
}

function check_f5_version( a, version )
{
  local_var a, version;
  local_var v, c_version, hotfixes, low_hotfix, hi_hotfix, both, b, v_high, v_low, hi, low, fvh, first_vuln_hotfix, v_h, c_hotfix, first, last;

  # 11.5.1 HF4-HF8
  if( a =~ "[0-9.]+_HF([0-9]+)-HF([0-9]+)" )
  {
    v = eregmatch( pattern:'([0-9.]+)_HF', string:a );
    if( isnull( v[1] ) )
      return;

    c_version = v[1];

    hotfixes = eregmatch( pattern:'[0-9.]+_HF([0-9]+)-HF([0-9]+)', string:a );
    if( isnull( hotfixes[1] ) || isnull( hotfixes[2] ) )
      return;

    low_hotfix = hotfixes[1];
    hi_hotfix  = hotfixes[2];

    if( version == c_version )
    {
      if( f5_hotfix_from_kb() < int( low_hotfix ) )
        return;

      if( f5_hotfix_from_kb() <= int( hi_hotfix ) )
        return TRUE;

    }
    return;
  }

  if( "_HF" >< a )
  {
    if( "-" >< a )
    {
      both = split( a, sep:'-', keep:FALSE );
      if( isnull( both[1] ) )
        return;

      a = both[1];
      b = both[0];

      v_high = eregmatch( pattern:'([0-9.]+)(_HF)?', string:a );
      v_low = eregmatch( pattern:'([0-9.]+)(_HF)?', string:b );

      hi = v_high[1];
      low = v_low[1];

      #11.4.0 - 11.5.4 HF1
      if( hi != low )
      {
        if( version_in_range( version:version, test_version:low, test_version2:hi ) )
        {
          if( version == hi )
          {
            fvh = eregmatch( pattern:'([0-9.]+)_HF([0-9]+)', string:a );
            if( ! isnull( fvh[2] ) )
            {
              if( f5_hotfix_from_kb() > int( fvh[2] ) )
                return;
            }
          }

          if( version == low )
          {
            fvh = eregmatch( pattern:'([0-9.]+)_HF([0-9]+)', string:b );
            if( ! isnull( fvh[2] ) )
            {
              if( f5_hotfix_from_kb() < int( fvh[2] ) )
                return;
            }
          }
          return TRUE;
        }
      }

      if( "_HF" >< b )
      {
        fvh = eregmatch( pattern:'([0-9.]+)_HF([0-9]+)', string:b );
        if( ! isnull( fvh[2] ) )
          first_vuln_hotfix = fvh[2];
      }
    }

    v_h = eregmatch( pattern:'([0-9.]+)_HF([0-9]+)', string:a );

    c_version = v_h[1];
    c_hotfix  = v_h[2];

    if( isnull( c_version ) || isnull( c_hotfix ) )
      return;

    if( c_version == version )
    {
      # Versions known to be vulnerable: 11.5.1 HF6 - 11.5.1 HF10
      if( first_vuln_hotfix )
      {
        if( f5_hotfix_from_kb() < int( first_vuln_hotfix ) )
          return;
      }

      if( f5_hotfix_from_kb() <= int( c_hotfix ) )
        return TRUE;
    }
    return;
  }
  else
  {
    if( "-" >< a )
    {
      both = split( a, sep:'-', keep:FALSE );
      if( isnull( both[0] ) || isnull( both[1] ) )
        return;

      first = both[0];
      last  = both[1];

      if( version_in_range( version:version, test_version:first, test_version2:last ) )
        return TRUE;
    }
    else
    {
      if( version == a )
        return TRUE;
    }
  }
  return;
}

function clean_version( v )
{
  local_var v, ret;

  ret = str_replace( string:v, find:';', replace:", " );
  ret = str_replace( string:ret, find:'_HF', replace:" HF" );
  ret = str_replace( string:ret, find:'-', replace:" - " );
  ret = ereg_replace( string:ret, pattern:', $', replace:"" );
  return ret;
}

function build_f5_report( affected_modules, unaffected_modules, version )
{
  local_var affected_modules, unaffected_modules, version;
  local_var report, hotfix;

  report = 'Installed Version: ' + version + '\n';

  hotfix = f5_hotfix_from_kb();
  if( hotfix )
    report += 'Installed Hotfix: ' + hotfix;

  report += '\n\nAffected Modules:\n\n' + affected_modules + '\n';
  report += '\nUnaffected Modules:\n\n' + unaffected_modules + '\n';
  return report;
}