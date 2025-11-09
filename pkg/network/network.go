// Copyright 2025 NetApp, Inc. All Rights Reserved.

package network

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"go.uber.org/multierr"

	"github.com/netapp/trident/logging"
)

var (
	DNS1123LabelRegex  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	DNS1123DomainRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

// ValidateCIDRs checks if a list of CIDR blocks are valid and returns a multi error containing all errors
// which may occur during the parsing process.
func ValidateCIDRs(ctx context.Context, cidrs []string) error {
	var err error
	// needed to capture all issues within the CIDR set
	errors := make([]error, 0)
	for _, cidr := range cidrs {
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			errors = append(errors, err)
			logging.Logc(ctx).WithError(err).Error("Found an invalid CIDR.")
		}
	}

	if len(errors) != 0 {
		err = multierr.Combine(errors...)
		return err
	}

	return err
}

// ValidateIPs parses a list of IPs, attempting to validate them with net.ParseIP.
// It returns error in case the IPs are not valid.
func ValidateIPs(ctx context.Context, ipStrings []string) error {
	var err error
	errors := make([]error, 0)

    for _, ipStr := range ipStrings {
        // net.ParseIP parses strings as an IP address, returning nil if error.
        ip := net.ParseIP(ipStr)
        if ip == nil {
			errors = append(errors, fmt.Errorf("found an invalid IP: %s", ipStr))
			logging.Logc(ctx).WithError(fmt.Errorf("found an invalid IP: %s", ipStr)).Error("Found an invalid IP.")
        }
    }
	
	if len(errors) != 0 {
		err = multierr.Combine(errors...)
		return err
	}

	return err
}

// FilterIPs takes a list of IPs and CIDRs and returns the sorted list of IPs that are contained by one or more of the
// CIDRs
func FilterIPs(ctx context.Context, ips, cidrs []string) ([]string, error) {
	networks := make([]*net.IPNet, len(cidrs))
	filteredIPs := make([]string, 0)

	for i, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			err = fmt.Errorf("error parsing CIDR; %v", err)
			logging.Logc(ctx).WithField("CIDR", cidr).Error(err)
			return nil, err
		}
		networks[i] = ipNet
	}

	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		parsedIP := net.ParseIP(ip)
		for _, network := range networks {
			fields := logging.LogFields{
				"IP":      ip,
				"Network": network.String(),
			}
			if network.Contains(parsedIP) {
				logging.Logc(ctx).WithFields(fields).Debug("IP found in network.")
				filteredIPs = append(filteredIPs, ip)
				break
			} else {
				logging.Logc(ctx).WithFields(fields).Debug("IP not found in network.")
			}
		}
	}

	sort.Strings(filteredIPs)

	return filteredIPs, nil
}

func IPv6Check(ip string) bool {
	return strings.Count(ip, ":") >= 2
}

// ParseHostportIP returns just the IP address part of the given input IP address and strips any port information
func ParseHostportIP(hostport string) string {
	ipAddress := ""
	if IPv6Check(hostport) {
		// this is an IPv6 address, remove port value and add square brackets around the IP address
		if hostport[0] == '[' {
			ipAddress = strings.Split(hostport, "]")[0] + "]"
		} else {
			// assumption here is that without the square brackets its only IP address without port information
			ipAddress = "[" + hostport + "]"
		}
	} else {
		ipAddress = strings.Split(hostport, ":")[0]
	}

	return ipAddress
}

// EnsureHostportFormatted ensures IPv6 hostport is in correct format
func EnsureHostportFormatted(hostport string) string {
	// If this is an IPv6 address, ensure IP address is enclosed in square
	// brackets, as in "[::1]:80".
	if IPv6Check(hostport) && hostport[0] != '[' {
		// assumption here is that without the square brackets its only IP address without port information
		return "[" + hostport + "]"
	}

	return hostport
}

// GetBaseImageName accepts a container image name and return just the base image.
func GetBaseImageName(name string) string {
	imageParts := strings.Split(name, "/")
	remainder := imageParts[len(imageParts)-1]
	return remainder
}

// ReplaceImageRegistry accepts a container image name and a registry name (FQDN[:port]/[subdir]*) and
// returns the same image name with the supplied registry.
func ReplaceImageRegistry(image, registry string) string {
	remainder := GetBaseImageName(image)
	if registry == "" {
		return remainder
	}
	return registry + "/" + remainder
}
