###################################
Upgrading with the Trident Operator
###################################

Trident's operator now supports brownfield installations and provides an easy
upgrade path for users that seek to use the operator and greatly simplify
Trident's operation. This document shall talk about the prerequisites and how
the upgrade will work.

.. _operator-prereq:

Prerequisites
-------------

To upgrade using the Trident operator:

1. Only CSI based Trident Installation may exist. To check if you are running
   CSI Trident, examine the pods in your Trident namespace. If they follow the
   ``trident-csi-*`` naming pattern, you are running CSI Trident. Move on to
   step 2.
2. Only CRD based Trident Installation may exist. This represents all Trident
   releases ``19.07`` and above. If you passed step 1, you have most likely also
   cleared step 2.
3. CRDs that **exist as a result of Trident CSI uninstallation** are supported.
   If you have uninstalled CSI Trident and the metadata from the installation
   persists, you can upgrade using the operator.
4. Only one Trident installation should exist across all the namespaces in a
   given Kubernetes cluster.
5. You must be using a Kubernetes cluster that runs version ``1.14`` and above.
   See :ref:`Requirements <Requirements>`
6. Alpha Snapshot CRDs should not be present. If they are present, you must
   remove them with ``tridentctl obliviate alpha-snapshot-crd``. This will delete
   the CRDs for the alpha snapshot spec. For existing snapshots that should be
   deleted/migrated, please read `this blog`_.

Initiating the upgrade
----------------------

After confirming that you meet the :ref:`Prerequisites <operator-prereq>`, you
are good to go ahead and upgrade using the operator.

Upgrading from the operator
~~~~~~~~~~~~~~~~~~~~~~~~~~~

First, begin by removing any existing Trident Operator deployments. If you have
an instance of the Trident Operator running (of the previous release, for example),
you must remove it.

.. code-block:: bash

  $ cd trident-installer
  $ kubectl delete -f deploy/bundle.yaml

Following this, you can now install the latest release of the operator by
fetching the latest `installer bundle`_ and then installing the operator.

.. code-block:: bash

   # Have you updated the yaml manifests? Generate your bundle.yaml
   # using the kustomization.yaml
   kubectl kustomize deploy/ > deploy/bundle.yaml

   # Create the resources and deploy the operator
   kubectl create -f deploy/bundle.yaml

You do not have to create the Operator CRDs nor the ``tridentprovisioner`` CR
again. Just deploy the operator with the respective Trident Operator image and
you are good to go.

Upgrading from a non-operator install
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If you have a CSI Trident instance that has satisfied the
:ref:`Prerequisites <operator-prereq>`, you can upgrade to the latest release
of the Trident Operator by following the instructions provided in the
:ref:`Operator deployment <deploying-with-operator>`. You must:

1. Download the latest Trident release.
2. Create the ``tridentProvisioner`` CRD from the manifest.
3. Deploy the operator.
4. Create a ``TridentProvisioner`` CR for installing Trident.
5. Existing backends and PVCs will be automatically available.

All of this is documented in thoroughly in the
:ref:`Operator deployment <deploying-with-operator>` section.

.. note::

   You will need to remove alpha snapshot CRDs (if they exist) before upgrading
   using the operator. Use ``tridentctl obliviate alpha-snapshot-crd`` to
   achieve this.

.. _this blog: https://netapp.io/2020/01/30/alpha-to-beta-snapshots/
.. _installer bundle: https://github.com/NetApp/trident/releases/latest
