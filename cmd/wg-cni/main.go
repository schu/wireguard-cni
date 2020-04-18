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
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

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
		PublicKey           string   `json:"publicKey"`
		PersistentKeepalive string   `json:"persistentKeepalive"`
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

	privateKey, err := wgtypes.ParseKey(conf.PrivateKey)
	if err != nil {
		return fmt.Errorf("could not parse private key: %v", err)
	}

	var peers []wgtypes.PeerConfig
	for _, peerConf := range conf.Peers {
		var peer wgtypes.PeerConfig

		peer.PublicKey, err = wgtypes.ParseKey(peerConf.PublicKey)
		if err != nil {
			return fmt.Errorf("could not parse public key: %v", err)
		}

		keepaliveInterval, err := time.ParseDuration(peerConf.PersistentKeepalive)
		if err != nil {
			return fmt.Errorf("could not parse keepalive duration string %q: %v", peerConf.PersistentKeepalive, err)
		}
		peer.PersistentKeepaliveInterval = &keepaliveInterval

		peer.Endpoint, err = net.ResolveUDPAddr("udp", peerConf.Endpoint)
		if err != nil {
			return fmt.Errorf("could not parse endpoint %q: %v", peerConf.Endpoint, err)
		}

		for _, allowedIP := range peerConf.AllowedIPs {
			_, ipnet, err := net.ParseCIDR(allowedIP)
			if err != nil {
				return fmt.Errorf("could not parse CIDR %q: %v", allowedIP, err)
			}

			peer.AllowedIPs = append(peer.AllowedIPs, *ipnet)
		}

		peers = append(peers, peer)
	}

	wgConfig := wgtypes.Config{
		PrivateKey: &privateKey,
		Peers:      peers,
	}

	netnsHandle, err := netns.GetFromPath(args.Netns)
	if err != nil {
		return fmt.Errorf("could not get container net ns handle: %v", err)
	}

	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = "wg0"

	wgLink := &wgnetlink.Wireguard{
		LinkAttrs: linkAttrs,
	}
	if err := netlink.LinkAdd(wgLink); err != nil {
		return fmt.Errorf("could not create wg network interface: %v", err)
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

	if err := netlink.AddrAdd(wgLink, addr); err != nil {
		return fmt.Errorf("could not add address: %v", err)
	}

	wgClient, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("could not get wgctrl client: %v", err)
	}
	defer wgClient.Close()

	if err := wgClient.ConfigureDevice("wg0", wgConfig); err != nil {
		return fmt.Errorf("could not configure device wg0: %v", err)
	}

	if err := netlink.LinkSetNsFd(wgLink, (int)(netnsHandle)); err != nil {
		return fmt.Errorf("could not move network interface into container's net namespace: %v", err)
	}

	netnsNetlinkHandle, err := netlink.NewHandleAt(netnsHandle)
	if err != nil {
		return fmt.Errorf("could not get container net ns netlink handle: %v", err)
	}

	if err := netnsNetlinkHandle.LinkSetUp(wgLink); err != nil {
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
