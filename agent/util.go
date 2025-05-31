package agent

import (
	"io"
	"net"
	"net/http"
)

func GetIPv6(testUrl string) (string, error) {
	rsp, err := http.Get(testUrl)
	if err != nil {
		return "", err
	}
	defer rsp.Body.Close()
	buf := make([]byte, 1024)
	n, err := rsp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	return string(buf[:n]), nil
}

func TestIPv6(testUrl string) error {
	ipv6, err := GetIPv6(testUrl)
	if err != nil {
		return err
	}
	_, err = net.ResolveIPAddr("ip6", ipv6)
	return err
}
