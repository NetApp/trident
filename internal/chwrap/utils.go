//go:build !windows

package chwrap

import "golang.org/x/sys/unix"

const (
	hostRootPath      = "/host"
	containerRootPath = "/"
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

func FindBinary(binary string) (rootPath, fullPath string) {
	if fullPath = findBinary(hostRootPath, binary); fullPath != "" {
		return hostRootPath, fullPath
	}
	return containerRootPath, findBinary("", binary)
}
