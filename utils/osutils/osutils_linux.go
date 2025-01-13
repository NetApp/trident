// Copyright 2024 NetApp, Inc. All Rights Reserved.

package osutils

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path"
	"time"

	"github.com/vishvananda/netlink"
	"github.com/zcalusic/sysinfo"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// NFSActiveOnHost will return if the rpc-statd daemon is active on the given host
func (o *OSUtils) NFSActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_linux.NFSActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_linux.NFSActiveOnHost")

	return o.ServiceActiveOnHost(ctx, "rpc-statd")
}

// ServiceActiveOnHost checks if the service is currently running
func (o *OSUtils) ServiceActiveOnHost(ctx context.Context, service string) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_linux.ServiceActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_linux.ServiceActiveOnHost")

	output, err := o.command.ExecuteWithTimeout(ctx, "systemctl", 30*time.Second, true, "is-active", service)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			Logc(ctx).WithField("service", service).Debug("Service is not active on the host.")
			return false, nil
		} else {
			err = fmt.Errorf("unexpected error while checking if service is active; %s; %+v", string(output), err)
			Logc(ctx).WithField("service", service).Error(err)
			return false, err
		}
	}
	Logc(ctx).WithField("service", service).Debug("Service is active on the host.")
	return true, nil
}

// GetHostSystemInfo returns information about the host system
func (o *OSUtils) GetHostSystemInfo(ctx context.Context) (*models.HostSystem, error) {
	Logc(ctx).Debug(">>>> osutils_linux.GetHostSystemInfo")
	defer Logc(ctx).Debug("<<<< osutils_linux.GetHostSystemInfo")

	var (
		data []byte
		err  error
	)

	osInfo := sysinfo.OS{}
	msg := "Problem reading host system info."

	if runningInContainer() {
		// Get the hosts' info via tridentctl because the sysInfo library needs to be chrooted in order to detect
		// the host OS and not the container's but chroot is irreversible and thus needs to run in a separate
		// short-lived binary
		data, err = o.command.ExecuteWithTimeout(ctx, "tridentctl", 5*time.Second, true, "system", "--chroot-path",
			"/host")
		if err != nil {
			Logc(ctx).WithField("err", err).Error(msg)
			return nil, err
		}
		err = json.Unmarshal(data, &osInfo)
		if err != nil {
			Logc(ctx).WithField("err", err).Error(msg)
			return nil, err
		}
	} else {
		// If we're not in a container, get the information directly
		var si sysinfo.SysInfo
		si.GetSysInfo()
		osInfo = si.OS
	}

	// sysInfo library is linux-only, so we must translate the data
	// into a platform agnostic struct here to return further up the stack
	host := &models.HostSystem{}
	host.OS.Distro = osInfo.Vendor
	host.OS.Version = osInfo.Version
	host.OS.Release = osInfo.Release
	return host, nil
}

// getIPAddresses uses the Linux-specific netlink library to get a host's external IP addresses.
func (o *OSUtils) getIPAddresses(ctx context.Context) ([]net.Addr, error) {
	Logc(ctx).Debug(">>>> osutils_linux.getIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils_linux.getIPAddresses")

	// Use a map to deduplicate addresses found via different algorithms.
	addressMap := make(map[string]net.Addr)

	// Consider addresses from non-dummy interfaces
	addresses, err := o.getIPAddressesExceptingDummyInterfaces(ctx)
	if err != nil {
		return nil, err
	}
	for _, addr := range addresses {
		addressMap[addr.String()] = addr
	}

	// Consider addresses from interfaces on default routes
	addresses, err = o.getIPAddressesExceptingNondefaultRoutes(ctx)
	if err != nil {
		return nil, err
	}
	for _, addr := range addresses {
		addressMap[addr.String()] = addr
	}

	addrs := make([]net.Addr, 0)
	for _, addr := range addressMap {
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// getIPAddressesExceptingDummyInterfaces returns all global unicast addresses from non-dummy interfaces.
func (o *OSUtils) getIPAddressesExceptingDummyInterfaces(ctx context.Context) ([]net.Addr, error) {
	Logc(ctx).Debug(">>>> osutils_linux.getAddressesExceptingDummyInterfaces")
	defer Logc(ctx).Debug("<<<< osutils_linux.getAddressesExceptingDummyInterfaces")

	allLinks, err := netLink.LinkList()
	if err != nil {
		Logc(ctx).Error(err)
		return nil, err
	}

	links := make([]netlink.Link, 0)
	for _, link := range allLinks {

		if link.Type() == "dummy" {
			Log().WithFields(LogFields{
				"interface": link.Attrs().Name,
				"type":      link.Type(),
			}).Debug("Dummy interface, skipping.")
			continue
		}

		links = append(links, link)
	}

	return o.getUsableAddressesFromLinks(ctx, links), nil
}

// getIPAddressesExceptingNondefaultRoutes returns all global unicast addresses from interfaces on default routes.
func (o *OSUtils) getIPAddressesExceptingNondefaultRoutes(ctx context.Context) ([]net.Addr, error) {
	Logc(ctx).Debug(">>>> osutils_linux.getAddressesExceptingNondefaultRoutes")
	defer Logc(ctx).Debug("<<<< osutils_linux.getAddressesExceptingNondefaultRoutes")

	// Get all default routes (nil destination)
	routes, err := netLink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{}, netlink.RT_FILTER_DST)
	if err != nil {
		Logc(ctx).Error(err)
		return nil, err
	}

	// Get deduplicated set of links associated with default routes
	intfIndexMap := make(map[int]struct{})
	for _, route := range routes {
		Logc(ctx).WithField("route", route.String()).Debug("Considering default route.")
		intfIndexMap[route.LinkIndex] = struct{}{}
	}

	links := make([]netlink.Link, 0)
	for linkIndex := range intfIndexMap {
		if link, err := netLink.LinkByIndex(linkIndex); err != nil {
			Logc(ctx).Error(err)
		} else {
			links = append(links, link)
		}
	}

	return o.getUsableAddressesFromLinks(ctx, links), nil
}

// getUsableAddressesFromLinks returns all global unicast addresses on the specified interfaces.
func (o *OSUtils) getUsableAddressesFromLinks(ctx context.Context, links []netlink.Link) []net.Addr {
	addrs := make([]net.Addr, 0)

	for _, link := range links {

		logFields := LogFields{"interface": link.Attrs().Name, "type": link.Type()}
		Logc(ctx).WithFields(logFields).Debug("Considering interface.")

		linkAddrs, err := netLink.AddrList(link, netlink.FAMILY_ALL)
		if err != nil {
			Log().WithFields(logFields).Errorf("Could not get addresses for interface; %v", err)
			continue
		}

		for _, linkAddr := range linkAddrs {

			logFields := LogFields{"interface": link.Attrs().Name, "address": linkAddr.String()}

			ipNet := linkAddr.IPNet
			if ipNet == nil {
				Log().WithFields(logFields).Debug("Address IPNet is nil, skipping.")
				continue
			}

			if !ipNet.IP.IsGlobalUnicast() {
				Log().WithFields(logFields).Debug("Address is not global unicast, skipping.")
				continue
			}

			Log().WithFields(logFields).Debug("Address is potentially viable.")
			addrs = append(addrs, ipNet)
		}
	}

	return addrs
}

// IsLikelyDir determines if mountpoint is a directory
func (o *OSUtils) IsLikelyDir(mountpoint string) (bool, error) {
	stat, err := o.osFs.Stat(mountpoint)
	if err != nil {
		return false, err
	}

	return stat.IsDir(), nil
}

// GetTargetFilePath method returns the path of target file based on OS.
func GetTargetFilePath(ctx context.Context, resourcePath, arg string) string {
	Logc(ctx).Debug(">>>> osutils_linux.GetTargetFilePath")
	defer Logc(ctx).Debug("<<<< osutils_linux.GetTargetFilePath")
	return path.Join(resourcePath, arg)
}

// SMBActiveOnHost will always return false on non-windows platform
func SMBActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_linux.SMBActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_linux.SMBActiveOnHost")
	return false, errors.UnsupportedError("SMBActiveOnHost is not supported for linux")
}
