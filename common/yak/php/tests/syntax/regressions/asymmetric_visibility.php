<?php
// PHP 8.4 asymmetric visibility: memberModifier accepts
// private(set) / public(set) / protected(set) after the read modifier
// (grammar: memberModifier gained (Pub|Prot|Priv) '(' Label ')').

namespace AsymmetricVisibilityFixture;

class Node
{
    public private(set) string $name;
    protected public(set) int $count;
    public private(set) ?string $internalSubset;
    private protected(set) array $tags = [];
}