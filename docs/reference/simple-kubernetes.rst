#########################
Simple Kubernetes install
#########################

Those that are interested in Trident and just getting started with Kubernetes
frequently ask us for a simple way to install Kubernetes to try it out.

NetApp provides a ready-to-use lab image that you can request through
`NetApp Test Drive <https://www.netapp.com/us/try-and-buy/test-drive/index.aspx>`_.
The Trident Test Drive provides you with a sandbox environment that comes with a
3-node Kubernetes cluster and Trident installed and configured. It is a great way
to familiarize yourself with Trident and explore its features.
 
Another option is to follow the `kubeadm install guide`_ provided by Kubernetes.
The Kubernetes cluster these instructions build should never be used in production.
Follow production deployment guides provided by your distribution for that.

If this is the first time you're using Kubernetes, we highly recommend a
`walkthrough`_ to familiarize yourself with the concepts and tools.

.. _kubeadm install guide: https://kubernetes.io/docs/setup/independent/install-kubeadm/
.. _NetApp Kubernetes Service: https://cloud.netapp.com/kubernetes-service?utm_source=GitHub&utm_campaign=Trident
.. _NetApp HCI: https://www.netapp.com/us/products/converged-systems/hyper-converged-infrastructure.aspx
.. _VMware: https://docs.netapp.com/us-en/kubernetes-service/create-vmware-cluster.html
.. _walkthrough: https://kubernetes.io/docs/home/
