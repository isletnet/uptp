package tunstack

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/xjasonlyu/tun2socks/v2/core/device"
	"github.com/xjasonlyu/tun2socks/v2/core/device/fdbased"
	"github.com/xjasonlyu/tun2socks/v2/core/device/tun"
)

func parseDevice(s string, mtu uint32) (device.Device, error) {
	if !strings.Contains(s, "://") {
		s = fmt.Sprintf("%s://%s", tun.Driver /* default driver */, s)
	}

	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	driver := strings.ToLower(u.Scheme)

	switch driver {
	case fdbased.Driver:
		return parseFD(u, mtu)
	case tun.Driver:
		return parseTUN(u, mtu)
	default:
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
}

func parseFD(u *url.URL, mtu uint32) (device.Device, error) {
	offset := 0
	// fd offset in ios
	// https://stackoverflow.com/questions/69260852/ios-network-extension-packet-parsing/69487795#69487795
	if runtime.GOOS == "ios" {
		offset = 4
	}
	return fdbased.Open(u.Host, mtu, offset)
}
