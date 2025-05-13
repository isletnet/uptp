package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"os"
	"strings"

	"github.com/google/uuid"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	log "github.com/ipfs/go-log/v2"
	"github.com/isletnet/uptp/p2pengine"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
)

var logger = log.Logger("main")

func main() {
	logger.Info("bootstrap start")
	lplConfig := log.GetConfig()
	lplConfig.Stderr = false
	lplConfig.Stdout = true
	log.SetupLogging(lplConfig)
	log.SetLogLevel("*", "error")
	log.SetLogLevel("main", "info")
	us, err := os.ReadFile("uuid")
	if err != nil && os.IsExist(err) {
		panic(err)
	}
	if len(us) < ed25519.SeedSize {
		u, err := uuid.NewRandom()
		if err != nil {
			panic(err)
		}
		str := strings.Replace(u.String(), "-", "", -1)
		if len(str) < ed25519.SeedSize {
			panic(str)
		}
		err = os.WriteFile("uuid", []byte(str), 0644)
		if err != nil {
			panic(err)
		}
		us = []byte(str)
	}
	priv, _, err := crypto.GenerateEd25519Key(bytes.NewBuffer(us[:ed25519.SeedSize]))
	if err != nil {
		panic(err)
	}
	h, err := libp2p.New(
		libp2p.Security(noise.ID, p2pengine.NewSessionTransport),
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings("/ip6/::/tcp/2025"),
		libp2p.Transport(tcp.NewTCPTransport),
	)
	if err != nil {
		panic(err)
	}
	defer h.Close()
	// 启用 DHT
	// Construct a datastore (needed by the DHT). This is just a simple, in-memory thread-safe datastore.
	dstore := dsync.MutexWrap(ds.NewMapDatastore())

	_, err = dht.New(context.Background(), h,
		dht.Datastore(dstore),
		dht.ProtocolPrefix("/uptp"),
		dht.Mode(dht.ModeServer))
	if err != nil {
		panic(err)
	}

	// 打印主机的 Peer ID 和地址
	logger.Info("Bootstrap node is running with ID:", h.ID().String())
	for _, addr := range h.Addrs() {
		logger.Infof("Listening on: %s/p2p/%s\n", addr, h.ID().String())
	}
	select {}
}
