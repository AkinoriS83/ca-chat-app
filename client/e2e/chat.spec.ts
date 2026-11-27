import { execFileSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import path from 'node:path'

import { expect, test, type Browser, type BrowserContext, type Page } from '@playwright/test'

// E2Eテスト(ADR-006)。「登録とログイン」の仕様「動作検証」の手順のうち、複数の端末にまたがる
// 主要な経路(手順1〜4、7、9)を、互いに状態を共有しない、複数のブラウザのコンテキストで確かめる。
// 動作検証には、代えない。
//
// 案内は、ローカルの開発環境の Mailpit が受け取る(ADR-018)。make db-up で、起動しておく。
// 案内は、Mailpit の API で、宛先を指定して読む。Mailpit は、テストの間で共有されるため、
// 連絡先は、実行のたびに変える。

const repositoryRoot = path.resolve(import.meta.dirname, '../..')

function requireEnv(name: string): string {
  const value = process.env[name]
  if (!value) {
    throw new Error(`${name} が、与えられていません。make e2e で実行してください。`)
  }
  return value
}

const mailpitApi = `http://127.0.0.1:${requireEnv('MAILPIT_UI_PORT')}/api/v1`

// テストごとに、始める前に、保管データを空の状態に戻す。
// サーバーは、利用者とメッセージを、メモリに保持しないため、再起動は要らない。
test.beforeEach(() => {
  execFileSync(
    'docker',
    [
      'compose', 'exec', '-T', 'postgres', 'psql', '-q', '-v', 'ON_ERROR_STOP=1',
      '-U', requireEnv('PGUSER'), '-d', 'chat_e2e',
      '-c', 'TRUNCATE messages, conversations, sessions, signups, users',
    ],
    { cwd: repositoryRoot, stdio: 'pipe' },
  )
})

// テストが開いた、ブラウザのコンテキスト。テストの終わりに閉じる。
// 閉じないと、前のテストの画面が、接続を保ったまま、次のテストの間も残る。
const openContexts: BrowserContext[] = []

test.afterEach(async () => {
  await Promise.all(openContexts.splice(0).map((context) => context.close()))
})

type Account = { contact: string; handle: string; displayName: string; secret: string }

const contactLabel = '連絡先(メールアドレス)'
const secretLabel = '秘密(パスワード)'

/** 実行のたびに異なる連絡先を持つ、利用者を作る。 */
function account(handle: string, displayName: string): Account {
  const suffix = randomBytes(6).toString('hex')
  return {
    contact: `${handle.toLowerCase()}-${suffix}@example.com`,
    handle,
    displayName,
    secret: `${handle} e2e secret ${suffix}`,
  }
}

/** 連絡先に届いた、登録を続けるための案内から、アドレスを取り出す。 */
async function signupLink(contact: string): Promise<string> {
  let link: string | undefined
  await expect
    .poll(
      async () => {
        const list = await fetch(`${mailpitApi}/search?query=${encodeURIComponent(`to:${contact}`)}`)
        const { messages } = (await list.json()) as { messages: { ID: string }[] }
        // 案内は、1通だけ届く(RL-01)。
        if (messages.length !== 1 || !messages[0]) {
          return messages.length
        }
        const mail = await fetch(`${mailpitApi}/message/${messages[0].ID}`)
        const { Text } = (await mail.json()) as { Text: string }
        link = /https?:\/\/\S+#signup=\S+/.exec(Text)?.[0]
        return 1
      },
      { message: `${contact} に、案内が1通届く` },
    )
    .toBe(1)
  if (!link) {
    throw new Error('案内に、登録を続けるためのアドレスが、ありません')
  }
  return link
}

/** 互いに状態を共有しない、新しいブラウザを開く。 */
async function newBrowser(browser: Browser): Promise<BrowserContext> {
  const context = await browser.newContext()
  openContexts.push(context)
  return context
}

/** 手順1: 連絡先を入力して、登録を申し込み、届いた案内に従って、登録を完了する。 */
async function register(context: BrowserContext, user: Account): Promise<Page> {
  const requestPage = await context.newPage()
  await requestPage.goto('/')
  await requestPage.getByRole('button', { name: '登録を申し込む' }).click()
  await requestPage.getByLabel(contactLabel).fill(user.contact)
  await requestPage.getByRole('button', { name: '申し込む' }).click()
  await expect(requestPage.getByRole('status')).toHaveText('案内を送りました。届いた案内に従って、登録を続けてください。')
  await requestPage.close()

  // 案内のアドレスを、開く。
  const page = await context.newPage()
  await page.goto(await signupLink(user.contact))
  await expect(page.getByRole('heading', { name: '登録', exact: true })).toBeVisible()
  // 確認のための資格は、読み取った後、アドレスから除かれている(RL-33)。
  expect(new URL(page.url()).hash).toBe('')

  await page.getByLabel('識別名', { exact: true }).fill(user.handle)
  await page.getByLabel('表示名', { exact: true }).fill(user.displayName)
  await page.getByLabel(secretLabel).fill(user.secret)
  await page.getByRole('button', { name: '登録する' }).click()
  await expect(page.getByRole('status')).toHaveText('登録を完了しました。登録した連絡先と、秘密で、ログインしてください。')
  return page
}

/** 手順2: ログインの画面から、ログインする。 */
async function login(page: Page, user: Account): Promise<void> {
  await page.getByLabel(contactLabel).fill(user.contact)
  await page.getByLabel(secretLabel).fill(user.secret)
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page.getByTestId('current-user')).toHaveText(user.displayName)
}

/** 登録して、ログインした状態の、新しいブラウザを開く。 */
async function registerAndLogin(browser: Browser, user: Account): Promise<{ context: BrowserContext; page: Page }> {
  const context = await newBrowser(browser)
  const page = await register(context, user)
  await login(page, user)
  return { context, page }
}

async function findPeer(page: Page, handle: string): Promise<void> {
  await page.getByLabel('相手の識別名').fill(handle)
  await page.getByRole('button', { name: '探す' }).click()
}

/** 相手を特定して、会話を開く。 */
async function openConversation(page: Page, handle: string, peer: Account): Promise<void> {
  await findPeer(page, handle)
  await expect(page.getByTestId('found-peer')).toContainText(`@${peer.handle}`)
  await page.getByRole('button', { name: '会話を開く' }).click()
  await expect(page.getByRole('region', { name: `${peer.displayName} との会話` })).toBeVisible()
}

async function send(page: Page, body: string): Promise<void> {
  await page.getByRole('textbox', { name: 'メッセージ' }).fill(body)
  await page.getByRole('button', { name: '送信' }).click()
}

/** 完了したメッセージの本文を、示されている順に返す。 */
function bodies(page: Page) {
  return page.getByTestId('message').locator('.message-body')
}

/** ログインを求める画面が示され、会話の内容と、利用者の情報が、示されていないことを確かめる。 */
async function expectLoginScreen(page: Page): Promise<void> {
  await expect(page.getByRole('button', { name: 'ログイン' })).toBeVisible()
  await expect(page.getByTestId('current-user')).toHaveCount(0)
  await expect(page.getByTestId('message')).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: 'メッセージ' })).toHaveCount(0)
}

test('手順1〜4: 登録し、ログインし、相手を特定して、メッセージをやり取りする', async ({ browser }) => {
  const alice = account('Alice_e2e', 'Alice')
  const bob = account('Bob_e2e', 'Bob')

  // 手順1、2: 互いに状態を共有しないブラウザで、登録し、ログインする(RL-01、RL-12)。
  const { page: browser1 } = await registerAndLogin(browser, alice)
  const { page: browser2 } = await registerAndLogin(browser, bob)
  await expect(browser1.getByTestId('current-handle')).toHaveText(`@${alice.handle}`)

  // 手順3: 大文字と小文字を違えて入力しても、相手が登録した通りの識別名と、表示名が示される(RL-24)。
  await openConversation(browser1, 'bob_E2E', bob)
  await expect(browser1.getByTestId('found-peer')).toContainText('Bob')
  await expect(browser1.getByText('メッセージはありません')).toBeVisible()
  await send(browser1, 'こんにちは、Bob')
  await expect(bodies(browser1)).toHaveText(['こんにちは、Bob'])

  // 相手は、会話を開くと、履歴が示される。返信は、双方に、操作なしに届く(RL-41)。
  await openConversation(browser2, alice.handle, alice)
  await expect(bodies(browser2)).toHaveText(['こんにちは、Bob'])
  await send(browser2, 'こんにちは、Alice')
  await expect(bodies(browser2)).toHaveText(['こんにちは、Bob', 'こんにちは、Alice'])
  await expect(bodies(browser1)).toHaveText(['こんにちは、Bob', 'こんにちは、Alice'])

  // 再読み込みの後も、ログインしたままである(RL-21)。会話を開くと、同じ履歴が示される。
  await browser1.reload()
  await expect(browser1.getByTestId('current-user')).toHaveText('Alice')
  await openConversation(browser1, bob.handle, bob)
  await expect(bodies(browser1)).toHaveText(['こんにちは、Bob', 'こんにちは、Alice'])

  // 手順4: 存在しない識別名と、識別名の一部は、見つからないと示される(RL-25)。
  for (const handle of ['nobody_e2e', 'Bob_e2']) {
    await findPeer(browser1, handle)
    await expect(browser1.getByRole('alert')).toHaveText('見つかりませんでした。')
    await expect(browser1.getByTestId('found-peer')).toHaveCount(0)
  }
  // 自分の識別名は、相手にできないと示される(RL-26)。
  await findPeer(browser1, alice.handle.toUpperCase())
  await expect(browser1.getByRole('alert')).toHaveText('自分自身は、相手にできません。')
})

test('手順7: ログアウトすると、同じブラウザの、すべてのタブが、ログインを求める画面になり、メッセージは届かない', async ({ browser }) => {
  const alice = account('Alice_e2e', 'Alice')
  const bob = account('Bob_e2e', 'Bob')
  const { context: context1, page: tab1 } = await registerAndLogin(browser, alice)
  const { page: browser2 } = await registerAndLogin(browser, bob)

  // 同じブラウザで、タブを、もう1つ開く。ログイン状態は、タブが共有する(RL-D4)。
  const tab2 = await context1.newPage()
  await tab2.goto('/')
  await expect(tab2.getByTestId('current-user')).toHaveText('Alice')
  for (const tab of [tab1, tab2]) {
    await openConversation(tab, bob.handle, bob)
  }
  await openConversation(browser2, alice.handle, alice)
  await send(browser2, 'ログアウトの前')
  for (const tab of [tab1, tab2]) {
    await expect(bodies(tab)).toHaveText(['ログアウトの前'])
  }

  // 一方のタブで、ログアウトする。
  await tab1.getByRole('button', { name: 'ログアウト' }).click()

  // どちらのタブにも、操作なしに、ログインを求める画面が示され、会話の内容が示されない(RL-17、RL-19)。
  await expectLoginScreen(tab1)
  await expectLoginScreen(tab2)
  await expect(tab2.getByRole('status')).toHaveText('ログインし直してください(ログイン状態が、無効になりました)。')

  // その後のメッセージは、届かない(RL-18)。送信した側には、示される。
  await send(browser2, 'ログアウトの後')
  await expect(bodies(browser2)).toHaveText(['ログアウトの前', 'ログアウトの後'])
  await expectLoginScreen(tab1)
  await expectLoginScreen(tab2)

  // 再読み込みしても、ログインしていない。
  await tab2.reload()
  await expectLoginScreen(tab2)
})

test('手順9: 別のタブで、別の利用者としてログインし直されたタブは、前の利用者の会話を示し続けない', async ({ browser }) => {
  const alice = account('Alice_e2e', 'Alice')
  const bob = account('Bob_e2e', 'Bob')
  const carol = account('Carol_e2e', 'Carol')
  const { context: context1, page: tab1 } = await registerAndLogin(browser, alice)
  const { page: browser2 } = await registerAndLogin(browser, bob)
  const carolContext = await newBrowser(browser)
  await (await register(carolContext, carol)).close()

  const tab2 = await context1.newPage()
  await tab2.goto('/')
  for (const tab of [tab1, tab2]) {
    await openConversation(tab, bob.handle, bob)
  }
  await send(tab1, 'alice から bob へ')
  await expect(bodies(tab2)).toHaveText(['alice から bob へ'])
  await tab2.getByRole('textbox', { name: 'メッセージ' }).fill('alice として、入力中の本文')

  // 一方のタブで、ログアウトし、carol としてログインする。
  await tab1.getByRole('button', { name: 'ログアウト' }).click()
  await login(tab1, carol)

  // もう一方のタブは、alice の会話を示し続けない。alice を示したまま、送信できない(RL-20)。
  await expectLoginScreen(tab2)
  await expect(tab2.getByText('alice から bob へ')).toHaveCount(0)

  // carol としてのタブには、alice と bob の会話は、示されない。bob から carol へのメッセージは、届く。
  await openConversation(tab1, bob.handle, bob)
  await expect(tab1.getByText('メッセージはありません')).toBeVisible()
  await openConversation(browser2, carol.handle, carol)
  await send(browser2, 'bob から carol へ')
  await expect(bodies(tab1)).toHaveText(['bob から carol へ'])

  // もう一方のタブを再読み込みすると、ブラウザが保つログイン状態(carol)で、開く。
  await tab2.reload()
  await expect(tab2.getByTestId('current-user')).toHaveText('Carol')
})
