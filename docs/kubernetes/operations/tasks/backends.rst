#################
Managing backends
#################

Creating a backend configuration
--------------------------------

We have an entire :ref:`backend configuration <Backend configuration>` guide to
help you with this.

Creating a backend
------------------

Once you have a :ref:`backend configuration <Backend configuration>` file, run:

.. code-block:: bash

  tridentctl create backend -f <backend-file>

If backend creation fails, something was wrong with the backend configuration.
You can view the logs to determine the cause by running:

.. code-block:: bash

  tridentctl logs

Once you identify and correct the problem with the configuration file you can
simply run the create command again.

Deleting a backend
------------------

.. note::
  If Trident has provisioned volumes from this backend that still exist,
  deleting the backend will prevent new volumes from being provisioned by it
  but the backend will continue to exist and Trident will continue to manage
  those volumes until they are deleted.

To delete a backend from Trident, run:

.. code-block:: bash

  # Retrieve the backend name
  tridentctl get backend

  tridentctl delete backend <backend-name>

Viewing the existing backends
-----------------------------

To view the backends that Trident knows about, run:

.. code-block:: bash

  # Summary
  tridentctl get backend

  # Full details
  tridentctl get backend -o json

Identifying the storage classes that will use a backend
-------------------------------------------------------

This is an example of the kind of questions you can answer with the JSON that
``tridentctl`` outputs for Trident backend objects. This uses the ``jq``
utility, which you may need to install first.

.. code-block:: bash

  tridentctl get backend -o json | jq '[.items[] | {backend: .name, storageClasses: [.storage[].storageClasses]|unique}]'

Updating a backend
------------------

Once you have a new :ref:`backend configuration <Backend configuration>` file, run:

.. code-block:: bash

  tridentctl update backend <backend-name> -f <backend-file>

If backend update fails, something was wrong with the backend configuration or
you attempted an invalid update.
You can view the logs to determine the cause by running:

.. code-block:: bash

  tridentctl logs

Once you identify and correct the problem with the configuration file you can
simply run the update command again.
