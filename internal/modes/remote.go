package modes

import "fmt"

// RuntimeModeReport holds the result of a remote runtime mode.
type RuntimeModeReport struct {
	Mode      string `json:"mode"`
	Connected bool   `json:"connected"`
	Detail    string `json:"detail"`
}

// AsText returns a text representation of the report.
func (r RuntimeModeReport) AsText() string {
	return fmt.Sprintf("mode=%s\nconnected=%v\ndetail=%s", r.Mode, r.Connected, r.Detail)
}

// RemoteMode prepares a remote control runtime connection.
func RemoteMode(target string) (RuntimeModeReport, error) {
	return RuntimeModeReport{
		Mode: "remote", Connected: true,
		Detail: fmt.Sprintf("Remote control placeholder prepared for %s", target),
	}, nil
}

// SSHMode prepares an SSH proxy connection.
func SSHMode(target string) (RuntimeModeReport, error) {
	return RuntimeModeReport{
		Mode: "ssh", Connected: true,
		Detail: fmt.Sprintf("SSH proxy placeholder prepared for %s", target),
	}, nil
}

// TeleportMode prepares a teleport resume/create connection.
func TeleportMode(target string) (RuntimeModeReport, error) {
	return RuntimeModeReport{
		Mode: "teleport", Connected: true,
		Detail: fmt.Sprintf("Teleport resume/create placeholder prepared for %s", target),
	}, nil
}
