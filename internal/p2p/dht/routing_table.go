package dht

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
	"github.com/dgraph-io/badger/v4"
	"google.golang.org/protobuf/proto"
)

const MaxBucketSize = 20

type RoutingTable struct {
	myPubKey types.PeerPublicKey
	myID     types.PeerID
	storage  storage.Storage
}

func NewRoutingTable(myPubKey types.PeerPublicKey, storage storage.Storage) *RoutingTable {
	hashID := sha256.Sum256(myPubKey[:])

	return &RoutingTable{
		myPubKey: myPubKey,
		myID:     hashID,
		storage:  storage,
	}
}

func (r *RoutingTable) GetPeers(index int) ([]*PeerStoreEntry, error) {
	fromStorage, err := r.getFromStorage(index)
	if err != nil {
		return nil, err
	}
	var bucket KBucket
	err = proto.Unmarshal(fromStorage, &bucket)
	if err != nil {
		return nil, err
	}
	return bucket.GetPeers(), nil
}

func (r *RoutingTable) saveInKBucket(index int, peer *PeerStoreEntry) error {
	kBucketProto := &KBucket{
		Index: int32(index),
	}

	bucket, err := r.getFromStorage(index)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			peers := make([]*PeerStoreEntry, 0, 1)
			peers = append(peers, peer)
			kBucketProto.Peers = peers
			data, err := proto.Marshal(kBucketProto)
			if err != nil {
				return err
			}
			return r.setInStorage(index, data)
		}
		return err
	}
	err = proto.Unmarshal(bucket, kBucketProto)
	if err != nil {
		return err
	}
	if len(kBucketProto.GetPeers()) >= MaxBucketSize {
		return errors.New("bucket cap")
	}
	kBucketProto.Peers = append(kBucketProto.Peers, peer)
	marshalBucket, err := proto.Marshal(kBucketProto)
	if err != nil {
		return err
	}

	return r.setInStorage(index, marshalBucket)
}

func (r *RoutingTable) setInStorage(index int, data []byte) error {
	return r.storage.Set(intToKey("routing_table:", index), data)
}

func (r *RoutingTable) getFromStorage(index int) ([]byte, error) {
	return r.storage.Get(intToKey("routing_table:", index))
}

func intToKey(prefix string, index int) []byte {
	buf := make([]byte, len(prefix)+8)

	copy(buf, prefix)

	binary.BigEndian.PutUint64(buf[len(prefix):], uint64(index))

	return buf
}

func keyToInt(key []byte, prefixLen int) int {
	numBytes := key[prefixLen:]
	return int(binary.BigEndian.Uint64(numBytes))
}
