package netlink

import (
	"github.com/vishvananda/netlink"
)

type Wireguard struct {
	netlink.LinkAttrs
}

func (wg *Wireguard) Attrs() *netlink.LinkAttrs {
	return &wg.LinkAttrs
}

func (wg *Wireguard) Type() string {
	return "wireguard"
}
