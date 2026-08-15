# ADR（Architecture Decision Record）

**意思決定とその理由を記録する場所。**

半年後に「なぜこうしたんだっけ」と必ずなる。特に**却下した案とその理由**を残しておかないと、同じ議論を何度も繰り返すことになる。

## 書き方のルール

- ファイル名は `<yyyymmdd-hhmm>-<内容がわかる名前>.md`
- 決定を変更する場合、**過去のADRを書き換えず、新しいADRを作って古いものから参照する**
- 最低限、以下を含める

```markdown
# <決定の内容を一文で>

- ステータス: 承認 / 更新済み / 却下
- 日付: yyyy-mm-dd hh:mm
- 関連: 他のADRへのリンク

## コンテキスト
何が問題で、どういう制約があったか

## 決定
何を決めたか

## 理由
なぜそう決めたか

## 却下した案
検討したが選ばなかったもの と その理由

## トレードオフとして受け入れること
この決定によって諦めたもの

## 結果として必要になる決定
この決定から派生して、次に決めるべきこと
```

## 一覧

| 日付 | 決定 | ステータス |
| --- | --- | --- |
| [2026-08-11 20:03](./20260811-2003-product-name.md) | プロダクト名を「コパン（COPAN）」とする | 承認 |
| [2026-08-11 20:15](./20260811-2015-monorepo-turborepo.md) | モノレポ構成に Turborepo を採用する | 承認 |
| [2026-08-12 06:59](./20260812-0659-backend-go-openapi-contract.md) | バックエンドを Go とし、型共有を TypeSpec 起点の OpenAPI で行う | 承認 |
| [2026-08-12 07:12](./20260812-0712-infrastructure-cloudflare-aws-hybrid.md) | インフラを Cloudflare（配信層）＋ AWS Lambda（計算層）で構成する | 承認（データ層のみ下記で更新） |
| [2026-08-12 08:05](./20260812-0805-database-neon-then-planetscale.md) | DBは Neon で開始し、v1.0公開前に東京リージョンのPostgresへ移行する | 承認 |
| [2026-08-12 09:05](./20260812-0905-development-workflow.md) | 実装は開発者本人が行い、Claude Code はチケット作成と支援に徹する | 承認 |
| [2026-08-12 13:05](./20260812-1305-go-in-turborepo-workspace.md) | Go を Turborepo 公式の多言語パターンでワークスペースに含める | 承認（パッケージ単位の `turbo.json` のみ下記で更新） |
| [2026-08-12 13:09](./20260812-1309-package-manager-pnpm.md) | パッケージマネージャに pnpm を採用する | 承認 |
| [2026-08-12 13:13](./20260812-1313-monorepo-directory-layout.md) | ディレクトリ構成を `apps/` + `packages/` の2系統とし、パッケージ名を `@repo/*` で揃える | 承認 |
| [2026-08-12 13:24](./20260812-1324-version-pinning-and-audit.md) | Node.js と pnpm のバージョンを `mise.toml` に集約し、依存を完全固定して CI で `pnpm audit` を強制する | 承認 |
| [2026-08-14 18:26](./20260814-1826-defer-trademark-research.md) | 商標の実査を繰り延べ、名称は「仮の COPAN」として実装を先行させる | 承認 |
| [2026-08-15 14:20](./20260815-1420-go-version-and-module-path.md) | Go を `mise.toml` で 1.26.6 に固定し、モジュールパスを `github.com/hi-lee-mon/copan/apps/api` とする | 承認 |
| [2026-08-15 15:21](./20260815-1521-go-layered-architecture.md) | Go の内部構成をモジュール単位のレイヤー構造とし、層は中身ができた時点で追加する | 承認 |
| [2026-08-15 15:48](./20260815-1548-router-stdlib-servemux.md) | HTTP ルーターは標準の `net/http.ServeMux` とし、Gin の採否は 004 と併せて判断する | 承認 |
| [2026-08-15 19:06](./20260815-1906-lambda-adapter-and-runtime.md) | Lambda アダプタに aws-lambda-go-api-proxy を使い、ランタイムは `provided.al2023` / arm64 とする | 承認 |
| [2026-08-15 23:14](./20260815-2314-go-build-output-and-turbo-tasks.md) | Go のビルド成果物を `apps/api/dist/bootstrap` に出し、パッケージ単位の `turbo.json` を作らない | 承認 |
| [2026-08-15 23:15](./20260815-2315-router-placement.md) | HTTP ルーターの組み立てを `internal/rest` に置き、各モジュールはハンドラ関数だけを公開する | 承認 |

## 決定の連鎖

主要な決定は互いに依存している。**上流の決定を覆すと、下流がすべて連動して変わる。**

```text
プロダクト名: コパン
      │
バックエンド言語: Go  ←──────── ここが最も影響が大きい
      │
      ├─→ Cloudflare Workers が使えない
      │        └─→ APIの実行環境: AWS Lambda
      │                 └─→ VPCを避けるため、DBはAWSの外
      │                          └─→ 外部のPostgreSQL（Neon → 東京へ移行）
      │
      └─→ フロントとの型共有が切れる
               └─→ TypeSpec → OpenAPI → 両側コード生成
```

**もし Go をやめて TypeScript にすると、「Cloudflare完結（Workers + D1）」が最良の構成になる**（月$5前後、レイテンシも最良）。
その検証結果は [20260812-0805 の却下した案](./20260812-0805-database-neon-then-planetscale.md#却下した案) に記録されている。
