package discovery

import (
	"time"

	"github.com/DmytroBuzhylov/echofog-core/internal/network"
	"github.com/DmytroBuzhylov/echofog-core/internal/p2p"
	"github.com/DmytroBuzhylov/echofog-core/internal/p2p/gossip"
	internal_pb "github.com/DmytroBuzhylov/echofog-core/internal/proto"
	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"

	"github.com/google/uuid"
)

type DiscoveryService struct {
	storage  storage.Storage
	gsp      *gossip.Manager
	swarm    *p2p.Swarm
	getter   *peerGetter
	giver    *peerGiver
	myPubKey types.PeerPublicKey
}

func NewDiscoveryService(storage storage.Storage, gsp *gossip.Manager, swarm *p2p.Swarm, myPubKey types.PeerPublicKey) *DiscoveryService {
	ds := &DiscoveryService{
		storage:  storage,
		gsp:      gsp,
		swarm:    swarm,
		myPubKey: myPubKey,
	}
	var getter = newPeerGetter(ds)
	giver := newPeerGiver(ds)
	ds.getter = getter
	ds.giver = giver

	return ds
}

func (d *DiscoveryService) Handle(msg *internal_pb.MessageData, peerID types.PeerID) {
	switch msg.Payload.(type) {
	case *internal_pb.MessageData_PeerRes:
		d.giver.Handle(msg, peerID)
	case *internal_pb.MessageData_PeerReq:
		d.getter.Handle(msg, peerID)
	}
}

func (d *DiscoveryService) GetSubscribedTypes() []interface{} {
	return []interface{}{
		(*internal_pb.MessageData_PeerReq)(nil),
		(*internal_pb.MessageData_PeerRes)(nil),
	}
}

type peerGetter struct {
	service *DiscoveryService
}

func newPeerGetter(service *DiscoveryService) *peerGetter {
	return &peerGetter{service: service}
}

func (g *peerGetter) Handle(msg *internal_pb.MessageData, peerID types.PeerID) {

}

type peerGiver struct {
	service *DiscoveryService
}

func newPeerGiver(service *DiscoveryService) *peerGiver {
	return &peerGiver{service: service}
}

func (g *peerGiver) Handle(msg *internal_pb.MessageData, peerID types.PeerID) {
	msgPeer, ok := msg.Payload.(*internal_pb.MessageData_PeerReq)
	if !ok {
		return
	}

	peers := g.service.swarm.GetMyRandomPeers(uint(msgPeer.PeerReq.Count))

	peersInfo := make([]*internal_pb.PeerInfo, 0, len(peers))
	for _, p := range peers {
		pubKey := p.PubKey()
		peersInfo = append(peersInfo, &internal_pb.PeerInfo{
			PubKey:  pubKey[:],
			Address: p.Addr(),
		})
	}

	mesID := uuid.New()
	peerResponse := &internal_pb.MessageData{
		MessageId: mesID[:],
		OriginId:  g.service.myPubKey[:],
		TargetId:  msg.GetOriginId(),
		Timestamp: uint64(time.Now().UnixNano()),
		HopLimit:  20,
		Payload: &internal_pb.MessageData_PeerRes{
			PeerRes: &internal_pb.PeerResponse{
				Peers: peersInfo,
			},
		},
	}

	originID := types.PeerID(msg.OriginId)

	if g.service.swarm.ThisIsActivePeer(originID) {
		err := g.service.swarm.SendDataForPeer(originID, network.TypeGetPeerResponse, peerResponse)
		if err == nil {
			return
		}
	}

	g.service.gsp.Broadcast(network.TypeGetPeerResponse, peerResponse)
}
