//go:build depcheck_sample

// 規則に違反する見本(ADR-005「検査の手段」)。
// make check-deps-selftest が、この import が違反として検出されることを確かめる。
// 通常のビルドと検査には、含まれない。
//
// このパッケージには、import の規則がない。規則のないパッケージは、すべての import が違反になる。
package depchecksample

// 違反: 規則のないパッケージからの import
import _ "errors"
