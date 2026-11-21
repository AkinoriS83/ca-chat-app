// 依存規則と飛ばし禁止の検査(ADR-005)の設定。
//
// make check-deps と make check-deps-selftest だけが使う。直接には、実行しない。
// 検査の記録(docs/dependency-check-log.md)に、残らないためである。
//
// 規則は、解決した後のファイルに対して適用する。型だけの import も、対象にする。
// 規則に違反する見本(__depcheck__)は、DEPCHECK_SAMPLES=1 のときだけ、対象にする。
// 置き場所を加えるときは、規則を加え、アーキテクチャ概要(docs/architecture.md)を更新する。
const includeSamples = process.env.DEPCHECK_SAMPLES === '1'

/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: 'application-only-imports-application',
      comment: 'application(port を含む)は、application の中だけを import する。React も、adapter も、ui も、import しない。',
      severity: 'error',
      from: { path: '^src/application/', pathNot: '\\.test\\.ts$' },
      to: { pathNot: '^src/application/' },
    },
    {
      name: 'application-test-imports',
      comment: 'application のテストは、application の中と、テスティングフレームワークだけを import する。',
      severity: 'error',
      from: { path: '^src/application/.*\\.test\\.ts$' },
      to: { pathNot: ['^src/application/', '^node_modules/vitest/'] },
    },
    {
      name: 'adapter-not-to-application-behavior',
      comment: 'adapter は、application のうち、port と、ポートが参照する型(model.ts)だけを import する。',
      severity: 'error',
      from: { path: '^src/adapter/' },
      to: { path: '^src/application/', pathNot: ['^src/application/port/', '^src/application/model\\.ts$'] },
    },
    {
      name: 'adapter-imports-model-as-type-only',
      comment: 'adapter から model.ts への import は、型だけとする。',
      severity: 'error',
      from: { path: '^src/adapter/' },
      to: { path: '^src/application/model\\.ts$', dependencyTypesNot: ['type-only'] },
    },
    {
      name: 'adapter-not-to-ui',
      comment: 'adapter と ui は、互いに import しない。',
      severity: 'error',
      from: { path: '^src/adapter/' },
      to: { path: '^src/(ui/|main\\.tsx$)' },
    },
    {
      name: 'adapter-no-react',
      comment: 'adapter は、React を使わない。画面は、ui が受け持つ。',
      severity: 'error',
      from: { path: '^src/adapter/' },
      to: { path: '^node_modules/(react|react-dom)/' },
    },
    {
      name: 'ui-not-to-adapter',
      comment: 'ui は、adapter(生成した型を含む)を import しない。画面の部品が、application を経由せずに通信する経路を、作らない。',
      severity: 'error',
      from: { path: '^src/ui/' },
      to: { path: '^src/(adapter/|main\\.tsx$)' },
    },
    {
      name: 'ui-not-to-port',
      comment: 'ui は、application のうち、port を import しない。',
      severity: 'error',
      from: { path: '^src/ui/' },
      to: { path: '^src/application/port/' },
    },
    {
      name: 'generated-only-from-adapter',
      comment: '生成した型は、adapter だけが import する。',
      severity: 'error',
      from: { pathNot: '^src/adapter/' },
      to: { path: '^src/adapter/generated/' },
    },
    {
      name: 'unlisted-location',
      comment: 'src の下の、規則のない置き場所。置き場所を加えるときは、規則を加える。',
      severity: 'error',
      from: { path: '^src/', pathNot: ['^src/(application|adapter|ui)/', '^src/main\\.tsx$', '^src/vite-env\\.d\\.ts$'] },
      to: {},
    },
    {
      name: 'no-circular',
      comment: '循環する import を、作らない。',
      severity: 'error',
      from: {},
      to: { circular: true },
    },
  ],
  options: {
    // 型だけの import も、依存として扱う。
    tsPreCompilationDeps: true,
    tsConfig: { fileName: 'tsconfig.json' },
    doNotFollow: { path: 'node_modules' },
    exclude: includeSamples ? undefined : { path: '__depcheck__' },
    enhancedResolveOptions: {
      exportsFields: ['exports'],
      conditionNames: ['import', 'require', 'node', 'default', 'types'],
      mainFields: ['module', 'main', 'types', 'typings'],
    },
  },
}
