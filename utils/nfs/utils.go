// Copyright 2025 NetApp, Inc. All Rights Reserved.

package nfs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/netapp/trident/pkg/collection"
)

var (
	majorVersionRegex      = regexp.MustCompile(`^(nfsvers|vers)=(?P<major>\d)$`)
	minorVersionRegex      = regexp.MustCompile(`^minorversion=(?P<minor>\d)$`)
	majorMinorVersionRegex = regexp.MustCompile(`^(nfsvers|vers)=(?P<major>\d)\.(?P<minor>\d)$`)
)

// GetNFSVersionFromMountOptions accepts a set of mount options, a default NFS version, and a list of
// supported NFS versions, and it returns the NFS version specified by the mount options, or the default
// if none is found, plus an error (if any).  If a set of supported versions is supplied, and the returned
// version isn't in it, this method returns an error; otherwise it returns nil.
func GetNFSVersionFromMountOptions(mountOptions, defaultVersion string, supportedVersions []string) (string, error) {
	major := ""
	minor := ""

	// Strip off -o prefix if present
	mountOptions = strings.TrimPrefix(mountOptions, "-o ")

	// Check each mount option using the three mutually-exclusive regular expressions.  Last option wins.
	for _, mountOption := range strings.Split(mountOptions, ",") {
		mountOption = strings.TrimSpace(mountOption)

		if matchGroups := getRegexSubmatches(majorMinorVersionRegex, mountOption); matchGroups != nil {
			major = matchGroups["major"]
			minor = matchGroups["minor"]
			continue
		}

		if matchGroups := getRegexSubmatches(majorVersionRegex, mountOption); matchGroups != nil {
			major = matchGroups["major"]
			continue
		}

		if matchGroups := getRegexSubmatches(minorVersionRegex, mountOption); matchGroups != nil {
			minor = matchGroups["minor"]
			continue
		}
	}

	version := ""

	// Assemble version string from major/minor values.
	if major == "" {
		// Minor doesn't matter if major isn't set.
		version = ""
	} else if minor == "" {
		// Major works by itself if minor isn't set.
		version = major
	} else {
		version = major + "." + minor
	}

	// Handle default & supported versions.
	if version == "" {
		return defaultVersion, nil
	} else if supportedVersions == nil || collection.ContainsString(supportedVersions, version) {
		return version, nil
	} else {
		return version, fmt.Errorf("unsupported NFS version: %s", version)
	}
}

// getRegexSubmatches accepts a regular expression with one or more groups and returns a map
// of the group matches found in the supplied string.
func getRegexSubmatches(r *regexp.Regexp, s string) map[string]string {
	match := r.FindStringSubmatch(s)
	if match == nil {
		return nil
	}

	paramsMap := make(map[string]string)
	for i, name := range r.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}
	return paramsMap
}
