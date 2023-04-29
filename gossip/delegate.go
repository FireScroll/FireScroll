package gossip

import (
	"encoding/json"
	"sync"
)

type delegate struct {
	GossipManager *Manager
	mu            *sync.RWMutex
	items         map[string]string
}

func (d *delegate) NodeMeta(limit int) []byte {
	return []byte{}
}

func (d *delegate) NotifyMsg(b []byte) {
	go handleDelegateMsg(d, b)
}

func (d *delegate) GetBroadcasts(overhead, limit int) [][]byte {
	return d.GossipManager.broadcasts.GetBroadcasts(overhead, limit)
}

func (d *delegate) LocalState(join bool) []byte {
	d.mu.RLock()
	m := d.items
	d.mu.RUnlock()
	b, _ := json.Marshal(m)
	return b
}

func (d *delegate) MergeRemoteState(buf []byte, join bool) {
	logger.Debug().Msg("merging remote state")
	if len(buf) == 0 {
		return
	}
	if !join {
		return
	}
	var m map[string]string
	if err := json.Unmarshal(buf, &m); err != nil {
		return
	}
	d.mu.Lock()
	for k, v := range m {
		d.items[k] = v
	}
	d.mu.Unlock()
}

func handleDelegateMsg(d *delegate, b []byte) {
	logger.Warn().Str("nodeID", d.GossipManager.Node.ID).Str("broadcast", string(b)).Msg("Got msg")
	if len(b) == 0 {
		return
	}

	//dec := msgpack.NewDecoder(bytes.NewBuffer(b))
	//msgType, err := dec.Query("Type")
	//if err != nil {
	//	logger.Error().Err(err).Str("partition", d.GossipManager.UltraQ.Partition).Msg("failed to get delegate msg 'Type'")
	//	return
	//}
	//
	//msgTypeStr, ok := msgType[0].(string)
	//if !ok {
	//	logger.Error().Err(err).Str("partition", d.GossipManager.UltraQ.Partition).Interface("msgType", msgType).Msg("failed to parse msg type to string")
	//	return
	//}
	//
	//// TODO: Handle duplicate messages from gossip so we don't triple process them?
	//switch msgTypeStr {
	//case "ptlu":
	//	var ptlu *PartitionTopicLengthUpdate
	//	err := msgpack.Unmarshal(b, &ptlu)
	//	if err != nil {
	//		logger.Error().Err(err).Str("partition", d.GossipManager.UltraQ.Partition).Msg("failed to unmarshal msgpack")
	//		return
	//	}
	//	// TODO: Remove log line
	//	logger.Debug().Str("partition", d.GossipManager.UltraQ.Partition).Interface("ptlu", ptlu).Msg("unpacked partition topic length update")
	//	go d.GossipManager.putIndexRemotePartitionTopicLength(ptlu.Partition, ptlu.Topic, ptlu.Length)
	//
	//case "paa":
	//	var paa *PartitionAddressAdvertise
	//	err := msgpack.Unmarshal(b, &paa)
	//	if err != nil {
	//		logger.Error().Err(err).Str("partition", d.GossipManager.UltraQ.Partition).Msg("failed to unmarshal msgpack")
	//		return
	//	}
	//	// TODO: Remove log line
	//	logger.Debug().Str("partition", d.GossipManager.UltraQ.Partition).Interface("paa", paa).Msg("unpacked partition advertise address message")
	//	if paa.Partition == d.GossipManager.UltraQ.Partition {
	//		logger.Warn().Str("partition", d.GossipManager.UltraQ.Partition).Msg("Got paa for self, ignoring")
	//		return
	//	}
	//	d.GossipManager.PartitionIndexMu.Lock()
	//	defer d.GossipManager.PartitionIndexMu.Unlock()
	//	d.GossipManager.PartitionIndex[paa.Partition] = &Node{
	//		ID:           paa.Partition,
	//		AdvertiseAddress: paa.Address,
	//		AdvertisePort:    paa.Port,
	//		LastUpdated:      time.Now(),
	//	}
	//
	//default:
	//	logger.Error().Err(err).Str("partition", d.GossipManager.UltraQ.Partition).Str("msgType", msgTypeStr).Msg("unknown message type")
	//	return
	//}
}
