node {

  // Define a var to store the resolved or provided branch
  // This can be in one of the following forms
  // master
  // stable/v19.10
  // The */my-branch-name is converted to my-branch-name
  def branch = ''

  // Define a var to store the git commit hash
  def commit = ''

  // Define a var for HOLD_JIG so we can overwrite it later for debugging
  def hold_jig = 'never'
  if (env.HOLD_JIG) {
    echo "Overriding HOLD_JIG to $env.HOLD_JIG"
    hold_jig = env.HOLD_JIG
  }

  // Define a var to hold the stage map
  def map = []

  // Define the ssh options that will be used with all ssh and scp commands
  def ssh_options='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ServerAliveInterval=60 -i tools/' + env.SSH_PRIVATE_KEY_PATH + ' -q'

  // Define the stage to phase maxtrix
  def plan = [
    [
      [
        'name': 'Build-Documentation',
        'context': 'openlab',
        'coverage': 'documentation,pre-merge,post-commit',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_os_update.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_build_documentation',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'Build-Trident',
        'context': 'openlab',
        'coverage': 'docker-ee,kubeadm,kubeadm-legacy,docker-volume-binary,docker-volume-plugin,openshift,pre-merge,post-commit,upgrade',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_build_trident',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'Unit-Test',
        'context': 'openlab',
        'coverage': 'pre-merge,post-commit',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_unit_test',
        'vm_provider': 'SCS'

      ]
    ],
    [
      [
        'name': 'CSI-Sanity-CentOS',
        'backend': 'ontap-san',
        'context': 'openlab',
        'coverage': 'kubeadm,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-san': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_SAN_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_csi_sanity',
        'vm_provider': 'SCS',
      ],
      [
        'name': 'KubeAdm-Ubuntu-CVS',
        'ami': 'ami-0181a7c55dca823bf',
        'aws_access_key_id': env.AWS_ACCESS_KEY_ID,
        'aws_secret_access_key': env.AWS_SECRET_ACCESS_KEY,
        'aws_default_region': 'eu-west-2',
        'aws-cvs': [
          'region': 'eu-west-2',
          'api_url': env.AWS_CVS_API_URL,
          'api_key': env.AWS_API_KEY,
          'api_secret_key': env.AWS_API_SECRET_KEY
        ],
        'aws-cvs-virtual-pools': [
          'region': 'eu-west-2',
          'api_url': env.AWS_CVS_API_URL,
          'api_key': env.AWS_API_KEY,
          'api_secret_key': env.AWS_API_SECRET_KEY
        ],
        'backend': 'aws-cvs,aws-cvs-virtual-pools',
        'context': 'cloud',
        'coverage': 'cvs',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'instance_type': 't2.large',
        'k8s_version': env.K8S_VERSION,
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'key_name': 'bamboo_rsa',
        'kubeadm_version': env.KUBEADM_VERSION,
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'security_group': 'sg-022c7ef620d8ad1a8',
        'stage': '_whelk_test',
        'subnet': 'subnet-045b4cde70b848dc2',
        'test': 'kubeadm',
        'trident_image_distribution': 'load',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'AWS'
      ],
      [
        'name': 'KubeAdm-CentOS-Solidfire-SAN',
        'backend': 'solidfire-san',
        'context': 'openlab',
        'coverage': 'kubeadm,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': env.K8S_VERSION,
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': env.KUBEADM_VERSION,
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7.yaml',
        'solidfire-san': [
            'username': env.SOLIDFIRE_KUBEADM_USERNAME,
            'password': env.SOLIDFIRE_KUBEADM_PASSWORD,
            'mvip': env.SOLIDFIRE_MVIP,
            'svip': env.SOLIDFIRE_SVIP
        ],
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'KubeAdm-CentOS-ONTAP-SAN',
        'backend': 'ontap-san',
        'context': 'openlab',
        'coverage': 'kubeadm,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': env.K8S_VERSION,
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': env.KUBEADM_VERSION,
        'ontap-san': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_SAN_MLIF,
        ],
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'KubeAdm-CentOS-ONTAP-SAN-Economy',
        'backend': 'ontap-san-economy',
        'context': 'openlab',
        'coverage': 'kubeadm,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': env.K8S_VERSION,
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': env.KUBEADM_VERSION,
        'ontap-san-economy': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_SAN_MLIF,
        ],
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'KubeAdm-CentOS-ONTAP-NAS',
        'backend': 'ontap-nas',
        'context': 'openlab',
        'coverage': 'kubeadm,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': env.K8S_VERSION,
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': env.KUBEADM_VERSION,
        'ontap-nas': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'KubeAdm-CentOS-ONTAP-NAS-Economy',
        'backend': 'ontap-nas-economy',
        'context': 'openlab',
        'coverage': 'kubeadm,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': env.K8S_VERSION,
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': env.KUBEADM_VERSION,
        'ontap-nas-economy': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'KubeAdm-CentOS-Flexgroup',
        'backend': 'ontap-nas-flexgroup',
        'context': 'openlab',
        'coverage': 'kubeadm,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': env.K8S_VERSION,
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': env.KUBEADM_VERSION,
        'ontap-nas-flexgroup': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'KubeAdm-CentOS-ESeries-ISCSI',
        'backend': 'eseries-iscsi',
        'context': 'openlab',
        'coverage': 'kubeadm',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'eseries-iscsi': [
          'array_password': env.ESERIES_ARRAY_PASSWORD,
          'controllera': env.ESERIES_CONTROLLERA,
          'controllerb': env.ESERIES_CONTROLLERB,
          'host_data_ip': env.ESERIES_HOST_DATA_IP,
          'username': env.ESERIES_ADMIN_USERNAME,
          'password': env.ESERIES_ADMIN_PASSWORD,
          'web_proxy_hostname': env.ESERIES_WEB_PROXY_HOSTNAME,
        ],
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': env.K8S_VERSION,
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': env.KUBEADM_VERSION,
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'KubeAdm-Ubuntu-ONTAP-NAS',
        'backend': 'ontap-nas',
        'context': 'openlab',
        'coverage': 'kubeadm,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': env.K8S_VERSION,
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': env.KUBEADM_VERSION,
        'ontap-nas': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'

      ],
      [
        'name': 'Docker-EE-CentOS-ONTAP-NAS',
        'backend': 'ontap-nas',
        'context': 'openlab',
        'coverage': 'docker-ee,pre-merge',
        'docker_ee_image': 'docker/ucp:3.1.11',
        'docker_ee_user': env.DOCKER_EE_USERNAME,
        'docker_ee_password': env.DOCKER_EE_PASSWORD,
        'docker_ee_repo': env.DOCKER_EE_REPO,
        'docker_ee_url': env.DOCKER_EE_URL,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': 'v1.11.5',
        'ontap-nas': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ee.playbook',
        'request': 'ci_centos_7_4cpu_8gbr.yaml',
        'stage': '_whelk_test',
        'test': 'docker_ee',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'OpenShift-CentOS-ONTAP-NAS-Economy',
        'backend': 'ontap-nas-economy',
        'context': 'openlab',
        'coverage': 'openshift,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas-economy': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'openshift_url': env.OPENSHIFT_URL,
        'post_deploy_playbook': 'ci_openshift.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'openshift',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-Ubuntu-ESeries-ISCSI',
        'backend': 'eseries-iscsi',
        'context': 'openlab',
        'coverage': 'docker-volume-binary',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'eseries-iscsi': [
          'array_password': env.ESERIES_ARRAY_PASSWORD,
          'controllera': env.ESERIES_CONTROLLERA,
          'controllerb': env.ESERIES_CONTROLLERB,
          'host_data_ip': env.ESERIES_HOST_DATA_IP,
          'username': env.ESERIES_ADMIN_USERNAME,
          'password': env.ESERIES_ADMIN_PASSWORD,
          'web_proxy_hostname': env.ESERIES_WEB_PROXY_HOSTNAME,
        ],
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-CentOS-ESeries-ISCSI',
        'backend': 'eseries-iscsi',
        'context': 'openlab',
        'coverage': 'docker-volume-binary',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'eseries-iscsi': [
          'array_password': env.ESERIES_ARRAY_PASSWORD,
          'controllera': env.ESERIES_CONTROLLERA,
          'controllerb': env.ESERIES_CONTROLLERB,
          'host_data_ip': env.ESERIES_HOST_DATA_IP,
          'username': env.ESERIES_ADMIN_USERNAME,
          'password': env.ESERIES_ADMIN_PASSWORD,
          'web_proxy_hostname': env.ESERIES_WEB_PROXY_HOSTNAME,
        ],
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-Ubuntu-ONTAP-NAS',
        'backend': 'ontap-nas',
        'context': 'openlab',
        'coverage': 'docker-volume-binary,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-CentOS-ONTAP-NAS',
        'backend': 'ontap-nas',
        'context': 'openlab',
        'coverage': 'docker-volume-binary,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-Ubuntu-ONTAP-NAS-Economy',
        'backend': 'ontap-nas-economy',
        'context': 'openlab',
        'coverage': 'docker-volume-binary,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas-economy': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-Centos-ONTAP-NAS-Economy',
        'backend': 'ontap-nas-economy',
        'context': 'openlab',
        'coverage': 'docker-volume-binary,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas-economy': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-CentOS-ONTAP-SAN',
        'backend': 'ontap-san',
        'context': 'openlab',
        'coverage': 'docker-volume-binary,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-san': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_SAN_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-Ubuntu-ONTAP-SAN-Economy',
        'backend': 'ontap-san-economy',
        'context': 'openlab',
        'coverage': 'docker-volume-binary,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-san-economy': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_SAN_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-Ubuntu-Solidfire-SAN',
        'backend': 'solidfire-san',
        'context': 'openlab',
        'coverage': 'docker-volume-binary,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'solidfire-san': [
          'username': env.SOLIDFIRE_NDVP_USERNAME,
          'password': env.SOLIDFIRE_NDVP_PASSWORD,
          'mvip': env.SOLIDFIRE_MVIP,
          'svip': env.SOLIDFIRE_SVIP
        ],
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVB-CentOS-Solidfire-SAN',
        'backend': 'solidfire-san',
        'context': 'openlab',
        'coverage': 'docker-volume-binary,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'solidfire-san': [
          'username': env.SOLIDFIRE_NDVP_USERNAME,
          'password': env.SOLIDFIRE_NDVP_PASSWORD,
          'mvip': env.SOLIDFIRE_MVIP,
          'svip': env.SOLIDFIRE_SVIP
        ],
        'stage': '_whelk_test',
        'test': 'ndvp_binary',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-CentOS-ESeries-ISCSI',
        'backend': 'eseries-iscsi',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'eseries-iscsi': [
          'array_password': env.ESERIES_ARRAY_PASSWORD,
          'controllera': env.ESERIES_CONTROLLERA,
          'controllerb': env.ESERIES_CONTROLLERB,
          'host_data_ip': env.ESERIES_HOST_DATA_IP,
          'username': env.ESERIES_ADMIN_USERNAME,
          'password': env.ESERIES_ADMIN_PASSWORD,
          'web_proxy_hostname': env.ESERIES_WEB_PROXY_HOSTNAME,
        ],
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-Ubuntu-ESeries-ISCSI',
        'backend': 'eseries-iscsi',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,,
        'eseries-iscsi': [
          'array_password': env.ESERIES_ARRAY_PASSWORD,
          'controllera': env.ESERIES_CONTROLLERA,
          'controllerb': env.ESERIES_CONTROLLERB,
          'host_data_ip': env.ESERIES_HOST_DATA_IP,
          'username': env.ESERIES_ADMIN_USERNAME,
          'password': env.ESERIES_ADMIN_PASSWORD,
          'web_proxy_hostname': env.ESERIES_WEB_PROXY_HOSTNAME,
        ],
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-Ubuntu-Flexgroup',
        'backend': 'ontap-nas-flexgroup',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': false,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas-flexgroup': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-CentOS-ONTAP-NAS',
        'backend': 'ontap-nas',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-Ubuntu-ONTAP-NAS',
        'backend': 'ontap-nas',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-CentOS-ONTAP-NAS-Economy',
        'backend': 'ontap-nas-economy',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas-economy': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-Ubuntu-ONTAP-NAS-Economy',
        'backend': 'ontap-nas-economy',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-nas-economy': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-Ubuntu-ONTAP-SAN',
        'backend': 'ontap-san',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-san': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_SAN_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-CentOS-ONTAP-SAN',
        'backend': 'ontap-san',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-san': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_SAN_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-CentOS-ONTAP-SAN-Economy',
        'backend': 'ontap-san-economy',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'ontap-san-economy': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_SAN_MLIF,
        ],
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-CentOS-Solidfire-SAN',
        'backend': 'solidfire-san',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_centos_7.yaml',
        'solidfire-san': [
          'username': env.SOLIDFIRE_NDVP_USERNAME,
          'password': env.SOLIDFIRE_NDVP_PASSWORD,
          'mvip': env.SOLIDFIRE_MVIP,
          'svip': env.SOLIDFIRE_SVIP
        ],
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'NDVP-Ubuntu-Solidfire-SAN',
        'backend': 'solidfire-san',
        'context': 'openlab',
        'coverage': 'docker-volume-plugin,pre-merge',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'post_deploy_playbook': 'ci_docker_ce.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'solidfire-san': [
          'username': env.SOLIDFIRE_NDVP_USERNAME,
          'password': env.SOLIDFIRE_NDVP_PASSWORD,
          'mvip': env.SOLIDFIRE_MVIP,
          'svip': env.SOLIDFIRE_SVIP
        ],
        'stage': '_whelk_test',
        'test': 'ndvp_plugin',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'Trident-Upgrade-CentOS-ONTAP-NAS',
        'backend': 'ontap-nas',
        'context': 'openlab',
        'coverage': 'pre-merge,upgrade',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': 'v1.15.4',
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': '1.15.4*',
        'ontap-nas': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7_4cpu_8gbr.yaml',
        'stage': '_whelk_test',
        'test': 'upgrade',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'Trident-Upgrade-Ubuntu-ONTAP-NAS',
        'backend': 'ontap-nas',
        'context': 'openlab',
        'coverage': 'pre-merge,upgrade',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': 'v1.15.4',
        'k8s_api_version': env.K8S_API_VERSION,
        'k8s_cni_version': env.K8S_CNI_VERSION,
        'kubeadm_version': '1.15.4*',
        'ontap-nas': [
          'username': env.ONTAP_ADMIN_USERNAME,
          'password': env.ONTAP_ADMIN_PASSWORD,
          'management_lif': env.ONTAP_NAS_MLIF,
        ],
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_ubuntu_bionic.yaml',
        'stage': '_whelk_test',
        'test': 'upgrade',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'KubeAdm-1.14-CentOS-Solidfire-SAN',
        'backend': 'solidfire-san',
        'context': 'openlab',
        'coverage': 'kubeadm-legacy',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': 'v1.14.6',
        'k8s_api_version': 'kubeadm.k8s.io/v1beta1',
        'k8s_cni_version': '0.7.5*',
        'kubeadm_version': '1.14.6*',
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7.yaml',
        'solidfire-san': [
            'username': env.SOLIDFIRE_KUBEADM_USERNAME,
            'password': env.SOLIDFIRE_KUBEADM_PASSWORD,
            'mvip': env.SOLIDFIRE_MVIP,
            'svip': env.SOLIDFIRE_SVIP
        ],
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ],
      [
        'name': 'KubeAdm-1.15-CentOS-Solidfire-SAN',
        'backend': 'solidfire-san',
        'context': 'openlab',
        'coverage': 'kubeadm-legacy',
        'docker_ce_version': env.DOCKER_CE_VERSION,
        'enabled': true,
        'go_download_url': env.GO_DOWNLOAD_URL,
        'hold_jig': hold_jig,
        'k8s_version': 'v1.15.4',
        'k8s_api_version': 'kubeadm.k8s.io/v1beta1',
        'k8s_cni_version': '0.7.5*',
        'kubeadm_version': '1.15.4*',
        'post_deploy_playbook': 'ci_kubeadm.playbook',
        'request': 'ci_centos_7.yaml',
        'solidfire-san': [
            'username': env.SOLIDFIRE_KUBEADM_USERNAME,
            'password': env.SOLIDFIRE_KUBEADM_PASSWORD,
            'mvip': env.SOLIDFIRE_MVIP,
            'svip': env.SOLIDFIRE_SVIP
        ],
        'stage': '_whelk_test',
        'test': 'kubeadm',
        'trident_image_distribution': 'pull',
        'trident_install_playbook': 'ci_trident.playbook',
        'vm_provider': 'SCS'
      ]
    ]
  ]

  if (env.BLACK_DUCK_SCAN) {
    if (env.BLACK_DUCK_PROJECT_VERSION == null) {
      error "BLACK_DUCK_PROJECT_VERSION must be the " +
        "trident version in the form of X.Y.Z"
    }
    plan = [
      [
        [
          'name': 'Black-Duck-Scan',
          'context': 'openlab',
          'coverage': 'black-duck-scan',
          'docker_ce_version': env.DOCKER_CE_VERSION,
          'enabled': true,
          'go_download_url': env.GO_DOWNLOAD_URL,
          'hold_jig': hold_jig,
          'post_deploy_playbook': 'ci_docker_ce.playbook',
          'request': 'ci_centos_7_4cpu_16gbr.yaml',
          'stage': '_build_trident',
          'vm_provider': 'SCS'
        ]
      ]
    ]
  }

  if (env.DEPLOY_TRIDENT) {
    plan = [
      [
        [
          'name': 'Deploy-Trident',
          'context': 'openlab',
          'coverage': 'deploy-trident',
          'docker_ce_version': env.DOCKER_CE_VERSION,
          'enabled': true,
          'go_download_url': env.GO_DOWNLOAD_URL,
          'hold_jig': hold_jig,
          'post_deploy_playbook': 'ci_docker_ce.playbook',
          'request': 'ci_centos_7.yaml',
          'stage': '_build_trident',
          'vm_provider': 'SCS'
        ]
      ]
    ]
  }

  // If the branch is a PR-# branch query Github for the PR info
  if (env.BRANCH_NAME && env.BRANCH_NAME.contains('PR-')) {
    def cmd = "curl -H \"Authorization: token $env.GITHUB_TOKEN\" " +
      "https://api.github.com/repos/$env.TRIDENT_PRIVATE_GITHUB_ORG/" +
      "$env.TRIDENT_PRIVATE_GITHUB_REPO/pulls/$env.CHANGE_ID"
    def response = sh(
      label: "Query Github for $env.BRANCH_NAME",
      returnStdout: true,
      script: cmd
    ).trim()

    def pr = readJSON text: response

    if (pr.containsKey('head') == false) {
      error "Error getting the PR info from Github for $env.BRANCH_NAME"
    }

    branch = pr['head']['ref']
  }

  def slack_result = 'PENDING'
  def slack_message = (
    _decode(env.JOB_NAME) + " #${env.BUILD_NUMBER}:\n" +
    "${env.BUILD_URL}\n"
  )
  if (env.BRANCH_NAME && env.BRANCH_NAME.contains('PR-')) {
    slack_message = (
      _decode(env.JOB_NAME) + " $branch #${env.BUILD_NUMBER}:\n" +
      "${env.BUILD_URL}\n"
    )
  }

  try {

    // Notify Slack we are starting
    _notify_slack(slack_result, slack_message)

    // Checkout the source from Github
    stage('Checkout-Source') {
      _clone()
    }

    // Read the plan variable and create the appropriate stages
    stage('Create-Stages') {
      map = _create_stages(ssh_options, plan, 10)
    }

    // Execute the stages defined in the plan
    for (phase in map) {
      parallel(phase)
    }

    // If we get this far propagate changes to the public trident repo
    stage('Propagate-Changes') {
      if (env.DEPLOY_TRIDENT) {
        echo "Skipping change propagation because DEPLOY_TRIDENT=true"
      } else if (env.BLACK_DUCK_SCAN) {
        echo "Skipping change propagation because BLACK_DUCK_SCAN=true"
      } else {
         _propagate_changes()
      }
    }

    slack_result = 'SUCCESS'

  } catch(Exception e) {

    // Fail the run
    slack_result = 'FAILURE'
    slack_message += e.getMessage()
    error e.getMessage()

  } finally {

    // No matter what the result of the above stages this stage get executed
    stage('Post') {

      if (fileExists("$env.WORKSPACE/status")) {
        sh (
          label: "Create status.tgz",
          script: "tar -cvzf status.tgz status"
        )
        archiveArtifacts artifacts: "status.tgz"
      } else {
        echo "No status files detected"
      }

      // Import all junit compatible files
      if (env.DEPLOY_TRIDENT) {
        echo "Skipping test junit file processing because DEPLOY_TRIDENT=true"
      } else if (env.BLACK_DUCK_SCAN) {
        echo "Skipping test junit file processing because BLACK_DUCK_SCAN=true"
      } else {
        if (fileExists("$env.WORKSPACE/junit")) {
          sh (
            label: "Create junit.tgz",
            script: "tar -cvzf junit.tgz junit"
          )
          archiveArtifacts artifacts: "junit.tgz"

          junit allowEmptyResults: true, testResults: '**/junit/*.xml'
        } else {
          echo "No junit directory detected"
        }
      }

      // Combine the cleanup tarballs for each stage
      if (fileExists("$env.WORKSPACE/cleanup")) {
        sh (
          label: "Concatinate all cleanup script to a master clearnup script",
          script: "cat cleanup/*cleanup.sh > cleanup.sh"
        )
        sh (label: "Sleep", script: "sleep 1")
        sh (label: "Add cleanup.sh to cleanup directory", script: "cp cleanup.sh cleanup/")
        sh (label: "Sleep", script: "sleep 1")
        sh (label: "Show cleanup/cleanup.sh contents", script: "cat cleanup/cleanup.sh")
        sh (label: "Add aws.py to cleanup directory", script: "cp tools/utils/aws.py cleanup/")
        sh (label: "Add scs.py cleanupi directory", script: "cp tools/utils/scs.py cleanup/")
        sh (label: "Create cleanup.tgz", script: "tar -cvzf cleanup.tgz cleanup")
        archiveArtifacts artifacts: "cleanup.tgz"
      } else {
        echo "No cleanup scripts detected"
      }

      // Notify Slack we are starting
      _notify_slack(slack_result, slack_message)
    }
  }
}

// Create List of stages to execute
def _create_stages(String ssh_options, List plan, Integer parallelism) {
  // The coverage var MUST consist of ONE of the following:
  //
  // docker-ee
  // documentation
  // kubeadm
  // docker-volume-binary
  // docker-volume-plugin
  // openshift
  // pre-merge
  // post-commit
  // upgrade,
  //
  // The default should be post-commit.  Which only runs the
  // Build-Documentation, Build-Trident and Unit-Tests stages
  //
  // If the boolean var BLACK_DUCK_SCAN or DEPLOY_TRIDENT are true then
  // coverage will automatically be set
  //
  // If a branch is name PR-* it should have the coverage set to pre-merge
  //
  // If a feature branch or PR has only documentation changes then the
  // coverage should be documentation
  def coverage = 'post-commit'

  // If jenkins is telling us what the branch name/ref is using BRANCH_NAME
  // then override coverage if:
  //   There are only documentation changes, set coverage to documentation
  //   The branch name contains PR-, set coverage to pre-merge
  if (env.BRANCH_NAME) {
    if (env.BRANCH_NAME != 'master' && env.BRANCH_NAME.startsWith('stable') != true) {

      def diff_target_branch_name = 'master'
      dir('src2/github.com/netapp/trident') {

        // If CHANGE_TARGET is not master then checkout the branch
        if (env.CHANGE_TARGET && env.CHANGE_TARGET != 'master') {
          diff_target_branch_name = env.CHANGE_TARGET
          sh(
            label: "Checkout CHANGE_TARGET($env.CHANGE_TARGET)",
            script: "git checkout $env.CHANGE_TARGET"
          )
        }

        sh (label: "Sleep", script: "sleep 1")

      }

      def src_git_log = []
      dir('src/github.com/netapp/trident') {
        src_git_log = sh(
          label: "Get the git log for src/github.com/netapp/trident",
          returnStdout: true,
          script: "git --no-pager log --max-count 100 --pretty=oneline --no-color --decorate=short"
        ).split('\\n')
      }

      def src2_git_log = []
      dir('src2/github.com/netapp/trident') {
        src2_git_log = sh(
          label: "Get the git log for src2/github.com/netapp/trident",
          returnStdout: true,
          script: "git --no-pager log --max-count 25 --pretty=oneline --no-color --decorate=short"
        ).split('\\n')
      }

      def diff_target_commit = ''
      for(src2_line in src2_git_log) {
        def src2_fields = src2_line.split(' ')
        def src2_commit = src2_fields[0]

        for(src_line in src_git_log) {
          def src_fields = src_line.split(' ')
          def src_commit = src_fields[0]
          if (src_commit == src2_commit) {
              diff_target_commit = src2_commit
              break
          }
        }
        if (diff_target_commit) { break }
      }

      if (diff_target_commit) {
        try {

          dir('src/github.com/netapp/trident') {

            changes = sh(
              label: "Check if $branch contains only documentation changes",
              returnStdout: true,
              script: "git diff --name-only $diff_target_commit $commit | " +
                "grep -v '^docs/' | grep -v '^.*\\.md'"
            )
          }

          echo "Non doumentation changes between $diff_target_branch_name and $branch:\n$changes"

          if (changes) {
            if (env.BRANCH_NAME.contains('PR-')) {
              echo "BRANCH_NAME($branch) contains more than " +
                "documentation changes, changing coverage to pre-merge"
              coverage = 'pre-merge'
            } else {
              echo "$branch contains more than documentation " +
                "changes, coverage remains $coverage"
            }
          } else {
            echo "$branch contains only documentation " +
              "changes, changing coverage to documentation"
            coverage = "documentation"
          }
        } catch(Exception e) {
          echo "Error diffing $diff_target_commit and $commit, coverage remains $coverage: " + e.getMessage()
        }

        // If we checked out a branch make sure we checkout master
        dir('src2/github.com/netapp/trident') {
          if (env.CHANGE_TARGET && env.CHANGE_TARGET != 'master') {
            sh(
              label: "Checkout master",
              script: "git checkout master"
            )
          }
        }
      } else {
        echo "Unable to check for documentaion only changes " +
          "because a common commit between $diff_target_branch_name " +
          "and $branch could not be found, coverage remains $coverage"
      }
    } else {
      coverage = 'pre-merge'
      echo (
        "BRANCH_NAME($branch) is master or stable/*, changing coverage to $coverage"
      )
    }
  }

  // If COVERAGE is defined by a build form, override coverage
  if (env.COVERAGE) {
    coverage = env.COVERAGE
    echo "COVERAGE is defined changing coverage to $coverage"
  }

  // If STAGE_NAME_REGEXP is defined override stage_name_regexp
  def stage_name_regexp = '\\.*'
  if (env.STAGE_NAME_REGEXP) {
    stage_name_regexp = env.STAGE_NAME_REGEXP
  }

  echo "Creating stages based on $coverage coverage and stage name matching $stage_name_regexp"

  // Define a var to track stage names
  def stage_list = ''

  // Define the stage map
  def map = []

  // Define some counter vars
  def group_index = 0
  def parallel_stages = 0

  // Iterate over each phase in the plan
  for (phase in plan) {

    // Create a copy of the current loop iterator
    def cop = phase

    // Define tracking vars
    def allocated_stages_this_phase = 0
    def stage_index = 0
    def active_stages_this_phase = 0

    // Get the active stages for the current phase
    for (stage in cop) {
      // Skip the stage if it is not enabled
      if (stage['enabled'] == false) {
        continue
      }

      // If BLACK_DUCK_SCAN and DEPLOY_TRIDENT are not true apply the coverage and
      // stage name filters
      if (env.BLACK_DUCK_SCAN == null && env.DEPLOY_TRIDENT == null) {

        // Skip the stage if the coverage does not match
        if (_coverage_match(stage, coverage) != true) {
          continue
        }

        // Skip the stage if the name does not match regex and it's not a _build_trident stage
        if (_stage_name_match(stage, stage_name_regexp) != true && stage['stage'] != '_build_trident') {
          continue
        }

      }
      active_stages_this_phase++
    }

    for (stage in cop) {
      // Create a copy of the current loop iterator
      def cos = stage

      // Skip the stage if it is not enabled
      if (cos['enabled'] != true) {
          echo 'Skipping ' + cos['name'] + ' because it\'s disabled'
          continue
      }

      // If BLACK_DUCK_SCAN and DEPLOY_TRIDENT are not true apply the coverage and
      // stage name filters
      if (env.BLACK_DUCK_SCAN == null && env.DEPLOY_TRIDENT == null) {

        // Skip the stage if the coverage does not match
        if (_coverage_match(cos, coverage) != true) {
          echo 'Skipping ' + cos['name'] + ' because it\'s coverage ' + cos['coverage'] + ' does not match ' + coverage

          // Documentation coverage should report all other pre-merge stages as skipped
          if (coverage == 'documentation' && _coverage_match(cos, 'pre-merge') == true) {
            // Notify Github the stage has been a success because it's been skipped
            _notify_github(cos, commit, 'skipped', 'Stage has been skipped', 'success')
          }

          continue
        }

        // Skip the stage if the name does not match regex and it's not a _build_trident stage
        if (_stage_name_match(cos, stage_name_regexp) != true && cos['stage'] != '_build_trident') {
          echo 'Skipping ' + cos['name'] + ' because it\'s name does not match ' + stage_name_regexp
          continue
        }

      }

      // Create the stage based on the stage attribute
      if (cos['stage'] == '_build_documentation') {

        if(map[group_index] == null) { map[group_index] = [:] }
        stage_index++
        def name = (group_index + 1) + '-' + stage_index + '-' + cos['name']
        echo 'Creating documentation build stage ' + name
        map[group_index].put(name, _build_documentation(name, ssh_options, cos))
        parallel_stages++
        allocated_stages_this_phase++
        stage_list += "$name\n"

      } else if (cos['stage'] == '_build_trident') {

        if(map[group_index] == null) { map[group_index] = [:] }
        stage_index++
        def name = (group_index + 1) + '-' + stage_index + '-' + cos['name']
        echo 'Creating compile stage ' + name
        map[group_index].put(name, _build_trident(name, ssh_options, cos))
        parallel_stages++
        allocated_stages_this_phase++
        stage_list += "$name\n"

      } else if (cos['stage'] == '_csi_sanity') {

        if(map[group_index] == null) { map[group_index] = [:] }
        stage_index++
        def name = (group_index + 1) + '-' + stage_index + '-' + cos['name']
        echo 'Creating CSI Sanity test stage ' + name
        map[group_index].put(name, _csi_sanity(name, ssh_options, cos))
        parallel_stages++
        allocated_stages_this_phase++
        stage_list += "$name\n"

      } else if (cos['stage'] == '_unit_test') {

        if(map[group_index] == null) { map[group_index] = [:] }
        stage_index++
        def name = (group_index + 1) + '-' + stage_index + '-' + cos['name']
        echo 'Creating unit test stage ' + name
        map[group_index].put(name, _unit_test(name, ssh_options, cos))
        parallel_stages++
        allocated_stages_this_phase++
        stage_list += "$name\n"

      } else if (cos['stage'] == '_whelk_test') {

        if(map[group_index] == null) { map[group_index] = [:] }
        stage_index++
        def name = (group_index + 1) + '-' + stage_index + '-' + cos['name']
        def msg = 'Creating whelk test stage ' + name + ' using backends ' + cos['backend']
        if (cos['install_backend']) {
            msg += '/' + cos['install_backend']
        }
        echo msg
        map[group_index].put(name, _whelk_test(name, ssh_options, cos))
        parallel_stages++
        allocated_stages_this_phase++
        stage_list += "$name\n"

      } else {
        echo cos['name'] + ' has an unkown stage: ' + cos['stage']
      }

      if ((parallel_stages == parallelism) || allocated_stages_this_phase >= active_stages_this_phase) {
        // Increment out unique group id
        group_index++

        // Reset the the counter vars to create a new group of stages
        stage_index = 0
        parallel_stages = 0
      }
    }
  }

  writeFile file: "stage_list.log", text: stage_list
  sh (label: "Sleep", script: "sleep 1")
  archiveArtifacts allowEmptyArchive: true, artifacts: "stage_list.log"

  return map
}

def _build_documentation(String name, String ssh_options, Map spec) {
  return {
    stage(name) {

      // Create var for HOLD_JIG
      def hold_jig = spec['hold_jig']

      // Define a unqiue random string
      def rs = _random_string(15)

      // Define a unique SCS purpose
      def purpose = rs

      // Define vars for the VM attributes
      def hostname = ''
      def domain = ''
      def ip_address = ''
      def scs_password = ''

      // Define a var for the path on $ip_address
      vm_path = ''

      // Define a var to track the stage status
      def status = 'pending'

      // Define a var to track the result of getMessage()
      def message = ''

      // Initialize the result file for this stage
      _initialize_status_file(name, status)

      try {

        sh (
          label: "Create directory $env.WORKSPACE/$name",
          script: "mkdir -p $name"
        )

        sh (label: "Sleep", script: "sleep 1")

        // Notify Github of our status
        _notify_github(spec, commit, name, 'Stage is running', status)

        // Create SCS VM(s) and create attribe variables we can use later
        def request = spec['request']
        def scs = _scs_create(name, request, purpose)
        hostname = scs[0]
        domain = scs[1]
        ip_address = scs[2]
        scs_password = scs[3]

        // Run the ansible playbook
        def target = hostname + '[' + ip_address + ']'
        def options = ['--ssh-public-key', "$env.WORKSPACE/tools/$env.SSH_PUBLIC_KEY_PATH"]
        if (spec['vm_provider'] == 'SCS') {
            options.push('--ssh-password')
            options.push(scs_password)
        } else if (spec['vm_provider'] == 'AWS') {
            options.push('--ssh-private-key')
            options.push("$env.WORKSPACE/tools/$env.SSH_PRIVATE_KEY_PATH")
        }
        options.push('--extra-vars')
        options.push(
          '\'context=' + spec['context'] + ' ' +
          'go_download_url=' + spec['go_download_url'] + ' ' +
          'package_repository=' + env.PACKAGE_REPOSITORY + '\''
        )
        _run_playbook(name, spec['post_deploy_playbook'], target, options)

        // Update the var for the path on $ip_address
        vm_path = "/tmp/" + _replace(env.BUILD_TAG, '/', '-')

        sh (
          label: "Install sphinx packages on $ip_address",
          script: "ssh $ssh_options root@$ip_address 'pip install -U Sphinx==1.7 sphinx_rtd_theme sphinx-autobuild'"
        )

        sh (
          label: "Create directory go/src/github.com/netapp on $ip_address",
          script: "ssh $ssh_options root@$ip_address mkdir -p $vm_path/go/src/github.com/netapp"
        )

        sh (
          label: "Create directory go/pkg on $ip_address",
          script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/go/pkg'"
        )

        sh (
          label: "Create directory go/bin on $ip_address",
          script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/go/bin'"
        )

        sh (label: "Sleep", script: "sleep 1")

        _scp(
          ssh_options,
          'root',
          ip_address,
          "src/github.com/netapp/trident",
          "$vm_path/go/src/github.com/netapp",
          true,
          true,
          true
        )

        sh (
          label: "Clean the source",
          script: "ssh $ssh_options root@$ip_address " +
            "'cd $vm_path/go/src/github.com/netapp/trident;" +
            "export GOPATH=$vm_path/go;" +
            "make clean'"
        )

        // Build the html docs
        sh (
          label: "Build the HTML docs",
          script: "ssh $ssh_options root@$ip_address " +
            "'cd $vm_path/go/src/github.com/netapp/trident/docs;" +
            "export GOPATH=$vm_path/docs;" +
            "make html'"
        )

        // Set the status to success
        status = 'success'

        // Write the status to a file
        _write_status_file(name, status)

      } catch(Exception e) {

        // Set the status to error
        status = 'error'

        // Update the status file to fail
        _write_status_file(name, status)

        // Update the message var
        message = e.getMessage()

        // Fail the run
        error "$name: $message"

      } finally {

        // Create the cleanup script in case we need it
        _create_cleanup_script(spec, name, purpose, ip_address, vm_path, false)

        if(hold_jig == 'always' || (status != 'success' && hold_jig == 'onfailure')) {
          // If HOLD_JIG is always or onfailure print the hold jig message
          _print_hold_jig_message()
        } else {
          if (spec['vm_provider'] == 'SCS') {
            _scs_delete(purpose)
          } else if (spec['vm_provider'] == 'AWS') {
            _aws_delete(spec, name)
          }
        }

        // Notify Github of our status
        _notify_github(spec, commit, name, 'Stage has completed', status)

        // Archive everything in the stage workdir
        _archive_artifacts(name)

      }
    }
  }
}

def _build_trident(String name, String ssh_options, Map spec) {
  return {
    stage(name) {

      // Create var for HOLD_JIG
      def hold_jig = spec['hold_jig']

      // Define a unqiue random string
      def rs = _random_string(15)

      // Define a unique SCS purpose
      def purpose = rs

      // Define vars for the VM attributes
      def hostname = ''
      def domain = ''
      def ip_address = ''
      def scs_password = ''

      // Define a var for the path on $ip_address
      def vm_path = ''

      // Define a var to track the stage status
      def status = 'pending'

      // Define a var to track the result of getMessage()
      def message = ''

      // Initialize the result file for this stage
      _initialize_status_file(name, status)

      if (env.DEPLOY_TRIDENT && env.COMMIT_HASH) {
        echo "Using the supplied git commit hash $env.COMMIT_HASH"
        commit = env.COMMIT_HASH.trim()
      }

      try {

        // Create a working dir for this spec
        sh (
          label: "Create directory $env.WORKSPACE/$name",
          script: "mkdir -p $name"
        )
        sh (label: "Sleep", script: "sleep 1")

        // Notify Github of our status
        if (env.DEPLOY_TRIDENT) {
          echo "DEPLOY_TRIDENT=true, skipping Github notification"
        } else if (env.BLACK_DUCK_SCAN) {
          echo "BLACK_DUCK_SCAN=true, skipping Github notification"
        } else {
          _notify_github(spec, commit, name, 'Stage is running', status)
        }

        // Create SCS VM(s) and create attribe variables we can use later
        def request = spec['request']
        def scs = _scs_create(name, request, purpose)
        hostname = scs[0]
        domain = scs[1]
        ip_address = scs[2]
        scs_password = scs[3]

        // Run the ansible playbook
        def target = hostname + '[' + ip_address + ']'
        def options = ['--ssh-public-key', "$env.WORKSPACE/tools/$env.SSH_PUBLIC_KEY_PATH"]
        if (spec['vm_provider'] == 'SCS') {
            options.push('--ssh-password')
            options.push(scs_password)
        } else if (spec['vm_provider'] == 'AWS') {
            options.push('--ssh-private-key')
            options.push("$env.WORKSPACE/tools/$env.SSH_PRIVATE_KEY_PATH")
        }
        options.push('--extra-vars')
        options.push(
          '\'context=' + spec['context'] + ' ' +
          'docker_ce_version=' + spec['docker_ce_version'] + ' ' +
          'go_download_url=' + spec['go_download_url'] + ' ' +
          'package_repository=' + env.PACKAGE_REPOSITORY + '\''
         )
        _run_playbook(name, spec['post_deploy_playbook'], target, options)

        // Create a var for the GOPATH on $ip_address
        vm_path = "/tmp/" + _replace(env.BUILD_TAG, '/', '-')

        sh (
          label: "Create directory go/src/github.com/netapp on $ip_address",
          script: "ssh $ssh_options root@$ip_address mkdir -p $vm_path/go/src/github.com/netapp"
        )

        sh (
          label: "Create directory go/pkg on $ip_address",
          script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/go/pkg'"
        )

        sh (
          label: "Create directory go/bin on $ip_address",
          script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/go/bin'"
        )
        sh (label: "Sleep", script: "sleep 1")

        _scp(
          ssh_options,
          'root',
          ip_address,
          "src/github.com/netapp/trident",
          "$vm_path/go/src/github.com/netapp",
          true,
          true,
          true
        )

        sh (
          label: "Clean the source",
          script: "ssh $ssh_options root@$ip_address " +
            "'cd $vm_path/go/src/github.com/netapp/trident;" +
            "export GOPATH=$vm_path/go;" +
            "make clean'"
        )

        try {
          def content = ''
          if (env.DEPLOY_TRIDENT && env.BUILD_TYPE == 'stable') {
            echo "Creating the build script for stable/master deployment"
            content = (
              "cd $vm_path/go/src/github.com/netapp/trident\n" +
              "docker login $env.PUBLIC_DOCKER_REGISTRY " +
              "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
              "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD\n" +
              "export GOPATH=$vm_path/go\n" +
              "export BUILD_TYPE=$env.BUILD_TYPE\n" +
              "export TRIDENT_VERSION=$env.TRIDENT_VERSION\n" +
              "export REGISTRY_ADDR=$env.PRIVATE_DOCKER_REGISTRY\n" +
              "make dist > build.log 2>&1"
            )
          } else if (env.DEPLOY_TRIDENT && env.BUILD_TYPE != 'stable') {
            echo "Creating the build script for alpha or beta deployment"
            content = (
              "cd $vm_path/go/src/github.com/netapp/trident\n" +
              "docker login $env.PUBLIC_DOCKER_REGISTRY " +
              "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
              "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD\n" +
              "export GOPATH=$vm_path/go\n" +
              "export BUILD_TYPE=$env.BUILD_TYPE\n" +
              "export TRIDENT_VERSION=$env.TRIDENT_VERSION\n" +
              "export BUILD_TYPE_REV=$env.TRIDENT_REVISION\n" +
              "export REGISTRY_ADDR=$env.PRIVATE_DOCKER_REGISTRY\n" +
              "make dist > build.log 2>&1"
            )
          } else {
            echo "Creating the build script for testing"
            content = (
              "cd $vm_path/go/src/github.com/netapp/trident\n" +
              "docker login $env.PRIVATE_DOCKER_REGISTRY " +
              "-u $env.PRIVATE_DOCKER_REGISTRY_USERNAME " +
              "-p $env.PRIVATE_DOCKER_REGISTRY_PASSWORD\n" +
              "export GOPATH=$vm_path/go\n" +
              "export BUILD_TYPE=test\n" +
              "export BUILD_TYPE_REV=" + _replace(env.BUILD_TAG, '/', '-') + "\n" +
              "export REGISTRY_ADDR=$env.PRIVATE_DOCKER_REGISTRY\n" +
              "export DIST_REGISTRY=$env.PRIVATE_DOCKER_REGISTRY\n" +
              "make dist > build.log 2>&1"
            )
          }

          writeFile file: "$name/build.sh", text: content
          sh (label: "Sleep", script: "sleep 1")

          _scp(
            ssh_options,
            'root',
            ip_address,
            "$name/build.sh",
            vm_path,
            true,
            false,
            true
          )

          sh (
            label: "Execute build.sh on $ip_address",
            script: "ssh $ssh_options root@$ip_address 'cd $vm_path;sh ./build.sh > build.log 2>&1'"
          )

          sh (label: "Sleep", script: "sleep 1")

          sh (
            label: "Rename trident_orchestrator",
            script: "ssh $ssh_options root@$ip_address " +
              "cp $vm_path/go/src/github.com/netapp/trident/bin/trident_orchestrator " +
              "$vm_path/go/src/github.com/netapp/trident/contrib/docker/plugin/trident"
          )
          sh (label: "Sleep", script: "sleep 1")

          _scp(
            ssh_options,
            'root',
            ip_address,
            "$vm_path/go/src/github.com/netapp/trident/contrib/docker/plugin/trident",
            ".",
            false,
            false,
            true
          )

          sh (label: "Sleep", script: "sleep 1")

          archiveArtifacts allowEmptyArchive: true, artifacts: 'trident'

          sh (label: "Sleep", script: "sleep 1")

          _scp(
            ssh_options,
            'root',
            ip_address,
            "$vm_path/go/src/github.com/netapp/trident/trident-installer-*.tar.gz",
            ".",
            false,
            false,
            true
          )

          sh (label: "Sleep", script: "sleep 1")

          // Archive the tgz
          archiveArtifacts allowEmptyArchive: true, artifacts: 'trident-installer-*.tar.gz'

          // Create a var to track the docker image name
          def image = ''
          if (env.DEPLOY_TRIDENT) {

            def trident_version = env.TRIDENT_VERSION
            def trident_revision = env.TRIDENT_REVISION
            def fields = trident_version.split("\\.")
            def major = fields[0]
            def minor = fields[1]
            def patch = fields[2]
            def tag = trident_version
            def major_minor_tag = "${major}.${minor}"
            def repository = env.PUBLIC_DOCKER_REGISTRY + '/trident'

            // Tag and push the image appropriately
            if (env.BUILD_TYPE == 'stable' && env.BRANCH_NAME == 'master') {
              echo (
                "Docker repository: $repository\n" +
                "Docker tag: $tag\n"
              )
              sh (
                label: "Push $repository:$tag from $ip_address from $ip_address",
                script: "ssh $ssh_options root@$ip_address '" +
                  "docker login " +
                  "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
                  "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD;" +
                  "docker push $repository:$tag'"
              )

              sh (
                label: "Tag $repository:$tag $repository:$major_minor_tag from $ip_address",
                script: "ssh $ssh_options root@$ip_address '" +
                  "docker login " +
                  "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
                  "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD;" +
                  "docker tag $repository:$tag $repository:$major_minor_tag'"
              )

              sh (
                label: "Push $repository:$major_minor_tag from $ip_address",
                script: "ssh $ssh_options root@$ip_address '" +
                  "docker login " +
                  "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
                  "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD;" +
                  "docker push $repository:$major_minor_tag'"
              )
            } else if (env.BUILD_TYPE == 'stable' && env.BRANCH_TYPE == 'stable') {
              echo (
                "Docker repository: $repository\n" +
                "Docker tag: $tag\n"
              )
              sh (
                label: "Push $repository:$tag from $ip_address",
                script: "ssh $ssh_options root@$ip_address '" +
                  "docker login " +
                  "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
                  "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD;" +
                  "docker push $repository:$tag'"
              )

              sh (
                label: "Tag $repository:$tag $repository:$major_minor_tag from $ip_address",
                script: "ssh $ssh_options root@$ip_address '" +
                  "docker login " +
                  "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
                  "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD;" +
                  "docker tag $repository:$tag $repository:$major_minor_tag'"
              )

              sh (
                label: "Push $repository:$major_minor_tag from $ip_address",
                script: "ssh $ssh_options root@$ip_address '" +
                  "docker login " +
                  "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
                  "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD;" +
                  "docker push $repository:$major_minor_tag'"
              )

            } else if (env.BUILD_TYPE != 'stable') {
              tag = (
                trident_version +
                '-' + env.BUILD_TYPE +
                '.' + env.TRIDENT_REVISION)
              echo (
                "Docker repository: $repository\n" +
                "Docker tag: $tag\n"
              )
              sh (
                label: "Push $repository:$tag from $ip_address",
                script: "ssh $ssh_options root@$ip_address '" +
                  "docker login " +
                  "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
                  "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD;" +
                  "docker push $repository:$tag'"
              )
            }

            if (env.MAKE_LATEST) {
              sh (
                label: "Tag $repository:$tag $repository:latest from $ip_address",
                script: "ssh $ssh_options root@$ip_address '" +
                  "docker login " +
                  "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
                  "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD;" +
                  "docker tag $repository:$tag $repository:latest'"
              )

              sh (
                label: "Push $repository:latest from $ip_address",
                script: "ssh $ssh_options root@$ip_address '" +
                  "docker login " +
                  "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
                  "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD;" +
                  "docker push $repository:latest'"
              )
            }

            // Get the trident image string using the git commit hash
            image = _get_image(ssh_options, ip_address, tag)

          } else {

            // Get the trident image string using the git commit hash
            image = _get_image(ssh_options, ip_address, _replace(env.BUILD_TAG, '/', '-'))

          }

          // Save the image name and tag so we can use it later on
          writeFile file: "trident_docker_image.log", text: "$image\n"
          sh (label: "Sleep", script: "sleep 1")

          sh (
            label: "Save the trident image",
            script: "ssh $ssh_options root@$ip_address " +
              "docker save $image -o $vm_path/trident_docker_image.tgz"
          )

          _scp(
            ssh_options,
            'root',
            ip_address,
            "$vm_path/trident_docker_image.tgz",
            '.',
            false,
            false,
            true
          )

          // Archive the tgz
          archiveArtifacts allowEmptyArchive: true, artifacts: "trident_docker_image.tgz"

        } catch(Exception e) {
          error e.getMessage()
        } finally {
          _scp(
            ssh_options,
            'root',
            ip_address,
            "$vm_path/go/src/github.com/netapp/trident/build.log",
            name,
            false,
            false,
            false
          )
        }

        try {
          // Create the script to create and push the docker plugin image
          def content = ''

          if (env.DEPLOY_TRIDENT) {
            content += (
              "docker login " +
              "-u $env.PUBLIC_DOCKER_REGISTRY_USERNAME " +
              "-p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD\n" +
              "cp -f ../../../chroot-host-wrapper.sh .\n" +
              "./createFS\n" +
              "sudo cp -f plugin.json myplugin/config.json\n" +
              "sudo docker logout\n" +
              "sudo docker login -u $env.PUBLIC_DOCKER_REGISTRY_USERNAME -p $env.PUBLIC_DOCKER_REGISTRY_PASSWORD\n"
            )
          } else {
            content += (
              "docker login $env.PRIVATE_DOCKER_REGISTRY " +
              "-u $env.PRIVATE_DOCKER_REGISTRY_USERNAME " +
              "-p $env.PRIVATE_DOCKER_REGISTRY_PASSWORD\n" +
              "cp -f ../../../chroot-host-wrapper.sh .\n" +
              "./createFS\n" +
              "sudo cp -f plugin.json myplugin/config.json\n" +
              "sudo docker logout\n" +
              "sudo docker login $env.PRIVATE_DOCKER_REGISTRY -u $env.PRIVATE_DOCKER_REGISTRY_USERNAME -p $env.PRIVATE_DOCKER_REGISTRY_PASSWORD\n"
            )
          }

          if (env.DEPLOY_TRIDENT) {
            def repository = env.PUBLIC_DOCKER_REGISTRY + '/trident-plugin'
            def trident_version = env.TRIDENT_VERSION
            def tag = trident_version
            def fields = trident_version.split("\\.")
            def major = fields[0]
            def minor = fields[1]
            def patch = fields[2]
            def major_minor_tag = "${major}.${minor}"
            if (env.BUILD_TYPE == 'stable' && env.BRANCH_NAME == 'master') {
              // For a stable build using the master branch we need :<version> and :<major>.<minor>
              echo (
                "Creating docker plugin script for:\n" +
                "Docker repository: $repository\n" +
                "Docker tags: $tag $major_minor_tag"
              )
              content += (
                "sudo docker plugin create $repository:$tag ./myplugin\n" +
                "sudo docker plugin push $repository:$tag\n" +
                "sudo docker plugin rm -f $repository:$tag\n" +
                "sudo docker plugin create $repository:$major_minor_tag ./myplugin\n" +
                "sudo docker plugin push $repository:$major_minor_tag\n" +
                "sudo docker plugin rm -f $repository:$major_minor_tag\n"
               )
             } else if (env.BUILD_TYPE == 'stable' && env.BRANCH_TYPE == 'stable') {
              // For a stable build not using the master branch we need :<version> and :<major>.<minor>
              echo (
                "Creating docker plugin script for:\n" +
                "Docker repository: $repository\n" +
                "Docker tags: $tag major_minor_tag"
              )
              content += (
                "sudo docker plugin create $repository:$tag ./myplugin\n" +
                "sudo docker plugin push $repository:$tag\n" +
                "sudo docker plugin rm -f $repository:$tag\n" +
                "sudo docker plugin create $repository:$major_minor_tag ./myplugin\n" +
                "sudo docker plugin push $repository:$major_minor_tag\n" +
                "sudo docker plugin rm -f $repository:$major_minor_tag\n"
              )
            } else if (env.BUILD_TYPE != 'stable') {
              // For alpha and beta builds we need just :<version>
              tag = (
                trident_version +
                '-' + env.BUILD_TYPE +
                '.' + env.TRIDENT_REVISION)
              echo (
                "Creating docker plugin script for:\n" +
                "Docker repository: $repository\n" +
                "Docker tags: $tag"
              )
              content += (
                "sudo docker plugin create $repository:$tag ./myplugin\n" +
                "sudo docker plugin push $repository:$tag\n" +
                "sudo docker plugin rm -f $repository:$tag\n"
               )
            }
            if (env.MAKE_LATEST) {
              echo (
                "Creating docker plugin script for:\n" +
                "Docker repository: $repository\n" +
                "Docker tags: latest"
              )
              content += (
                "sudo docker plugin create $repository:latest ./myplugin\n" +
                "sudo docker plugin push $repository:latest\n" +
                "sudo docker plugin rm -f $repository:latest\n"
              )
            }
          } else {
            def repository = env.PRIVATE_DOCKER_REGISTRY + '/trident-plugin'

            // For test builds we need the git commit hash as the image tag
            content += (
              "sudo docker plugin create $repository:" + _replace(env.BUILD_TAG, '/', '-') + " myplugin\n" +
              "sudo docker plugin push $repository:" + _replace(env.BUILD_TAG, '/', '-') + "\n"
            )
          }

          writeFile file: "$name/create_plugin.sh", text: content
          sh (label: "Sleep", script: "sleep 1")

          _scp(
            ssh_options,
            'root',
            ip_address,
            "$name/create_plugin.sh",
            "$vm_path/go/src/github.com/netapp/trident/contrib/docker/plugin",
            true,
            false,
            true
          )

          sh (
            label: "Execute create_plugin.sh on $ip_address",
            script: "ssh $ssh_options root@$ip_address '" +
              "cd $vm_path/go/src/github.com/netapp/trident/contrib/docker/plugin;" +
              "sh create_plugin.sh > create_plugin.log 2>&1'"
          )

          sh (label: "Sleep", script: "sleep 1")

        } catch(Exception e) {
          error e.getMessage()
        } finally {
          _scp(
            ssh_options,
            'root',
            ip_address,
            "$vm_path/go/src/github.com/netapp/trident/contrib/docker/plugin/create_plugin.log",
            name,
            false,
            false,
            true
          )

        }

        if (env.BLACK_DUCK_SCAN) {
          try {

            def url = (
              "https://sig-repo.synopsys.com/" +
              "bds-integrations-release/com/synopsys/integration/synopsys-detect/" +
              "$env.BLACK_DUCK_SYNOPSIS_VERSION/synopsys-detect-${env.BLACK_DUCK_SYNOPSIS_VERSION}.jar"
            )

            sh (
              label: "Get the Black Duck JAR file",
              script: "ssh $ssh_options root@$ip_address " +
                "'cd $vm_path;" +
                "curl $url --output synopsys-detect-${env.BLACK_DUCK_SYNOPSIS_VERSION}.jar'"
            )

            sh (label: "Sleep", script: "sleep 1")

            sh (
              label: "Show directory contents on $vm_path",
              script: "ssh $ssh_options root@$ip_address 'ls -l $vm_path'"
            )

            def code_location_name = "Trident_${env.BLACK_DUCK_PROJECT_VERSION}_code"
            def bom_aggregate_name = "Trident_${env.BLACK_DUCK_PROJECT_VERSION}_bom"

            // java  -jar synopsys-detect-$SYNOPSYS_VERSION.jar
            //       --blackduck.url=https://blackduck.eng.netapp.com
            //       --blackduck.trust.cert=true \
            //       --blackduck.api.token=NmJlY2IxMWUtZmIxMC00ODAwLThmNjktNjA5OTVhMDBmMzk3OmUyYzRmOTk4LTYwZDctNDk0Zi04MDYyLTI4NjUxZmRhN2QwNA== \
            //       --detect.project.name='Trident'
            //       --detect.project.version.name='19.07.1'
            //       --detect.cleanup=false \
            //       --detect.blackduck.signature.scanner.exclusion.name.patterns='directoryDoesNotExist' \
            //       --detect.detector.search.depth=50
            //       --detect.detector.search.continue=true \
            //       --detect.code.location.name="Trident_19.07.1_code"
            //       --detect.bom.aggregate.name="Trident_19.07.1_bom" \
            //       --logging.level.com.blackducksoftware.integration=DEBUG \
            //       --detect.blackduck.signature.scanner.paths=${bamboo.build.working.directory}/go/src/github.com/netapp/trident

            sh (
              label: "Execute the black duck scan on $ip_address",
              script: "ssh $ssh_options root@$ip_address " +
                "'cd $vm_path;" +
                "java -jar \"synopsys-detect-${env.BLACK_DUCK_SYNOPSIS_VERSION}.jar\" " +
                "--blackduck.url=$env.BLACK_DUCK_URL " +
                "--blackduck.trust.cert=$env.BLACK_DUCK_TRUST_CERT " +
                "--blackduck.api.token=$env.BLACK_DUCK_API_TOKEN " +
                "--detect.project.name=$env.BLACK_DUCK_PROJECT_NAME " +
                "--detect.project.version.name=$env.BLACK_DUCK_PROJECT_VERSION " +
                "--detect.cleanup=$env.BLACK_DUCK_CLEANUP " +
                "--detect.blackduck.signature.scanner.exclusion.name.patterns=\"$env.BLACK_DUCK_EXCLUSION_PATTERN\" " +
                "--detect.detector.search.depth=$env.BLACK_DUCK_SEARCH_DEPTH " +
                "--detect.detector.search.continue=$env.BLACK_DUCK_SEARCH_CONTINUE " +
                "--detect.code.location.name=$code_location_name " +
                "--detect.bom.aggregate.name=$bom_aggregate_name " +
                "--logging.level.com.blackducksoftware.integration=$env.BLACK_DUCK_SOFTWARE_INTEGRATION " +
                "--detect.blackduck.signature.scanner.paths=$vm_path/go/src/github.com/netapp/trident > black_duck.log 2>&1 || true'"
            )

          } catch(Exception e) {
            error "Black Duck scan failure"
          } finally {
            _scp(
              ssh_options,
              'root',
              ip_address,
              "$vm_path/black_duck.log",
              name,
              false,
              false,
              false
            )
          }
        }

        if (env.DEPLOY_TRIDENT) {

          _scp(
            ssh_options,
            'root',
            ip_address,
            "src2",
            "$vm_path/go/src2",
            true,
            true,
            true
          )

          // Tag the release
          def release_name = 'v' + env.TRIDENT_VERSION
          try {
            echo "Creating script to tag the release"

            def content = (
              "tag=`git tag --points-at $commit`\n" +
              "if [ \"$release_name\" != \"\$tag\" ]; then\n" +
              "    echo \"Creating tag $release_name at commit $commit\"\n" +
              "    git tag $release_name $commit\n" +
              "    git push origin tag $release_name\n" +
              "fi\n"
            )

            writeFile file: "$name/tag_release.sh", text: content

            sh (label: "Sleep", script: "sleep 1")

            _scp(
              ssh_options,
              'root',
              ip_address,
              "$name/tag_release.sh",
              "$vm_path/go/src2/github.com/netapp/trident",
              true,
              false,
              true
            )

            sh (
              label: "Execute tag_release.sh on $ip_address",
              script: "ssh $ssh_options root@$ip_address '" +
                "cd $vm_path/go/src2/github.com/netapp/trident/;" +
                "sh ./tag_release.sh > tag_release.log 2>&1'"
            )

            sh (label: "Sleep", script: "sleep 1")

          } catch(Exception e) {
            error "Deployment failure"
          error e.getMessage()
          } finally {
            // Copy the tag_release.log from the VM
            _scp(
              ssh_options,
              'root',
              ip_address,
              "$vm_path/go/src2/github.com/netapp/trident/tag_release.log",
              name,
              false,
              false,
              false
            )
          }

          sh (
            label: "Install the required modules for release_to_github.py",
            script: "ssh $ssh_options root@$ip_address 'pip install requests uritemplate'"
          )

          sh (label: "Sleep", script: "sleep 1")

          _scp(
            ssh_options,
            'root',
            ip_address,
            "tools",
            vm_path,
            true,
            true,
            true
          )

          if (env.BUILD_TYPE == 'stable') {

            def tarball = "trident-installer-${env.TRIDENT_VERSION}.tar.gz"

            sh (
              label: "Copy $tarball to src2/github.com/netapp/trident",
              script: "ssh $ssh_options root@$ip_address 'cp " +
                "$vm_path/go/src/github.com/netapp/trident/$tarball " +
                "$vm_path/go/src2/github.com/netapp/trident'"
            )

            sh (label: "Sleep", script: "sleep 1")

            sh (
              label: "Execute release_to_github.py on $ip_address",
              script: "ssh $ssh_options root@$ip_address '" +
                "cd $vm_path/go/src2/github.com/netapp/trident;" +
                "python $vm_path/tools/utils/release_to_github.py " +
                "--changelog CHANGELOG.md " +
                "--github-org $env.TRIDENT_PUBLIC_GITHUB_ORG " +
                "--github-password $env.GITHUB_TOKEN " +
                "--github-repo $env.TRIDENT_PUBLIC_GITHUB_REPO " +
                "--github-user $env.GITHUB_USERNAME " +
                "--release-name $release_name " +
                "--release-hash $commit " +
                "--tarball $tarball'"
            )
          } else {

            def tarball = "trident-installer-${env.TRIDENT_VERSION}-${env.BUILD_TYPE}.${env.TRIDENT_REVISION}.tar.gz"

            sh (
              label: "Copy $tarball to src2/github.com/netapp/trident",
              script: "ssh $ssh_options root@$ip_address 'cp " +
                "$vm_path/go/src/github.com/netapp/trident/$tarball " +
                "$vm_path/go/src2/github.com/netapp/trident'"
            )

            sh (label: "Sleep", script: "sleep 1")

            sh (
              label: "Execute release_to_github.py on $ip_address",
              script: "ssh $ssh_options root@$ip_address '" +
                "cd $vm_path/go/src2/github.com/netapp/trident;" +
                "python $vm_path/tools/utils/release_to_github.py " +
                "--changelog CHANGELOG.md " +
                "--github-org $env.TRIDENT_PUBLIC_GITHUB_ORG " +
                "--github-password $env.GITHUB_TOKEN " +
                "--github-repo $env.TRIDENT_PUBLIC_GITHUB_REPO " +
                "--github-user $env.GITHUB_USERNAME " +
                "--prerelease " +
                "--release-name ${release_name}-${env.BUILD_TYPE}.${env.TRIDENT_REVISION} " +
                "--release-hash $commit " +
                "--tarball $tarball'"
            )
          }
        }

        // Set the status to success
        status = 'success'

        // Write the status to a file
        _write_status_file(name, status)

      } catch(Exception e) {

        // Set the status to error
        status = 'error'

        // Update the status file to fail
        _write_status_file(name, status)

        // Update the message var
        message = e.getMessage()

        // Fail the run
        error "$name: $message"


      } finally {

        // Create the cleanup script in case we need it
        _create_cleanup_script(spec, name, purpose, ip_address, vm_path, false)

        if(hold_jig == 'always' || (status != 'success' && hold_jig == 'onfailure')) {
          // If HOLD_JIG is always or onfailure print the hold jig message
          _print_hold_jig_message()
        } else {
          if (spec['vm_provider'] == 'SCS') {
            _scs_delete(purpose)
          } else if (spec['vm_provider'] == 'AWS') {
            _aws_delete(spec, name)
          }
        }

        // Notify Github of our status
        if (env.DEPLOY_TRIDENT) {
          echo "DEPLOY_TRIDENT=true, skipping Github notification"
        } else if (env.BLACK_DUCK_SCAN) {
          echo "BLACK_DUCK_SCAN=true, skipping Github notification"
        } else {
          _notify_github(spec, commit, name, 'Stage has completed', status)
        }

        // Archive everything in the stage workdir
        _archive_artifacts(name)

      }
    }
  }
}

def _csi_sanity(String name, String ssh_options, Map spec) {
  return {
    stage(name) {

      // Create var for HOLD_JIG
      def hold_jig = spec['hold_jig']

      // Define backend var
      def backend = spec['backend']

      // Define a unqiue random string
      def rs = _random_string(15)

      // Define a unique SCS purpose
      def purpose = rs

      // Define vars for the VM attributes
      def hostname = ''
      def domain = ''
      def ip_address = ''
      def scs_password = ''

      // Define a var for the path on $ip_address
      def vm_path = ''

      // Track if setup was complete
      def backends_configured = false

      // Define a var to track the stage status
      def status = 'pending'

      // Define a var to track the result of getMessage()
      def message = ''

      // Initialize the result file for this stage
      _initialize_status_file(name, status)

      try {

        sh (
          label: "Create directory $env.WORKSPACE/junit",
          script: "mkdir -p junit"
        )

        sh (
          label: "Create directory $env.WORKSPACE/$name",
          script: "mkdir -p $name"
        )

        // Notify Github of our status
        _notify_github(spec, commit, name, 'Stage is running', status)

        if (spec['vm_provider'] == 'SCS') {

          // Create SCS VM(s)
          def request = spec['request']
          def scs = _scs_create(name, request, purpose)

          // Create attribe variables we can use later
          hostname = scs[0]
          domain = scs[1]
          ip_address = scs[2]
          scs_password = scs[3]

        } else if (spec['vm_provider'] == 'AWS') {

          // Create AWS VM(s)
          def aws = _aws_create(spec, name)

          // Create attribe variables we can use later
          echo "Getting hostname from fqdn"
          hostname = _get_hostname_from_fqdn(aws[0])
          echo "Getting domain from fqdn"
          domain = _get_domain_from_fqdn(aws[0])
          ip_address = aws[1]

        }

        // Run the ansible playbook
        def target = hostname + '[' + ip_address + ']'
        def options = ['--ssh-public-key', "$env.WORKSPACE/tools/$env.SSH_PUBLIC_KEY_PATH"]
        if (spec['vm_provider'] == 'SCS') {
            options.push('--ssh-password')
            options.push(scs_password)
        } else if (spec['vm_provider'] == 'AWS') {
            options.push('--ssh-private-key')
            options.push("$env.WORKSPACE/tools/$env.SSH_PRIVATE_KEY_PATH")
        }
        options.push('--extra-vars')
        options.push(
          '\'context=' + spec['context'] + ' ' +
          'docker_ce_version=' + spec['docker_ce_version'] + ' ' +
          'go_download_url=' + spec['go_download_url'] + ' ' +
          'package_repository=' + env.PACKAGE_REPOSITORY + '\''
        )
        _run_playbook(name, spec['post_deploy_playbook'], target, options)

        // Create a var for the GOPATH on $ip_address
        vm_path = "/tmp/" + _replace(env.BUILD_TAG, '/', '-')

        sh (
          label: "Create directory /var/lib/trident/tracking on $ip_address to support node volume expand",
          script: "ssh $ssh_options root@$ip_address mkdir -p /var/lib/trident/tracking"
        )

        sh (
          label: "Create directory go/src/github.com/kubernetes-csi on $ip_address",
          script: "ssh $ssh_options root@$ip_address mkdir -p $vm_path/go/src/github.com/kubernetes-csi"
        )

        sh (
          label: "Create directory go/pkg on $ip_address",
          script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/go/pkg'"
        )

        sh (
          label: "Create directory go/bin on $ip_address",
          script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/go/bin'"
        )

        sh (label: "Sleep", script: "sleep 1")

        _scp(
          ssh_options,
          'root',
          ip_address,
          "src/github.com/kubernetes-csi/csi-test",
          "$vm_path/go/src/github.com/kubernetes-csi",
          true,
          true,
          true
        )

        sh (
          label: "Build CSI sanity",
          script: "ssh $ssh_options root@$ip_address " +
            "'cd $vm_path/go/src/github.com/kubernetes-csi/csi-test/cmd/csi-sanity;" +
            "export GOPATH=$vm_path/go;" +
            "make dist'"
        )

        _scp(
          ssh_options,
          'root',
          ip_address,
          "tools/csi-sanity/certs",
          "/",
          true,
          true,
          true
        )

        _scp(
          ssh_options,
          'root',
          ip_address,
          "trident-installer-*.tar.gz",
          "$vm_path/trident-installer.tar.gz",
          true,
          false,
          true
        )

        _scp(
          ssh_options,
          'root',
          ip_address,
          "test",
          vm_path,
          true,
          true,
          true
        )

        // Perform the required test setup
        echo "CSI sanity configuration"
        def id = rs
        backends_configured = _csi_sanity_config(
          name,
          ssh_options,
          id,
          vm_path,
          ip_address,
          backend,
          spec)

        // Run the whelk tests
        _run_csi_sanity_tests(name, ssh_options, ip_address, vm_path, spec, id)

        // Set the status to success
        status = 'success'

        // Write the status to a file
        _write_status_file(name, status)

      } catch(Exception e) {

        // Set the status to error
        status = 'error'

        // If there is a junit file and contains failures mark the
        // status as failure
         if (_test_failures("$name/csi_sanity.xml")) {
          status = 'failure'
        }

        // Update the status file to fail
        _write_status_file(name, status)

        // Update the message var
        message = e.getMessage()

        // Kill the run of STOP_ON_TEST_ERRORS is true
        if (env.STOP_ON_TEST_ERRORS == 'true') {
          error "$name: $message"
        } else {
          echo "$name: $message"
        }

      } finally {

        // Create the cleanup script in case we need it
        _create_cleanup_script(spec, name, purpose, ip_address, vm_path, backends_configured)

        if(hold_jig == 'always' || (status == 'success' && hold_jig == 'onfailure')) {
          // If HOLD_JIG is always or onfailure print the hold jig message
          _print_hold_jig_message()

        } else {
          // Cleanup the test backends
          if (backends_configured) {
          echo "Cleaning up the test backends"
            _cleanup_backends(ssh_options, ip_address, name, "$name/backends", vm_path, 'backends')
          }

          if (spec['vm_provider'] == 'SCS') {
            _scs_delete(purpose)
          } else if (spec['vm_provider'] == 'AWS') {
            _aws_delete(spec, name)
          }
        }

        // Notify Github of our status
        _notify_github(spec, commit, name, 'Stage has completed', status)

        // Archive everything in the stage workdir
        _archive_artifacts(name)

      }
    }
  }
}

def _unit_test(String name, String ssh_options, Map spec) {
  return {
    stage(name) {

      // Create var for HOLD_JIG
      def hold_jig = spec['hold_jig']

      // Define a unqiue random string
      def rs = _random_string(15)

      // Define a unique SCS purpose
      def purpose = rs

      // Define vars for the VM attributes
      def hostname = ''
      def domain = ''
      def ip_address = ''
      def scs_password = ''

      // Define a var for the path on $ip_address
      def vm_path = ''

      // Define a var to track the stage status
      def status = 'pending'

      // Define a var to track the result of getMessage()
      def message = ''

      // Initialize the result file for this stage
      _initialize_status_file(name, status)

      try {

        // Create a working dir for this spec
        sh (
          label: "Create directory $env.WORKSPACE/$name",
          script: "mkdir -p $name"
        )
        sh (label: "Sleep", script: "sleep 1")

        // Notify Github of our status
        _notify_github(spec, commit, name, 'Stage is running', status)

        // Create SCS VM(s) and create attribe variables we can use later
        def request = spec['request']
        def scs = _scs_create(name, request, purpose)
        hostname = scs[0]
        domain = scs[1]
        ip_address = scs[2]
        scs_password = scs[3]

        // Run the ansible playbook
        def target = hostname + '[' + ip_address + ']'
        def options = ['--ssh-public-key', "$env.WORKSPACE/tools/$env.SSH_PUBLIC_KEY_PATH"]
        if (spec['vm_provider'] == 'SCS') {
            options.push('--ssh-password')
            options.push(scs_password)
        } else if (spec['vm_provider'] == 'AWS') {
            options.push('--ssh-private-key')
            options.push("$env.WORKSPACE/tools/$env.SSH_PRIVATE_KEY_PATH")
        }
        options.push('--extra-vars')
        options.push(
          '\'context=' + spec['context'] + ' ' +
          'docker_ce_version=' + spec['docker_ce_version'] + ' ' +
          'go_download_url=' + spec['go_download_url'] + ' ' +
          'package_repository=' + env.PACKAGE_REPOSITORY + '\''
        )
        _run_playbook(name, spec['post_deploy_playbook'], target, options)

        // Create a var for the GOPATH on $ip_address
        vm_path = "/tmp/" + _replace(env.BUILD_TAG, '/', '-')

        sh (
          label: "Create directory go/src/github.com/netapp on $ip_address",
          script: "ssh $ssh_options root@$ip_address mkdir -p $vm_path/go/src/github.com/netapp"
        )

        sh (
          label: "Create directory go/pkg on $ip_address",
          script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/go/pkg'"
        )

        sh (
          label: "Create directory go/bin on $ip_address",
          script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/go/bin'"
        )

        sh (label: "Sleep", script: "sleep 1")

        sh (
          label: "Get goimports",
          script: "ssh $ssh_options root@$ip_address " +
            "'export GOPATH=$vm_path/go;" +
            "go get golang.org/x/tools/cmd/goimports'"
        )

        _scp(
          ssh_options,
          'root',
          ip_address,
          "src",
          "$vm_path/go",
          true,
          true,
          true
        )

        echo "Creating syntax checker script lint.sh"
        writeFile file: "$name/lint.sh", text: '''
files=`$GOPATH/bin/goimports -l ./*.go | sed 's/\\.\\.\\.//g')`
if [ -n "${files}" ]; then
  echo "Format errors detected in the following file(s):"
  echo "${files}"
  echo
  $GOPATH/bin/goimports -d ./*.go | sed 's/\\.\\.\\.//g')
  exit 1
fi
        '''
        sh (label: "Sleep", script: "sleep 1")

        _scp(
          ssh_options,
          'root',
          ip_address,
          "$name/lint.sh",
          "$vm_path/go/src/github.com/netapp/trident",
          true,
          false,
          true
        )

        // Run the syntax checker script and direct the output to lint.log
        try {
          sh (
            label: "Run lint.sh and direct the output to lint.log",
            script: "ssh $ssh_options root@$ip_address " +
              "'cd $vm_path/go/src/github.com/netapp/trident;" +
              "export GOPATH=$vm_path/go;" +
              "sh lint.sh > lint.log 2>&1'")
        } catch(Exception e) {

          // Fail the run
          // error e.getMessage()
          echo "Syntax failure"

        } finally {
          _scp(
            ssh_options,
            'root',
            ip_address,
            "$vm_path/go/src/github.com/netapp/trident/lint.log",
            name,
            false,
            false,
            false
          )
        }

        // Run the unit tests on $ip_address and direct the output to unit_tests.log
        try {
          sh (
            label: "Execute the unit tests on $ip_address and direct the output to unit_tests.log",
            script: "ssh $ssh_options root@$ip_address " +
              "'cd $vm_path/go/src/github.com/netapp/trident;" +
              "export GOPATH=$vm_path/go;" +
              "set -o pipefail;" +
              "make test | tee unit_tests.log'"
          )

        } catch(Exception e) {

          // Fail the run
          error "Unit test failure"

        } finally {
          _scp(
            ssh_options,
            'root',
            ip_address,
            "$vm_path/go/src/github.com/netapp/trident/unit_tests.log",
            name,
            false,
            false,
            false
          )
        }

        // Set the status to success
        status = 'success'

        // Write the status to a file
        _write_status_file(name, status)

      } catch(Exception e) {

        // Set the status to error
        status = 'error'

        // Update the status file
        _write_status_file(name, status)

        // Update the message var
        message = e.getMessage()

        // Kill the run of STOP_ON_TEST_ERRORS is true
        if (env.STOP_ON_TEST_ERRORS == 'true') {
          error "$name: $message"
        } else {
          echo "$name: $message"
        }


      } finally {

        // Create the cleanup script in case we need it
        _create_cleanup_script(spec, name, purpose, ip_address, vm_path, false)

        if(hold_jig == 'always' || (status != 'success' && hold_jig == 'onfailure')) {
          // If HOLD_JIG is always or onfailure print the hold jig message
          _print_hold_jig_message()

        } else {
          if (spec['vm_provider'] == 'SCS') {
            _scs_delete(purpose)
          } else if (spec['vm_provider'] == 'AWS') {
            _aws_delete(spec, name)
          }
        }

        // Notify Github of our status
        _notify_github(spec, commit, name, 'Stage has completed', status)

        // Archive everything in the stage workdir
        _archive_artifacts(name)

      }
    }
  }
}

def _whelk_test(String name, String ssh_options, Map spec) {
  return {
    stage(name) {

      // Create var for HOLD_JIG
      def hold_jig = spec['hold_jig']

      // Define backend var
      def backend = spec['backend']

      // Define a unqiue random string
      def rs = _random_string(15)

      // Define a unique SCS purpose
      def purpose = rs

      // Define vars for the VM attributes
      def hostname = ''
      def domain = ''
      def ip_address = ''
      def scs_password = ''

      // Define a var for the path on $ip_address
      def vm_path = ''

      // Define a flag var to signal if setup has succeeded
      def backends_configured = false

      // Define a var to hold the test type
      def test = spec['test']

      // Define a var to track the stage status
      def status = 'pending'

      // Define a var to track the result of getMessage()
      def message = ''

      // Initialize the result file for this stage
      _initialize_status_file(name, status)

      try {

        sh (
          label: "Create directory $env.WORKSPACE/junit",
          script: "mkdir -p junit"
        )

        sh (
          label: "Create directory $env.WORKSPACE/$name",
          script: "mkdir -p $name"
        )

        sh (label: "Sleep", script: "sleep 1")

        // Notify Github of our status
        _notify_github(spec, commit, name, 'Stage is running', status)

        if (spec['vm_provider'] == 'SCS') {

          // Create SCS VM(s)
          def request = spec['request']
          def scs = _scs_create(name, request, purpose)

          // Create attribe variables we can use later
          hostname = scs[0]
          domain = scs[1]
          ip_address = scs[2]
          scs_password = scs[3]

        } else if (spec['vm_provider'] == 'AWS') {

          // Create AWS VM(s)
          def aws = _aws_create(spec, name)

          // Create attribe variables we can use later
          echo "Getting hostname from fqdn"
          hostname = _get_hostname_from_fqdn(aws[0])
          echo "Getting domain from fqdn"
          domain = _get_domain_from_fqdn(aws[0])
          ip_address = aws[1]

        }

        // Run the ansible playbook
        def target = hostname + '[' + ip_address + ']'
        def options = ['--ssh-public-key', "$env.WORKSPACE/tools/$env.SSH_PUBLIC_KEY_PATH"]
        if (spec['vm_provider'] == 'SCS') {
          options.push('--ssh-password')
          options.push(scs_password)
        } else if (spec['vm_provider'] == 'AWS') {
          options.push('--ssh-private-key')
          options.push("$env.WORKSPACE/tools/$env.SSH_PRIVATE_KEY_PATH")
        }

        if (test == 'docker_ee')  {
          options.push('--extra-vars')
          options.push(
            '\'context=' + spec['context'] + ' ' +
            'docker_ee_address=' + ip_address + ' ' +
            'docker_ee_fqdn=' + hostname + '.' + domain  + ' ' +
            'docker_ee_hostname=' + hostname + ' ' +
            'docker_ee_image=' + spec['docker_ee_image'] + ' ' +
            'docker_ee_password=' + spec['docker_ee_password'] + ' ' +
            'docker_ee_repo=' + spec['docker_ee_repo'] + ' ' +
            'docker_ee_user=' + spec['docker_ee_user'] + ' ' +
            'docker_ee_url=' + spec['docker_ee_url'] + ' ' +
            'go_download_url=' + spec['go_download_url'] + ' ' +
            'k8s_version=' + spec['k8s_version'] + ' ' +
            'package_repository=' + env.PACKAGE_REPOSITORY + '\''
          )
        }

        if (test == 'kubeadm')  {
          options.push('--extra-vars')
          options.push(
            '\'context=' + spec['context'] + ' ' +
            'docker_ce_version=' + spec['docker_ce_version'] + ' ' +
            'go_download_url=' + spec['go_download_url'] + ' ' +
            'k8s_version=' + spec['k8s_version'] + ' ' +
            'k8s_api_version=' + spec['k8s_api_version'] + ' ' +
            'k8s_cni_version=' + spec['k8s_cni_version'] + ' ' +
            'kubeadm_version=' + spec['kubeadm_version'] + ' ' +
            'package_repository=' + env.PACKAGE_REPOSITORY + '\''
          )
        }

        if (test == 'ndvp_binary')  {
          options.push('--extra-vars')
          options.push(
            '\'context=' + spec['context'] + ' ' +
            'docker_ce_version=' + spec['docker_ce_version'] + ' ' +
            'go_download_url=' + spec['go_download_url'] + ' ' +
            'package_repository=' + env.PACKAGE_REPOSITORY + '\''
          )
        }

        if (test == 'ndvp_plugin')  {
          options.push('--extra-vars')
          options.push(
            '\'context=' + spec['context'] + ' ' +
            'docker_ce_version=' + spec['docker_ce_version'] + ' ' +
            'go_download_url=' + spec['go_download_url'] + ' ' +
            'package_repository=' + env.PACKAGE_REPOSITORY + '\''
          )
        }

        if (test == 'openshift')  {
          options.push('--extra-vars')
          options.push(
            '\'context=' + spec['context'] + ' ' +
            'docker_ce_version=' + spec['docker_ce_version'] + ' ' +
            'go_download_url=' + spec['go_download_url'] + ' ' +
            'openshift_url=' + spec['openshift_url'] + ' ' +
            'package_repository=' + env.PACKAGE_REPOSITORY + '\''
          )
        }

        if (test == 'upgrade')  {
          options.push('--extra-vars')
          options.push(
            '\'context=' + spec['context'] + ' ' +
            'docker_ce_version=' + spec['docker_ce_version'] + ' ' +
            'go_download_url=' + spec['go_download_url'] + ' ' +
            'k8s_version=' + spec['k8s_version'] + ' ' +
            'k8s_api_version=' + spec['k8s_api_version'] + ' ' +
            'k8s_cni_version=' + spec['k8s_cni_version'] + ' ' +
            'kubeadm_version=' + spec['kubeadm_version'] + ' ' +
            'package_repository=' + env.PACKAGE_REPOSITORY + '\''
          )
        }

        try {
          _run_playbook(name, spec['post_deploy_playbook'], target, options)
        } finally {
          if (test == 'docker_ee')  {
            _scp(
              ssh_options,
              'root',
              ip_address,
              "/root/ucp_install.log",
              name,
              false,
              false,
              false
            )
          }
        }

        // Create a var for the path on $ip_address
        vm_path = "/tmp/" + _replace(env.BUILD_TAG, '/', '-')

        sh (
          label: "Create directory $vm_path on $ip_address",
          script: "ssh $ssh_options root@$ip_address mkdir -p $vm_path"
        )

        sh (label: "Sleep", script: "sleep 1")

        _scp(
          ssh_options,
          'root',
          ip_address,
          "test",
          vm_path,
          true,
          true,
          true
        )

        // Perform the required test setup
        if (test == 'docker_ee')  {

          echo "Docker EE configuration"
          def id = rs
          backends_configured = _docker_ee_config(
            name,
            ssh_options,
            id,
            vm_path,
            ip_address,
            target,
            scs_password,
            spec)

        } else if (test == 'kubeadm')  {

          echo "KubeAdm configuration"
          def id = rs
          backends_configured = _kubeadm_config(
            name,
            ssh_options,
            id,
            vm_path,
            ip_address,
            target,
            scs_password,
            spec)

        } else if (test == 'ndvp_binary')  {

          echo "nDVP binary configuration"
          def id = rs
          backends_configured = _ndvp_binary_config(
            name,
            ssh_options,
            id,
            vm_path,
            ip_address,
            backend,
            spec)

        } else if (test == 'ndvp_plugin')  {

          echo "nDVP plugin configuration"
          def id = rs
          backends_configured = _ndvp_plugin_config(
            name,
            ssh_options,
            id,
            vm_path,
            ip_address,
            backend,
            spec)

        } else if (test == 'openshift')  {

          echo "Openshift configuration"

          def id = rs
          backends_configured = _openshift_config(
          name,
          ssh_options,
          id,
          vm_path,
          ip_address,
          target,
          scs_password,
          spec)

        } else if (test == 'upgrade')  {

          echo "Upgrade configuration"

          def id = rs
          backends_configured = _upgrade_config(
          name,
          ssh_options,
          id,
          vm_path,
          ip_address,
          target,
          scs_password,
          spec)

        }

        // Run the whelk tests
        _run_whelk_tests(name, ssh_options, ip_address, vm_path, spec)

        // Set the status to success
        status = 'success'

        // Write the status to a file
        _write_status_file(name, status)

      } catch(Exception e) {

        // Set the status to error
        status = 'error'

        // If there is a junit file and it contains failures mark
        // the status as failure
         if (_test_failures("$name/nosetests.xml")) {
          status = 'failure'
        }

        // Update the status file
        _write_status_file(name, status)

        // Update the message var
        message = e.getMessage()

        // Kill the run of STOP_ON_TEST_ERRORS is true
        if (env.STOP_ON_TEST_ERRORS == 'true') {
          error "$name: $message"
        } else {
          echo "$name: $message"
        }

      } finally {

        // Create the cleanup script in case we need it
        _create_cleanup_script(spec, name, purpose, ip_address, vm_path, backends_configured)

        if(hold_jig == 'always' || (status != 'success' && hold_jig == 'onfailure')) {
          // If HOLD_JIG is always or onfailure print the hold jig message
          _print_hold_jig_message()

        } else {
          // Cleanup the test backends if they have been configured
          if (backends_configured) {
            echo "Cleaning up the test backends"
            _cleanup_backends(ssh_options, ip_address, name, "$name/backends", vm_path, 'backends')

            if (spec['install_backend']) {
              echo "Install backend detected"
              // Figure out if the install backend is the same as the test backend
              def install_backend = spec['install_backend']
              def install_backend_matches = false
              for(be in backend.split(',')) {
                echo "Checking if $install_backend == $be"
                if (install_backend == be) {
                  install_backend_matches = true
                  echo "Install backend $install_backend matches test backend $be"
                  break
                }
              }

              // If the install backend does not match any test backend, clean it up
              if (install_backend_matches == false) {
                echo "Cleaning up seperate $install_backend"
                _cleanup_backends(ssh_options, ip_address, name, "$name/install_backend", vm_path, 'install_backend')
              }
            }
          }

          if (spec['vm_provider'] == 'SCS') {
            _scs_delete(purpose)
          } else if (spec['vm_provider'] == 'AWS') {
            _aws_delete(spec, name)
          }
        }

        // Notify Github of our status
        _notify_github(spec, commit, name, 'Stage has completed', status)

        // Archive everything in the stage workdir
        _archive_artifacts(name)

      }
    }
  }
}

def _archive_artifacts(String name) {

        sh (
          label: "Remove ansible files",
          script: "rm -rf $name/ansible"
        )

        sh (label: "Sleep", script: "sleep 1")

        sh (
          label: "Create ${name}.tgz",
          script: "tar -cvzf ${name}.tgz $name"
        )

        archiveArtifacts artifacts: "${name}.tgz"
}

def _aws_create(Map spec, String name){

  def aws = []
  try {

    sh (
      label: "Create AWS instances",
      script: "export AWS_ACCESS_KEY_ID=" + spec['aws_access_key_id'] + ";" +
        "export AWS_SECRET_ACCESS_KEY=" + spec['aws_secret_access_key'] + ";" +
        "export AWS_DEFAULT_REGION=" + spec['aws_default_region'] + ";" +
        "python3 tools/utils/aws.py " +
        "--action create " +
        "--ami " + spec['ami'] + " " +
        "--config-file $name/aws_config.json " +
        "--count 1 " +
        "--key-name " + spec['key_name'] + " " +
        "--subnet " + spec['subnet'] + " " +
        "--security " + spec['security_group'] + " " +
        "--type " + spec['instance_type']
    )

    def cmd = ("export AWS_ACCESS_KEY_ID=" + spec['aws_access_key_id'] + ";" +
        "export AWS_SECRET_ACCESS_KEY=" + spec['aws_secret_access_key'] + ";" +
        "export AWS_DEFAULT_REGION=" + spec['aws_default_region'] + ";" +
        "python3 tools/utils/aws.py " +
        "--action get " +
        "--config-file $name/aws_config.json " +
        "--get-attributes PublicDnsName,PublicIpAddress")

    def output = sh(label: "Get AWS instance information", returnStdout: true, script: cmd).trim()

    echo output

    aws = output.split(';')
    echo "returning from _aws_create"
    return aws

  } catch(Exception e) {
    // Fail the run
    error 'Failed to create AWS instance(s)'
  }
}

def _aws_delete(Map spec, String name){

  try {

    sh (
      label: "Delete AWS instances",
      script: "export AWS_ACCESS_KEY_ID=" + spec['aws_access_key_id'] + ";" +
        "export AWS_SECRET_ACCESS_KEY=" + spec['aws_secret_access_key'] + ";" +
        "export AWS_DEFAULT_REGION=" + spec['aws_default_region'] + ";" +
        "python3 tools/utils/aws.py " +
        "--action delete " +
        "--config-file $name/aws_config.json"
    )

  } catch(Exception e) {
    // Fail the run
    error 'Failed to delete AWS instance(s)'
  }
}

def _cleanup_backends(String ssh_options, String ip_address, String name, String src_path, String vm_path, String dst_path) {

  echo "Cleaning up backend storage using backend files in $src_path"

  sh (
    label: "Create directory $dst_path path on $ip_address",
    script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/$dst_path'"
  )

  sh (label: "Sleep", script: "sleep 1")

  _scp(
    ssh_options,
    'root',
    ip_address,
    "$src_path/*",
    "$vm_path/$dst_path",
    true,
    false,
    true
  )

  sh (
    label: "Show destination path $src_path on $ip_address",
    script: "ssh $ssh_options root@$ip_address 'ls -l $vm_path/$dst_path'"
  )

  sh (
    label: "Change permissions for cleanup_backend.py on $ip_address",
    script: "ssh $ssh_options root@$ip_address 'chmod +x $vm_path/test/whelk/ci/cleanup_backend.py'"
  )

  // Run cleanup_backend.py
  try {
    sh (
      label: "Execute cleanup_backend.py on $ip_address",
      script: "ssh $ssh_options root@$ip_address '" +
        "export NDVP_CONFIG=$vm_path/$dst_path;" +
        "export PYTHONPATH=$vm_path/test;" +
        "export SF_ADMIN_PASSWORD=$env.SOLIDFIRE_ADMIN_PASSWORD;" +
        "cd $vm_path/test; " +
        "python whelk/ci/cleanup_backend.py'"
    )
  } catch(Exception e) {
    echo "Error cleaning up backends: " + e.getMessage()
  }

}

def _clone_from_github(url, credentials, branch, destination) {

  checkout(
    [$class: 'GitSCM', branches: [[name: "$branch"]],
    doGenerateSubmoduleConfigurations: false,
    extensions: [[$class: 'RelativeTargetDirectory', relativeTargetDir: "$destination"]],
    submoduleCfg: [],
    userRemoteConfigs: [[credentialsId: "$credentials", url: "$url"]]])
}

def _clone() {

  // Some pipelines populate BRANCH_TYPE
  def branch_type = '*'
  if (env.BRANCH_TYPE) {
    branch_type = env.BRANCH_TYPE
  }

  // All pipelines populate BRANCH_NAME
  def branch_name = env.BRANCH_NAME
  if (env.BRANCH_NAME.contains('/')) {
    (branch_type, branch_name) = env.BRANCH_NAME.split('/')
  }

  branch = "$branch_type/$branch_name"
  if (branch_type == '*') {
    branch = branch_name
  }

  def private_org = env.TRIDENT_PRIVATE_GITHUB_ORG
  def private_repo = env.TRIDENT_PRIVATE_GITHUB_REPO
  def user = env.GITHUB_USERNAME
  def token = env.GITHUB_TOKEN

  echo "Clone trident:$branch"

  // Create the directory for the trident source since it's unclear hot to
  // call checkout scm with a destination dir
  def jenkins_trident_src = 'src/github.com/netapp/trident'
  sh (
    label: "Create directory $jenkins_trident_src",
    script: "mkdir -p $jenkins_trident_src"
  )
  sh (label: "Sleep", script: "sleep 1")

  // Change to the directory we just created and call checkout scm
  // We need to use checkout scm becasue it handles PRs
  dir(jenkins_trident_src) {
    def co = checkout scm
    commit = co.GIT_COMMIT
  }

  def full_trident_src = 'src2/github.com/netapp/trident'
  sh (
    label: "Full clone master for change processing",
    script: "git clone https://$user:$token@github.com/$private_org/$private_repo $full_trident_src"
  )

  if (env.BRANCH_NAME && env.BRANCH_NAME.contains('PR-')) {
    def cmd = "curl -H \"Authorization: token $env.GITHUB_TOKEN\" " +
      "https://api.github.com/repos/$env.TRIDENT_PRIVATE_GITHUB_ORG/" +
      "$env.TRIDENT_PRIVATE_GITHUB_REPO/pulls/$env.CHANGE_ID"
    def response = sh(
      label: "Query Github for $env.BRANCH_NAME",
      returnStdout: true,
      script: cmd
    ).trim()

    def pr = readJSON text: response

    if (pr.containsKey('head') == false) {
      error "Error getting the PR info from Github for $env.BRANCH_NAME"
    }

    branch = pr['head']['ref']

    echo "The branch name for $env.BRANCH_NAME is $branch"

    commit = pr['head']['sha']

  }

  echo "The commit hash for $env.BRANCH_NAME is $commit"

  def fallback_branch = "master"
  if (env.BRANCH_NAME && env.BRANCH_NAME.contains('PR-')) {
    fallback_branch = env.CHANGE_TARGET
  }

  // Try to checkout the corresponding tools branch and if that fails fall
  // back to use the master branch
  def tools_src = 'tools'
  echo "Clone tools:$branch"
  try {
    _clone_from_github(
      env.TOOLS_PRIVATE_GITHUB_URL,
      env.GITHUB_CREDENTIAL_ID,
      branch,
      'tools')
  } catch(Exception e1) {
    echo "Error cloning tools $branch: " + e1.getMessage()
    echo "Clone tools:$fallback_branch"
    try {
      sh (label: "Remove tools directory", script: "rm -rf $tools_src")
      sh (label: "Sleep", script: "sleep 1")
       _clone_from_github(
         env.TOOLS_PRIVATE_GITHUB_URL,
         env.GITHUB_CREDENTIAL_ID,
         fallback_branch,
         'tools')
    } catch(Exception e2) {
      echo "Error cloning $fallback_branch: " + e2.getMessage()
      if (fallback_branch != 'master') {
        echo "Clone tools:master"
        try {
          sh (label: "Remove tools directory", script: "rm -rf $tools_src")
          sh (label: "Sleep", script: "sleep 1")
           _clone_from_github(
             env.TOOLS_PRIVATE_GITHUB_URL,
             env.GITHUB_CREDENTIAL_ID,
             'master',
             'tools')
        } catch(Exception e3) {
          error "Error cloning tools:master: " + e3.getMessage()
        }
      } else {
        error "Error cloning tools: " + e2.getMessage()
      }
    }
  }

  sh (
    label: "Change permissions on private key",
    script: "chmod 700 tools/" + env.SSH_PRIVATE_KEY_PATH
  )

  if (env.BLACK_DUCK_SCAN == null && env.DEPLOY_TRIDENT == null) {

    // Try to checkout the corresponding whelk branch and if that failed fall
    // back to use the master branch
    def whelk_src = 'test'
    echo "Clone whelk:$branch"
    try {
      _clone_from_github(
        env.WHELK_PRIVATE_GITHUB_URL,
        env.GITHUB_CREDENTIAL_ID,
        branch,
        whelk_src)
    } catch(Exception e1) {
      echo "Error cloning whelk:$branch:" + e1.getMessage()
      echo "Clone whelk:$fallback_branch"
      try {
         sh (label: "Remove test directory", script: "rm -rf $whelk_src")
         sh (label: "Sleep", script: "sleep 1")
         _clone_from_github(
           env.WHELK_PRIVATE_GITHUB_URL,
           env.GITHUB_CREDENTIAL_ID,
           fallback_branch,
           'test')
      } catch(Exception e2) {
        echo "Error cloning whelk:$fallback_branch:" + e2.getMessage()
        if (fallback_branch != 'master') {
          echo "Clone whelk:master"
          try {
            sh (label: "Remove test directory", script: "rm -rf $whelk_src")
            sh (label: "Sleep", script: "sleep 1")
            _clone_from_github(
              env.WHELK_PRIVATE_GITHUB_URL,
              env.GITHUB_CREDENTIAL_ID,
              'master',
              'test')
          } catch(Exception e3) {
            error "Error cloning whelk: " + e3.getMessages()
          }
        } else {
          error "Error cloning whelk: " + e2.getMessages()
        }
      }
    }

    // Clone the csi-sanity source
    // https://github.com/kubernetes-csi/csi-test/releases
    echo "Clone csi-sanity"
    _clone_from_github(
      'https://github.com/kubernetes-csi/csi-test.git',
      env.GITHUB_CREDENTIAL_ID,
      'v2.3.0',
      'src/github.com/kubernetes-csi/csi-test')
  }

}

def _configure_backend_files(String ssh_options, String ip_address, String id, String src_path, String vm_path, String dst_path, String backends, Map spec) {

  echo "Configuring backend files using the id $id"

  sh (
    label: "Create directory $src_path",
    script: "mkdir -p $src_path"
  )

  sh (label: "Sleep", script: "sleep 1")

  // Create the backend file
  backend_index = 0
  for(backend in backends.split(',')) {
    backend_index++
    backend_id = id + backend_index
    backend_file = "$src_path/" + backend + '.json'
    _create_backend_file(backend_file, backend_id, backend, spec)
  }
  sh (label: "Sleep", script: "sleep 1")

  sh (
    label: "Create directory $dst_path on $ip_address",
    script: "ssh $ssh_options root@$ip_address 'mkdir -p $vm_path/$dst_path'"
  )

  sh (label: "Sleep", script: "sleep 1")

  _scp(
    ssh_options,
    'root',
    ip_address,
    "$src_path/*",
    "$vm_path/$dst_path",
    true,
    false,
    true
  )

  sh (
    label: "Show $dst_path on $ip_address",
    script: "ssh $ssh_options root@$ip_address 'ls -l $vm_path/$dst_path'"
  )

  return true

}

def _create_backend_file(String path, String id, String backend, Map spec) {

  def content = ''
  if (backend == 'aws-cvs') {
    content = ('\n' +
    '{\n' +
    '  "version": 1,\n' +
    '  "storageDriverName": "aws-cvs",\n' +
    '  "cvsVolumeName": "' + id + '",\n' +
    '  "apiRegion": "' + spec[backend]['region'] + '",\n' +
    '  "apiURL": "' + spec[backend]['api_url'] + '",\n' +
    '  "apiKey": "' + spec[backend]['api_key'] + '",\n' +
    '  "secretKey": "' + spec[backend]['api_secret_key'] + '",\n' +
    '  "debugTraceFlags": {"method": true, "api": true}\n' +
    '}')
  }

  if (backend == 'aws-cvs-virtual-pools') {
    content = ('\n' +
    '{\n' +
    '  "version": 1,\n' +
    '  "storageDriverName": "aws-cvs",\n' +
    '  "cvsVolumeName": "' + id + '",\n' +
    '  "apiRegion": "' + spec[backend]['region'] + '",\n' +
    '  "apiURL": "' + spec[backend]['api_url'] + '",\n' +
    '  "apiKey": "' + spec[backend]['api_key'] + '",\n' +
    '  "secretKey": "' + spec[backend]['api_secret_key'] + '",\n' +
    '  "debugTraceFlags": {"method": true, "api": true},\n' +
    '  \n' +
    '  "defaults": {\n' +
    '      "snapshotReserve": "10",\n' +
    '      "exportRule": "0.0.0.0/0,10.0.0.0/24",\n' +
    '      "size": "10Gi"\n' +
    '  },\n' +
    ' \n' +
    '  "labels": {"cloud": "aws"},\n' +
    '  "serviceLevel": "standard",\n' +
    '  "region": "us-east",\n' +
    '  "zone": "us-east-1a",\n' +
    ' \n' +
    '  "storage": [\n' +
    '      {\n' +
    '          "labels": {"performance": "gold", "cost": "3"},\n' +
    '          "serviceLevel": "extreme",\n' +
    '          "defaults": {\n' +
    '              "snapshotReserve": "5",\n' +
    '              "exportRule": "0.0.0.0/0",\n' +
    '              "size": "2Gi"\n' +
    '           }\n' +
    '      },\n' +
    '      {\n' +
    '          "labels": {"performance": "silver", "cost": "2"},\n' +
    '          "serviceLevel": "premium"\n' +
    '      },\n' +
    '      {\n' +
    '          "labels": {"performance": "bronze", "cost": "1"},\n' +
    '          "serviceLevel": "standard"\n' +
    '      }\n' +
    '  ]\n' +
    '}')
  }
  if (backend == 'solidfire-san') {
    content = ('\n' +
    '{\n' +
    '  "version": 1,\n' +
    '  "storageDriverName": "solidfire-san",\n' +
    '  "Endpoint": "https://' + spec[backend]['username'] + ':' + spec[backend]['password'] + '@' + spec[backend]['mvip'] + '/json-rpc/7.0",\n' +
    '  "SVIP": "' + spec[backend]['svip'] + ':3260",\n' +
    '  "TenantName": "' + id + '",\n' +
    '  "InitiatorIFace": "default",\n' +
    '  "DefaultVolSz": 1,\n' +
    '  "Types": [{"Type": "Bronze", "Qos": {"minIOPS": 1000, "maxIOPS": 2000, "burstIOPS": 4000}},\n' +
    '            {"Type": "Silver", "Qos": {"minIOPS": 4000, "maxIOPS": 6000, "burstIOPS": 8000}},\n' +
    '            {"Type": "Gold", "Qos": {"minIOPS": 6000, "maxIOPS": 8000, "burstIOPS": 10000}}],\n' +
    '  "debugTraceFlags": {"method": true, "api": true, "sensitive": true}\n' +
    '}')
  }

  if (backend == 'ontap-san-economy') {
    content = ('\n' +
    '{\n' +
    '  "version": 1,\n' +
    '  "storageDriverName": "ontap-san-economy",\n' +
    '  "managementLIF": "' + spec[backend]['management_lif'] + '",\n' +
    '  "svm": "' + id + '",\n' +
    '  "username": "' + spec[backend]['username'] + '",\n' +
    '  "password": "' + spec[backend]['password'] + '",\n' +
    '  "storagePrefix": "' + id + '_",\n' +
    '  "debugTraceFlags": {"method": true, "api": true}\n' +
    '}')
  }

  if (backend == 'ontap-san') {
    content = ('\n' +
    '{\n' +
    '  "version": 1,\n' +
    '  "storageDriverName": "ontap-san",\n' +
    '  "managementLIF": "' + spec[backend]['management_lif'] + '",\n' +
    '  "svm": "' + id + '",\n' +
    '  "username": "' + spec[backend]['username'] + '",\n' +
    '  "password": "' + spec[backend]['password'] + '",\n' +
    '  "storagePrefix": "' + id + '_",\n' +
    '  "debugTraceFlags": {"method": true, "api": true}\n' +
    '}')
  }

  if (backend == 'ontap-nas') {
    content = ('\n' +
    '{\n' +
      '"version": 1,\n' +
    '  "storageDriverName": "ontap-nas",\n' +
    '  "managementLIF": "' + spec[backend]['management_lif'] + '",\n' +
    '  "svm": "' + id + '",\n' +
    '  "username": "' + spec[backend]['username'] + '",\n' +
    '  "password": "' + spec[backend]['password'] + '",\n' +
    '  "storagePrefix": "' + id + '_",\n' +
    '  "debugTraceFlags": {"method": true, "api": true}\n' +
    '}')
  }

  if (backend == 'ontap-nas-economy') {
    content = ('\n' +
    '{\n' +
    '  "version": 1,\n' +
    '  "storageDriverName": "ontap-nas-economy",\n' +
    '  "managementLIF": "' + spec[backend]['management_lif'] + '",\n' +
    '  "svm": "' + id + '",\n' +
    '  "username": "' + spec[backend]['username'] + '",\n' +
    '  "password": "' + spec[backend]['password'] + '",\n' +
    '  "storagePrefix": "' + id + '_",\n' +
    '  "nfsMountOptions": "-o nfsvers=4",\n' +
    '  "defaults": {\n' +
    '        "snapshotDir": "true"\n' +
    '      },\n' +
    '  "debugTraceFlags": {"method": true, "api": true}\n' +
    '}')
  }

  if (backend == 'ontap-nas-flexgroup') {
    content = ('\n' +
    '{\n' +
    '  "version": 1,\n' +
    '  "storageDriverName": "ontap-nas-flexgroup",\n' +
    '  "managementLIF": "' + spec[backend]['management_lif'] + '",\n' +
    '  "svm": "' + id + '",\n' +
    '  "username": "' + spec[backend]['username'] + '",\n' +
    '  "password": "' + spec[backend]['password'] + '",\n' +
    '  "storagePrefix": "' + id + '_",\n' +
    '  "debugTraceFlags": {"method": true, "api": true}\n' +
    '}\n')
  }
  if (backend == 'eseries-iscsi') {
    content = ('\n' +
    '{\n' +
    '  "version": 1,\n' +
    '  "storageDriverName": "eseries-iscsi",\n' +
    '  "webProxyHostname": "' + spec[backend]['web_proxy_hostname'] + '",\n' +
    '  "webProxyPort": "8080",\n' +
    '  "webProxyUseHTTP": true,\n' +
    '  "webProxyVerifyTLS": false,\n' +
    '  "username": "' + spec[backend]['username'] + '",\n' +
    '  "password": "' + spec[backend]['password'] + '",\n' +
    '  "controllerA": "' + spec[backend]['controllera'] + '",\n' +
    '  "controllerB": "' + spec[backend]['controllerb'] + '",\n' +
    '  "passwordArray": "' + spec[backend]['array_password'] + '",\n' +
    '  "hostData_IP": "' + spec[backend]['host_data_ip'] + '",\n' +
    '  "storagePrefix": "' + id + '_",\n' +
    '  "debugTraceFlags": {"method": true, "api": true}\n' +
    '}\n')
  }
  writeFile file: path, text: content
}

def _create_cleanup_script(Map spec, String name, String purpose, String ip_address, String vm_path, Boolean backends_configured) {

   echo "Creating stage cleanup script for $name"
   sh (
     label: "Create directory $env.WORKSPACE/cleanup",
     script: "mkdir -p $env.WORKSPACE/cleanup"
   )

   sh (label: "Sleep", script: "sleep 1")

   // Create the cleanup script contents
   def content = (
     "ssh_options=\"-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PasswordAuthentication=no -q -i \$1\"\n" +
     "vm_path=\"$vm_path\"\n"
   )

   if (backends_configured) {
       content += (
         "ssh \$ssh_options root@$ip_address " +
         "\"export NDVP_CONFIG=\$vm_path/backends;" +
         "export SF_ADMIN_PASSWORD=$env.SOLIDFIRE_ADMIN_PASSWORD;" +
         "export PYTHONPATH=\$vm_path/test;cd \$vm_path/test;" +
         "python whelk/ci/cleanup_backend.py\"\n"
       )

       if (spec['install_backend']) {
         content += (
           "ssh \$ssh_options root@$ip_address " +
           "\"export NDVP_CONFIG=\$vm_path/install_backend;" +
           "export SF_ADMIN_PASSWORD=$env.SOLIDFIRE_ADMIN_PASSWORD;" +
           "export PYTHONPATH=\$vm_path/test;" +
           "cd \$vm_path/test;python whelk/ci/cleanup_backend.py\"\n"
         )
       }
   }

   if (spec['vm_provider'] == 'SCS') {
       content += (
         "python3 scs.py --action delete-vms --endpoint $env.SCS_ENDPOINT " +
         "--token $env.SCS_TOKEN --user $env.SCS_USERNAME --purpose $purpose\n"
       )
   } else if (spec['vm_provider'] == 'AWS') {
     content += (
       "export AWS_ACCESS_KEY_ID=" + spec['aws_access_key_id'] + ";" +
       "export AWS_SECRET_ACCESS_KEY=" + spec['aws_secret_access_key'] + ";" +
       "export AWS_DEFAULT_REGION=" + spec['aws_default_region'] + ";" +
       "python3 aws.py " +
       "--action delete " +
       "--config-file ${name}_aws_config.json --wait 0\n"
     )

     sh (
       label: "Copy aws_config.json to cleanup",
       script: "cp $name/aws_config.json $env.WORKSPACE/cleanup/${name}_aws_config.json"
     )
   }

   // Write to the file
   writeFile file: "$env.WORKSPACE/cleanup/${name}_cleanup.sh", text: content
   sh (label: "Sleep", script: "sleep 1")

}

def _csi_sanity_config(
  String name,
  String ssh_options,
  String id,
  String vm_path,
  String ip_address,
  String backend,
  Map spec) {

    // Configure the backend(s) with a unique object names and copy to the the VM
    def backends_configured = false
    _configure_backend_files(
      ssh_options,
      ip_address,
      "${id}t",
      "$name/backends",
      vm_path,
      "backends",
      backend,
      spec)

    backends_configured = _setup_backends(
      ssh_options,
      ip_address,
      name,
      vm_path,
      "backends")

    return backends_configured
}

def _decode(String string) {
  return URLDecoder.decode(string)
}

def _docker_ee_config(
  String name,
  String ssh_options,
  String id,
  String vm_path,
  String ip_address,
  String target,
  String scs_password,
  Map spec) {

    def image = sh(returnStdout: true, script: 'cat trident_docker_image.log').trim()

    if (spec['trident_image_distribution'] == 'load') {

      _scp(
        ssh_options,
        'root',
        ip_address,
        "trident_docker_image.tgz",
        vm_path,
        true,
        false,
        true
      )

      sh (
        label: "Load trident_docker_image.tgz on $ip_address",
        script: "ssh $ssh_options root@$ip_address docker load -i $vm_path/trident_docker_image.tgz"
      )

    }

    if (spec['trident_image_distribution'] == 'pull') {
      sh (
        label: "Pull $image",
        script: "ssh $ssh_options root@$ip_address docker pull $image"
      )
    }

    // Determine the trident installer name
    // trident-installer-19.04.0-test.trident-ci_master.tar.gz
    def cmd = 'ls trident-installer-*-test.' + _replace(env.BUILD_TAG, '/', '-') + '.tar.gz'
    def installer = sh(returnStdout: true, script: cmd).trim()

    // Configure the backend(s) with a unique object names and copy to the the VM
    def backend = spec['backend']
    def backends_configured = false
    _configure_backend_files(
      ssh_options,
      ip_address,
      "${id}t",
      "$name/backends",
      vm_path,
      "backends",
      backend,
      spec)

    backends_configured = _setup_backends(
      ssh_options,
      ip_address,
      name,
      vm_path,
      "backends")

    def install_backend = spec['install_backend']
    def install_backend_path = ''
    if (install_backend) {
      install_backend_matches = false
      for(be in backend.split(',')) {
        if (install_backend == be) {
          install_backend_matches = true
          echo "Install backend $install_backend matches test backend $be"
          break
        }
      }

      if (install_backend_matches == true) {
        // Use one of the backends we configured previously
        install_backend_path = "$name/backends/${install_backend}.json"
        echo 'Using ' + install_backend_path + ' for Trident installation'
      } else {
        install_backend_path = "$name/install_backend/${install_backend}.json"
        echo 'Using ' + install_backend_path + ' for Trident installation'
        // Create the backend file name that will be used to install trident
        _configure_backend_files(
          ssh_options,
          ip_address,
          "${id}i",
          "$name/install_backend",
          vm_path,
          "install_backend",
          install_backend,
          spec)

        backends_configured = _setup_backends(
          ssh_options,
          ip_address,
          name,
          vm_path,
          "install_backend")
      }
    }

    // Create the options that will be passed to ansible.py
    def options = []
    if (install_backend) {
      options = [
        '--extra-vars',
        '\'backend=' + env.WORKSPACE + '/' + install_backend_path +  ' ' +
        'docker_ee_address=' + ip_address + ' ' +
        'image=' + image + ' ' +
        'installer=' + env.WORKSPACE + '/' + installer + ' ' +
        'install_target=docker_ee ' +
        'k8s_version=' + spec['k8s_version'] + ' ' +
        'namespace=trident ' +
        'remote_path=' + vm_path + '\'']
    } else {
      options = [
        '--extra-vars',
        '\'docker_ee_address=' + ip_address + ' ' +
        'image=' + image + ' ' +
        'installer=' + env.WORKSPACE + '/' + installer + ' ' +
        'install_target=docker_ee ' +
        'k8s_version=' + spec['k8s_version'] + ' ' +
        'namespace=trident ' +
        'remote_path=' + vm_path + '\'']
    }

    if (spec['vm_provider'] == 'SCS') {
        options.push('--ssh-password')
        options.push(scs_password)
    } else if (spec['vm_provider'] == 'AWS') {
        options.push('--ssh-private-key')
        options.push("$env.WORKSPACE/tools/$env.SSH_PRIVATE_KEY_PATH")
    }

    // Run the trident installer playbook
    try {
      _run_playbook(name, spec['trident_install_playbook'], target, options)
    } catch(Exception e) {
      error e.getMessage()
    } finally {
      _scp(
        ssh_options,
        'root',
        ip_address,
        "$vm_path/trident_install.log",
        name,
        false,
        false,
        false
      )
    }

    return backends_configured
}

def _gather_triage_information(Map spec, String ssh_options, String ip_address, String name, String vm_path) {

  if (_whelk_test_match(spec, 'docker_ee|kubeadm|openshift|upgrade')) {

    try {
      sh (
        label: "Execute journalctl -u kube*",
        script: "ssh $ssh_options root@$ip_address 'journalctl -u kube* > $vm_path/kubernetes.log'"
      )
      sh (label: "Sleep", script: "sleep 1")
    } catch(Exception e) {
      echo "Error executing journalctl -u kube*"
    } finally {
      _scp(
        ssh_options,
        'root',
        ip_address,
        "$vm_path/kubernetes.log",
        name,
        false,
        false,
        false
      )
    }

    try {
      def cmd = "'cd $vm_path;tridentctl logs -a "
      if (_whelk_test_match(spec, 'docker_ee')) {
        cmd = (
          "'export DOCKER_TLS_VERIFY=1;" +
          "export COMPOSE_TLS_VERSION=TLSv1_2;" +
          "export DOCKER_CERT_PATH=/root;" +
          "export DOCKER_HOST=tcp://$ip_address:443;" +
          "export KUBECONFIG=/root/kube.yml;" +
          "cd $vm_path;tridentctl logs -a "
        )
      }
      if (_whelk_test_match(spec, 'docker_ee|kubeadm')) {
        cmd += "-n trident"
      } else if (_whelk_test_match(spec, 'openshift')) {
        cmd += "-n myproject"
      }
      cmd += "'"

      sh (
        label: "Execute tridentctl logs -a",
        script: "ssh $ssh_options root@$ip_address $cmd"
      )

      sh (label: "Sleep", script: "sleep 1")
    } catch(Exception e) {
      echo "Error executing tridentctl logs -a"
    } finally {
      _scp(
        ssh_options,
        'root',
        ip_address,
        "$vm_path/support*.zip",
        name,
        false,
        false,
        false
      )
    }
  }

  def os = sh (
    label: "Get the OS name from os-release",
    returnStdout: true,
    script: "ssh $ssh_options root@$ip_address 'grep \"^NAME=\" /etc/os-release'"
  ).trim()

  if (os.contains('Ubuntu')) {
    _scp(
      ssh_options,
      'root',
      ip_address,
      "/var/log/kern.log",
      name,
      false,
      false,
      false
    )
    _scp(
      ssh_options,
      'root',
      ip_address,
      "/var/log/syslog",
      name,
      false,
      false,
      false
    )
  } else if (os.contains('CentOS')) {
    _scp(
      ssh_options,
      'root',
      ip_address,
      "/var/log/dmesg",
      name,
      false,
      false,
      false
    )
    _scp(
      ssh_options,
      'root',
      ip_address,
      "/var/log/messages",
      name,
      false,
      false,
      false
    )
  }
}

def _get_image(String ssh_options, String ip_address, String regexp) {

  // Get the trident image string
  def cmd = "ssh $ssh_options root@$ip_address 'docker images | grep \'$regexp\''"
  def output = sh(
    label: "List docker images matching $regexp",
    returnStdout: true,
    script: cmd).trim()

  def fields = output.split()
  def image = fields[0] + ':' + fields[1]
  return image
}

def _initialize_status_file(String name, String status) {

  sh (
    label: "Create directory $env.WORKSPACE/status",
    script: "mkdir -p $env.WORKSPACE/status"
  )

  sh (label: "Sleep", script: "sleep 1")

  writeFile file: "$env.WORKSPACE/status/${name}-status.txt", text: "$status\n"

  sh (label: "Sleep", script: "sleep 1")
}

def _kubeadm_config(
  String name,
  String ssh_options,
  String id,
  String vm_path,
  String ip_address,
  String target,
  String scs_password,
  Map spec) {

    def image = sh(returnStdout: true, script: 'cat trident_docker_image.log').trim()

    if (spec['trident_image_distribution'] == 'load') {

      _scp(
        ssh_options,
        'root',
        ip_address,
        "trident_docker_image.tgz",
        vm_path,
        true,
        false,
        true
      )

      sh (
        label: "Load trident_docker_image.tgz on $ip_address",
        script: "ssh $ssh_options root@$ip_address docker load -i $vm_path/trident_docker_image.tgz"
      )

    }

    if (spec['trident_image_distribution'] == 'pull') {
      sh (
        label: "Pull $image on $ip_address",
        script: "ssh $ssh_options root@$ip_address docker pull $image"
      )
    }

    // Determine the trident installer name
    // trident-installer-19.04.0-test.trident-ci_master.tar.gz
    def cmd = 'ls trident-installer-*-test.' + _replace(env.BUILD_TAG, '/', '-') + '.tar.gz'
    def installer = sh(returnStdout: true, script: cmd).trim()

    // Configure the backend(s) with a unique object names and copy to the the VM
    def backend = spec['backend']
    def backends_configured = false
    _configure_backend_files(
      ssh_options,
      ip_address,
      "${id}t",
      "$name/backends",
      vm_path,
      "backends",
      backend,
      spec)

    backends_configured = _setup_backends(
      ssh_options,
      ip_address,
      name,
      vm_path,
      "backends")

    // Figure out if the install backend is the same as the test backend
    def install_backend = spec['install_backend']
    def install_backend_path = ''
    if (install_backend) {
      install_backend_matches = false
      for(be in backend.split(',')) {
        if (install_backend == be) {
          install_backend_matches = true
          echo "Install backend $install_backend matches test backend $be"
          break
        }
      }

      if (install_backend_matches == true) {
        // Use one of the backends we configured previously
        install_backend_path = "$name/backends/${install_backend}.json"
        echo 'Using ' + install_backend_path + ' for Trident installation'
      } else {
        install_backend_path = "$name/install_backend/${install_backend}.json"
        echo 'Using ' + install_backend_path + ' for Trident installation'
        // Create the backend file name that will be used to install trident
        _configure_backend_files(
          ssh_options,
          ip_address,
          "${id}i",
          "$name/install_backend",
          vm_path,
          "install_backend",
          install_backend,
          spec)

        backends_configured = _setup_backends(
          ssh_options,
          ip_address,
          name,
          vm_path,
          "install_backend")
      }
    }

    // Create the options that will be passed to ansible.py
    def options = []
    if (install_backend) {
      options = [
        '--extra-vars',
        '\'backend=' + env.WORKSPACE + '/' + install_backend_path +  ' ' +
        'image=' + image + ' ' +
        'installer=' + env.WORKSPACE + '/' + installer + ' ' +
        'install_target=kubeadm ' +
        'k8s_version=' + spec['k8s_version'] + ' ' +
        'namespace=trident ' +
        'remote_path=' + vm_path + '\'']
    } else {
      options = [
        '--extra-vars',
        '\'image=' + image + ' ' +
        'installer=' + env.WORKSPACE + '/' + installer + ' ' +
        'install_target=kubeadm ' +
        'k8s_version=' + spec['k8s_version'] + ' ' +
        'namespace=trident ' +
        'remote_path=' + vm_path + '\'']
    }

    if (spec['vm_provider'] == 'SCS') {
        options.push('--ssh-password')
        options.push(scs_password)
    } else if (spec['vm_provider'] == 'AWS') {
        options.push('--ssh-private-key')
        options.push("$env.WORKSPACE/tools/$env.SSH_PRIVATE_KEY_PATH")
    }

    // Run the trident installer playbook
    try {
      _run_playbook(name, spec['trident_install_playbook'], target, options)
    } catch(Exception e) {
      error e.getMessage()
    } finally {
      _scp(
        ssh_options,
        'root',
        ip_address,
        "$vm_path/trident_install.log",
        name,
        false,
        false,
        false
      )
    }

    return backends_configured
}

def _ndvp_binary_config(
  String name,
  String ssh_options,
  String id,
  String vm_path,
  String ip_address,
  String backend,
  Map spec) {

    _scp(
      ssh_options,
      'root',
      ip_address,
      "trident",
      vm_path,
      true,
      false,
      true
    )

    // Configure the backend(s) with a unique object names and copy to the the VM
    _configure_backend_files(
      ssh_options,
      ip_address,
      "${id}t",
      "$name/backends",
      vm_path,
      "backends",
      backend,
      spec)

    backends_configured = _setup_backends(
      ssh_options,
      ip_address,
      name,
      vm_path,
      "backends")

    return backends_configured
}

def _ndvp_plugin_config(
  String name,
  String ssh_options,
  String id,
  String vm_path,
  String ip_address,
  String backend,
  Map spec) {

    // Configure the backend(s) with a unique object names and copy to the the VM
    _configure_backend_files(
      ssh_options,
      ip_address,
      "${id}t",
      "$name/backends",
      vm_path,
      "backends",
      backend,
      spec)

    backends_configured = _setup_backends(
      ssh_options,
      ip_address,
      name,
      vm_path,
      "backends")

    // Move backend file to the default location
    sh (
      label: "Create directory /etc/netappdvp on $ip_address",
      script: "ssh $ssh_options root@$ip_address 'mkdir -p /etc/netappdvp'"
    )

    sh (label: "Sleep", script: "sleep 1")

    sh (
      label: "Copy backends/${backend}.json to $ip_address",
      script: "ssh $ssh_options root@$ip_address " +
        "'cp $vm_path/backends/${backend}.json /etc/netappdvp/config.json'"
    )

    sh (label: "Sleep", script: "sleep 1")

    sh (
      label: "List ISCSI modules on $ip_address",
      script: "ssh $ssh_options root@$ip_address 'lsmod | grep iscsi'"
    )

    sh (
      label: "Probe iscsi_tcp module on $ip_address",
      script: "ssh $ssh_options root@$ip_address 'modprobe iscsi_tcp || true'"
    )

    return backends_configured
}


def _notify_github(Map spec, String commit, String dir, String description, String state) {

  def name = spec['name']
  if (fileExists(dir) == false) {
    sh (
      label: "Create $dir",
      script: "mkdir -p $dir"
    )

    sh (label: "Sleep", script: "sleep 1")
  }

  sh (
    label: "Set Github status for $name to $state",
    script: "curl -s -S -H \"Authorization: token $env.GITHUB_TOKEN\" --request POST --data " +
    "'{\"state\": \"$state\", \"context\": \"$name\", \"description\": " +
    "\"$description\", \"target_url\": \"$env.BUILD_URL\"}' " +
    "$env.TRIDENT_GITHUB_STATUS_URL/$commit > $dir/notify_github_${state}.log 2>&1"
  )

}

def _notify_slack(String status, String message) {

    def color

    if (status == 'PENDING') {
        color = '#00FFFF'
    } else if (status == 'SUCCESS') {
        color = '#008000'
    } else if (status == 'ERROR') {
        color = '#FF0000'
    } else if (status == 'FAILURE') {
        color = '#FF0000'
    }

    slackSend(color: color, message: "$status: $message")
}

def _openshift_config(
  String name,
  String ssh_options,
  String id,
  String vm_path,
  String ip_address,
  String target,
  String scs_password,
  Map spec) {

    def image = sh(returnStdout: true, script: 'cat trident_docker_image.log').trim()

    if (spec['trident_image_distribution'] == 'load') {

      _scp(
        ssh_options,
        'root',
        ip_address,
        "trident_docker_image.tgz",
        vm_path,
        true,
        false,
        true
      )

      sh (
        label: "Load trident_docker_image.tgz on $ip_address",
        script: "ssh $ssh_options root@$ip_address docker load -i $vm_path/trident_docker_image.tgz"
      )

    }

    if (spec['trident_image_distribution'] == 'pull') {
      sh (
        label: "Pull $image",
        script: "ssh $ssh_options root@$ip_address docker pull $image"
      )
    }

    // Determine the trident installer name
    // trident-installer-19.04.0-test.trident-ci_master.tar.gz
    def cmd = 'ls trident-installer-*-test.' + _replace(env.BUILD_TAG, '/', '-') + '.tar.gz'
    def installer = sh(returnStdout: true, script: cmd).trim()

    // Configure the backend(s) with a unique object names and copy to the the VM
    def backend = spec['backend']
    def backends_configured = false
    _configure_backend_files(
      ssh_options,
      ip_address,
      "${id}t",
      "$name/backends",
      vm_path,
      "backends",
      backend,
      spec)

    backends_configured = _setup_backends(
      ssh_options,
      ip_address,
      name,
      vm_path,
      "backends")

    // Figure out if the install backend is the same as the test backend
    def install_backend = spec['install_backend']
    if (install_backend) {
      install_backend_matches = false
      for(be in backend.split(',')) {
        if (install_backend == be) {
          install_backend_matches = true
          echo "Install backend $install_backend matches test backend $be"
          break
        }
      }

      def install_backend_path = ''
      if (install_backend_matches == true) {
        // Use one of the backends we configured previously
        install_backend_path = "$name/backends/${install_backend}.json"
        echo 'Using ' + install_backend_path + ' for Trident installation'
      } else {
        install_backend_path = "$name/install_backend/${install_backend}.json"
        echo 'Using ' + install_backend_path + ' for Trident installation'
        // Create the backend file name that will be used to install trident
        _configure_backend_files(
          ssh_options,
          ip_address,
          "${id}i",
          "$name/install_backend",
          vm_path,
          "install_backend",
          install_backend,
          spec)

        backends_configured = _setup_backends(
          ssh_options,
          ip_address,
          name,
          vm_path,
          "install_backend")
      }
    }

    // Create the options that will be passed to ansible.py
    def options = []
    if (install_backend) {
      options = [
        '--extra-vars',
        '\'backend=' + env.WORKSPACE + '/' + install_backend_path +  ' ' +
        'image=' + image + ' ' +
        'installer=' + env.WORKSPACE + '/' + installer + ' ' +
        'install_target=openshift ' +
        'k8s_version=' + spec['k8s_version'] + ' ' +
        'namespace=myproject ' +
        'remote_path=' + vm_path + '\'']
    } else {
      options = [
        '--extra-vars',
        '\'image=' + image + ' ' +
        'installer=' + env.WORKSPACE + '/' + installer + ' ' +
        'install_target=openshift ' +
        'k8s_version=' + spec['k8s_version'] + ' ' +
        'namespace=myproject ' +
        'remote_path=' + vm_path + '\'']
    }

    if (spec['vm_provider'] == 'SCS') {
        options.push('--ssh-password')
        options.push(scs_password)
    } else if (spec['vm_provider'] == 'AWS') {
        options.push('--ssh-private-key')
        options.push("$env.WORKSPACE/tools/$env.SSH_PRIVATE_KEY_PATH")
    }

    // Run the trident installer playbook
    try {
      _run_playbook(name, spec['trident_install_playbook'], target, options)
    } catch(Exception e) {
      error e.getMessage()
    } finally {
      _scp(
        ssh_options,
        'root',
        ip_address,
        "$vm_path/trident_install.log",
        name,
        false,
        false,
        false
      )
    }

    return backends_configured
}

def _print_hold_jig_message() {

  // Get the job name
  def job_name≡ = ''
  def jn = _decode(env.JOB_NAME)
  if (jn.contains('/')) {
    (job_name) = jn.split('/')
  }

  // Display the message
  echo ("HOLD_JIG is set to $hold_jig, skipping stage cleanup.\n" +
        "When you are done debugging run:\n" +
        "tools/utils/ci_cleanup_by_artifact.sh production " +
        "$job_name $branch $env.BUILD_NUMBER")
}

def _propagate_changes() {

  // If there is a status dir then make sure all tests have recorded a
  // pass result before propagating

  // Check if the status dir exists in the workspace
  if (fileExists("$env.WORKSPACE/status")) {

    // Get the list of files
    echo "Checking for test failures in $env.WORKSPACE/status"
    def files = sh(script: "ls -1 $env.WORKSPACE/status", returnStdout: true).split()
    echo "Found " + files.size() + " files"

    // Get the status from each stages file
    def propagate = true
    for (file in files) {
      def status = sh(returnStdout: true, script: "head -1 $env.WORKSPACE/status/$file").trim()
      if (status == 'success') {
        echo "$file: SUCCESS"
      } else if (status == 'error') {
        echo "$file: ERROR"
        propagate = false
      } else if (status == 'failure') {
        echo "$file: FAILURE"
        propagate = false
      } else if (status == 'pending') {
        echo "$file: PENDING"
        propagate = false
      }
    }
    echo "Based on test status propagation is $propagate"

    // If all the tests are success and the branch name is master or stable/*
    // propagate the commit to the public repo

    if (propagate && (branch == 'master' || branch.startsWith('stable'))) {

      def name = "Propagate-Changes"
      try {

        // Define shortcut vars to save line space
        echo "Propagating changes to public repository"
        def private_org = env.TRIDENT_PRIVATE_GITHUB_ORG
        def private_repo = env.TRIDENT_PRIVATE_GITHUB_REPO
        def public_org = env.TRIDENT_PUBLIC_GITHUB_ORG
        def public_repo = env.TRIDENT_PUBLIC_GITHUB_REPO
        def user = env.GITHUB_USERNAME
        def token = env.GITHUB_TOKEN

        sh (
          label: "Create directory $env.WORKSPACE/$name",
          script: "mkdir -p $name"
        )

        sh (label: "Sleep", script: "sleep 1")

        // Create the propagate.sh script
        def content = (
          "cd $env.WORKSPACE/src2/github.com/netapp/trident\n" +
          "git checkout $branch\n" +
          "git status\n" +
          "git show-ref\n" +
          "git push -f https://$user:$token@github.com/$public_org/$public_repo $branch:$branch\n"
        )

        writeFile file: "$env.WORKSPACE/$name/propagate.sh", text: content
        sh (label: "Sleep", script: "sleep 1")

        sh (
          label: "Execute propagate.sh",
          script: "cd $env.WORKSPACE/$name;sh ./propagate.sh > propagate.log 2>&1"
        )

      } catch(Exception e) {

        // Throw an error if one is caught
        error "$name: " + e.getMessage()

      } finally {

        // Archive the work dir
        _archive_artifacts(name)
      }
    } else {

      // Explain why there is no propagation
      echo ("Skipping change propagation due to one or more of the following reasons: \n" +
            "- One or more tests failed \n" +
            "- The current BRANCH_NAME($branch) is not master \n" +
            "- The current BRANCH_NAME($branch) does not start with stable")
    }
  }
}

def _random_string(Integer length) {

  // Start the random string with a known string for easier cleanup
  if (length <= 4) {
      error "Generating random strings less than 4 characters is not supported"
  }

  // Find the real number of random strings needed
  length -= 2

  // Get a UUID from /dev/urandom with enought entropy
  def rs = 'ci'
  rs += sh (
    label: "Retrieve random string using /def/urandom",
    returnStdout: true,
    script: "head -c 500 /dev/urandom",
  ).trim().replaceAll("[^a-zA-Z0-9]+","").take(length)

  return rs
}

def _replace(String string, String regexp, String replacement) {
  def new_string = _decode(string)
  new_string.replaceAll(regexp, replacement)
  return new_string
}

def _run_csi_sanity_tests(String name, String ssh_options, String ip_address, String vm_path, Map spec, String id) {

  try {

    echo "Running CSI sanity test"

    def backend = spec['backend']
    content = ('\n' +
"cd $vm_path\n" +
"export TRIDENT_SERVER=127.0.0.1:8000\n" +
'echo "127.0.0.1 trident-csi" >> /etc/hosts\n' +
"tar -xvf trident-installer.tar.gz\n" +
"cp trident-installer/tridentctl /bin/tridentctl\n" +
"cp trident-installer/extras/bin/trident .\n" +
"cp trident-installer/tridentctl .\n" +
"cp $vm_path/go/src/github.com/kubernetes-csi/csi-test/cmd/csi-sanity/csi-sanity .\n" +
"sleep 1\n" +
"chmod +x trident tridentctl csi-sanity\n" +
"./trident --https_rest --csi_endpoint unix://tmp/csi.sock --csi_node_name $ip_address --no_persistence --debug --csi_role allInOne --https_port=34571 2> trident.log & PID=\$!\n" +
"TIMEOUT=180\n" +
"while ! tridentctl get node $ip_address; do\n" +
"    if (( TIMEOUT <= 5 )); then\n" +
"         echo \"Trident failed to come up in time\"\n" +
"         kill \$PID\n" +
"         exit 1\n" +
"    fi\n" +
"    sleep 5\n" +
"    (( TIMEOUT -= 5 ))\n" +
"done\n" +
"tridentctl create backend -f $vm_path/backends/${backend}.json\n" +
"./csi-sanity --csi.endpoint=/tmp/csi.sock -csi.junitfile=csi_sanity.xml\n" +
"kill \$PID\n")

    writeFile file: "$name/csi_sanity.sh", text: content
    sh (label: "Sleep", script: "sleep 1")

    _scp(
      ssh_options,
      'root',
      ip_address,
      "$name/csi_sanity.sh",
      vm_path,
      true,
      false,
      true
    )

    sh (label: "Sleep", script: "sleep 1")

    sh (
      label: "Execute csi_sanity.sh on $ip_address",
      script: "ssh $ssh_options root@$ip_address 'cd $vm_path;sh ./csi_sanity.sh > csi_sanity.log 2>&1'"
    )

  } catch(Exception e) {
    // Fail the run
    error "CSI Sanity failure"
  } finally {

    _scp(
      ssh_options,
      'root',
      ip_address,
      "$vm_path/*.log",
      name,
      false,
      false,
      false
    )

    _scp(
      ssh_options,
      'root',
      ip_address,
      "$vm_path/*.xml",
      name,
      false,
      false,
      false
    )

    try {

      sh (
        label: "Copy csi_sanity.xml to junit",
        script: "cp $name/csi_sanity.xml junit/$name-csi-sanity.xml"
      )

    } catch(Exception e) {
      echo "Failed to copy $name/csi_sanity.xml"
    }

  }
}

def _run_playbook(String name, String playbook, String target, List options=[]){

  sh (
    label: "Copy ansible source to stage directory",
    script: "cp -r tools/ansible $name"
  )

  sh (label: "Sleep", script: "sleep 1")

  try {
    // Run the appropriate ansible playbook
    def cmd =("cd $name/ansible; python3 $env.WORKSPACE/tools/utils/ansible.py " +
              "--log-path $env.WORKSPACE/$name/${playbook}.log " +
              "--playbook $playbook " +
              "--targets $target ")
    if (options) {
        cmd += options.join(' ')
    }
    sh (
      label: "Execute ansible playbook $playbook against $target",
      script: "$cmd"
    )
  } catch(Exception e) {
    // Fail
    error "Failure running ansible playbook $playbook"
  }
}

def _run_whelk_tests(String name, String ssh_options, String ip_address, String vm_path, Map spec) {

  def test = spec['test']
  try {
    if (test == 'kubeadm') {

      sh (
        label: "Change permissions for run_trident_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address 'chmod +x $vm_path/test/run_trident_tests.sh'"
      )

      sh (
        label: "Execute run_trident_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address " +
          "'cd $vm_path/test;" +
          "$vm_path/test/run_trident_tests.sh " +
          "-n trident -c $vm_path/backends > " +
          "$vm_path/test/whelk_stdout.log 2>&1'"
        )
    }
    if (test == 'docker_ee') {

      sh (
        label: "Change permissions for run_trident_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address 'chmod +x $vm_path/test/run_trident_tests.sh'"
      )

      sh (
        label: "Execute run_trident_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address " +
          "'cd $vm_path/test;" +
          "export DOCKER_TLS_VERIFY=1;" +
          "export COMPOSE_TLS_VERSION=TLSv1_2;"+
          "export DOCKER_CERT_PATH=/root;" +
          "export DOCKER_HOST=tcp://$ip_address:443;" +
          "export KUBECONFIG=/root/kube.yml;" +
          "$vm_path/test/run_trident_tests.sh " +
          "-n trident -k trident -c $vm_path/backends > " +
          "$vm_path/test/whelk_stdout.log 2>&1'"
        )
    }
    if (test == 'ndvp_binary') {

      sh (
        label: "Change permissions for run_ndvp_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address 'chmod +x $vm_path/test/run_ndvp_tests.sh'"
      )

      def backend = spec['backend']

      sh (
        label: "Execute run_ndvp_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address " +
          "'cd $vm_path/test;" +
          "$vm_path/test/run_ndvp_tests.sh " +
          "-b $vm_path/trident " +
          "-c $vm_path/backends/${backend}.json > " +
          "$vm_path/test/whelk_stdout.log 2>&1'"
      )
    }
    if (test == 'ndvp_plugin') {

      sh (
        label: "Change permissions for run_plugin_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address 'chmod +x $vm_path/test/run_plugin_tests.sh'"
      )

      sh (
        label: "Execute run_plugin_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address " +
          "'cd $vm_path/test;" +
          "$vm_path/test/run_plugin_tests.sh -i $env.PRIVATE_DOCKER_REGISTRY/trident-plugin:" +
          _replace(env.BUILD_TAG, '/', '-') +
          " > " +
          "$vm_path/test/whelk_stdout.log 2>&1'"
        )
    }
    if (test == 'openshift') {

      sh (
        label: "Change permissions for run_trident_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address 'chmod +x $vm_path/test/run_trident_tests.sh'"
      )

      sh (
        label: "Execute run_trident_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address " +
          "'cd $vm_path/test;" +
          "$vm_path/test/run_trident_tests.sh -f " +
          "-n trident -c $vm_path/backends " +
          "-n myproject -o > " +
          "$vm_path/test/whelk_stdout.log 2>&1'"
      )
    }
    if (test == 'upgrade') {

      // Determine the trident installer name
      // trident-installer-19.04.0-test.trident-ci_master.tar.gz
      def cmd = 'ls trident-installer-*-test.' + _replace(env.BUILD_TAG, '/', '-') + '.tar.gz'
      def installer = sh(
        label: cmd,
        returnStdout: true,
        script: cmd).trim()

      _scp(
        ssh_options,
        'root',
        ip_address,
        installer,
        vm_path,
        true,
        false,
        true
      )

      sh (
        label: "Uncompress the trident installer",
        script: "ssh $ssh_options root@$ip_address 'cd $vm_path;tar -xvzf $installer'"
      )

      sh (
        label: "Change permissions for run_trident_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address 'chmod +x $vm_path/test/run_trident_tests.sh'"
      )

      sh (
        label: "Execute run_trident_tests.sh on $ip_address",
        script: "ssh $ssh_options root@$ip_address " +
          "'cd $vm_path/test;" +
          "ETCD_VERSION=v3.3.13 " +
          "$vm_path/test/run_trident_tests.sh " +
          "-s -u -c $vm_path/backends " +
          "-g $env.GITHUB_TOKEN " +
          "-i $vm_path/trident-installer " +
          "whelk.tests.trident.scenario.test_upgrade " +
          "> $vm_path/test/whelk_stdout.log 2>&1'"
      )
    }
  } catch(Exception e) {
    // Fail the run
    error "Failure in Whelk test(s)"
  } finally {

    _scp(
      ssh_options,
      'root',
      ip_address,
      "$vm_path/test/*.log",
      name,
      false,
      false,
      false
    )

    try {

      sh (
        label: "Remove the whelk color log",
        script: "rm -f $name/whelk.color.log"
      )

    } catch(Exception e) {
      echo "Error removing whelk_color.log"
    }

    _scp(
      ssh_options,
      'root',
      ip_address,
      "$vm_path/test/*.xml",
      name,
      false,
      false,
      false
    )

    try {

      sh (
        label: "Copy all test reports to the junit directory",
        script: "cd $name; for f in *.xml; do cp \$f ../junit/$name-\$f; done"
      )

    } catch(Exception e) {
      echo "Failure copying $name/nosetests.xml"
    }

    _gather_triage_information(spec, ssh_options, ip_address, name, vm_path)

  }
}

def _scp (
  String ssh_options,
  String username,
  String ip_address,
  String src,
  String dst,
  Boolean source_is_local,
  Boolean recurse,
  Boolean error_on_exception) {

  try {
    def label = ''
    def script = "scp $ssh_options "
    if (recurse) { script += '-r ' }
    if (source_is_local) {
      label = "Copy $src to $ip_address"
      script +="$src $username@$ip_address:$dst"
    } else {
      label = "Copy $src from $ip_address"
      script +="$username@$ip_address:$src $dst"
    }

    sh (label: label, script: script)
    sh (label: "Sleep", script: "sleep 1")
  } catch(Exception e) {
    msg = "Error copying "
    if (source_is_local) {
      msg += "to "
    } else {
      msg += "from "
    }
    msg += "$ip_address: " + e.getMessage()

    if (error_on_exception) {
      error msg
    } else {
      echo msg
    }
  }
}

def _scs_create(String name, String request, String purpose) {

  def cmd = ''
  def ri = []
  def scs = []
  def ti = []
  def vmi = []

  try {

    // Get the template info from the request file
    cmd = ("python3 tools/utils/parse_scs_request.py --file tools/scs/$request")
    ri = sh(label: "Parse the SCS request file", returnStdout: true, script: cmd).trim().split(',')

  } catch(Exception e) {
    // Fail the run
    error "Failed to parse tools/scs/$request"
  }

  try {

    // Get the template info from SCS
    cmd = ("python3 tools/utils/scs.py " +
        "--action get-templates " +
        "--endpoint $env.SCS_ENDPOINT " +
        "--token $env.SCS_TOKEN " +
        "--user $env.SCS_USERNAME " +
        "--get-templates-name ^" + ri[0] + "\$ " +
        "--get-templates-locality " + ri[1] + " " +
        "--get-templates-attributes name,locality,guest_username,guest_password")
    ti = sh(label: "Get the root password from the template", returnStdout: true, script: cmd).trim().split(',')

  } catch(Exception e) {
    // Fail the run
    error "Failed to get template info for " + ri[0]
  }

  def provisioned = false
  def attempt = 0

  while(provisioned == false) {
    attempt++
    try {
      sh (
        label: "Create SCS VMs attempt #" + attempt + "/3",
        script: "python3 tools/utils/scs.py " +
          "--action create-vms " +
          "--endpoint $env.SCS_ENDPOINT " +
          "--token $env.SCS_TOKEN " +
          "--user $env.SCS_USERNAME " +
          "--purpose $purpose " +
          "--days $env.SCS_REQUEST_LENGTH " +
          "--request tools/scs/$request " +
          "--wait $env.SCS_REQUEST_WAIT"
      )
      provisioned = true
    } catch(Exception e) {
      _scs_delete(purpose)
      if(attempt == 3) {
        error "Failed to deploy VM(s) after 3 attempts"
        return
      }
    }
  }

  try {
    // Get the client info from SCS
    cmd = ("python3 tools/utils/scs.py " +
        "--action get-vms " +
        "--endpoint $env.SCS_ENDPOINT " +
        "--token $env.SCS_TOKEN " +
        "--user $env.SCS_USERNAME " +
        "--purpose $purpose " +
        "--get-vms-attributes host_name,domain_name,ip_addr")
    vmi = sh(label: "Get the IP addresses for all VMs", returnStdout: true, script: cmd).trim().split(';')
    scs.add(vmi[0])
    scs.add(vmi[1])
    scs.add(vmi[2])
    scs.add(ti[3])

  } catch(Exception e) {
    // Fail the run
    error "Failed to get VM info for " + ri[0]
  }

  return scs

}

def _scs_delete(String purpose) {

  def attempt = 0
  def deleted = false
  while(deleted == false && attempt < 3) {
    attempt++
    try {
      sh (
        label: "Delete SCS VMs attempt #" + attempt + "/3",
        script: "python3 tools/utils/scs.py " +
          "--action delete-vms " +
          "--endpoint $env.SCS_ENDPOINT " +
          "--token $env.SCS_TOKEN " +
          "--user $env.SCS_USERNAME " +
          "--purpose $purpose"
      )
      deleted = true
    } catch(Exception e) {
      if(attempt == 3) {
        error "Failed to delete VMs after 3 attempts"
      }
      sh (label: "Sleep", script: "sleep 10")
    }
  }
}

def _setup_backends(String ssh_options, String ip_address, String name, String vm_path, String dst_path) {

  echo "Setting up backends"

  sh (
    label: "Install required python packages on $ip_address",
    script: "ssh $ssh_options root@$ip_address 'pip install " +
      "-r $vm_path/test/requirements.txt > $vm_path/test/requirements.log 2>&1'"
  )

  _scp(
    ssh_options,
    'root',
    ip_address,
    "$vm_path/test/requirements.log",
    name,
    false,
    false,
    false
  )

  sh (
    label: "Change permissions for setup_backend.py on $ip_address",
    script: "ssh $ssh_options root@$ip_address 'chmod +x $vm_path/test/whelk/ci/setup_backend.py'"
  )

  sh (
    label: "Execute setup_backend.py on $ip_address",
    script: "ssh $ssh_options root@$ip_address '" +
      "export NDVP_CONFIG=$vm_path/$dst_path;" +
      "export PYTHONPATH=$vm_path/test;" +
      "export SF_ADMIN_PASSWORD=$env.SOLIDFIRE_ADMIN_PASSWORD;" +
      "cd $vm_path/test; " +
      "python whelk/ci/setup_backend.py'"
  )

  return true

}

def _test_failures(String path) {

  if (fileExists(path)) {
    def cmd = "cat $path | grep testsuite"
    def output = sh(returnStdout: true, script: cmd).trim()
    def ts = _parse_testsuite_line(output)
    if (ts[2] > 0) {
      return true
    } else {
      return false
    }
  }
}

def _upgrade_config(
  String name,
  String ssh_options,
  String id,
  String vm_path,
  String ip_address,
  String target,
  String scs_password,
  Map spec) {

    def image = sh(returnStdout: true, script: 'cat trident_docker_image.log').trim()

    if (spec['trident_image_distribution'] == 'load') {

      _scp(
        ssh_options,
        'root',
        ip_address,
        "trident_docker_image.tgz",
        vm_path,
        true,
        false,
        true
      )

      sh (
        label: "Load trident_docker_image.tgz on $ip_address",
        script: "ssh $ssh_options root@$ip_address docker load -i $vm_path/trident_docker_image.tgz"
      )

    }

    if (spec['trident_image_distribution'] == 'pull') {
      sh (
        label: "Pull $image on $ip_address",
        script: "ssh $ssh_options root@$ip_address docker pull $image"
      )
    }

    // Determine the trident installer name
    // trident-installer-19.04.0-test.trident-ci_master.tar.gz
    cmd = 'ls trident-installer-*-test.' + _replace(env.BUILD_TAG, '/', '-') + '.tar.gz'
    def installer = sh(returnStdout: true, script: cmd).trim()

    // Configure the backend(s) with a unique object names and copy to the the VM
    def backend = spec['backend']
    def backends_configured = false
    _configure_backend_files(
      ssh_options,
      ip_address,
      "${id}t",
      "$name/backends",
      vm_path,
      "backends",
      backend,
      spec)

    backends_configured = _setup_backends(
      ssh_options,
      ip_address,
      name,
      vm_path,
      "backends")

    // Figure out if the install backend is the same as the test backend
    def install_backend = spec['install_backend']
    def install_backend_path = ''
    if (install_backend) {
      install_backend_matches = false
      for(be in backend.split(',')) {
        if (install_backend == be) {
          install_backend_matches = true
          echo "Install backend $install_backend matches test backend $be"
          break
        }
      }

      if (install_backend_matches == true) {
        // Use one of the backends we configured previously
        install_backend_path = "$name/backends/${install_backend}.json"
        echo 'Using ' + install_backend_path + ' for Trident installation'
      } else {
        install_backend_path = "$name/install_backend/${install_backend}.json"
        echo 'Using ' + install_backend_path + ' for Trident installation'
        // Create the backend file name that will be used to install trident
        _configure_backend_files(
          ssh_options,
          ip_address,
          "${id}i",
          "$name/install_backend",
          vm_path,
          "install_backend",
          install_backend,
          spec)

        backends_configured = _setup_backends(
          ssh_options,
          ip_address,
          name,
          vm_path,
          "install_backend")
      }
    }
    // Create the options that will be passed to ansible.py
    def options = []
    if (install_backend) {
      options = [
        '--extra-vars',
        '\'backend=' + env.WORKSPACE + '/' + install_backend_path +  ' ' +
        'image=' + image + ' ' +
        'installer=' + env.WORKSPACE + '/' + installer + ' ' +
        'install_target=upgrade ' +
        'k8s_version=' + spec['k8s_version'] + ' ' +
        'remote_path=' + vm_path + '\'']
    } else {
      options = [
        '--extra-vars',
        '\'image=' + image + ' ' +
        'installer=' + env.WORKSPACE + '/' + installer + ' ' +
        'install_target=upgrade ' +
        'k8s_version=' + spec['k8s_version'] + ' ' +
        'remote_path=' + vm_path + '\'']
    }

    if (spec['vm_provider'] == 'SCS') {
        options.push('--ssh-password')
        options.push(scs_password)
    } else if (spec['vm_provider'] == 'AWS') {
        options.push('--ssh-private-key')
        options.push("$env.WORKSPACE/tools/$env.SSH_PRIVATE_KEY_PATH")
    }

    // Run the trident installer playbook
    try {
      _run_playbook(name, spec['trident_install_playbook'], target, options)
    } catch(Exception e) {
      error e.getMessage()
    } finally {
      _scp(
        ssh_options,
        'root',
        ip_address,
        "$vm_path/trident_install.log",
        name,
        false,
        false,
        false
      )
    }

    return backends_configured
}

def _write_status_file(String name, String status) {

  writeFile file: "$name/${name}-status.txt", text: "$status\n"

  sh (label: "Sleep", script: "sleep 1")

  sh (
    label: "Copy ${name}-status.txt to status directory",
    script: "cp $name/${name}-status.txt $env.WORKSPACE/status/"
  )
}

@NonCPS
def _coverage_match(Map spec, String regexp) {

  def m = spec['coverage'] =~ /$regexp/
  if (m) {
    return true
  } else {
    return false
  }
}

@NonCPS
def _get_domain_from_fqdn(String line) {

  def m = line =~ /^([^\\.]+).(.*)/
  def hostname = ''
  if (m) {
    hostname = m[0][2]
  } else {
    // Fail the run
    error 'Failed to match domain in FQDN : ' + line
  }
  return hostname
}

@NonCPS
def _get_hostname_from_fqdn(String line) {

  def m = line =~ /^([^\\.]+)/
  def hostname = ''
  if (m) {
    hostname = m[0][1]
  } else {
    // Fail the run
    error 'Failed to match hostname in FQDN : ' + line
  }
  return hostname
}

@NonCPS
def _parse_testsuite_line(String line) {

  def errors = 0
  def failures = 0
  def tests = 0

  def e = line =~ /errors="([0-9]*)"/
  if (e) {
    errors = e[0][1]
  }

  def f = line =~ /failures="([0-9]*)"/
  if (f) {
    failures = f[0][1]
  }

  def t = line =~ /tests="([0-9]*)"/
  if (t) {
    tests = t[0][1]
  }

  return [tests, errors, failures]
}

@NonCPS
def _stage_name_match(Map spec, String regexp) {

  def m = spec['name'] =~ /$regexp/
  if (m) {
    return true
  } else {
    return false
  }
}

@NonCPS
def _whelk_test_match(Map spec, String regexp) {

  def m = spec['test'] =~ /$regexp/
  if (m) {
    return true
  } else {
    return false
  }
}
