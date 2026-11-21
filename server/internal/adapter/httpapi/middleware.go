package httpapi

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"

	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// RequireAppHost は、Host を検証するミドルウェアを返す。appOrigin は、アプリのオリジン
// (例: http://localhost:5173)である。
//
// Host が、アプリのオリジンのホストでも、ループバックのアドレスの表記でもない要求を、
// forbidden_origin(403)で拒否する。Host がない要求も、拒否する。
//
// Origin の検証(RequireAppOrigin)は、Origin がない要求を通す。第三者のサイトのページが、
// 自分のホストの名前を、このサーバーのアドレスに向け直すと(DNS rebinding)、そのページからの GET は、
// ブラウザにとって同一オリジンになり、Origin が付かない。この要求は、Host に、そのサイトの名前を持つ。
// Host を検証して、拒否する(WS-29、ADR-002)。
//
//   - アプリのオリジンのホストは、クライアントを配信するサーバーが、Host を書き換えずに中継する要求のものである。
//   - ループバックのアドレスの表記(127.0.0.1:8080 など)は、このサーバーへ直接に送る要求のものである。
//     名前ではないため、他のアドレスに向け直せない。
func RequireAppHost(appOrigin string) (func(http.Handler) http.Handler, error) {
	u, err := url.Parse(appOrigin)
	if err != nil || u.Host == "" {
		return nil, errors.New("httpapi: invalid app origin")
	}
	appHost := u.Host

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.EqualFold(r.Host, appHost) && !isLoopbackHost(r.Host) {
				writeError(w, openapi.ForbiddenOrigin)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// isLoopbackHost は、Host(ホスト、または、ホスト:ポート)のホストが、
// ループバックのアドレス(127.0.0.0/8、::1)の表記であるかを返す。名前(localhost を含む)は、当たらない。
func isLoopbackHost(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		// ポートのない、IPv6 のアドレスの表記([::1])。
		name = host[1 : len(host)-1]
	}
	addr, err := netip.ParseAddr(name)
	return err == nil && addr.Zone() == "" && addr.IsLoopback()
}

// RequireAppOrigin は、Origin を検証するミドルウェアを返す。
//
// Origin ヘッダーがあり、アプリのオリジンと一致しない要求を、forbidden_origin(403)で拒否する。
// Origin がない要求は、通す。ブラウザは、同一オリジンの GET に、Origin を付けないためである(ADR-002)。
// Origin が付かない要求のうち、第三者のサイトの名前に向けたものは、RequireAppHost が拒否する。
// CORS のヘッダーは、付けない。
func RequireAppOrigin(appOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origins := r.Header.Values("Origin")
			if len(origins) > 1 || (len(origins) == 1 && origins[0] != appOrigin) {
				writeError(w, openapi.ForbiddenOrigin)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireCanonicalPath は、パスが正規の形でない要求(空の要素、.、.. を含む)に、
// not_found(404)を返すミドルウェアである。ServeMux より前に置く。
// ServeMux は、これらの要求に、転送の応答(307)と、HTML の本文を返すためである(WS-30)。
func RequireCanonicalPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !strings.HasPrefix(p, "/") || path.Clean(p) != p {
			writeError(w, openapi.NotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NotFoundHandler は、定義されていないパスと方法を受ける、受け皿を返す。
// ServeMux に、"/" として登録する。Go の既定の応答(文言)を、使わないためである。
// 受け皿があると、パスは定義されているが方法が違う要求(OPTIONS を含む)も、ここに届く。
func NotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, openapi.NotFound)
	})
}
