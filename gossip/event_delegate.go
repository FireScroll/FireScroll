package gossip

import (
	"github.com/hashicorp/memberlist"
)

type eventDelegate struct {
	gm *Manager
}

func (ed *eventDelegate) NotifyJoin(node *memberlist.Node) {
	logger.Debug().Msg("A node has joined: " + node.String())
	if ed.gm.broadcasts != nil {
		// Broadcast out advertise address and port
		go ed.gm.broadcastAdvertiseMessage()
	}
}

func (ed *eventDelegate) NotifyLeave(node *memberlist.Node) {
	logger.Debug().Msg("A node has left: " + node.Name)
	if node.Name != ed.gm.Node.ID {
		go ed.gm.removePartitionsForAddr(node.Name)
	}
}

func (ed *eventDelegate) NotifyUpdate(node *memberlist.Node) {
	logger.Trace().Msg("A node was updated: " + node.String())
}
