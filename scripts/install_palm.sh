#!/bin/bash

echo $1
pid=$(pidof palm)
if [[ -n "${pid}" ]]; then
  kill -9 $pid
  sleep 3
fi
sudo mkdir -p /usr/share/palm
sudo mkdir -p /usr/share/palm/scripts

sudo cp ./$1  /usr/share/palm
sudo ln -s -f /usr/share/palm/$1 /usr/bin/palm

sudo cp ./palm.service /usr/share/palm/scripts
#sudo \\cp   -fr ./hids.service /usr/share/palm/scripts

sudo mv -f palm.service /etc/init.d/palm
sudo mv -f palm.service /etc/systemd/system/palm.service
sudo service palm install
