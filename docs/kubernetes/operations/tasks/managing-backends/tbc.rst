.. _manage_tbc_backend:

###################################
Managing backends using ``kubectl``
###################################

After Trident is installed, the next step is to create a Trident backend. Trident
introduces the ``TridentBackendConfig`` Custom Resource Definition (CRD) for users
to create and manage Trident backends directly through the Kubernetes interface.
You can do this by using ``kubectl`` or the equivalent CLI tool for your
Kubernetes distribution.

``TridentBackendConfig`` (``tbc``, ``tbconfig``, ``tbackendconfig``) is a
frontend, namespaced CRD that enables users to manage Trident backends using ``kubectl``.
Kubernetes and storage admins can now create and manage backends directly through
the Kubernetes CLI without requiring a dedicated command-line utility (``tridentctl``).
Upon the creation of a ``TridentBackendConfig`` object:

1. A backend is created automatically by Trident based on the configuration provided.
   This is represented internally as a ``TridentBackend`` (``tbe``, ``tridentbackend``) CR.
2. The ``TridentBackendConfig`` is uniquely bound to a ``TridentBackend`` that
   was created by Trident.

Each ``TridentBackendConfig`` maintains a 1-to-1 mapping with a ``TridentBackend``.
The former is the interface provided to the user to design and configure
backends; the latter is how Trident represents the actual backend object.

.. warning::

	``TridentBackend`` CRs are created by Trident automatically. They **must not be modified**. Updates to backends must **always** be handled by modifying the ``TridentBackendConfig`` object.

To create a new backend, you should do the following:

* Create a `Kubernetes Secret <https://kubernetes.io/docs/concepts/configuration/secret/>`_. The secret contains the credentials Trident needs to communicate with the storage cluster/service.
* Create a ``TridentBackendConfig`` object. This contains specifics about the storage cluster/service and references the secret created in the previous step.

After you create a Trident backend, you can observe its status by using ``kubectl`` [``kubectl get tbc <tbc-name> -n <trident-namespace>``] and gather additional details.

See the following example for the format of the ``TridentBackendConfig`` CR:

.. code-block:: console

    apiVersion: trident.netapp.io/v1
    kind: TridentBackendConfig
    metadata:
      name: backend-tbc-ontap-san
    spec:
      version: 1
      backendName: ontap-san-backend
      storageDriverName: ontap-san
      managementLIF: 10.0.0.1
      dataLIF: 10.0.0.2
      svm: trident_svm
      credentials:
        name: backend-tbc-ontap-san-secret

You can also take a look at the examples present in the
`trident-installer <https://github.com/NetApp/trident/tree/stable/v21.07/trident-installer/sample-input/backends-samples>`_
directory for sample configurations for the desired storage platform/service.

The ``spec`` takes backend-specific configuration parameters. In this example,
the backend uses the ``ontap-san`` storage driver and uses the configuration
parameters that are tabulated :ref:`here <ontap-san-configuration-parameters>`.
To obtain the list of configuration options for your desired storage driver,
make sure to visit its dedicated backend configuration guide from the
:ref:`Backend Configuration landing page <Backend configuration>`.

The ``spec`` section also includes ``credentials`` and ``deletionPolicy`` fields,
which are newly introduced in the ``TridentBackendConfig`` CR:

* ``credentials``: This parameter is a **required field** and contains the
  credentials used to authenticate with the storage system/service. This is set
  to a user-created Kubernetes Secret. The credentials cannot be passed
  in plain text and will result in an error.
* ``deletionPolicy``: This field defines what should happen when the ``TridentBackendConfig``
  is deleted. It can take one of two possible values:
    * **delete**: This results in the deletion of both ``TridentBackendConfig`` CR
      and the associated Trident backend. This is the default value.
    * **retain**: When a ``TridentBackendConfig`` CR is deleted, the backend
      definition will still be present and can be managed with ``tridentctl``. Setting the
      deletion policy to ``retain`` lets users downgrade to an earlier release (pre-21.04)
      and retain created backends.

  The value for this field can be updated after a ``TridentBackendConfig`` was created.

.. note::

  The name of a backend is set using ``spec.backendName``. If unspecified, the name of the
  backend is set to the name of the ``TridentBackendConfig`` object (``metadata.name``).
  It is recommended to **explicitly set backend names** using ``spec.backendName``.

.. note::

   Backends that were created with ``tridentctl`` **will not** have an associated ``TridentBackendConfig``
   object. You can choose to manage such backends through ``kubectl`` by creating a ``TridentBackendConfig``
   CR. Care must be taken to specify **identical config parameters** (such as ``spec.backendName``,
   ``spec.storagePrefix``, ``spec.storageDriverName``, and so on). Trident will automatically
   bind the newly-created ``TridentBackendConfig`` with the pre-existing backend. This is covered in detail
   with an example in :ref:`Moving between backend management options <moving-between-options>`.

Step 1: Create a Kubernetes Secret
----------------------------------

You will first create a Secret that contains the access credentials for the backend.
This is unique to each storage service/platform. Here's an example:

.. code-block:: console

  $ kubectl -n trident create -f backend-tbc-ontap-san-secret.yaml

.. code-block:: console

    apiVersion: v1
    kind: Secret
    metadata:
      name: backend-tbc-ontap-san-secret
    type: Opaque
    stringData:
      username: cluster-admin
      password: t@Ax@7q(>

This table summarizes the fields that must be included in the Secret for each
Storage Platform:

+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
| Storage Platform              | Secret Fields                 | Description                                                                                                    |
+===============================+===============================+================================================================================================================+
| Azure NetApp Files            | clientID                      | The client ID from an App Registration.                                                                        |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | clientSecret                  | The client secret from an App Registration.                                                                    |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
| Cloud Volumes Service for AWS | apiKey                        | CVS account API key.                                                                                           |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | secretKey                     | CVS account secret key.                                                                                        |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
| Cloud Volumes Service for GCP | private_key_id                | ID of the private key. Part of API key for GCP Service Account with CVS admin role.                            |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | private_key                   | Private key. Part of API key for GCP Service Account with CVS admin role.                                      |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
| Element (HCI/SolidFire)       | Endpoint                      | MVIP for the SolidFire cluster with tenant credentials.                                                        |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
| ONTAP                         | username                      | Username to connect to the cluster/SVM. Used for credential-based auth.                                        |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | password                      | Password to connect to the cluster/SVM. Used for credential-based auth.                                        |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | clientPrivateKey              | Base64-encoded value of client private key. Used for certificate-based auth.                                   |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | chapUsername                  | Inbound username. Required if ``useCHAP=true``. NOTE: For ``ontap-san`` and ``ontap-san-economy``.             |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | chapInitiatorSecret           | CHAP initiator secret. Required if ``useCHAP=true``. NOTE: For ``ontap-san`` and ``ontap-san-economy``.        |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | chapTargetUsername            | Target username. Required if ``useCHAP=true``. NOTE: For ``ontap-san`` and ``ontap-san-economy``.              |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | chapTargetInitiatorSecret     | CHAP target initiator secret. Required if ``useCHAP=true``. NOTE: For ``ontap-san`` and ``ontap-san-economy``. |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
| SANtricity (E-Series)         | username                      | Username for the web services proxy                                                                            |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | password                      | Password for the web services proxy                                                                            |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+
|                               | passwordArray                 | Password for the storage array, if set                                                                         |
+-------------------------------+-------------------------------+----------------------------------------------------------------------------------------------------------------+

The Secret created in this step will be referenced in the ``spec.credentials`` field of the ``TridentBackendConfig`` object that is created in the next step.

Step 2: Create the ``TridentBackendConfig`` CR
----------------------------------------------

You are now ready to create your ``TridentBackendConfig`` CR. In this example, a
Trident backend that uses the ``ontap-san`` driver is created by using the
``TridentBackendConfig`` object shown below:

.. code-block:: console

  $ kubectl -n trident create -f backend-tbc-ontap-san.yaml

.. code-block:: console

    apiVersion: trident.netapp.io/v1
    kind: TridentBackendConfig
    metadata:
      name: backend-tbc-ontap-san
    spec:
      version: 1
      backendName: ontap-san-backend
      storageDriverName: ontap-san
      managementLIF: 10.0.0.1
      dataLIF: 10.0.0.2
      svm: trident_svm
      credentials:
        name: backend-tbc-ontap-san-secret

Step 3: Verify the status of the ``TridentBackendConfig`` CR
------------------------------------------------------------

Now that you created the ``TridentBackendConfig`` CR, you can verify the status. See the following example:

.. code-block:: console

    $ kubectl -n trident get tbc backend-tbc-ontap-san
    NAME                    BACKEND NAME          BACKEND UUID                           PHASE   STATUS
    backend-tbc-ontap-san   ontap-san-backend     8d24fce7-6f60-4d4a-8ef6-bab2699e6ab8   Bound   Success

A backend was successfully created and bound to the ``TridentBackendConfig`` CR.

Phase can take one of the following values:

* **Bound**: The ``TridentBackendConfig`` CR is associated with a Trident backend,
  and that backend contains ``configRef`` set to the ``TridentBackendConfig`` CR's
  ``uid``.
* **Unbound**: Represented using ``""``. The ``TridentBackendConfig`` object is
  not bound to a Trident backend. All newly created  ``TridentBackendConfig``
  CRs are in this phase by default. After the phase changes, it cannot revert to
  ``Unbound`` again.
* **Deleting**: The ``TridentBackendConfig`` CR's ``deletionPolicy`` was set to
  ``delete``. When the ``TridentBackendConfig`` CR is deleted, it transitions to
  the ``Deleting`` state.
    * If no PVCs exist on the backend, deleting the ``TridentBackendConfig`` will
      result in Trident deleting the backend as well as the ``TridentBackendConfig``
      CR.
    * If one or more PVCs are present on the backend, it goes to a ``deleting``
      state. The ``TridentBackendConfig`` CR subsequently also enters ``deleting``
      phase. The backend and ``TridentBackendConfig`` are deleted only
      **after all PVCs are deleted**.
* **Lost**: The Trident backend associated with the ``TridentBackendConfig`` CR was accidentally or deliberately deleted and the ``TridentBackendConfig`` CR still has a reference to the deleted backend. The ``TridentBackendConfig`` CR can still be deleted irrespective of the ``deletionPolicy`` value.
* **Unknown**: Trident is unable to determine the state or existence of the Trident backend associated with the ``TridentBackendConfig`` CR. For example, if the API server is not responding or if the ``tridentbackends.trident.netapp.io`` CRD is missing. This might require the user's intervention.

At this stage, a backend is successfully created! There are several operations
that can additionally be handled, such as:

* **Backend Updates**: These could include credential rotation and/or
  backend configuration updates.
* **Backend Deletions**.

This is covered in :ref:`Backend operations with kubectl <kubectl-backend-management>`.

To understand the considerations involved in moving between ``tridentctl`` and
``TridentBackendConfig`` to manage backends, be sure to read
:ref:`Moving between backend management options <moving-between-options>`.

(Optional) Step 4: Get more details
-----------------------------------

You can run the following command to get more information about your backend:

.. code-block:: console

  kubectl -n trident get tbc backend-tbc-ontap-san -o wide

  NAME                    BACKEND NAME        BACKEND UUID                           PHASE   STATUS    STORAGE DRIVER   DELETION POLICY
  backend-tbc-ontap-san   ontap-san-backend   8d24fce7-6f60-4d4a-8ef6-bab2699e6ab8   Bound   Success   ontap-san        delete

In addition, you can also obtain a YAML/JSON dump of the ``TridentBackendConfig``.

.. code-block:: console

  $ kubectl -n trident get tbc backend-tbc-ontap-san -o yaml

  apiVersion: trident.netapp.io/v1
  kind: TridentBackendConfig
  metadata:
    creationTimestamp: "2021-04-21T20:45:11Z"
    finalizers:
    - trident.netapp.io
    generation: 1
    name: backend-tbc-ontap-san
    namespace: trident
    resourceVersion: "947143"
    uid: 35b9d777-109f-43d5-8077-c74a4559d09c
  spec:
    backendName: ontap-san-backend
    credentials:
      name: backend-tbc-ontap-san-secret
    managementLIF: 10.0.0.1
    dataLIF: 10.0.0.2
    storageDriverName: ontap-san
    svm: trident_svm
    version: 1
  status:
    backendInfo:
      backendName: ontap-san-backend
      backendUUID: 8d24fce7-6f60-4d4a-8ef6-bab2699e6ab8
    deletionPolicy: delete
    lastOperationStatus: Success
    message: Backend 'ontap-san-backend' created
    phase: Bound

The ``backendInfo`` contains the ``backendName`` and the ``backendUUID`` of the
Trident backend that got created in response to the ``TridentBackendConfig`` CR.
The ``lastOperationStatus`` field represents the status of the last operation of
the ``TridentBackendConfig`` CR, which can be user-triggered (for example, user
changed something in ``spec``) or triggered by Trident (for example, during
Trident restarts). It can either be Success or Failed. ``phase`` represents the
status of the relation between the ``TridentBackendConfig`` CR and the Trident
backend. In the example above, ``phase`` has the value ``Bound``, which means
that the ``TridentBackendConfig`` CR is associated with the Trident backend.

You can use the ``kubectl -n trident describe tbc <tbc-cr-name>`` command to get details of the event logs.

.. warning::

  You **cannot** update or delete a backend which contains an associated ``TridentBackendConfig``
  object using ``tridentctl``. To understand the steps involved in switching
  between ``tridentctl`` and ``TridentBackendConfig``, refer
  :ref:`Moving between backend management options <moving-between-options>`.
