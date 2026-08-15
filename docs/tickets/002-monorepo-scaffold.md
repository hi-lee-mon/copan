# 002: Turborepo のモノレポ骨組みを作る

- ステータス: 未着手（**着手可能**）
- 見積: 3h / 実績: -h
- 依存: **なし**（001 への依存は解消。パッケージ名を `@repo/*` にしたため名称確定を待たない — [ADR 20260812-1313](../adr/20260812-1313-monorepo-directory-layout.md) / [ADR 20260814-1826](../adr/20260814-1826-defer-trademark-research.md)）
- 関連: [ADR 20260811-2015 Turborepo](../adr/20260811-2015-monorepo-turborepo.md)、[ADR 20260812-1305 Go の置き方](../adr/20260812-1305-go-in-turborepo-workspace.md)、[ADR 20260812-1309 pnpm](../adr/20260812-1309-package-manager-pnpm.md)、[ADR 20260812-1313 ディレクトリ構成](../adr/20260812-1313-monorepo-directory-layout.md)、[ADR 20260812-1324 バージョン固定と audit](../adr/20260812-1324-version-pinning-and-audit.md)、[技術選定ガイド G-1 / G-2](../tech-decision-guide.md)

## 1. 目的

`apps/` と `packages/` を持つワークスペースを作り、**リポジトリのルートから `turbo` でタスクを流せる状態**にする。

## 2. 背景

[ADR 20260811-2015](../adr/20260811-2015-monorepo-turborepo.md) で Turborepo を採用済み。同ADRの「結果として必要になる決定」に残っている **G-2（パッケージマネージャ）とディレクトリ構成** を、このチケットで確定させる。

以降のチケット（TypeSpec の生成、Go のビルド、CI）はすべてこの上に載るため、ここで構成を決めておかないと後続が置き場所に困る。

## 3. 事前調査

### ワークスペースと Turborepo の役割分担

混同しやすいので先に分けておく。

|                                                        | 担当                                                  |
| ------------------------------------------------------ | ----------------------------------------------------- |
| **パッケージマネージャのワークスペース**（pnpm / npm） | 依存の解決とインストール。パッケージ間の参照          |
| **Turborepo**                                          | **タスクの依存グラフとキャッシュ。** 依存解決はしない |

Turborepo はパッケージマネージャの代替ではなく、その上に乗る**タスクランナー**。

### 決定済み: pnpm（G-2）

[ADR 20260812-1309](../adr/20260812-1309-package-manager-pnpm.md) で **pnpm** に決定。決め手はファントム依存を構造的に防げること。npm workspaces を却下した理由は ADR 側に記録してある。

|                  | pnpm                                               | npm workspaces                 |
| ---------------- | -------------------------------------------------- | ------------------------------ |
| インストール方式 | コンテンツアドレス可能なストア＋シンボリックリンク | フラットな巻き上げ             |
| 未宣言の依存     | **参照できない**（厳密）                           | 巻き上げにより参照できてしまう |
| ディスク・速度   | 有利                                               | 不利                           |

ワークスペース定義は **`pnpm-workspace.yaml`**（`package.json` の `workspaces` ではない）。

> **pnpm 10 の落とし穴**: 依存パッケージの lifecycle スクリプトが既定で実行されない。`postinstall` でバイナリを取得する類（esbuild など）は `package.json` の `pnpm.onlyBuiltDependencies` に列挙しないと動かない。**「インストールは成功したのに動かない」という形で出る。**

### turbo.json の要点

タスクと、その依存関係を宣言する。理解しておくべき記法はこれ。

```jsonc
{
  "tasks": {
    "build": {
      "dependsOn": ["^build"], // ← この "^" が何を指すかを調べる
      "outputs": [], // ← キャッシュ対象の成果物を書く
    },
    "test": {},
    "lint": {},
  },
}
```

`outputs` の指定を誤ると**キャッシュが効かない、あるいは壊れた成果物を復元する**。ここは調べる価値がある。

### 決定済み: ディレクトリ構成

[ADR 20260812-1313](../adr/20260812-1313-monorepo-directory-layout.md) で確定。

```text
copan/
├── apps/
│   ├── web/          # Next.js       → @repo/web
│   └── api/          # Go            → @repo/api
├── packages/
│   ├── api-spec/     # TypeSpec の定義（手で書く）        → @repo/api-spec
│   └── api-client/   # 生成された TS の型（コミットする） → @repo/api-client
├── package.json          # private + devEngines.packageManager（turbo が要求する）
├── turbo.jsonc
├── pnpm-workspace.yaml   # apps/* と packages/* + saveExact
└── mise.toml             # node 24.19.0 / pnpm 11.21.0
```

ビルド順序は `api-spec → api-client → web`。

> `packages/` の**中身**（生成タスク、出力パス、生成コマンド）は 004 で TypeSpec に触れてから決める。**このチケットでは空のディレクトリと `package.json` だけでよい。**

Go は Go modules で依存を管理するため npm/pnpm の依存解決には乗らないが、タスク（`go build` / `go test`）としては Turborepo から叩ける。その方法は下記に調査済み。

### Go を Turborepo に載せる（調査済み・採否の判断は自分で）

Turborepo には**公式の多言語ガイドがあり、Go が例として使われている**（[Multi-language support](https://turborepo.dev/docs/guides/multi-language)）。

> Turborepo uses package-manager workspaces and `package.json` scripts to discover most packages and tasks. A script can invoke any toolchain, so you can integrate a language without native Turborepo support by **giving each independently cacheable project a package boundary**.

つまり **Turborepo の認識単位は `package.json` の有無**であって、言語ではない。Go のディレクトリに `package.json` を1枚置けば1パッケージとして見える。

```jsonc
// apps/api/package.json — ツールチェーンを叩くだけの薄い殻
{
  "name": "@repo/api",
  "private": true,
  "scripts": {
    "build": "go build -o ??? ???",
    "test": "go test ./...",
    "lint": "go vet ./???",
  },
}
```

```jsonc
// apps/api/turbo.json — パッケージ単位の設定は "extends" で足す
{
  "extends": ["//"],
  "tasks": {
    "build": { "outputs": ["???"] }, // ← Go のビルド成果物のパス
  },
}
```

公式が明示している境界:

> Turborepo does not interpret `go.mod` or Go imports.

Go modules の解決とコンパイルは Go ツールチェーンの責務のまま。**Turborepo が見るのはファイル・`outputs`・`package.json` 上の依存関係だけ。** 学習TODOで整理した「依存解決とタスク実行を分けて考える」が、そのまま公式の設計になっている。

#### キャッシュが二重になる論点

Go は `go build` / `go test` に**コンテンツハッシュベースのキャッシュを標準装備**している（2回目の `go test` は `(cached)` と出る）。Turborepo のキャッシュはこの上に重なる。

|              | Go 標準のキャッシュ | Turborepo のキャッシュ |
| ------------ | ------------------- | ---------------------- |
| ローカル2回目 | 効く                | 効く（重複）           |
| CIのランナー間 | 単体では効かない     | リモートキャッシュで効く |
| 粒度         | Go パッケージ単位    | タスク単位（粗い）      |

ただし CI については **`actions/setup-go` が既定でモジュールキャッシュとビルド成果物をキャッシュする**（`cache` の既定値は `true`、`go.mod` のハッシュがキー）。「CI では Turborepo でないとキャッシュが共有できない」は成立しない。

#### 決定済み: A（公式パターンに従う）

[ADR 20260812-1305](../adr/20260812-1305-go-in-turborepo-workspace.md) で **A に決定**。却下した B / C とその理由は ADR 側に記録してある。

|     | 内容                                     | 結果                             |
| --- | ---------------------------------------- | -------------------------------- |
| A   | 含める + キャッシュも Turborepo に任せる  | **採用**（公式ガイドの形）        |
| B   | 含める + `"cache": false`                 | 却下                             |
| C   | 含めない（Makefile 等で独立）             | 却下                             |

したがってこのチケットでは、**Go のディレクトリにも `package.json` と `turbo.json` を置く**前提で構成を組む。

#### 順序制御の書き方

フロント側の `package.json` に Go パッケージを `devDependencies` として書くと `dependsOn: ["^build"]` に乗る。**これはオーケストレーションのためのメタデータであり、JS から Go を import できるようになるわけではない。**

```jsonc
// apps/web/package.json
{
  "devDependencies": {
    "@repo/api": "workspace:*",
  },
}
```

> **このチケットで `apps/web` に `@repo/api` を書く必要があるかは、自分で判断すること。** Go のビルドを web より先に走らせたい理由が実際にあるか、が判断基準（[ADR 20260812-1313](../adr/20260812-1313-monorepo-directory-layout.md) の「結果として必要になる決定」）。

## 4. 学習TODO

- [x] `dependsOn: ["^build"]` の `^` が何を指すか説明できる
  - [x] 各プロジェクトのpackage.jsonのdependenciesが依存を定義している
    - [x] "workspace:*"とすることで、モノレポ内のパッケージを参照することを示す
  - [x] `^` あり = **依存パッケージの同名タスク**を先に実行する。`^` なし（`"build"`）は同一パッケージ内の別タスクを指す
- [x] Turborepo のキャッシュが何をキーに算出されるか説明できる
  - [x] 入力をハッシュ化して fingerprint を作る。fingerprint が一致するとキャッシュヒットと判定される
  - [x] ハッシュは**グローバルハッシュ**と**タスクハッシュ**の2階層。**どちらか一方でも変わればそのタスクはキャッシュミスになる**（キャッシュは2つ作られるのではなく、ハッシュが2つある）
    - [x] グローバルハッシュの入力（変わると**リポジトリ全体**のタスクがミスする）
      - [x] ルート `turbo.json` とパッケージ `turbo.json` から解決されたタスク定義
      - [x] ワークスペースルートに影響する lockfile の変更
      - [x] ワークスペースルートが使う内部パッケージのソースファイル
      - [x] `globalDependencies` に挙げたファイルの内容
      - [x] `globalEnv` に挙げた環境変数の**値**
      - [x] タスクの実行時挙動を変えるフラグ値、および passthrough 引数
    - [x] タスクハッシュの入力（変わっても**そのパッケージのタスクだけ**がミスする）
      - [x] そのパッケージの Package Configuration（パッケージ内 `turbo.json`）の変更
      - [x] そのパッケージに影響する lockfile の変更
      - [x] そのパッケージの `package.json` の変更
      - [x] ソースファイルの変更（既定は Git 管理下の全ファイル。`inputs` で絞れる）
  - [x] 各パッケージのソースファイルがタスクハッシュ側に分かれているからこそ、**`apps/web` の変更で `apps/api` のキャッシュが飛ばない**。これを全部グローバル側に入れるとモノレポのキャッシュが意味をなさなくなる
- [x] `outputs` を指定し忘れると何が起きるか説明できる
  - [x] ハッシュは通常どおり作られる。**キャッシュに保存されないのはファイル成果物だけ**で、ログは caching が有効なら常に保存される（[Configuration Reference / outputs](https://turborepo.dev/docs/reference/configuration#outputs)）
  - [x] 1回目: タスクは**実行され、`dist/` も生成される**。キャッシュに入るのはログだけ
  - [x] 2回目: ハッシュ一致で**キャッシュヒット → タスクはスキップ**。ログだけ復元され、**`dist/` は復元されない**
    - [x] ローカルでは1回目の `dist/` が残っているため**気づけない**。壊れるのは `dist/` が存在しない環境 — CI の新しいランナー、fresh clone、`rm -rf dist` の後
    - [x] ビルドログが流れて緑で終わるのに成果物が無い。CI ではキャッシュが効いた瞬間にデプロイ対象が空になる
    - [x] 「キャッシュしなくなる（毎回ビルドされて遅いだけ）」ではない。危険度が逆
    - [x] 「成果物が生成されない」でもない。生成はされる。**保存・復元されない**
  - [x] linter のようにファイルを吐かないタスクではこの挙動で正しい。だから `outputs` は optional になっている
- [x] Go のディレクトリを npm ワークスペースに**どこまで**含められるか説明できる
  - [x] **依存解決の対象にはできない。** Go の依存は `go.mod` と module cache が解決し、npm/pnpm は `node_modules` への配置しか行わない。互いのマニフェストを解釈しないため
  - [x] 一方で**タスクの単位としては含められる。** `apps/api/package.json` に `scripts` だけ置けば、Turborepo はそこを1パッケージとして扱い `go build` / `go test` を叩ける
    - [x] その場合 `outputs` に Go のビルド成果物のパスを書く必要がある（上の指定漏れがそのまま当てはまる）
  - [x] つまり「含められない理由」ではなく「**依存解決とタスク実行を分けて考える**」のが正しい整理。実際にこの手段を採るかは「5. 不足情報TODO」で決める
- [x] pnpm の「厳密な依存解決」が何を防ぐのか説明できる
  - [x] package.jsonに書いていない依存関係の使用を防止する
    - [x] node.jsはnode_modules直下を見る。npmの管理方法だと、package.jsonの依存が依存するものをnode_modules直下に並べるため、importできてしまう。これをファントム依存と呼ぶ。
    - [x] pnpmはグローバルストアを作成して、node_modules直下はグローバルストアに対するシンボリックリンクになる。このときリンクさせるのがpackage.jsonにあるものだけ。

## 5. 不足情報TODO

- [x] ~~**G-2 パッケージマネージャを決める**~~ → **pnpm で決定。[ADR 20260812-1309](../adr/20260812-1309-package-manager-pnpm.md)**
- [x] ~~apps/ と packages/ の切り方を決める~~ → **`apps/`（web・api）+ `packages/`（api-spec・api-client）、パッケージ名は `@repo/*` で決定。[ADR 20260812-1313](../adr/20260812-1313-monorepo-directory-layout.md)**
- [x] ~~Node.js のバージョン固定方法~~ → **`mise.toml` に Node と pnpm を一元化（CI も `jdx/mise-action` で同じファイルを読む）、依存は完全固定、CI で `pnpm audit` で決定。[ADR 20260812-1324](../adr/20260812-1324-version-pinning-and-audit.md)**
- [x] ~~Turborepo に Go を含めるか~~ → **A（公式の多言語パターンに従う）で決定。[ADR 20260812-1305](../adr/20260812-1305-go-in-turborepo-workspace.md)**

## 6. 実装ステップ

このチケットは設定ファイル中心でテストを書く対象が薄い。**「壊れていないこと」を確認する手段を先に決める**のが実質的な赤→緑にあたる。

1. **確認手段を決める**: ルートで `turbo run build` と `turbo run test` が成功することを、このチケットの検証コマンドとする
2. **ツールチェーンを揃える**
   - `mise.toml` に `node = "24.19.0"` と `pnpm = "11.21.0"` を書く
   - `mise install` を実行し、`node -v` / `pnpm -v` が一致することを確認する（pnpm はグローバルの 10.13.1 から切り替わる）
   - **legacy の `packageManager` フィールドも Corepack も使わない。** ただし **Turborepo は `package.json` にパッケージマネージャの宣言を要求する**ため、`devEngines.packageManager` を**範囲で**置く（[ADR 20260812-1324](../adr/20260812-1324-version-pinning-and-audit.md)）
3. ワークスペースを初期化し、`apps/` `packages/` を作る
   - `pnpm-workspace.yaml`（ワークスペースの範囲 + `saveExact: true`）/ ルート `package.json`（`private: true`）
   - **pnpm 11 では設定の置き場所が `.npmrc` から `pnpm-workspace.yaml` に移っている。** `.npmrc` に `save-exact=true` と書くと pnpm が自動移行する
4. `turbo.json` にタスクを定義する
5. 各ワークスペースに最小のダミータスクを置き、ルートから流れることを確認する
6. 2回目の実行でキャッシュがヒットすることを確認する（`FULL TURBO` 表示）
7. `pnpm audit --audit-level=high` がローカルで通ることを確認する（CI への組み込みは 006）
8. `.gitignore` が新しい構成と整合しているか確認する

### テスト観点

- ルートからタスクが全ワークスペースに流れるか
- 2回目の実行でキャッシュがヒットするか
- 依存関係のあるタスクが正しい順序で走るか
- 1つのワークスペースを変更したとき、無関係なワークスペースがキャッシュでスキップされるか

## 7. 完了条件

- [ ] `node -v` が `mise.toml` の指定（24.19.0）と一致する
- [ ] `pnpm -v` が `mise.toml` の指定（11.21.0）と一致する
- [ ] ルートで `turbo run build` が成功する（中身が空でもよい）
- [ ] 同じコマンドを2回目に実行するとキャッシュがヒットする（`FULL TURBO`）
- [ ] `pnpm audit --audit-level=high` が終了コード 0 で終わる
- [ ] `.gitignore` が新しいディレクトリ構成と整合している
- [ ] 学習TODOがすべて埋まっている
- [x] ~~G-2（パッケージマネージャ）の ADR が存在する~~ → [20260812-1309](../adr/20260812-1309-package-manager-pnpm.md)

## 8. 振り返り（完了時に本人が記入）

- 詰まった点:
- 分かったこと:
- 見積とのズレと、その原因:
