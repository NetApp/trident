Troubleshooting
^^^^^^^^^^^^^^^

* If there was a failure during install, run ``tridentctl logs -l all -n trident``
  and look for problems in the logs for the ``trident-main`` and CSI containers (when
  using the CSI provisioner).
  Alternatively, you can use ``kubectl logs`` to retrieve the logs for the
  ``trident-********-****`` pod.
* If the Trident pod fails to come up properly (e.g., when Trident pod is stuck
  in the ``ContainerCreating`` phase with fewer than 2 ready containers),
  running ``kubectl -n trident describe deployment trident`` and
  ``kubectl -n trident describe pod trident-********-****`` can provide
  additional insights. Obtaining kubelet logs
  (e.g., via ``journalctl -xeu kubelet``) can also be helpful.
* If there's not enough information in the Trident logs, you can try enabling
  the debug mode for Trident by passing the ``-d`` flag to the install
  parameter: ``./tridentctl install -d -n trident``.
* If there are problems with mounting a PV to a container, ensure that ``rpcbind`` is
  installed and running. Use the required package manager for the host OS and check if
  ``rpcbind`` is running. You can check the status of the ``rpcbind`` service by running
  a ``systemctl status rpcbind`` or its equivalent.
* If you encounter permission issues when installing Trident with Docker as the container
  runtime, attempt the installation of Trident with the ``--in-cluster=false`` flag. This
  will not use an installer pod and avoid permission troubles seen due to the ``trident-installer``
  user.
* The :ref:`uninstall parameter <Uninstalling Trident>` can help with cleaning up
  after a failed run. By default the script does not remove the CRDs that have
  been created by Trident, making it safe to uninstall and install again even in a running
  deployment.
* If you are looking to downgrade to an earlier version of Trident, first execute the
  ``tridenctl uninstall`` command to remove Trident. Download the desired `Trident version`_
  and install using the ``tridentctl install`` command. Only consider a downgrade if there
  are no new PVs created and if no changes have been made to already existing PVs/backends/
  storage classes. Since Trident now uses CRDs for maintaining state, all storage entities
  created (backends, storage classes, PVs and Volume Snapshots) have
  :ref:`associated CRD objects <Kubernetes CustomResourceDefinition Objects>`
  instead of data written into the PV that was
  used by the earlier installed version of Trident. **Newly created PVs will
  not be usable when moving back to an earlier version.**
  **Changes made to objects
  such as backends, PVs, storage classes and Volume Snapshots 
  (created/updated/deleted) will not be visible to Trident when
  downgraded**. The PV that was used by the earlier version of Trident installed will still be
  visible to Trident. Going back to an earlier version will not disrupt access for
  PVs that were already created using the older release, unless they have been upgraded.
* To completely remove Trident, execute the ``tridentctl obliviate crd`` command. This will
  remove all CRD objects and undefine the CRDs. Trident will no longer manage any PVs it had
  already provisioned. Remember that Trident will need to be
  reconfigured from scratch after this.
* After a successful install, if a PVC is stuck in the ``Pending`` phase,
  running ``kubectl describe pvc`` can provide additional information on why
  Trident failed to provsion a PV for this PVC.
* If you require further assistance, please create a support bundle via
  ``tridentctl logs -a -n trident`` and send it to :ref:`NetApp Support <Getting Help>`.

.. _Trident version: https://github.com/NetApp/trident/releases
