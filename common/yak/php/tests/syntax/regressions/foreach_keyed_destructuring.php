<?php
// Keyed foreach array destructuring (PHP 7.1+): grammar foreachValue
// accepts "as $k => [$a, $b]" and "as $k => &$v" tails
// (php-src run-tests.php / curl stubs use the keyed destructuring form).

namespace ForeachKeyedDestructuringFixture;

function iterate(array $curlConstants, array $notInPHP, array $data)
{
    foreach ($curlConstants as $name => [$introduced, $deprecated, $removed]) {
        echo $name, $introduced, $deprecated, $removed;
    }
    foreach ($notInPHP as $name => [$introduced, $removed]) {
        echo $name, $introduced, $removed;
    }
    foreach ($data as [$a, $b]) {
        echo $a, $b;
    }
    foreach ($data as $k => $v) {
        echo $k, $v;
    }
    foreach ($data as $key => &$ref) {
        echo $key, $ref;
    }
}