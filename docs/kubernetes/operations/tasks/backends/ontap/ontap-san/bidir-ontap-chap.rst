.. _ontap-bidir-chap:

#################################
Using CHAP with ONTAP SAN drivers
#################################

Trident 20.04 introduces bidirectional CHAP support for the ``ontap-san`` and
``ontap-san-economy`` drivers. This simplifies the configuration of CHAP on the ONTAP
cluster and provides a convenient method of creating CHAP credentials and rotating
them using ``tridentctl``. Enabling CHAP on the ONTAP backend requires adding the
``useCHAP`` option and the CHAP secrets in your backend configuration as shown below:

Configuration
-------------

.. code::

   {
       "version": 1,
       "storageDriverName": "ontap-san",
       "backendName": "ontap_san_chap",
       "managementLIF": "192.168.0.135",
       "svm": "ontap_iscsi_svm",
       "useCHAP": true,
       "username": "vsadmin",
       "password": "FaKePaSsWoRd",
       "igroupName": "trident",
       "chapInitiatorSecret": "cl9qxIm36DKyawxy",
       "chapTargetInitiatorSecret": "rqxigXgkesIpwxyz",
       "chapTargetUsername": "iJF4heBRT0TCwxyz",
       "chapUsername": "uh2aNCLSd6cNwxyz",
   }

.. warning::

   The ``useCHAP`` parameter is a Boolean option that can be configured only once.
   It is set to ``false`` by default. Once set to ``true``, it cannot be set to
   ``false``. NetApp recommends using Bidirectional CHAP to authenticate connections.

In addition to ``useCHAP=true``, the ``chapInitiatorSecret``,
``chapTargetInitiatorSecret``, ``chapTargetUsername`` and ``chapUsername``
fields **must be included** in the backend definition. The secrets can
be changed after a backend is created using ``tridentctl update``.

How it works
------------

By setting ``useCHAP`` to ``true``, the storage administrator instructs Trident to
configure CHAP on the storage backend. This includes:

1. Setting up CHAP on the SVM:

   a. If the SVM's default initiator security type is ``none`` (set by default)
      **AND** there are no pre-existing LUNs already present in the volume,
      Trident will set the default security type to ``CHAP`` and proceed to
      step 2.
   b. If the SVM contains LUNs, Trident **will not enable CHAP** on the SVM.
      This ensures that access to LUNs that are already present on the SVM isn't
      restricted.

2. Configuring the CHAP initiator and target username and secrets; these options must
   be specified in the backend configuration (as shown above).
3. Managing the addition of inititators to the ``igroupName`` given in the backend. If
   unspecified, this defaults to ``trident``.

Once the backend is created, Trident creates a corresponding ``tridentbackend`` CRD
and stores the CHAP secrets and usernames as Kubernetes secrets. All PVs that are created
by Trident on this backend will be mounted and attached over CHAP.

Rotating credentials and updating backends
------------------------------------------

The CHAP credentials can be rotated by updating the CHAP parameters in
the ``backend.json`` file. This will require updating the CHAP secrets
and using the ``tridentctl update`` command to reflect these changes.

.. warning::

   When updating the CHAP secrets for a backend, you **must use**
   ``tridentctl`` to update the backend. **Do not** update the credentials
   on the storage cluster through the CLI/ONTAP UI as Trident
   will not be able to pick up these changes.

.. code-block:: console

   $ cat backend-san.json
   {
       "version": 1,
       "storageDriverName": "ontap-san",
       "backendName": "ontap_san_chap",
       "managementLIF": "192.168.0.135",
       "svm": "ontap_iscsi_svm",
       "useCHAP": true,
       "username": "vsadmin",
       "password": "FaKePaSsWoRd",
       "igroupName": "trident",
       "chapInitiatorSecret": "cl9qxUpDaTeD",
       "chapTargetInitiatorSecret": "rqxigXgkeUpDaTeD",
       "chapTargetUsername": "iJF4heBRT0TCwxyz",
       "chapUsername": "uh2aNCLSd6cNwxyz",
   }

   $ ./tridentctl update backend ontap_san_chap -f backend-san.json -n trident
   +----------------+----------------+--------------------------------------+--------+---------+
   |   NAME         | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
   +----------------+----------------+--------------------------------------+--------+---------+
   | ontap_san_chap | ontap-san      | aa458f3b-ad2d-4378-8a33-1a472ffbeb5c | online |       7 |
   +----------------+----------------+--------------------------------------+--------+---------+

Existing connections will remain unaffected; they will continue to remain active if the credentials
are updated by Trident on the SVM. New connections will use the updated credentials and existing
connections continue to remain active. Disconnecting and reconnecting old PVs will result in them
using the updated credentials.
