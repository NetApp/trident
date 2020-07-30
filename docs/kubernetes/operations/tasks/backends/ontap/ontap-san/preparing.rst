###########
Preparation
###########

For all ONTAP backends, Trident requires at least one
`aggregate assigned to the SVM`_.

.. _aggregate assigned to the SVM: https://library.netapp.com/ecmdocs/ECMP1368404/html/GUID-5255E7D8-F420-4BD3-AEFB-7EF65488C65C.html

Remember that you can also run more than one driver, and create storage
classes that point to one or the other. For example, you could configure a
``san-dev`` class that uses the ``ontap-san`` driver and a ``san-default`` class that
uses the ``ontap-san-economy`` one.

All of your Kubernetes worker nodes must have the appropriate iSCSI tools
installed. See the :ref:`worker configuration guide <iSCSI>` for more details.

Trident uses `igroups`_ to control access to the volumes (LUNs) that it
provisions. It expects to find an igroup called ``trident`` unless a different
igroup name is specified in the configuration.

.. _igroups: https://library.netapp.com/ecmdocs/ECMP1196995/html/GUID-CF01DCCD-2C24-4519-A23B-7FEF55A0D9A3.html

While Trident associates new LUNs with the configured igroup, it does not
create or otherwise manage igroups themselves. The igroup must exist before the
storage backend is added to Trident.

If Trident is configured to function as a
CSI Provisioner, Trident manages the addition of IQNs from worker nodes when
mounting PVCs. As and when PVCs are attached to pods running on a given node,
Trident adds the node's IQN to the igroup configured in your backend definition.

If Trident does not run as a CSI Provisioner, the igroup must be manually updated
to contain the iSCSI IQNs from every worker node in the Kubernetes cluster. The
igroup needs to be updated when new nodes are added to the cluster, and
they should be removed when nodes are removed as well.

Trident can authenticate iSCSI sessions with bidirectional CHAP beginning with 20.04
for the ``ontap-san`` and ``ontap-san-economy`` drivers. This requires enabling the
``useCHAP`` option in your backend definition. When set to ``true``, Trident
configures the SVM's default initiator security to bidirectional CHAP and set
the username and secrets from the backend file. The section below explains this
in detail.
