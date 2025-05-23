package tunstack

import (
	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/tunnel"
	"gvisor.dev/gvisor/pkg/log"
)

func netstack(k *Key) (err error) {
	if _defaultDevice, err = parseDevice(k.Device, uint32(k.MTU)); err != nil {
		return
	}

	if _defaultStack, err = core.CreateStack(&core.Config{
		LinkEndpoint:     _defaultDevice,
		TransportHandler: tunnel.T(),
	}); err != nil {
		return
	}

	log.Infof(
		"[STACK] %s://%s ",
		_defaultDevice.Type(), _defaultDevice.Name(),
	)
	return
}
