<?php
// demo1
$doc = new DOMDocument();
$doc->load('xxe.xml', LIBXML_NOENT); // Noncompliant

// demo2
$xml = file_get_contents('xxe.xml');
$doc = simplexml_load_string($xml, 'SimpleXMLElement', LIBXML_NOENT); // Noncompliant
