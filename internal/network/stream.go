package network

import (
	"context"

	"sync"

	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"

	"github.com/quic-go/quic-go"
)

type StreamMessage struct {
	Type    MessageType
	Payload []byte
}

type Stream struct {
	StreamID quic.StreamID
	RemoteID types.PeerID

	Incoming chan *StreamMessage
	Outgoing chan *StreamMessage

	stream *quic.Stream

	ctx    context.Context
	cancel context.CancelFunc

	onClose func()
	once    sync.Once
}

func NewStream(ctx context.Context, stream *quic.Stream, StreamID quic.StreamID, remoteID types.PeerID, onClose func()) *Stream {
	childCtx, cancel := context.WithCancel(ctx)
	s := &Stream{
		StreamID: StreamID,
		RemoteID: remoteID,
		Incoming: make(chan *StreamMessage, 100),
		Outgoing: make(chan *StreamMessage, 100),
		stream:   stream,
		ctx:      childCtx,
		cancel:   cancel,
		onClose:  onClose,
	}

	go s.readLoop()
	go s.writeLoop()

	return s
}

func (s *Stream) Close() error {
	var err error
	s.once.Do(func() {
		s.cancel()
		s.stream.CancelRead(quic.StreamErrorCode(ErrCodeNormalClose))
		err = s.stream.Close()

		if s.onClose != nil {
			s.onClose()
		}
	})

	return err
}

func (s *Stream) readLoop() {
	defer s.Close()
	for {
		msgType, payload, err := readFrame(s.stream)
		if err != nil {
			return
		}

		select {
		case s.Incoming <- &StreamMessage{
			Type:    msgType,
			Payload: payload,
		}:
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Stream) writeLoop() {
	defer s.Close()
	for {
		select {
		case msg := <-s.Outgoing:
			writeFrame(s.stream, msg.Type, msg.Payload)

		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Stream) Send(Type MessageType, Payload []byte) {
	s.Outgoing <- &StreamMessage{
		Type:    Type,
		Payload: Payload,
	}
}

func (s *Stream) ReadCh() <-chan *StreamMessage {
	return s.Incoming
}
