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

After understanding :ref:`when to downgrade/not downgrade <When to downgrade>`, these
are the steps involved in moving down to an earlier release. This sequence walks you
through the downgrade process to move from Trident ``19.10`` to ``19.07``

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
