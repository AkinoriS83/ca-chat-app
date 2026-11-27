// よく使われる秘密の一覧を、出所のファイルから取り出す(ADR-017)。
// scripts/extract-common-secrets.sh が呼ぶ。標準ライブラリだけを使う。
//
//	go run scripts/extract-common-secrets.go <出所のファイル> <書き出す先>
//
// 取り出す条件は、次の通りである。行の順は、出所のままとする。
//   - 15文字以上、128文字以下の行。文字は、Unicode のコードポイントで数える(仕様「入力の規則」)。
//   - @ を含む行は、除く(連絡先の形の値を、リポジトリに写さないため)。
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	minLength = 15
	maxLength = 128
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: extract-common-secrets <出所のファイル> <書き出す先>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "extract-common-secrets:", err)
		os.Exit(1)
	}
}

func run(sourcePath, outPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(out)

	inLength, withAt, written := 0, 0, 0
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		length := utf8.RuneCountInString(line)
		if length < minLength || length > maxLength {
			continue
		}
		inLength++
		if strings.Contains(line, "@") {
			withAt++
			continue
		}
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return err
		}
		written++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	fmt.Printf("長さの条件を満たす行: %d、@ を含む行: %d、書き出した行: %d\n", inLength, withAt, written)
	return nil
}
