//go:build !windows && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package main

import "golang.org/x/sys/unix"

const (
	ioctlReadTermios  = unix.TCGETS
	ioctlWriteTermios = unix.TCSETS
)
