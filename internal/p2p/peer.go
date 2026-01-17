package p2p

import (
	"context"
	"crypto/sha256"

	"log"
	"sync"

	"github.com/DmytroBuzhylov/echofog-core/internal/dispatcher"
	"github.com/DmytroBuzhylov/echofog-core/internal/network"
	internal_pb "github.com/DmytroBuzhylov/echofog-core/internal/proto"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"

	"google.golang.org/protobuf/proto"
)

type Peer struct {
	transport *network.PeerWrapper
	id        types.PeerID // this sha256 from ed25519 pub key
	pubKey    types.PeerPublicKey
	addr      string
	isOut     bool

	sendChan chan *internal_pb.Envelope
	ctx      context.Context
	cancel   context.CancelFunc

	mu      sync.RWMutex
	isReady bool

	dispatcher *dispatcher.Dispatcher

	//handshakeSignal chan struct{}
	//handshakeOnce   sync.Once
}

func NewPeer(peerPubKey types.PeerPublicKey, dispatcher *dispatcher.Dispatcher, addr string, isOut bool) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	hashID := sha256.Sum256(peerPubKey[:])

	return &Peer{
		id:         hashID,
		pubKey:     peerPubKey,
		addr:       addr,
		isOut:      isOut,
		sendChan:   make(chan *internal_pb.Envelope, 100),
		ctx:        ctx,
		cancel:     cancel,
		dispatcher: dispatcher,
	}
}

func (p *Peer) Close() {
	p.cancel()
}

func (p *Peer) SetTransport(transport *network.PeerWrapper) {
	p.transport = transport
	p.addr = transport.RemoteAddr().String()
}

func (p *Peer) ID() types.PeerID {
	return p.id
}

func (p *Peer) PubKey() types.PeerPublicKey {
	return p.pubKey
}

func (p *Peer) Addr() string {
	return p.addr
}

func (p *Peer) Send(msgType network.MessageType, msgData *internal_pb.Envelope) error {
	data, err := proto.Marshal(msgData)
	if err != nil {
		log.Println(err)
		return err
	}

	return p.transport.SendGossipMessage(msgType, data)
}

func (p *Peer) readLoop() {

}

func (p *Peer) waitForResponse() {

}

func (p *Peer) handleIncomingConnection() {

}
