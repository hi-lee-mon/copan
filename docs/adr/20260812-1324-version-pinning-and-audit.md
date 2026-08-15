# Node.js と pnpm のバージョンは `mise.toml` に集約し、依存はバージョン完全固定＋CI で `pnpm audit` を強制する

- ステータス: 承認
- 日付: 2026-08-12 13:24
- 関連: [20260811-2015 Turborepo](./20260811-2015-monorepo-turborepo.md)、[20260812-1309 パッケージマネージャに pnpm を採用する](./20260812-1309-package-manager-pnpm.md)、[20260812-1313 モノレポのディレクトリ構成](./20260812-1313-monorepo-directory-layout.md)、[チケット 002](../tickets/002-monorepo-scaffold.md)

## コンテキスト

[20260812-1309](./20260812-1309-package-manager-pnpm.md) で pnpm を採用した際、「Node.js と pnpm のバージョン固定方法」が残った。固定対象は2つある。

1. **Node.js 本体**
2. **pnpm 自身**

満たしたい条件は、**ローカルで自動的に切り替わること**と、**CI が同じバージョンを再現すること**の2つ。

手段は複数あり、しかも互いに競合する。

| 手段 | 対象 | 読むもの | 効き方 |
| --- | --- | --- | --- |
| `engines.node` | Node | pnpm | **検証のみ**（不一致でエラー。切り替えはしない） |
| `.node-version` | Node | mise / fnm / nvm / `actions/setup-node` | シェル側で切替 |
| `mise.toml` | **Node と pnpm の両方** | mise / `jdx/mise-action` | シェル側で切替 |
| `packageManager` | pnpm | Corepack / pnpm 自身 / turbo | 自動切替。**完全一致のみ**（範囲指定不可） |
| `devEngines.runtime` | Node | pnpm **10.14+** | 範囲指定可。満たさなければ自動ダウンロード |
| `devEngines.packageManager` | pnpm | pnpm **10.14+** / turbo | 範囲指定可。満たさなければ自動ダウンロード |

調査で判明した制約:

- **Turborepo は `package.json` にパッケージマネージャの宣言を要求する。** これが無いと、タスクを実行する以前にワークスペースの解決に失敗する。

  ```text
  × Could not resolve workspace.
  ╰─▶ Missing `devEngines.packageManager` or legacy `packageManager` field in package.json
  ```

  turbo は lockfile の種類から推測せず、宣言を読む。**つまり「バージョン情報を `package.json` の外だけに置く」構成は、Turborepo を採用した時点で成立しない**
- **`packageManager` フィールドは pnpm 11 で legacy 扱いになった。** `managePackageManagerVersions` / `packageManagerStrict` / `packageManagerStrictVersion` は削除され `pmOnFail` に統合。pnpm 11 の推奨は `devEngines.packageManager`（[pnpm 11.0 リリースノート](https://pnpm.io/blog/releases/11.0)）
- **Corepack は Node.js 25 以降で同梱されなくなった**（Node.js TSC が2025年3月に決議）。Node.js 24 LTS には同梱されている
- **pnpm は `devEngines` の `onFail` について `download` しか実装していない。** `error` / `warn` / `ignore` を書いても無視され、**範囲を満たさないバージョンで叩くと黙って別バージョンをダウンロードして切り替える**
- **mise は既定で `.node-version` を読まない。** mise 2025.10.0 以降、idiomatic version file（`.node-version` / `.nvmrc`）はパース負荷を理由に既定で無効。`mise settings add idiomatic_version_file_enable_tools node` というマシンごとの設定が要る
- 開発マシンでは**すでに mise が Node・Go・Python・AWS CLI などを管理している**
- mise から `node@24.19.0` と `pnpm@11.21.0` の両方が取得できることを `mise ls-remote` で確認済み

加えて、依存パッケージそのもののバージョン方針と、脆弱性の検出方法も未決だった。

## 決定

### 1. バージョンの決定権は `mise.toml` に置く

```toml
# mise.toml
[tools]
node = "24.19.0"
pnpm = "11.21.0"
```

- **Node.js は 24 系（LTS）の最新パッチ**を指定する。26.7.0 が最新だが Current であり、LTS 化は2026年10月予定
- **pnpm は 2026年8月時点の最新である 11.21.0**（ローカルの 10.13.1 から上げる）
- セットアップは `mise install` の1コマンドに集約する
- **バージョンを上げるときに書き換えるのは、原則このファイルだけ**

### 2. `package.json` には `devEngines.packageManager` を範囲で置く

Turborepo が要求するため、`package.json` 側にも宣言が要る。ただし**バージョンを重複させない形にする**。

```jsonc
// package.json
{
  "devEngines": {
    "packageManager": { "name": "pnpm", "version": "11.x" }
  }
}
```

- **書くのは範囲のみ。** 具体的なバージョンは `mise.toml` が決める
- **`devEngines.runtime`（Node）は書かない。** turbo が要求するのは packageManager だけであり、書けば `mise.toml` と重複するだけになる
- **legacy の `packageManager` フィールドは使わない**
- `.node-version` と Corepack も使わない

### 3. CI も同じ `mise.toml` を読む

```yaml
- uses: jdx/mise-action@v4
```

`actions/setup-node` と `pnpm/action-setup` は使わない。ジョブに組み込む具体はチケット 006 で行う。

### 4. 依存パッケージはバージョンを完全固定する

```yaml
# pnpm-workspace.yaml
saveExact: true
```

`^` や `~` を付けず、`pnpm add` した時点のバージョンをそのまま記録する。

**pnpm 11 では、この種の設定の置き場所が `.npmrc` から `pnpm-workspace.yaml` に移っている。** `.npmrc` に `save-exact=true` と書いた場合、pnpm が `pnpm-workspace.yaml` へ自動的に移行する。

### 5. CI で `pnpm audit` を実行し、critical / high があれば落とす

```bash
pnpm audit --audit-level=high
```

`--audit-level=high` は high 以上（＝ high と critical）の脆弱性が見つかった場合に非ゼロで終了する。CI のジョブとして組み込む具体はチケット 006 で行う。

## 理由

### `mise.toml` に決定権を集約した理由

- **`package.json` 側を範囲指定に留めることで、バージョンを直す場所が1箇所に保たれる。** Turborepo の要求により宣言そのものは2箇所に現れるが、`11.x` という範囲は pnpm のメジャーバージョンを上げるときにしか動かない。**日常的な更新（パッチ・マイナー）で2箇所を同期する必要がない**
- **非推奨・廃止予定の仕組みに乗らない。** legacy `packageManager` は pnpm 11 で非推奨、Corepack は Node 25 で同梱されなくなる。この2つを組み合わせた構成は、**採用した時点ですでに移行予定を抱えている**
- **ローカルのセットアップが `mise install` だけになる。** `.node-version` を使う場合は「mise の idiomatic version file を有効化する」というマシンごとの設定が別途要り、**忘れると黙って無視されてグローバルの Node が使われる**
- **既知からの距離が近い。** 開発マシンではすでに mise が Go や Python を管理している。同じ道具に揃えることで、覚えることが増えない
- **Go も同じ枠組みに載る。** チケット 003 で Go のバージョンを固定する必要が出るが、`mise.toml` に1行足すだけで済む。`devEngines` は JS エコシステム専用のため Go には使えない
- 可逆性が高い。`mise.toml` を削除して `.node-version` + `packageManager` に戻すのは、ファイル2枚の書き換えで済む

### 依存を完全固定する理由

- **再現性。** 開発者1名・週5時間という状況では、「昨日動いていたものが今日動かない」の原因調査に使える時間がない。lockfile があれば `pnpm install` の結果は固定されるが、`package.json` 側も固定しておくことで、依存を追加した際に既存の依存が意図せず動くことを防げる
- **更新を能動的な行為にする。** `^` があると更新が「たまたま起きる」ものになり、いつ何が変わったかがコミット履歴に残らない。完全固定なら、更新は必ず差分として現れる

### CI で `pnpm audit` を強制する理由

- **依存を完全固定する決定と対になっている。** 固定すると脆弱性の修正も自動では入らない。放置を防ぐ仕組みが要る
- **閾値を critical / high に置くのは、落ちたら必ず対応する水準に合わせるため。** moderate 以下まで落とすと、対応できない指摘で CI が赤いままになり、赤信号が無視されるようになる。これは仕組みとして最悪の状態

## 却下した案

### `devEngines.packageManager` にも完全一致のバージョンを書く

`mise.toml` と同じ `11.21.0` を書けば、pnpm のバージョンが `package.json` を見るだけで分かる。

**却下の理由**: **更新のたびに2箇所を直す運用になり、直し忘れが静かに事故になる。** pnpm は `onFail` の `download` しか実装していないため、`mise.toml` を 11.22.0 に上げて `package.json` を直し忘れると、**警告も出さずに 11.21.0 を別途ダウンロードして切り替える**。mise が用意したバージョンが無視されていることに気づけない。範囲指定ならこの事故が起きない。

### `devEngines.runtime`（Node）も併せて書く

Node の要件も `package.json` に現れるので、mise を使っていない人にも伝わる。

**却下の理由**: **turbo は要求していない。** 書けば `mise.toml` と重複するだけで、上と同じ「直し忘れ」のリスクを Node にも広げることになる。しかも pnpm は範囲を満たさない Node を勝手にダウンロードするため、**mise が管理する Node と pnpm が落としてきた Node が併存する**という分かりにくい状態を生む。

### legacy の `packageManager` フィールドを使う

turbo はこちらも受け付ける。実務で最も広く使われている形でもある。

**却下の理由**: **範囲指定ができず完全一致のみ**のため、上記の「2箇所を直す運用」が避けられない。加えて pnpm 11 で非推奨になっており、採用する pnpm 自身が別の手段を推奨しているフィールドを新規に採用することになる。

### `.node-version`（mise）+ `packageManager`（Corepack）

Node は `.node-version` に書いて mise が切り替え、pnpm は `packageManager` に書いて Corepack が切り替える。CI は `actions/setup-node` と `pnpm/action-setup` がそれぞれ同じファイルを読む。**GitHub 公式・pnpm 公式の action だけで組めるのが最大の利点。**

**却下の理由**: 3点ある。

1. **`packageManager` が pnpm 11 で非推奨**、かつ範囲指定ができない
2. **Corepack が Node 25 で同梱されなくなる。** 採用と同時に移行予定を抱える
3. **`.node-version` を mise に読ませるには、マシンごとの設定が要る。** clone しただけでは効かず、**設定漏れがエラーにならず、グローバルの Node が黙って使われる**

### `mise.toml`（ローカル）+ CI は `actions/setup-node` / `pnpm/action-setup`（個別指定）

ローカルは mise に統一しつつ、CI は公式 action にバージョンを直接書く。workflow 単体の分かりやすさと、サードパーティ action に依存しない点が利点。

**却下の理由**: バージョンの情報源が `mise.toml` と workflow の**2箇所に分かれる**。Node を上げるときに2箇所直す必要があり、**直し忘れても両方が正常終了してしまう**。「ローカルでは通るのに CI だけ落ちる」という、原因の特定に時間がかかる形で表面化する。CI 側の記述量も `jdx/mise-action` の1ステップに対して2ステップで、単純さでも勝っていない。

### `engines.node` で指定する

**却下の理由**: pnpm は `engines` を検証してエラーにするが、**バージョンを切り替えてはくれない**。「合っていないと分かる」だけで「合わせる」手段が別途要る。`mise.toml` を置く以上、重複した情報になる。

### 依存に `^`（キャレット）を許す

マイナー・パッチの修正が自動で入り、脆弱性対応が受動的に進む。

**却下の理由**: 再現性と「更新を差分として残す」ことを優先した。脆弱性対応は `pnpm audit` を CI で強制することで別途担保する。

## トレードオフとして受け入れること

- **バージョンの宣言は完全には1箇所にならない。** Turborepo の要求により `package.json` にも `devEngines.packageManager` が要る。範囲指定にすることで**日常的な更新では同期不要**にしたが、**pnpm のメジャーバージョンを上げるときは `mise.toml` と `package.json` の両方を直す必要がある**
- **`mise.toml` は初回に `mise trust` が必要。** mise は未信頼の設定ファイルを実行しない。促されるので気づけるが、手順としては残る
- **mise が入っていない環境では、Node のバージョン指定が無効になる。** pnpm については `devEngines.packageManager` が最低限の防波堤になるが、Node には何もない。**セットアップ手順を README に書くこと**が前提になる
- **CI が `jdx/mise-action` に依存する。** GitHub 公式でも pnpm 公式でもなく、mise 作者が提供するサードパーティ action。メンテナンスが止まった場合は、却下した案（`actions/setup-node` + `pnpm/action-setup`）がそのまま代替になる
- **CI のセットアップが `actions/setup-node` より遅くなる可能性がある。** `mise-action` は既定でキャッシュが有効だが、実測は 006 で確認する
- **依存の更新が手作業になる。** 完全固定のため、更新は明示的に行う必要がある。Dependabot / Renovate の導入は別途検討する
- **`pnpm audit` が外部要因で CI を落としうる。** 自分のコードを1行も変えていなくても、新たに報告された脆弱性で赤くなる。これは意図した挙動として受け入れる
- **pnpm を 10.13.1 から 11.21.0 へメジャーアップグレードする。** スキャフォールドの時点なので影響範囲は最小

## 結果として必要になる決定

- **Go のバージョンを `mise.toml` に含めるか**（チケット 003）。含めるなら CI の Go セットアップも `mise-action` に寄せることになる
- mise の lockfile（`mise.lock`）を使うか。バージョンを完全固定しているため必須ではないが、ツールのダウンロード元まで固定したい場合に検討する
- 依存更新の自動化（Dependabot / Renovate を入れるか、入れるならどちらか）
- `pnpm audit` で検出されたが**修正版が存在しない**脆弱性をどう扱うか（`pnpm.auditConfig.ignoreCves` による除外の運用ルール）。実際に詰まった時点で決める
- CI のジョブ構成（チケット 006）
- Node.js 24 の EOL に向けた更新のタイミング
- **mise が `package.json` の `devEngines` を読むようになった場合**、宣言を1箇所に戻せる可能性がある（[jdx/mise Discussion #7379](https://github.com/jdx/mise/discussions/7379)）。実装されたら再検討する
