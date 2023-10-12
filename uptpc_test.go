package uptp

import (
	"testing"
	"time"
)

func TestNattest(t *testing.T) {
	ip1, p1, lp, err := natTest("nattest.isletnet.cn", 1921, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ip1, ":", p1)
	ip2, p2, _, err := natTest("nattest.isletnet.cn", 1949, lp)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ip2, ":", p2)
	ip6, p6, _, err := natTest("nattest6.isletnet.cn", 1949, lp)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ip6, ":", p6)
}

func TestPublicIPTest(t *testing.T) {
	ok, err := publicIPTest("192.168.6.3", 34567)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("public ip test failed")
	}
}

func TestNatTypeTest(t *testing.T) {
	info, err := natTypeTest("nattest.isletnet.cn", 1921, 1949, 34567)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", info)
}

func TestNatTestTcp(t *testing.T) {
	ip1, pp1, lp, err := natTestTcp("nattest.isletnet.cn", 1921, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n", ip1, ":", pp1, "\n")
	time.Sleep(time.Second)
	ip2, pp2, _, err := natTestTcp("nattest.isletnet.cn", 1949, lp)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n", ip2, ":", pp2, "\n")
}

func TestGetPublicIPv4Info(t *testing.T) {
	info, err := getPublicIPv4Info()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("\n%+v\n", *info)
}
