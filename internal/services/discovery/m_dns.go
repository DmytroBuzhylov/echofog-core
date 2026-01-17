package discovery

import (
	"context"
	"encoding/hex"
	"fmt"

	"log"
	"strings"

	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"

	"github.com/grandcat/zeroconf"
)

const (
	ServiceInstance = "EchoFogNode"
	ServiceType     = "_echo-for-p2p._udp"
	Domain          = "local."
)

type PeerNotifier interface {
	AddPotentialPeer(id types.PeerID, addr string)
}

type MDNSService struct {
	server *zeroconf.Server
	myID   types.PeerID
	port   int
	notif  PeerNotifier
}

func NewMDNS(id types.PeerID, port int, notifier PeerNotifier) *MDNSService {
	return &MDNSService{
		myID:  id,
		port:  port,
		notif: notifier,
	}
}

func (s *MDNSService) Start(ctx context.Context) error {
	idHex := hex.EncodeToString(s.myID[:])
	txtRecords := []string{fmt.Sprintf("id=%s", idHex), "v=1.0.0"}

	server, err := zeroconf.Register(
		fmt.Sprintf("EchoFog-%s", idHex[:6]),
		ServiceType,
		Domain,
		s.port,
		txtRecords,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %w", err)
	}
	s.server = server

	go s.browseLoop(ctx)

	return nil
}

func (s *MDNSService) browseLoop(ctx context.Context) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Printf("[mDNS] Failed to initialize resolver: %v", err)
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)

	go func() {
		if err := resolver.Browse(ctx, ServiceType, Domain, entries); err != nil {
			log.Printf("[mDNS] Browse failed: %v", err)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			s.server.Shutdown()
			return
		case entry := <-entries:
			s.handleEntry(entry)
		}
	}
}

func (s *MDNSService) handleEntry(entry *zeroconf.ServiceEntry) {

	var remoteID types.PeerID
	foundID := false

	for _, record := range entry.Text {
		if strings.HasPrefix(record, "id=") {
			idStr := strings.TrimPrefix(record, "id=")
			idBytes, err := hex.DecodeString(idStr)
			if err == nil {
				remoteID, err = types.ToPeerID(idBytes)
				if err == nil {
					foundID = true
				}
			}
			break
		}
	}

	if !foundID || remoteID == s.myID {
		return
	}

	var bestAddr string
	if len(entry.AddrIPv4) > 0 {
		bestAddr = fmt.Sprintf("%s:%d", entry.AddrIPv4[0].String(), entry.Port)
	} else if len(entry.AddrIPv6) > 0 {
		bestAddr = fmt.Sprintf("[%s]:%d", entry.AddrIPv6[0].String(), entry.Port)
	} else {
		return
	}
	log.Printf("[Discovery] Found peer %x at %s", remoteID[:4], bestAddr)
	s.notif.AddPotentialPeer(remoteID, bestAddr)
}

func (s *MDNSService) Stop() {
	if s.server != nil {
		s.server.Shutdown()
	}
}
