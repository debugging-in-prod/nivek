package main

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

var staticCoreApiConfig *CoreApiConfig

func GetCoreApiConfig() CoreApiConfig {
	if staticCoreApiConfig == nil {
		parsed := Parse()
		staticCoreApiConfig = &parsed
	}

	return *staticCoreApiConfig
}

func Parse() (config CoreApiConfig) {
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found, using environment variables")
	}
	envconfig.MustProcess("", &config)
	return config
}

type CoreApiConfig struct {
	ApiServerPort string `envconfig:"CORE_API_PORT" default:""`
	ListenAddress string `envconfig:"CORE_API_LISTEN_ADDRESS" default:""`
}
