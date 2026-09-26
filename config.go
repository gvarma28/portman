package main

type Config struct {
	Hosts []Host `yaml:"hosts"`
}

type Host struct {
	Name     string    `yaml:"name"`
	Host     string    `yaml:"host"`
	Forwards []Forward `yaml:"forwards"`
}

type Forward struct {
	Local  int `yaml:"local"`
	Remote int `yaml:"remote"`
}
