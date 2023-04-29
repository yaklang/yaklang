#!/bin/bash


pid=$(pidof palm)
if [[ -n "${pid}" ]]; then
  kill -9 $pid
  sleep 3
fi
sudo rm -rf /usr/share/palm/www/fe
sudo mkdir -p /usr/share/palm
sudo mkdir -p /usr/share/palm/www
sudo mv fe  /usr/share/palm/www
sudo service palm restart
