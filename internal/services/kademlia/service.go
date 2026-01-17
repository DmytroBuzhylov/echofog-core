package kademlia

import (
	internal_pb "github.com/DmytroBuzhylov/echofog-core/internal/proto"
	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
)

type KademliaService struct {
	db storage.Storage
}

func (k KademliaService) Handle(msg *internal_pb.MessageData, peerID types.PeerID) {
	//TODO implement me
	panic("implement me")
}

func (k KademliaService) GetSubscribedTypes() []interface{} {
	//TODO implement me
	panic("implement me")
}

func NewKademliaService() *KademliaService {
	return &KademliaService{}
}
