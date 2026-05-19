package config

type EnvVarHelp struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Example     string `json:"example" yaml:"example"`
}

type Config struct {
	App         AppConfig         `json:"app" yaml:"app"`
	Logging     LoggingConfig     `json:"logging" yaml:"logging"`
	Auth        AuthConfig        `json:"auth" yaml:"auth"`
	TLS         TLSConfig         `json:"tls" yaml:"tls"`
	MediaEngine MediaEngineConfig `json:"media_engine" yaml:"media_engine"`
	UI          UIConfig          `json:"ui" yaml:"ui"`
	Ingests     []IngestConfig    `json:"ingests" yaml:"ingests"`
	Slate       SlateConfig       `json:"slate" yaml:"slate"`
	FFmpeg      FFmpegConfig      `json:"ffmpeg" yaml:"ffmpeg"`
	Profiles    []ProfileConfig   `json:"profiles" yaml:"profiles"`
	Targets     []TargetConfig    `json:"targets" yaml:"targets"`
	Runtime     RuntimeConfig     `json:"runtime" yaml:"runtime"`
	Behavior    BehaviorConfig    `json:"behavior" yaml:"behavior"`
}

type AppConfig struct {
	Name          string `json:"name" yaml:"name"`
	Listen        string `json:"listen" yaml:"listen"`
	PublicBaseURL string `json:"public_base_url" yaml:"public_base_url"`
	DataDir       string `json:"data_dir" yaml:"data_dir"`
	UIMode        string `json:"ui_mode" yaml:"ui_mode"`
}

type LoggingConfig struct {
	Level  string `json:"level" yaml:"level"`
	Format string `json:"format" yaml:"format"` // "text" or "json"
}

type AuthConfig struct {
	Mode               string       `json:"mode" yaml:"mode"`
	PSKEnv             string       `json:"psk_env" yaml:"psk_env"`
	AllowTokenLoginURL bool         `json:"allow_token_login_url" yaml:"allow_token_login_url"`
	Cookie             CookieConfig `json:"cookie" yaml:"cookie"`
}

type CookieConfig struct {
	Name     string `json:"name" yaml:"name"`
	Secure   bool   `json:"secure" yaml:"secure"`
	SameSite string `json:"same_site" yaml:"same_site"`
	TTL      string `json:"ttl" yaml:"ttl"`
}

type TLSConfig struct {
	Enabled  bool   `json:"enabled" yaml:"enabled"`
	CertFile string `json:"cert_file" yaml:"cert_file"`
	KeyFile  string `json:"key_file" yaml:"key_file"`
}

type MediaEngineConfig struct {
	Type     string         `json:"type" yaml:"type"`
	MediaMTX MediaMTXConfig `json:"mediamtx" yaml:"mediamtx"`
}

type MediaMTXConfig struct {
	APIURL           string `json:"api_url" yaml:"api_url"`
	InternalRTMPBase string `json:"internal_rtmp_base" yaml:"internal_rtmp_base"`
	HLSBaseURL       string `json:"hls_base_url" yaml:"hls_base_url"`
}

type UIConfig struct {
	Title                string `json:"title" yaml:"title"`
	Subtitle             string `json:"subtitle" yaml:"subtitle"`
	PreviewHLSURL        string `json:"preview_hls_url" yaml:"preview_hls_url"`
	CameraSourceURL      string `json:"camera_source_url" yaml:"camera_source_url"`
	ShowLogs             bool   `json:"show_logs" yaml:"show_logs"`
	ShowConfigValidation bool   `json:"show_config_validation" yaml:"show_config_validation"`
}

type IngestConfig struct {
	ID                string        `json:"id" yaml:"id"`
	Label             string        `json:"label" yaml:"label"`
	Type              string        `json:"type" yaml:"type"`
	Path              string        `json:"path" yaml:"path"`
	Enabled           bool          `json:"enabled" yaml:"enabled"`
	PublicPublishURL  string        `json:"public_publish_url" yaml:"public_publish_url"`
	PublishUser       string        `json:"publish_user" yaml:"publish_user"`
	PublishPassEnv    string        `json:"publish_pass_env" yaml:"publish_pass_env"`
	InternalSourceURL string        `json:"internal_source_url" yaml:"internal_source_url"`
	Preview           PreviewConfig `json:"preview" yaml:"preview"`
}

type PreviewConfig struct {
	Type string `json:"type" yaml:"type"`
	URL  string `json:"url" yaml:"url"`
}

type SlateConfig struct {
	Enabled bool             `json:"enabled" yaml:"enabled"`
	Type    string           `json:"type" yaml:"type"`
	Path    string           `json:"path" yaml:"path"`
	Text    string           `json:"text" yaml:"text"`
	Audio   SlateAudioConfig `json:"audio" yaml:"audio"`
	Video   SlateVideoConfig `json:"video" yaml:"video"`
}

type SlateAudioConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	Type       string `json:"type" yaml:"type"`
	SampleRate int    `json:"sample_rate" yaml:"sample_rate"`
	Channels   int    `json:"channels" yaml:"channels"`
	Bitrate    string `json:"bitrate" yaml:"bitrate"`
}

type SlateVideoConfig struct {
	Width            int    `json:"width" yaml:"width"`
	Height           int    `json:"height" yaml:"height"`
	FrameRate        int    `json:"frame_rate" yaml:"frame_rate"`
	Bitrate          string `json:"bitrate" yaml:"bitrate"`
	KeyframeInterval int    `json:"keyframe_interval" yaml:"keyframe_interval"`
}

type FFmpegConfig struct {
	Binary   string `json:"binary" yaml:"binary"`
	LogLevel string `json:"log_level" yaml:"log_level"`
}

type ProfileConfig struct {
	ID          string   `json:"id" yaml:"id"`
	Label       string   `json:"label" yaml:"label"`
	Description string   `json:"description" yaml:"description"`
	Args        []string `json:"args" yaml:"args"`
}

type TargetConfig struct {
	ID            string                 `json:"id" yaml:"id"`
	Label         string                 `json:"label" yaml:"label"`
	Platform      string                 `json:"platform" yaml:"platform"`
	Enabled       bool                   `json:"enabled" yaml:"enabled"`
	IngestID      string                 `json:"ingest_id" yaml:"ingest_id"`
	PreviewSource string                 `json:"preview_source" yaml:"preview_source"`
	LiveSource    string                 `json:"live_source" yaml:"live_source"`
	ProfileID     string                 `json:"profile_id" yaml:"profile_id"`
	RTMPSURLEnv   string                 `json:"rtmps_url_env" yaml:"rtmps_url_env"`
	RTMPSKeyEnv   string                 `json:"rtmps_key_env" yaml:"rtmps_key_env"`
	Lifecycle     TargetLifecycleConfig  `json:"lifecycle" yaml:"lifecycle"`
	Control       map[string]interface{} `json:"control" yaml:"control"`
	Reconnect     ReconnectConfig        `json:"reconnect" yaml:"reconnect"`
}

type TargetLifecycleConfig struct {
	SupportsPreview            bool `json:"supports_preview" yaml:"supports_preview"`
	SupportsGoLive             bool `json:"supports_go_live" yaml:"supports_go_live"`
	SupportsEnd                bool `json:"supports_end" yaml:"supports_end"`
	SwitchToCameraBeforeGoLive bool `json:"switch_to_camera_before_go_live" yaml:"switch_to_camera_before_go_live"`
	SingleWriter               bool `json:"single_writer" yaml:"single_writer"`
}

type ReconnectConfig struct {
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	InitialDelay string `json:"initial_delay" yaml:"initial_delay"`
	MaxDelay     string `json:"max_delay" yaml:"max_delay"`
	MaxAttempts  int    `json:"max_attempts" yaml:"max_attempts"`
}

type RuntimeConfig struct {
	Store RuntimeStoreConfig `json:"store" yaml:"store"`
	Logs  RuntimeLogsConfig  `json:"logs" yaml:"logs"`
}

type RuntimeStoreConfig struct {
	Type string `json:"type" yaml:"type"`
	Path string `json:"path" yaml:"path"`
}

type RuntimeLogsConfig struct {
	RetainLinesPerRelay int `json:"retain_lines_per_relay" yaml:"retain_lines_per_relay"`
	RetainEvents        int `json:"retain_events" yaml:"retain_events"`
}

type BehaviorConfig struct {
	AutoGoLive                bool   `json:"auto_go_live" yaml:"auto_go_live"`
	PreviewStartsSlate        bool   `json:"preview_starts_slate" yaml:"preview_starts_slate"`
	GoLiveRequiresCameraReady bool   `json:"go_live_requires_camera_ready" yaml:"go_live_requires_camera_ready"`
	GoLiveStabilityDelay      string `json:"go_live_stability_delay" yaml:"go_live_stability_delay"`
	StopOrder                 string `json:"stop_order" yaml:"stop_order"`
	ReconnectTargetsWhileLive bool   `json:"reconnect_targets_while_live" yaml:"reconnect_targets_while_live"`
	SourceLossPolicy          string `json:"source_loss_policy" yaml:"source_loss_policy"`
}

var RelevantEnvVars = []EnvVarHelp{
	{
		Name:        "GOLIVEPILOT_CONFIG",
		Description: "Path to the YAML configuration file.",
		Example:     "/etc/golivepilot/config.yml",
	},
	{
		Name:        "GOLIVEPILOT_OPERATOR_PSK",
		Description: "Pre-Shared Key for operator authentication (if enabled).",
		Example:     "a-very-secure-random-string",
	},
	{
		Name:        "FB_RTMPS_URL",
		Description: "Facebook RTMPS base ingest URL.",
		Example:     "rtmps://live-api-s.facebook.com:443/rtmp/",
	},
	{
		Name:        "FB_RTMPS_KEY",
		Description: "Facebook Stream Key.",
		Example:     "FB-1234567890-0-AbCdEfGhIjKlMnOp",
	},
	{
		Name:        "YT_RTMPS_URL",
		Description: "YouTube RTMPS ingest URL.",
		Example:     "rtmps://a.rtmps.youtube.com:443/live2/",
	},
	{
		Name:        "FB_PAGE_ACCESS_TOKEN",
		Description: "Facebook Page Access Token for API control.",
		Example:     "EAAG...",
	},
	{
		Name:        "YT_ACCESS_TOKEN",
		Description: "YouTube OAuth2 Access Token for API control.",
		Example:     "ya29...",
	},
}
