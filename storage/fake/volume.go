package fake

type Volume struct {
	Name          string `json:"name"`
	RequestedPool string `json:"requestedPool"`
	PhysicalPool  string
	SizeBytes     uint64 `json:"size"`
}

type CreatingVolume struct {
	Name string
	Iterations int
	FinalIteration int
	Error string
}
