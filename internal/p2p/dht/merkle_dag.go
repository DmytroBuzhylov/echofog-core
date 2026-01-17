package dht

import (
	"crypto/sha256"
	"github.com/DmytroBuzhylov/echofog-core/internal/storage"

	"google.golang.org/protobuf/proto"
)

const ChunkSize = 1024 * 256

const dagStorageKey = "dag_storage:"

type MerkleDagStorage struct {
	db storage.Storage
}

func NewMerkleDagStorage(db storage.Storage) *MerkleDagStorage {
	return &MerkleDagStorage{
		db: db,
	}
}

func (m *MerkleDagStorage) saveAndHashData(data []byte) ([]byte, error) {
	hashKey := sha256.Sum256(data)
	key := append([]byte(dagStorageKey), hashKey[:]...)
	err := m.db.Set(key, data)
	if err != nil {
		return nil, err
	}
	return hashKey[:], nil
}

func (m *MerkleDagStorage) getDataByHash(hashedID []byte) ([]byte, error) {
	key := append([]byte(dagStorageKey), hashedID...)
	return m.db.Get(key)
}

func (m *MerkleDagStorage) calculateFileChunk(file []byte) ([]byte, error) {
	if len(file) <= ChunkSize {
		metaLink, err := createMetaLink(file, nil, uint64(len(file)))
		if err != nil {
			return nil, err
		}
		return m.saveAndHashData(metaLink)
	}

	fileChunksKeys := make([][]byte, 0, len(file)/ChunkSize)
	for i := 0; i < len(file); i += ChunkSize {
		end := i + ChunkSize
		if end > len(file) {
			end = len(file)
		}
		chunk, err := createFileChunk(file[i:end], uint64(len(file[i:end])))
		if err != nil {
			return nil, err
		}
		key, err := m.saveAndHashData(chunk)
		if err != nil {
			return nil, err
		}
		fileChunksKeys = append(fileChunksKeys, key)
	}

	metaLink, err := createMetaLink(nil, fileChunksKeys, uint64(len(file)))
	if err != nil {
		return nil, err
	}

	return m.saveAndHashData(metaLink)
}

func (m *MerkleDagStorage) findFileByID(id []byte) ([]byte, error) {
	nodeBytes, err := m.getDataByHash(id)
	if err != nil {
		return nil, err
	}

	var node MerkleNode
	err = proto.Unmarshal(nodeBytes, &node)
	if err != nil {
		return nil, err
	}

	if node.Type == MerkleNode_META_LINK {
		if len(node.Data) > 0 {
			return node.Data, nil
		}

		fullFile := make([]byte, 0, node.Size)

		for _, childKey := range node.Links {
			childBytes, err := m.getDataByHash(childKey)
			if err != nil {
				return nil, err
			}

			var childNode MerkleNode
			proto.Unmarshal(childBytes, &childNode)
			fullFile = append(fullFile, childNode.Data...)
		}
		return fullFile, nil
	}

	return node.Data, nil
}

func (m *MerkleDagStorage) hashExists(hash []byte) bool {
	key := append([]byte(dagStorageKey), hash...)
	return m.db.Exists(key)
}

func createMetaLink(data []byte, links [][]byte, size uint64) ([]byte, error) {
	return proto.Marshal(&MerkleNode{
		Type:  MerkleNode_META_LINK,
		Data:  data,
		Links: links,
		Size:  size,
	})
}

func createFileChunk(data []byte, size uint64) ([]byte, error) {
	return proto.Marshal(&MerkleNode{
		Type: MerkleNode_FILE_CHUNK,
		Data: data,
		Size: size,
	})
}
