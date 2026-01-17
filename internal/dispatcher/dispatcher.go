package dispatcher

import (
	"context"

	"log"
	"reflect"
	"runtime"
	"sync"

	"github.com/DmytroBuzhylov/echofog-core/internal/crypto"
	internal_pb "github.com/DmytroBuzhylov/echofog-core/internal/proto"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"

	"google.golang.org/protobuf/proto"
)

type Handler interface {
	Handle(msg *internal_pb.MessageData, peerID types.PeerID)
}

type Dispatcher struct {
	// The channel on which ALL peers send incoming packets
	ingressChan chan IngressPacket

	handlersMu sync.RWMutex
	handlers   map[reflect.Type]Handler // string = payload type

	workersNum int
	ctx        context.Context
	cancel     context.CancelFunc
}

type IngressPacket struct {
	Envelope *internal_pb.Envelope
	PeerID   types.PeerID
}

func NewDispatcher(ctx context.Context) *Dispatcher {
	ctx, cancel := context.WithCancel(ctx)
	workerNum := runtime.NumCPU() - 1
	if workerNum < 0 {
		workerNum = 1
	}
	return &Dispatcher{
		ingressChan: make(chan IngressPacket, 2000),
		handlers:    make(map[reflect.Type]Handler),
		workersNum:  workerNum,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (d *Dispatcher) Registry(payloadProto interface{}, handler Handler) {
	d.handlersMu.Lock()
	defer d.handlersMu.Unlock()
	t := reflect.TypeOf(payloadProto)
	d.handlers[t] = handler
}

// PushMessage calls Peer when it has read something from the network
func (d *Dispatcher) PushMessage(env *internal_pb.Envelope, peerID types.PeerID) {
	select {
	case d.ingressChan <- IngressPacket{
		Envelope: env,
		PeerID:   peerID,
	}:
	default:
		log.Println("Dispatcher queue full, dropping message from", peerID)
	}
}

func (d *Dispatcher) Start() {
	for i := 0; i < d.workersNum; i++ {
		go d.workerLoop()
	}
}

func (d *Dispatcher) Stop() {
	d.cancel()
}

func (d *Dispatcher) workerLoop() {
	select {
	case packet := <-d.ingressChan:
		d.processPacket(packet)
	case <-d.ctx.Done():
		return
	}
}

func (d *Dispatcher) processPacket(packet IngressPacket) {
	env := packet.Envelope
	pubKey := types.PeerPublicKey(env.PubKey)
	if len(pubKey) != 32 || !crypto.VerifySignature(pubKey, env.Data, env.Signature) {
		log.Printf("Security Alert: Invalid signature from peer!")
		return
	}

	var msgData internal_pb.MessageData
	if err := proto.Unmarshal(env.Data, &msgData); err != nil {
		log.Printf("Failed to unmarshal MessageData: %v", err)
		return
	}

	d.route(&msgData, packet.PeerID)
}

func (d *Dispatcher) route(msg *internal_pb.MessageData, fromPeer types.PeerID) {
	payloadType := reflect.TypeOf(msg.Payload)

	d.handlersMu.RLock()
	handler, ok := d.handlers[payloadType]
	d.handlersMu.RUnlock()

	if ok {
		handler.Handle(msg, fromPeer)
	} else {
		log.Printf("No handler registered for type: %v", payloadType)
	}
}
