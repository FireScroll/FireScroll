package gossip

import (
	"github.com/hashicorp/memberlist"
)

type eventDelegate struct {
	gm *Manager
}

func (ed *eventDelegate) NotifyJoin(node *memberlist.Node) {
	logger.Warn().Msg("A node has joined: " + node.String())
	if ed.gm.broadcasts != nil {
		// Broadcast out advertise address and port
		//go ed.gm.broadcastAdvertiseAddress()
	}
}

func (ed *eventDelegate) NotifyLeave(node *memberlist.Node) {
	logger.Warn().Msg("A node has left: " + node.Name)
	if node.Name != ed.gm.Node.ID {
		//go ed.gm.deletePartitionFromIndex(node.Name)
		//go ed.gm.deletePartitionFromTopicIndex(node.Name)
	}
}

func (ed *eventDelegate) NotifyUpdate(node *memberlist.Node) {
	logger.Warn().Msg("A node was updated: " + node.String())
}
