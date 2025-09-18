package bitswap

import "github.com/libp2p/go-libp2p/core/peer"

type ConnectionListener struct {
}

func (l *ConnectionListener) PeerConnected(peer.ID) {

}

func (l *ConnectionListener) PeerDisconnected(peer.ID) {

}
