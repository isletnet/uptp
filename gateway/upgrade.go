package gateway

import (
	"io"
	"net/http"
	"os"

	"github.com/isletnet/uptp/apiutil.go"
	"github.com/isletnet/uptp/common"
	"github.com/isletnet/uptp/logging"
	"github.com/isletnet/uptp/upgrade"
)

type versionCheck struct {
	CurrVersion   string `json:"current_version"`
	LatestVersion string `json:"latest_version"`
}

// func (g *Gateway) checkVersion(w http.ResponseWriter, r *http.Request) {
// 	rsp := apiutil.ApiResponse{}
// 	resp, err := upgrade.QueryLatestVersion(g.pe.Libp2pHost(), "gateway")
// 	if err != nil {
// 		rsp.Code = 500
// 		rsp.Message = "query latest version of gateway failed"
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}
// 	var version string
// 	err = apiutil.ParseHttpResponse(resp, &version)
// 	if err != nil {
// 		rsp.Code = 500
// 		rsp.Message = "read latest version of gateway failed: "
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}
// 	// common.CompareVersion(version, common.GatewayVersion)
// 	rsp.Data = versionCheck{
// 		CurrVersion:   common.GatewayVersion,
// 		LatestVersion: version,
// 	}
// 	rsp.Message = "success"
// 	apiutil.SendAPIRespWithOk(w, rsp)
// 	// defer resp.Body.Close()
// 	// transferHTTP(w, resp)
// }

func (g *Gateway) upgradeMyself(w http.ResponseWriter, r *http.Request) {
	var rsp apiutil.ApiResponse
	latestGWInfo, err := upgrade.QueryLatestVersion(g.pe.Libp2pHost(), "gateway")
	if err != nil {
		logging.Error("upgrade gateway failed: query latest version error: %s", err)
		rsp.Code = 500
		rsp.Message = "query latest version failed"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	logging.Info("got latest gateway info %+v", latestGWInfo)
	if common.CompareVersion(common.GatewayVersion, latestGWInfo.Version) != common.LESS {
		rsp.Code = 0
		rsp.Message = "is latest version"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	resp, err := upgrade.DownloadLatestGateway(g.pe.Libp2pHost(), latestGWInfo.DownloadServer, latestGWInfo.DownloadPath)
	if err != nil {
		logging.Error("upgrade gateway failed: download latest version error: %s", err)
		rsp.Code = 500
		rsp.Message = "download latest version failed"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logging.Error("upgrade gateway failed: doanload latest version return: %s", resp.Status)
		rsp.Code = 500
		rsp.Message = "download latest version failed"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	downloadFile, err := os.CreateTemp("", "uptp-gateway*.tmp")
	if err != nil {
		logging.Error("upgrade gateway failed: create temp file error: %s", err)
		rsp.Code = 500
		rsp.Message = "create team file failed"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	_, err = io.Copy(downloadFile, resp.Body)
	if err != nil {
		logging.Error("upgrade gateway failed: download latest version error: %s", err)
		rsp.Code = 500
		rsp.Message = "download latest version failed"
		apiutil.SendAPIRespWithOk(w, rsp)
		downloadFile.Close()
		return
	}
	downloadFile.Close()
	//todo checksum test

	binPath, err := os.Executable()
	if err != nil {
		logging.Error("upgrade gateway failed: get executable path error: %s", err)
		rsp.Code = 500
		rsp.Message = "get executable path failed"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	err = os.Rename(binPath, binPath+".bak")
	if err != nil {
		logging.Error("upgrade gateway failed: rename executable error: %s", err)
		rsp.Code = 500
		rsp.Message = "rename executable failed"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	err = os.Rename(downloadFile.Name(), binPath)
	if err != nil {
		logging.Error("upgrade gateway failed: rename downloaded file error: %s", err)
		rsp.Code = 500
		rsp.Message = "rename downloaded file failed"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	err = os.Chmod(binPath, 0775)
	if err != nil {
		logging.Error("upgrade gateway failed: chmod executable error: %s", err)
		rsp.Code = 500
		rsp.Message = "chmod executable failed"
		return
	}
	rsp.Message = "upgrade gateway success, please restart"
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) downloadAPK(w http.ResponseWriter, r *http.Request) {
	resp, err := upgrade.DownloadGatewayAgentApk(g.pe.Libp2pHost())
	if err != nil {
		logging.Error("download gateway agent apk error: %s", err)
		var rsp apiutil.ApiResponse
		rsp.Code = 500
		rsp.Message = "download gateway agent apk failed"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	defer resp.Body.Close()
	transferHTTP(w, resp)
}
