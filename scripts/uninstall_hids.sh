#!/bin/bash

echo $1
pid=$(pidof hids)
if [[ -n "${pid}" ]]; then
  kill -9 $pid
  sleep 1
fi

rm -rf /usr/share/hids
rm -f /usr/bin/hids
rm -f /var/log/hids.log

