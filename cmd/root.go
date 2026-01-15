package cmd

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-core/cli"
	coreconfig "github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-skills/pkg/service"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// svc is the shared service instance initialized by PersistentPreRunE.
// It may be nil for commands that don't require workspace services.
var svc *service.Service

// Initialize creates and returns the root command with all subcommands.
// The service is initialized lazily via PersistentPreRunE when commands are executed.
func Initialize() (*cobra.Command, error) {
	rootCmd := cli.NewStandardCommand("grove-skills", "Agent Skill Integrations")

	// PersistentPreRunE initializes the shared service for all commands
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		logger := logging.NewLogger("grove-skills")

		// Load configuration (best effort - we can proceed without it)
		cfg, err := coreconfig.LoadDefault()
		if err != nil {
			cfg = &coreconfig.Config{}
			logger.Debugf("could not load grove config, proceeding with defaults: %v", err)
		}

		// Discover workspaces (best effort - we can proceed without full discovery)
		discoveryLogger := logrus.New()
		discoveryLogger.SetOutput(os.Stderr)
		discoveryLogger.SetLevel(logrus.WarnLevel)
		discoveryService := workspace.NewDiscoveryService(discoveryLogger)
		result, err := discoveryService.DiscoverAll()
		if err != nil {
			// Non-fatal: we can still function without workspace discovery
			logger.Debugf("workspace discovery failed, notebook skills will not be available: %v", err)
			result = &workspace.DiscoveryResult{}
		}
		provider := workspace.NewProvider(result)

		// Initialize the main service
		svc, err = service.New(provider, cfg, logger)
		if err != nil {
			return fmt.Errorf("failed to initialize service: %w", err)
		}
		return nil
	}

	// Add commands directly to root (no "skills" subcommand needed)
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newSkillsInstallCmd())
	rootCmd.AddCommand(newSkillsListCmd())
	rootCmd.AddCommand(newSkillsSyncCmd())
	rootCmd.AddCommand(newSkillsRemoveCmd())

	// Keep "skills" as an alias for backwards compatibility
	rootCmd.AddCommand(newSkillsCmd())

	return rootCmd, nil
}

// GetService returns the shared service instance.
// It may return nil if the service has not been initialized yet.
func GetService() *service.Service {
	return svc
}

// Execute runs the root command.
func Execute() error {
	rootCmd, err := Initialize()
	if err != nil {
		return err
	}
	return cli.Execute(rootCmd)
}
