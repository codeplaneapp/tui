package config

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/agent/hyper"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/env"
	"github.com/charmbracelet/crush/internal/fsext"
	"github.com/charmbracelet/crush/internal/home"
	powernapConfig "github.com/charmbracelet/x/powernap/pkg/config"
	"github.com/qjebbs/go-jsons"
)

const defaultCatwalkURL = "https://catwalk.charm.sh"

// Load loads the configuration from the default paths and returns a
// ConfigStore that owns both the pure-data Config and all runtime state.
func Load(workingDir, dataDir string, debug bool) (*ConfigStore, error) {
	configPaths := lookupConfigs(workingDir)

	cfg, err := loadFromConfigPaths(configPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from paths %v: %w", configPaths, err)
	}

	cfg.setDefaults(workingDir, dataDir)

	store := &ConfigStore{
		config:         cfg,
		workingDir:     workingDir,
		globalDataPath: GlobalConfigData(),
		workspacePath:  workspaceConfigPath(cfg.Options.DataDirectory),
	}

	if debug {
		cfg.Options.Debug = true
	}

	// Load workspace config last so it has highest priority.
	if wsData, err := os.ReadFile(store.workspacePath); err == nil && len(wsData) > 0 {
		merged, mergeErr := loadFromBytes(append([][]byte{mustMarshalConfig(cfg)}, wsData))
		if mergeErr == nil {
			// Preserve defaults that setDefaults already applied.
			dataDir := cfg.Options.DataDirectory
			*cfg = *merged
			cfg.setDefaults(workingDir, dataDir)
			store.config = cfg
		}
	}

	if !isInsideWorktree() {
		const depth = 2
		const items = 100
		slog.Warn("No git repository detected in working directory, will limit file walk operations", "depth", depth, "items", items)
		assignIfNil(&cfg.Tools.Ls.MaxDepth, depth)
		assignIfNil(&cfg.Tools.Ls.MaxItems, items)
		assignIfNil(&cfg.Options.TUI.Completions.MaxDepth, depth)
		assignIfNil(&cfg.Options.TUI.Completions.MaxItems, items)
	}

	if isAppleTerminal() {
		slog.Warn("Detected Apple Terminal, enabling transparent mode")
		assignIfNil(&cfg.Options.TUI.Transparent, true)
	}

	// Load known providers, this loads the config from catwalk
	providers, err := Providers(cfg)
	if err != nil {
		return nil, err
	}
	store.knownProviders = providers

	env := env.New()
	// Configure providers
	valueResolver := NewShellVariableResolver(env)
	store.resolver = valueResolver
	if err := cfg.configureProviders(store, env, valueResolver, store.knownProviders); err != nil {
		return nil, fmt.Errorf("failed to configure providers: %w", err)
	}

	if !cfg.IsConfigured() {
		slog.Warn("No providers configured")
		return store, nil
	}

	if err := configureSelectedModels(store, store.knownProviders); err != nil {
		return nil, fmt.Errorf("failed to configure selected models: %w", err)
	}
	store.SetupAgents()
	return store, nil
}

// mustMarshalConfig marshals the config to JSON bytes, returning empty JSON on
// error.
func mustMarshalConfig(cfg *Config) []byte {
	data, err := json.Marshal(cfg)
	if err != nil {
		return []byte("{}")
	}
	return data
}

type envValue struct {
	value   string
	present bool
}

func PushPopCodeplaneEnv() func() {
	prefixes := []string{"CRUSH_", "SMITHERS_TUI_", "CODEPLANE_"}
	found := make(map[string]struct{})
	for _, ev := range os.Environ() {
		for _, prefix := range prefixes {
			if strings.HasPrefix(ev, prefix) {
				pair := strings.SplitN(ev, "=", 2)
				if len(pair) != 2 {
					continue
				}
				found[strings.TrimPrefix(pair[0], prefix)] = struct{}{}
			}
		}
	}

	backups := make(map[string]envValue, len(found))
	for ev := range found {
		value, present := os.LookupEnv(ev)
		backups[ev] = envValue{value: value, present: present}
	}

	for ev := range found {
		if value, ok := lookupPrefixedEnvValue("CODEPLANE_", ev); ok {
			os.Setenv(ev, value)
			continue
		}
		if value, ok := lookupPrefixedEnvValue("SMITHERS_TUI_", ev); ok {
			os.Setenv(ev, value)
			continue
		}
		if value, ok := lookupPrefixedEnvValue("CRUSH_", ev); ok {
			os.Setenv(ev, value)
		}
	}

	return func() {
		for k, v := range backups {
			if v.present {
				os.Setenv(k, v.value)
				continue
			}
			os.Unsetenv(k)
		}
	}
}

func lookupPrefixedEnvValue(prefix, name string) (string, bool) {
	return os.LookupEnv(prefix + name)
}

func (c *Config) configureProviders(store *ConfigStore, env env.Env, resolver VariableResolver, knownProviders []catwalk.Provider) error {
	knownProviderNames := make(map[string]bool)
	restore := PushPopCodeplaneEnv()
	defer restore()

	// When disable_default_providers is enabled, skip all default/embedded
	// providers entirely. Users must fully specify any providers they want.
	// We skip to the custom provider validation loop which handles all
	// user-configured providers uniformly.
	if c.Options.DisableDefaultProviders {
		knownProviders = nil
	}

	for _, p := range knownProviders {
		knownProviderNames[string(p.ID)] = true
		config, configExists := c.Providers.Get(string(p.ID))
		// if the user configured a known provider we need to allow it to override a couple of parameters
		if configExists {
			if config.BaseURL != "" {
				p.APIEndpoint = config.BaseURL
			}
			if config.APIKey != "" {
				p.APIKey = config.APIKey
			}
			if len(config.Models) > 0 {
				models := []catwalk.Model{}
				seen := make(map[string]bool)

				for _, model := range config.Models {
					if seen[model.ID] {
						continue
					}
					seen[model.ID] = true
					if model.Name == "" {
						model.Name = model.ID
					}
					models = append(models, model)
				}
				for _, model := range p.Models {
					if seen[model.ID] {
						continue
					}
					seen[model.ID] = true
					if model.Name == "" {
						model.Name = model.ID
					}
					models = append(models, model)
				}

				p.Models = models
			}
		}

		headers := map[string]string{}
		if len(p.DefaultHeaders) > 0 {
			maps.Copy(headers, p.DefaultHeaders)
		}
		if len(config.ExtraHeaders) > 0 {
			maps.Copy(headers, config.ExtraHeaders)
		}
		for k, v := range headers {
			resolved, err := resolver.ResolveValue(v)
			if err != nil {
				slog.Error("Could not resolve provider header", "err", err.Error())
				continue
			}
			headers[k] = resolved
		}
		prepared := ProviderConfig{
			ID:                 string(p.ID),
			Name:               p.Name,
			BaseURL:            p.APIEndpoint,
			APIKey:             p.APIKey,
			APIKeyTemplate:     p.APIKey, // Store original template for re-resolution
			OAuthToken:         config.OAuthToken,
			Type:               p.Type,
			Disable:            config.Disable,
			SystemPromptPrefix: config.SystemPromptPrefix,
			ExtraHeaders:       headers,
			ExtraBody:          config.ExtraBody,
			ExtraParams:        make(map[string]string),
			Models:             p.Models,
		}

		switch {
		case p.ID == catwalk.InferenceProviderAnthropic && config.OAuthToken != nil:
			// Claude Code subscription is not supported anymore. Remove to show onboarding.
			store.RemoveConfigField(ScopeGlobal, "providers.anthropic")
			c.Providers.Del(string(p.ID))
			continue
		case p.ID == catwalk.InferenceProviderCopilot && config.OAuthToken != nil:
			prepared.SetupGitHubCopilot()
		}

		switch p.ID {
		// Handle specific providers that require additional configuration
		case catwalk.InferenceProviderVertexAI:
			var (
				project  = env.Get("VERTEXAI_PROJECT")
				location = env.Get("VERTEXAI_LOCATION")
			)
			if project == "" || location == "" {
				if configExists {
					slog.Warn("Skipping Vertex AI provider due to missing credentials")
					c.Providers.Del(string(p.ID))
				}
				continue
			}
			prepared.ExtraParams["project"] = project
			prepared.ExtraParams["location"] = location
		case catwalk.InferenceProviderAzure:
			endpoint, err := resolver.ResolveValue(p.APIEndpoint)
			if err != nil || endpoint == "" {
				if configExists {
					slog.Warn("Skipping Azure provider due to missing API endpoint", "provider", p.ID, "error", err)
					c.Providers.Del(string(p.ID))
				}
				continue
			}
			prepared.BaseURL = endpoint
			prepared.ExtraParams["apiVersion"] = env.Get("AZURE_OPENAI_API_VERSION")
		case catwalk.InferenceProviderBedrock:
			if !hasAWSCredentials(env) {
				if configExists {
					slog.Warn("Skipping Bedrock provider due to missing AWS credentials")
					c.Providers.Del(string(p.ID))
				}
				continue
			}
			prepared.ExtraParams["region"] = env.Get("AWS_REGION")
			if prepared.ExtraParams["region"] == "" {
				prepared.ExtraParams["region"] = env.Get("AWS_DEFAULT_REGION")
			}
			for _, model := range p.Models {
				if !strings.HasPrefix(model.ID, "anthropic.") {
					return fmt.Errorf("bedrock provider only supports anthropic models for now, found: %s", model.ID)
				}
			}
		default:
			// if the provider api or endpoint are missing we skip them
			v, err := resolver.ResolveValue(p.APIKey)
			if v == "" || err != nil {
				if configExists {
					slog.Warn("Skipping provider due to missing API key", "provider", p.ID)
					c.Providers.Del(string(p.ID))
				}
				continue
			}
		}
		c.Providers.Set(string(p.ID), prepared)
	}

	// validate the custom providers
	for id, providerConfig := range c.Providers.Seq2() {
		if knownProviderNames[id] {
			continue
		}

		// Make sure the provider ID is set
		providerConfig.ID = id
		providerConfig.Name = cmp.Or(providerConfig.Name, id) // Use ID as name if not set
		// default to OpenAI if not set
		providerConfig.Type = cmp.Or(providerConfig.Type, catwalk.TypeOpenAICompat)
		if !slices.Contains(catwalk.KnownProviderTypes(), providerConfig.Type) && providerConfig.Type != hyper.Name {
			slog.Warn("Skipping custom provider due to unsupported provider type", "provider", id)
			c.Providers.Del(id)
			continue
		}

		if providerConfig.Disable {
			slog.Debug("Skipping custom provider due to disable flag", "provider", id)
			c.Providers.Del(id)
			continue
		}
		if providerConfig.APIKey == "" {
			slog.Warn("Provider is missing API key, this might be OK for local providers", "provider", id)
		}
		if providerConfig.BaseURL == "" {
			slog.Warn("Skipping custom provider due to missing API endpoint", "provider", id)
			c.Providers.Del(id)
			continue
		}
		if len(providerConfig.Models) == 0 {
			slog.Warn("Skipping custom provider because the provider has no models", "provider", id)
			c.Providers.Del(id)
			continue
		}
		apiKey, err := resolver.ResolveValue(providerConfig.APIKey)
		if apiKey == "" || err != nil {
			slog.Warn("Provider is missing API key, this might be OK for local providers", "provider", id)
		}
		baseURL, err := resolver.ResolveValue(providerConfig.BaseURL)
		if baseURL == "" || err != nil {
			slog.Warn("Skipping custom provider due to missing API endpoint", "provider", id, "error", err)
			c.Providers.Del(id)
			continue
		}

		for k, v := range providerConfig.ExtraHeaders {
			resolved, err := resolver.ResolveValue(v)
			if err != nil {
				slog.Error("Could not resolve provider header", "err", err.Error())
				continue
			}
			providerConfig.ExtraHeaders[k] = resolved
		}

		c.Providers.Set(id, providerConfig)
	}

	if c.Providers.Len() == 0 && c.Options.DisableDefaultProviders {
		return fmt.Errorf("default providers are disabled and there are no custom providers are configured")
	}

	return nil
}

func (c *Config) setDefaults(workingDir, dataDir string) {
	if c.Options == nil {
		c.Options = &Options{}
	}
	if c.Options.TUI == nil {
		c.Options.TUI = &TUIOptions{}
	}
	if dataDir != "" {
		c.Options.DataDirectory = dataDir
	} else if c.Options.DataDirectory == "" {
		if path, ok := fsext.LookupClosest(workingDir, defaultDataDirectory); ok {
			c.Options.DataDirectory = path
		} else {
			for _, legacyDataDirectory := range legacyDataDirectories {
				if path, ok := fsext.LookupClosest(workingDir, legacyDataDirectory); ok {
					c.Options.DataDirectory = path
					break
				}
			}
			if c.Options.DataDirectory == "" {
				c.Options.DataDirectory = filepath.Join(workingDir, defaultDataDirectory)
			}
		}
	}
	if c.Providers == nil {
		c.Providers = csync.NewMap[string, ProviderConfig]()
	}
	if c.Models == nil {
		c.Models = make(map[SelectedModelType]SelectedModel)
	}
	if c.RecentModels == nil {
		c.RecentModels = make(map[SelectedModelType][]SelectedModel)
	}
	if c.MCP == nil {
		c.MCP = make(map[string]MCPConfig)
	}
	if c.LSP == nil {
		c.LSP = make(map[string]LSPConfig)
	}
	smithersMode := c.Smithers != nil
	if !smithersMode && isSmithersProject(workingDir) {
		c.Smithers = &SmithersConfig{}
		smithersMode = true
	}
	if smithersMode {
		defaultDBPath, defaultWorkflowDir := defaultSmithersPaths(workingDir)
		c.Smithers.DBPath = cmp.Or(
			c.Smithers.DBPath,
			defaultDBPath,
		)
		c.Smithers.WorkflowDir = cmp.Or(
			c.Smithers.WorkflowDir,
			defaultWorkflowDir,
		)

		// Default the large model to claude-opus-4-6 when workflow config is
		// present and the user has not explicitly chosen a large model.
		if _, ok := c.Models[SelectedModelTypeLarge]; !ok {
			if c.Models == nil {
				c.Models = make(map[SelectedModelType]SelectedModel)
			}
			c.Models[SelectedModelTypeLarge] = SelectedModel{
				Model:    "claude-opus-4-6",
				Provider: "anthropic",
				Think:    true,
			}
		}

		// Add default workflow MCP server if not already configured by user.
		if _, exists := c.MCP[SmithersMCPName]; !exists {
			c.MCP[SmithersMCPName] = DefaultSmithersMCPConfig()
		}

		// Apply default disabled tools if user hasn't set any.
		if c.Options.DisabledTools == nil {
			c.Options.DisabledTools = DefaultDisabledTools()
		}
	}

	// Apply defaults to LSP configurations
	c.applyLSPDefaults()

	// Add the default context paths if they are not already present
	c.Options.ContextPaths = append(defaultContextPaths, c.Options.ContextPaths...)
	slices.Sort(c.Options.ContextPaths)
	c.Options.ContextPaths = slices.Compact(c.Options.ContextPaths)

	// Add the default skills directories if not already present.
	for _, dir := range GlobalSkillsDirs() {
		if !slices.Contains(c.Options.SkillsPaths, dir) {
			c.Options.SkillsPaths = append(c.Options.SkillsPaths, dir)
		}
	}

	// Project specific skills dirs.
	c.Options.SkillsPaths = append(c.Options.SkillsPaths, ProjectSkillsDir(workingDir)...)

	if str := envWithFallback("CODEPLANE_DISABLE_PROVIDER_AUTO_UPDATE", "SMITHERS_TUI_DISABLE_PROVIDER_AUTO_UPDATE", "CRUSH_DISABLE_PROVIDER_AUTO_UPDATE"); str != "" {
		c.Options.DisableProviderAutoUpdate, _ = strconv.ParseBool(str)
	}

	if str := envWithFallback("CODEPLANE_DISABLE_DEFAULT_PROVIDERS", "SMITHERS_TUI_DISABLE_DEFAULT_PROVIDERS", "CRUSH_DISABLE_DEFAULT_PROVIDERS"); str != "" {
		c.Options.DisableDefaultProviders, _ = strconv.ParseBool(str)
	}

	if c.Options.Attribution == nil {
		c.Options.Attribution = &Attribution{
			TrailerStyle:  TrailerStyleAssistedBy,
			GeneratedWith: true,
		}
	} else if c.Options.Attribution.TrailerStyle == "" {
		// Migrate deprecated co_authored_by or apply default
		if c.Options.Attribution.CoAuthoredBy != nil {
			if *c.Options.Attribution.CoAuthoredBy {
				c.Options.Attribution.TrailerStyle = TrailerStyleCoAuthoredBy
			} else {
				c.Options.Attribution.TrailerStyle = TrailerStyleNone
			}
		} else {
			c.Options.Attribution.TrailerStyle = TrailerStyleAssistedBy
		}
	}

	if c.Options.Observability == nil {
		c.Options.Observability = &ObservabilityOptions{}
	}
	if c.Options.Observability.TraceBufferSize <= 0 {
		c.Options.Observability.TraceBufferSize = 512
	}
	assignIfNil(&c.Options.Observability.TraceSampleRatio, 1.0)
	assignIfNil(&c.Options.Observability.OTLPInsecure, false)

	if str := envWithFallback("CODEPLANE_OBSERVABILITY_ADDR", "SMITHERS_TUI_OBSERVABILITY_ADDR", "CRUSH_OBSERVABILITY_ADDR"); str != "" {
		c.Options.Observability.Address = str
	}
	if str := envWithFallback("CODEPLANE_TRACE_BUFFER_SIZE", "SMITHERS_TUI_TRACE_BUFFER_SIZE", "CRUSH_TRACE_BUFFER_SIZE"); str != "" {
		if size, err := strconv.Atoi(str); err == nil && size > 0 {
			c.Options.Observability.TraceBufferSize = size
		}
	}
	if str := envWithFallback("CODEPLANE_TRACE_SAMPLE_RATIO", "SMITHERS_TUI_TRACE_SAMPLE_RATIO", "CRUSH_TRACE_SAMPLE_RATIO"); str != "" {
		if ratio, err := strconv.ParseFloat(str, 64); err == nil {
			ratio = max(0, min(1, ratio))
			c.Options.Observability.TraceSampleRatio = &ratio
		}
	}
	if str := envWithFallback("CODEPLANE_OTLP_ENDPOINT", "SMITHERS_TUI_OTLP_ENDPOINT", "CRUSH_OTLP_ENDPOINT"); str != "" {
		c.Options.Observability.OTLPEndpoint = str
	}
	if str := envWithFallback("CODEPLANE_OTLP_INSECURE", "SMITHERS_TUI_OTLP_INSECURE", "CRUSH_OTLP_INSECURE"); str != "" {
		if insecure, err := strconv.ParseBool(str); err == nil {
			c.Options.Observability.OTLPInsecure = &insecure
		}
	}
	if str := envWithFallback("CODEPLANE_OTLP_HEADERS", "SMITHERS_TUI_OTLP_HEADERS", "CRUSH_OTLP_HEADERS"); str != "" {
		headers := parseKeyValueEnv(str)
		if len(headers) > 0 {
			if c.Options.Observability.OTLPHeaders == nil {
				c.Options.Observability.OTLPHeaders = map[string]string{}
			}
			maps.Copy(c.Options.Observability.OTLPHeaders, headers)
		}
	}

	c.Options.InitializeAs = cmp.Or(c.Options.InitializeAs, defaultInitializeAs)
}

func isSmithersProject(workingDir string) bool {
	_, _, ok := lookupSmithersProjectDir(workingDir)
	return ok
}

func defaultSmithersPaths(workingDir string) (dbPath, workflowDir string) {
	if dir, modern, ok := lookupSmithersProjectDir(workingDir); ok {
		if modern {
			return filepath.Join(dir, "codeplane.db"), filepath.Join(dir, "workflows")
		}
		return filepath.Join(dir, "smithers.db"), filepath.Join(dir, "workflows")
	}

	return filepath.Join(workingDir, defaultDataDirectory, "codeplane.db"),
		filepath.Join(workingDir, defaultDataDirectory, "workflows")
}

func lookupSmithersProjectDir(workingDir string) (path string, modern bool, ok bool) {
	// Require workflows/ subdir to distinguish Smithers projects from normal
	// projects that just have a .codeplane data directory.
	if path, ok := fsext.LookupClosest(workingDir, filepath.Join(defaultDataDirectory, "workflows")); ok {
		return filepath.Dir(path), true, true
	}
	if path, ok := fsext.LookupClosest(workingDir, filepath.Join(".smithers", "workflows")); ok {
		return filepath.Dir(path), false, true
	}
	return "", false, false
}

func parseKeyValueEnv(raw string) map[string]string {
	headers := make(map[string]string)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		headers[key] = value
	}
	return headers
}

// applyLSPDefaults applies default values from powernap to LSP configurations
func (c *Config) applyLSPDefaults() {
	// Get powernap's default configuration
	configManager := powernapConfig.NewManager()
	configManager.LoadDefaults()

	// Apply defaults to each LSP configuration
	for name, cfg := range c.LSP {
		// Try to get defaults from powernap based on name or command name.
		base, ok := configManager.GetServer(name)
		if !ok {
			base, ok = configManager.GetServer(cfg.Command)
			if !ok {
				continue
			}
		}
		if cfg.Options == nil {
			cfg.Options = base.Settings
		}
		if cfg.InitOptions == nil {
			cfg.InitOptions = base.InitOptions
		}
		if len(cfg.FileTypes) == 0 {
			cfg.FileTypes = base.FileTypes
		}
		if len(cfg.RootMarkers) == 0 {
			cfg.RootMarkers = base.RootMarkers
		}
		cfg.Command = cmp.Or(cfg.Command, base.Command)
		if len(cfg.Args) == 0 {
			cfg.Args = base.Args
		}
		if len(cfg.Env) == 0 {
			cfg.Env = base.Environment
		}
		// Update the config in the map
		c.LSP[name] = cfg
	}
}

func (c *Config) defaultModelSelection(knownProviders []catwalk.Provider) (largeModel SelectedModel, smallModel SelectedModel, err error) {
	if len(knownProviders) == 0 && c.Providers.Len() == 0 {
		err = fmt.Errorf("no providers configured, please configure at least one provider")
		return largeModel, smallModel, err
	}

	// Use the first provider enabled based on the known providers order
	// if no provider found that is known use the first provider configured
	for _, p := range knownProviders {
		providerConfig, ok := c.Providers.Get(string(p.ID))
		if !ok || providerConfig.Disable {
			continue
		}
		defaultLargeModel := c.GetModel(string(p.ID), p.DefaultLargeModelID)
		if defaultLargeModel == nil {
			err = fmt.Errorf("default large model %s not found for provider %s", p.DefaultLargeModelID, p.ID)
			return largeModel, smallModel, err
		}
		largeModel = SelectedModel{
			Provider:        string(p.ID),
			Model:           defaultLargeModel.ID,
			MaxTokens:       defaultLargeModel.DefaultMaxTokens,
			ReasoningEffort: defaultLargeModel.DefaultReasoningEffort,
		}

		defaultSmallModel := c.GetModel(string(p.ID), p.DefaultSmallModelID)
		if defaultSmallModel == nil {
			err = fmt.Errorf("default small model %s not found for provider %s", p.DefaultSmallModelID, p.ID)
			return largeModel, smallModel, err
		}
		smallModel = SelectedModel{
			Provider:        string(p.ID),
			Model:           defaultSmallModel.ID,
			MaxTokens:       defaultSmallModel.DefaultMaxTokens,
			ReasoningEffort: defaultSmallModel.DefaultReasoningEffort,
		}
		return largeModel, smallModel, err
	}

	enabledProviders := c.EnabledProviders()
	slices.SortFunc(enabledProviders, func(a, b ProviderConfig) int {
		return strings.Compare(a.ID, b.ID)
	})

	if len(enabledProviders) == 0 {
		err = fmt.Errorf("no providers configured, please configure at least one provider")
		return largeModel, smallModel, err
	}

	providerConfig := enabledProviders[0]
	if len(providerConfig.Models) == 0 {
		err = fmt.Errorf("provider %s has no models configured", providerConfig.ID)
		return largeModel, smallModel, err
	}
	defaultLargeModel := c.GetModel(providerConfig.ID, providerConfig.Models[0].ID)
	largeModel = SelectedModel{
		Provider:  providerConfig.ID,
		Model:     defaultLargeModel.ID,
		MaxTokens: defaultLargeModel.DefaultMaxTokens,
	}
	defaultSmallModel := c.GetModel(providerConfig.ID, providerConfig.Models[0].ID)
	smallModel = SelectedModel{
		Provider:  providerConfig.ID,
		Model:     defaultSmallModel.ID,
		MaxTokens: defaultSmallModel.DefaultMaxTokens,
	}
	return largeModel, smallModel, err
}

func configureSelectedModels(store *ConfigStore, knownProviders []catwalk.Provider) error {
	c := store.config
	defaultLarge, defaultSmall, err := c.defaultModelSelection(knownProviders)
	if err != nil {
		return fmt.Errorf("failed to select default models: %w", err)
	}
	large, small := defaultLarge, defaultSmall

	largeModelSelected, largeModelConfigured := c.Models[SelectedModelTypeLarge]
	if largeModelConfigured {
		if largeModelSelected.Model != "" {
			large.Model = largeModelSelected.Model
		}
		if largeModelSelected.Provider != "" {
			large.Provider = largeModelSelected.Provider
		}
		model := c.GetModel(large.Provider, large.Model)
		if model == nil {
			large = defaultLarge
			// override the model type to large
			err := store.UpdatePreferredModel(ScopeGlobal, SelectedModelTypeLarge, large)
			if err != nil {
				return fmt.Errorf("failed to update preferred large model: %w", err)
			}
		} else {
			if largeModelSelected.MaxTokens > 0 {
				large.MaxTokens = largeModelSelected.MaxTokens
			} else {
				large.MaxTokens = model.DefaultMaxTokens
			}
			if largeModelSelected.ReasoningEffort != "" {
				large.ReasoningEffort = largeModelSelected.ReasoningEffort
			}
			large.Think = largeModelSelected.Think
			if largeModelSelected.Temperature != nil {
				large.Temperature = largeModelSelected.Temperature
			}
			if largeModelSelected.TopP != nil {
				large.TopP = largeModelSelected.TopP
			}
			if largeModelSelected.TopK != nil {
				large.TopK = largeModelSelected.TopK
			}
			if largeModelSelected.FrequencyPenalty != nil {
				large.FrequencyPenalty = largeModelSelected.FrequencyPenalty
			}
			if largeModelSelected.PresencePenalty != nil {
				large.PresencePenalty = largeModelSelected.PresencePenalty
			}
		}
	}
	smallModelSelected, smallModelConfigured := c.Models[SelectedModelTypeSmall]
	if smallModelConfigured {
		if smallModelSelected.Model != "" {
			small.Model = smallModelSelected.Model
		}
		if smallModelSelected.Provider != "" {
			small.Provider = smallModelSelected.Provider
		}

		model := c.GetModel(small.Provider, small.Model)
		if model == nil {
			small = defaultSmall
			// override the model type to small
			err := store.UpdatePreferredModel(ScopeGlobal, SelectedModelTypeSmall, small)
			if err != nil {
				return fmt.Errorf("failed to update preferred small model: %w", err)
			}
		} else {
			if smallModelSelected.MaxTokens > 0 {
				small.MaxTokens = smallModelSelected.MaxTokens
			} else {
				small.MaxTokens = model.DefaultMaxTokens
			}
			if smallModelSelected.ReasoningEffort != "" {
				small.ReasoningEffort = smallModelSelected.ReasoningEffort
			}
			if smallModelSelected.Temperature != nil {
				small.Temperature = smallModelSelected.Temperature
			}
			if smallModelSelected.TopP != nil {
				small.TopP = smallModelSelected.TopP
			}
			if smallModelSelected.TopK != nil {
				small.TopK = smallModelSelected.TopK
			}
			if smallModelSelected.FrequencyPenalty != nil {
				small.FrequencyPenalty = smallModelSelected.FrequencyPenalty
			}
			if smallModelSelected.PresencePenalty != nil {
				small.PresencePenalty = smallModelSelected.PresencePenalty
			}
			small.Think = smallModelSelected.Think
		}
	}
	c.Models[SelectedModelTypeLarge] = large
	c.Models[SelectedModelTypeSmall] = small
	return nil
}

// lookupConfigs searches config files recursively from CWD up to FS root
func lookupConfigs(cwd string) []string {
	// prepend default config paths
	configPaths := []string{
		GlobalConfig(),
		GlobalConfigData(),
	}

	configNames := []string{appName + ".json", "." + appName + ".json"}
	for _, legacyAppName := range legacyAppNames {
		configNames = append(configNames, legacyAppName+".json", "."+legacyAppName+".json")
	}

	foundConfigs, err := fsext.Lookup(cwd, configNames...)
	if err != nil {
		// returns at least default configs
		return configPaths
	}

	// reverse order so last config has more priority
	slices.Reverse(foundConfigs)

	return append(configPaths, foundConfigs...)
}

func loadFromConfigPaths(configPaths []string) (*Config, error) {
	var configs [][]byte

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to open config file %s: %w", path, err)
		}
		if len(data) == 0 {
			continue
		}
		if isLegacyConfigPath(path) {
			slog.Warn("Loaded legacy config file; please migrate to Codeplane naming",
				"path", path)
		}
		configs = append(configs, data)
	}

	return loadFromBytes(configs)
}

func loadFromBytes(configs [][]byte) (*Config, error) {
	if len(configs) == 0 {
		return &Config{}, nil
	}

	data, err := jsons.Merge(configs)
	if err != nil {
		return nil, err
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func hasAWSCredentials(env env.Env) bool {
	if env.Get("AWS_BEARER_TOKEN_BEDROCK") != "" {
		return true
	}

	if env.Get("AWS_ACCESS_KEY_ID") != "" && env.Get("AWS_SECRET_ACCESS_KEY") != "" {
		return true
	}

	if env.Get("AWS_PROFILE") != "" || env.Get("AWS_DEFAULT_PROFILE") != "" {
		return true
	}

	if env.Get("AWS_REGION") != "" || env.Get("AWS_DEFAULT_REGION") != "" {
		return true
	}

	if env.Get("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
		env.Get("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" {
		return true
	}

	if _, err := os.Stat(filepath.Join(home.Dir(), ".aws/credentials")); err == nil && !testing.Testing() {
		return true
	}

	return false
}

// GlobalConfig returns the global configuration file path for the application.
func GlobalConfig() string {
	if globalConfig := envWithFallback("CODEPLANE_GLOBAL_CONFIG", "SMITHERS_TUI_GLOBAL_CONFIG", "CRUSH_GLOBAL_CONFIG"); globalConfig != "" {
		return preferExistingPath("global_config", namedPathsInDir(globalConfig)...)
	}

	return preferExistingPath("global_config", configPathsFor(home.Config())...)
}

// GlobalCacheDir returns the path to the global cache directory for the
// application.
func GlobalCacheDir() string {
	if cacheDir := envWithFallback("CODEPLANE_CACHE_DIR", "SMITHERS_TUI_CACHE_DIR", "CRUSH_CACHE_DIR"); cacheDir != "" {
		return cacheDir
	}
	if xdgCacheHome := os.Getenv("XDG_CACHE_HOME"); xdgCacheHome != "" {
		return filepath.Join(xdgCacheHome, appName)
	}
	if runtime.GOOS == "windows" {
		localAppData := cmp.Or(
			os.Getenv("LOCALAPPDATA"),
			filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local"),
		)
		return filepath.Join(localAppData, appName, "cache")
	}
	return filepath.Join(home.Dir(), ".cache", appName)
}

// GlobalConfigData returns the path to the main data directory for the application.
// this config is used when the app overrides configurations instead of updating the global config.
func GlobalConfigData() string {
	if globalData := envWithFallback("CODEPLANE_GLOBAL_DATA", "SMITHERS_TUI_GLOBAL_DATA", "CRUSH_GLOBAL_DATA"); globalData != "" {
		return preferExistingPath("global_data", namedPathsInDir(globalData)...)
	}
	if xdgDataHome := os.Getenv("XDG_DATA_HOME"); xdgDataHome != "" {
		return preferExistingPath("global_data", dataPathsFor(xdgDataHome)...)
	}

	// Return the path to the main data directory.
	// For windows, it should be in `%LOCALAPPDATA%/codeplane/`.
	// For linux and macOS, it should be in `$HOME/.local/share/codeplane/`.
	if runtime.GOOS == "windows" {
		localAppData := cmp.Or(
			os.Getenv("LOCALAPPDATA"),
			filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local"),
		)
		return preferExistingPath("global_data", dataPathsFor(localAppData)...)
	}

	return preferExistingPath("global_data", dataPathsFor(filepath.Join(home.Dir(), ".local", "share"))...)
}

// GlobalWorkspaceDir returns the path to the global server workspace
// directory. This directory acts as a meta-workspace for the server
// process, giving it a real workingDir so that config loading, scoped
// writes, and provider resolution behave identically to project
// workspaces.
func GlobalWorkspaceDir() string {
	return filepath.Dir(GlobalConfigData())
}

func assignIfNil[T any](ptr **T, val T) {
	if *ptr == nil {
		*ptr = &val
	}
}

func isInsideWorktree() bool {
	bts, err := exec.CommandContext(
		context.Background(),
		"git", "rev-parse",
		"--is-inside-work-tree",
	).CombinedOutput()
	return err == nil && strings.TrimSpace(string(bts)) == "true"
}

// GlobalSkillsDirs returns the default directories for Agent Skills.
// Skills in these directories are auto-discovered and their files can be read
// without permission prompts.
func GlobalSkillsDirs() []string {
	if skillsDir := envWithFallback("CODEPLANE_SKILLS_DIR", "SMITHERS_TUI_SKILLS_DIR", "CRUSH_SKILLS_DIR"); skillsDir != "" {
		return []string{skillsDir}
	}

	paths := []string{
		filepath.Join(home.Config(), appName, "skills"),
		filepath.Join(home.Config(), "agents", "skills"),
	}
	for _, legacyAppName := range legacyAppNames {
		paths = append(paths, filepath.Join(home.Config(), legacyAppName, "skills"))
	}

	// On Windows, also load from app data on top of `$HOME/.config/codeplane`.
	// This is here mostly for backwards compatibility.
	if runtime.GOOS == "windows" {
		appData := cmp.Or(
			os.Getenv("LOCALAPPDATA"),
			filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local"),
		)
		paths = append(
			paths,
			filepath.Join(appData, appName, "skills"),
			filepath.Join(appData, "agents", "skills"),
		)
		for _, legacyAppName := range legacyAppNames {
			paths = append(paths, filepath.Join(appData, legacyAppName, "skills"))
		}
	}

	return paths
}

// ProjectSkillsDir returns the default project directories for which Codeplane
// will look for skills.
func ProjectSkillsDir(workingDir string) []string {
	return []string{
		filepath.Join(workingDir, ".agents/skills"),
		filepath.Join(workingDir, ".codeplane/skills"),
		filepath.Join(workingDir, ".smithers-tui/skills"),
		filepath.Join(workingDir, ".crush/skills"),
		filepath.Join(workingDir, ".claude/skills"),
		filepath.Join(workingDir, ".cursor/skills"),
	}
}

func isAppleTerminal() bool { return os.Getenv("TERM_PROGRAM") == "Apple_Terminal" }

// envWithFallback returns the value of the primary env var, falling back to
// legacy names if unset. A warning is logged when a legacy name is used so
// operators can migrate.
// TODO(codeplane): remove SMITHERS_TUI_* and CRUSH_* fallbacks after v1.0.
func envWithFallback(primary string, legacy ...string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	for _, name := range legacy {
		if v := os.Getenv(name); v != "" {
			slog.Warn("Using legacy environment variable; please migrate to the new name",
				"legacy", name, "replacement", primary)
			return v
		}
	}
	return ""
}

func preferExistingPath(kind string, paths ...string) string {
	if len(paths) == 0 {
		return ""
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			if path != paths[0] {
				slog.Warn(fmt.Sprintf("Using legacy %s path; please migrate to the Codeplane path", kind),
					"selected", path, "replacement", paths[0])
			}
			return path
		}
	}
	return paths[0]
}

func workspaceConfigPath(dataDir string) string {
	return preferExistingPath("workspace_config", workspaceConfigPathsFor(dataDir)...)
}

func isLegacyConfigPath(path string) bool {
	base := filepath.Base(path)
	for _, legacyAppName := range legacyAppNames {
		if base == legacyAppName+".json" || base == "."+legacyAppName+".json" {
			return true
		}
		if strings.Contains(path, string(filepath.Separator)+legacyAppName+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// IsLegacyConfigPath reports whether a selected config or data path still uses
// a Smithers/Crush-era name.
func IsLegacyConfigPath(path string) bool {
	return isLegacyConfigPath(path)
}

func configPathsFor(baseDir string) []string {
	paths := []string{
		filepath.Join(baseDir, appName, fmt.Sprintf("%s.json", appName)),
	}
	for _, legacyAppName := range legacyAppNames {
		paths = append(paths, filepath.Join(baseDir, legacyAppName, fmt.Sprintf("%s.json", legacyAppName)))
	}
	return paths
}

func dataPathsFor(baseDir string) []string {
	paths := []string{
		filepath.Join(baseDir, appName, fmt.Sprintf("%s.json", appName)),
	}
	for _, legacyAppName := range legacyAppNames {
		paths = append(paths, filepath.Join(baseDir, legacyAppName, fmt.Sprintf("%s.json", legacyAppName)))
	}
	return paths
}

func workspaceConfigPathsFor(dataDir string) []string {
	paths := []string{
		filepath.Join(dataDir, fmt.Sprintf("%s.json", appName)),
	}
	for _, legacyAppName := range legacyAppNames {
		paths = append(paths, filepath.Join(dataDir, fmt.Sprintf("%s.json", legacyAppName)))
	}
	return paths
}

func namedPathsInDir(dir string) []string {
	paths := []string{
		filepath.Join(dir, fmt.Sprintf("%s.json", appName)),
	}
	for _, legacyAppName := range legacyAppNames {
		paths = append(paths, filepath.Join(dir, fmt.Sprintf("%s.json", legacyAppName)))
	}
	return paths
}
