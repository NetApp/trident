###################################
Upgrading with the Trident Operator
###################################

Trident's operator now supports brownfield installations and provides an easy
upgrade path for users that seek to use the operator and greatly simplify
Trident's operation. This document shall talk about the prerequisites and how
the upgrade will work.

.. _operator-prereq:

Prerequisites
-------------

To upgrade using the Trident operator:

1. Only CSI based Trident Installation may exist. To check if you are running
   CSI Trident, examine the pods in your Trident namespace. If they follow the
   ``trident-csi-*`` naming pattern, you are running CSI Trident. Move on to
   step 2.
2. Only CRD based Trident Installation may exist. This represents all Trident
   releases ``19.07`` and above. If you passed step 1, you have most likely also
   cleared step 2.
3. CRDs that **exist as a result of Trident CSI uninstallation** are supported.
   If you have uninstalled CSI Trident and the metadata from the installation
   persists, you can upgrade using the operator.
4. Only one Trident installation should exist across all the namespaces in a
   given Kubernetes cluster.
5. You must be using a Kubernetes cluster that runs version ``1.14`` and above.
   See :ref:`Requirements <Requirements>`
6. Alpha Snapshot CRDs should not be present. If they are present, you must
   remove them with ``tridentctl obliviate alpha-snapshot-crd``. This will delete
   the CRDs for the alpha snapshot spec. For existing snapshots that should be
   deleted/migrated, please read `this blog`_.

Initiating the upgrade
----------------------

After confirming that you meet the :ref:`Prerequisites <operator-prereq>`, you
are good to go ahead and upgrade using the operator.

.. warning::

   If you are running Kubernetes 1.17 or later, and are looking to upgrade to
   ``20.07.1`` or above, it is important you provide ``parameter.fsType`` in
   StorageClasses used by Trident. StorageClasses can be deleted and recreated
   without disrupting pre-existing volumes. This is a **requirement** for
   enforcing `Security Contexts <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_
   for SAN volumes. The `sample-input <https://github.com/NetApp/trident/tree/master/trident-installer/sample-input>`_
   directory contains examples, such as
   `storage-class-basic.yaml.templ <https://github.com/NetApp/trident/blob/master/trident-installer/sample-input/storage-class-basic.yaml.templ>`_,
   and `storage-class-bronze-default.yaml <https://github.com/NetApp/trident/blob/master/trident-installer/sample-input/storage-class-bronze-default.yaml>`_.
   For more information, take a look at the :ref:`Known issues <fstype-fix>` tab.

Upgrading Operator-based Installs
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

This section walks you through the steps involved in upgrading an instance of
Trident (installed using the operator) to the latest version of Trident, using
the operator.

What's new
==========

The 21.01 release of Trident introduces some key architectural changes to the
operator, namely:

a. The operator is now **cluster-scoped**. Previous instances of the Trident Operator
   (versions ``20.04`` through ``20.10``) were **namespace-scoped**. An operator
   that is cluster-scoped is advantageous for the following reasons:

       i. **Resource accountability**: Since it is cluster-scoped, the operator now
          manages resources associated with a Trident installation at the cluster
          level. As part of installing Trident, the operator creates and maintains
          several resources by using
          `ownerReferences <https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/>`_.
          Maintaining ``ownerReferences`` on cluster-scoped resources can throw up
          errors on certain Kubernetes distributors such as OpenShift. This is
          mitigated with a cluster-scoped operator. For auto-healing and patching
          Trident resources, this is an essential requirement.
       ii. **Cleaning up during uninstalls**: A complete removal of Trident would require
           all associated resources to be deleted. A namespace-scoped operator may
           experience issues with the removal of cluster-scoped resources (such as
           the clusterRole, ClusterRoleBinding and PodSecurityPolicy) and lead to
           an incomplete clean-up. A cluster-scoped operator eliminates this issue.
           Users can completely uninstall Trident and install afresh if needed.
b. ``TridentProvisioner`` is now replaced with ``TridentOrchestrator`` as the
   Custom Resource used to install and manage Trident. In addition, a new field
   is introduced to the ``TridentOrchestrator`` spec. Users can specify the
   namespace Trident must be installed/upgraded from using the ``spec.namespace``
   field. You can take a look at an example `here <https://github.com/NetApp/trident/blob/stable/v21.01/deploy/crds/tridentorchestrator_cr.yaml>`_.

Upgrading Trident using the cluster-scoped operator
===================================================

To upgrade **from**: an instance of Trident installed using the namespace-scoped
operator [versions ``20.04`` through ``20.10``] **to**: a newer release that makes
use of the cluster-scoped operator [versions ``21.01`` and later], here is the
set of steps to be followed:

1. Before all else, determine the status of the existing Trident install. To do
   this, check the ``Status`` field of the ``TridentProvisioner``. The ``Status``
   **must be ``Installed``**.

  .. code-block:: bash

      #Check the status of TridentProvisioner
      $ kubectl describe tprov trident -n trident | grep Message: -A 3
      Message:  Trident installed
      Status:   Installed
      Version:  v20.10.1

  After confirming the ``Status`` is ``Installed``, proceed to step 2. If the
  ``Status`` is something else (such as ``Updating``), make sure to address this
  before proceeding. For a list of possible status values, take a look
  :ref:`here <Observing the status of the operator>`.

2. Create the ``TridentOrchestrator`` CRD using the manifest provided with the
   Trident installer.

  .. code-block:: bash

      # Download the release required [21.01]
      $ mkdir 21.01.1
      $ cd 21.01.1
      $ wget https://github.com/NetApp/trident/releases/download/v21.01.1/trident-installer-21.01.1.tar.gz
      $ tar -xf trident-installer-21.01.1.tar.gz
      $ cd trident-installer

      # Is your Kubernetes version < 1.16?
      $ kubectl create -f deploy/crds/trident.netapp.io_tridentorchestrators_crd_pre1.16.yaml

      # If not, your Kubernetes version must be 1.16 and above
      $ kubectl create -f deploy/crds/trident.netapp.io_tridentorchestrators_crd_post1.16.yaml

3. Delete the namespace-scoped operator using its manifest. To complete this step,
   you require the ``bundle.yaml`` file used to deploy the namespace-scoped
   operator. You can always obtain the ``bundle.yaml`` from `Trident repository <https://github.com/NetApp/trident/blob/stable/v20.10/deploy/bundle.yaml>`_.
   Make sure to **use the appropriate branch**.

  .. important::

    Changes that need to be made to the Trident install parameters (such as changing
    the tridentImage, autosupportImage, private image repository, providing
    ``imagePullSecrets`` for example) **must be performed after deleting the
    namespace-scoped operator and before installing the cluster-scoped operator**.
    For a complete list of parameters that can be updated, take a look at the list of
    parameters available to customize the ``TridentProvisioner``. You can find that in
    `this table <https://netapp-trident.readthedocs.io/en/stable-v20.10/kubernetes/deploying/operator-deploy.html#customizing-your-deployment>`_.

  Confirm the operator is removed before proceeding to step 4.

  .. code-block:: bash

     #Ensure you are in the right directory
     $ pwd
     $ /root/20.10.1/trident-installer

     #Delete the namespace-scoped operator
     $ kubectl delete -f deploy/bundle.yaml
     $ serviceaccount "trident-operator" deleted
     $ clusterrole.rbac.authorization.k8s.io "trident-operator" deleted
     $ clusterrolebinding.rbac.authorization.k8s.io "trident-operator" deleted
     $ deployment.apps "trident-operator" deleted
     $ podsecuritypolicy.policy "tridentoperatorpods" deleted

     #Confirm the Trident operator was removed
     $ kubectl get all -n trident
     NAME                               READY   STATUS    RESTARTS   AGE
     pod/trident-csi-68d979fb85-dsrmn   6/6     Running   12         99d
     pod/trident-csi-8jfhf              2/2     Running   6          105d
     pod/trident-csi-jtnjz              2/2     Running   6          105d
     pod/trident-csi-lcxvh              2/2     Running   8          105d

     NAME                  TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)              AGE
     service/trident-csi   ClusterIP   10.108.174.125   <none>        34571/TCP,9220/TCP   105d

     NAME                         DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR                                     AGE
     daemonset.apps/trident-csi   3         3         3       3            3           kubernetes.io/arch=amd64,kubernetes.io/os=linux   105d

     NAME                          READY   UP-TO-DATE   AVAILABLE   AGE
     deployment.apps/trident-csi   1/1     1            1           105d

     NAME                                     DESIRED   CURRENT   READY   AGE
     replicaset.apps/trident-csi-68d979fb85   1         1         1       105d

  At this stage, the ``trident-operator-xxxxxxxxxx-xxxxx`` pod is deleted.

4. **[OPTIONAL]**: If the install parameters need to be modified, update the
   ``TridentProvisioner`` spec. These could be changes such as modifying the
   private image registry to pull container images from, enabling debug logs, or
   specifying image pull secrets.

   .. code-block:: bash

     $  kubectl patch tprov <trident-provisioner-name> -n <trident-namespace> --type=merge -p '{"spec":{"debug":true}}'

5. Install the cluster-scoped operator.

   .. important::

    Upgrading Trident using the cluster-scoped operator will result in the migration
    of ``tridentProvisioner`` to a ``tridentOrchestrator`` object with the same
    name. This is automatically handled by the operator. The upgrade will also
    have Trident installed in the same namespace as before.

   .. code-block:: bash

     #Ensure you are in the correct directory
     $ pwd
     $ /root/21.01.1/trident-installer

     #Install the cluster-scoped operator in the **same namespace**
     $ kubectl create -f deploy/bundle.yaml
     serviceaccount/trident-operator created
     clusterrole.rbac.authorization.k8s.io/trident-operator created
     clusterrolebinding.rbac.authorization.k8s.io/trident-operator created
     deployment.apps/trident-operator created
     podsecuritypolicy.policy/tridentoperatorpods created

     #All tridentProvisioners will be removed, including the CRD itself
     $ kubectl get tprov -n trident
     Error from server (NotFound): Unable to list "trident.netapp.io/v1, Resource=tridentprovisioners": the server could not find the requested resource (get tridentprovisioners.trident.netapp.io)

     #tridentProvisioners are replaced by tridentOrchestrator
     $ kubectl get torc
     NAME      AGE
     trident   13s

     #Examine Trident pods in the namespace
     $ kubectl get pods -n trident
     NAME                                READY   STATUS    RESTARTS   AGE
     trident-csi-79df798bdc-m79dc        6/6     Running   0          1m41s
     trident-csi-xrst8                   2/2     Running   0          1m41s
     trident-operator-5574dbbc68-nthjv   1/1     Running   0          1m52s

     #Confirm Trident has been updated to the desired version
     $ kubectl describe torc trident | grep Message -A 3
     Message:                Trident installed
     Namespace:              trident
     Status:                 Installed
     Version:                v21.01.1


  Installing the cluster-scoped operator will:

  i. Initiate the migration of ``TridentProvisioner`` objects to ``TridentOrchestrator``
     objects.
  ii. Delete ``TridentProvisioner`` objects and the ``tridentprovisioner`` CRD.
  iii. Upgrade Trident to the version of the cluster-scoped operator being used.
       In the example above, Trident was upgraded to ``21.01.1``.

Upgrading a Helm-based operator install
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If you have a Helm-based operator install, to upgrade, do the following:

1. Download the latest Trident release.
2. Use the ``helm upgrade`` command. See the following example:

.. code-block:: console

 $ helm upgrade <name> trident-operator-21.01.1.tgz

where ``trident-operator-21.01.1.tgz`` reflects the version that you want to upgrade to.

If you run ``helm list``, the output shows that the chart and app version have both been upgraded.

To pass configuration data during the upgrade, use --set. For example, to change the default value of ``tridentDebug``, run the following --set command:

.. code-block:: console

  $ helm upgrade <name> trident-operator-21.01.1-custom.tgz --set tridentDebug=true

If you run ``$ tridentctl logs``, you can see the debug messages.

.. note::

  If you set any non-default options during the initial installation, ensure that the options are included in the upgrade command, or else, the values will be reset to their defaults.

Upgrading from a non-operator install
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If you have a CSI Trident instance that has satisfied the
:ref:`Prerequisites <operator-prereq>`, you can upgrade to the latest release
of the Trident Operator by following the instructions provided in the
:ref:`Operator deployment <deploying-with-operator>`. You must:

1. Download the latest Trident release.

.. code-block:: bash

  # Download the release required [21.01]
  $ mkdir 21.01.1
  $ cd 21.01.1
  $ wget https://github.com/NetApp/trident/releases/download/v21.01.1/trident-installer-21.01.1.tar.gz
  $ tar -xf trident-installer-21.01.1.tar.gz
  $ cd trident-installer

2. Create the ``tridentorchestrator`` CRD from the manifest.

.. code-block:: bash

  # Is your Kubernetes version < 1.16?
  $ kubectl create -f deploy/crds/trident.netapp.io_tridentorchestrators_crd_pre1.16.yaml

  # If not, your Kubernetes version must be 1.16 and above
  $ kubectl create -f deploy/crds/trident.netapp.io_tridentorchestrators_crd_post1.16.yaml

3. Deploy the operator.

.. code-block:: bash

  #Install the cluster-scoped operator in the **same namespace**
  $ kubectl create -f deploy/bundle.yaml
  serviceaccount/trident-operator created
  clusterrole.rbac.authorization.k8s.io/trident-operator created
  clusterrolebinding.rbac.authorization.k8s.io/trident-operator created
  deployment.apps/trident-operator created
  podsecuritypolicy.policy/tridentoperatorpods created

  #Examine the pods in the Trident namespace
  NAME                                READY   STATUS    RESTARTS   AGE
  trident-csi-79df798bdc-m79dc        6/6     Running   0          150d
  trident-csi-xrst8                   2/2     Running   0          150d
  trident-operator-5574dbbc68-nthjv   1/1     Running   0          1m30s

4. Create a ``TridentOrchestrator`` CR for installing Trident.

.. code-block:: bash

  #Create a tridentOrchestrator to initate a Trident install
  $ cat deploy/crds/tridentorchestrator_cr.yaml
  apiVersion: trident.netapp.io/v1
  kind: TridentOrchestrator
  metadata:
    name: trident
  spec:
    debug: true
    namespace: trident

  $ kubectl create -f deploy/crds/tridentorchestrator_cr.yaml

  #Examine the pods in the Trident namespace
  NAME                                READY   STATUS    RESTARTS   AGE
  trident-csi-79df798bdc-m79dc        6/6     Running   0          1m
  trident-csi-xrst8                   2/2     Running   0          1m
  trident-operator-5574dbbc68-nthjv   1/1     Running   0          5m41s

  #Confirm Trident was upgraded to the desired version
  $ kubectl describe torc trident | grep Message -A 3
  Message:                Trident installed
  Namespace:              trident
  Status:                 Installed
  Version:                v21.01.1

5. Existing backends and PVCs will be automatically available.

All of this is documented thoroughly in the
:ref:`Operator deployment <deploying-with-operator>` section.

.. note::

   You will need to remove alpha snapshot CRDs (if they exist) before upgrading
   using the operator. Use ``tridentctl obliviate alpha-snapshot-crd`` to
   achieve this.

.. _this blog: https://netapp.io/2020/01/30/alpha-to-beta-snapshots/
.. _installer bundle: https://github.com/NetApp/trident/releases/latest
