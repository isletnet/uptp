package uptp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	reuseport "github.com/libp2p/go-reuseport"
)

const (
	natTypeNone      = 0
	natTypeCone      = 1
	natTypeSymmetric = 2
	natTypeUnknown   = 500
)

type natTypeInfo struct {
	publicIP string
	natType  int

	isExclusivePublicIPV4 bool
}

func natTest(testHost string, testPort, localPort int) (string, int, int, error) {
	la, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", localPort))
	if err != nil {
		return "", 0, 0, fmt.Errorf("resolve local addr on %d error: %s", localPort, err)
	}
	// conn, err := net.ListenUDP("udp", la)
	// if err != nil {
	// 	return "", 0, 0, fmt.Errorf("listen on %d error: %s", localPort, err)
	// }
	// defer conn.Close()
	// if localPort == 0 {
	// 	localPort, _ = strconv.Atoi(strings.Split(conn.LocalAddr().String(), ":")[1])
	// }

	dst, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", testHost, testPort))
	if err != nil {
		return "", 0, 0, fmt.Errorf("resolve test addr %s:%d error: %s", testHost, testPort, err)
	}
	conn, err := net.DialUDP("udp", la, dst)
	if err != nil {
		return "", 0, 0, fmt.Errorf("udp dial to %s:%d on %d error: %s", testHost, testPort, localPort, err)
	}
	defer conn.Close()
	if localPort == 0 {
		ua := conn.LocalAddr().(*net.UDPAddr)
		localPort = ua.Port
	}
	_, err = conn.Write([]byte("hello"))
	if err != nil {
		return "", 0, 0, fmt.Errorf("write to %s error: %s", dst.String(), err)
	}
	err = conn.SetReadDeadline(time.Now().Add(natTestTimeout))
	if err != nil {
		return "", 0, 0, fmt.Errorf("set read dead line error: %s", err)
	}
	buffer := make([]byte, 1024)
	nRead, err := conn.Read(buffer)
	if err != nil {
		return "", 0, 0, fmt.Errorf("read error: %s", err)
	}
	res, err := net.ResolveUDPAddr("udp", string(buffer[:nRead]))
	if err != nil {
		return "", 0, 0, fmt.Errorf("resolve result addr %s error: %s", string(buffer[:nRead]), err)
	}

	return res.IP.String(), res.Port, localPort, nil
}

func natTestTcp(testHost string, testPort, localPort int) (string, int, int, error) {
	conn, err := reuseport.DialTimeout("tcp4", fmt.Sprintf("%s:%d", "0.0.0.0", localPort), fmt.Sprintf("%s:%d", testHost, testPort), natTestTimeout)
	if err != nil {
		return "", 0, 0, fmt.Errorf("dial %s:%d error: %s", testHost, testPort, err)
	}
	defer conn.Close()
	if localPort == 0 {
		localPort, _ = strconv.Atoi(strings.Split(conn.LocalAddr().String(), ":")[1])
	}
	_, err = conn.Write([]byte("uptpc"))
	if err != nil {
		return "", 0, 0, fmt.Errorf("write nat test error: %s", err)
	}
	b := make([]byte, 1000)
	conn.SetReadDeadline(time.Now().Add(natTestTimeout))
	n, err := conn.Read(b)
	if err != nil {
		return "", 0, 0, fmt.Errorf("read nat test error: %s", err)
	}
	arr := strings.Split(string(b[:n]), ":")
	if len(arr) < 2 {
		return "", 0, 0, fmt.Errorf("read wrong nat test result: %s", string(b[:n]))
	}
	publicPort, err := strconv.Atoi(arr[1])
	if err != nil {
		return "", 0, 0, fmt.Errorf("read wrong nat test port: %s", arr[1])
	}
	publicIP := arr[0]
	return publicIP, publicPort, localPort, nil
}

func launchTmpEchoService(ctx context.Context, localPort int) (int, error) {
	ua, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("0.0.0.0:%d", localPort))
	if err != nil {
		return 0, fmt.Errorf("resolve udp addr error: %s", err)
	}
	echoConn, err := net.ListenUDP("udp4", ua)
	if err != nil {
		return 0, fmt.Errorf("listen on %d error: %s", localPort, err)
	}
	if localPort == 0 {
		localPort, _ = strconv.Atoi(strings.Split(echoConn.LocalAddr().String(), ":")[1])
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		buf := make([]byte, 1600)
		for {
			echoConn.SetReadDeadline(time.Now().Add(time.Second * 5))
			n, addr, err := echoConn.ReadFromUDP(buf)
			if err == nil {
				if string(buf[:n]) != "uptpcpublictest" {
					continue
				}
				echoConn.WriteToUDP(buf[0:n], addr)
				echoConn.Close()
				return
			}
			e, ok := err.(*net.OpError)
			if ok && e.Timeout() && ctx.Err() == nil {
				continue
			}
			echoConn.Close()
			return
		}
	}()
	wg.Wait()
	return localPort, nil
}

func publicIPTest(publicIP string, localPort int) (bool, error) {
	ctx, cancle := context.WithCancel(context.Background())
	defer cancle()
	realPort, err := launchTmpEchoService(ctx, localPort)
	if err != nil {
		return false, fmt.Errorf("launch tmp echo service on %d error: %s", localPort, err)
	}
	localPort = realPort

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return false, fmt.Errorf("listen udp error: %s", err)
	}
	defer conn.Close()
	dst, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", publicIP, localPort))
	if err != nil {
		return false, fmt.Errorf("resolve public %s:%d addr error: %s", publicIP, localPort, err)
	}
	conn.WriteToUDP([]byte("uptpcpublictest"), dst)
	buf := make([]byte, 1600)

	// wait for echo testing
	conn.SetReadDeadline(time.Now().Add(publicIPTestTimeout))
	_, _, err = conn.ReadFromUDP(buf)
	if err != nil {
		e, ok := err.(*net.OpError)
		if ok && e.Timeout() {
			return false, nil
		}
		return false, fmt.Errorf("read echo on %d error: %s", localPort, err)
	}
	return true, nil
}

func natTypeTest(testHost string, testPort1, testPort2, publicPort int) (*natTypeInfo, error) {
	ip1, port1, lp, err := natTest(testHost, testPort1, 0)
	if err != nil {
		return nil, fmt.Errorf("nat test 1 on %d error: %s", 0, err)
	}
	ok, err := publicIPTest(ip1, publicPort)
	if err != nil {
		return nil, fmt.Errorf("public ip test error: %s", err)
	}
	ip2, port2, _, err := natTest(testHost, testPort2, lp)
	if err != nil {
		return nil, fmt.Errorf("nat test 2 on %d error: %s", lp, err)
	}
	natType := natTypeSymmetric
	if ip1 != ip2 {
		natType = natTypeUnknown
	} else if port1 == port2 {
		natType = natTypeCone
	}
	ret := &natTypeInfo{
		publicIP: ip1,
		natType:  natType,

		isExclusivePublicIPV4: ok,
	}
	return ret, nil
}
