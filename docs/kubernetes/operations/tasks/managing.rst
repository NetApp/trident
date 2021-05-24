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

Uninstalling Trident
--------------------

Depending on how Trident is installed, there are multiple options to uninstall
Trident.

Uninstalling using Helm
***********************

If Trident was installed using Helm, it can be uninstalled using ``helm uninstall``.

.. code-block:: bash

  #List the Helm release corresponding to the Trident install.
  $ helm ls -n trident
  NAME   	NAMESPACE	REVISION	UPDATED                                	STATUS  	CHART                          	APP VERSION
  trident	trident  	1       	2021-04-20 00:26:42.417764794 +0000 UTC	deployed	trident-operator-21.07.0  	21.07.0

  #Uninstall Helm release to remove Trident
  $ helm uninstall trident -n trident
  release "trident" uninstalled

Uninstalling with the Trident Operator
**************************************

If you have installed Trident using the :ref:`operator <deploying-with-operator>`,
you can uninstall Trident by either:

1. **Editing the TridentOrchestrator to set the uninstall flag:** You can
   edit the TridentOrchestrator and set ``spec.uninstall=true`` to
   uninstall Trident.

2. **Deleting the TridentOrchestrator:** By removing the ``TridentOrchestrator``
   CR that was used to deploy Trident, you instruct the operator to
   uninstall Trident. The operator processes the removal of the
   TridentOrchestrator and proceeds to remove the Trident deployment and
   daemonset, deleting the Trident pods it had created on
   installation.

To uninstall Trident, edit the ``TridentOrchestrator`` and set the
``uninstall`` flag as shown below:

.. code-block:: bash

  $  kubectl patch torc <trident-orchestrator-name> --type=merge -p '{"spec":{"uninstall":true}}'

When the ``uninstall`` flag is set to ``true``, the Trident Operator
uninstalls Trident but doesn't remove the TridentOrchestrator itself. You
must clean up the TridentOrchestrator and create a new one if you want to
install Trident again.

To completely remove Trident (including the CRDs it creates) and effectively
wipe the slate clean, you can edit the ``TridentOrchestrator`` to pass the
``wipeout`` option.

.. warning::

   You must only consider wiping out the CRDs when performing a complete
   uninstallation. This will completely uninstall Trident and cannot be
   undone. **Do not wipeout the CRDs unless you are looking to start over
   and create a fresh Trident install**.

.. code-block:: bash

   $ kubectl patch torc <trident-orchestrator-name> --type=merge -p '{"spec":{"wipeout":["crds"],"uninstall":true}}'


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
