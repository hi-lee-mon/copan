# Lambda アダプタに aws-lambda-go-api-proxy を使い、ランタイムは `provided.al2023` / arm64 とする

- ステータス: 承認
- 日付: 2026-08-15 19:06
- 関連: [20260812-0712 インフラ構成](./20260812-0712-infrastructure-cloudflare-aws-hybrid.md)、[20260815-1521 Go の内部構成](./20260815-1521-go-layered-architecture.md)、[20260815-1548 ルーターは標準 ServeMux](./20260815-1548-router-stdlib-servemux.md)、[20260812-1324 バージョン固定と audit](./20260812-1324-version-pinning-and-audit.md)、[チケット 003](../tickets/003-go-api-skeleton.md)

## コンテキスト

[20260812-0712](./20260812-0712-infrastructure-cloudflare-aws-hybrid.md) で API の実行環境は **AWS Lambda（VPC外）+ Function URL** に決まっている。

Lambda はポートを listen しない。Runtime API から JSON のイベントを受け取り、JSON を返す実行モデルである。一方、[20260815-1521](./20260815-1521-go-layered-architecture.md) の決定によりアプリ本体は `http.Handler` として書かれている。**この2つを繋ぐ変換役（アダプタ）が必要**になった。

あわせて、ランタイムの種別・バイナリ名・CPU アーキテクチャも決める必要がある。

## 決定

### 1. アダプタは `aws-lambda-go-api-proxy` の `httpadapter` を使う

```go
func main() {
	router := handler.NewRouter()
	adapter := httpadapter.NewV2(router)
	lambda.Start(adapter.ProxyWithContext)
}
```

依存は2つ。**Go modules が `go.mod` / `go.sum` で厳密なバージョンと内容ハッシュを固定する**ため、[20260812-1324](./20260812-1324-version-pinning-and-audit.md) の「依存は完全固定」は追加作業なしに満たされる。

| モジュール | 用途 | 採用時の最新 |
| --- | --- | --- |
| `github.com/aws/aws-lambda-go` | 公式 SDK。`lambda.Start` と各種イベント型 | v1.54.0 |
| `github.com/awslabs/aws-lambda-go-api-proxy` | `http.Handler` を包むアダプタ | v0.16.2 |

### 2. `NewV2`（API Gateway HTTP API 用）を Function URL に流用する

**このライブラリには Function URL 専用のアダプタが無い。** 実際の API は次のとおり。

```go
func NewV2(handler http.Handler) *HandlerAdapterV2
func (h *HandlerAdapterV2) ProxyWithContext(
	ctx context.Context,
	event events.APIGatewayV2HTTPRequest,   // ← LambdaFunctionURLRequest ではない
) (events.APIGatewayV2HTTPResponse, error)
```

受け取る型は `events.LambdaFunctionURLRequest` ではなく **`events.APIGatewayV2HTTPRequest`** である。

これで動くと判断した根拠は、**Function URL のペイロードが API Gateway HTTP API の payload format 2.0 と同一形式**であること。`lambda.Start` は届いた JSON をハンドラの引数型に unmarshal するだけなので、JSON の形が同じなら型名が違っても成立する。Go の JSON デコードは未知のフィールドを無視し、欠けたフィールドをゼロ値にするため、差分があっても壊れない。

**ただしこれは机上の判断であり、実地検証はチケット 007（デプロイ）で行う。**

### 3. ランタイムは `provided.al2023`、バイナリ名は `bootstrap`

Go 専用ランタイム（`go1.x`）は廃止されており、Go は**カスタムランタイム**として動かす。`provided.al2023` は Amazon Linux 2023 ベースのカスタムランタイムで、**デプロイパッケージの直下に `bootstrap` という名前の実行ファイルがあること**を要求する。名前は固定で選択の余地がない。

`.gitignore` には既に `bootstrap` が入っている。

### 4. CPU アーキテクチャは arm64

ビルドは次の形になる。

```bash
GOOS=linux GOARCH=arm64 go build -o bootstrap ./cmd/lambda
```

## 理由

### アダプタを自前で書かなかった理由

変換対象は「メソッド・パス・ヘッダー」だけではない。**ベース64エンコードされたボディ、複数値を持つヘッダー、cookie、クエリ文字列の再組み立て**といった細部があり、いずれも**取りこぼしても手元では気づかず、本番でだけ壊れる**種類のものである。

開発は週5時間・1名。ここは学習対象（設計・インフラ・DB・認証・ログ設計）から外れた、純粋な車輪の再発明にあたる。

### arm64 を選んだ理由

- **Graviton（arm64）のほうが単価が安い。** 同じメモリ・実行時間なら x86_64 より約2割安い
- **開発機が darwin/arm64。** クロスコンパイルで変わるのは `GOOS` だけになり、`GOARCH` は据え置き。手元とアーキテクチャが揃うぶん、アーキ固有の問題が出にくい
- **可逆である。** 変えるときはビルドし直して再デプロイするだけ

### `provided.al2023` を選んだ理由

Go 専用ランタイムが廃止されている以上、カスタムランタイム一択。世代としては `provided.al2` より新しい `provided.al2023` を選ぶ。

## 却下した案

### アダプタを自前で書く

依存が公式 SDK 1つで済み、変換の仕組みが完全に見通せる。学習価値もある。

**却下の理由**: 上記のとおり、細部の取りこぼしが本番でだけ表面化する。この領域は CLAUDE.md が定める学習対象に含まれない。

### AWS Lambda Web Adapter を使う

Lambda のレイヤーとして追加すると、**通常の HTTP サーバーをそのまま Lambda で動かせる**仕組み。採用すれば `cmd/local` と `cmd/lambda` を分ける必要すら無くなり、`ListenAndServe` するバイナリ1つで済む。

**却下の理由**: Lambda レイヤーという追加要素が入り、チケット 007（デプロイ）の構成が変わる。またリクエストごとに**プロセス内で HTTP のラウンドトリップが発生する**ため、レイテンシに不利。[20260812-0712](./20260812-0712-infrastructure-cloudflare-aws-hybrid.md) は DB がシンガポールにある構成でレイテンシを気にしており、余分な往復を足す判断は取りにくい。

なお本案は「Go 以外の言語でも同じ形にできる」ことが最大の利点だが、本プロジェクトは Go 一本なのでその利点が効かない。

## トレードオフとして受け入れること

- **Function URL 専用の型を使えていない。** `APIGatewayV2HTTPRequest` での流用が本当に問題ないかは 007 で検証するまで確定しない。もし差分が出た場合は、自前アダプタか別ライブラリへの切り替えを検討することになる
- **依存が2つ増える。** `pnpm audit` は Go を見ないため、Go 側の脆弱性チェックは別途必要になる（下記）
- **`aws-lambda-go-api-proxy` は v0 系。** メジャーバージョンが 1 に達しておらず、破壊的変更が入りうる
- **ローカルと Lambda で経路が違う。** `cmd/local` は `ServeHTTP` を直接呼び、`cmd/lambda` はアダプタを通す。ハンドラは同一だが、**アダプタの変換部分だけはローカルのテストで検証されない**

## 結果として必要になる決定

- **Go の依存に対する脆弱性チェックをどう回すか。** [20260812-1324](./20260812-1324-version-pinning-and-audit.md) が CI で強制すると決めた `pnpm audit` は Node の依存しか見ない。Go には `govulncheck` があり、CI（チケット 006）に組み込むかを決める
- **`APIGatewayV2HTTPRequest` の流用が成立するかの実地検証**（チケット 007）
- **`bootstrap` をどう zip に固めてデプロイするか**（チケット 007）
- **turbo の `build` タスクで、ローカル用と Lambda 用のどちらを（あるいは両方を）ビルドするか**（チケット 003 のステップ6）
