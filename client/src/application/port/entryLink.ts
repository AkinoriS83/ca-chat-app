/** アプリを開いたときのアドレス。案内から登録を続ける訪問者を、見分ける。 */
export interface EntryLink {
  /**
   * 案内の資格(確認のための資格)を、アドレスから読み取り、アドレスから除く。
   * アドレスに、資格がなければ、null を返す。
   */
  takeSignupCredential(): string | null
}
