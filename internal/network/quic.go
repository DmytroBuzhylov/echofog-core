package network

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"

	"log"
	"net"
	"sync"
	"time"

	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"

	"github.com/quic-go/quic-go"
)

type MessageType int

const (
	TypeUnknown MessageType = iota
	TypeHandshake
	TypeReady
	TypeGossip
	TypePing
	TypeChatMessage
	TypeDatagram
	TypeGetPeerRequest
	TypeGetPeerResponse
	TypeBlockRequest
	TypeBlockResponse
	TypeSessionInitRequest
	TypeSessionInitResponse
	TypeStreamInitRequest
	TypeChunkRequest
	TypeChunkResponse
	TypeStreamCancel
)

type QuicErrorCode = quic.ApplicationErrorCode

const (
	ErrCodeNoError QuicErrorCode = iota
	ErrCodeProtocolViolation
	ErrCodeSpamDetected
	ErrCodeStreamError
	ErrCodeAuthFailed
	ErrCodeNormalClose
)

func GetQuicConfig() *quic.Config {
	return &quic.Config{
		Allow0RTT:             true,
		KeepAlivePeriod:       10 * time.Second,
		MaxIdleTimeout:        30 * time.Second,
		MaxIncomingStreams:    1000,
		MaxIncomingUniStreams: 1000,
		EnableDatagrams:       true,
	}
}

type QuicTransport struct {
	tr              *quic.Transport
	tlsCfg          *tls.Config
	quicCgf         *quic.Config
	privKey         types.PeerPrivateKey
	protocolVersion uint32

	addr net.Addr

	connChan chan NewConnEvent
}

func NewQUICTransport(addr string, tlsCfg *tls.Config, quicCgf *quic.Config, privKey types.PeerPrivateKey, protocolVersion uint32) *QuicTransport {
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	tr := &quic.Transport{
		Conn: udpConn,
	}

	return &QuicTransport{
		tr:              tr,
		tlsCfg:          tlsCfg,
		quicCgf:         quicCgf,
		privKey:         privKey,
		connChan:        make(chan NewConnEvent, 10),
		protocolVersion: protocolVersion,
		addr:            udpConn.LocalAddr(),
	}
}

func (q *QuicTransport) Addr() net.Addr {
	return q.addr
}

func (q *QuicTransport) ListenEarly(ctx context.Context) error {
	ln, err := q.tr.ListenEarly(q.tlsCfg, q.quicCgf)
	if err != nil {
		return fmt.Errorf("QuicTransport Listen error: %v", err)
	}

	go q.AcceptConn(ctx, ln, q.protocolVersion)

	return nil
}

func (q *QuicTransport) DialEarly(ctx context.Context, addr string) error {
	targetAddres, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	conn, err := q.tr.DialEarly(ctx, targetAddres, q.tlsCfg, q.quicCgf)
	if err != nil {
		return err
	}

	stream, err := conn.OpenStream()
	if err != nil {
		conn.CloseWithError(ErrCodeStreamError, "stream error")
		return err
	}
	defer stream.Close()

	peerPubKey, err := q.authenticatePeer(stream, q.protocolVersion)
	if err != nil {
		conn.CloseWithError(ErrCodeAuthFailed, "auth failed")
		return err
	}
	peerID := types.PeerPubKeyToID(peerPubKey)
	if !sendReadyFrame(stream) {
		return errors.New("error to send ready frame to peer")
	}

	q.newConn(conn, true, conn.RemoteAddr().String(), peerID, peerPubKey)
	fmt.Printf("Peer %s authenticated and TLS confirmed\n", peerID)

	return nil
}

func (q *QuicTransport) AcceptConn(ctx context.Context, ln *quic.EarlyListener, protocolVersion uint32) {
	defer ln.Close()
	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			fmt.Println(err)
			continue
		}

		go func(c *quic.Conn) {
			stream, err := c.AcceptStream(ctx)
			if err != nil {
				c.CloseWithError(ErrCodeStreamError, "stream error")
				return
			}
			defer stream.Close()

			peerPubKey, err := q.authenticatePeer(stream, protocolVersion)
			if err != nil {
				c.CloseWithError(ErrCodeAuthFailed, "auth failed")
				return
			}
			peerID := types.PeerPubKeyToID(peerPubKey)
			if !acceptReadyFrame(stream) {
				return
			}

			q.newConn(conn, false, c.RemoteAddr().String(), peerID, peerPubKey)

			fmt.Printf("Peer %s authenticated and TLS confirmed\n", peerID)

		}(conn)
	}
}

func (q *QuicTransport) ConnChan() <-chan NewConnEvent {
	return q.connChan
}

func (q *QuicTransport) newConn(conn *quic.Conn, isOut bool, addr string, PeerID types.PeerID, PeerPubKey types.PeerPublicKey) {
	q.connChan <- NewConnEvent{
		Conn:       conn,
		IsOut:      isOut,
		PeerID:     PeerID,
		Addr:       addr,
		PeerPubKey: PeerPubKey,
	}
}

func (q *QuicTransport) authenticatePeer(stream *quic.Stream, protocolVersion uint32) (types.PeerPublicKey, error) {
	myNonce := make([]byte, 32)
	rand.Read(myNonce)

	if err := sendHandshake(stream, myNonce); err != nil {
		return types.PeerPublicKey{}, err
	}

	if err := acceptHandshake(stream, q.privKey, protocolVersion); err != nil {
		return types.PeerPublicKey{}, err
	}

	peerPubKey, err := checkHandshakeResponse(stream, myNonce)
	if err != nil {
		return types.PeerPublicKey{}, err
	}

	return peerPubKey, nil
}

type PeerWrapper struct {
	conn   *quic.Conn
	peerID types.PeerID

	ctx    context.Context
	cancel context.CancelFunc

	streamsMu sync.RWMutex
	streams   map[quic.StreamID]*Stream

	onData      func(msgType MessageType, payload []byte, peerID types.PeerID)
	onNewStream func(stream *Stream)
}

func NewPeerWrapper(parentCtx context.Context, conn *quic.Conn) *PeerWrapper {
	ctx, cancel := context.WithCancel(parentCtx)
	return &PeerWrapper{
		conn:    conn,
		ctx:     ctx,
		cancel:  cancel,
		streams: make(map[quic.StreamID]*Stream),
	}
}

func (p *PeerWrapper) Close() error {
	p.cancel()
	return p.conn.CloseWithError(ErrCodeNormalClose, "normal close")
}

func (p *PeerWrapper) OnData(onData func(msgType MessageType, payload []byte, peerID types.PeerID)) {
	p.onData = onData
}

func (p *PeerWrapper) OnNewStream(handler func(stream *Stream)) {
	p.onNewStream = handler
}

func (p *PeerWrapper) GetConn() *quic.Conn {
	return p.conn
}

func (p *PeerWrapper) RemoteAddr() net.Addr {
	return p.conn.RemoteAddr()
}

func (p *PeerWrapper) StartLoops() {
	go p.AcceptUniLoop(p.ctx)
	go p.AcceptDatagramLoop(p.ctx)
	go p.AcceptStreamLoop(p.ctx)
}

func (p *PeerWrapper) SendGossipMessage(msgType MessageType, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := p.conn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}

	stream.SetWriteDeadline(time.Now().Add(1 * time.Second))

	if err := writeFrame(stream, msgType, data); err != nil {
		return err
	}

	return stream.Close()
}

func (p *PeerWrapper) SendGossipDatagram(data []byte) error {
	if len(data) > 1200 {
		return fmt.Errorf("message too big for datagram")
	}
	return p.conn.SendDatagram(data)
}

func (p *PeerWrapper) AcceptUniLoop(ctx context.Context) {
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}
		stream, err := p.conn.AcceptUniStream(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Println("Peer disconnected: ", err)
			return
		}

		go p.handleGossipStream(stream)
	}
}

func (p *PeerWrapper) AcceptStream(ctx context.Context) (*Stream, error) {
	stream, err := p.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	streamWrapper := p.wrapAndRegister(stream)

	if p.onNewStream != nil {
		go p.onNewStream(streamWrapper)
	}

	return streamWrapper, nil
}

func (p *PeerWrapper) AcceptDatagramLoop(ctx context.Context) {
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}
		msg, err := p.conn.ReceiveDatagram(ctx)
		if err != nil {
			return
		}

		fmt.Printf("Gossip (Unreliable): %s\n", string(msg))
	}
}

func (p *PeerWrapper) AcceptStreamLoop(ctx context.Context) {
	go func() {
		for {
			stream, err := p.conn.AcceptStream(ctx)
			if err != nil {
				return
			}

			streamWrapper := p.wrapAndRegister(stream)

			if p.onNewStream != nil {
				go p.onNewStream(streamWrapper)
			}
		}
	}()
}

func (p *PeerWrapper) handleGossipStream(stream *quic.ReceiveStream) {
	msgType, msg, err := readFrame(stream)
	if err == nil && p.onData != nil {
		p.onData(msgType, msg, p.peerID)
	}
}

func (p *PeerWrapper) OpenBidirectionalStream(ctx context.Context) (*Stream, error) {
	stream, err := p.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	return p.wrapAndRegister(stream), nil
}

func (p *PeerWrapper) wrapAndRegister(qStream *quic.Stream) *Stream {
	cleanup := func() {
		p.removeStream(qStream.StreamID())
	}

	stream := NewStream(p.ctx, qStream, qStream.StreamID(), p.peerID, cleanup)

	p.streamsMu.Lock()
	p.streams[stream.StreamID] = stream
	p.streamsMu.Unlock()

	return stream
}

func (p *PeerWrapper) removeStream(id quic.StreamID) {
	p.streamsMu.Lock()
	delete(p.streams, id)
	p.streamsMu.Unlock()
}
