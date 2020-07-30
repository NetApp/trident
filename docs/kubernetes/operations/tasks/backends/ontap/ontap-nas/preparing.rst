###########
Preparation
###########

For all ONTAP backends, Trident requires at least one
`aggregate assigned to the SVM`_.

.. _aggregate assigned to the SVM: https://library.netapp.com/ecmdocs/ECMP1368404/html/GUID-5255E7D8-F420-4BD3-AEFB-7EF65488C65C.html

Remember that you can also run more than one driver, and create storage
classes that point to one or the other. For example, you could configure a
*Gold* class that uses the ``ontap-nas`` driver and a *Bronze* class that
uses the ``ontap-nas-economy`` one.

ontap-nas, ontap-nas-economy, ontap-nas-flexgroups
--------------------------------------------------

All of your Kubernetes worker nodes must have the appropriate NFS tools
installed. See the :ref:`worker configuration guide <NFS>` for more details.

Trident uses NFS `export policies`_ to control access to the volumes that it
provisions.

.. _export policies: https://library.netapp.com/ecmdocs/ECMP1196891/html/GUID-9A2B6C3E-C86A-4125-B778-6072A3A19657.html

Trident provides two options when working with export policies:

1. Trident can **dynamically manage the export policy itself**; in this mode of
   operation, the storage admin specifies a list of CIDR blocks that
   represent admissible IP addresses. Trident adds node IPs that fall in
   these ranges to the export policy automatically. Alternatively, when no
   CIDRs are specified, any global-scoped unicast IP found on the nodes will
   be added to the export policy.

2. Storage admins can create an export policy and add rules manually. Trident uses
   the ``default`` export policy unless a different export policy name is specified
   in the configuration.

With (1), Trident automates the management of export policies, creating an export
policy and taking care of additions and deletions of rules to the export policy based
on the worker nodes it runs on. As and when nodes are removed or added to the
Kubernetes cluster, Trident can be set up to permit access to the nodes, thus
providing a more robust way of managing access to the PVs it creates. Trident
will create one export policy per backend. **This feature requires CSI Trident**.

With (2), Trident does not create or otherwise manage export policies themselves.
The export policy must exist before the storage backend is added to Trident,
and it needs to be configured to allow access to every worker node in the
Kubernetes cluster. If the export policy is locked down to specific hosts,
it will need to be updated when new nodes are added to the cluster
and that access should be removed when nodes are removed as well.
