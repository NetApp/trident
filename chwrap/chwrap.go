// Copyright 2022 NetApp, Inc. All Rights Reserved.

//go:build !windows
// +build !windows

package main

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func validBinary(path string) bool {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); nil != err {
		// Can't stat file
		return false
	}
	if (stat.Mode&unix.S_IFMT) != unix.S_IFREG && (stat.Mode&unix.S_IFMT) != unix.S_IFLNK {
		// Not a regular file or symlink
		return false
	}
	if 0 == stat.Mode&unix.S_IRUSR || 0 == stat.Mode&unix.S_IXUSR {
		// Not readable or not executable
		return false
	}
	return true
}

func findBinary(prefix, binary string) string {
	for _, part1 := range []string{"usr/local/", "usr/", ""} {
		for _, part2 := range []string{"sbin", "bin"} {
			path := "/" + part1 + part2 + "/" + binary
			if validBinary(prefix + path) {
				return path
			}
		}
	}
	return ""
}

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
	rootPath := "/host"
	// First modify argv0 to strip off any absolute or relative paths
	argv := os.Args
	binary := argv[0]
	idx := strings.LastIndexByte(binary, '/')
	if 0 <= idx {
		binary = binary[idx+1:]
	}
	// Now implement the path search logic, but in the host's filesystem
	argv0 := findBinary(rootPath, binary)
	if "" == argv0 {
		// binary is not found on /host.
		argv0 = findBinary("", binary)
		if "" == argv0 {
			panic(binary + " not found")
		} else {
			// Binary found locally.
			rootPath = "/"
		}
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
