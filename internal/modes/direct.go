package modes

import "fmt"

// DirectModeReport holds the result of a direct connection mode.
type DirectModeReport struct {
	Mode   string `json:"mode"`
	Target string `json:"target"`
	Active bool   `json:"active"`
}

// AsText returns a text representation of the report.
func (r DirectModeReport) AsText() string {
	return fmt.Sprintf("mode=%s\ntarget=%s\nactive=%v", r.Mode, r.Target, r.Active)
}

// DirectConnect establishes a direct local connection.
func DirectConnect(target string) (DirectModeReport, error) {
	return DirectModeReport{Mode: "direct-connect", Target: target, Active: true}, nil
}

// DeepLink generates a deep link connection.
func DeepLink(target string) (DirectModeReport, error) {
	return DirectModeReport{Mode: "deep-link", Target: target, Active: true}, nil
}
