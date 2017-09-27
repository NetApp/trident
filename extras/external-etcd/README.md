# Advanced etcd Workflows with Trident

* [Upgrade from etcdv2 to etcdv3](#upgrade-from-etcdv2-to-etcdv3)
* [External etcd Cluster Setup](#external-etcd-cluster-setup)
  * [Install etcd Cluster](#install-etcd-cluster)
  * [Test etcd Cluster](#test-etcd-cluster)
  * [Uninstall etcd Cluster](#uninstall-etcd-cluster)
* [Configuring Trident with an External etcd Cluster](#configuring-trident-with-an-external-etcd-cluster)
* [etcd-copy Utility](#etcd-copy-utility)

## Upgrade from etcdv2 to etcdv3

The default configuration to use etcd with Trident is to deploy the etcd
container as part of the Trident pod. This etcd instance is backed by a NetApp
volume created by Trident launcher and because all communication between the
Trident and etcd containers are local to the host, there is no need to encrypt
the communication between the two containers.

Prior to release v18.01, Trident used the etcdv2 API to communicate with the
local etcd container. Starting from v18.01, Trident supports both etcdv2 and
etcdv3 APIs, with the default configuration being etcdv3 (see
`setup/trident-deployment.yaml` in a v18.01 or later installer bundle) as
etcdv3 offers features like snapshot and TLS that etcdv2 lacks.

It is important to note that the etcdv2 data is not accessible via the etcdv3 API.
Therefore, there might be a need to migrate the preexisting Trident etcdv2 data
to etcdv3. The good news is that by following the
[Trident update procedure](https://github.com/netapp/trident#updating-trident)
to upgrade to v18.01 or a later release, Trident automatically performs the
etcdv2 to etcdv3 state migration as part of its bootstrapping process.

The [etcd-copy Utility](#etcd-copy-utility) section demonstrates that it is
also possible to migrate data from a local etcdv2 endpoint to a remote etcdv3
endpoint.

## External etcd Cluster Setup

This section captures the instructions to set up an external etcd cluster using
[etcd operator](https://github.com/coreos/etcd-operator) with RBAC and TLS. The
purpose of this section is not necessarily to set up a production etcd
cluster. Rather, it provides a reference to show how Trident operates with an
external etcd cluster. These instructions are generic enough to be used
for applications other than Trident.

These instructions are based on the information found in
[Cluster Spec](https://github.com/coreos/etcd-operator/blob/master/doc/user/spec_examples.md),
[Cluster TLS Guide](https://github.com/coreos/etcd-operator/blob/master/doc/user/cluster_tls.md),
[etcd Client Service](https://github.com/coreos/etcd-operator/blob/master/doc/user/client_service.md),
[Operator RBAC Setup](https://github.com/coreos/etcd-operator/blob/master/doc/user/rbac.md),
and [Generating Self-signed Certificates](https://coreos.com/os/docs/latest/generate-self-signed-certificates.html).


### Install etcd Cluster

[Trident installer bundle](https://github.com/NetApp/trident/releases) includes
a set of scripts and configuration files to set up an external cluster. These
files can be found under `trident-installer/extras/external-etcd/`.

To install the etcd cluster in namespace `trident`, run the following command:
```bash
trident-installer$ cd extras/external-etcd/
trident-installer/extras/external-etcd$ ./install_etcd.sh -n trident
Installer assumes you have deployed Kubernetes. If this is an OpenShift deployment, make sure 'oc' is in the $PATH.
cfssl and cfssljson have already been downloaded.
serviceaccount "etcd-operator" created
clusterrole "etcd-operator" created
clusterrolebinding "etcd-operator" created
deployment "etcd-operator" created
secret "etcd-client-tls" created
secret "etcd-server-tls" created
secret "etcd-peer-tls" created
etcdcluster "trident-etcd" created
```

The above script creates a few Kubernetes objects, including the following:
```bash
trident-installer/extras/external-etcd$ kubectl get pod
NAME                             READY     STATUS    RESTARTS   AGE
etcd-operator-3986959281-m048l   1/1       Running   0          1m
trident-etcd-0000                1/1       Running   0          20s
trident-installer/extras/external-etcd$ kubectl get service
NAME                  CLUSTER-IP    EXTERNAL-IP   PORT(S)             AGE
trident-etcd          None          <none>        2379/TCP,2380/TCP   1m
trident-etcd-client   10.99.21.44   <none>        2379/TCP            1m
trident-installer/extras/external-etcd$ kubectl get secret
NAME                        TYPE                                  DATA      AGE
default-token-ql7s3         kubernetes.io/service-account-token   3         72d
etcd-client-tls             Opaque                                3         1m
etcd-operator-token-nsh2n   kubernetes.io/service-account-token   3         1m
etcd-peer-tls               Opaque                                3         1m
etcd-server-tls             Opaque                                3         1m
```

The Kubernetes Secrets shown above are constructed using the CA, certificates,
and private keys generated by the installer script:
```bash
trident-installer/extras/external-etcd$ ls certs/
ca-config.json  ca-key.pem  client-csr.json  gen-ca.sh      gen-server.sh  peer-key.pem  server-csr.json
ca.csr          ca.pem      client-key.pem   gen-client.sh  peer.csr       peer.pem      server-key.pem
ca-csr.json     client.csr  client.pem       gen-peer.sh    peer-csr.json  server.csr    server.pem
```
For more information about the Secrets used by the operator, please see
[Cluster TLS Guide](https://github.com/coreos/etcd-operator/blob/master/doc/user/cluster_tls.md)
and [Generating Self-signed Certificates](https://coreos.com/os/docs/latest/generate-self-signed-certificates.html).


### Testing etcd Cluster

To verify the cluster we brought up in the previous step is working properly,
we can run the following commands:
```bash
trident-installer/extras/external-etcd$ kubectl create -f kubernetes-yaml/etcdctl-pod.yaml
trident-installer/extras/external-etcd$ kubectl exec etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt member list -w table
trident-installer/extras/external-etcd$ kubectl exec etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt put foo bar
trident-installer/extras/external-etcd$ kubectl exec etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt get foo
trident-installer/extras/external-etcd$ kubectl exec etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt del foo
trident-installer/extras/external-etcd$ kubectl delete -f kubernetes-yaml/etcdctl-pod.yaml
```

The above commands invoke the `etcdctl` binary inside the etcdctl pod to
interact with the etcd cluster. Please see `kubernetes-yaml/etcdctl-pod.yaml`
to understand how client credentials are supplied using the `etcd-client-tls`
Secret to the etcdctl pod. It is important to note that etcd operator requires
a working kube-dns pod as it relies on a Kubernetes Service to communicate with
the etcd cluster.

### Uninstall etcd Cluster

To uninstall the etcd cluster in namespace `trident`, run the following:
```bash
trident-installer/extras/external-etcd$ ./uninstall_etcd.sh -n trident
```

## Configuring Trident with an External etcd Cluster

While the default configuration is to deploy etcd as part of the Trident pod,
it is possible to deploy Trident with an external etcd cluster. For this
configuration, we recommend configuring Trident with an etcdv3 client and TLS.

The instructions in this section should cover the case where you are deploying
Trident for the first time with an external etcd cluster or the case where you
already have a Trident deployment that uses the local etcd container and you
want to move to an external etcd cluster without losing any state maintained
by Trident.


**Step 1:** Bring down Trident

Assuming you have a running v18.01 or later instance of Trident that uses the
etcdv3 API (as described in [Upgrade from etcdv2 to etcdv3](#upgrade-from-etcdv2-to-etcdv3)),
you first need to bring down Trident so that no new state is written to etcd.

In this example, our Trident deployment manages one backend and one storage
class:

```bash
trident-installer$ ./tridentctl get backend
+--------------------------+----------------+--------+---------+
|           NAME           | STORAGE DRIVER | ONLINE | VOLUMES |
+--------------------------+----------------+--------+---------+
| solidfire_10.250.118.144 | solidfire-san  | true   |       0 |
+--------------------------+----------------+--------+---------+
trident-installer$ ./tridentctl get storageclass
+-----------+
|   NAME    |
+-----------+
| solidfire |
+-----------+
trident-installer$ ./uninstall_trident.sh -n trident
```

**Step 2:** Copy scripts and the deployment file

As part of this step, we copy three files to the root of the Trident installer
bundle directory:
```bash
trident-installer$ ls extras/external-etcd/trident/
etcdcopy-job.yaml  install_trident_external_etcd.sh  trident-deployment-external-etcd.yaml
trident-installer$ cp extras/external-etcd/trident/* .
```
`etcdcopy-job.yaml` contains the definition for an application that copies etcd
data from one endpoint to another. If you are setting up a new Trident instance
with an external cluster for the first time, you can ignore this file and Step 3.

`trident-deployment-external-etcd.yaml` contains the Deployment definition for
the Trident instance that is configured to work with an external cluster. This
file is used by `install_trident_external_etcd.sh`.

As the contents of `trident-deployment-external-etcd.yaml` and `install_trident_external_etcd.sh`
suggest, the install process is much simpler with an external etcd cluster as
there is no need to run Trident launcher to provision a volume, PVC, and PV for
the Trident deployment.


**Step 3:** Copy etcd data from the local endpoint to the remote endpoint

Once you make sure that Trident is not running as instructed in Step 1,
configure `etcdcopy-job.yaml` with the information about the destination
cluster. In this example, we are copying data from the local etcd instance
used by the terminated Trident deployment to the remote cluster set up in
[Install etcd Cluster](#install-etcd-cluster).

`etcdcopy-job.yaml` makes reference to the Kubernetes Secret `etcd-client-tls`,
which was created as part of [Install etcd Cluster](#install-etcd-cluster). If
you already have a production etcd cluster set up, you need to generate the
Secret yourself and adjust the parameters taken by the `etcd-copy` container
in `etcdcopy-job.yaml`. For example, `etcdcopy-job.yaml` is based on a Secret
that was created using the following command:
```bash
trident-installer/extras/external-etcd$ kubectl --namespace=trident create secret generic etcd-client-tls --from-file=etcd-client-ca.crt=./certs/ca.pem --from-file=etcd-client.crt=./certs/client.pem --from-file=etcd-client.key=./certs/client-key.pem
```
Based on how you set up your external cluster and how you name the files that
make up the Secret, you may have to modify `etcdv3_dest` arguments in
`etcdcopy-job.yaml`:
```
- -etcdv3_dest
- https://trident-etcd-client:2379
- -etcdv3_dest_cacert
- /root/certs/etcd-client-ca.crt
- -etcdv3_dest_cert
- /root/certs/etcd-client.crt
- -etcdv3_dest_key
- /root/certs/etcd-client.key
```
Once `etcdcopy-job.yaml`is configured properly, you can start migrating data
between the two etcd endpoints:

```bash
trident-installer$ kubectl create -f etcdcopy-job.yaml
job "etcd-copy" created
trident-installer$ kubectl get pod -aw
NAME                             READY     STATUS      RESTARTS   AGE
etcd-copy-fzhqm                  1/2       Completed   0          14s
etcd-operator-3986959281-782hx   1/1       Running     0          1d
etcdctl                          1/1       Running     0          1d
trident-etcd-0000                1/1       Running     0          1d
trident-installer$ kubectl logs etcd-copy-fzhqm -c etcd-copy
time="2017-11-03T14:36:35Z" level=debug msg="Read key from the source." key="/trident/v1/backend/solidfire_10.250.118.144" 
time="2017-11-03T14:36:35Z" level=debug msg="Wrote key to the destination." key="/trident/v1/backend/solidfire_10.250.118.144" 
time="2017-11-03T14:36:35Z" level=debug msg="Read key from the source." key="/trident/v1/storageclass/solidfire" 
time="2017-11-03T14:36:35Z" level=debug msg="Wrote key to the destination." key="/trident/v1/storageclass/solidfire"
trident-installer$ kubectl delete -f etcdcopy-job.yaml 
job "etcd-copy" deleted
```

The logs for `etcd-copy` suggests that this Job has successfully copied the
backend and the storage class that we saw in Step 1 to the remote etcd cluster.

**Step 4:** Install Trident with an external etcd cluster

Prior to running the install script, please adjust `trident-deployment-external-etcd.yaml`
to reflect your setup. More specifically, you may need to change the etcdv3
endpoint and Secret if you did not rely on the instructions on this page to
set up a production etcd cluster.

```bash
trident-installer$ ./install_trident_external_etcd.sh -n trident
trident-installer$ ./tridentctl get backend
+--------------------------+----------------+--------+---------+
|           NAME           | STORAGE DRIVER | ONLINE | VOLUMES |
+--------------------------+----------------+--------+---------+
| solidfire_10.250.118.144 | solidfire-san  | true   |       0 |
+--------------------------+----------------+--------+---------+
trident-installer$ ./tridentctl get storageclass
+-----------+
|   NAME    |
+-----------+
| solidfire |
+-----------+
trident-installer$ ./uninstall_trident.sh
```
As the logs show, we have successfully transferred the state from the
local etcd endpoint to the remote etcd cluster that we set up earlier. 

## etcd-copy Utility

As demonstrated in Step 3 of 
[Configuring Trident with an External etcd Cluster](#configuring-trident-with-an-external-etcd-cluster),
we have written an application called `etcd-copy` to transfer data between two
etcd endpoints. `etcd-copy` binary is part of `netapp/trident` image. This
binary can also be found at `extras/external-etcd/bin/` in the installer
bundle.

For more information about `etcd-copy`, please run the following command:
```bash
trident-installer$ extras/external-etcd/bin/etcd-copy -h
```

