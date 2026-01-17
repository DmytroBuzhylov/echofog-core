package network

import (
	"errors"

	"github.com/DmytroBuzhylov/echofog-core/internal/crypto"
	internal_pb "github.com/DmytroBuzhylov/echofog-core/internal/proto"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

func sendHandshake(stream *quic.Stream, nonce []byte) error {
	//nonce := make([]byte, 32)
	//rand.Read(nonce)

	hs := &internal_pb.MessageData{
		Payload: &internal_pb.MessageData_HandshakeInit{
			HandshakeInit: &internal_pb.HandshakeInit{Nonce: nonce},
		},
	}
	data, err := proto.Marshal(hs)
	if err != nil {
		return err
	}

	return writeFrame(stream, TypeHandshake, data)
}

func checkHandshakeResponse(stream *quic.Stream, nonce []byte) (types.PeerPublicKey, error) {

	msgType, protoData, err := readFrame(stream)
	if err != nil {
		return types.PeerPublicKey{}, err
	}
	if msgType != TypeHandshake {
		return types.PeerPublicKey{}, errors.New("message type is not handshake")
	}

	var data internal_pb.MessageData
	err = proto.Unmarshal(protoData, &data)
	if err != nil {
		return types.PeerPublicKey{}, err
	}

	switch msg := data.Payload.(type) {
	case *internal_pb.MessageData_HandshakeResponse:
		dataForVerify := append(nonce, msg.HandshakeResponse.GetPubKey()...)
		pubKey := types.PeerPublicKey(msg.HandshakeResponse.GetPubKey())

		ok := crypto.VerifySignature(pubKey, dataForVerify, msg.HandshakeResponse.GetSignature())
		if !ok {
			return types.PeerPublicKey{}, errors.New("invalid signature")
		}

		return types.PeerPublicKey(msg.HandshakeResponse.GetPubKey()), nil
	default:
		return types.PeerPublicKey{}, errors.New("invalid proto type")
	}

}

func acceptHandshake(stream *quic.Stream, privKey types.PeerPrivateKey, version uint32) error {
	msgType, protoData, err := readFrame(stream)
	if err != nil {
		return err
	}
	if msgType != TypeHandshake {
		return errors.New("message type is not handshake")
	}

	var data internal_pb.MessageData
	err = proto.Unmarshal(protoData, &data)
	if err != nil {
		return err
	}
	hs, ok := data.Payload.(*internal_pb.MessageData_HandshakeInit)
	if !ok {
		return errors.New("invalid proto type")
	}

	myPubKey := types.PeerPrivateKeyToPublic(privKey)

	dataForSiganature := append(hs.HandshakeInit.GetNonce(), myPubKey[:]...)

	signature := crypto.CreateSignature(dataForSiganature, privKey[:])

	hsResp := &internal_pb.MessageData{
		Payload: &internal_pb.MessageData_HandshakeResponse{
			HandshakeResponse: &internal_pb.HandshakeResponse{
				PubKey:    myPubKey[:],
				Signature: signature,
				Version:   version,
			},
		},
	}

	bytes, err := proto.Marshal(hsResp)
	if err != nil {
		return err
	}
	return writeFrame(stream, TypeHandshake, bytes)
}
