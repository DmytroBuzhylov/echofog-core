package gossip

import (
	"sync"

	"github.com/DmytroBuzhylov/echofog-core/internal/network"
	"github.com/DmytroBuzhylov/echofog-core/internal/p2p"
	internal_pb "github.com/DmytroBuzhylov/echofog-core/internal/proto"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
)

type Manager struct {
	swarm *p2p.Swarm

	mu        sync.RWMutex
	seenCache map[types.MessageID]bool
}

func NewManager(swarm *p2p.Swarm) *Manager {
	return &Manager{
		swarm:     swarm,
		seenCache: make(map[types.MessageID]bool),
	}
}

func (g *Manager) Broadcast(msgType network.MessageType, msgData *internal_pb.MessageData) {
	mesID, err := types.ParseMessageID(msgData.GetMessageId())
	if err != nil {
		return
	}
	g.mu.Lock()
	if g.seenCache[mesID] {
		g.mu.Unlock()
		return
	}
	g.seenCache[mesID] = true
	g.mu.Unlock()

	msgData.HopLimit--
	if msgData.HopLimit <= 0 {
		return
	}

	peers := g.swarm.GetAllPeers()

	for _, peer := range peers {

		if peer.ID() == types.PeerID(msgData.OriginId) {
			continue
		}

		go g.swarm.SendDataForPeer(peer.ID(), msgType, msgData)
	}
}

func (g *Manager) HandleIncoming(msgType network.MessageType, msgData *internal_pb.MessageData) {
	mesID, err := types.ParseMessageID(msgData.GetMessageId())
	if err != nil {
		return
	}
	g.mu.RLock()
	seen := g.seenCache[mesID]
	g.mu.RUnlock()

	if seen {
		return
	}

	g.Broadcast(msgType, msgData)
}
