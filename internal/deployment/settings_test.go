package deployment

import "testing"

func TestSettingsValidate(t *testing.T) {
	valid := Settings{Channel: "stable", WebUIExposure: ExposureLocal, DNSExposure: ExposureLAN}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, settings := range []Settings{{Channel: "nightly", WebUIExposure: ExposureLocal, DNSExposure: ExposureLocal}, {Channel: "stable", WebUIExposure: "internet", DNSExposure: ExposureLocal}, {Channel: "stable", WebUIExposure: ExposureLocal, DNSExposure: "everywhere"}} {
		if err := settings.Validate(); err == nil {
			t.Fatalf("accepted %#v", settings)
		}
	}
}
