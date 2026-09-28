package sgnds

import (
	_ "embed"
)

//go:embed bridge/sgnds-bridge.jar
var bridgeJarBytes []byte

//go:embed bridge/sg-jdbc-driver.jar
var driverJarBytes []byte
