package logging

const (
	WorkflowFlagSeparator      = ":"
	workflowCategorySeparator  = "="
	workflowOperationSeparator = ","

	CategoryCore           = WorkflowCategory("core")
	CategoryCRDController  = WorkflowCategory("crd_controller")
	CategoryCR             = WorkflowCategory("cr")
	CategoryStorageClient  = WorkflowCategory("storage_client")
	CategoryPlugin         = WorkflowCategory("plugin")
	CategoryVolume         = WorkflowCategory("volume")
	CategoryStorageClass   = WorkflowCategory("storage_class")
	CategoryNode           = WorkflowCategory("node")
	CategoryBackend        = WorkflowCategory("backend")
	CategorySnapshot       = WorkflowCategory("snapshot")
	CategoryController     = WorkflowCategory("controller")
	CategoryNodeServer     = WorkflowCategory("node_server")
	CategoryIdentityServer = WorkflowCategory("identity_server")
	CategoryGRPC           = WorkflowCategory("grpc")
	CategoryTridentREST    = WorkflowCategory("trident_rest")
	CategoryK8sClient      = WorkflowCategory("k8s_client")
	CategoryNone           = WorkflowCategory("none")

	OpBootstrap        = WorkflowOperation("bootstrap")
	OpVersion          = WorkflowOperation("version")
	OpNodeReconcile    = WorkflowOperation("node_reconcile")
	OpBackendReconcile = WorkflowOperation("backend_reconcile")
	OpReconcile        = WorkflowOperation("reconcile")
	OpTrace            = WorkflowOperation("trace")
	OpLogger           = WorkflowOperation("logger")
	OpActivate         = WorkflowOperation("activate")
	OpDeactivate       = WorkflowOperation("deactivate")
	OpCreate           = WorkflowOperation("create")
	OpGet              = WorkflowOperation("get")
	OpGetInfo          = WorkflowOperation("get_info")
	OpGetStats         = WorkflowOperation("get_stats")
	OpGetPath          = WorkflowOperation("get_path")
	OpList             = WorkflowOperation("list")
	OpUpdate           = WorkflowOperation("update")
	OpDelete           = WorkflowOperation("delete")
	OpUpgrade          = WorkflowOperation("upgrade")
	OpClone            = WorkflowOperation("clone")
	OpCloneFrom        = WorkflowOperation("clone_from")
	OpImport           = WorkflowOperation("import")
	OpResize           = WorkflowOperation("resize")
	OpMount            = WorkflowOperation("mount")
	OpUnmount          = WorkflowOperation("unmount")
	OpGetCapabilties   = WorkflowOperation("get_capabilities")
	OpProbe            = WorkflowOperation("probe")
	OpGetResponse      = WorkflowOperation("get_response")
	OpPublish          = WorkflowOperation("publish")
	OpUnpublish        = WorkflowOperation("unpublish")
	OpStage            = WorkflowOperation("stage")
	OpUnstage          = WorkflowOperation("unstage")
	OpHealISCSI        = WorkflowOperation("heal_iscsi")
	OpReconcilePubs    = WorkflowOperation("reconcile_publications")
	OpTraceFactory     = WorkflowOperation("trace_factory")
	OpTraceAPI         = WorkflowOperation("trace_api")
	OpInit             = WorkflowOperation("init")
	OpNone             = WorkflowOperation("none")
	OpAll              = WorkflowOperation("all")
)

var (
	// Allowable workflows for trace logging.
	WorkflowCoreBootstrap        = Workflow{CategoryCore, OpBootstrap}
	WorkflowCoreVersion          = Workflow{CategoryCore, OpVersion}
	WorkflowCoreInit             = Workflow{CategoryCore, OpInit}
	WorkflowCoreNodeReconcile    = Workflow{CategoryCore, OpNodeReconcile}
	WorkflowCoreBackendReconcile = Workflow{CategoryCore, OpBackendReconcile}

	WorkflowGRPCTrace = Workflow{CategoryGRPC, OpTrace}

	WorkflowTridentRESTLogger = Workflow{CategoryTridentREST, OpLogger}

	WorkflowK8sClientFactory = Workflow{CategoryK8sClient, OpTraceFactory}
	WorkflowK8sClientAPI     = Workflow{CategoryK8sClient, OpTraceAPI}

	WorkflowCRDControllerCreate = Workflow{CategoryCRDController, OpCreate}
	WorkflowCRReconcile         = Workflow{CategoryCR, OpReconcile}

	WorkflowStorageClientCreate = Workflow{CategoryStorageClient, OpCreate}

	WorkflowPluginCreate     = Workflow{CategoryPlugin, OpCreate}
	WorkflowPluginList       = Workflow{CategoryPlugin, OpList}
	WorkflowPluginGet        = Workflow{CategoryPlugin, OpGet}
	WorkflowPluginActivate   = Workflow{CategoryPlugin, OpActivate}
	WorkflowPluginDeactivate = Workflow{CategoryPlugin, OpDeactivate}

	WorkflowVolumeCreate          = Workflow{CategoryVolume, OpCreate}
	WorkflowVolumeUpdate          = Workflow{CategoryVolume, OpUpdate}
	WorkflowVolumeUpgrade         = Workflow{CategoryVolume, OpUpgrade}
	WorkflowVolumeGet             = Workflow{CategoryVolume, OpGet}
	WorkflowVolumeGetStats        = Workflow{CategoryVolume, OpGetStats}
	WorkflowVolumeGetPath         = Workflow{CategoryVolume, OpGetPath}
	WorkflowVolumeList            = Workflow{CategoryVolume, OpList}
	WorkflowVolumeDelete          = Workflow{CategoryVolume, OpDelete}
	WorkflowVolumeClone           = Workflow{CategoryVolume, OpClone}
	WorkflowVolumeImport          = Workflow{CategoryVolume, OpImport}
	WorkflowVolumeResize          = Workflow{CategoryVolume, OpResize}
	WorkflowVolumeMount           = Workflow{CategoryVolume, OpMount}
	WorkflowVolumeUnmount         = Workflow{CategoryVolume, OpUnmount}
	WorkflowVolumeGetCapabilities = Workflow{CategoryVolume, OpGetCapabilties}

	WorkflowStorageClassCreate = Workflow{CategoryStorageClass, OpCreate}
	WorkflowStorageClassGet    = Workflow{CategoryStorageClass, OpGet}
	WorkflowStorageClassUpdate = Workflow{CategoryStorageClass, OpUpdate}
	WorkflowStorageClassList   = Workflow{CategoryStorageClass, OpList}
	WorkflowStorageClassDelete = Workflow{CategoryStorageClass, OpDelete}

	WorkflowNodeCreate          = Workflow{CategoryNode, OpCreate}
	WorkflowNodeGet             = Workflow{CategoryNode, OpGet}
	WorkflowNodeGetInfo         = Workflow{CategoryNode, OpGetInfo}
	WorkflowNodeGetCapabilities = Workflow{CategoryNode, OpGetCapabilties}
	WorkflowNodeGetResponse     = Workflow{CategoryNode, OpGetResponse}
	WorkflowNodeUpdate          = Workflow{CategoryNode, OpUpdate}
	WorkflowNodeList            = Workflow{CategoryNode, OpList}
	WorkflowNodeDelete          = Workflow{CategoryNode, OpDelete}

	WorkflowIdentityProbe           = Workflow{CategoryIdentityServer, OpProbe}
	WorkflowIdentityGetInfo         = Workflow{CategoryIdentityServer, OpGetInfo}
	WorkflowIdentityGetCapabilities = Workflow{CategoryIdentityServer, OpGetCapabilties}

	WorkflowBackendCreate = Workflow{CategoryBackend, OpCreate}
	WorkflowBackendDelete = Workflow{CategoryBackend, OpDelete}
	WorkflowBackendGet    = Workflow{CategoryBackend, OpGet}
	WorkflowBackendUpdate = Workflow{CategoryBackend, OpUpdate}
	WorkflowBackendList   = Workflow{CategoryBackend, OpList}

	WorkflowSnapshotCreate    = Workflow{CategorySnapshot, OpCreate}
	WorkflowSnapshotDelete    = Workflow{CategorySnapshot, OpDelete}
	WorkflowSnapshotGet       = Workflow{CategorySnapshot, OpGet}
	WorkflowSnapshotUpdate    = Workflow{CategorySnapshot, OpUpdate}
	WorkflowSnapshotList      = Workflow{CategorySnapshot, OpList}
	WorkflowSnapshotCloneFrom = Workflow{CategorySnapshot, OpCloneFrom}

	WorkflowControllerPublish         = Workflow{CategoryController, OpPublish}
	WorkflowControllerUnpublish       = Workflow{CategoryController, OpUnpublish}
	WorkflowControllerGetCapabilities = Workflow{CategoryController, OpGetCapabilties}

	WorkflowNodeStage         = Workflow{CategoryNodeServer, OpStage}
	WorkflowNodeUnstage       = Workflow{CategoryNodeServer, OpUnstage}
	WorkflowNodePublish       = Workflow{CategoryNodeServer, OpPublish}
	WorkflowNodeUnpublish     = Workflow{CategoryNodeServer, OpUnpublish}
	WorkflowNodeHealISCSI     = Workflow{CategoryNodeServer, OpHealISCSI}
	WorkflowNodeReconcilePubs = Workflow{CategoryNodeServer, OpReconcilePubs}

	WorkflowNone = Workflow{CategoryNone, OpNone}

	workflowTypes = []Workflow{
		WorkflowCoreBootstrap,
		WorkflowCoreVersion,
		WorkflowCoreInit,
		WorkflowCoreNodeReconcile,
		WorkflowGRPCTrace,
		WorkflowTridentRESTLogger,
		WorkflowCRDControllerCreate,
		WorkflowCRReconcile,
		WorkflowStorageClientCreate,
		WorkflowPluginCreate,
		WorkflowPluginList,
		WorkflowPluginGet,
		WorkflowPluginActivate,
		WorkflowPluginDeactivate,
		WorkflowVolumeCreate,
		WorkflowVolumeUpdate,
		WorkflowVolumeUpgrade,
		WorkflowVolumeGet,
		WorkflowVolumeGetStats,
		WorkflowVolumeGetPath,
		WorkflowVolumeList,
		WorkflowVolumeDelete,
		WorkflowVolumeClone,
		WorkflowVolumeImport,
		WorkflowVolumeResize,
		WorkflowVolumeMount,
		WorkflowVolumeUnmount,
		WorkflowVolumeGetCapabilities,
		WorkflowStorageClassCreate,
		WorkflowStorageClassGet,
		WorkflowStorageClassUpdate,
		WorkflowStorageClassList,
		WorkflowStorageClassDelete,
		WorkflowNodeCreate,
		WorkflowNodeGet,
		WorkflowNodeGetInfo,
		WorkflowNodeGetCapabilities,
		WorkflowNodeGetResponse,
		WorkflowNodeUpdate,
		WorkflowNodeList,
		WorkflowNodeDelete,
		WorkflowBackendCreate,
		WorkflowBackendDelete,
		WorkflowBackendGet,
		WorkflowBackendUpdate,
		WorkflowBackendList,
		WorkflowSnapshotCreate,
		WorkflowSnapshotDelete,
		WorkflowSnapshotGet,
		WorkflowSnapshotUpdate,
		WorkflowSnapshotList,
		WorkflowSnapshotCloneFrom,
		WorkflowControllerPublish,
		WorkflowControllerUnpublish,
		WorkflowControllerGetCapabilities,
		WorkflowNodeStage,
		WorkflowNodeUnstage,
		WorkflowNodePublish,
		WorkflowNodeUnpublish,
		WorkflowK8sClientFactory,
		WorkflowK8sClientAPI,
	}
)
