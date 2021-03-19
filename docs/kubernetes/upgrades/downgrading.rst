###################
Downgrading Trident
###################

This section examines the various factors involved when attempting to move
back to an earlier release of Trident and its implications. There could be
certain reasons that require considering a downgrade path:

1. Contingency planning
2. Immediate fix for bugs observed as a result of an upgrade
3. Dependency issues, unsuccessful and incomplete upgrades

With the ``19.07`` release, Trident introduced
:ref:`Custom Resource Definitions(CRDs) <Kubernetes CustomResourceDefinition objects>`
to store Trident's metadata. This marked a move from previous versions,
which used a dedicated etcd instance to write the metadata to a PV created
and managed by Trident for this purpose (the ``trident`` PV). A
greenfield installation of Trident versions ``19.07`` and above will create these CRDs
and use the CRDs to maintain its state. Upgrading from any version of Trident that
used the legacy etcd [versions ``19.04`` and below] to versions ``19.07`` and above
will involve
:ref:`migrating the etcd contents to create CRD objects <What happens when you upgrade>`.
The ``trident`` volume still lives on the storage cluster.

When to downgrade
=================

Only consider a downgrade when moving to a Trident release that uses CRDs.
Since Trident now uses CRDs for maintaining state, all
storage entities created (backends, storage classes, PVs and Volume Snapshots) have
associated CRD objects instead of data written into the ``trident`` PV [used by the
earlier installed version of Trident]. Newly created PVs, backends and Storage
Classes are all maintained as CRD objects. If you need to downgrade, this should
only be attempted for a version of Trident that runs using CRDs (``19.07`` and above).
This way, all operations performed on the current Trident release are visible once
the downgrade occurs.

When not to downgrade
=====================

You **must not downgrade to a release of Trident that uses etcd to maintain state
(19.04 and below)**. All operations performed with the current Trident release
will **not be reflected after the downgrade**. **Newly created PVs will not be
usable when moving back to an earlier version. Changes made to objects such as
backends, PVs, storage classes and Volume Snapshots (created/updated/deleted) will
not be visible to Trident when downgraded moving back to an earlier version**. Going
back to an earlier version will not disrupt access for PVs that were already created
using the older release, unless they have been upgraded.

How to downgrade
================

A couple of options exist when attempting to downgrade Trident, based on how it
has been installed.

- If Trident is installed using the operator, downgrades involve deleting the
  existing Trident install **as well as** the operator. This will be followed with
  the installation of the downgrade candidate (using the operator or ``tridentctl``).
  The operator is a supported mode of installation for versions ``20.04`` and above.
- If Trident is installed using ``tridentctl``, a downgrade requires uninstalling
  Trident with ``tridentctl``. Once this is complete, the desired downgrade
  candidate can be installed using ``tridentctl`` or the operator. The operator
  is a supported mode of installation for versions ``20.04`` and above.

If you are unsure as to how Trident is installed, here is a simple test to run:

1. List the pods present in the Trident namespace. If you see a pod named
   ``trident-operator-xxxxxxxxxx-xxxxx``, then you likely have Trident installed using
   the operator. If you do not see a Trident operator pod, it's likely you installed
   using ``tridentctl``. You can confirm this by running steps 2 and 3.
2. Identify the version of Trident running in your cluster. You can either use
   ``tridentctl`` or take a look at the image used in the Trident pods.
3. Depending on the version of Trident installed:

    a. If using ``21.01`` or above, list the ``tridentorchestrator`` objects present, and
       check its status to confirm it is ``Installed``. **Trident has been installed
       using the cluster-scoped operator**.
    b. If using a version between [``20.04`` - ``20.10``], list the ``tridentprovisioner``
       objects present, and check its status to confirm it is ``Installed``.
       **Trident has been installed using the namespace-scoped operator**.
    c. If you **do not see** a ``tridentOrchestrator`` (or) a ``tridentprovisioner``
       (or) a pod named ``trident-operator-xxxxxxxxxx-xxxxx``, then **Trident has
       been installed with ``tridentctl``**.


.. code-block:: bash


  #Obtain a list of pods in the Trident namespace. Do you see a trident-operator-xxxxxxxxxx-xxxxx pod?
  #If yes, then you have Trident installed using the operator.
  $ kubectl get pods -n trident
  NAME                                READY   STATUS    RESTARTS   AGE
  trident-csi-79df798bdc-2jzpq        6/6     Running   0          19h
  trident-csi-mhw2d                   2/2     Running   0          19h
  trident-operator-5574dbbc68-nthjv   1/1     Running   0          20h

  #Obtain the version of Trident installed. Either use tridentctl or check the Trident
  #image that is used.
  $ tridentctl -n trident version
  +----------------+----------------+
  | SERVER VERSION | CLIENT VERSION |
  +----------------+----------------+
  | 21.01.2        | 21.01.2        |
  +----------------+----------------+

  $ kubectl describe pod trident-csi-79df798bdc-2jzpq -n trident | grep "Image" -A 2 -B 2 | head -4
  trident-main:
    Container ID:  docker://e088b1ffc7017ddba8144d334cbc1eb646bf3491be031ef583a3f189ed965213
    Image:         netapp/trident:21.01.2
    Image ID:      docker-pullable://netapp/trident@sha256:28095a20d8cfffaaaaakkkkkeeeeeec4925ac5d652341b6eaa2ea9352f1e0

  #Is the version of Trident being used >=21.01? If yes, check if a ``tridentorchestrator`` is present.
  #If yes, then you have installed Trident using the operator.
  $ kubectl get torc
  NAME        AGE
  trident     19h

  $ kubectl describe torc trident | grep Message: -A 3
  Message:                Trident installed
  Namespace:              trident
  Status:                 Installed
  Version:                v21.01.2

  #Is the version of Trident being used in the range [20.04 - 20.10]? If yes, check if a ``tridentprovisioner`` is present.
  #If yes, then you have installed Trident using the operator.
  $ kubectl get tprov -n trident
  NAME         AGE
  trident-2010 38d

  $ kubectl describe tprov trident-2010 -n trident | grep Message: -A 3
  Message:                Trident installed
  Status:                 Installed
  Version:                v20.10.1

Handling downgrades with ``tridentctl``
---------------------------------------

After understanding :ref:`when to downgrade/not downgrade <When to downgrade>`, these
are the steps involved in moving down to an earlier release using ``tridentctl``.
This sequence walks you through the downgrade process to move from
Trident ``19.10`` to ``19.07``.

1. Before beginning the downgrade, it is recommended to take a snapshot of your
   Kubernetes cluster's etcd. This allows you to backup the current state of Trident's
   CRDs.
2. Uninstall Trident with the existing ``tridentctl`` binary. In this case, you will
   uninstall with the ``19.10`` binary.

  .. code-block:: console

     $ tridentctl version -n trident
     +----------------+----------------+
     | SERVER VERSION | CLIENT VERSION |
     +----------------+----------------+
     | 19.10.0        | 19.10.0        |
     +----------------+----------------+

     $ tridentctl uninstall -n trident
     INFO Deleted Trident deployment.
     INFO Deleted Trident daemonset.
     INFO Deleted Trident service.
     INFO Deleted Trident secret.
     INFO Deleted cluster role binding.
     INFO Deleted cluster role.
     INFO Deleted service account.
     INFO Deleted pod security policy.                  podSecurityPolicy=tridentpods
     INFO The uninstaller did not delete Trident's namespace in case it is going to be reused.
     INFO Trident uninstallation succeeded.

3. Obtain the Trident binary for the desired version [``19.07``]
   and use it to install Trident.
   Generate custom yamls for a :ref:`customized installation <Customized Installation>`
   if needed.

   .. code-block:: console

      $ cd 19.07/trident-installer/
      $ ./tridentctl install -n trident-ns
      INFO Created installer service account.            serviceaccount=trident-installer
      INFO Created installer cluster role.               clusterrole=trident-installer
      INFO Created installer cluster role binding.       clusterrolebinding=trident-installer
      INFO Created installer configmap.                  configmap=trident-installer
      ...
      ...
      INFO Deleted installer cluster role binding.
      INFO Deleted installer cluster role.
      INFO Deleted installer service account.

The downgrade process is complete.

How to downgrade using Helm
---------------------------

To downgrade, use the ``helm rollback`` command. See the following example:

.. code-block:: console

  $ helm rollback trident [revision #]

Handling downgrades with the Trident Operator
---------------------------------------------

For installs done using the Trident Operator, the downgrade process is different
and does not require the use of ``tridentctl``. There can be one of three options:

i. Trident is installed using the cluster-scoped operator (``21.01`` and above).
ii. Trident is installed using the namespace-scoped operator (``20.04`` - ``20.10``).
iii. Trident was installed using ``tridenctl``, and not the operator.

Downgrading from cluster-scoped operator to namespace-scoped operator
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

This section summarizes the steps involved in downgrading

**FROM:** Trident ``21.01`` and above, installed using the cluster-scoped operator.

**TO:** Trident release that falls in the range [``20.04`` - ``20.10``],
which will be installed using the namespace-scoped operator.

1. Uninstall Trident [See :ref:`Uninstalling with the Trident Operator <Uninstalling with the Trident Operator>`].
   **Do not wipeout the CRDs unless you want to completely remove an existing install**.

2. Make sure the ``tridentorchestrator`` is deleted.

  .. code-block:: bash

    #Check to see if there are any tridentorchestrators present
    $ kubectl get torc
    NAME        AGE
    trident     20h

    #Looks like there is a tridentorchestrator that needs deleting
    $ kubectl delete torc trident
    tridentorchestrator.trident.netapp.io "trident" deleted

3. Delete the cluster-scoped operator. To do this, you will need the manifest
   used to deploy the operator. You can obtain it `here <https://github.com/NetApp/trident/blob/stable/v21.01/deploy/bundle.yaml>`_
   from the Trident GitHub repo. **Make sure you switch to the required branch**.

4. Delete the ``tridentorchestrator`` CRD.

   .. code-block:: bash

      #Check to see if ``tridentorchestrators.trident.netapp.io`` CRD is present and delete it.
      $ kubectl get crd tridentorchestrators.trident.netapp.io
      NAME                                     CREATED AT
      tridentorchestrators.trident.netapp.io   2021-01-21T21:11:37Z
      $ kubectl delete crd tridentorchestrators.trident.netapp.io
      customresourcedefinition.apiextensions.k8s.io "tridentorchestrators.trident.netapp.io" deleted

5. Trident has been uninstalled. Continue downgrading by installing the desired
   version. Follow the documentation for the desired release. For example, the
   instructions to install ``20.07`` are available in the
   `Deploying with the Trident Operator <https://netapp-trident.readthedocs.io/en/stable-v20.07/kubernetes/deploying/operator-deploy.html#>`_.
