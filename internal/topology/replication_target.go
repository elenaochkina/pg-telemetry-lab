package topology

// ReplicaReplicationTarget is a replica connection target + the subscription name we expect on it.
type ReplicaReplicationTarget struct {
	Target           PGTarget
	Index            int
	SubscriptionName string
}
