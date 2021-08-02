#####################
SANtricity (E-Series)
#####################

Based on systems telemetry and in accordance with the 
`maintenance notice`_ provided earlier, the ``eseries-iscsi`` storage driver
has been deprecated. Users are advised to leverage the
`upstream iSCSI driver`_/NetApp’s `BeeGFS`_ storage driver
to continue provisioning persistent storage on E/EF-Series clusters.

.. _maintenance notice: https://netapp.io/2021/06/16/eseries-iscsi-maintenance/
.. _upstream iSCSI driver: https://github.com/kubernetes-csi/csi-driver-iscsi
.. _BeeGFS: https://github.com/NetApp/beegfs-csi-driver
