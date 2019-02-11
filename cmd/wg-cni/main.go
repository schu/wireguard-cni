// Copyright 2019 Michael Schubert <schu@schu.io>
// Copyright 2017 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This is a sample chained plugin that supports multiple CNI versions. It
// parses prevResult according to the cniVersion
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	wgnetlink "github.com/schu/wireguard-cni/pkg/netlink"
)

// PluginConf is whatever you expect your configuration json to be. This is whatever
// is passed in on stdin. Your plugin may wish to expose its functionality via
// runtime args, see CONVENTIONS.md in the CNI spec.
type PluginConf struct {
	types.NetConf // You may wish to not nest this type
	RuntimeConfig *struct {
		SampleConfig map[string]interface{} `json:"sample"`
	} `json:"runtimeConfig"`

	// This is the previous result, when called in the context of a chained
	// plugin. Because this plugin supports multiple versions, we'll have to
	// parse this in two passes. If your plugin is not chained, this can be
	// removed (though you may wish to error if a non-chainable plugin is
	// chained.
	// If you need to modify the result before returning it, you will need
	// to actually convert it to a concrete versioned struct.
	RawPrevResult *map[string]interface{} `json:"prevResult"`
	PrevResult    *current.Result         `json:"-"`

	// Add plugin-specifc flags here
	Address    string `json:"address"`
	PrivateKey string `json:"privateKey"`
	Peers      []struct {
		Endpoint            string   `json:"endpoint"`
		PublicKey           string   `json:"endpointPublicKey"`
		PersistentKeepalive int      `json:"persistentKeepalive"`
		AllowedIPs          []string `json:"allowedIPs"`
	} `json:"peers"`
}

// parseConfig parses the supplied configuration (and prevResult) from stdin.
func parseConfig(stdin []byte) (*PluginConf, error) {
	conf := PluginConf{}

	if err := json.Unmarshal(stdin, &conf); err != nil {
		return nil, fmt.Errorf("failed to parse network configuration: %v", err)
	}

	// Parse previous result. Remove this if your plugin is not chained.
	if conf.RawPrevResult != nil {
		resultBytes, err := json.Marshal(conf.RawPrevResult)
		if err != nil {
			return nil, fmt.Errorf("could not serialize prevResult: %v", err)
		}
		res, err := version.NewResult(conf.CNIVersion, resultBytes)
		if err != nil {
			return nil, fmt.Errorf("could not parse prevResult: %v", err)
		}
		conf.RawPrevResult = nil
		conf.PrevResult, err = current.NewResultFromResult(res)
		if err != nil {
			return nil, fmt.Errorf("could not convert result to current version: %v", err)
		}
	}
	// End previous result parsing

	// Do any validation here
	if len(conf.Peers) == 0 {
		return nil, fmt.Errorf("no peer specified")
	}
	if conf.Address == "" {
		return nil, fmt.Errorf("address must be specified")
	}
	if conf.PrivateKey == "" {
		return nil, fmt.Errorf("privateKey must be specified")
	}

	return &conf, nil
}

// cmdAdd is called for ADD requests
func cmdAdd(args *skel.CmdArgs) error {
	conf, err := parseConfig(args.StdinData)
	if err != nil {
		return err
	}

	if conf.PrevResult == nil {
		return fmt.Errorf("must be called as chained plugin")
	}

	devname := "wg0"

	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = devname

	wg := &wgnetlink.Wireguard{
		LinkAttrs: linkAttrs,
	}

	if err := netlink.LinkAdd(wg); err != nil {
		fmt.Errorf("could net add link: %v", err)
	}

	wgnetlinkFam, err := netlink.GenlFamilyGet(wgnetlink.WG_GENL_NAME)
	if err != nil {
		return fmt.Errorf("could not get wireguard netlink fam: %v", err)
	}

	netlinkRequest := nl.NewNetlinkRequest(int(wgnetlinkFam.ID), unix.NLM_F_ACK)

	nlMsg := &nl.Genlmsg{
		Command: wgnetlink.WG_CMD_SET_DEVICE,
		Version: wgnetlink.WG_GENL_VERSION,
	}
	netlinkRequest.AddData(nlMsg)

	b := make([]byte, len(devname)+1)
	copy(b, devname)
	netlinkRequest.AddData(nl.NewRtAttr(wgnetlink.WGDEVICE_A_IFNAME, b))

	keyBytes, err := base64.StdEncoding.DecodeString(conf.PrivateKey)
	if err != nil {
		return fmt.Errorf("could not base64 decode key: %v", err)
	}
	netlinkRequest.AddData(nl.NewRtAttr(wgnetlink.WGDEVICE_A_PRIVATE_KEY, keyBytes))

	peers := nl.NewRtAttr(wgnetlink.WGDEVICE_A_PEERS, nil)

	for _, peer := range conf.Peers {
		peerNest := peers.AddRtAttr(unix.NLA_F_NESTED, nil)

		keyBytes, err = base64.StdEncoding.DecodeString(peer.PublicKey)
		if err != nil {
			return fmt.Errorf("could not base64 decode key: %v", err)
		}
		peerNest.AddRtAttr(wgnetlink.WGPEER_A_PUBLIC_KEY, keyBytes)

		// TODO(schu): handle IPv6 addresses

		parts := strings.SplitN(peer.Endpoint, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("endpoint %q not in expected format '<host>:<port>'", peer.Endpoint)
		}

		addrs, err := net.LookupHost(parts[0])
		if err != nil {
			return fmt.Errorf("could not lookup host %q: %v", parts[0], err)
		}

		ip := net.ParseIP(addrs[0])
		if ip == nil {
			return fmt.Errorf("could not parse IP %q: %v", ip, err)
		}

		portNumber, err := net.LookupPort("", parts[1])
		if err != nil {
			return fmt.Errorf("could not lookup port %q: %v", parts[1], err)
		}

		var portBytes [2]byte
		binary.LittleEndian.PutUint16(portBytes[:], uint16(portNumber))

		if ip.To4() != nil {
			sa := unix.RawSockaddrInet4{
				Family: unix.AF_INET,
				Port:   binary.BigEndian.Uint16(portBytes[:]),
			}
			copy(sa.Addr[:], ip.To4())
			var buf bytes.Buffer
			if err := binary.Write(&buf, binary.LittleEndian, sa); err != nil {
				return fmt.Errorf("could not binary encode sockaddr: %v", err)
			}
			peerNest.AddRtAttr(wgnetlink.WGPEER_A_ENDPOINT, buf.Bytes())
		} else {
			panic("IPv6 support not implemented yet")
		}

		if peer.PersistentKeepalive != 0 {
			var keepaliveBytes [2]byte
			binary.LittleEndian.PutUint16(keepaliveBytes[:], uint16(peer.PersistentKeepalive))
			peerNest.AddRtAttr(wgnetlink.WGPEER_A_PERSISTENT_KEEPALIVE_INTERVAL, keepaliveBytes[:])
		}

		allowedIPs := peerNest.AddRtAttr(wgnetlink.WGPEER_A_ALLOWEDIPS, nil)
		for _, allowedIP := range peer.AllowedIPs {
			allowed := allowedIPs.AddRtAttr(unix.NLA_F_NESTED, nil)

			ip, ipNet, err := net.ParseCIDR(allowedIP)
			if err != nil {
				return fmt.Errorf("could not parse CIDR %q: %v", allowedIP, err)
			}

			if ip.To4() != nil {
				var familyBytes [2]byte
				binary.LittleEndian.PutUint16(familyBytes[:], uint16(unix.AF_INET))
				allowed.AddRtAttr(wgnetlink.WGALLOWEDIP_A_FAMILY, familyBytes[:])

				buf := bytes.NewBuffer(make([]byte, 0, 4))
				if err := binary.Write(buf, binary.BigEndian, ip.To4()); err != nil {
					return fmt.Errorf("could not binary encode '%v': %v", ip, err)
				}
				allowed.AddRtAttr(wgnetlink.WGALLOWEDIP_A_IPADDR, buf.Bytes())

				ones, _ := ipNet.Mask.Size()
				allowed.AddRtAttr(wgnetlink.WGALLOWEDIP_A_CIDR_MASK, []byte{uint8(ones)})
			} else {
				panic("IPv6 support not implemented yet")
			}
		}
	}

	netlinkRequest.AddData(peers)

	_, err = netlinkRequest.Execute(unix.NETLINK_GENERIC, 0)
	if err != nil {
		return fmt.Errorf("could not execute netlink request: %v", err)
	}

	netnsHandle, err := netns.GetFromPath(args.Netns)
	if err != nil {
		return fmt.Errorf("could not get container net ns handle: %v", err)
	}

	if err := netlink.LinkSetNsFd(wg, int(netnsHandle)); err != nil {
		return fmt.Errorf("could not put link into ns: %v", err)
	}

	ip, ipNet, err := net.ParseCIDR(conf.Address)
	if err != nil {
		return fmt.Errorf("could not parse cidr %q: %v", conf.Address, err)
	}

	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   ip,
			Mask: ipNet.Mask,
		},
	}

	netlinkNsHandle, err := netlink.NewHandleAt(netnsHandle)
	if err != nil {
		return fmt.Errorf("could not get ns netlink handle: %v", err)
	}

	if err := netlinkNsHandle.AddrAdd(wg, addr); err != nil {
		return fmt.Errorf("could not add address: %v", err)
	}

	if err := netlinkNsHandle.LinkSetUp(wg); err != nil {
		return fmt.Errorf("could not set link up: %v", err)
	}

	// Pass through the result for the next plugin
	return types.PrintResult(conf.PrevResult, conf.CNIVersion)
}

// cmdDel is called for DELETE requests
func cmdDel(args *skel.CmdArgs) error {
	conf, err := parseConfig(args.StdinData)
	if err != nil {
		return err
	}
	_ = conf

	// Do your delete here

	return nil
}

func main() {
	// TODO: implement plugin version
	skel.PluginMain(cmdAdd, cmdGet, cmdDel, version.All, "TODO")
}

func cmdGet(args *skel.CmdArgs) error {
	// TODO: implement
	return fmt.Errorf("not implemented")
}
