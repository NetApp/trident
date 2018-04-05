###################
Element (SolidFire)
###################

To create and use a SolidFire backend, you will need:

* A :ref:`supported SolidFire storage system <Supported backends (storage)>`
* Complete `SolidFire backend preparation`_
* Credentials to a SolidFire cluster admin or tenant user that can manage volumes

.. _SolidFire backend preparation:

Preparation
-----------

All of your Kubernetes worker nodes must have the appropriate iSCSI tools
installed. See the :ref:`worker configuration guide <iSCSI>` for more details.

If you're using CHAP (``UseCHAP`` is *true*), no further preparation is
required. You do have to explicitly set that option to use CHAP. Otherwise, see
the :ref:`access groups guide <Using access groups>` below.

Backend configuration options
-----------------------------

================== =============================================================== ================================================
Parameter          Description                                                     Default
================== =============================================================== ================================================
version            Always 1
storageDriverName  Always "solidfire-san"
Endpoint           MVIP for the SolidFire cluster with tenant credentials
SVIP               Storage (iSCSI) IP address and port
TenantName         Tenant name to use (created if not found)
InitiatorIFace     Restrict iSCSI traffic to a specific host interface             "default"
UseCHAP            Use CHAP to authenticate iSCSI (otherwise uses access groups)   false
AccessGroups       List of Access Group IDs to use                                 Finds the ID of an access group named "trident"
Types              QoS specifications (see below)
================== =============================================================== ================================================

Example configuration
---------------------

.. code-block:: json

  {
      "version": 1,
      "storageDriverName": "solidfire-san",
      "Endpoint": "https://<user>:<password>@<mvip>/json-rpc/8.0",
      "SVIP": "<svip>:3260",
      "TenantName": "<tenant>",
      "UseCHAP": true,
      "Types": [{"Type": "Bronze", "Qos": {"minIOPS": 1000, "maxIOPS": 2000, "burstIOPS": 4000}},
                {"Type": "Silver", "Qos": {"minIOPS": 4000, "maxIOPS": 6000, "burstIOPS": 8000}},
                {"Type": "Gold", "Qos": {"minIOPS": 6000, "maxIOPS": 8000, "burstIOPS": 10000}}]
  }

In this case we're using CHAP authentication and modeling three volume types
with specific QoS guarantees. Most likely you would then define storage classes
to consume each of these using the ``IOPS`` storage class parameter.

Using access groups
-------------------

.. note::
  Ignore this section if you are using CHAP, which we recommend to simplify
  management and avoid the scaling limit described below.

Trident can use volume access groups to control access to the volumes that it
provisions. If CHAP is disabled it expects to find an access group called
``trident`` unless one or more access group IDs are specified in the
configuration.

While Trident associates new volumes with the configured access group(s), it
does not create or otherwise manage access groups themselves. The access
group(s) must exist before the storage backend is added to Trident, and they
need to contain the iSCSI IQNs from every node in the Kubernetes cluster that
could potentially mount the volumes provisioned by that backend. In most
installations that's every worker node in the cluster.

For Kubernetes clusters with more than 64 nodes, you will need to use multiple
access groups. Each access group may contain up to 64 IQNs, and each volume can
belong to 4 access groups. With the maximum 4 access groups configured, any
node in a cluster up to 256 nodes in size will be able to access any volume.

If you're modifying the configuration from one that is using the default
``trident`` access group to one that uses others as well, include the ID for
the ``trident`` access group in the list.
