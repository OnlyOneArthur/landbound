//go:build linux

package demo

import "golang.org/x/sys/unix"

func unixClose(fd int) {
	_ = unix.Close(fd)
}
