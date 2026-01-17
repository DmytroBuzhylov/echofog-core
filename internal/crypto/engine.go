package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"

	"io"

	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/hkdf"
)

func edPrivToX25519(edPriv ed25519.PrivateKey) []byte {
	h := sha512.Sum512(edPriv.Seed())
	out := h[:32]

	out[0] &= 248
	out[31] &= 127
	out[31] |= 64

	return out
}

// EdPubKeyToX25519 Helper: Converts an Ed25519 public key to an X25519 public key
// Input: []byte (32 bytes of Ed25519 Public Key)
// Output: []byte (32 bytes of X25519 Public Key)
func EdPubKeyToX25519(edPub types.PeerPublicKey) ([]byte, error) {
	pt := new(edwards25519.Point)

	if _, err := pt.SetBytes(edPub[:]); err != nil {
		return nil, fmt.Errorf("invalid ed25519 public key point: %w", err)
	}

	return pt.BytesMontgomery(), nil
}

type Engine struct {
	myDiffieHellmanKey *ecdh.PrivateKey
}

// NewEngine takes your identity key (Ed25519) and creates an engine
func NewEngine(identityKey ed25519.PrivateKey) (*Engine, error) {
	x25519Bytes := edPrivToX25519(identityKey)

	priv, err := ecdh.X25519().NewPrivateKey(x25519Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create X25519 private key: %w", err)
	}

	return &Engine{
		myDiffieHellmanKey: priv,
	}, nil
}

func (e *Engine) computeSharedKey(theirEdPub types.PeerPublicKey) ([]byte, error) {
	theirX25519Bytes, err := EdPubKeyToX25519(theirEdPub)
	if err != nil {
		return nil, err
	}

	theirPubKey, err := ecdh.X25519().NewPublicKey(theirX25519Bytes)
	if err != nil {
		return nil, fmt.Errorf("crypto/ecdh rejected the public key: %w", err)
	}

	sharedSecret, err := e.myDiffieHellmanKey.ECDH(theirPubKey)
	if err != nil {
		return nil, fmt.Errorf("ecdh computation failed: %w", err)
	}

	hkdfReader := hkdf.New(sha256.New, sharedSecret, nil, []byte("p2p-messenger-v1-aes-key"))
	aesKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, aesKey); err != nil {
		return nil, err
	}

	return aesKey, nil
}

// Encrypt accepts clear text and the recipient's Ed25519 key
func (e *Engine) Encrypt(plaintext []byte, recipientEdPub types.PeerPublicKey) ([]byte, error) {
	key, err := e.computeSharedKey(recipientEdPub)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// Decrypt accepts the ciphertext and the sender's Ed25519 key
func (e *Engine) Decrypt(ciphertext []byte, senderEdPub types.PeerPublicKey) ([]byte, error) {
	key, err := e.computeSharedKey(senderEdPub)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong key or tampering): %w", err)
	}

	return plaintext, nil
}
