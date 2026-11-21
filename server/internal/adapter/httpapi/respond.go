package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/AkinoriS83/ca-chat-app/server/internal/openapi"
)

// writeJSON は、応答を JSON で書く。
func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		// 生成した型は、常に JSON にできる。ここには、来ない。
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// 応答は、利用者ごとの内容である。ブラウザと中継に、保存させない。
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// writeError は、拒否とエラーの応答を書く。
// 応答は、code だけを持つ。文言、詳細、内部の情報は、含めない(WS-30)。
func writeError(w http.ResponseWriter, code openapi.ErrorCode) {
	writeJSON(w, statusOf(code), openapi.ErrorResponse{Code: code})
}

// statusOf は、code に対応する、HTTP の状態を返す。
func statusOf(code openapi.ErrorCode) int {
	switch code {
	case openapi.ForbiddenOrigin:
		return http.StatusForbidden
	case openapi.InvalidInput, openapi.InvalidBody, openapi.InvalidPeer:
		return http.StatusBadRequest
	case openapi.Unauthenticated:
		return http.StatusUnauthorized
	case openapi.NotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
