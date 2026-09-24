package envconfig

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_PORT", "")
	t.Setenv("APP_HOST", "")
	t.Setenv("APP_DEBUG", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg != (Config{Port: 8080, Host: "127.0.0.1", Debug: false}) {
		t.Errorf("defaults = %+v", cfg)
	}
}

func TestLoadValues(t *testing.T) {
	t.Setenv("APP_PORT", "9000")
	t.Setenv("APP_HOST", "0.0.0.0")
	for _, v := range []string{"true", "TRUE", "1"} {
		t.Setenv("APP_DEBUG", v)
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg != (Config{Port: 9000, Host: "0.0.0.0", Debug: true}) {
			t.Errorf("APP_DEBUG=%q: %+v", v, cfg)
		}
	}
	for _, v := range []string{"yes", "0", "false", "t"} {
		t.Setenv("APP_DEBUG", v)
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Debug {
			t.Errorf("APP_DEBUG=%q should be false", v)
		}
	}
}

func TestLoadRejectsBadPort(t *testing.T) {
	t.Setenv("APP_HOST", "")
	t.Setenv("APP_DEBUG", "")
	for _, v := range []string{"abc", "0", "-1", "65536", "80.5"} {
		t.Setenv("APP_PORT", v)
		if _, err := Load(); err == nil {
			t.Errorf("APP_PORT=%q should be rejected", v)
		}
	}
	t.Setenv("APP_PORT", "65535")
	if cfg, err := Load(); err != nil || cfg.Port != 65535 {
		t.Errorf("APP_PORT=65535: %+v %v", cfg, err)
	}
}
