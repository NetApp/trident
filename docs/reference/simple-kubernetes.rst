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
  apt-get install -y kubeadm=1.15.1 kubectl=1.15.1 kubelet=1.15.1 kubernetes-cni=0.7.5

Configure the host
==================

.. code-block:: bash

  swapoff -a
  # Comment out swap line in fstab so that it remains disabled after reboot
  vi /etc/fstab

Create the cluster
==================

.. code-block:: bash

  kubeadm init --kubernetes-version stable-1.14 --token-ttl 0 --pod-network-cidr=192.168.0.0/16

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

   kubectl apply -f https://docs.projectcalico.org/v3.7/manifests/calico.yaml

Verify that all of the services started
=======================================

After completing those steps, you should see output similar to this within a
few minutes:

.. code-block:: console

  # kubectl get po -n kube-system

    NAMESPACE     NAME                                         READY   STATUS    RESTARTS   AGE
    kube-system   calico-kube-controllers-658558ddf8-4hwf4     1/1     Running   0          20m
    kube-system   calico-node-75zdm                            1/1     Running   0          20m
    kube-system   coredns-5c98db65d4-bgtlq                     1/1     Running   0          20m
    kube-system   coredns-5c98db65d4-pr8rf                     1/1     Running   0          20m
    kube-system   etcd-trident-1907                            1/1     Running   0          20m
    kube-system   kube-apiserver-trident-1907                  1/1     Running   0          20m
    kube-system   kube-controller-manager-trident-1907         1/1     Running   0          20m
    kube-system   kube-proxy-q4c75                             1/1     Running   0          20m
    kube-system   kube-scheduler-trident-1907                  1/1     Running   0          20m

Notice that all of the Kubernetes services are in a *Running* state.
Congratulations! At this point your cluster is operational.

If this is the first time you're using Kubernetes, we highly recommend a
`walkthrough`_ to familiarize yourself with the concepts and tools.

.. _kubeadm install guide: https://kubernetes.io/docs/setup/independent/install-kubeadm/
.. _walkthrough: https://kubernetes.io/docs/user-guide/walkthrough/
