package main

import (
	"log/slog"

	"hourglass-rejections-rpa/src/integrations/config"
	"hourglass-rejections-rpa/src/integrations/filesystem/storage"
	"hourglass-rejections-rpa/src/services/hourglass"
)

type dependencyBundle struct {
	apiClient *hourglass.Client
	analyzer  *hourglass.APIAnalyzer
	store     *storage.FileStorage
}

type authPaths struct {
	tokensPath        string
	browserProfileDir string
}

func buildDependencies(cfg *config.Config) dependencyBundle {
	paths := resolveAuthPaths(cfg)
	apiClient := newConfiguredAPIClient(cfg, paths)

	if !enableWebAuthnTokenManager(apiClient, cfg) {
		applyStaticAuthentication(apiClient, cfg, paths.tokensPath)
	}

	return dependencyBundle{
		apiClient: apiClient,
		analyzer:  hourglass.NewAPIAnalyzer(apiClient),
		store:     storage.New(cfg),
	}
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
