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

Downgrading to a previous release of Trident is **not recommended**.
Refer the :ref:`Troubleshooting Guide<Troubleshooting>` for downgrading Trident to
a previous release.
