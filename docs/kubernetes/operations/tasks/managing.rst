################
Managing Trident
################

Installing Trident
------------------

Follow the extensive :ref:`deployment <deploying-in-kubernetes>` guide.

Updating Trident
----------------

The best way to update to the latest version of Trident is to download the
latest `installer bundle`_ and run:

.. code-block:: bash

  ./tridentctl uninstall -n <namespace>
  ./tridentctl install -n <namespace>

By default the uninstall command will leave all of Trident's state intact by
not deleting the PVC and PV used by the Trident deployment, allowing an
uninstall followed by an install to act as an upgrade.

PVs that have already been provisioned will remain available while Trident is
offline, and Trident will provision volumes for any PVCs that are created in
the interim once it is back online.

.. _installer bundle: https://github.com/NetApp/trident/releases/latest

Uninstalling Trident
--------------------

The uninstall command in tridentctl will remove all of the
resources associated with Trident except for the PVC, PV and backing volume,
making it easy to run the installer again to update to a more recent version.

.. code-block:: bash

  ./tridentctl uninstall -n <namespace>

To fully uninstall Trident and remove the PVC and PV as well, specify the
``-a`` switch. The backing volume on the storage will still need to be removed
manually.

.. warning::
  If you remove Trident's PVC, PV and/or backing volume, you will need to
  reconfigure Trident from scratch if you install it again. Also, it will
  no longer manage any of the PVs it had provisioned.
