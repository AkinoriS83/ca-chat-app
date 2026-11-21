package httpapi

import (
	"net/http"
	"path"
	"strings"

	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// RequireAppOrigin は、Origin を検証するミドルウェアを返す。
//
// Origin ヘッダーがあり、アプリのオリジンと一致しない要求を、forbidden_origin(403)で拒否する。
// Origin がない要求は、通す。ブラウザは、同一オリジンの GET に、Origin を付けないためである(ADR-002)。
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
