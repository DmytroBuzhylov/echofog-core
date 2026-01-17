package p2p

import (
	"context"

	"sync"
	"time"

	"github.com/DmytroBuzhylov/echofog-core/internal/network"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"

	"github.com/google/uuid"
)

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[types.SessionID]*TransferSession
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[types.SessionID]*TransferSession),
	}
}

func (sm *SessionManager) CreateNewSession(parentCtx context.Context, RootHash types.ContentHash, PeerID types.PeerID) types.SessionID {
	sessionID := [16]byte(uuid.New())
	transfer := NewTransferSession(parentCtx, sessionID, RootHash, PeerID, true)

	sm.mu.Lock()
	sm.sessions[sessionID] = transfer
	sm.mu.Unlock()

	return sessionID
}

func (sm *SessionManager) AddStream(sessionID types.SessionID, stream *network.Stream) bool {
	sm.mu.Lock()
	transfer, ok := sm.sessions[sessionID]
	sm.mu.Unlock()
	if !ok {
		return false
	}

	transfer.addStream(stream)

	select {
	case transfer.NewStreams <- stream:
		return true
	case <-transfer.Ctx.Done():
		return false
	}

	return true
}

func (sm *SessionManager) Register(id types.SessionID, sess *TransferSession) {
	sm.mu.Lock()
	sm.sessions[id] = sess
	sm.mu.Unlock()
}

func (sm *SessionManager) Get(id types.SessionID) *TransferSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

type TransferSession struct {
	SessionID    types.SessionID
	RootHash     types.ContentHash
	PeerID       types.PeerID
	IsOutgoing   bool
	Streams      []*network.Stream
	LastActive   time.Time
	StreamsCount uint32
	Mu           sync.Mutex

	NewStreams chan *network.Stream

	Ctx    context.Context
	Cancel context.CancelFunc
}

func NewTransferSession(parentCtx context.Context, SessionID types.SessionID, RootHash types.ContentHash, PeerID types.PeerID, IsOutgoing bool) *TransferSession {
	ctx, cancel := context.WithCancel(parentCtx)
	return &TransferSession{
		SessionID:  SessionID,
		RootHash:   RootHash,
		PeerID:     PeerID,
		IsOutgoing: IsOutgoing,
		NewStreams: make(chan *network.Stream, 10),
		Ctx:        ctx,
		Cancel:     cancel,
	}
}

func (t *TransferSession) Close() error {
	t.Cancel()
	return nil
}

func (t *TransferSession) addStream(stream *network.Stream) {
	t.Mu.Lock()
	defer t.Mu.Unlock()

	if t.Streams == nil {
		t.Streams = make([]*network.Stream, 0)
	}
	t.Streams = append(t.Streams, stream)
}
