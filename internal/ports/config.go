package ports

type Config struct {
	Hosts []Host `yaml:"hosts" json:"hosts"`
}

type Host struct {
	Name     string    `yaml:"name" json:"name"`
	Host     string    `yaml:"host" json:"host"`
	Forwards []Forward `yaml:"forwards" json:"forwards"`
}

type Forward struct {
	Local  int `yaml:"local" json:"local"`
	Remote int `yaml:"remote" json:"remote"`
}
