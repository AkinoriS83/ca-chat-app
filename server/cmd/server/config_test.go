package main

import (
	"strings"
	"testing"
)

func envOf(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func validEnv() map[string]string {
	return map[string]string{
		"PGUSER":     "chat",
		"PGPASSWORD": "secret",
		"PGDATABASE": "chat",
		"APP_ORIGIN": "http://localhost:5173",
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := loadConfig(envOf(validEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:8080" || cfg.AppOrigin != "http://localhost:5173" {
		t.Fatalf("設定が違う: %+v", cfg)
	}
}

// 秘密と、アプリのオリジンが与えられていない場合、既定の値では動かない(ADR-007)。
func TestLoadConfig_RequiresVariables(t *testing.T) {
	for _, name := range []string{"PGUSER", "PGPASSWORD", "PGDATABASE", "APP_ORIGIN"} {
		t.Run(name, func(t *testing.T) {
			env := validEnv()
			delete(env, name)

			_, err := loadConfig(envOf(env))

			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("%s がないことを示すエラーを、返さなかった: %v", name, err)
			}
		})
	}
}

func TestLoadConfig_ErrorDoesNotContainSecret(t *testing.T) {
	env := validEnv()
	env["PGPASSWORD"] = "very-secret-value"
	env["LISTEN_ADDR"] = "0.0.0.0:8080"

	_, err := loadConfig(envOf(env))

	if err == nil || strings.Contains(err.Error(), "very-secret-value") {
		t.Fatalf("エラーが、秘密を含む: %v", err)
	}
}

func TestLoadConfig_AppOrigin(t *testing.T) {
	tests := []struct {
		origin string
		ok     bool
	}{
		{"http://localhost:5173", true},
		{"http://localhost:15173", true},
		{"https://chat.example", true},
		{"http://localhost:5173/", false},
		{"http://localhost:5173/app", false},
		{"localhost:5173", false},
		{"ws://localhost:5173", false},
		{"http://", false},
		{"http://localhost:5173?x=1", false},
	}
	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			env := validEnv()
			env["APP_ORIGIN"] = tt.origin

			_, err := loadConfig(envOf(env))

			if (err == nil) != tt.ok {
				t.Fatalf("ok = %v を期待した: %v", tt.ok, err)
			}
		})
	}
}

// 仮実装(WS-P2): 待ち受け先は、ループバックのアドレスに限る(DD-27)。
func TestLoadConfig_ListenAddrMustBeLoopback(t *testing.T) {
	tests := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:8080", true},
		{"127.0.0.1:18080", true},
		{"127.1.2.3:8080", true},
		{"[::1]:8080", true},
		{"0.0.0.0:8080", false},
		{"[::]:8080", false},
		{":8080", false},
		{"localhost:8080", false},
		{"192.168.1.10:8080", false},
		{"10.0.0.1:8080", false},
		{"example.com:8080", false},
		{"127.0.0.1", false},
		{"8080", false},
		{"[fe80::1%eth0]:8080", false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			env := validEnv()
			env["LISTEN_ADDR"] = tt.addr

			cfg, err := loadConfig(envOf(env))

			if (err == nil) != tt.ok {
				t.Fatalf("ok = %v を期待した: %v", tt.ok, err)
			}
			if tt.ok && cfg.ListenAddr != tt.addr {
				t.Fatalf("待ち受け先が違う: %s", cfg.ListenAddr)
			}
		})
	}
}
