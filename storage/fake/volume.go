package fake

type Volume struct {
	Name          string
	RequestedPool string
	PhysicalPool  string
	SizeBytes     uint64
}
