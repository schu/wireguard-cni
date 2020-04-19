module github.com/schu/wireguard-cni

go 1.14

require (
	github.com/containernetworking/cni v0.7.1
	github.com/vishvananda/netlink v1.1.0
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df
	golang.org/x/crypto v0.0.0-20200414173820-0848c9571904 // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e // indirect
	golang.org/x/sys v0.0.0-20200413165638-669c56c373c4
	golang.zx2c4.com/wireguard v0.0.20200320 // indirect
	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20200324154536-ceff61240acf
	k8s.io/apimachinery v0.17.5 // indirect
	k8s.io/client-go v0.17.0
)
