################
Managing Trident
################

Installing Trident
------------------

Follow the extensive :ref:`deployment <deploying-in-kubernetes>` guide.

Upgrading Trident
-----------------

The :ref:`Upgrade Guide <Upgrading Trident>` details the procedure for upgrading
to the latest version of Trident.

Monitoring Trident
------------------

Trident 20.01 provides a set of Prometheus metrics that can be used to obtain
insight on how Trident operates. You can now define a Prometheus target to gather
the metrics exposed by Trident and obtain information on the backends it manages,
the volumes it creates and so on. Trident's metrics are exposed on the target port
``8001``. These metrics are enabled by default when Trident is installed; to disable
them from being reported, you will have to generate custom YAMLs (using the
``--generate-custom-yaml`` flag) and edit them to remove the ``--metrics`` flag
from being invoked for the ``trident-main`` container.

This `blog <https://netapp.io/2020/02/20/prometheus-and-trident/>`_ is a great
place to start. It explains how Prometheus and Grafana can
be used with Trident 20.01 and above to retrieve metrics. The blog explains how you
can run Prometheus as an operator in your Kubernetes cluster and the creation of a
ServiceMonitor to obtain Trident's metrics.

Uninstalling Trident
--------------------

Depending on how Trident is installed, there are multiple options to uninstall
Trident.

Uninstalling with the Trident Operator
**************************************

If you have installed Trident using the :ref:`operator <deploying-with-operator>`,
you can uninstall Trident by either:

1. **Editing the TridentProvisioner to set the uninstall flag:** You can
   edit the TridentProvisioner and set ``spec.uninstall=true`` to 
   uninstall Trident.

2. **Deleting the TridentProvisioner:** By removing the ``TridentProvisioner``
   CR that was used to deploy Trident, you instruct the operator to
   uninstall Trident. The operator processes the removal of the
   TridentProvisioner and proceeds to remove the Trident deployment and
   daemonset, deleting the Trident pods it had created on
   installation.

To uninstall Trident, edit the ``TridentProvisioner`` and set the
``uninstall`` flag as shown below:

.. code-block:: bash

  $  kubectl patch tprov <trident-provisioner-name> -n trident --type=merge -p '{"spec":{"uninstall":true}}'

When the ``uninstall`` flag is set to ``true``, the Trident Operator
uninstalls Trident but doesn't remove the TridentProvisioner itself. You
must clean up the TridentProvisioner and create a new one if you want to
install Trident again.

To completely remove Trident (including the CRDs it creates) and effectively
wipe the slate clean, you can edit the ``TridentProvisioner`` to pass the
``wipeout`` option.

.. warning::
      
   You must only consider wiping out the CRDs when performing a complete
   uninstallation. This will completely uninstall Trident and cannot be
   undone. **Do not wipeout the CRDs unless you are looking to start over
   and create a fresh Trident install**.

.. code-block:: bash

   $ kubectl patch tprov <trident-provisioner-name> -n trident --type=merge -p '{"spec":{"wipeout":["crds"],"uninstall":true}}'


This will **completely uninstall Trident and clear all metadata related
to backends and volumes it manages**. Subsequent installations will
be treated as a fresh install.
 
Uninstalling with tridentctl
****************************

The uninstall command in tridentctl will remove all of the
resources associated with Trident except for the CRDs and related objects,
making it easy to run the installer again to update to a more recent version.

.. code-block:: bash

  ./tridentctl uninstall -n <namespace>

To perform a complete removal of Trident, you will need to remove the finalizers
for the CRDs created by Trident and delete the CRDs. Refer the
:ref:`Troubleshooting Guide<Troubleshooting>` for the steps to completely uninstall Trident.

Downgrading Trident
-------------------

Downgrading to a previous release of Trident is **not recommended** and should
not be performed unless absolutely neccessary. Downgrades to versions ``19.04``
and earlier are **not supported**.
Refer the :ref:`downgrade section <Downgrading Trident>` for considerations and
factors that can influence your decision to downgrade.
