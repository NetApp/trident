#################
Upgrading Trident
#################

Trident follows a quarterly release cadence, delivering 4 releases every calendar
year. Each new release builds on top of the previous releases, providing new
features and performance enhancements as well as bug fixes and improvements. Users
are always encouraged to upgrade Trident at least once a year in order to take
advantage of the great new features Trident has to offer.

.. important::
   Upgrading to a Trident release that is more than a year ahead (5 releases
   ahead and above) will require you to perform a multi-step upgrade.

This section walks you through the upgrade process to move to the
latest release of Trident.

Determining your upgrade candidate
----------------------------------

Trident recommends upgrading to the ``YY.MM`` release from the ``YY-1.MM`` release
and any in between; for example, to perform a direct upgrade to ``20.07`` it is
recommended to do so from ``19.07`` and above (including dot releases such as
``19.07.1``). If you are working with an earlier release ( < ``YY-1.MM``), you
must perform a multi-step upgrade. This will require you to first move to the
most recent release that fits your four-release window.

If you are running ``18.07`` and seek to upgrade to the ``20.07`` release, then:

1. You must first upgrade from ``18.07`` to ``19.07``. Refer to the documentation
   of the respective release to obtain specific instructions for upgrading.
2. Upgrade from ``19.07`` to ``20.07`` using the instructions provided below.

.. important::
   All upgrades for versions ``19.04`` and earlier will require the migration of
   Trident's metadata from it's own etcd to CRD objects. Make sure you check the
   documentation of the release to understand how the upgrade will work.

.. important::

  If you are running Kubernetes 1.17 or later, and are looking to upgrade to
  ``20.07.1``, you should specify ``parameter.fsType`` (file system type) in
  StorageClasses used by Trident. This is a **requirement** for
  enforcing `Security Contexts <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_
  for SAN volumes. You can delete and re-create StorageClass objects without
  disrupting pre-existing volumes.
  The `sample-input <https://github.com/NetApp/trident/tree/stable/v20.07/trident-installer/sample-input>`_
  directory contains examples, such as
  `storage-class-basic.yaml.templ <https://github.com/NetApp/trident/blob/stable/v20.07/trident-installer/sample-input/storage-class-basic.yaml.templ>`_,
  and `storage-class-bronze-default.yaml <https://github.com/NetApp/trident/blob/stable/v20.07/trident-installer/sample-input/storage-class-bronze-default.yaml>`_.
  For more information, take a look at the :ref:`Known issues <fstype-fix>` tab.

Understanding your upgrade paths
--------------------------------

When upgrading to ``20.07``, users have two distinct paths:

1. Using the Trident Operator.
2. Using ``tridentctl``.

We will examine both here.

.. warning::

   Trident only supports the beta feature release of Volume Snapshots. When upgrading
   Trident, all previous alpha snapshot CRs and CRDs (Volume Snapshot Classes,
   Volume Snapshots and Volume Snapshot Contents) must be removed before the upgrade is performed.
   Refer to `this blog <https://netapp.io/2020/01/30/alpha-to-beta-snapshots/>`_ to understand the
   steps involved in migrating alpha snapshots to the beta spec.

Which one do I choose?
----------------------

This is an important question. Based on how you choose to install and use Trident,
there could be a number of good reasons to choose one over the other.

Trident Operator upgrades
~~~~~~~~~~~~~~~~~~~~~~~~~

There are multiple reasons to want to use the Trident Operator, as detailed
:ref:`here <Why should I use the Trident Operator?>`.

You can use the Trident Operator to upgrade as long as you are:

1. Running CSI Trident (``19.07`` and above).
2. Using a CRD-based Trident release (``19.07`` and above).
3. **not** performing a customized install (using custom YAMLs). To take a look
   at the customization that can be done with an operator install, check out the
   :ref:`Customizing your deployment <operator-customize>` section.

.. warning::

   Do not use the operator to upgrade Trident if you are using an etcd based
   Trident release (``19.04`` or earlier).

For a complete list of prerequisites, click :ref:`here <operator-prereq>`.

``tridentctl`` upgrades
~~~~~~~~~~~~~~~~~~~~~~~

If you are not interested in what the Trident Operator has to offer (or) you have
a customized install that cannot be supported by the operator, you can always
choose to upgrade using ``tridentctl``. This is the preferred method of updating
for Trident releases ``19.04`` and earlier.
