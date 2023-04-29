package gossip

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/danthegoodman1/Firescroll/gologger"
	"github.com/danthegoodman1/Firescroll/partitions"
	"github.com/danthegoodman1/Firescroll/utils"
	"github.com/samber/lo"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

var (
	logger = gologger.NewLogger()

	ErrNoRemotePartitions = errors.New("no remote partitions")
)

type Manager struct {
	// The partition ID
	Node       *Node
	broadcasts *memberlist.TransmitLimitedQueue

	MemberList *memberlist.Memberlist

	PartitionManager *partitions.PartitionManager

	// Map of remote addresses for a given partition
	remotePartitions map[int32][]string
	remotePartMu     *sync.RWMutex

	broadcastTicker *time.Ticker
	closeChan       chan struct{}
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
		ID:               utils.Env_AdvertiseAddr,
		AdvertiseAddress: advertisePort,
		AdvertisePort:    advertiseHost,
		LastUpdated:      time.Now(),
	}

	gm = &Manager{
		Node:             myNode,
		PartitionManager: pm,
		closeChan:        make(chan struct{}, 1),
		broadcastTicker:  time.NewTicker(time.Millisecond * time.Duration(utils.Env_GossipBroadcastMS)),
		remotePartitions: map[int32][]string{},
		remotePartMu:     &sync.RWMutex{},
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
		config.LogOutput = VoidWriter{}
	}
	config.Delegate = &delegate{
		GossipManager: gm,
		mu:            &sync.RWMutex{},
		items:         map[string]string{},
	}
	config.Name = utils.Env_AdvertiseAddr

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
	go gm.startBroadcastLoop()

	return gm, nil
}

func (gm *Manager) broadcastAdvertiseMessage() {
	b, err := json.Marshal(GossipMessage{
		Addr:         utils.Env_AdvertiseAddr,
		Partitions:   gm.PartitionManager.GetPartitionIDs(),
		ReplicaGroup: utils.Env_ReplicaGroupName,
		MsgType:      AdvertiseMessage,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("error marshaling advertise message, exiting")
	}
	gm.broadcasts.QueueBroadcast(&broadcast{
		msg:    b,
		notify: nil,
	})
}

func (gm *Manager) startBroadcastLoop() {
	logger.Debug().Msg("starting broadcast loop")
	for {
		select {
		case <-gm.broadcastTicker.C:
			gm.broadcastAdvertiseMessage()
		case <-gm.closeChan:
			logger.Debug().Msg("broadcast ticker received on close channel, exiting")
			return
		}
	}
}

func (gm *Manager) Shutdown() error {
	gm.broadcastTicker.Stop()
	gm.closeChan <- struct{}{}
	err := gm.MemberList.Leave(time.Second * 10)
	if err != nil {
		return fmt.Errorf("error in MemberList.Leave: %w", err)
	}
	err = gm.MemberList.Shutdown()
	if err != nil {
		return fmt.Errorf("error in MemberList.Shutdown: %w", err)
	}
	return nil
}

// checkForPartitionDifference will find new and removed partitions for a known address with a read lock to
// reduce lock contention. partitions can be removed and added separately
func (gm *Manager) checkForPartitionDifference(partitions []int32, addr string) (newPartitions []int32, removedPartitions []int32) {
	gm.remotePartMu.RLock()
	defer gm.remotePartMu.RUnlock()

	// Find new partitions
	for partition, addrs := range gm.remotePartitions {
		thinkAddrHasPart := lo.Contains(addrs, addr)
		addrHasPart := lo.Contains(partitions, partition)

		if thinkAddrHasPart && !addrHasPart {
			removedPartitions = append(removedPartitions, partition)
		} else if !thinkAddrHasPart && addrHasPart {
			newPartitions = append(newPartitions, partition)
		}
	}
	for _, partition := range partitions {
		if _, exists := gm.remotePartitions[partition]; !exists {
			newPartitions = append(newPartitions, partition)
		}
	}
	return
}

func (gm *Manager) addRemotePartitions(partitions []int32, addr string) {
	logger.Debug().Msgf("adding remote partitions %+v at %s", partitions, addr)
	gm.remotePartMu.Lock()
	defer gm.remotePartMu.Unlock()
	for _, partition := range partitions {
		if _, exists := gm.remotePartitions[partition]; !exists {
			gm.remotePartitions[partition] = []string{addr}
			return
		}
		gm.remotePartitions[partition] = append(gm.remotePartitions[partition], addr)
	}
}

func (gm *Manager) removeRemotePartitions(partitions []int32, addr string) {
	logger.Debug().Msgf("removing remote partition %+v at %s", partitions, addr)
	gm.remotePartMu.Lock()
	defer gm.remotePartMu.Unlock()
	for _, partition := range partitions {
		if _, exists := gm.remotePartitions[partition]; !exists {
			return
		}
		gm.remotePartitions[partition] = lo.Filter(gm.remotePartitions[partition], func(item string, _ int) bool {
			return item != addr
		})
	}
}

func (gm *Manager) removePartitionsForAddr(addr string) {
	logger.Debug().Msgf("removing all partitions at %s", addr)
	gm.remotePartMu.Lock()
	defer gm.remotePartMu.Unlock()
	// not sure if we can modify a map in place so being safe
	var parts []int32
	for part, addrs := range gm.remotePartitions {
		if lo.Contains(addrs, addr) {
			parts = append(parts, part)
		}
	}
	for _, part := range parts {
		gm.remotePartitions[part] = lo.Filter(gm.remotePartitions[part], func(item string, _ int) bool {
			return item != addr
		})
	}
}

func (gm *Manager) getRemotePartitions(partition int32) []string {
	gm.remotePartMu.RLock()
	defer gm.remotePartMu.RUnlock()
	return gm.remotePartitions[partition]
}

// GetRandomRemotePartition randomly select an address for the partition
func (gm *Manager) GetRandomRemotePartition(partition int32) (string, error) {
	gm.remotePartMu.RLock()
	defer gm.remotePartMu.RUnlock()
	remoteParts := gm.getRemotePartitions(partition)
	if remoteParts == nil {
		return "", ErrNoRemotePartitions
	}
	randInd := rand.Intn(len(remoteParts))
	return remoteParts[randInd], nil
}
