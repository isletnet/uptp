package uptp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"hash/crc64"
	"net"
	"net/http"
	"time"
)

func hashStringToInt(s string) uint64 {
	return crc64.Checksum([]byte(s), crc64.MakeTable(crc64.ECMA))
}

type publicIPInfo struct {
	IP         net.IP  `json:"ip"`
	IPDecimal  uint32  `json:"ip_decimal"`
	Country    string  `json:"country,omitempty"`
	CountryISO string  `json:"country_iso,omitempty"`
	CountryEU  *bool   `json:"country_eu,omitempty"`
	RegionName string  `json:"region_name,omitempty"`
	RegionCode string  `json:"region_code,omitempty"`
	MetroCode  uint    `json:"metro_code,omitempty"`
	PostalCode string  `json:"zip_code,omitempty"`
	City       string  `json:"city,omitempty"`
	Latitude   float64 `json:"latitude,omitempty"`
	Longitude  float64 `json:"longitude,omitempty"`
	Timezone   string  `json:"time_zone,omitempty"`
	ASN        string  `json:"asn,omitempty"`
	ASNOrg     string  `json:"asn_org,omitempty"`
	Hostname   string  `json:"hostname,omitempty"`
}

func getPublicIPv4Info() (*publicIPInfo, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp4", addr)
		},
	}

	client := &http.Client{Transport: tr, Timeout: time.Second * 10}
	r, err := client.Get("https://ifconfig.co/json")
	if err != nil {
		return nil, fmt.Errorf("http get request error: %s", err)
	}
	defer r.Body.Close()
	buf := make([]byte, 1024*64)
	n, err := r.Body.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read http response error: %s", err)
	}
	ret := &publicIPInfo{}
	if err = json.Unmarshal(buf[:n], ret); err != nil {
		return nil, fmt.Errorf("unmarshal http response error: %s", err)
	}
	return ret, nil
}

func getLocalIPv4() (string, error) {
	conn, err := net.Dial("udp", "1.2.3.4:5678")
	if err != nil {
		return "", fmt.Errorf("dial udp error: %s", err)
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}
