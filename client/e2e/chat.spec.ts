import { execFileSync } from 'node:child_process'
import path from 'node:path'

import { expect, test, type Browser, type BrowserContext, type Page } from '@playwright/test'

// E2Eテスト(ADR-006)。動作検証(仕様「動作検証」)の手順のうち、複数の端末にまたがる
// 主要な経路(手順1〜5、手順9)を、互いに状態を共有しない、複数のブラウザのコンテキストで確かめる。
// 動作検証には、代えない。

const repositoryRoot = path.resolve(import.meta.dirname, '../..')

// テストごとに、始める前に、データを空の状態に戻す。
// サーバーは、メッセージと会話を、メモリに保持しないため、再起動は要らない。
test.beforeEach(() => {
  const user = process.env.PGUSER
  if (!user) {
    throw new Error('PGUSER が、与えられていません。make e2e で実行してください。')
  }
  execFileSync(
    'docker',
    ['compose', 'exec', '-T', 'postgres', 'psql', '-q', '-v', 'ON_ERROR_STOP=1', '-U', user, '-d', 'chat_e2e', '-c', 'TRUNCATE messages, conversations'],
    { cwd: repositoryRoot, stdio: 'pipe' },
  )
})

// テストが開いた、ブラウザのコンテキスト。テストの終わりに閉じる。
// 閉じないと、前のテストの画面が、接続を保ったまま、次のテストの間も残る。
const openContexts: BrowserContext[] = []

test.afterEach(async () => {
  await Promise.all(openContexts.splice(0).map((context) => context.close()))
})

/** 互いに状態を共有しないブラウザで、アプリを開き、利用者と相手を選ぶ。 */
async function openAs(browser: Browser, user: string, peer: string): Promise<Page> {
  const context = await browser.newContext()
  openContexts.push(context)
  const page = await context.newPage()
  await page.goto('/')
  await page.getByRole('button', { name: user }).click()
  await expect(page.getByTestId('current-user')).toHaveText(user)
  await page.getByRole('button', { name: peer }).click()
  await expect(page.getByRole('region', { name: `${peer} との会話` })).toBeVisible()
  return page
}

async function send(page: Page, body: string): Promise<void> {
  await page.getByRole('textbox', { name: 'メッセージ' }).fill(body)
  await page.getByRole('button', { name: '送信' }).click()
}

/** 完了したメッセージの本文を、示されている順に返す。 */
function bodies(page: Page) {
  return page.getByTestId('message').locator('.message-body')
}

test('手順1〜5: 複数の端末の間で、メッセージをやり取りする', async ({ browser, request }) => {
  // 手順1: どちらにも、メッセージがないことが示される(WS-21)。
  const browser1 = await openAs(browser, 'Alice', 'Bob')
  const browser2 = await openAs(browser, 'Bob', 'Alice')
  await expect(browser1.getByText('メッセージはありません')).toBeVisible()
  await expect(browser2.getByText('メッセージはありません')).toBeVisible()

  // 手順2: ブラウザ1に、完了したメッセージが示される。ブラウザ2に、操作なしに、同じメッセージが示される(WS-01、WS-02)。
  await send(browser1, 'こんにちは、Bob')
  for (const page of [browser1, browser2]) {
    const message = page.getByTestId('message')
    await expect(message).toHaveCount(1)
    await expect(message.locator('.message-body')).toHaveText('こんにちは、Bob')
    await expect(message.locator('.message-sender')).toHaveText('Alice')
    await expect(message.locator('time')).toHaveAttribute('datetime', /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/)
    await expect(message.locator('time')).not.toBeEmpty()
  }
  // 送信に使われた端末には、完了の応答と配信の両方が届くが、1通だけが示される(WS-37)。
  await expect(browser1.getByTestId('outgoing')).toHaveCount(0)
  await expect(browser1.getByRole('textbox', { name: 'メッセージ' })).toHaveValue('')

  // 手順3: 返信と、双方からの数通が、操作なしに示され、順序が、双方で一致する(WS-02、WS-07)。
  await send(browser2, 'こんにちは、Alice')
  await expect(bodies(browser1)).toHaveCount(2)
  const expected = ['こんにちは、Bob', 'こんにちは、Alice']
  for (let i = 1; i <= 3; i++) {
    await send(browser1, `Alice から ${i}`)
    await send(browser2, `Bob から ${i}`)
  }
  await expect(bodies(browser1)).toHaveCount(8)
  await expect(bodies(browser2)).toHaveCount(8)
  const order1 = await bodies(browser1).allTextContents()
  const order2 = await bodies(browser2).allTextContents()
  expect(order1).toEqual(order2)
  expect([...order1].sort()).toEqual(
    [...expected, 'Alice から 1', 'Alice から 2', 'Alice から 3', 'Bob から 1', 'Bob から 2', 'Bob から 3'].sort(),
  )
  // 1人が送った順は、保たれる。
  expect(order1.filter((body) => body.startsWith('Alice から'))).toEqual(['Alice から 1', 'Alice から 2', 'Alice から 3'])
  expect(order1.filter((body) => body.startsWith('Bob から'))).toEqual(['Bob から 1', 'Bob から 2', 'Bob から 3'])

  // 手順4: ブラウザ3に、それまでの履歴が示される。ブラウザ1からのメッセージが、ブラウザ3にも、
  // 操作なしに示される(WS-03、WS-20)。
  const browser3 = await openAs(browser, 'Alice', 'Bob')
  await expect(bodies(browser3)).toHaveText(order1)
  await send(browser1, '3つ目のブラウザにも届く')
  await expect(bodies(browser3)).toHaveText([...order1, '3つ目のブラウザにも届く'])
  await expect(bodies(browser2)).toHaveText([...order1, '3つ目のブラウザにも届く'])

  // 手順5: ブラウザ3で、carol を選び直し、相手に alice を選ぶ。alice と bob の会話のメッセージは、
  // 履歴としても、配信としても示されない(WS-25、WS-41)。
  await browser3.getByRole('button', { name: '利用者を選び直す' }).click()
  await browser3.getByRole('button', { name: 'Carol' }).click()
  await expect(browser3.getByTestId('current-user')).toHaveText('Carol')
  await browser3.getByRole('button', { name: 'Alice' }).click()
  await expect(browser3.getByText('メッセージはありません')).toBeVisible()

  await send(browser1, 'Carol には届かない')
  await expect(bodies(browser2)).toHaveCount(10)
  // 番兵: alice が carol に送ったメッセージを、サーバーへ直接送る。ブラウザ3が次に示すのは、これである。
  // alice と bob の会話のメッセージが届いていれば、これより先に示される。
  const sentinel = await request.post('/api/direct-conversations/carol/messages', {
    headers: { 'X-Fixed-User': 'alice' },
    data: { sendId: crypto.randomUUID(), body: 'Carol への番兵' },
  })
  expect(sentinel.status()).toBe(200)
  await expect(bodies(browser3)).toHaveText(['Carol への番兵'])
  await expect(browser3.getByText('Carol には届かない')).toHaveCount(0)
})

test('手順9: 画面の構造やプログラムとして解釈されうる本文が、文字として示され、実行されない(WS-27)', async ({ browser }) => {
  const browser1 = await openAs(browser, 'Alice', 'Bob')
  const browser2 = await openAs(browser, 'Bob', 'Alice')
  const dialogs: string[] = []
  for (const page of [browser1, browser2]) {
    page.on('dialog', (dialog) => {
      dialogs.push(dialog.message())
      void dialog.dismiss()
    })
  }

  const hostile = [
    '<script>window.__xss = true; alert("xss")</script>',
    '<img src="x" onerror="window.__xss = true; alert(\'xss\')">',
    '<b>太字</b> &lt;i&gt; &amp; <a href="javascript:alert(1)">link</a>',
  ]
  for (const body of hostile) {
    await send(browser1, body)
  }
  // 改行とタブは、そのまま示される(WS-11)。Shift+Enter で、改行を入力する。
  const textbox = browser1.getByRole('textbox', { name: 'メッセージ' })
  await textbox.click()
  await textbox.pressSequentially('1行目')
  await textbox.press('Shift+Enter')
  await textbox.pressSequentially('2行目')
  await textbox.press('Enter')

  for (const page of [browser1, browser2]) {
    await expect(bodies(page)).toHaveCount(4)
    expect(await bodies(page).allTextContents()).toEqual([...hostile, '1行目\n2行目'])
    // 本文から、要素が作られていない。
    await expect(page.getByRole('list', { name: 'メッセージ' }).locator('script, img, b, a, i')).toHaveCount(0)
    expect(await page.evaluate(() => (window as unknown as { __xss?: boolean }).__xss)).toBeUndefined()
    // 改行は、改行として示される。
    const lastBody = bodies(page).last()
    expect(await lastBody.evaluate((element) => getComputedStyle(element).whiteSpace)).toMatch(/^pre-wrap/)
    const singleLine = await bodies(page).first().evaluate((element) => element.getBoundingClientRect().height)
    const twoLines = await lastBody.evaluate((element) => element.getBoundingClientRect().height)
    expect(twoLines).toBeGreaterThan(singleLine)
  }
  expect(dialogs).toEqual([])
})
