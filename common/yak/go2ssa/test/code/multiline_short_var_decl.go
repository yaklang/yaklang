//go:build ignore
// +build ignore

package main

func establishKeys() {
	clientMAC, serverMAC, clientKey, serverKey, clientIV, serverIV :=
		keysFromMasterSecret(1, nil, nil, nil, nil, 1, 1, 1)
	_ = clientMAC
}
