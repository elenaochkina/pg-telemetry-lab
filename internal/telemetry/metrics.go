package telemetry

import "time"

// ReplicationMetrics contains all replication lag metrics.
type ReplicationMetrics struct {
	Timestamp time.Time        `json:"timestamp"`
	Primary   PrimaryMetrics   `json:"primary"`
	Replicas  []ReplicaMetrics `json:"replicas"`
}

// PrimaryMetrics contains metrics from the primary database.
type PrimaryMetrics struct {
	Host         string            `json:"host"`
	CurrentLSN   int64             `json:"current_lsn_bytes"` // LSN as long integer
	Connections  []ReplicationSlot `json:"connections"`
}

// ReplicationSlot represents a single replication connection.
type ReplicationSlot struct {
	ApplicationName string `json:"application_name"`
	ClientAddr      string `json:"client_addr"`
	State           string `json:"state"`
	SentLSN         string `json:"sent_lsn"`    // Hex format: "0/5A2F3C8"
	WriteLSN        string `json:"write_lsn"`   // Hex format
	FlushLSN        string `json:"flush_lsn"`   // Hex format
	ReplayLSN       string `json:"replay_lsn"`  // Hex format: "0/5A2F000"
	LagBytes        int64  `json:"lag_bytes"`   // Long integer: 968
	SyncState       string `json:"sync_state"`
}

// ReplicaMetrics contains metrics from a replica database.
type ReplicaMetrics struct {
	Host             string    `json:"host"`
	SubscriptionName string    `json:"subscription_name"`
	ReceivedLSN      string    `json:"received_lsn"`     // Hex format
	LatestEndLSN     string    `json:"latest_end_lsn"`   // Hex format
	LagBytes         int64     `json:"lag_bytes"`        // Long integer
	LagMilliseconds  float64   `json:"lag_milliseconds"` // Calculated from timestamps
	LastMsgSendTime  time.Time `json:"last_msg_send_time"`
	LastMsgRecvTime  time.Time `json:"last_msg_recv_time"`
}
