<?php
    // demo1
        $a = $_GET['a'];
        include $a;
    //demo2
        $a = $_GET['a'] ?: "aaaa";
        include(xxx($a));
    $INCLUDE_ALLOW_LIST = [
        "home.php",
        "dashboard.php",
        "profile.php",
        "settings.php"
    ];

    $filename = $_GET["filename"];
    $d = filter($filename, $INCLUDE_ALLOW_LIST);
    include($d);