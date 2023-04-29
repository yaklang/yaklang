#!/usr/bin/env bash

set -e

PALM_DIR=/go/src/palm

cd $PALM_DIR

echo "Start Build HIDS Agent"
rm -rf $PALM_DIR/build/hids
mkdir -p $PALM_DIR/build/hids

cd ./hidsnode/cmd/
flags=" -X main._version_=`git show -s --format='%H'` -linkmode "external" -extldflags "-static" "
gitversion=$(git show -s --format='%H')
hids_name="hids_"+$gitversion
go build  -ldflags "$flags" -v  -o $PALM_DIR/build/hids/$hids_name hids-agent.go

cd $PALM_DIR
echo "makeself hids start"

echo "copy hids.service"
cp ./scripts/hids.service $PALM_DIR/build/hids
echo "copy install_hids.sh"
cp ./scripts/install_hids.sh $PALM_DIR/build/hids

makeself  $PALM_DIR/build/hids $PALM_DIR/build/hids.package "hids package" ./install_hids.sh $hids_name

echo "makeself hdis over"


echo "Start Build Log Agent"
rm -rf $PALM_DIR/build/logagent
mkdir -p $PALM_DIR/build/logagent

cd ./logagent/cmd/
flags=" -X main._version_=`git show -s --format='%H'` -linkmode "external" -extldflags "-static" "
gitversion=$(git show -s --format='%H')
logagent_name="logagent_"+$gitversion
go build  -ldflags "$flags" -v  -o $PALM_DIR/build/logagent/$logagent_name logagent.go

cd $PALM_DIR
echo "makeself logagent start"

echo "copy logagent.service"
cp ./scripts/logagent.service $PALM_DIR/build/logagent
echo "copy install_logagent.sh"
cp ./scripts/install_logagent.sh $PALM_DIR/build/logagent

makeself  $PALM_DIR/build/logagent $PALM_DIR/build/logagent.package "logagent package" ./install_logagent.sh $logagent_name

echo "makeself logagent over"




echo "Start Build Palm"
rm -rf $PALM_DIR/build/palm
mkdir -p $PALM_DIR/build/palm

cd $PALM_DIR
cd ./server/cmd/
flags=" -X main._version_=`git show -s --format='%H'` -linkmode "external" -extldflags "-static" "
palm_name="palm_"+$gitversion

go build -v -ldflags "$flags"  -o $PALM_DIR/build/palm/$palm_name server.go

cd $PALM_DIR
echo "makeself palm start"

echo "copy palm.service"
cp ./scripts/palm.service $PALM_DIR/build/palm

echo "copy hids.service"
cp ./scripts/hids.service $PALM_DIR/build/palm

echo  "copy install_palm.sh"
cp ./scripts/install_palm.sh $PALM_DIR/build/palm

makeself  $PALM_DIR/build/palm $PALM_DIR/build/palm.package "palm package"  ./install_palm.sh $palm_name

echo "makeself palm end"

#暂时屏蔽掉
exit 0

cd $PALM_DIR
echo "Start Build Scan Node"
cd ./scannode/cmd/
CGO_ENABLED=1 go build -v -o $PALM_DIR/build/scannode scan-node.go
go build -v -o $PALM_DIR/build/scannode scan-node.go



