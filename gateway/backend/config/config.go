package config

type Config struct {
	Running struct {
		Port int `mapstructure:"port"`
	} `mapstructure:"running"`
	Redis struct {
		Addr     string `mapstructure:"addr"`
		Password string `mapstructure:"password"`
	} `mapstructure:"redis"`
	Mysql struct {
		DSN string `mapstructure:"dsn"`
	} `mapstructure:"mysql"`
	Kafka struct {
		Brokers []string `mapstructure:"brokers"`
		Topic   string   `mapstructure:"topic"`
	} `mapstructure:"kafka"`
	Auth struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"auth"`
	Collab struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"collab"`
	Social struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"social"`
}
