Troubleshooting
^^^^^^^^^^^^^^^

* Run ``tridentctl logs -l all -n trident``. Trident incorporates a multi-phase
  installation process that involves multiple pods, and the Trident pod itself
  includes multiple containers, all of which have their own logs. When
  something goes wrong, this is the best way to quickly examine all of the
  relevant logs.
* If the Trident pod fails to come up properly (e.g., when Trident pod is stuck
  in the ``ContainerCreating`` phase with fewer than 2 ready containers),
  running ``kubectl -n trident describe deployment trident`` and
  ``kubectl -n trident describe pod trident-********-****`` can provide
  additional insights. Obtaining kubelet logs
  (e.g., via ``journalctl -xeu kubelet``) can also be helpful here.
* If there's not enough information in the Trident and Trident launcher logs,
  you can try enabling debug mode for Trident and Trident launcher by passing
  the ``-d`` flag to the install script: ``./install_trident.sh -d -n trident``.
* The :ref:`uninstall script <Uninstalling Trident>` can help with cleaning up
  after a failed run. By default the script does not touch the etcd backing
  store, making it safe to uninstall and install again even in a running
  deployment.
* If service accounts are not available, the logs will report an error that
  ``/var/run/secrets/kubernetes.io/serviceaccount/token`` does not exist.  In
  this case, you will either need to enable service accounts or connect to the
  API server by specifying the insecure address and port on the command line.
