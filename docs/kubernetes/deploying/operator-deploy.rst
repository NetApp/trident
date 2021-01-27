.. _deploying-with-operator:

###################################
Deploying with the Trident Operator
###################################

If you are looking to deploy Trident using the Trident Operator, you are
in the right place. This page contains all the steps required for getting
started with the Trident Operator to install and manage Trident. You can deploy Trident Operator either manually or using Helm.

Deploy Trident Operator by using Helm
=====================================

Perform the steps listed to deploy Trident Operator by using Helm.

Prerequisites
-------------

To deploy Trident Operator by using Helm, you need the following:

* Kubernetes 1.16 and later
* Helm version 3

1: Download the installer bundle
--------------------------------

Download the Trident 21.01 installer bundle from the `Trident GitHub <https://github.com/netapp/trident/releases>`_ page. The installer bundle includes the Helm chart in the ``/helm`` directory.

2: Deploy the Trident operator
------------------------------

Use the ``helm install`` command and specify a name for your deployment. See the following example:

.. code-block:: console

  $ helm install <name> trident-operator-21.01.0.tgz

There are two ways to pass configuration data during the install:

* --values (or -f): Specify a YAML file with overrides. This can be specified multiple times and the rightmost file will take precedence.
* --set: Specify overrides on the command line.

For example, to change the default value of ``debug``, run the following --set command:

.. code-block:: console

  $ helm install <name> trident-operator-21.01.0.tgz --set tridentDebug=true

The ``values.yaml`` file, which is part of the Helm chart provides the list of keys and their default values.

``helm list`` shows you details about the Trident installation, such as name, namespace, chart, status, app version, revision number, and so on.

Deploy Trident Operator manually
================================

Perform the steps listed to manually deploy Trident Operator.

Prerequisites
-------------

If you have not already familiarized yourself with the
:ref:`basic concepts <What is Trident?>`, now is a great time to do that. Go
ahead, we'll be here when you get back.

To deploy Trident using the operator you need:

.. sidebar:: Need Kubernetes?

  If you do not already have a Kubernetes cluster, you can easily create one for
  demonstration purposes using our
  :ref:`simple Kubernetes install guide <Simple Kubernetes install>`.

* Full privileges to a
  :ref:`supported Kubernetes cluster <Supported frontends (orchestrators)>`
  running Kubernetes ``1.14`` and above.
* Access to a
  :ref:`supported NetApp storage system <Supported backends (storage)>`
* :ref:`Volume mount capability <Preparing the worker node>` from all of the
  Kubernetes worker nodes
* A Linux host with ``kubectl`` (or ``oc``, if you're using OpenShift) installed
  and configured to manage the Kubernetes cluster you want to use
* Set the ``KUBECONFIG`` environment variable to point to your Kubernetes
  cluster configuration.
* Enable the :ref:`Feature Gates <Feature Requirements>` required by Trident
* If you are using Kubernetes with Docker Enterprise, `follow their steps
  to enable CLI access <https://docs.docker.com/ee/ucp/user-access/cli/>`_.

Got all that? Great! Let's get started.

1: Qualify your Kubernetes cluster
----------------------------------

You made sure that you have everything in hand from the
:ref:`previous section <Before you begin>`, right? Right.

The first thing you need to do is log into the Linux host and verify that it is
managing a *working*,
:ref:`supported Kubernetes cluster <Supported frontends (orchestrators)>` that
you have the necessary privileges to.

.. note::
  With OpenShift, you will use ``oc`` instead of ``kubectl`` in all of the
  examples that follow, and you need to login as **system:admin** first by
  running ``oc login -u system:admin`` or ``oc login -u kube-admin``.

.. code-block:: bash

  # Is your Kubernetes version greater than 1.14?
  kubectl version

  # Are you a Kubernetes cluster administrator?
  kubectl auth can-i '*' '*' --all-namespaces

  # Can you launch a pod that uses an image from Docker Hub and can reach your
  # storage system over the pod network?
  kubectl run -i --tty ping --image=busybox --restart=Never --rm -- \
    ping <management IP>

2: Download & setup the operator
--------------------------------

.. note::

   Using the Trident Operator to install Trident requires creating the
   ``TridentOrchestrator`` Custom Resource Definition and defining other
   resources. You will need to perform these steps to setup the operator
   before you can install Trident.

Download the latest version of the `Trident installer bundle`_ from the
*Downloads* section and extract it.

.. code-block:: console

   wget https://github.com/NetApp/trident/releases/download/v21.01.0/trident-installer-21.01.0.tar.gz
   tar -xf trident-installer-21.01.0.tar.gz
   cd trident-installer

.. _Trident installer bundle: https://github.com/NetApp/trident/releases/latest

Use the appropriate CRD manifest to create the ``TridentOrchestrator`` Custom
Resource Definition. You will then create a ``TridentOrchestrator`` Custom Resource
later on to instantiate a Trident install by the operator.

.. code-block:: bash

  # Is your Kubernetes version < 1.16?
  kubectl create -f deploy/crds/trident.netapp.io_tridentorchestrators_crd_pre1.16.yaml

  # If not, your Kubernetes version must be 1.16 and above
  kubectl create -f deploy/crds/trident.netapp.io_tridentorchestrators_crd_post1.16.yaml

Once the ``TridentOrchestrator`` CRD is created, you will then have to create
the resources required for the operator deployment, such as:

* a ServiceAccount for the operator.
* a ClusterRole and ClusterRoleBinding to the ServiceAccount.
* a dedicated PodSecurityPolicy.
* the Operator itself.

The Trident Installer contains manifests for defining these resources. By default
the operator is deployed in ``trident`` namespace, if the ``trident`` namespace
does not exist use the below manifest to create one.

.. code-block:: console

  $ kubectl apply -f deploy/namespace.yaml

If you would like to deploy the operator in a namespace other than
the default ``trident`` namespace, you will need to update the
``serviceaccount.yaml``, ``clusterrolebinding.yaml`` and ``operator.yaml``
manifests and generate your ``bundle.yaml``.

.. code-block:: bash

  # Have you updated the yaml manifests? Generate your bundle.yaml
  # using the kustomization.yaml
  kubectl kustomize deploy/ > deploy/bundle.yaml

  # Create the resources and deploy the operator
  kubectl create -f deploy/bundle.yaml

You can check the status of the operator once you have deployed.

.. code-block:: console

   $ kubectl get deployment -n <operator-namespace>
   NAME               READY   UP-TO-DATE   AVAILABLE   AGE
   trident-operator   1/1     1            1           3m

   $ kubectl get pods -n <operator-namespace>
   NAME                              READY   STATUS             RESTARTS   AGE
   trident-operator-54cb664d-lnjxh   1/1     Running            0          3m

The operator deployment successfully creates a pod running on one of the
worker nodes in your cluster.

.. important::

   There must only be **one instance of the operator in a Kubernetes cluster**.
   **Do not create multiple deployments of the Trident operator**.

3: Creating a TridentOrchestrator CR and installing Trident
-----------------------------------------------------------

You are now ready to install Trident using the operator! This will require
creating a TridentOrchestrator CR. The Trident installer comes with example
defintions for creating a TridentOrchestrator CR.

.. code-block:: console

   $ kubectl create -f deploy/crds/tridentorchestrator_cr.yaml
   tridentorchestrator.trident.netapp.io/trident created

   $  kubectl get torc -n trident
   NAME      AGE
   trident   5s
   $ kubectl describe torc trident -n trident
   Name:         trident
   Namespace:    trident
   Labels:       <none>
   Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                   {"apiVersion":"trident.netapp.io/v1","kind":"TridentOrchestrator","metadata":{"annotations":{},"name":"trident","namespace":"trident"},"spe...
   API Version:  trident.netapp.io/v1
   Kind:         TridentOrchestrator
   ...
   Spec:
     Debug:          true
   Status:
     Current Installation Params:
       IPv6:   false
       Debug:  true
       Image Pull Secrets:
       Image Registry:  k8s.gcr.io/sig-storage (k8s 1.17+, otherwise quay.io/k8scsi)
       k8sTimeout:      30
       Kubelet Dir:     /var/lib/kubelet
       Log Format:      text
       Trident Image:   netapp/trident:21.01.0
     Message:           Trident installed
     Status:            Installed
     Version:           v21.01.0
   Events:
     Type    Reason      Age   From                        Message
     ----    ------      ----  ----                        -------
     Normal  Installing  19s   trident-operator.netapp.io  Installing Trident
     Normal  Installed   5s    trident-operator.netapp.io  Trident installed

Observing the status of the operator
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The Status of the TridentOrchestrator will indicate if the installation
was successful and will display the version of Trident installed.

+-----------------+--------------------------------------------------------------------------+
| Status          |              Description                                                 |
+=================+==========================================================================+
| Installing      | The operator is installing Trident using this ``TridentOrchestrator`` CR.|
+-----------------+--------------------------------------------------------------------------+
| Installed       | Trident has successfully installed.                                      |
+-----------------+--------------------------------------------------------------------------+
| Uninstalling    | The operator is uninstalling Trident, since ``spec.uninstall=true``.     |
+-----------------+--------------------------------------------------------------------------+
| Uninstalled     | Trident is uninstalled.                                                  |
+-----------------+--------------------------------------------------------------------------+
| Failed          | The operator could not install, patch, update or uninstall Trident; the  |
+-----------------+--------------------------------------------------------------------------+
|                 | operator will automatically try to recover from this state. If this      |
+-----------------+--------------------------------------------------------------------------+
|                 | state persists you will require troubleshooting.                         |
+-----------------+--------------------------------------------------------------------------+
| Updating        | The operator is updating an existing Trident installation.               |
+-----------------+--------------------------------------------------------------------------+
| Error           | The ``TridentOrchestrator`` is not used. Another one already exists.     |
+-----------------+--------------------------------------------------------------------------+

During the installation, the status of the ``TridentOrchestrator``
will change from ``Installing`` to ``Installed``. If you observe
the ``Failed`` status and the operator is unable to recover by
itself, there's probably something wrong and you
will need to check the logs of the operator by running
``tridentctl logs -l trident-operator``.

You can also confirm if the Trident install completed
by taking a look at the pods that have been created:

.. code-block:: console

   $ kubectl get pod -n trident
   NAME                                READY   STATUS    RESTARTS   AGE
   trident-csi-7d466bf5c7-v4cpw        5/5     Running   0           1m
   trident-csi-mr6zc                   2/2     Running   0           1m
   trident-csi-xrp7w                   2/2     Running   0           1m
   trident-csi-zh2jt                   2/2     Running   0           1m
   trident-operator-766f7b8658-ldzsv   1/1     Running   0           3m


You can also use ``tridentctl`` to check the version of Trident installed.

.. code-block:: console

   $ ./tridentctl -n trident version
   +----------------+----------------+
   | SERVER VERSION | CLIENT VERSION |
   +----------------+----------------+
   | 21.01.0        | 21.01.0        |
   +----------------+----------------+

If that's what you see, you're done with this step, but **Trident is not
yet fully configured.** Go ahead and continue to the
:ref:`next step <1: Creating a Trident backend>` to create
a Trident backend using ``tridentctl``.

However, if the installer does not complete successfully or you don't see
a **Running** ``trident-csi-<generated id>``, then Trident had a problem and the platform was *not*
installed.

To understand why the installation of Trident was unsuccessful, you should
first take a look at the ``TridentOrchestrator`` status.

.. code-block:: console

  $ kubectl describe torc trident-2 -n trident
  Name:         trident-2
  Namespace:    trident
  Labels:       <none>
  Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                  {"apiVersion":"trident.netapp.io/v1","kind":"TridentOrchestrator","metadata":{"annotations":{},"name":"trident","namespace":"trident"},"spe...
  API Version:  trident.netapp.io/v1
  Kind:         TridentOrchestrator
  Status:
    Current Installation Params:
      IPv6:
      Debug:
      Image Pull Secrets:  <nil>
      Image Registry:
      k8sTimeout:
      Kubelet Dir:
      Log Format:
      Trident Image:
    Message:               Trident is bound to another CR 'trident' in the same namespace
    Status:                Error
    Version:
  Events:                  <none>

This error indicates that there already exists a TridentOrchestrator that was
used to install Trident. Since each Kubernetes cluster can only have one instance
of Trident, the operator ensures that at any given time there only exists one
active TridentOrchestrator that it can create.

Another thing to do is to check the operator logs. Trailing the logs of the
``trident-operator`` container can point to where the problem lies.

.. code-block:: console

   $ tridentctl logs -l trident-operator

For example, one such issue could be the inability to pull the required container
images from upstream registries in an airgapped environment. The logs from the
operator can help identify this problem and fix it.

In addition, observing the status of the Trident pods can often indicate if
something is not right.

.. code-block:: console

   $ kubectl get pods -n trident

   NAME                                READY   STATUS             RESTARTS   AGE
   trident-csi-4p5kq                   1/2     ImagePullBackOff   0          5m18s
   trident-csi-6f45bfd8b6-vfrkw        4/5     ImagePullBackOff   0          5m19s
   trident-csi-9q5xc                   1/2     ImagePullBackOff   0          5m18s
   trident-csi-9v95z                   1/2     ImagePullBackOff   0          5m18s
   trident-operator-766f7b8658-ldzsv   1/1     Running            0          8m17s

You can clearly see that the pods are not able to intialize completely as one
or more container images were not fetched.

To address the problem, you must edit the TridentOrchestrator CR. Alternatively,
you can delete the TridentOrchestrator and create a new one with the modified,
accurate definition.

If you continue to have trouble, visit the
:ref:`troubleshooting guide <Troubleshooting>` for more advice.

.. _operator-customize:

Customizing your deployment
~~~~~~~~~~~~~~~~~~~~~~~~~~~

The Trident operator provides users the ability to customize the manner in which
Trident is installed, using the following attributes in the TridentOrchestrator ``spec``:

========================= ============================================================================== ==========================================================
Parameter                 Description                                                                    Default
========================= ============================================================================== ==========================================================
debug                     Enable debugging for Trident                                                   'false'
useIPv6                   Install Trident over IPv6                                                      'false'
k8sTimeout                Timeout for Kubernetes operations                                              30sec
silenceAutosupport        Don't send autosupport bundles to NetApp automatically                         'false'
enableNodePrep            Manage worker node dependencies automatically (**BETA**)                       'false'
autosupportImage          The container image for Autosupport Telemetry                                  "netapp/trident-autosupport:21.01.0"
autosupportProxy          The address/port of a proxy for sending Autosupport Telemetry                  "http://proxy.example.com:8888"
uninstall                 A flag used to uninstall Trident                                               'false'
logFormat                 Trident logging format to be used [text,json]                                  "text"
tridentImage              Trident image to install                                                       "netapp/trident:21.01"
imageRegistry             Path to an internal registry, of the format ``<registry FQDN>[:port][/subpath] `` "k8s.gcr.io/sig-storage (k8s 1.17+) or quay.io/k8scsi"
kubeletDir                Path to the kubelet directory on the host                                      "/var/lib/kubelet"
wipeout                   A list of resources to delete to perform a complete removal of Trident
imagePullSecrets          Secrets to pull images from an internal registry
========================= ============================================================================== ==========================================================

.. warning::

  Automatic worker node prep is a **beta feature** meant to be used in
  non-production environments only.

You can use the attributes mentioned above when defining a TridentOrchestrator to
customize your Trident installation. Here's an example:

.. code-block:: console

   $ cat deploy/crds/tridentorchestrator_cr_imagepullsecrets.yaml
   apiVersion: trident.netapp.io/v1
   kind: TridentOrchestrator
   metadata:
     name: trident
     namespace: trident
   spec:
     debug: true
     tridentImage: netapp/trident:21.01.0
     imagePullSecrets:
     - thisisasecret


If you are looking to customize Trident's installation beyond what the TridentOrchestrator's
arguments allow, you should consider using ``tridentctl`` to generate custom
yaml manifests that you can modify as desired. Head on over to the
:ref:`deployment guide for tridentctl <deploying-with-tridentctl>` to learn
how this works.

Post-deployment steps
=====================

After you deploy Trident with the operator, you can proceed with creating a Trident backend, creating a storage class, provisioning a volume, and mounting the volume in a pod.

1: Creating a Trident backend
-----------------------------

You can now go ahead and create a backend that will be used by Trident
to provision volumes. To do this, create a ``backend.json`` file that
contains the necessary parameters. Sample configuration files for
different backend types can be found in the ``sample-input`` directory.

Visit the :ref:`backend configuration guide <Backend configuration>`
for more details about how to craft the configuration file for
your backend type.

.. code-block:: bash

  cp sample-input/<backend template>.json backend.json
  # Fill out the template for your backend
  vi backend.json

.. code-block:: console

    ./tridentctl -n trident create backend -f backend.json
    +-------------+----------------+--------------------------------------+--------+---------+
    |    NAME     | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
    +-------------+----------------+--------------------------------------+--------+---------+
    | nas-backend | ontap-nas      | 98e19b74-aec7-4a3d-8dcf-128e5033b214 | online |       0 |
    +-------------+----------------+--------------------------------------+--------+---------+

If the creation fails, something was wrong with the backend configuration. You
can view the logs to determine the cause by running:

.. code-block:: console

   ./tridentctl -n trident logs

After addressing the problem, simply go back to the beginning of this step
and try again. If you continue to have trouble, visit the
:ref:`troubleshooting guide <Troubleshooting>` for more advice on how to
determine what went wrong.

2: Creating a Storage Class
---------------------------

Kubernetes users provision volumes using persistent volume claims (PVCs) that
specify a `storage class`_ by name. The details are hidden from users, but a
storage class identifies the provisioner that will be used for that class (in
this case, Trident) and what that class means to the provisioner.

.. sidebar:: Basic too basic?

    This is just a basic storage class to get you started. There's an art to
    :ref:`crafting differentiated storage classes <Designing a storage class>`
    that you should explore further when you're looking at building them for
    production.

Create a storage class Kubernetes users will specify when they want a volume.
The configuration of the class needs to model the backend that you created
in the previous step so that Trident will use it to provision new volumes.

The simplest storage class to start with is one based on the
``sample-input/storage-class-csi.yaml.templ`` file that comes with the
installer, replacing ``__BACKEND_TYPE__`` with the storage driver name.

.. code-block:: bash

    ./tridentctl -n trident get backend
    +-------------+----------------+--------------------------------------+--------+---------+
    |    NAME     | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
    +-------------+----------------+--------------------------------------+--------+---------+
    | nas-backend | ontap-nas      | 98e19b74-aec7-4a3d-8dcf-128e5033b214 | online |       0 |
    +-------------+----------------+--------------------------------------+--------+---------+

    cp sample-input/storage-class-csi.yaml.templ sample-input/storage-class-basic-csi.yaml

    # Modify __BACKEND_TYPE__ with the storage driver field above (e.g., ontap-nas)
    vi sample-input/storage-class-basic-csi.yaml

This is a Kubernetes object, so you will use ``kubectl`` to create it in
Kubernetes.

.. code-block:: console

    kubectl create -f sample-input/storage-class-basic-csi.yaml

You should now see a **basic** storage class in both Kubernetes and Trident,
and Trident should have discovered the pools on the backend.

.. code-block:: console

    kubectl get sc basic-csi
    NAME         PROVISIONER             AGE
    basic-csi    csi.trident.netapp.io   15h

    ./tridentctl -n trident get storageclass basic-csi -o json
    {
      "items": [
        {
          "Config": {
            "version": "1",
            "name": "basic-csi",
            "attributes": {
              "backendType": "ontap-nas"
            },
            "storagePools": null,
            "additionalStoragePools": null
          },
          "storage": {
            "ontapnas_10.0.0.1": [
              "aggr1",
              "aggr2",
              "aggr3",
              "aggr4"
            ]
          }
        }
      ]
    }

.. _storage class: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#storageclasses

3: Provision your first volume
------------------------------

Now you're ready to dynamically provision your first volume. How exciting! This
is done by creating a Kubernetes `persistent volume claim`_ (PVC) object, and
this is exactly how your users will do it too.

.. _persistent volume claim: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims

Create a persistent volume claim (PVC) for a volume that uses the storage
class that you just created.

See ``sample-input/pvc-basic-csi.yaml`` for an example. Make sure the storage
class name matches the one that you created in 6.

.. code-block:: bash

    kubectl create -f sample-input/pvc-basic-csi.yaml

    kubectl get pvc --watch
    NAME      STATUS    VOLUME                                     CAPACITY   ACCESS MODES  STORAGECLASS   AGE
    basic     Pending                                                                       basic          1s
    basic     Pending   pvc-3acb0d1c-b1ae-11e9-8d9f-5254004dfdb7   0                        basic          5s
    basic     Bound     pvc-3acb0d1c-b1ae-11e9-8d9f-5254004dfdb7   1Gi        RWO           basic          7s

4: Mount the volume in a pod
----------------------------

Now that you have a volume, let's mount it. We'll launch an nginx pod that
mounts the PV under ``/usr/share/nginx/html``.

.. code-block:: bash

  cat << EOF > task-pv-pod.yaml
  kind: Pod
  apiVersion: v1
  metadata:
    name: task-pv-pod
  spec:
    volumes:
      - name: task-pv-storage
        persistentVolumeClaim:
         claimName: basic
    containers:
      - name: task-pv-container
        image: nginx
        ports:
          - containerPort: 80
            name: "http-server"
        volumeMounts:
          - mountPath: "/usr/share/nginx/html"
            name: task-pv-storage
  EOF
  kubectl create -f task-pv-pod.yaml

.. code-block:: bash

  # Wait for the pod to start
  kubectl get pod --watch

  # Verify that the volume is mounted on /usr/share/nginx/html
  kubectl exec -it task-pv-pod -- df -h /usr/share/nginx/html
  Filesystem                                                          Size  Used Avail Use% Mounted on
  10.xx.xx.xx:/trident_pvc_3acb0d1c_b1ae_11e9_8d9f_5254004dfdb7       1.0G  256K  1.0G   1% /usr/share/nginx/html


  # Delete the pod
  kubectl delete pod task-pv-pod

At this point the pod (application) no longer exists but the volume is still
there. You could use it from another pod if you wanted to.

To delete the volume, simply delete the claim:

.. code-block:: console

  kubectl delete pvc basic

Where do you go from here? you can do things like:

  * :ref:`Configure additional backends <Backend configuration>`.
  * :ref:`Model additional storage classes <Managing storage classes>`.
  * Review considerations for moving this into production.
