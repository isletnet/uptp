package main

import (
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/isletnet/uptp"
	"gopkg.in/yaml.v2"
)

type conf struct {
	uptp.NptpcConfig
}

func loadNptpcConfig(p string) (*conf, error) {
	buf, err := ioutil.ReadFile(p)
	if err != nil {
		if os.IsExist(err) {
			return nil, err
		}
	}
	nc := &conf{}
	err = yaml.Unmarshal(buf, &nc)
	if err != nil {
		return nil, err
	}
	log.Printf("nptpc config in config.yml:%+v", nc)
	return nc, nil
}

// func saveNptpcConfig(nc *conf, p string) error {
// 	buf, err := yaml.Marshal(nc)
// 	if err != nil {
// 		return err
// 	}
// 	err = ioutil.WriteFile(p, buf, 644)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

func handle666(from uint64, data []byte) {
	log.Println("recv 666: ", string(data))
}
func main() {
	nc, err := loadNptpcConfig("config.yml")
	if err != nil {
		log.Println("load nptp client config fail: ", err)
	}
	uc := uptp.NewUPTPClient(os.Args[1], nc.NptpcConfig)

	err = uc.Start()
	if err != nil {
		log.Panic(err)
	}
	uc.RegisterAppID(666, handle666)
	// for {
	// 	id := uc.GetNptpCID()
	// 	if id == 0 {
	// 		continue
	// 	}
	// 	nc.NptpcConfig.Token = id
	// 	saveNptpcConfig(nc, "config.yml")
	// 	break
	// }
	testLoop(uc, os.Args[2])
	uc.Stop()
}

func testLoop(uc *uptp.Uptpc, peer string) {
	peerID := uptp.GetIDByName(peer)
	for {
		err := uc.SendToTCP(peerID, 666, []byte("6666666666666666666666666666666"))
		if err != nil {
			log.Println("send 666 fail:", err)
		}
		time.Sleep(time.Second * 10)
	}
}
