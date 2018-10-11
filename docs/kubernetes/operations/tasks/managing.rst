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
  ./tridentctl install   -n <namespace>

By default the uninstall command will leave all of Trident's state intact by
not deleting the PVC and PV used by the Trident deployment, allowing an
uninstall followed by an install to act as an upgrade.

PVs that have already been provisioned will remain available while Trident is
offline, and Trident will provision volumes for any PVCs that are created in
the interim once it is back online.

.. note::
  When using Kubernetes with Docker EE 2.0, you must also provide
  ``--ucp-host`` and ``--ucp-bearer-token`` for the uninstall and install commands::

      UCP_HOST="1.2.3.4"
      EE_USER="admin"
      EE_PASS="password"
      AUTHTOKEN=$(curl -sk -d "{\"username\":\"${EE_USER}\",\"password\":\"${EE_PASS}\"}" https://${UCP_HOST}/auth/login | jq -r .auth_token)
      # ./tridentctl uninstall -n <namespace> --ucp-bearer-token="${AUTHTOKEN}" --ucp-host="${UCP_HOST}"
      # ./tridentctl install   -n <namespace> --ucp-bearer-token="${AUTHTOKEN}" --ucp-host="${UCP_HOST}"

.. warning::
  Trident's support for Docker EE 2.0's UCP access control will be removed in the 
  next release, replaced by the native Kubernetes access control support in
  Docker EE 2.1 and beyond. The ``--ucp-host`` and ``--ucp-bearer-token`` parameters
  will be deprecated and will not be needed in order to install or uninstall Trident.

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

.. note::
  When using Kubernetes with Docker EE 2.0, you must also provide
  ``--ucp-host`` and ``--ucp-bearer-token`` for the uninstall command::

      UCP_HOST="1.2.3.4"
      EE_USER="admin"
      EE_PASS="password"
      AUTHTOKEN=$(curl -sk -d "{\"username\":\"${EE_USER}\",\"password\":\"${EE_PASS}\"}" https://${UCP_HOST}/auth/login | jq -r .auth_token)
      # ./tridentctl uninstall -n <namespace> --ucp-bearer-token="${AUTHTOKEN}" --ucp-host="${UCP_HOST}"

.. warning::
  If you remove Trident's PVC, PV and/or backing volume, you will need to
  reconfigure Trident from scratch if you install it again. Also, it will
  no longer manage any of the PVs it had provisioned.
