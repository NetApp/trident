########################
Managing storage classes
########################

Designing a storage class
-------------------------

The :ref:`StorageClass concept guide <Kubernetes StorageClass objects>` will
help you understand what they do and how you configure them.

Creating a storage class
------------------------

Once you have a storage class file, run:

.. code-block:: bash

  kubectl create -f <storage-class-file>

Deleting a storage class
------------------------

To delete a storage class from Kubernetes, run:

.. code-block:: bash

  kubectl delete storageclass <storage-class>

Any persistent volumes that were created through this storage class will
remain untouched, and Trident will continue to manage them.

.. note::

  Trident enforces a blank ``fsType`` for the volumes it creates. For iSCSI backends,
  it is recommended to enforce ``parameters.fsType`` in the StorageClass.
  Existing StorageClasses must be deleted and recreated with ``parameters.fsType``
  specified.

Viewing the existing storage classes
------------------------------------

.. code-block:: bash

  # Kubernetes storage classes
  kubectl get storageclass

  # Kubernetes storage class detail
  kubectl get storageclass <storage-class> -o json

  # Trident's synchronized storage classes
  tridentctl get storageclass

  # Trident's synchronized storage class detail
  tridentctl get storageclass <storage-class> -o json

Setting a default storage class
-------------------------------

Kubernetes v1.6 added the ability to set a default storage class. This is
the storage class that will be used to provision a PV if a user does not
specify one in a PVC.

You can define a default storage class by setting the annotation
``storageclass.kubernetes.io/is-default-class`` to ``true`` in the storage
class definition. According to the specification, any other value or absence of
the annotation is interpreted as ``false``.

It is possible to configure an existing storage class to be the default storage
class by using the following command:

.. code-block:: bash

  kubectl patch storageclass <storage-class-name> -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'

Similarly, you can remove the default storage class annotation by using the
following command:

.. code-block:: bash

  kubectl patch storageclass <storage-class-name> -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"false"}}}'

There are also examples in the Trident installer bundle that include this
annotation.

.. note::
  You should only have one default storage class in your cluster at any given
  time. Kubernetes does not technically prevent you from having more than one,
  but it will behave as if there is no default storage class at all.

Identifying the Trident backends that a storage class will use
--------------------------------------------------------------

This is an example of the kind of questions you can answer with the JSON that
``tridentctl`` outputs for Trident backend objects. This uses the ``jq``
utility, which you may need to install first.

.. code-block:: bash

  tridentctl get storageclass -o json | jq  '[.items[] | {storageClass: .Config.name, backends: [.storage]|unique}]'
