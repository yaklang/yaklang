#!/bin/bash

echo $1
pid=$(pidof logagent)
if [[ -n "${pid}" ]]; then
  kill -9 $pid
  sleep 1
fi

sudo mkdir -p /usr/share/logagent
sudo cp ./$1  /usr/share/logagent
sudo ln -s -f /usr/share/logagent/$1 /usr/bin/logagent
sudo mv -f ./logagent.service /etc/init.d/logagent
#默认mq host ip
# /usr/bin/logagent mmhp --mh 192.168.248.116
sudo service logagent install

