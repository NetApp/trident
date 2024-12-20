// Copyright 2024 NetApp, Inc. All Rights Reserved.

package osutils

import "github.com/vishvananda/netlink"

var netLink NetLink = NewNetLinkClient()

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

func (n *NetLinkClient) RouteListFiltered(family int, filter *netlink.Route, filterMask uint64) ([]netlink.Route, error) {
	return netlink.RouteListFiltered(family, filter, filterMask)
}

func (n *NetLinkClient) LinkByIndex(index int) (netlink.Link, error) {
	return netlink.LinkByIndex(index)
}
