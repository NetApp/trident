#########################
Simple Kubernetes install
#########################

Those that are interested in Trident and just getting started with Kubernetes
frequently ask us for a simple way to install Kubernetes to try it out.

These instructions provide a bare-bones single node cluster that Trident will
be able to integrate with for demonstration purposes.

.. warning:: The Kubernetes cluster these instructions build should never be
  used in production. Follow production deployment guides provided by your
  distribution for that.

This is a simplification of the `kubeadm install guide`_ provided by
Kubernetes. If you're having trouble, your best bet is to revert to that guide.

Prerequisites
=============

An Ubuntu 16.04 machine with at least 1 GB of RAM.

These instructions are very opinionated by design, and will not work with
anything else. For more generic instructions, you will need to run through the
entire `kubeadm install guide`_.

Install Docker CE 17.03
=======================

.. code-block:: bash

  apt-get update && apt-get install -y curl apt-transport-https

  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
  cat <<EOF >/etc/apt/sources.list.d/docker.list
  deb https://download.docker.com/linux/$(lsb_release -si | tr '[:upper:]' '[:lower:]') $(lsb_release -cs) stable
  EOF
  apt-get update && apt-get install -y docker-ce=$(apt-cache madison docker-ce | grep 17.03 | head -1 | awk '{print $3}')

Install the appropriate version of kubeadm, kubectl and kubelet
===============================================================

.. code-block:: bash

  curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
  cat <<EOF >/etc/apt/sources.list.d/kubernetes.list
  deb http://apt.kubernetes.io/ kubernetes-xenial main
  EOF
  apt-get update
  apt-get install -y kubeadm=1.12\* kubectl=1.12\* kubelet=1.12\* kubernetes-cni=0.6\*

Configure the host
==================

.. code-block:: bash

  swapoff -a
  # Comment out swap line in fstab so that it remains disabled after reboot
  vi /etc/fstab

Create the cluster
==================

.. code-block:: bash

  kubeadm init --kubernetes-version stable-1.12 --token-ttl 0 --pod-network-cidr=192.168.0.0/16

Install the kubectl creds and untaint the cluster
=================================================

.. code-block:: bash

  mkdir -p $HOME/.kube
  sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
  sudo chown $(id -u):$(id -g) $HOME/.kube/config
  kubectl taint nodes --all node-role.kubernetes.io/master-

Add an overlay network
======================

.. code-block:: bash

  kubectl apply -f https://docs.projectcalico.org/v2.6/getting-started/kubernetes/installation/hosted/kubeadm/1.6/calico.yaml

Verify that all of the services started
=======================================

After completing those steps, you should see output similar to this within a
few minutes:

.. code-block:: console

  # kubectl get po -n kube-system
  NAME                                       READY     STATUS    RESTARTS   AGE
  calico-etcd-rvgzs                          1/1       Running   0          9d
  calico-kube-controllers-6ff88bf6d4-db64s   1/1       Running   0          9d
  calico-node-xpg2l                          2/2       Running   0          9d
  etcd-scspa0333127001                       1/1       Running   0          9d
  kube-apiserver-scspa0333127001             1/1       Running   0          9d
  kube-controller-manager-scspa0333127001    1/1       Running   0          9d
  kube-dns-545bc4bfd4-qgkrn                  3/3       Running   0          9d
  kube-proxy-zvjcf                           1/1       Running   0          9d
  kube-scheduler-scspa0333127001             1/1       Running   0          9d

Notice that all of the Kubernetes services are in a *Running* state.
Congratulations! At this point your cluster is operational.

If this is the first time you're using Kubernetes, we highly recommend a
`walkthrough`_ to familiarize yourself with the concepts and tools.

.. _kubeadm install guide: https://kubernetes.io/docs/setup/independent/install-kubeadm/
.. _walkthrough: https://kubernetes.io/docs/user-guide/walkthrough/
