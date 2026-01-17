package p2p

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	//"github.com/DmytroBuzhylov/echofog-core/api/proto"
	"github.com/DmytroBuzhylov/echofog-core/internal/config"
	"github.com/DmytroBuzhylov/echofog-core/internal/crypto"
	"github.com/DmytroBuzhylov/echofog-core/internal/dispatcher"
	"github.com/DmytroBuzhylov/echofog-core/internal/network"
	internal_pb "github.com/DmytroBuzhylov/echofog-core/internal/proto"
	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/proto"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
	//"github.com/DmytroBuzhylov/echofog-core/pkg/api/proto"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

type Swarm struct {
	mu          sync.RWMutex
	activePeers map[types.PeerID]*Peer

	dispatcher *dispatcher.Dispatcher
	storage    storage.Storage

	selfID       types.PeerID // this sha256 from ed25519 pub key
	myPrivKey    types.PeerPrivateKey
	netTransport network.QuicTransport

	sessionManager *SessionManager

	cfg *config.AppConfig
}

func NewSwarm(selfID types.PeerID, privKey types.PeerPrivateKey, d *dispatcher.Dispatcher, connChan <-chan network.NewConnEvent, storage storage.Storage, cfg *config.AppConfig) *Swarm {
	s := &Swarm{
		activePeers:    make(map[types.PeerID]*Peer),
		dispatcher:     d,
		selfID:         selfID,
		storage:        storage,
		cfg:            cfg,
		sessionManager: NewSessionManager(),
		myPrivKey:      privKey,
	}

	go s.registrationLoop(connChan)

	return s
}

func (s *Swarm) registrationLoop(ch <-chan network.NewConnEvent) {
	for event := range ch {
		p := s.AddPeer(event.PeerPubKey, event.PeerID, event.Conn, event.Addr, event.IsOut)

		go p.transport.StartLoops()
	}
}

func (s *Swarm) findAndConnectToPeers() {
	go func() {
		for _, peer := range s.GetHistoryConnected(0) {

			hashPeerID := sha256.Sum256(peer.PubKey)

			s.mu.RLock()
			if len(s.activePeers) > s.cfg.Network.MaxConnections {
				break
			}
			_, ok := s.activePeers[hashPeerID]
			s.mu.RUnlock()
			if ok {
				continue
			}

			go s.connect(peer.GetLastKnownAddr())
		}

		s.mu.RLock()
		if len(s.activePeers) > s.cfg.Network.MaxConnections {
			return
		}
		s.mu.RUnlock()

	}()
}

func (s *Swarm) AddPeer(peerPubKey types.PeerPublicKey, peerID types.PeerID, conn *quic.Conn, addr string, isOut bool) *Peer {

	s.mu.Lock()
	if old, exists := s.activePeers[peerID]; exists {
		old.Close()
	}
	s.mu.Unlock()

	p := NewPeer(peerPubKey, s.dispatcher, addr, isOut)
	pw := network.NewPeerWrapper(p.ctx, conn)

	go s.SavePeer(peerPubKey, p.addr, 100)

	pw.OnData(func(msgType network.MessageType, payload []byte, peerID types.PeerID) {

		var data internal_pb.Envelope
		err := proto.Unmarshal(payload, &data)
		if err != nil {
			return
		}

		s.dispatcher.PushMessage(&data, peerID)
	})
	pw.OnNewStream(func(stream *network.Stream) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			select {
			case ch := <-stream.ReadCh():
				if ch.Type == network.TypeStreamInitRequest {
					var msg api_pb.ContentMessage
					if err := proto.Unmarshal(ch.Payload, &msg); err != nil {
						stream.Close()
						return
					}

					req, ok := msg.Payload.(*api_pb.ContentMessage_ChunkReq)
					if !ok {
						return
					}

					sessionID, err := uuid.ParseBytes(req.ChunkReq.GetSessionId())
					if err != nil {
						return
					}
					if !s.sessionManager.AddStream(types.SessionID(sessionID), stream) {
						err = errors.New("add stream error")
						return
					}
				}
				stream.Close()
			case <-ctx.Done():
				stream.Close()
			}
		}()
	})

	p.SetTransport(pw)

	s.mu.Lock()
	s.activePeers[peerID] = p
	s.mu.Unlock()

	return p
}

func (s *Swarm) RemovePeer(peerID types.PeerID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.activePeers[peerID]; ok {
		p.Close()
		delete(s.activePeers, peerID)
	}
}

func (s *Swarm) GetPeer(peerID types.PeerID) *Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activePeers[peerID]
}

func (s *Swarm) GetAllPeers() []*Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*Peer, 0, len(s.activePeers))
	for _, p := range s.activePeers {
		res = append(res, p)
	}
	return res
}

func (s *Swarm) ThisIsActivePeer(peerID types.PeerID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.activePeers[peerID]
	return ok
}

func (s *Swarm) GetMyRandomPeers(count uint) []*Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*Peer, 0, len(s.activePeers))

	var mapCount uint

	for _, p := range s.activePeers {
		if mapCount >= count {
			break
		}

		res = append(res, p)
		mapCount++
	}
	return res
}

func (s *Swarm) BanPeer(peerID types.PeerID) {
	key := append([]byte("bans:peer:"), peerID[:]...)

	s.storage.Set(key, []byte("true"))

	s.mu.RLock()
	peer, ok := s.activePeers[peerID]
	s.mu.RUnlock()
	if !ok || peer == nil {
		return
	}

	peer.Close()
}

func (s *Swarm) UnBanPeer(peerID types.PeerID) {
	key := append([]byte("bans:peer:"), peerID[:]...)
	s.storage.Delete(key)
}

func (s *Swarm) CheckOnBan(peerID types.PeerID) bool {
	key := append([]byte("bans:peer:"), peerID[:]...)
	_, err := s.storage.Get(key)
	if err != nil {
		return true
	}
	return false
}

func (s *Swarm) SavePeer(peerPubKey types.PeerPublicKey, addr string, trustScore uint32) {

	data := &storage.PeerStoreEntry{
		PubKey:        peerPubKey[:],
		LastKnownAddr: addr,
		LastSeen:      uint64(time.Now().UnixNano()),
		TrustScore:    trustScore,
	}
	protoData, err := proto.Marshal(data)
	if err != nil {
		return
	}

	hash := sha256.Sum256(peerPubKey[:])

	prefix := []byte("saved:peers:")
	key := make([]byte, len(prefix)+len(hash))
	copy(key, prefix)
	copy(key[len(prefix):], hash[:])

	s.storage.Set(key, protoData)
}

func (s *Swarm) connect(addr string) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
	s.netTransport.DialEarly(ctx, addr)
}

func (s *Swarm) SendDataForPeer(peerID types.PeerID, msgType network.MessageType, data *internal_pb.MessageData) error {
	peer := s.GetPeer(peerID)
	if peer == nil {
		return errors.New("this peer is not connected")
	}

	env, err := s.signMessageData(data)
	if err != nil {
		return err
	}

	return peer.Send(msgType, env)
}

// GetHistoryConnected Set to 0 to get all peers
func (s *Swarm) GetHistoryConnected(count uint) []storage.PeerStoreEntry {
	findValues, err := s.storage.FindValues([]byte("saved:peers:"))
	if err != nil {
		return nil
	}
	var (
		peers     []storage.PeerStoreEntry
		peerCount uint
	)

	for _, v := range findValues {
		if count != 0 {
			if peerCount >= count {
				break
			}
		}

		var peer storage.PeerStoreEntry
		err = proto.Unmarshal(v.([]byte), &peer)
		if err != nil {
			continue
		}

		peers = append(peers, peer)

		if count != 0 {
			peerCount++
		}
	}
	return peers
}

func (s *Swarm) OpenNewStreams(ctx context.Context, peerID types.PeerID, count uint32) ([]*network.Stream, error) {
	s.mu.RLock()
	peer, ok := s.activePeers[peerID]
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("this peer is inactive")
	}
	var (
		mu                = sync.Mutex{}
		wg                = sync.WaitGroup{}
		streams           = make([]*network.Stream, 0, count)
		openedCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
	)
	defer cancel()

	wg.Add(int(count))
	for range count {
		go func() {
			defer wg.Done()
			stream, err := peer.transport.OpenBidirectionalStream(openedCtx)
			if err != nil {
				return
			}
			mu.Lock()
			streams = append(streams, stream)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(streams) == 0 {
		return nil, errors.New("failed to open any streams")
	}

	return streams, nil
}

func (s *Swarm) GetSessionManager() *SessionManager {
	return s.sessionManager
}

func (s *Swarm) signMessageData(mesData *internal_pb.MessageData) (*internal_pb.Envelope, error) {
	Data, err := proto.Marshal(mesData)
	if err != nil {
		return nil, err
	}
	pubKey := types.PeerPrivateKeyToPublic(s.myPrivKey)
	signature := crypto.CreateSignature(Data, ed25519.PrivateKey(s.myPrivKey[:]))
	env := &internal_pb.Envelope{
		Data:      Data,
		Signature: signature,
		PubKey:    pubKey[:],
	}
	return env, nil
}

func (s *Swarm) AddPotentialPeer(id types.PeerID, addr string) {
	s.mu.Lock()
	_, connected := s.activePeers[id]
	s.mu.Unlock()

	if connected {
		return
	}

	go func() {
		log.Printf("Attempting automatic local connection to %s", addr)
		err := s.netTransport.DialEarly(context.Background(), addr)
		if err != nil {
			log.Printf("Failed to dial discovered peer: %v", err)
		}
	}()
}
