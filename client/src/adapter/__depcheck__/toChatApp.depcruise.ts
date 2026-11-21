// 規則に違反する見本(ADR-005「検査の手段」)。
// make check-deps-selftest が、この import(または、APIの使用)が、違反として検出されることを確かめる。
// 通常の検査とビルドには、含まれない。ファイルの名前の .depcruise は、dependency-cruiser が、
// .eslint は、ESLint(eslint.deps.config.js)が、検出することを表す。

// 違反: adapter から、application の振る舞い(chatApp.ts)へ。型だけの import も、違反である
import type { ChatApp } from '../../application/chatApp'

export type Sample = ChatApp
