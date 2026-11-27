package main

import (
	"strings"
	"testing"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
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
		"SMTP_ADDR":  "127.0.0.1:1025",
		"MAIL_FROM":  "no-reply@chat.example",
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
	if cfg.SMTPAddr != "127.0.0.1:1025" || cfg.MailFrom != "no-reply@chat.example" {
		t.Fatalf("設定が違う: %+v", cfg)
	}
	// RL-28: 設定がない上限は、既定値で働く。
	if cfg.Limits != domain.DefaultLimitRules() {
		t.Fatalf("上限が、既定値でない: %+v", cfg.Limits)
	}
}

// 秘密と、アプリのオリジンが与えられていない場合、既定の値では動かない(ADR-007)。
func TestLoadConfig_RequiresVariables(t *testing.T) {
	for _, name := range []string{"PGUSER", "PGPASSWORD", "PGDATABASE", "APP_ORIGIN", "SMTP_ADDR", "MAIL_FROM"} {
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

// RL-38: 待ち受け先は、ループバックのアドレスに限る(SEC-2、RL-D1)。
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

// RL-38、ADR-018: SMTP のサーバーの接続先は、ループバックのアドレスに限る。
func TestLoadConfig_SMTPAddrMustBeLoopback(t *testing.T) {
	tests := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:1025", true},
		{"127.0.0.1:11025", true},
		{"[::1]:1025", true},
		{"localhost:1025", false},
		{"0.0.0.0:1025", false},
		{"192.168.1.10:25", false},
		{"smtp.example.com:587", false},
		{"127.0.0.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			env := validEnv()
			env["SMTP_ADDR"] = tt.addr

			cfg, err := loadConfig(envOf(env))

			if (err == nil) != tt.ok {
				t.Fatalf("ok = %v を期待した: %v", tt.ok, err)
			}
			if tt.ok && cfg.SMTPAddr != tt.addr {
				t.Fatalf("接続先が違う: %s", cfg.SMTPAddr)
			}
		})
	}
}

func TestLoadConfig_MailFrom(t *testing.T) {
	tests := []struct {
		value string
		ok    bool
	}{
		{"no-reply@chat.example", true},
		{"no-reply", false},
		{"Chat <no-reply@chat.example>", false},
		{"no-reply@chat.example\r\nBcc: x@example.com", false},
		{"a@b.example, c@d.example", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			env := validEnv()
			env["MAIL_FROM"] = tt.value

			_, err := loadConfig(envOf(env))

			if (err == nil) != tt.ok {
				t.Fatalf("ok = %v を期待した: %v", tt.ok, err)
			}
		})
	}
}

// RL-28: 設定で変えた上限は、その値で働く。設定がない値は、既定値で働く。
func TestLoadConfig_Limits(t *testing.T) {
	env := validEnv()
	env["LIMIT_SIGNUP_REQUEST_PER_SOURCE"] = "1000/1h"
	env["LIMIT_SIGNUP_REQUEST_PER_CONTACT"] = "5/30m"
	env["LIMIT_SIGNUP_COMPLETION_PER_SOURCE"] = "7/2h"
	env["LIMIT_LOGIN_FAILURE_PER_CONTACT"] = "3/90s"
	env["LIMIT_LOGIN_FAILURE_PER_SOURCE"] = "4/1h30m"

	cfg, err := loadConfig(envOf(env))
	if err != nil {
		t.Fatal(err)
	}

	want := domain.LimitRules{
		SignupRequestPerSource:    domain.LimitRule{Count: 1000, Window: time.Hour},
		SignupRequestPerContact:   domain.LimitRule{Count: 5, Window: 30 * time.Minute},
		SignupCompletionPerSource: domain.LimitRule{Count: 7, Window: 2 * time.Hour},
		LoginFailurePerContact:    domain.LimitRule{Count: 3, Window: 90 * time.Second},
		LoginFailurePerSource:     domain.LimitRule{Count: 4, Window: 90 * time.Minute},
		PeerLookupPerUser:         domain.DefaultLimitRules().PeerLookupPerUser,
	}
	if cfg.Limits != want {
		t.Fatalf("上限が違う: %+v", cfg.Limits)
	}
}

// RL-28: 読めない設定と、正の値でない設定では、サーバーは起動しない。
func TestLoadConfig_InvalidLimits(t *testing.T) {
	names := []string{
		"LIMIT_SIGNUP_REQUEST_PER_SOURCE",
		"LIMIT_SIGNUP_REQUEST_PER_CONTACT",
		"LIMIT_SIGNUP_COMPLETION_PER_SOURCE",
		"LIMIT_LOGIN_FAILURE_PER_CONTACT",
		"LIMIT_LOGIN_FAILURE_PER_SOURCE",
		"LIMIT_PEER_LOOKUP_PER_USER",
	}
	values := []string{"0/1h", "-1/1h", "10/0s", "10/-1h", "10", "10/", "/1h", "ten/1h", "10/1", "10/hour", "1.5/1h", "10/1h/2"}

	for _, name := range names {
		for _, value := range values {
			t.Run(name+"="+value, func(t *testing.T) {
				env := validEnv()
				env[name] = value

				_, err := loadConfig(envOf(env))

				if err == nil || !strings.Contains(err.Error(), name) {
					t.Fatalf("%s が正しくないことを示すエラーを、返さなかった: %v", name, err)
				}
			})
		}
	}
}
