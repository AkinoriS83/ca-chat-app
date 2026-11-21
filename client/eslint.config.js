// make lint が使う、ESLint の設定。
//
// application が、React と、ブラウザの入出力のAPIを使わないことの規則は、ここには含めない。
// make check-deps と make check-deps-selftest だけが、eslint.deps.config.js で実行する。
// make lint が違反を検出すると、検査の記録(docs/dependency-check-log.md)に残らないまま、
// 直されてしまうためである(ADR-008「取り漏らしを防ぐ手段」)。
import js from '@eslint/js'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  {
    ignores: [
      'dist/**',
      'coverage/**',
      'test-results/**',
      'playwright-report/**',
      'src/adapter/generated/**',
      // 規則に違反する見本。make check-deps-selftest だけが使う。
      '**/__depcheck__/**',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['src/**/*.{ts,tsx}'],
    languageOptions: { globals: globals.browser },
    rules: {
      '@typescript-eslint/consistent-type-imports': 'error',
    },
  },
  {
    files: ['src/**/*.tsx'],
    rules: {
      // 本文は、React の文字列として描画する。HTML として解釈させない(WS-27)。
      'no-restricted-syntax': [
        'error',
        {
          selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']",
          message: 'dangerouslySetInnerHTML は、使わない。本文は、文字列として描画する(WS-27)。',
        },
        {
          selector: "Property[key.name='dangerouslySetInnerHTML']",
          message: 'dangerouslySetInnerHTML は、使わない。本文は、文字列として描画する(WS-27)。',
        },
        {
          selector: "MemberExpression[property.name='innerHTML'], MemberExpression[property.name='outerHTML']",
          message: 'innerHTML は、使わない。本文は、文字列として描画する(WS-27)。',
        },
      ],
    },
  },
  {
    files: ['*.{js,ts,cjs}', 'e2e/**/*.ts'],
    languageOptions: { globals: globals.node },
  },
  {
    files: ['**/*.cjs'],
    languageOptions: { sourceType: 'commonjs' },
    rules: { '@typescript-eslint/no-require-imports': 'off' },
  },
)
