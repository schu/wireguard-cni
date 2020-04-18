# wireguard-cni

Status: alpha, work in progress

wireguard-cni is a CNI plugin for [WireGuard](https://www.wireguard.com/).

## Installation

Configure the apiserver endpoint that `wg-cni` should use. Example:

```
kubectl -n kube-system create configmap wg-cni-env --from-literal=KUBERNETES_APISERVER_ENDPOINT=https://10.76.188.104:6443
```

Install wg-cni and its kubeconfig file on all nodes in the cluster:

```
kubectl apply -f manifests/wg-cni.yml
```

## Usage

The current prototype can be used as a chained CNI plugin, see the examples
below.

The WireGuard interface configuration must be provided through [CNI network configuration](https://github.com/containernetworking/cni/blob/master/SPEC.md#network-configuration)
for the moment. This will change soon and configuration will be stored
with the Kubernetes apiserver.

Example wg-cni config section:

```
{
  "type": "wg-cni",
  "address": "10.13.13.210/24",
  "privateKey": "AAev16ZVYhmCQliIYKXMje1zObRp6TmET0KiUx7MJXc=",
  "peers": [
    {
      "endpoint": "1.2.3.4:51820",
      "publicKey": "+gXCSfkib2xFMeebKXIYBVZxV/Vh2mbi1dJeHCCjQmg=",
      "allowedIPs": [
        "10.13.13.0/24"
      ],
      "persistentKeepalive": "25s"
    }
  ]
}
```

### Example: chained plugin with flannel

Edit the `kube-flannel-cfg` configmap and add `wg-cni` as a chained
plugin. Deploy new flannel pods for the configuration to be written.
To do that, you can delete the currently running flannel pods with
`kubectl -n kube-system delete pods -l app=flannel`.

Edit the configmap:

```
kubectl -n kube-system edit configmap kube-flannel-cfg
```

Example kube-flannel-cfg configmap:

```
kind: ConfigMap
apiVersion: v1
metadata:
  name: kube-flannel-cfg
  namespace: kube-system
  labels:
    tier: node
    app: flannel
data:
  cni-conf.json: |
    {
      "name": "cbr0",
      "plugins": [
        {
          "type": "flannel",
          "delegate": {
            "hairpinMode": true,
            "isDefaultGateway": true
          }
        },
        {
          "type": "portmap",
          "capabilities": {
            "portMappings": true
          }
        },
        {
          "type": "wg-cni",
          "address": "10.13.13.210/24",
          "privateKey": "AAev16ZVYhmCQliIYKXMje1zObRp6TmET0KiUx7MJXc=",
          "peers": [
            {
              "endpoint": "1.2.3.4:51820",
              "publicKey": "+gXCSfkib2xFMeebKXIYBVZxV/Vh2mbi1dJeHCCjQmg=",
              "allowedIPs": [
                "10.13.13.0/24"
              ],
              "persistentKeepalive": "25s"
            }
          ]
        }
      ]
    }
  net-conf.json: |
    {
      "Network": "10.244.0.0/16",
      "Backend": {
        "Type": "vxlan"
      }
    }
```

## Roadmap / Todo

* [x] Switch to https://github.com/mdlayher/wireguardctrl for netlink
* [ ] Provide a container and manifest to install the wg-cni plugin binary
  on all nodes in a cluster
* [ ] Allow dynamic configuration through Kubernetes resources
* [ ] Allow wireguard-cni to be used in standalone and chained mode?
