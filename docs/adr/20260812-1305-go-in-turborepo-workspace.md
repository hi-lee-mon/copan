# Go を Turborepo 公式の多言語パターンでワークスペースに含める

- ステータス: 承認
- 日付: 2026-08-12 13:05
- 関連: [20260811-2015 モノレポ構成に Turborepo を採用する](./20260811-2015-monorepo-turborepo.md)、[20260812-0659 バックエンドを Go とする](./20260812-0659-backend-go-openapi-contract.md)、[チケット 002](../tickets/002-monorepo-scaffold.md)

## コンテキスト

[20260811-2015](./20260811-2015-monorepo-turborepo.md) で Turborepo を採用したが、**Go のディレクトリをどう扱うかは未決**だった（同ADR「結果として必要になる決定」のディレクトリ構成）。

Go の依存は `go.mod` と module cache が解決するため、npm/pnpm のワークスペースの依存解決には乗らない。一方でルートから `turbo run build` / `turbo run test` を流したい。この2つをどう両立させるかが論点だった。

調査の結果、**Turborepo には公式の多言語ガイドがあり、Go がその例として使われている**ことが分かった（[Multi-language support](https://turborepo.dev/docs/guides/multi-language)）。

> Turborepo uses package-manager workspaces and `package.json` scripts to discover most packages and tasks. A script can invoke any toolchain, so you can integrate a language without native Turborepo support by giving each independently cacheable project a package boundary.

## 決定

**Turborepo 公式の多言語パターンに従い、Go のディレクトリに `package.json` を置いてワークスペースに含める。キャッシュも Turborepo に任せる。**

具体的には次の3点。

1. ワークスペース定義に Go のディレクトリを含める
2. Go のディレクトリに `package.json` を置き、`scripts` から `go build` / `go test` / `go vet` を叩く
3. パッケージ単位の `turbo.json` を `"extends": ["//"]` で置き、`build` の `outputs` に Go のビルド成果物を指定する

**公式ガイドから逸脱する独自の工夫を持ち込まない。** これが本ADRの主眼。

### 責務の境界

公式が明示している境界を、そのまま本プロジェクトの前提とする。

> Turborepo does not interpret `go.mod` or Go imports.

- **Turborepo が見るもの** — ファイル、`outputs`、`package.json` 上の依存関係
- **Go ツールチェーンが見るもの** — Go modules の解決、コンパイル

つまり「依存解決」と「タスク実行」を分けて考える。Turborepo は後者だけを担当する。

### 順序制御の方法

フロント側の `package.json` に Go パッケージを `devDependencies` として書くことで、`dependsOn: ["^build"]` の順序制御に乗る。公式はこれを**オーケストレーションのためのメタデータ**と位置づけている。

> This dependency is orchestration metadata for Turborepo and the package manager; it does not make the Go module importable from JavaScript.

**JavaScript から Go を import できるようになるわけではない。** 後から誤解しやすい点なので明記しておく。

## 理由

- **公式パターンが存在する以上、それを正とする。** 独自解は、公式が将来変わったときに追従できなくなる。週5時間・1名という制約に対し、自前の工夫を保守し続けるコストは割に合わない
- `package.json` を置く方法は「回避策」ではなく**公式の推奨手段**だと確認できた。異物感を理由に避ける根拠がなくなった
- タスクの依存グラフが `turbo.json` 1ファイルに集約される。TypeSpec の生成 → 両側のビルド、という順序を宣言で担保できる。[CLAUDE.md の制約](../../CLAUDE.md)「生成物はコミットし、CI で再生成して差分が出たら失敗させる」を守るには、この順序保証が要る
- 変更されていない側をスキップできる。フロントだけの変更で Go のテストが走らない

## 却下した案

### B: ワークスペースに含めるが `"cache": false` にする

順序制御だけ Turborepo に任せ、キャッシュは Go 標準のものに委ねる案。Go は `go build` / `go test` にコンテンツハッシュベースのキャッシュを標準装備しており、Turborepo のキャッシュと二重になるため。

**却下の理由**: 公式ガイドは `outputs` を指定する形（＝キャッシュ有効）を示しており、`cache: false` は「長時間走る開発タスク」や「実行グラフに含まれたら必ず走らせたいタスク」向けの設定とされている。ビルドはそのどちらでもない。二重になること自体の実害は「キャッシュが余分に保存される」だけで、公式から外れる代償に見合わない。

### C: ワークスペースに含めず、Makefile 等で独立させる

Go のディレクトリに JS のマニフェストを置かずに済み、`cd apps/api && go test ./...` という素直な操作が残る。

**却下の理由**: 生成 → ビルドの順序を `turbo.json` の外で担保する責任が生じる。順序の定義が2箇所（turbo.json と Makefile）に分散し、片方だけ更新して壊れる経路ができる。**Turborepo に Go を載せたい動機は、キャッシュではなく順序制御にある**という整理からすると、この案は動機そのものを捨てることになる。

### 補足: CI のキャッシュ共有は判断材料にならなかった

検討の途中で「CI のランナー間で Go のビルドキャッシュを共有するには Turborepo のリモートキャッシュが要る」と考えたが、**誤りだった**。`actions/setup-go` は `cache` の既定値が `true` で、`go.mod` のハッシュをキーにモジュールキャッシュとビルド成果物をキャッシュする。この点は A / B / C の優劣に影響しない。

## トレードオフとして受け入れること

- **キャッシュが二重になる。** ローカルでは Go 標準のキャッシュが先に効くため、Turborepo 側の恩恵は薄い
- **キャッシュの粒度が粗い。** Go 標準はパッケージ単位、Turborepo はタスク単位。`go test` の再実行判定は Go 側のほうが正確
- **Go のディレクトリに JS のマニフェストが同居する。** Go だけを触る作業でも `package.json` の存在を意識することになる
- **二重の入口ができる。** ローカルでは `cd apps/api && go test ./...` を直接叩くほうが速い場面が残る。禁止はしない
- `go test` はファイルを吐かないため `outputs` に書くものがなく、キャッシュされるのはログのみになる（これは正しい挙動）

## 結果として必要になる決定

- **G-2 パッケージマネージャ** — ワークスペース定義の書き方が変わる（pnpm は `pnpm-workspace.yaml`、npm/yarn/bun は `package.json` の `workspaces`）。本ADRはどちらでも成立する
- Go のディレクトリを `apps/api` に置くか `services/api` に置くか（公式の例は `services/`）
- Go のビルド成果物の出力先と、`outputs` に書くパス。Lambda 向けのバイナリ名の制約と併せて決める
- TypeSpec の生成タスクを、どのパッケージのどのタスク名で定義するか（チケット 004 で確定）
