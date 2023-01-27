package common

type GetLogLevelResponse struct {
	LogLevel string `json:"logLevel"`
	Error    string `json:"error,omitempty"`
}

type GetLoggingWorkflowsResponse struct {
	LogWorkflows string `json:"logWorkflows"`
	Error        string `json:"error,omitempty"`
}

type GetLoggingLayersResponse struct {
	LogLayers string `json:"logLayers"`
	Error     string `json:"error,omitempty"`
}
