package tunstack

import (
	"net/url"

	"github.com/gorilla/schema"
	"github.com/xjasonlyu/tun2socks/v2/core/device"
	"github.com/xjasonlyu/tun2socks/v2/core/device/tun"
	"golang.org/x/sys/windows"
	wun "golang.zx2c4.com/wireguard/tun"
)

const (
	tunGUID = "{D5BFEE3A-9DC2-44CC-9FFC-A7F6B53474F2}"
)

func init() {
	guid, _ := windows.GUIDFromString(tunGUID)
	wun.WintunStaticRequestedGUID = &guid
	wun.WintunTunnelType = "uptptun"
}

func parseTUN(u *url.URL, mtu uint32) (device.Device, error) {
	opts := struct {
		GUID string
	}{}
	if err := schema.NewDecoder().Decode(&opts, u.Query()); err != nil {
		return nil, err
	}
	if opts.GUID != "" {
		guid, err := windows.GUIDFromString(opts.GUID)
		if err != nil {
			return nil, err
		}
		wun.WintunStaticRequestedGUID = &guid
	}
	return tun.Open(u.Host, mtu)
}
