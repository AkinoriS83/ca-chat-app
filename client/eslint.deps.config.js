// 依存規則の検査(ADR-005)のうち、ESLint で行うものの設定。
//
// application が、React と、ブラウザの入出力のAPIを使わないことを検査する。
// make check-deps と make check-deps-selftest だけが使う。直接には、実行しない。
// 検査の記録(docs/dependency-check-log.md)に、残らないためである。
//
// 規則に違反する見本(__depcheck__)は、DEPCHECK_SAMPLES=1 のときだけ、対象にする。
import tseslint from 'typescript-eslint'

const includeSamples = process.env.DEPCHECK_SAMPLES === '1'

// ブラウザと Node.js の、入出力のAPI。application は、ポートを通して使う。
const restrictedGlobals = [
  'fetch',
  'XMLHttpRequest',
  'WebSocket',
  'EventSource',
  'BroadcastChannel',
  'Worker',
  'SharedWorker',
  'setTimeout',
  'setInterval',
  'setImmediate',
  'clearTimeout',
  'clearInterval',
  'clearImmediate',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'requestIdleCallback',
  'queueMicrotask',
  'crypto',
  'performance',
  'window',
  'self',
  'globalThis',
  'document',
  'navigator',
  'location',
  'history',
  'localStorage',
  'sessionStorage',
  'indexedDB',
  'caches',
  'cookieStore',
  'alert',
  'confirm',
  'prompt',
  'process',
  'require',
].map((name) => ({
  name,
  message: 'application は、ブラウザの入出力のAPIを使わない。ポートを通す(ADR-005)。',
}))

export default tseslint.config(
  {
    ignores: includeSamples ? [] : ['**/__depcheck__/**'],
  },
  {
    files: ['src/application/**/*.{ts,tsx}'],
    languageOptions: { parser: tseslint.parser },
    linterOptions: {
      // この設定にない規則についての、無効化の注釈は、報告しない。
      reportUnusedDisableDirectives: 'off',
    },
    rules: {
      'no-restricted-globals': ['error', ...restrictedGlobals],
      'no-restricted-imports': [
        'error',
        {
          paths: [
            { name: 'react', message: 'application は、React を使わない(ADR-005)。' },
            { name: 'react-dom', message: 'application は、React を使わない(ADR-005)。' },
          ],
          patterns: [
            { group: ['react/*', 'react-dom/*'], message: 'application は、React を使わない(ADR-005)。' },
            { group: ['node:*'], message: 'application は、入出力のAPIを使わない(ADR-005)。' },
          ],
        },
      ],
    },
  },
)
