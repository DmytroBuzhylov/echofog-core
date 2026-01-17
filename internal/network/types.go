package network

import (
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
	"github.com/quic-go/quic-go"
)

type NewConnEvent struct {
	Conn       *quic.Conn
	IsOut      bool
	PeerID     types.PeerID
	PeerPubKey types.PeerPublicKey
	Addr       string
}
