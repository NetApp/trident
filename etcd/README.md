## Overview
The external etcd cluster setup is based on the information found in
[Cluster Spec](https://github.com/coreos/etcd-operator/blob/master/doc/user/spec_examples.md),
[Cluster TLS Guide](https://github.com/coreos/etcd-operator/blob/master/doc/user/cluster_tls.md),
[etcd Client Service](https://github.com/coreos/etcd-operator/blob/master/doc/user/client_service.md),
[Operator RBAC Setup](https://github.com/coreos/etcd-operator/blob/master/doc/user/rbac.md),
and [Generating Self-signed Certificates](https://coreos.com/os/docs/latest/generate-self-signed-certificates.html).


## Install etcd cluster
To install the etcd cluster in namespace `trident`, run the following:
```bash
./install_etcd.sh -n trident
```

## Uninstall etcd cluster
To uninstall the etcd cluster in namespace `trident`, run the following:
```bash
./uninstall_etcd.sh -n trident
```

## Test
```bash
kubectl create -f kubernetes-yaml/etcdctl-pod.yaml
kubectl exec -it etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt member list -w table
kubectl exec -it etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt put foo bar
kubectl exec -it etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt get foo
kubectl exec -it etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt del foo
kubectl delete -f kubernetes-yaml/etcdctl-pod.yaml
```

