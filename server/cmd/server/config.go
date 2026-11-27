package main

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// defaultListenAddr は、待ち受け先の既定の値である。
const defaultListenAddr = "127.0.0.1:8080"

// config は、サーバーの設定である。
type config struct {
	ListenAddr string // 待ち受け先
	AppOrigin  string // アプリのオリジン(例: http://localhost:5173)
	SMTPAddr   string // 案内を送る、SMTP のサーバーの接続先(ホスト:ポート)
	MailFrom   string // 案内の送信元のアドレス
	Limits     domain.LimitRules
}

// 保管先への接続情報のうち、与えられていなければ、起動に失敗するもの。
// 値は、pgx が、環境変数からそのまま読む。接続文字列には、組み立てない。
// PGHOST と PGPORT は、pgx の既定(localhost、5432)を許す。
var requiredStoreVariables = []string{"PGUSER", "PGPASSWORD", "PGDATABASE"}

// loadConfig は、環境変数の値から、設定を読む。
// 秘密と、アプリのオリジンが与えられていない場合、既定の値では動かず、エラーを返す(ADR-007)。
func loadConfig(getenv func(string) string) (config, error) {
	for _, name := range requiredStoreVariables {
		if getenv(name) == "" {
			return config{}, fmt.Errorf("環境変数 %s が、与えられていません", name)
		}
	}

	appOrigin := getenv("APP_ORIGIN")
	if appOrigin == "" {
		return config{}, errors.New("環境変数 APP_ORIGIN が、与えられていません")
	}
	if err := validateOrigin(appOrigin); err != nil {
		return config{}, fmt.Errorf("環境変数 APP_ORIGIN が、正しくありません: %w", err)
	}

	listenAddr := getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}
	if err := validateLoopback(listenAddr); err != nil {
		return config{}, fmt.Errorf("環境変数 LISTEN_ADDR が、正しくありません: %w", err)
	}

	// SMTP の経路は、暗号化も、認証もしない。接続先を、ループバックのアドレスに限る(ADR-018、RL-37)。
	smtpAddr := getenv("SMTP_ADDR")
	if smtpAddr == "" {
		return config{}, errors.New("環境変数 SMTP_ADDR が、与えられていません")
	}
	if err := validateLoopback(smtpAddr); err != nil {
		return config{}, fmt.Errorf("環境変数 SMTP_ADDR が、正しくありません: %w", err)
	}

	mailFrom := getenv("MAIL_FROM")
	if mailFrom == "" {
		return config{}, errors.New("環境変数 MAIL_FROM が、与えられていません")
	}
	if err := validateMailAddress(mailFrom); err != nil {
		return config{}, fmt.Errorf("環境変数 MAIL_FROM が、正しくありません: %w", err)
	}

	limits, err := loadLimitRules(getenv)
	if err != nil {
		return config{}, err
	}

	return config{
		ListenAddr: listenAddr,
		AppOrigin:  appOrigin,
		SMTPAddr:   smtpAddr,
		MailFrom:   mailFrom,
		Limits:     limits,
	}, nil
}

// loadLimitRules は、上限の値を読む(仕様「上限」、RL-28)。
// 与えられていない値は、既定値で働く。読めない値と、回数か時間が、正の値でない値は、エラーとする。
func loadLimitRules(getenv func(string) string) (domain.LimitRules, error) {
	rules := domain.DefaultLimitRules()
	variables := []struct {
		name string
		rule *domain.LimitRule
	}{
		{"LIMIT_SIGNUP_REQUEST_PER_SOURCE", &rules.SignupRequestPerSource},
		{"LIMIT_SIGNUP_REQUEST_PER_CONTACT", &rules.SignupRequestPerContact},
		{"LIMIT_SIGNUP_COMPLETION_PER_SOURCE", &rules.SignupCompletionPerSource},
		{"LIMIT_LOGIN_FAILURE_PER_CONTACT", &rules.LoginFailurePerContact},
		{"LIMIT_LOGIN_FAILURE_PER_SOURCE", &rules.LoginFailurePerSource},
		{"LIMIT_PEER_LOOKUP_PER_USER", &rules.PeerLookupPerUser},
	}
	for _, v := range variables {
		value := getenv(v.name)
		if value == "" {
			continue
		}
		rule, err := parseLimitRule(value)
		if err != nil {
			return domain.LimitRules{}, fmt.Errorf("環境変数 %s が、正しくありません: %w", v.name, err)
		}
		*v.rule = rule
	}
	return rules, nil
}

// parseLimitRule は、「回数/時間」(例: 10/1h)の形の値を読む。時間は、15m、1h などの形である。
func parseLimitRule(value string) (domain.LimitRule, error) {
	countText, windowText, found := strings.Cut(value, "/")
	if !found {
		return domain.LimitRule{}, errors.New("回数/時間 の形(例: 10/1h)で指定してください")
	}
	count, err := strconv.Atoi(countText)
	if err != nil {
		return domain.LimitRule{}, errors.New("回数を、整数で指定してください")
	}
	window, err := time.ParseDuration(windowText)
	if err != nil {
		return domain.LimitRule{}, errors.New("時間を、15m、1h などの形で指定してください")
	}
	rule := domain.LimitRule{Count: count, Window: window}
	if !rule.Valid() {
		return domain.LimitRule{}, errors.New("回数と時間には、正の値を指定してください")
	}
	return rule, nil
}

// validateMailAddress は、値が、メールアドレス(名前を伴わない形)として読めることを確かめる。
func validateMailAddress(value string) error {
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value {
		return errors.New("メールアドレスだけを指定してください(例: no-reply@chat.example)")
	}
	return nil
}

// validateOrigin は、値が、オリジン(スキーム://ホスト[:ポート])の形であることを確かめる。
// ブラウザが送る Origin ヘッダーと、文字列として比べるためである。
func validateOrigin(origin string) error {
	u, err := url.Parse(origin)
	if err != nil {
		return errors.New("URL として読めません")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("http:// または https:// で始まる、オリジンを指定してください")
	}
	if u.Scheme+"://"+u.Host != origin {
		return errors.New("パスや末尾のスラッシュを含めず、オリジンだけを指定してください")
	}
	return nil
}

// validateLoopback は、ホスト:ポート のホストが、ループバックのアドレス(127.0.0.0/8、::1)の
// 表記であることを確かめる。ホストの名前(localhost を含む)と、ホストのない形(:8080)は、受け付けない。
// 待ち受け先と、SMTP のサーバーの接続先に使う。
//
// 通信経路と、SMTP の経路を、暗号化しないためである(SEC-2、RL-D1、ADR-018)。ローカルの開発環境の外から、
// 到達できてはならず、案内を、開発者の端末の外へ送ってはならない(RL-37、RL-38)。
// この制限は、設置環境を扱うスライスで、暗号化の構成を定めるときに解く。
func validateLoopback(hostPort string) error {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return errors.New("ホスト:ポート の形で指定してください")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || addr.Zone() != "" || !addr.IsLoopback() {
		return errors.New("ホストには、ループバックのアドレス(127.0.0.0/8、::1)を指定してください")
	}
	return nil
}
