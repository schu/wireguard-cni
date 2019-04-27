# wireguard-cni

Status: alpha, work in progress

wireguard-cni is a CNI plugin for [WireGuard](https://www.wireguard.com/).

## Usage

The current prototype can be used as a chained CNI plugin, the
configuration must be provided through [CNI network configuration](https://github.com/containernetworking/cni/blob/master/SPEC.md#network-configuration)
for the moment.

### Example: chained plugin with flannel

Edit the `kube-flannel-cfg` configmap and add `wg-cni` as a chained
plugin. Make sure `wg-cni` is available in the CNI path, `/opt/cni/bin`.
Deploy new flannel pods for the configuration to be written.

```
kubectl -n kube-system edit configmap kube-flannel-cfg
```

Example wg-cni config section:

```
{
  "type": "wg-cni",
  "address": "10.13.13.210/24",
  "privateKey": "AAev16ZVYhmCQliIYKXMje1zObRp6TmET0KiUx7MJXc=",
  "peers": [
    {
      "endpoint": "1.2.3.4:51820",
      "endpointPublicKey": "+gXCSfkib2xFMeebKXIYBVZxV/Vh2mbi1dJeHCCjQmg=",
      "allowedIPs": [
        "10.13.13.0/24"
      ],
      "persistentKeepalive": 25
    }
  ]
}
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
              "endpointPublicKey": "+gXCSfkib2xFMeebKXIYBVZxV/Vh2mbi1dJeHCCjQmg=",
              "allowedIPs": [
                "10.13.13.0/24"
              ],
              "persistentKeepalive": 25
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

* Provide a container and manifest to install the wg-cni plugin binary
  on all nodes in a cluster
* Allow dynamic configuration through Kubernetes resources
* Allow wireguard-cni to be used in standalone and chained mode
