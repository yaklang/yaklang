
<?php
    // demo 1
    if( isset( $_POST[ 'Submit' ]  ) ) {
        // Get input
        $target = $_REQUEST[ 'ip' ];
        if( stristr( php_uname( 's' ), 'Windows NT' ) ) {
            // Windows
            $cmd = shell_exec( 'ping  ' . $target );
        }
        else {
            // *nix
            $cmd = shell_exec( 'ping  -c 4 ' . $target );
        }
    }

    // demo2
    if( isset( $_POST[ 'Submit' ]  ) ) {
        $target = $_REQUEST[ 'ip' ];
        $substitutions = array(
            '&'  => '',
            ';'  => '',
            '| ' => '',
            '-'  => '',
            '$'  => '',
            '('  => '',
            ')'  => '',
            '`'  => '',
            '||' => '',
        );
        $target = trim( array_keys( $substitutions ), $substitutions, $target );
        if( stristr( php_uname( 's' ), 'Windows NT' ) ) {
            // Windows
            $cmd = shell_exec( 'ping  ' . $target );
        }
        else {
            $cmd = shell_exec( 'ping  -c 4 ' . $target );
        }
    }
    //demo3
    if( isset( $_POST[ 'Submit' ]  ) ) {
        $target = $_REQUEST[ 'ip' ];
        $substitutions = array(
            '&'  => '',
            ';'  => '',
            '| ' => '',
            '-'  => '',
            '$'  => '',
            '('  => '',
            ')'  => '',
            '`'  => '',
            '||' => '',
        );
        $target = str_replace( array_keys( $substitutions ), $substitutions, $target );
        if( stristr( php_uname( 's' ), 'Windows NT' ) ) {
            // Windows
            $cmd = shell_exec( 'ping  ' . $target );
        }
        else {
            $cmd = shell_exec( 'ping  -c 4 ' . $target );
        }
    }

    // demo4
    if( isset( $_POST[ 'Submit' ]  ) ) {
        $target = $_REQUEST[ 'ip' ];
        $substitutions = array(
            '&'  => '',
            ';'  => '',
            '| ' => '',
            '-'  => '',
            '$'  => '',
            '('  => '',
            ')'  => '',
            '`'  => '',
            '||' => '',
        );
        $target = preg_replace( array_keys( $substitutions ), $substitutions, $target );
        if( stristr( php_uname( 's' ), 'Windows NT' ) ) {
            // Windows
            $cmd = shell_exec( 'ping  ' . $target );
        }
        else {
            $cmd = shell_exec( 'ping  -c 4 ' . $target );
        }
    }

    // demo 5

    if( isset( $_POST[ 'Submit' ]  ) ) {
        $target = $_REQUEST[ 'ip' ];
        $substitutions = array(
            '&'  => '',
            ';'  => '',
            '| ' => '',
            '-'  => '',
            '$'  => '',
            '('  => '',
            ')'  => '',
            '`'  => '',
            '||' => '',
        );
        $target = trim( array_keys( $substitutions ), $substitutions, $target );
        if( stristr( php_uname( 's' ), 'Windows NT' ) ) {
            // Windows
            $cmd = shell_exec( 'ping  ' . $target );
        }
        else {
            $cmd = shell_exec( 'ping  -c 4 ' . $target );
        }
    }

    // demo 6

    if( isset( $_POST[ 'Submit' ]  ) ) {
        $target = $_REQUEST[ 'ip' ];
        $substitutions = array(
            '&'  => '',
            ';'  => '',
            '| ' => '',
            '-'  => '',
            '$'  => '',
            '('  => '',
            ')'  => '',
            '`'  => '',
            '||' => '',
        );
        $target = str_replace( array_keys( $substitutions ), $substitutions, $target );
        if( stristr( php_uname( 's' ), 'Windows NT' ) ) {
            // Windows
            $cmd = shell_exec( 'ping  ' . $target );
        }
        else {
            $cmd = shell_exec( 'ping  -c 4 ' . $target );
        }
    }

    // demo 7
    if( isset( $_POST[ 'Submit' ]  ) ) {
        $target = $_REQUEST[ 'ip' ];
        $substitutions = array(
            '&'  => '',
            ';'  => '',
            '| ' => '',
            '-'  => '',
            '$'  => '',
            '('  => '',
            ')'  => '',
            '`'  => '',
            '||' => '',
        );
        $target = preg_replace( array_keys( $substitutions ), $substitutions, $target );
        if( stristr( php_uname( 's' ), 'Windows NT' ) ) {
            // Windows
            $cmd = shell_exec( 'ping  ' . $target );
        }
        else {
            $cmd = shell_exec( 'ping  -c 4 ' . $target );
        }
    }