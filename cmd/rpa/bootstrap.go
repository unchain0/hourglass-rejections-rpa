package main

import (
	"fmt"
	"log/slog"

	"hourglass-rejections-rpa/src/integrations/config"
	"hourglass-rejections-rpa/src/integrations/database/preferences"
	"hourglass-rejections-rpa/src/services/hourglass"
)

type dependencyBundle struct {
	apiClient *hourglass.Client
	analyzer  *hourglass.APIAnalyzer
	store     *preferences.Store
}

type authPaths struct {
	tokensPath        string
	browserProfileDir string
}

func openPreferenceStore(cfg *config.Config) (*preferences.Store, error) {
	if cfg.DatabaseURL != "" {
		return preferences.NewStoreFromConfig(&preferences.DatabaseConfig{
			Type: "postgres",
			DSN:  cfg.DatabaseURL,
		})
	}

	return preferences.NewStoreFromConfig(nil)
}

var newPreferenceStore = openPreferenceStore

func buildDependencies(cfg *config.Config) (dependencyBundle, error) {
	paths := resolveAuthPaths(cfg)
	apiClient := newConfiguredAPIClient(cfg, paths)
	store, err := newPreferenceStore(cfg)
	if err != nil {
		return dependencyBundle{}, fmt.Errorf("failed to initialize database store: %w", err)
	}

	if !enableWebAuthnTokenManager(apiClient, cfg) {
		applyStaticAuthentication(apiClient, cfg, paths.tokensPath)
	}

	return dependencyBundle{
		apiClient: apiClient,
		analyzer:  hourglass.NewAPIAnalyzer(apiClient),
		store:     store,
	}, nil
}

func resolveAuthPaths(cfg *config.Config) authPaths {
	return authPaths{
		tokensPath:        resolveTokensPath(cfg),
		browserProfileDir: resolveChromeProfileDir(cfg),
	}
}

func newConfiguredAPIClient(cfg *config.Config, paths authPaths) *hourglass.Client {
	apiClient := hourglass.NewClient()
	apiClient.SetBaseURL(cfg.HourglassURL)

	if paths.tokensPath != "" {
		apiClient.SetWebAuthnTokensPath(paths.tokensPath)
	}

	if paths.browserProfileDir != "" {
		apiClient.SetBrowserProfileDir(paths.browserProfileDir)
	}

	return apiClient
}

func applyStaticAuthentication(apiClient *hourglass.Client, cfg *config.Config, tokensPath string) {
	if cfg.HourglassXSRFToken != "" && cfg.HourglassHGLogin != "" {
		apiClient.SetXSRFToken(cfg.HourglassXSRFToken)
		apiClient.SetHGLogin(cfg.HourglassHGLogin)
		slog.Info("using tokens from environment variables")
		return
	}

	if tokensPath == "" {
		return
	}

	if err := apiClient.LoadTokensFromFile(tokensPath); err != nil {
		slog.Warn("failed to load tokens from file", "path", tokensPath, "error", err)
		return
	}

	slog.Info("loaded tokens from file", "path", tokensPath)
}
