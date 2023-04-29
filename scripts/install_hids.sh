#!/bin/bash

echo $1
pid=$(pidof hids)
if [[ -n "${pid}" ]]; then
  kill -9 $pid
  sleep 1
fi

sudo mkdir -p /usr/share/hids
sudo cp ./$1  /usr/share/hids
sudo ln -s -f /usr/share/hids/$1 /usr/bin/hids
sudo mv -f ./hids.service /etc/init.d/hids
#默认mq host ip
# /usr/bin/hids mmhp --mh 192.168.248.116
sudo service hids install

