package gossip

type MessageType string

const (
	AdvertiseMessage MessageType = "advertise"
)

type GossipMessage struct {
	MsgType MessageType

	Addr         string  `json:",omitempty"`
	ReplicaGroup string  `json:",omitempty"`
	Partitions   []int32 `json:",omitempty"`
}
