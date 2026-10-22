// Copyright 2026 NetApp, Inc. All Rights Reserved.

package osutils

import "github.com/vishvananda/netlink"

//go:generate mockgen -destination=../../mocks/mock_utils/mock_osutils/mock_netlink.go github.com/netapp/trident/utils/osutils NetLink

var netLink NetLink = NewNetLinkClient()

// NetLink dumps may return netlink.ErrDumpInterrupted together with the data read, when the table
// changed during the dump. Callers decide whether to use that data.
type NetLink interface {
	LinkList() ([]netlink.Link, error)
	AddrList(link netlink.Link, family int) ([]netlink.Addr, error)
	RouteListFiltered(family int, filter *netlink.Route, filterMask uint64) ([]netlink.Route, error)
	LinkByIndex(index int) (netlink.Link, error)
}

type NetLinkClient struct{}

func NewNetLinkClient() *NetLinkClient {
	return &NetLinkClient{}
}

func (n *NetLinkClient) LinkList() ([]netlink.Link, error) {
	return netlink.LinkList()
}

func (n *NetLinkClient) AddrList(link netlink.Link, family int) ([]netlink.Addr, error) {
	return netlink.AddrList(link, family)
}

// RouteListFiltered uses netlink.RouteListFilteredIter because netlink.RouteListFiltered discards the
// routes read when the dump is interrupted.
func (n *NetLinkClient) RouteListFiltered(family int, filter *netlink.Route, filterMask uint64) ([]netlink.Route, error) {
	var routes []netlink.Route
	err := netlink.RouteListFilteredIter(family, filter, filterMask, func(route netlink.Route) bool {
		routes = append(routes, route)
		return true
	})
	return routes, err
}

func (n *NetLinkClient) LinkByIndex(index int) (netlink.Link, error) {
	return netlink.LinkByIndex(index)
}
