package config

import (
	"fmt"
)

func ValidateConfig(cfg *Config) error {
	seenIngests := map[string]bool{}
	for _, ingest := range cfg.Ingests {
		if ingest.ID == "" {
			return fmt.Errorf("ingest missing id")
		}
		if seenIngests[ingest.ID] {
			return fmt.Errorf("duplicate ingest id %q", ingest.ID)
		}
		seenIngests[ingest.ID] = true
	}

	seenProfiles := map[string]bool{}
	for _, profile := range cfg.Profiles {
		if profile.ID == "" {
			return fmt.Errorf("profile missing id")
		}
		if seenProfiles[profile.ID] {
			return fmt.Errorf("duplicate profile id %q", profile.ID)
		}
		seenProfiles[profile.ID] = true
	}

	seenTargets := map[string]bool{}
	for _, target := range cfg.Targets {
		if target.ID == "" {
			return fmt.Errorf("target missing id")
		}
		if seenTargets[target.ID] {
			return fmt.Errorf("duplicate target id %q", target.ID)
		}
		seenTargets[target.ID] = true

		if !target.Enabled {
			continue
		}

		if target.IngestID != "" && !seenIngests[target.IngestID] {
			return fmt.Errorf("target %q references missing ingest %q", target.ID, target.IngestID)
		}

		if target.ProfileID != "" && !seenProfiles[target.ProfileID] {
			return fmt.Errorf("target %q references missing profile %q", target.ID, target.ProfileID)
		}

		if target.RTMPSURLEnv == "" {
			return fmt.Errorf("enabled target %q missing rtmps_url_env", target.ID)
		}
	}

	if cfg.TLS.Enabled {
		if cfg.TLS.CertFile == "" {
			return fmt.Errorf("tls.enabled=true but tls.cert_file is empty")
		}
		if cfg.TLS.KeyFile == "" {
			return fmt.Errorf("tls.enabled=true but tls.key_file is empty")
		}
	}

	return nil
}
