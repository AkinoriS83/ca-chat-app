// 規則に違反する見本(ADR-005「検査の手段」)。
// make check-deps-selftest が、この import(または、APIの使用)が、違反として検出されることを確かめる。
// 通常の検査とビルドには、含まれない。ファイルの名前の .depcruise は、dependency-cruiser が、
// .eslint は、ESLint(eslint.deps.config.js)が、検出することを表す。

// 違反: ui から、adapter へ(画面の部品が、application を経由せずに通信する経路)
import { BrowserTimer } from '../../adapter/browserTimer'

export const sample = BrowserTimer
