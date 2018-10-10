Troubleshooting
^^^^^^^^^^^^^^^

* If there was a failure during install, run ``tridentctl logs -l all -n trident``
  and look for problems in the logs for the ``trident-main`` and ``etcd`` containers.
  Alternatively, you can use ``kubectl logs`` to retrieve the logs for the
  ``trident-********-****`` pod.
* If the Trident pod fails to come up properly (e.g., when Trident pod is stuck
  in the ``ContainerCreating`` phase with fewer than 2 ready containers),
  running ``kubectl -n trident describe deployment trident`` and
  ``kubectl -n trident describe pod trident-********-****`` can provide
  additional insights. Obtaining kubelet logs
  (e.g., via ``journalctl -xeu kubelet``) can also be helpful if there is a
  problem in mounting the ``trident`` PVC (the ``etcd`` volume).
* If there's not enough information in the Trident logs, you can try enabling
  the debug mode for Trident by passing the ``-d`` flag to the install
  parameter: ``./tridentctl install -d -n trident``.
* The :ref:`uninstall parameter <Uninstalling Trident>` can help with cleaning up
  after a failed run. By default the script does not touch the etcd backing
  store, making it safe to uninstall and install again even in a running
  deployment.
* After a successful install, if a PVC is stuck in the ``Pending`` phase,
  running ``kubectl describe pvc`` can provide additional information on why
  Trident failed to provsion a PV for this PVC.
* If you require further assistance, please create a support bundle via
  ``tridentctl logs -a -n trident`` and send it to :ref:`NetApp Support <Getting Help>`.
