package crypto

import (
	"crypto/ed25519"

	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
)

func CreateSignature(payload []byte, privKey ed25519.PrivateKey) []byte {
	return ed25519.Sign(ed25519.PrivateKey(privKey[:]), payload)
}

func VerifySignature(pubKey types.PeerPublicKey, payload []byte, sig []byte) bool {
	if len(sig) != ed25519.SignatureSize {
		return false
	}

	return ed25519.Verify(pubKey[:], payload, sig)
}
