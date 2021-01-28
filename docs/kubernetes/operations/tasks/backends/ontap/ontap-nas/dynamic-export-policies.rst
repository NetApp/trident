.. _dynamic-export-policy-ontap:
######################################
Dynamic Export Policies with ONTAP NAS
######################################

The 20.04 release of CSI Trident provides the ability to dynamically manage
export policies for ONTAP backends. This provides the storage administrator
the ability to specify a permissible address space for worker node
IPs, rather than defining explicit rules manually. Since Trident automates the
export policy creation and configuration, it greatly simplifies export policy management for
the storage admin and the Kubernetes admin; modifications to the export policy no
longer require manual intervention on the storage cluster. Moreover, this helps
restrict access to the storage cluster only to worker nodes that have IPs in the
range specified, supporting a finegrained and automated managment.

Prerequisites
-------------

.. warning::

   The auto-management of export policies is only available for CSI Trident.
   It is important to ensure that **the worker nodes are not being NATed**.
   For Trident to discover the node IPs and add rules to the export policy
   dynamically, it **must be able to discover the node IPs**.

There are two configuration options that must be used. Here's an example backend
definition:

.. code::

   {
       "version": 1,
       "storageDriverName": "ontap-nas",
       "backendName": "ontap_nas_auto_export,
       "managementLIF": "192.168.0.135",
       "svm": "svm1",
       "username": "vsadmin",
       "password": "FaKePaSsWoRd",
       "autoExportCIDRs": ["192.168.0.0/24"],
       "autoExportPolicy": true
   }

.. warning::

   When using auto export policies, you must ensure that the root junction
   in your SVM has a pre-created export policy with an export rule that
   permits the node CIDR block (such as the ``default`` export policy). All
   volumes created by Trident are mounted under the root junction. Always
   follow NetApp's recommended best practice of dedicating a SVM for Trident.

How it works
------------

From the example shown above:

1. ``autoExportPolicy`` is set to ``true``. This indicates that Trident will
   create an export policy for the ``svm1`` SVM and handle the addition and
   deletion of rules using the ``autoExportCIDRs`` address blocks. The export
   policy will be named using the format ``trident-<uuid>``. For example, a backend
   with UUID ``403b5326-8482-40db-96d0-d83fb3f4daec`` and ``autoExportPolicy`` set
   to ``true`` will see Trident create an export policy named
   ``trident-403b5326-8482-40db-96d0-d83fb3f4daec`` on the SVM.

2. ``autoExportCIDRs`` contains a list of address blocks. **This field is
   optional and it defaults to** ``["0.0.0.0/0", "::/0"]``. **If not defined,
   Trident adds all globally-scoped unicast addresses found on the worker
   nodes**.

   In this example, the ``192.168.0.0/24`` address space is provided.
   This indicates that Kubernetes node IPs that fall within this address range
   will be added by Trident to the export policy it creates in (1).
   When Trident registers a node it runs on,
   it retrieves the IP addresses of the node and checks them against the address
   blocks provided in ``autoExportCIDRs``. After filtering the IPs, Trident creates
   export policy rules for the client IPs it discovers, with one rule for each node
   it identifies.

   The ``autoExportPolicy`` and ``autoExportCIDRs`` parameters can be updated for
   backends after they are created. You can append new CIDRs for a backend that's
   automatically managed or delete existing CIDRs. Exercise care **when deleting
   CIDRs to ensure that existing connections are not dropped**. You can also choose to disable
   ``autoExportPolicy`` for a backend and fall back to a manually created export
   policy. This will require setting the ``exportPolicy`` parameter in your backend
   config.

After Trident creates/updates a backend, you can check the backend using ``tridentctl``
or the corresponding tridentbackend CRD:

.. code-block:: bash

   $ ./tridentctl get backends ontap_nas_auto_export -n trident -o yaml
   items:
   - backendUUID: 403b5326-8482-40db-96d0-d83fb3f4daec
     config:
       aggregate: ""
       autoExportCIDRs:
       - 192.168.0.0/24
       autoExportPolicy: true
       backendName: ontap_nas_auto_export
       chapInitiatorSecret: ""
       chapTargetInitiatorSecret: ""
       chapTargetUsername: ""
       chapUsername: ""
       dataLIF: 192.168.0.135
       debug: false
       debugTraceFlags: null
       defaults:
         encryption: "false"
         exportPolicy: <automatic>
         fileSystemType: ext4

Updating your Kubernetes cluster configuration
----------------------------------------------

As nodes are added to a Kubernetes cluster and registered with the Trident controller,
export policies of existing backends are updated (provided they fall in the address
range specified in the ``autoExportCIDRs`` for the backend). The CSI Trident daemonset
spins up a pod on all available nodes in the Kuberentes cluster.
Upon registering an eligible node, Trident checks if it contains IP addresses in the
CIDR block that is allowed on a per-backend basis. Trident then updates the export policies
of all possible backends, adding a rule for each node that meets the criteria.

A similar workflow is observed when nodes are deregistered from the Kubernetes cluster.
When a node is removed, Trident checks all backends that are online to remove the access rule
for the node. By removing this node IP from the export policies of managed backends, Trident
prevents rogue mounts, unless this IP is reused by a new node in the cluster.

Updating legacy backends
------------------------

For previously existing backends, updating the backend with ``tridentctl update backend``
will ensure Trident manages the export policies automatically. This will create a new export
policy named after the backend's UUID and volumes that are present on the backend will use
the newly created export policy when they are mounted again.

.. note::

   Deleting a backend with auto managed export policies will delete the dynamically
   created export policy. If the backend is recreated, it is treated as a new backend
   and will result in the creation of a new export policy.

.. note::

   If the IP address of a live node is updated, you must restart the Trident pod
   on the node. Trident will then update the export policy for backends it manages
   to reflect this IP change.
