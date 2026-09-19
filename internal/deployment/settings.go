// Package deployment defines the small, fixed protocol used by the unprivileged
// management API and the root-owned local deployment agent.
package deployment

import "fmt"

const (
	ExposureLocal = "local"
	ExposureLAN   = "lan"
)

type Settings struct {
	Channel       string `json:"channel"`
	WebUIExposure string `json:"web_ui_exposure"`
	DNSExposure   string `json:"dns_exposure"`
}

func (s Settings) Validate() error {
	if s.Channel != "stable" && s.Channel != "beta" && s.Channel != "alpha" {
		return fmt.Errorf("channel must be stable, beta, or alpha")
	}
	if s.WebUIExposure != ExposureLocal && s.WebUIExposure != ExposureLAN {
		return fmt.Errorf("web_ui_exposure must be local or lan")
	}
	if s.DNSExposure != ExposureLocal && s.DNSExposure != ExposureLAN {
		return fmt.Errorf("dns_exposure must be local or lan")
	}
	return nil
}

type Response struct {
	Settings Settings `json:"settings"`
	Applied  bool     `json:"applied"`
}
