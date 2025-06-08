package gateway

import (
	"math/big"
	"net"
	"sync"

	"github.com/emirpasic/gods/trees/avltree"
	"github.com/emirpasic/gods/utils"
	"github.com/isletnet/uptp/socks5"
)

type proxyRouter struct {
	once sync.Once
	data *avltree.Tree
	mtx  sync.RWMutex
}

func newProxyRouter() *proxyRouter {
	r := &proxyRouter{}
	r.init()
	return r
}

type routeItem struct {
	max    uint32
	dialer *socks5.Dialer
}

func (r *proxyRouter) init() {
	r.once.Do(func() {
		r.data = avltree.NewWith(utils.UInt32Comparator)
	})
	r.data.Clear()
}

func (r *proxyRouter) addRoute(ipNet *net.IPNet, dialer *socks5.Dialer) bool {
	min, max := getCidrIntRange(ipNet)
	i, f := r.data.Get(min)
	if !f {
		ri := routeItem{
			max:    max,
			dialer: dialer,
		}
		return r.add(min, ri)
	}
	ri := i.(routeItem)
	if ri.max != max {
		return false
	}
	ri.dialer = dialer
	r.data.Put(min, ri)
	return true
}

func (r *proxyRouter) delRoute(ipNet *net.IPNet) {
	min, _ := getCidrIntRange(ipNet)
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.data.Remove(min)
}

func (r *proxyRouter) add(key uint32, value routeItem) bool {
	if key > value.max {
		return false
	}
	r.mtx.Lock()
	defer r.mtx.Unlock()
	newMinIP := key
	newMaxIP := value.max
	cur := r.data.Root
	for {
		if cur == nil {
			break
		}
		curMaxIP := cur.Value.(routeItem).max
		curMinIP := cur.Key.(uint32)

		// has no interset
		if newMinIP > curMaxIP {
			cur = cur.Children[1]
			continue
		}
		if newMaxIP < curMinIP {
			cur = cur.Children[0]
			continue
		}
		return false
	}
	//  put in the tree
	r.data.Put(newMinIP, value)
	return true
}

func (r *proxyRouter) get(key uint32) *socks5.Dialer {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	if r.data == nil {
		return nil
	}
	n := r.data.Root
	for n != nil {
		ri := n.Value.(routeItem)
		curMaxIP := ri.max
		curMinIP := n.Key.(uint32)
		switch {
		case key >= curMinIP && key <= curMaxIP:
			return ri.dialer
		case key < curMinIP:
			n = n.Children[0]
			break
		default:
			n = n.Children[1]
		}
	}
	return nil
}

func getCidrIntRange(ipNet *net.IPNet) (uint32, uint32) {
	ret := big.NewInt(0)
	ret.SetBytes(ipNet.IP.To4())
	ipMin := uint32(ret.Int64())
	var m uint32
	m = 0xffffffff
	ones, allBits := ipNet.Mask.Size()
	m = m << uint32(allBits-ones)
	ipMax := ipMin + ^m
	return ipMin, ipMax
}
