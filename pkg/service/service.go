package service

import (
	coreconfig "github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/sirupsen/logrus"
)

// Service holds shared services initialized at startup.
// It provides access to workspace discovery and notebook location services
// for all commands.
type Service struct {
	Provider        *workspace.Provider
	NotebookLocator *workspace.NotebookLocator
	Config          *coreconfig.Config
	Logger          *logrus.Entry
}

// New creates a new service instance.
func New(provider *workspace.Provider, cfg *coreconfig.Config, logger *logrus.Entry) (*Service, error) {
	locator := workspace.NewNotebookLocator(cfg)
	return &Service{
		Provider:        provider,
		NotebookLocator: locator,
		Config:          cfg,
		Logger:          logger,
	}, nil
}
