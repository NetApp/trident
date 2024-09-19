// Copyright 2022 NetApp, Inc. All Rights Reserved.

//go:build !windows
// +build !windows

package main

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/netapp/trident/internal/chwrap"
)

func modifyEnv(oldEnv []string) []string {
	var newEnv []string
	for _, e := range oldEnv {
		if !strings.HasPrefix(e, "PATH=") {
			newEnv = append(newEnv, e)
		}
	}
	newEnv = append(newEnv, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	return newEnv
}

func main() {
	// First modify argv0 to strip off any absolute or relative paths
	argv := os.Args
	binary := argv[0]
	idx := strings.LastIndexByte(binary, '/')
	if 0 <= idx {
		binary = binary[idx+1:]
	}

	rootPath, argv0 := chwrap.FindBinary(binary)
	if "" == argv0 {
		panic(binary + " not found")
	}

	// Chroot in the host's FS
	if err := unix.Chroot(rootPath); nil != err {
		panic(err)
	}
	// Change cwd to the root
	if err := unix.Chdir("/"); nil != err {
		panic(err)
	}
	// Exec the intended binary
	if err := unix.Exec(argv0, argv, modifyEnv(os.Environ())); nil != err {
		panic(err)
	}
}
