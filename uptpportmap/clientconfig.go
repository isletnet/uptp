package main

import (
	"os"

	"github.com/isletnet/uptp"
	"gopkg.in/yaml.v2"
)

type conf struct {
	LogLevel int `yaml:"log_level"`

	UptpServerConfig     *uptp.UptpServerOption `yaml:"uptp_server"`
	PortMapServiceConfig *portMapServiceConf    `yaml:"port_map_service"`
	PortMapConfig        []portMapConf          `yaml:"port_map"`
}

func loadNptpcConfig(p string) (*conf, error) {
	buf, err := os.ReadFile(p)
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
	return nc, nil
}

func saveNptpcConfig(nc *conf, p string) error {
	buf, err := yaml.Marshal(nc)
	if err != nil {
		return err
	}
	err = os.WriteFile(p, buf, 644)
	if err != nil {
		return err
	}
	return nil
}
