package content

import (
	"github.com/DmytroBuzhylov/echofog-core/internal/p2p"
	"github.com/DmytroBuzhylov/echofog-core/internal/p2p/dht"
	internal_pb "github.com/DmytroBuzhylov/echofog-core/internal/proto"
	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
)

type ContentService struct {
	db    storage.Storage
	swarm *p2p.Swarm
	dht   *dht.DHT
}

func (c *ContentService) GetSubscribedTypes() []interface{} {
	//TODO implement me
	panic("implement me")
}

func (c *ContentService) Handle(msg *internal_pb.MessageData, peerID types.PeerPublicKey) {
	//TODO implement me
	panic("implement me")
}

func NewContentService(db storage.Storage, swarm *p2p.Swarm, dht *dht.DHT) *ContentService {
	return &ContentService{
		db:    db,
		swarm: swarm,
		dht:   dht,
	}
}

func (c *ContentService) DownloadFile(fileHash types.ContentHash, peerID types.PeerPublicKey) {
	c.swarm.GetSessionManager()
}
