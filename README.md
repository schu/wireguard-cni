# wireguard-cni

Status: alpha, use with caution

wireguard-cni is a CNI plugin for [WireGuard](https://www.wireguard.com/).

## Installation

Configure the apiserver endpoint that `wg-cni` should use to query
configuration:

```
kubectl -n kube-system create configmap wg-cni-env --from-literal=KUBERNETES_APISERVER_ENDPOINT=https://<IP_ADDRESS>:<PORT>
```

Install wg-cni and its kubeconfig file on all nodes in the cluster:

```
kubectl apply -f manifests/wg-cni.yml
```

wg-cni is set up as a chained CNI plugin. This means you have
to configure wg-cni as an additional CNI plugin in your configuration.

To do this, add wg-cni to the list of `plugins`:

```
{
  "type": "wg-cni",
  "kubeConfigPath": "/etc/kubernetes/wg-cni.kubeconfig"
}
```

Note that the `wg-cni.kubeconfig` file gets created automatically by
wg-cni during installation.

wg-cni should now be ready and running - you can check with:

```
kubectl -n kube-system get pods -l k8s-app=wg-cni
```

### Example: chained plugin configuration with flannel

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
          "kubeConfigPath": "/etc/kubernetes/wg-cni.kubeconfig"
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

## Usage

To add a WireGuard connection to a pod, two things are required:

1. a secret with the configuration and
1. an annotation in the pod's metadata to signal wg-cni that it should
   configuare a link for it and where the configuration can be found.

Note: pods that are not annotated are skipped by wg-cni.

Create a file `config.json` with the following structure:

```
{
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

Create a secret from the file:

```
kubectl create secret generic wgcni-demo --from-file ./config.json
```

Start a new pod with a corresponding `wgcni.schu.io/configsecret` annotation:

```
apiVersion: v1
kind: Pod
metadata:
  name: test
  annotations:
    wgcni.schu.io/configsecret: "wgcni-demo"
spec:
  ...
```

The value `wgcni-demo` is the name of the secret in the pod's namespace.

Once running, the pod should have a `wg<suffix>` interface that is
configured according to your configuration.

If an error occurs, you should find a message in the events:

```
kubectl get events
```

## Roadmap / Todo

* [x] Switch to https://github.com/WireGuard/wgctrl-go for netlink
* [x] Provide a container and manifest to install the wg-cni plugin binary
  and required configuration on all nodes in a cluster
* [ ] Allow dynamic configuration through Kubernetes resources
* [ ] Consider allowing wg-cni to be used in standalone and chained mode
