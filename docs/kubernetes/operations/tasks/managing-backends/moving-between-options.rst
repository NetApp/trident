.. _moving-between-options:

#########################################
Moving between backend management options
#########################################

With the introduction of ``TridentBackendConfig``, Trident administrators now
have two unique ways of managing backends. This poses multiple questions:

* Can backends created using ``tridentctl`` be managed with ``TridentBackendConfig``?
* Can backends created using ``TridentBackendConfig`` be managed using ``tridentctl``?

This page provides answers in detail. If you are not familiar with what a
``TridentBackendConfig`` is, you can learn about it here: :ref:`Managing backends using kubectl <manage_tbc_backend>`.

.. _tridentctl-to-kubectl:

Managing ``tridentctl`` backends using ``TridentBackendConfig``
---------------------------------------------------------------

This section covers the steps required to manage backends that were created using ``tridentctl``
directly through the Kubernetes interface by creating ``TridentBackendConfig`` objects.

This will apply for:

* Pre-existing backends, that don't have a ``TridentBackendConfig`` since they
  were created with ``tridentctl``.
* New Trident backends that were created with ``tridentctl``, while other
  ``TridentBackendConfig`` objects exist.

In both cases, backends will continue to be present, with Trident scheduling
volumes and operating on them. Trident admins have one of two choices here:

* Continue using ``tridentctl`` to manage backends that were created using it.
* Bind backends created using ``tridentctl`` to a new ``TridentBackendConfig``
  object. Doing so would mean the backends will be managed using ``kubectl``
  and **not** ``tridentctl``.

To manage a pre-existing backend using ``kubectl``, you will need to create a
``TridentBackendConfig`` that binds to the existing backend. Here is how that works:

1. Create a `Kubernetes Secret <https://kubernetes.io/docs/concepts/configuration/secret/>`_.
   The secret contains the credentials Trident needs to communicate with the storage cluster/service.
2. Create a ``TridentBackendConfig`` object. This contains specifics about the
   storage cluster/service and references the secret created in the previous step.
   Care must be taken to specify identical config parameters (such as ``spec.backendName``,
   ``spec.storagePrefix``, ``spec.storageDriverName``, and so on). ``spec.backendName``
   **must be set to the name of the existing backend**.

Step 0: Identify the backend
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

To create a ``TridentBackendConfig`` that binds to an existing backend, you will
need to obtain the backend's configuration. In this example, let us assume a
backend was created using the following JSON definition:

.. code-block:: bash

  $ tridentctl get backend ontap-nas-backend -n trident
  +---------------------+----------------+--------------------------------------+--------+---------+
  |          NAME       | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
  +---------------------+----------------+--------------------------------------+--------+---------+
  | ontap-nas-backend   | ontap-nas      | 52f2eb10-e4c6-4160-99fc-96b3be5ab5d7 | online |      25 |
  +---------------------+----------------+--------------------------------------+--------+---------+

  $ cat ontap-nas-backend.json

  {
      "version": 1,
      "storageDriverName": "ontap-nas",
      "managementLIF": "10.10.10.1",
      "dataLIF": "10.10.10.2",
      "backendName": "ontap-nas-backend",
      "svm": "trident_svm",
      "username": "cluster-admin",
      "password": "admin-password",

      "defaults": {
          "spaceReserve": "none",
          "encryption": "false"
      },
      "labels":{"store":"nas_store"},
      "region": "us_east_1",
      "storage": [
          {
              "labels":{"app":"msoffice", "cost":"100"},
              "zone":"us_east_1a",
              "defaults": {
                  "spaceReserve": "volume",
                  "encryption": "true",
                  "unixPermissions": "0755"
              }
          },
          {
              "labels":{"app":"mysqldb", "cost":"25"},
              "zone":"us_east_1d",
              "defaults": {
                  "spaceReserve": "volume",
                  "encryption": "false",
                  "unixPermissions": "0775"
              }
          }
      ]
  }

Step 1: Create a Kubernetes Secret
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Create a Secret that contains the credentials for the backend, as shown in
this example:

.. code-block:: bash

  $ cat tbc-ontap-nas-backend-secret.yaml

  apiVersion: v1
  kind: Secret
  metadata:
    name: ontap-nas-backend-secret
  type: Opaque
  stringData:
    username: cluster-admin
    passWord: admin-password

  $ kubectl create -f tbc-ontap-nas-backend-secret.yaml -n trident
  secret/backend-tbc-ontap-san-secret created

Step 2: Create a ``TridentBackendConfig`` CR
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The next step is to create a ``TridentBackendConfig`` CR that will automatically
bind to the pre-existing ``ontap-nas-backend``. Ensure the following requirements
are met:

* The **same backend name** is defined in ``spec.backendName``.
* Configuration parameters are **identical** to the original backend.
* Virtual Storage Pools (if present) **must retain the same order** as in the original
  backend.
* Credentials are provided through a Kubernetes Secret and **not in plain-text**.

In this case, the ``TridentBackendConfig`` will look like this:

.. code-block:: bash

  $ cat backend-tbc-ontap-nas.yaml
  apiVersion: trident.netapp.io/v1
  kind: TridentBackendConfig
  metadata:
    name: tbc-ontap-nas-backend
  spec:
    version: 1
    storageDriverName: ontap-nas
    managementLIF: 10.10.10.1
    dataLIF: 10.10.10.2
    backendName: ontap-nas-backend
    svm: trident_svm
    credentials:
      name: mysecret
    defaults:
      spaceReserve: none
      encryption: 'false'
    labels:
      store: nas_store
    region: us_east_1
    storage:
    - labels:
        app: msoffice
        cost: '100'
      zone: us_east_1a
      defaults:
        spaceReserve: volume
        encryption: 'true'
        unixPermissions: '0755'
    - labels:
        app: mysqldb
        cost: '25'
      zone: us_east_1d
      defaults:
        spaceReserve: volume
        encryption: 'false'
        unixPermissions: '0775'

  $ kubectl create -f backend-tbc-ontap-nas.yaml -n trident
  tridentbackendconfig.trident.netapp.io/tbc-ontap-nas-backend created

Step 3: Verify the status of the ``TridentBackendConfig`` CR
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

After the ``TridentBackendConfig`` has been created, its phase must be ``Bound``.
It should also reflect the same backend name and UUID as that of the existing backend.

.. code-block:: bash

  $ kubectl -n trident get tbc tbc-ontap-nas-backend -n trident
  NAME                   BACKEND NAME          BACKEND UUID                           PHASE   STATUS
  tbc-ontap-nas-backend  ontap-nas-backend     52f2eb10-e4c6-4160-99fc-96b3be5ab5d7   Bound   Success

  #confirm that no new backends were created (i.e., TridentBackendConfig did not end up creating a new backend)
  $ tridentctl get backend -n trident
  +---------------------+----------------+--------------------------------------+--------+---------+
  |          NAME       | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
  +---------------------+----------------+--------------------------------------+--------+---------+
  | ontap-nas-backend   | ontap-nas      | 52f2eb10-e4c6-4160-99fc-96b3be5ab5d7 | online |      25 |
  +---------------------+----------------+--------------------------------------+--------+---------+

The backend will now be completely managed using the ``tbc-ontap-nas-backend`` ``TridentBackendConfig``
object.

.. _kubectl-to-tridentctl:

Managing ``TridentBackendConfig`` backends using ``tridentctl``
---------------------------------------------------------------

``tridentctl`` can be used to list backends that were created using ``TridentBackendConfig``.
In addition, Trident admins can also choose to completely manage such backends
through ``tridentctl`` by deleting the ``TridentBackendConfig`` and making sure
``spec.deletionPolicy`` is set to ``retain``.

Step 0: Identify the Backend
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

For example, let us assume the following backend was created using ``TridentBackendConfig``:

.. code-block:: bash

  $ kubectl get tbc backend-tbc-ontap-san -n trident -o wide
  NAME                    BACKEND NAME        BACKEND UUID                           PHASE   STATUS    STORAGE DRIVER   DELETION POLICY
  backend-tbc-ontap-san   ontap-san-backend   81abcb27-ea63-49bb-b606-0a5315ac5f82   Bound   Success   ontap-san        delete

  $ tridentctl get backend ontap-san-backend -n trident
  +-------------------+----------------+--------------------------------------+--------+---------+
  |       NAME        | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
  +-------------------+----------------+--------------------------------------+--------+---------+
  | ontap-san-backend | ontap-san      | 81abcb27-ea63-49bb-b606-0a5315ac5f82 | online |      33 |
  +-------------------+----------------+--------------------------------------+--------+---------+

From the output, it is seen that the ``TridentBackendConfig`` was created successfully and is
bound to a Trident backend [observe the backend's UUID].

Step 1: Confirm ``deletionPolicy`` is set to ``retain``
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Let us take a look at the value of ``deletionPolicy``. This **needs to be set** to
``retain``. This will ensure that when a ``TridentBackendConfig`` CR is deleted,
the backend definition will still be present and can be managed with ``tridentctl``.

.. code-block:: bash

  $ kubectl get tbc backend-tbc-ontap-san -n trident -o wide
  NAME                    BACKEND NAME        BACKEND UUID                           PHASE   STATUS    STORAGE DRIVER   DELETION POLICY
  backend-tbc-ontap-san   ontap-san-backend   81abcb27-ea63-49bb-b606-0a5315ac5f82   Bound   Success   ontap-san        delete

  # Patch value of deletionPolicy to retain
  $ kubectl patch tbc backend-tbc-ontap-san --type=merge -p '{"spec":{"deletionPolicy":"retain"}}' -n trident
  tridentbackendconfig.trident.netapp.io/backend-tbc-ontap-san patched

  #Confirm the value of deletionPolicy
  $ kubectl get tbc backend-tbc-ontap-san -n trident -o wide
  NAME                    BACKEND NAME        BACKEND UUID                           PHASE   STATUS    STORAGE DRIVER   DELETION POLICY
  backend-tbc-ontap-san   ontap-san-backend   81abcb27-ea63-49bb-b606-0a5315ac5f82   Bound   Success   ontap-san        retain

**Do not proceed to the next step unless** ``deletionPolicy`` **equals**
``retain``.

Step 2: Delete the ``TridentBackendConfig`` CR
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The final step is to delete the ``TridentBackendConfig`` CR. After confirming the
``deletionPolicy`` is set to ``retain``, you can go ahead with the deletion:

.. code-block:: bash

  $ kubectl delete tbc backend-tbc-ontap-san -n trident
  tridentbackendconfig.trident.netapp.io "backend-tbc-ontap-san" deleted

  $ tridentctl get backend ontap-san-backend -n trident
  +-------------------+----------------+--------------------------------------+--------+---------+
  |       NAME        | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
  +-------------------+----------------+--------------------------------------+--------+---------+
  | ontap-san-backend | ontap-san      | 81abcb27-ea63-49bb-b606-0a5315ac5f82 | online |      33 |
  +-------------------+----------------+--------------------------------------+--------+---------+

Upon the deletion of the ``TridentBackendConfig`` object, Trident simply removes it
without actually deleting the backend itself.
