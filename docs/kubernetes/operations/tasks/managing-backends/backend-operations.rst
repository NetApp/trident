###########################
Handling backend operations
###########################

Users have two unique options for managing backends:

* Using ``kubectl``: Using the ``TridentBackendConfig`` CRD,
  backends can be created directly using the Kubernetes CLI.
  :ref:`This section <kubectl-backend-management>` explains how operations are
  handled. In addition, :ref:`Manage Trident backends by using kubectl <manage_tbc_backend>`
  talks about the ``TridentBackendConfig`` CRD and explains how it works.
* Using ``tridentctl``: Trident provides a dedicated command-line tool (``tridentctl``)
  that can be used to manage backends. :ref:`This section <tridentctl-backend-management>`
  explains the various operations that can be performed.

Creating a backend configuration
--------------------------------

We have an entire :ref:`backend configuration <Backend configuration>` guide to
help you with this.

.. _kubectl-backend-management:

Backend operations with ``kubectl``
-----------------------------------

Creating a backend
==================

Run the following command:

.. code-block:: bash

  $ kubectl create -f <tbc-backend-file.yaml> -n trident

If you did not specify the ``backendName`` field  in the ``spec``, it defaults
to the ``.metadata.name`` of the ``TridentBackendConfig`` CR.

If the backend creation fails, you can run one of the following commands to view the logs:

.. code-block:: bash

  $ kubectl get tbc <tbc-name> -o yaml -n trident

Alternatively, you can use the ``kubectl -n trident describe tbc <tbc-name>`` command to get details of the event logs.

Deleting a backend
==================

By deleting a ``TridentBackendConfig``, users instruct Trident to delete/retain
backends (based on the ``deletionPolicy``) as well as deleting the ``TridentBackendConfig``.
To delete a backend, ensure that ``deletionPolicy`` is set to ``delete``.
To delete just the ``TridentBackendConfig``, ensure that ``deletionPolicy`` is set
to ``retain``. This will ensure the backend is still present and manageable through
``tridentctl``.

Run the following command:

.. code-block:: bash

  $ kubectl delete tbc <tbc-name> -n trident

Trident does not delete the Kubernetes Secrets that were in use by the
``TridentBackendConfig``. The Kubernetes user is responsible for cleaning up secrets.
Care must be taken when deleting secrets; only delete secrets if they are **not in
use** by Trident backends.

Viewing the existing backends
=============================

To view the backends that have been created through ``kubectl``:

.. code-block:: bash

  $ kubectl get tbc -n trident

You can also run ``tridentctl get backend -n trident`` or ``tridentctl get backend -o yaml -n trident``
to obtain a list all backends that exist. This list will also include backends that
were created with ``tridentctl``.

Updating a backend
==================

There can be multiple reasons to update a backend:

1. Credentials to the storage system have changed.
2. Parameters (such as the name of the ONTAP SVM being used) need to be updated.

In the case of (1), the Kubernetes Secret that is used in the ``TridentBackendConfig``
object must be updated. Trident will automatically update the backend with the
latest credentials provided.

.. code-block:: bash

  # Credentials need to be updated?
  $ kubectl apply -f <updated-secret-file.yaml> -n trident

In the case of (2), ``TridentBackendConfig`` objects can be updated directly
through Kubernetes.

.. code-block:: bash

  # TridentBackendConfig needs to be updated?
  $ kubectl apply -f <updated-backend-file.yaml>

Alternatively, make changes to the existing ``TridentBackendConfig`` CR by running:

.. code-block:: bash

  # TridentBackendConfigs can be edited using kubectl
  $ kubectl edit tbc <tbc-name> -n trident

If a backend update fails, the backend continues to remain in its last known configuration.
You can view the logs to determine the cause by running ``kubectl get tbc <tbc-name> -o yaml -n trident``
or ``kubectl describe tbc <tbc-name> -n trident``.

After you identify and correct the problem with the configuration file, you can re-run the update command.

.. _tridentctl-backend-management:

Backend operations with ``tridentctl``
--------------------------------------

Use the information below to manage backends with ``tridentctl``.

Creating a backend
==================

Once you have a :ref:`backend configuration <Backend configuration>` file, run:

.. code-block:: bash

  $ tridentctl create backend -f <backend-file> -n trident

If backend creation fails, something was wrong with the backend configuration.
You can view the logs to determine the cause by running:

.. code-block:: bash

  $ tridentctl logs -n trident

Once you identify and correct the problem with the configuration file you can
simply run the create command again.

Deleting a backend
==================

.. note::

  If Trident has provisioned volumes and snapshots from this backend that still exist,
  deleting the backend will prevent new volumes from being provisioned by it.
  The backend will continue to exist in a "Deleting" state and Trident
  will continue to manage those volumes and snapshots until they are deleted.

To delete a backend from Trident, run:

.. code-block:: bash

  # Retrieve the backend name
  $ tridentctl get backend -n trident

  $ tridentctl delete backend <backend-name> -n trident

Viewing the existing backends
=============================

To view the backends that Trident knows about, run:

.. code-block:: bash

  # Summary
  $ tridentctl get backend -n trident

  # Full details
  $ tridentctl get backend -o json -n trident

Updating a backend
==================

Once you have a new :ref:`backend configuration <Backend configuration>` file, run:

.. code-block:: bash

  $ tridentctl update backend <backend-name> -f <backend-file> -n trident

If backend update fails, something was wrong with the backend configuration or
you attempted an invalid update.
You can view the logs to determine the cause by running:

.. code-block:: bash

  $ tridentctl logs -n trident

Once you identify and correct the problem with the configuration file you can
simply run the update command again.

Identifying the storage classes that will use a backend
-------------------------------------------------------

This is an example of the kind of questions you can answer with the JSON that
``tridentctl`` outputs for Trident backend objects. This uses the ``jq``
utility, which you may need to install first.

.. code-block:: bash

  $ tridentctl get backend -o json | jq '[.items[] | {backend: .name, storageClasses: [.storage[].storageClasses]|unique}]'

This also applies for backends that were created using ``TridentBackendConfig``.
