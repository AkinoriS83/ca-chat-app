package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// このパッケージのテストは、ユースケースを組み立てずに確かめられるものに限る。
// ユースケースを通る振る舞いは、cmd/server のテストで確かめる。

func decodeRequest(body string) (sendID, decoded string, ok bool) {
	r := httptest.NewRequest(http.MethodPost, "/api/direct-conversations/bob/messages", strings.NewReader(body))
	return decodeSendMessageRequest(httptest.NewRecorder(), r)
}

func TestDecodeSendMessageRequest_Valid(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantSendID string
		wantBody   string
	}{
		{"通常の要求", `{"sendId":"abc","body":"hello"}`, "abc", "hello"},
		{"項目の順が逆で、空白がある", " {\n \"body\" : \"hello\" ,\n \"sendId\" : \"abc\" }\n", "abc", "hello"},
		{"未知の項目は、無視する", `{"sendId":"abc","body":"hello","extra":{"a":[1,2]}}`, "abc", "hello"},
		{"空の文字列は、形式には合う(値は、内側の層が判定する)", `{"sendId":"","body":""}`, "", ""},
		{"本文のエスケープを解く", `{"sendId":"abc","body":"a\nb\t\"c\""}`, "abc", "a\nb\t\"c\""},
		{"本文の改行の表現を、変えない", `{"sendId":"abc","body":"a\r\nb\rc"}`, "abc", "a\r\nb\rc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sendID, body, ok := decodeRequest(tt.body)
			if !ok {
				t.Fatal("形式に合わないと判定した")
			}
			if sendID != tt.wantSendID || body != tt.wantBody {
				t.Fatalf("got (%q, %q), want (%q, %q)", sendID, body, tt.wantSendID, tt.wantBody)
			}
		})
	}
}

// 本文の中の、文字列として正しくない並びは、形式の誤りとして拒否せず、そのまま内側へ渡す。
func TestDecodeSendMessageRequest_KeepsInvalidBodySequences(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"対にならないサロゲートのエスケープ", `{"sendId":"abc","body":"a` + esc("d83d") + `b"}`},
		{"正しくない UTF-8 のバイト", "{\"sendId\":\"abc\",\"body\":\"a\xFFb\"}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, body, ok := decodeRequest(tt.body)
			if !ok {
				t.Fatal("形式に合わないと判定した")
			}
			if utf8.ValidString(body) {
				t.Fatalf("正しい UTF-8 の並びに置き換わった: %q", body)
			}
			if strings.Contains(body, "\xEF\xBF\xBD") {
				t.Fatalf("U+FFFD に置き換わった: %q", body)
			}
		})
	}
}

func TestDecodeSendMessageRequest_Malformed(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"空", ""},
		{"JSON として壊れている", `{"sendId":"abc","body":"hello"`},
		{"JSON ではない", `hello`},
		{"オブジェクトではない(配列)", `["abc","hello"]`},
		{"オブジェクトではない(文字列)", `"hello"`},
		{"null", `null`},
		{"sendId がない", `{"body":"hello"}`},
		{"body がない", `{"sendId":"abc"}`},
		{"sendId が null", `{"sendId":null,"body":"hello"}`},
		{"body が null", `{"sendId":"abc","body":null}`},
		{"sendId が数値", `{"sendId":123,"body":"hello"}`},
		{"body が数値", `{"sendId":"abc","body":123}`},
		{"body がオブジェクト", `{"sendId":"abc","body":{"text":"hello"}}`},
		{"body が配列", `{"sendId":"abc","body":["hello"]}`},
		{"JSON の値の後ろに、余分な内容がある", `{"sendId":"abc","body":"hello"} {"sendId":"abc","body":"hello"}`},
		{"JSON の値の後ろに、余分な文字がある", `{"sendId":"abc","body":"hello"}x`},
		{"本文に、エスケープされていない制御文字がある", "{\"sendId\":\"abc\",\"body\":\"a\x01b\"}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, ok := decodeRequest(tt.body); ok {
				t.Fatal("形式に合うと判定した")
			}
		})
	}
}

func TestDecodeSendMessageRequest_SizeLimit(t *testing.T) {
	const overhead = len(`{"sendId":"abc","body":""}`)

	atLimit := `{"sendId":"abc","body":"` + strings.Repeat("a", maxRequestBodyBytes-overhead) + `"}`
	if len(atLimit) != maxRequestBodyBytes {
		t.Fatalf("テストの前提が違う: %d", len(atLimit))
	}
	if _, _, ok := decodeRequest(atLimit); !ok {
		t.Fatal("上限と同じ大きさの要求を、形式に合わないと判定した")
	}

	overLimit := `{"sendId":"abc","body":"` + strings.Repeat("a", maxRequestBodyBytes-overhead+1) + `"}`
	if _, _, ok := decodeRequest(overLimit); ok {
		t.Fatal("上限を超える要求を、形式に合うと判定した")
	}
}
