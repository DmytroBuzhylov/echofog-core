package types

import (
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
)

type PeerID [32]byte
type SessionID [16]byte
type MessageID [16]byte
type ContentHash [32]byte
type PeerPublicKey [32]byte
type PeerPrivateKey [64]byte

func ToPeerID(b []byte) (PeerID, error) {
	var id PeerID
	if len(b) != 32 {
		return id, fmt.Errorf("wrong length: %d", len(b))
	}
	copy(id[:], b)
	return id, nil
}

func PeerPubKeyToID(pubKey PeerPublicKey) PeerID {
	return sha256.Sum256(pubKey[:])
}

func PeerPrivateKeyToPublic(privKey PeerPrivateKey) PeerPublicKey {
	key := privKey[32:]
	var pubKey PeerPublicKey
	copy(pubKey[:], key)
	return pubKey
}

func ParseMessageID(data []byte) (MessageID, error) {
	bytes, err := uuid.FromBytes(data)
	if err != nil {
		return [16]byte{}, err
	}

	return MessageID(bytes), nil
}
