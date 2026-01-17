package messenger

import (
	"log"
	"time"

	"github.com/DmytroBuzhylov/echofog-core/internal/crypto"
	"github.com/DmytroBuzhylov/echofog-core/internal/network"
	"github.com/DmytroBuzhylov/echofog-core/internal/p2p/gossip"
	internal_pb "github.com/DmytroBuzhylov/echofog-core/internal/proto"
	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"

	"github.com/google/uuid"
)

type MessageService struct {
	cryptoEngine *crypto.Engine
	storage      storage.Storage
	gsp          *gossip.Manager
	myPrivKey    types.PeerPrivateKey
}

func NewMessageService(myPrivKey types.PeerPrivateKey, cryptoEngine *crypto.Engine, storage storage.Storage, gsp *gossip.Manager) *MessageService {
	return &MessageService{
		cryptoEngine: cryptoEngine,
		storage:      storage,
		gsp:          gsp,
		myPrivKey:    myPrivKey,
	}
}

func (s *MessageService) Handle(msg *internal_pb.MessageData, peerID types.PeerID) {
	encryptedPayload := msg.Payload.(*internal_pb.MessageData_ChatMessage).ChatMessage.GetEncryptedPayload()

	pubKey := types.PeerPublicKey(msg.GetOriginId())
	data, err := s.cryptoEngine.Decrypt(encryptedPayload, pubKey)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("Decrypted data: ", string(data))
}

func (s *MessageService) GetSubscribedTypes() []interface{} {
	return []interface{}{
		(*internal_pb.MessageData_ChatMessage)(nil),
	}
}
func (s *MessageService) Send(toPeerPubKey types.PeerPublicKey, data []byte) error {
	encryptData, err := s.cryptoEngine.Encrypt(data, toPeerPubKey)
	if err != nil {
		return err
	}

	pubKey := types.PeerPrivateKeyToPublic(s.myPrivKey)

	chatMessage := s.PackChatMessage(encryptData, pubKey[:], toPeerPubKey[:])
	s.gsp.Broadcast(network.TypeChatMessage, chatMessage)
	return nil
}

func (s *MessageService) PackChatMessage(encryptedPayload []byte, from []byte, to []byte) *internal_pb.MessageData {
	id := uuid.New()
	return &internal_pb.MessageData{
		MessageId: id[:],
		OriginId:  from,
		TargetId:  to,
		Timestamp: uint64(time.Now().UnixNano()),
		HopLimit:  20,
		Payload: &internal_pb.MessageData_ChatMessage{
			ChatMessage: &internal_pb.ChatMessage{
				EncryptedPayload: encryptedPayload,
			},
		},
	}
}
