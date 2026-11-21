package main

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
)

// defaultListenAddr は、待ち受け先の既定の値である。
const defaultListenAddr = "127.0.0.1:8080"

// config は、サーバーの設定である。
type config struct {
	ListenAddr string // 待ち受け先
	AppOrigin  string // アプリのオリジン(例: http://localhost:5173)
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

	return config{ListenAddr: listenAddr, AppOrigin: appOrigin}, nil
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

// validateLoopback は、待ち受け先のホストが、ループバックのアドレス(127.0.0.0/8、::1)の
// 表記であることを確かめる。ホストの名前(localhost を含む)と、ホストのない形(:8080)は、受け付けない。
//
// 仮実装(WS-P2): 仮のログインは、なりすましを防がない。ローカルの開発環境の外から、
// 到達できてはならない(WS-D6、ADR-007)。フェーズ1の認証で、仮のログインとともに取り除く。
func validateLoopback(listenAddr string) error {
	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return errors.New("ホスト:ポート の形で指定してください")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || addr.Zone() != "" || !addr.IsLoopback() {
		return errors.New("ホストには、ループバックのアドレス(127.0.0.0/8、::1)を指定してください")
	}
	return nil
}
