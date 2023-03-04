package main

import (
	"github.com/isletnet/uptp"
)

func main() {
	us := uptp.NewUPTPServer(uptp.NptpsConfig{
		Udp6Addr: "[::]:1929",
		SnowNode: 1,
	})

	us.Start()
	defer us.Stop()

	us.Wait()
}
