package config

type Config struct {
	Running struct {
		Port int `mapstructure:"port"`
	} `mapstructure:"running"`
	MySQL struct {
		DSN string `mapstructure:"dsn"`
	} `mapstructure:"mysql"`
	Auth struct {
		Path     string `mapstructure:"path"`
		GRPCAddr string `mapstructure:"grpc_addr"`
	} `mapstructure:"auth"`
	Collab struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"collab"`
	Social struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"social"`
	Semaphore struct {
		Limit int `mapstructure:"limit"`
	} `mapstructure:"semaphore"`
}
