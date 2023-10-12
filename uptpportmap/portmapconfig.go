package main

type portMapServiceConf struct {
	NodeName  string `yaml:"node_name"`
	ServiceID uint32 `yaml:"service_id"`
	ForwardID uint32 `yaml:"forward_id"`
}

type PortMapItem struct {
	Name       string `yaml:"name"`
	Protocol   string `yaml:"protocol"`
	LocalPort  int    `yaml:"local_port"`
	TargetAddr string `yaml:"target_addr"`
	TargetPort int    `yaml:"target_port"`
}

type portMapConf struct {
	Peer      string        `yaml:"peer"`
	ServiceID uint32        `yaml:"service_id"`
	Name      string        `yaml:"name"`
	MapList   []PortMapItem `yaml:"map_list"`
}
