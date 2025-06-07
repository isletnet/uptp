package upgrade

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/isletnet/uptp/apiutil.go"
	p2phttp "github.com/libp2p/go-libp2p-http"
	"github.com/libp2p/go-libp2p/core/host"
)

var UpgradeServer string = "12D3KooWCobLm1afjdE3aqTvHjPNdFyHT54Zgjkpmrg8ABV3FzWh"

func p2pHttpClient(h host.Host) *http.Client {
	tr := &http.Transport{}
	tr.RegisterProtocol("libp2p", p2phttp.NewTransport(h))
	return &http.Client{Transport: tr}
}

type LatestInfo struct {
	Program        string `json:"program"`
	Version        string `json:"version"`
	DownloadServer string `json:"download_Server"`
	DownloadPath   string `json:"download_path"`
	Checksum       string `json:"checksum"`
}

func QueryLatestVersion(h host.Host, program string) (ret LatestInfo, err error) {
	resp, err := p2pHttpClient(h).Get(fmt.Sprintf("libp2p://%s/version/%s?sys_type=%s-%s", UpgradeServer, program, runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return
	}

	err = apiutil.ParseHttpResponse(resp, &ret)
	if err != nil {
		return
	}
	return
}

func DownloadGatewayAgentApk(h host.Host) (*http.Response, error) {
	return p2pHttpClient(h).Get(fmt.Sprintf("libp2p://%s/gateway-agent.apk", UpgradeServer))
}
func DownloadLatestGateway(h host.Host, downloadServer string, downloadPath string) (*http.Response, error) {
	return p2pHttpClient(h).Get(fmt.Sprintf("libp2p://%s/%s", downloadServer, downloadPath))
}
