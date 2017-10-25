####
etcd
####

Trident's use of etcd
---------------------

Trident uses `etcd`_ to maintain state for the
:ref:`objects <Kubernetes and Trident objects>` that it manages.

.. _etcd: https://coreos.com/etcd/

By default, Trident deploys an etcd container as part of the Trident pod. This
is a single node etcd cluster managed by Trident that's backed by a highly
reliable volume from a NetApp storage system. This is perfectly acceptable
for production.

Using an external etcd cluster
------------------------------

In some cases there may already be a production etcd cluster available that
you would like Trident to use instead, or you would like to build one.

.. note::

  Kubernetes itself uses an etcd cluster for its objects. While it's technically
  possible to use that cluster to store Trident's state as well, it is highly
  discouraged by the Kubernetes community.

Beginning with Trident 18.01, this is a supported configuration provided that
the etcd cluster is using v3. It is also highly recommend that you encrypt
communication to the remote cluster with TLS.

The instructions in this section cover both the case where you are deploying
Trident for the first time with an external etcd cluster and the case where you
already have a Trident deployment that uses the local etcd container and you
want to move to an external etcd cluster without losing any state maintained
by Trident.

**Step 1:** Bring down Trident

First, make sure that you have started Trident successfully at least once with
version 18.01 or above. Previous versions of Trident used etcdv2, and Trident
needs to start once with a more recent version to automatically upgrade its
state to the etcdv3 format.

Now we need to bring down Trident so that no new state is written to etcd.
This is most easily done by running the uninstall script, which retains all
state by default.

The uninstall script is located in the `Trident installer bundle`_ that you
downloaded to install Trident.

.. code-block:: bash

  trident-installer$ ./uninstall_trident.sh -n <namespace>

**Step 2:** Copy scripts and the deployment file

As part of this step, we copy three files to the root of the Trident installer
bundle directory:

.. code-block:: bash

  trident-installer$ ls extras/external-etcd/trident/
  etcdcopy-job.yaml  install_trident_external_etcd.sh  trident-deployment-external-etcd.yaml
  trident-installer$ cp extras/external-etcd/trident/* .

``etcdcopy-job.yaml`` contains the definition for an application that copies
etcd data from one endpoint to another. If you are setting up a new Trident
instance with an external cluster for the first time, you can ignore this file
and Step 3.

``trident-deployment-external-etcd.yaml`` contains the Deployment definition for
the Trident instance that is configured to work with an external cluster. This
file is used by the ``install_trident_external_etcd.sh`` script.

As the contents of ``trident-deployment-external-etcd.yaml`` and
``install_trident_external_etcd.sh`` suggest, the install process is much
simpler with an external etcd cluster as there is no need to run Trident
launcher to provision a volume, PVC, and PV for the Trident deployment.

**Step 3:** Copy etcd data from the local endpoint to the remote endpoint

Once you make sure that Trident is not running as instructed in Step 1,
configure ``etcdcopy-job.yaml`` with the information about the destination
cluster. In this example, we are copying data from the local etcd instance
used by the terminated Trident deployment to the remote cluster.

``etcdcopy-job.yaml`` makes reference to the Kubernetes Secret
``etcd-client-tls``, which was created automatically if you installed the
sample etcd cluster. If you already have a production etcd cluster set up, you
need to generate the Secret yourself and adjust the parameters taken by the
``etcd-copy`` container in ``etcdcopy-job.yaml``.

For example, ``etcdcopy-job.yaml`` is based on a Secret that was created using
the following command:

.. code-block:: bash

  trident-installer/extras/external-etcd$ kubectl --namespace=trident create secret generic etcd-client-tls --from-file=etcd-client-ca.crt=./certs/ca.pem --from-file=etcd-client.crt=./certs/client.pem --from-file=etcd-client.key=./certs/client-key.pem

Based on how you set up your external cluster and how you name the files that
make up the Secret, you may have to modify ``etcdv3_dest`` arguments in
``etcdcopy-job.yaml`` as well:

.. code-block:: yaml

  - -etcdv3_dest
  - https://trident-etcd-client:2379
  - -etcdv3_dest_cacert
  - /root/certs/etcd-client-ca.crt
  - -etcdv3_dest_cert
  - /root/certs/etcd-client.crt
  - -etcdv3_dest_key
  - /root/certs/etcd-client.key

Once ``etcdcopy-job.yaml`` is configured properly, you can start migrating data
between the two etcd endpoints:

.. code-block:: console

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

The logs for `etcd-copy` should indicate that Job has successfully copied
Trident's state to the remote etcd cluster.

**Step 4:** Install Trident with an external etcd cluster

Prior to running the install script, please adjust
``trident-deployment-external-etcd.yaml`` to reflect your setup. More
specifically, you may need to change the etcdv3 endpoint and Secret if you did
not rely on the instructions on this page to set up your etcd cluster.

.. code-block:: bash

  trident-installer$ ./install_trident_external_etcd.sh -n trident

That's it! Trident is now up and running against an external etcd cluster. You
should now be able to run :ref:`tridentctl` and see all of the same
configuration you had before.

Building your own etcd cluster
------------------------------

We needed to be able to easily create etcd clusters with RBAC and TLS enabled
for testing purposes. We think that the tools we built to do that are also a
useful way to help others understand how to do that.

This provides a reference to show how Trident operates with an external etcd
cluster, and it should be generic enough to use for applications other than
Trident.

These instructions use the `etcd operator`_ and are based on the information
found in `Cluster Spec`_, `Cluster TLS Guide`_, `etcd Client Service`_,
`Operator RBAC Setup`_, and `Generating Self-signed Certificates`_.

.. _etcd operator: https://github.com/coreos/etcd-operator
.. _Cluster Spec: https://github.com/coreos/etcd-operator/blob/master/doc/user/spec_examples.md
.. _Cluster TLS Guide: https://github.com/coreos/etcd-operator/blob/master/doc/user/cluster_tls.md
.. _etcd Client Service: https://github.com/coreos/etcd-operator/blob/master/doc/user/client_service.md
.. _Operator RBAC Setup: https://github.com/coreos/etcd-operator/blob/master/doc/user/rbac.md
.. _Generating Self-signed Certificates: https://coreos.com/os/docs/latest/generate-self-signed-certificates.html

Installing
^^^^^^^^^^

The `Trident installer bundle`_ includes a set of scripts and configuration
files to set up an external cluster. These files can be found under
``trident-installer/extras/external-etcd/``.

.. _Trident installer bundle: https://github.com/NetApp/trident/releases

To install the etcd cluster in namespace ``trident``, run the following command:

.. code-block:: console

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

The above script creates a few Kubernetes objects, including the following:

.. code-block:: console

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

The Kubernetes Secrets shown above are constructed using the CA, certificates,
and private keys generated by the installer script:

.. code-block:: console

  trident-installer/extras/external-etcd$ ls certs/
  ca-config.json  ca-key.pem  client-csr.json  gen-ca.sh      gen-server.sh  peer-key.pem  server-csr.json
  ca.csr          ca.pem      client-key.pem   gen-client.sh  peer.csr       peer.pem      server-key.pem
  ca-csr.json     client.csr  client.pem       gen-peer.sh    peer-csr.json  server.csr    server.pem

For more information about the Secrets used by the operator, please see
`Cluster TLS Guide`_ and `Generating Self-signed Certificates`_.

Testing
^^^^^^^

To verify the cluster we brought up in the previous step is working properly,
we can run the following commands:

.. code-block:: bash

  trident-installer/extras/external-etcd$ kubectl create -f kubernetes-yaml/etcdctl-pod.yaml
  trident-installer/extras/external-etcd$ kubectl exec etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt member list -w table
  trident-installer/extras/external-etcd$ kubectl exec etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt put foo bar
  trident-installer/extras/external-etcd$ kubectl exec etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt get foo
  trident-installer/extras/external-etcd$ kubectl exec etcdctl -- etcdctl --endpoints=https://trident-etcd-client:2379 --cert=/root/certs/etcd-client.crt --key=/root/certs/etcd-client.key --cacert=/root/certs/etcd-client-ca.crt del foo
  trident-installer/extras/external-etcd$ kubectl delete -f kubernetes-yaml/etcdctl-pod.yaml

The above commands invoke the ``etcdctl`` binary inside the etcdctl pod to
interact with the etcd cluster. Please see ``kubernetes-yaml/etcdctl-pod.yaml``
to understand how client credentials are supplied using the ``etcd-client-tls``
Secret to the etcdctl pod. It is important to note that etcd operator requires
a working kube-dns pod as it relies on a Kubernetes Service to communicate with
the etcd cluster.

Uninstalling
^^^^^^^^^^^^

To uninstall the etcd cluster in namespace `trident`, run the following:

.. code-block:: bash

  trident-installer/extras/external-etcd$ ./uninstall_etcd.sh -n trident
