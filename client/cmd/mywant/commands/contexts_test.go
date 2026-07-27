package commands

import "testing"

func baseConfig() *MyWantConfig {
	return &MyWantConfig{
		ServerHost: "localhost",
		ServerPort: 8080,
		Contexts: map[string]ServerContext{
			"local": {Server: "http://localhost:8080"},
			"fly": {
				Server:      "https://example.fly.dev",
				Username:    "admin",
				PasswordEnv: "TEST_MYWANT_PW",
			},
		},
	}
}

func TestResolveServerUsesCurrentContext(t *testing.T) {
	SetContextOverride("")
	t.Setenv("TEST_MYWANT_PW", "s3cret")

	config := baseConfig()
	config.CurrentContext = "fly"

	url, auth, err := ResolveServer(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://example.fly.dev" {
		t.Errorf("url = %q, want https://example.fly.dev", url)
	}
	if auth.Username != "admin" || auth.Password != "s3cret" {
		t.Errorf("auth = %+v, want admin/s3cret from password_env", auth)
	}
}

func TestResolveServerContextOverrideWins(t *testing.T) {
	SetContextOverride("local")
	defer SetContextOverride("")

	config := baseConfig()
	config.CurrentContext = "fly"

	url, auth, err := ResolveServer(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://localhost:8080" {
		t.Errorf("url = %q, want http://localhost:8080", url)
	}
	if !auth.IsZero() {
		t.Errorf("auth = %+v, want empty for the local context", auth)
	}
}

func TestResolveServerEnvOverridesContext(t *testing.T) {
	SetContextOverride("")
	t.Setenv("TEST_MYWANT_PW", "s3cret")
	t.Setenv("MYWANT_SERVER", "https://override.example")
	t.Setenv("MYWANT_TOKEN", "tok")

	config := baseConfig()
	config.CurrentContext = "fly"

	url, auth, err := ResolveServer(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://override.example" {
		t.Errorf("url = %q, want the MYWANT_SERVER value", url)
	}
	if auth.Token != "tok" {
		t.Errorf("token = %q, want tok", auth.Token)
	}
}

func TestResolveServerFallsBackToHostPort(t *testing.T) {
	SetContextOverride("")

	config := baseConfig()
	config.CurrentContext = ""
	config.ServerPort = 9090

	url, auth, err := ResolveServer(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://localhost:9090" {
		t.Errorf("url = %q, want http://localhost:9090", url)
	}
	if !auth.IsZero() {
		t.Errorf("auth = %+v, want empty", auth)
	}
}

func TestResolveServerUnknownContextErrors(t *testing.T) {
	SetContextOverride("nope")
	defer SetContextOverride("")

	if _, _, err := ResolveServer(baseConfig()); err == nil {
		t.Fatal("expected an error for an unknown context")
	}
}

func TestResolveServerTrimsTrailingSlash(t *testing.T) {
	SetContextOverride("")

	config := baseConfig()
	config.Contexts["slash"] = ServerContext{Server: "https://example.fly.dev/"}
	config.CurrentContext = "slash"

	url, _, err := ResolveServer(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://example.fly.dev" {
		t.Errorf("url = %q, want the trailing slash trimmed", url)
	}
}
