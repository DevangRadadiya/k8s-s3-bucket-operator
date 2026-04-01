package backend

// Capabilities describes optional features for documentation and future validation.
type Capabilities struct {
	ObjectLocking bool
	BucketQuota   bool
	Lifecycle     bool
	Replication   bool
	// IAMUsers indicates MinIO-style named users / policies vs other credential models.
	IAMUsers bool
}

// CapabilitiesForKind returns a static capability matrix for the given kind.
// Values will evolve as backends are implemented.
func CapabilitiesForKind(k Kind) Capabilities {
	switch k {
	case KindMinIO:
		return Capabilities{
			ObjectLocking: true,
			BucketQuota:   true,
			Lifecycle:     true,
			Replication:   true,
			IAMUsers:      true,
		}
	case KindAWS:
		return Capabilities{
			ObjectLocking: true,
			BucketQuota:   false,
			Lifecycle:     true,
			Replication:   false,
			IAMUsers:      true,
		}
	default:
		return Capabilities{}
	}
}
