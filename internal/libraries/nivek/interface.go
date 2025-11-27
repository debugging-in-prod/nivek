package nivek

import (
	"github.com/sirupsen/logrus"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/abstractservice"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/config"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/conman"
)

type NivekService interface {
	abstractservice.Service

	//
	// Core NivekService

	// Postgres return a postgres connection manager
	Postgres() conman.PostgresConnectionManager

	// Logger return a logger
	Logger() *logrus.Logger

	// CommonConfig returns a common configuration object
	CommonConfig() config.Config

	// CustomConfig returns a custom configuration object
	CustomConfig() any

	// ReplaceCustomConfig sets a custom configuration object
	ReplaceCustomConfig(any)
}
