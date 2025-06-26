package gateway

import (
	"net"

	"github.com/sagernet/netlink"
)

func setTunAddr(ifname, localAddr, remoteAddr string) error {
	ifce, err := netlink.LinkByName(ifname)
	if err != nil {
		return err
	}
	netlink.LinkSetMTU(ifce, 1375)
	netlink.LinkSetTxQLen(ifce, 100)
	netlink.LinkSetUp(ifce)

	ln, err := netlink.ParseIPNet(localAddr)
	if err != nil {
		return err
	}
	ln.Mask = net.CIDRMask(32, 32)
	rn, err := netlink.ParseIPNet(remoteAddr)
	if err != nil {
		return err
	}
	rn.Mask = net.CIDRMask(32, 32)

	addr := &netlink.Addr{
		IPNet: ln,
		Peer:  rn,
	}
	return netlink.AddrAdd(ifce, addr)
}

func addTunAddr(ifname, localAddr, remoteAddr string) error {
	ifce, err := netlink.LinkByName(ifname)
	if err != nil {
		return err
	}

	ln, err := netlink.ParseIPNet(localAddr)
	if err != nil {
		return err
	}
	ln.Mask = net.CIDRMask(32, 32)
	rn, err := netlink.ParseIPNet(remoteAddr)
	if err != nil {
		return err
	}
	rn.Mask = net.CIDRMask(32, 32)

	addr := &netlink.Addr{
		IPNet: ln,
		Peer:  rn,
	}
	return netlink.AddrAdd(ifce, addr)
}

func addRoute(dst, gw string) error {
	_, networkid, err := net.ParseCIDR(dst)
	if err != nil {
		return err
	}
	ipGW, _, err := net.ParseCIDR(gw)
	if err != nil {
		return err
	}
	route := &netlink.Route{
		Dst: networkid,
		Gw:  ipGW,
	}
	return netlink.RouteAdd(route)
}

func delRoute(dst, gw string) error {
	_, networkid, err := net.ParseCIDR(dst)
	if err != nil {
		return err
	}
	route := &netlink.Route{
		Dst: networkid,
	}
	return netlink.RouteDel(route)
}
