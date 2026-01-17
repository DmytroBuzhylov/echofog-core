package dht

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
)

type DHT struct {
	storage      storage.Storage
	myPubKey     types.PeerPublicKey
	myID         types.PeerID
	routingTable *RoutingTable
	merkle       *MerkleDagStorage
}

func NewDHT(storage storage.Storage, myPubKey types.PeerPublicKey) *DHT {
	myID := sha256.Sum256(myPubKey[:])
	routingTable := NewRoutingTable(myPubKey, storage)
	merkle := NewMerkleDagStorage(storage)

	return &DHT{
		storage:      storage,
		myPubKey:     myPubKey,
		myID:         myID,
		routingTable: routingTable,
		merkle:       merkle,
	}
}

func (d *DHT) GetNearPeers(peerID [32]byte) ([]*PeerStoreEntry, error) {
	index := getBucketIndex(d.myID, peerID)
	return d.routingTable.GetPeers(index)
}

func (d *DHT) SavePeer(peerID [32]byte, PubKey ed25519.PublicKey, addr string, lastSeen uint64, trustScore uint32) error {
	index := getBucketIndex(d.myID, peerID)
	peer := &PeerStoreEntry{
		PubKey:        PubKey,
		HashId:        peerID[:],
		LastKnownAddr: addr,
		LastSeen:      lastSeen,
		TrustScore:    trustScore,
	}
	return d.routingTable.saveInKBucket(index, peer)
}

func (d *DHT) SaveFile(file []byte) (metaLinkKey []byte, err error) {
	hash := sha256.Sum256(file)
	if d.merkle.hashExists(hash[:]) {
		return nil, nil
	}

	metaLinkKey, err = d.merkle.calculateFileChunk(file)
	if err != nil {
		return nil, err
	}
	return metaLinkKey, nil
}

func (d *DHT) GetFileChunk(chunkHash []byte) ([]byte, error) {
	return d.merkle.findFileByID(chunkHash)
}

func (d *DHT) CalculateDistance(peerID [32]byte) int {
	return getBucketIndex(d.myID, peerID)
}
