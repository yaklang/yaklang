#!/bin/sh
git log --format="%s" $(git for-each-ref --sort=-creatordate --format '%(refname:short)' refs/tags | grep -v '\-alpha' | grep -v '\-beta' | head -n 1)..$(git rev-parse HEAD) > /tmp/msg.txt && cat /tmp/msg.txt
