package gossip

import (
	"encoding/json"
	"fmt"
	"github.com/danthegoodman1/Firescroll/gologger"
	"github.com/danthegoodman1/Firescroll/partitions"
	"github.com/danthegoodman1/Firescroll/utils"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

var (
	logger = gologger.NewLogger()
)

type Manager struct {
	// The partition ID
	Node       *Node
	broadcasts *memberlist.TransmitLimitedQueue

	MemberList *memberlist.Memberlist

	PartitionManager *partitions.PartitionManager
}

type Node struct {
	// The partition ID
	ID               string
	AdvertiseAddress string
	AdvertisePort    string
	LastUpdated      time.Time
}

func NewGossipManager(pm *partitions.PartitionManager) (gm *Manager, err error) {
	advertiseHost, advertisePort, err := net.SplitHostPort(utils.Env_AdvertiseAddr)
	if err != nil {
		return nil, fmt.Errorf("error splitting advertise address: %w", err)
	}

	myNode := &Node{
		ID:               utils.Env_InstanceID,
		AdvertiseAddress: advertisePort,
		AdvertisePort:    advertiseHost,
		LastUpdated:      time.Now(),
	}

	gm = &Manager{
		Node:             myNode,
		PartitionManager: pm,
	}

	var config *memberlist.Config
	if strings.Contains(utils.Env_AdvertiseAddr, "localhost") {
		config = memberlist.DefaultLocalConfig()
	} else {
		config = memberlist.DefaultLANConfig()
	}

	config.BindPort = int(utils.Env_GossipPort)
	config.Events = &eventDelegate{
		gm: gm,
	}
	if !utils.Env_GossipDebug {
		config.Logger = nil
	}
	config.Delegate = &delegate{
		GossipManager: gm,
		mu:            &sync.RWMutex{},
		items:         map[string]string{},
	}
	config.Name = utils.Env_InstanceID

	gm.MemberList, err = memberlist.Create(config)
	if err != nil {
		logger.Error().Err(err).Msg("Error creating memberlist")
		return nil, err
	}

	existingMembers := strings.Split(utils.Env_GossipPeers, ",")
	if len(existingMembers) > 0 && existingMembers[0] != "" {
		// Join existing nodes
		joinedHosts, err := gm.MemberList.Join(existingMembers)
		if err != nil {
			return nil, fmt.Errorf("error in MemberList.Join: %w", err)
		}
		logger.Info().Int("joinedHosts", joinedHosts).Msg("Successfully joined gossip cluster")
	} else {
		logger.Info().Msg("Starting new gossip cluster")
	}

	gm.broadcasts = &memberlist.TransmitLimitedQueue{
		NumNodes: func() int {
			return gm.MemberList.NumMembers()
		},
		RetransmitMult: 3,
	}

	node := gm.MemberList.LocalNode()
	logger.Info().Str("name", node.Name).Str("addr", node.Address()).Int("port", int(node.Port)).Msg("Node started")

	gm.broadcastAdvertiseMessage()

	return gm, nil
}

type AdvertiseMessage struct {
	Addr         string
	Partitions   []int32
	ReplicaGroup string
}

func (gm *Manager) broadcastAdvertiseMessage() {
	b, err := json.Marshal(AdvertiseMessage{
		Addr:         utils.Env_AdvertiseAddr,
		Partitions:   gm.PartitionManager.GetPartitionIDs(),
		ReplicaGroup: utils.Env_ReplicaGroupName,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("error marshaling advertise message, exiting")
	}
	gm.broadcasts.QueueBroadcast(&broadcast{
		msg:    b,
		notify: nil,
	})
}

func (gm *Manager) Shutdown() error {
	err := gm.MemberList.Leave(time.Second * 10)
	if err != nil {
		return fmt.Errorf("error in <MemberList.Leave: %w", err)
	}
	err = gm.MemberList.Shutdown()
	if err != nil {
		return fmt.Errorf("error in MemberList.Shutdown: %w", err)
	}
	return nil
}
