package config

type ServiceMapping struct {
	Name       string
	SourcePort int
	TargetAddr string
	TargetPort int
}

type config struct {
	TSHostname  string `env:"TS_HOSTNAME,required"`
	TSAuthKey   string `env:"TS_AUTHKEY,required"`
	TSExtraArgs string `env:"TS_EXTRA_ARGS"`

	ServiceMappings []ServiceMapping
}
