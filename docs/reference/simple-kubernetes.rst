#########################
Simple Kubernetes install
#########################

Those that are interested in Trident and just getting started with Kubernetes
frequently ask us for a simple way to install Kubernetes to try it out.

We can't help but recommend the `NetApp Kubernetes Service`_, which makes it really
easy to deploy and manage a production Kubernetes cluster in Azure, GCP, AWS, or even
on-premises with `NetApp HCI`_ or `VMware`_.

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
