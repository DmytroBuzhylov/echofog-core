package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/DmytroBuzhylov/echofog-core/internal/config"
	"github.com/DmytroBuzhylov/echofog-core/internal/crypto"
	"github.com/DmytroBuzhylov/echofog-core/internal/dispatcher"
	"github.com/DmytroBuzhylov/echofog-core/internal/identity"
	"github.com/DmytroBuzhylov/echofog-core/internal/logger"
	"github.com/DmytroBuzhylov/echofog-core/internal/network"
	"github.com/DmytroBuzhylov/echofog-core/internal/p2p"
	"github.com/DmytroBuzhylov/echofog-core/internal/p2p/gossip"
	"github.com/DmytroBuzhylov/echofog-core/internal/services"
	"github.com/DmytroBuzhylov/echofog-core/internal/services/discovery"
	"github.com/DmytroBuzhylov/echofog-core/internal/services/messenger"
	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
	"github.com/dgraph-io/badger/v4"
)

type Node struct {
	ID      types.PeerID
	PrivKey types.PeerPrivateKey
	PubKey  types.PeerPublicKey

	Cfg        *config.AppConfig
	Storage    storage.Storage
	Transport  *network.QuicTransport
	Dispatcher *dispatcher.Dispatcher
	Swarm      *p2p.Swarm

	Discovery *discovery.DiscoveryService
	Messenger *messenger.MessageService

	Logger  *slog.Logger
	LogChan chan logger.LogEntry
}

func NewNode(cfg *config.AppConfig) *Node {
	logChan := make(chan logger.LogEntry, 100)

	handler := logger.NewChannelHandler(logChan, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	return &Node{
		Cfg:     cfg,
		LogChan: logChan,
		Logger:  slog.New(handler),
	}
}

func (n *Node) Start(ctx context.Context, password string) error {
	n.Logger.Info("EchoFog Core is initializing...", "version", config.CurrentProtocolVersion)

	opts := badger.DefaultOptions(n.Cfg.Storage.DatabasePath)
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return fmt.Errorf("failed to open badger db: %w", err)
	}

	n.Storage = storage.NewBadgerStorage(db)
	n.Logger.Info("Storage initialized", "path", n.Cfg.Storage.DatabasePath)

	keyStore := crypto.NewSecureKeyStore([]byte(password), n.Storage)
	privKeyEd, err := keyStore.GetOrGenerateKeys()
	if err != nil {
		return fmt.Errorf("auth failed: %w", err)
	}

	n.PrivKey = types.PeerPrivateKey(privKeyEd)
	n.PubKey = types.PeerPrivateKeyToPublic(n.PrivKey)
	n.ID = types.PeerPubKeyToID(n.PubKey)

	n.Logger.Info("Identity unlocked", "peer_id", hex.EncodeToString(n.ID[:]))

	tlsConfig, err := crypto.GenerateTLSConfig(privKeyEd)
	if err != nil {
		return fmt.Errorf("tls config failed: %w", err)
	}

	n.Dispatcher = dispatcher.NewDispatcher(ctx)

	n.Transport = network.NewQUICTransport(
		n.Cfg.Network.ListenAddr,
		tlsConfig,
		network.GetQuicConfig(),
		n.PrivKey,
		n.Cfg.Network.ProtocolVersion,
	)

	n.Swarm = p2p.NewSwarm(
		n.ID,
		n.PrivKey,
		n.Dispatcher,
		n.Transport.ConnChan(),
		n.Storage,
		n.Cfg,
	)

	eng, err := crypto.NewEngine(privKeyEd)
	if err != nil {
		return fmt.Errorf("crypto engine init failed: %w", err)
	}

	gsp := gossip.NewManager(n.Swarm)

	n.Discovery = discovery.NewDiscoveryService(n.Storage, gsp, n.Swarm, n.PubKey)

	n.Messenger = messenger.NewMessageService(n.PrivKey, eng, n.Storage, gsp)

	svcList := []services.Service{
		n.Discovery,
		n.Messenger,
	}

	for _, s := range svcList {
		for _, msgType := range s.GetSubscribedTypes() {
			n.Dispatcher.Registry(msgType, s)
		}
	}

	n.Logger.Info("Starting network stack...")

	go n.Dispatcher.Start()

	if err := n.Transport.ListenEarly(ctx); err != nil {
		return fmt.Errorf("transport listen failed: %w", err)
	}

	localIP, _ := identity.GetLocalIP()
	outboundIP, _ := identity.GetOutboundIP()
	n.Logger.Info("Node started successfully",
		"local_ip", localIP,
		"outbound_ip", outboundIP,
		"listen_addr", n.Transport.Addr().String(),
	)

	return nil
}

func (n *Node) GetLogChannel() <-chan logger.LogEntry {
	return n.LogChan
}

func (n *Node) Stop() {
	if n.Storage != nil {
		// n.Storage.Close()
	}
	// n.Transport.Close()
	n.Logger.Info("Node stopped")
}
