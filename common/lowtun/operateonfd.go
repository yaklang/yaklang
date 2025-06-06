//go:build darwin || freebsd

/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2017-2023 WireGuard LLC. All Rights Reserved.
 */

package lowtun

import (
	"fmt"
)

func (tun *NativeTun) operateOnFd(fn func(fd uintptr)) {
	sysconn, err := tun.tunFile.SyscallConn()
	if err != nil {
		tun.errors <- fmt.Errorf("unable to find sysconn for tunfile: %s", err.Error())
		return
	}
	err = sysconn.Control(fn)
	if err != nil {
		tun.errors <- fmt.Errorf("unable to control sysconn for tunfile: %s", err.Error())
	}
}
