package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"time"

	"github.com/DmytroBuzhylov/echofog-core/internal/storage"

	"github.com/dgraph-io/badger/v4"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"google.golang.org/protobuf/proto"
)

//type NodeKeys struct {
//	PrivateKey string `json:"private_key"`
//	PublicKey  string `json:"public_key"`
//}

type SecureKeyStore struct {
	Password []byte
	storage  storage.Storage
}

func NewSecureKeyStore(pass []byte, s storage.Storage) *SecureKeyStore {
	return &SecureKeyStore{
		Password: pass,
		storage:  s,
	}
}

func (s *SecureKeyStore) encryptPrivateKey(privKey ed25519.PrivateKey) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	key := argon2.IDKey(s.Password, salt, 1, 64*1024, 4, 32)

	aead, _ := chacha20poly1305.NewX(key)
	nonce := make([]byte, aead.NonceSize())
	io.ReadFull(rand.Reader, nonce)

	cipherText := aead.Seal(nil, nonce, privKey, nil)
	protoKeys := &EncryptedPrivateKey{
		Salt:         salt,
		Nonce:        nonce,
		EncryptedKey: cipherText,
		Algorithm:    "argon2id-chacha20",
	}
	return proto.Marshal(protoKeys)
}

func (s *SecureKeyStore) decryptPrivateKey(encryptedData []byte) (ed25519.PrivateKey, error) {
	var protoData EncryptedPrivateKey
	err := proto.Unmarshal(encryptedData, &protoData)
	if err != nil {
		return nil, err
	}

	salt := protoData.GetSalt()
	nonce := protoData.GetNonce()
	actualCipher := protoData.GetEncryptedKey()

	key := argon2.IDKey(s.Password, salt, 1, 64*1024, 4, 32)

	aead, _ := chacha20poly1305.NewX(key)

	decrypted, err := aead.Open(nil, nonce, actualCipher, nil)
	if err != nil {
		return nil, errors.New("decryption error (wrong password?)")
	}

	return ed25519.PrivateKey(decrypted), nil
}

// GetOrGenerateKeys loads keys from a file or creates new ones
func (s *SecureKeyStore) GetOrGenerateKeys() (ed25519.PrivateKey, error) {

	keysBytes, err := s.storage.Get([]byte("secret:keys"))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			_, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return nil, err
			}

			keyData, err := s.encryptPrivateKey(priv)
			if err != nil {
				return nil, err
			}

			err = s.storage.Set([]byte("secret:keys"), keyData)
			if err != nil {
				return nil, err
			}

			return priv, nil
		} else {
			log.Println(err)
			return nil, err
		}
	}

	return s.decryptPrivateKey(keysBytes)
}

// GenerateTLSConfig creates a config for QUIC/TLS based on the Identity key
func GenerateTLSConfig(priv ed25519.PrivateKey) (*tls.Config, error) {
	pub := priv.Public().(ed25519.PublicKey)
	pubHex := hex.EncodeToString(pub)

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   pubHex,
			Organization: []string{"P2P-Gossip-Messenger"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 365 * 10),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, priv)
	if err != nil {
		return nil, err
	}

	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{tlsCert},
		NextProtos:         []string{"my-gossip-protocol"},
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return nil
		},
		ClientSessionCache: tls.NewLRUClientSessionCache(100),
	}, nil
}

func decodeKey(hexStr string) (ed25519.PrivateKey, error) {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	if len(data) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key size")
	}
	return ed25519.PrivateKey(data), nil
}

func GetPeerID(privKey ed25519.PrivateKey) string {
	pubKey := privKey.Public()

	edPub := pubKey.(ed25519.PublicKey)

	return hex.EncodeToString(edPub)
}
