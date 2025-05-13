package p2pengine

import (
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/protocol"
	tptu "github.com/libp2p/go-libp2p/p2p/net/upgrader"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
)

// type transportEarlyDataHandler struct {
// 	muxers         []protocol.ID
// 	receivedMuxers []protocol.ID
// }

// var _ noise.EarlyDataHandler = &transportEarlyDataHandler{}

// func newTransportEDH(muxers []protocol.ID) *transportEarlyDataHandler {
// 	return &transportEarlyDataHandler{muxers: muxers}
// }

// func (i *transportEarlyDataHandler) Send(context.Context, net.Conn, peer.ID) *pb.NoiseExtensions {
// 	return &pb.NoiseExtensions{
// 		StreamMuxers: protocol.ConvertToStrings(i.muxers),
// 	}
// }

// func (i *transportEarlyDataHandler) Received(_ context.Context, _ net.Conn, extension *pb.NoiseExtensions) error {
// 	// Discard messages with size or the number of protocols exceeding extension limit for security.
// 	if extension != nil && len(extension.StreamMuxers) <= 100 {
// 		i.receivedMuxers = protocol.ConvertFromStrings(extension.GetStreamMuxers())
// 	}
// 	return nil
// }

const (
	NoisePrologue = "uptp-noise"
)

func NewSessionTransport(id protocol.ID, privkey crypto.PrivKey, muxers []tptu.StreamMuxer) (*noise.SessionTransport, error) {
	transport, err := noise.New(id, privkey, muxers)
	if err != nil {
		return nil, err
	}
	// muxerIDs := make([]protocol.ID, 0, len(muxers))
	// for _, m := range muxers {
	// 	muxerIDs = append(muxerIDs, m.ID)
	// }

	return transport.WithSessionOptions(
		// noise.EarlyData(newTransportEDH(muxerIDs), newTransportEDH(muxerIDs)),
		noise.Prologue([]byte(NoisePrologue)),
	)
}
