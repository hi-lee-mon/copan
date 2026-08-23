# OpenAPI から Go を生成するツールに oapi-codegen を採用し、strict-server まで生成させる

- ステータス: 承認
- 日付: 2026-08-23 14:53
- 関連: [20260812-0659 バックエンドを Go とし、型共有を TypeSpec 起点の OpenAPI で行う](./20260812-0659-backend-go-openapi-contract.md)、[20260812-1324 バージョン固定と audit](./20260812-1324-version-pinning-and-audit.md)、[20260815-1420 Go のバージョンとモジュールパス](./20260815-1420-go-version-and-module-path.md)、[20260815-1521 Go の内部構成](./20260815-1521-go-layered-architecture.md)、[20260815-1548 ルーターは標準の ServeMux](./20260815-1548-router-stdlib-servemux.md)、[20260815-2314 ビルド成果物と turbo タスク](./20260815-2314-go-build-output-and-turbo-tasks.md)、[20260815-2315 ルーターの置き場所](./20260815-2315-router-placement.md)、[20260823-1423 OpenAPI の出力先](./20260823-1423-openapi-output-location.md)、[チケット 005](../tickets/005-health-from-generated-types.md)

## コンテキスト

[20260812-0659](./20260812-0659-backend-go-openapi-contract.md) が定めた「TypeSpec → OpenAPI → 両側コード生成」のうち、004 で OpenAPI の生成までが通った。[チケット 005](../tickets/005-health-from-generated-types.md) で**後半（OpenAPI → Go）を繋ぐ**にあたり、次の6つを同時に決める必要があった。

1. どの生成ツールを使うか
2. どこまで生成させるか
3. 生成コードをどこに置くか
4. API 全体で1つになる `StrictServerInterface` を、モジュール分割とどう噛み合わせるか
5. 生成ツール自身のバージョンをどこで固定するか
6. 生成を turbo のどのタスクで回すか

これらは互いに絡む。**2 が 4 の形を決め、1 が 5 の手段を決め、6 は [20260823-1423](./20260823-1423-openapi-output-location.md) が 005 に持ち越した宿題**であるため、1本の ADR にまとめる。

制約は次のとおり。

- **[20260815-1548](./20260815-1548-router-stdlib-servemux.md) が「Gin の採否は生成ツールの選定と併せて再判断する」と保留している。** 本 ADR がその決着にあたる
- **[20260815-2315](./20260815-2315-router-placement.md) が「生成コードが入ったとき `NewRouter()` がどう変わるか」を宿題に残している**
- **[20260815-1521](./20260815-1521-go-layered-architecture.md) の `internal/{module}/` レイヤー分割は決定済み。** 一方、生成コードはどのモジュールにも属さない
- 開発は**週5時間・1名**。[技術選定ガイド](../tech-decision-guide.md)は「過剰な設計を持ち込まない」ことを求めている

## 決定

### 1. 生成ツールは `oapi-codegen` v2.8.0

```sh
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
```

**あわせて [20260815-1548](./20260815-1548-router-stdlib-servemux.md) が保留していた「Gin の採否」を決着させる。Gin は不採用のままとする。** oapi-codegen が標準 `net/http` 向けの生成（`std-http-server`）を持つため、現在の `net/http.ServeMux` をそのまま維持できる。

### 2. 生成範囲は `models` + `std-http-server` + `strict-server`

```yaml
# apps/api/oapi-codegen.yaml
package: gen
output: internal/rest/gen/server.gen.go
generate:
  models: true
  std-http-server: true
  strict-server: true
```

### 3. 生成コードの置き場所は `apps/api/internal/rest/gen/`（`package gen`）

**生成物を使うのは `internal/rest/router.go` であり、使う場所の隣に置く。**

```text
apps/api/internal/
├── rest/
│   ├── router.go                        package rest    ← ルーティングの組み立て
│   ├── server.go                        package rest    ← 集約 struct（下記4）
│   └── gen/
│       └── server.gen.go                package gen     ← 生成物。手で触らない
└── healthz/
    └── interface/rest/handler/
        └── healthz.go                   package handler ← StrictServerInterface の1メソッドを実装
```

**モジュール側（`handler`）が `internal/rest/gen` を import することを明示的に許す。** [20260815-2315](./20260815-2315-router-placement.md) は「モジュール側は `rest` を import しない」と定めているが、**`rest/gen` は `rest` の実装ではなく、契約から生成された共有物**である。この例外を本 ADR で明記する。

### 4. `StrictServerInterface` は `internal/rest/server.go` の集約 struct が満たす。各モジュールは埋め込む

```go
// internal/rest/server.go
package rest

type server struct {
	handler.Healthz // healthz モジュール
	// handler.Post   ← モジュールが増えたら1行足す
	// handler.Shop
}
```

Go の**埋め込み（embedding）**により、埋め込んだ型のメソッドが `server` のメソッドとして昇格する。これによって `server` が `gen.StrictServerInterface` を満たす。

```go
// internal/rest/router.go
func NewRouter() http.Handler {
	si := gen.NewStrictHandler(server{}, nil)
	return gen.HandlerWithOptions(si, gen.StdHTTPServerOptions{})
}
```

### 5. 生成ツールのバージョンは `apps/api/go.mod` の `tool` ディレクティブで固定する

```text
tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
```

実行は `go tool oapi-codegen`。**`mise.toml` には書かない。**

### 6. 生成は各パッケージの `build` スクリプトに含める。`turbo.jsonc` は変更しない

```jsonc
// packages/api-spec/package.json
{ "scripts": { "build": "tsp compile . --emit=@typespec/openapi3" } }

// apps/api/package.json
{
  "scripts": {
    "build": "go tool oapi-codegen -config oapi-codegen.yaml ../../packages/api-spec/tsp-output/@typespec/openapi3/openapi.yaml && mkdir -p dist && GOOS=linux GOARCH=arm64 go build -o dist/bootstrap ./cmd/lambda",
  },
}
```

**これにより [20260823-1423](./20260823-1423-openapi-output-location.md) が 005 に持ち越した宿題が解消する。** 同 ADR の決定3（生成を `build` に載せない）は、本 ADR で置き換えられる。ルートの `gen-oa` スクリプトは、契約だけを回したいときのショートカットとして残してよい。

## 理由

### 1. oapi-codegen — 採用規模が桁違い

今日時点の実測。

| | **oapi-codegen** | ogen |
| --- | --- | --- |
| GitHub スター | **8,530** | 2,126 |
| **pkg.go.dev の被 import 数** | **1,674** | 59 |
| 最終 push | 2026-08-21 | 2026-08-21 |
| 初版 | 2019-02 | 2021-05 |
| 最新版 | v2.8.0（2026-07-17） | v1.24.0 |

**判断材料にしたのは被 import 数である。** スターは「気になった人」の数だが、被 import 数は**公開されている Go のモジュールのうち実際に依存しているものの数**であり、実運用の採用を反映する。28倍の差がある。

これは[技術選定ガイド](../tech-decision-guide.md)の6軸目「**学習の一般性**」に当たる。上位の軸（要件適合・可逆性・開発時間・運用コスト）で両者に決定的な差が無かったため、この軸で決めた。

なお **OpenAPI の外に出れば Protobuf + gRPC のほうが採用規模は大きい**が、それは [20260812-0659](./20260812-0659-backend-go-openapi-contract.md) で「TypeSpec → OpenAPI」と決めた時点で選択肢の外にある。本 ADR はその決定を前提とする。

### 2. strict-server — 契約違反がコンパイルエラーになる範囲が最も広い

生成範囲を広げるほど、手書きが減り、契約違反が実行時ではなく**ビルド時**に出る。

| 生成範囲 | 手書きに残るもの |
| --- | --- |
| `models` のみ | パス・HTTP メソッド・ステータス・Content-Type・JSON エンコード |
| `+ std-http-server` | ステータス・Content-Type・JSON エンコード |
| `+ strict-server` | **ハンドラの中身だけ** |

`strict-server` を選ぶと、ハンドラのシグネチャがこうなる（実測）。

```go
HealthCheck(ctx context.Context, request gen.HealthCheckRequestObject) (gen.HealthCheckResponseObject, error)
```

**契約に無いレスポンスを返そうとするとコンパイルが通らない。** 200 と `Content-Type: application/json` は生成コードが書き込む。CLAUDE.md の「API 契約の単一情報源は TypeSpec」を、最も強く担保する形である。

`models` のみを選ぶと、`main.tsp` のパスを変えても Go 側は壊れずに通ってしまう。**契約とコードが二重管理になる**ため、契約駆動を採る意味が大きく削れる。

### 3. `internal/rest/gen/` — 使う場所の隣に置く

生成物を import するのは `internal/rest`（ルーティングの組み立て）と各モジュールの `handler` である。ルーティングの組み立てが主たる利用者であり、その真下に置くのが最も距離が近い。

`internal/gen/` や `internal/openapi/` に置くと `internal/` 直下の要素が増える。[20260815-1521](./20260815-1521-go-layered-architecture.md) は `internal/{module}/` を規則とし、`internal/rest` を唯一の例外として認めている。**例外を2つに増やすより、既にある例外の中に収めるほうが規則が保たれる。**

### 4. 埋め込み — モジュールが増えたときの追加が1行で済む

`StrictServerInterface` は API 全体で1つになるため、**どこかで全モジュールのメソッドを1つの型に集める**必要がある。

- **モジュールの struct が直接満たす形**は、`/healthz` 1本の今なら成立するが、`post` が増えた時点で `healthz` の struct に `CreatePost` を書くことになる。[20260815-1521](./20260815-1521-go-layered-architecture.md) のモジュール凝集に正面から反する
- **委譲メソッドを手書きする形**は、何がどこへ行くかがコードに見えるが、エンドポイントが増えるたびに定型のメソッドが1つ増える。週5時間の制約に対して手数が重い
- **埋め込み**は、モジュールが増えても `server` に1行足すだけで済む。加えて **`server.go` を見れば「どのモジュールが API を構成しているか」が一覧になる**

[20260815-2315](./20260815-2315-router-placement.md) が「依存の向きは `rest` → 各モジュールの一方通行」と定めた向きも、そのまま維持される。

### 5. `tool` ディレクティブ — Go の依存は Go の場所で固定する

- **Go 1.24（2025年2月）から入った公式の場所**であり、oapi-codegen の README もこの形を案内している
- **`go.sum` によるハッシュ検証がかかる。** [20260812-1324](./20260812-1324-version-pinning-and-audit.md) が求める「依存の完全固定」を、追加の仕組みなしで満たす
- **既存の形と揃う。** `aws-lambda-go` などのライブラリは既に `go.mod` で固定されている。生成ツールはツールチェーン（`mise.toml` の担当）ではなく、**このモジュールのビルドに使う依存**であるため、`go.mod` 側が自然

`mise.toml` の `go:` バックエンドは「バージョンは `mise.toml` に集約する」という文言には直接沿うが、裏で `go install` が走るだけで `go.sum` による検証がかからない。**[20260812-1324](./20260812-1324-version-pinning-and-audit.md) の目的（再現性と検証）に照らすと、文言より `tool` ディレクティブのほうが適合する。**

### 6. `build` に含める — 追加の設定がゼロで、順序は既に保証されている

`@repo/api` は `@repo/api-spec` を `devDependencies` に持つため、`dependsOn: ["^build"]` によって **`@repo/api-spec` の `build` が先に走る**（002 で確認済み）。生成を `build` スクリプトの先頭に足すだけで、

```text
main.tsp を編集 → api-spec の build（tsp compile）→ api の build（oapi-codegen → go build）
```

という順序が自動的に成立する。**`turbo.jsonc` に1行も足さずに済む。**

これにより、[20260823-1423](./20260823-1423-openapi-output-location.md) が挙げていた最大の実害——**`main.tsp` を直して生成を忘れると、古い `openapi.yaml` のまま静かに通る**——が構造的に消える。turbo の入力ハッシュが変わり、キャッシュミスして生成が走るためである。

なお **004 が整理した「`outputs` に載らないとキャッシュから復元されない」という害は、ここでは発生しない。** `openapi.yaml` も `server.gen.go` もコミットされており、復元すべき対象が常に git に在るためである（[20260823-1423](./20260823-1423-openapi-output-location.md) の同項参照）。したがって `turbo.jsonc` の `outputs` に生成物のパスを足す必要も無い。

## 却下した案

### ogen を使う

生成コードに `interface{}` とリフレクションを使わず、省略可能な項目を `OptString` のような専用型で表す（ポインタを使わないためヒープへの負荷が小さい）。**enum の値を実際に検証する**という、oapi-codegen に無い利点もある。

**却下の理由**: 被 import 数が 59 対 1,674。技術的な優位はあるが、上位の軸で決定的な差が無い以上、「学習の一般性」で 28 倍の差を覆すだけの重みは無い。**enum 検証の欠落は実害になりうる**が、それが効いてくるのはリクエストボディを受ける投稿機能からであり、そのときに改めて扱う（下記「結果として必要になる決定」）。

### 生成範囲を `models` のみ、または `std-http-server` までにする

生成コードの量が減り、`net/http` の語彙がそのまま見える。

**却下の理由**: `models` のみだと**パスが `main.tsp` と `router.go` の二重管理**になり、契約駆動を採った意味が最も大きく削れる。`std-http-server` までだと、ステータスコードと Content-Type が型で守られない。[20260815-2315](./20260815-2315-router-placement.md) が「ルーティングが生成物に置き換わる」ことを前提に `internal/rest/router.go` を切り出しており、その前提とも合わない。

### 生成コードを `internal/gen/` または `internal/openapi/` に置く

`internal/` 直下に置けば「特定の層のものではない」ことが構造で示せる。`internal/openapi/` は名前で出所が分かる利点もある。

**却下の理由**: [20260815-1521](./20260815-1521-go-layered-architecture.md) の `internal/{module}/` という規則の例外が2つに増える。既にある例外（`internal/rest`）の中に収まるなら、そちらのほうが規則が保たれる。

### モジュールの struct が直接 `StrictServerInterface` を満たす / 委譲メソッドを手書きする

前者は今なら最小の手数、後者は経路がコードに見える。

**却下の理由**: 前者は `post` が増えた時点で `healthz` の struct に無関係なメソッドを書くことになり、モジュール凝集に反する。後者はエンドポイントごとに定型のメソッドが増え、週5時間の制約に対して手数が重い。**どちらも「今は同じ、増えたときに差が出る」という構造であり、今のうちに埋め込みを選んでおくコストはゼロである。**

### `mise.toml` の `go:` バックエンドでバージョンを固定する

「バージョンは `mise.toml` に集約する」という [20260812-1324](./20260812-1324-version-pinning-and-audit.md) の文言に直接沿う。`apps/api/go.mod` が生成ツールの依存で汚れない。

**却下の理由**: 裏で `go install` が走るだけで、`go.sum` によるハッシュ検証がかからない。同 ADR の**目的**（再現性と検証）に照らすと `tool` ディレクティブのほうが適合する。加えて生成ツールはツールチェーンではなくビルド依存であり、`mise.toml` の担当範囲（Node / pnpm / Go 本体）とは性質が違う。

### `generate` タスクを turbo に新設する

`build` と `test` の両方が `generate` を待つ形にすれば、**テスト時も必ず最新の生成コードになる**。

**却下の理由**: `turbo.jsonc` に3タスク分の依存関係を書くことになり、得られるのは「`turbo run test` 単独でも生成が走る」ことだけである。**生成コードはコミットされているため、テストが対象を見失うことはない**（古いものを見る可能性が残るだけで、これは 006 の CI が塞ぐ）。過剰な設計を持ち込まない方針に照らして、いまは足さない。

### Gin に寄せる

[20260815-1548](./20260815-1548-router-stdlib-servemux.md) が「生成ツールが前提とするルーターに合わせて再判断する」と保留していた。

**却下の理由**: oapi-codegen は `gin-server` も生成できるため技術的には可能だが、同 ADR が挙げた却下理由（バリデーションを Gin に持たせると契約が二重管理になる／ログ・リカバリ・CORS は他の層と役割が重なる）は**何一つ変わっていない**。むしろ `strict-server` を選んだことで、バリデーションとレスポンス生成が生成コード側に寄り、**Gin の残存価値がさらに減った**。

## トレードオフとして受け入れること

- **`turbo run test` は生成を待たない。** `main.tsp` を直した直後に `test` だけを流すと、コミット済みの古い生成コードに対してテストが走る。`build` を流すか、006 の CI が塞ぐまでは人間の責任
- **`apps/api/go.mod` に間接依存が 17 行増える**（実測）。実行時ではなくビルド時だけの依存だが、`go.mod` の見た目は重くなる。`pnpm audit` の対象外でもあるため、Go 側の脆弱性確認は別途必要になる
- **モジュールが `internal/rest/gen` を import する。** [20260815-2315](./20260815-2315-router-placement.md) の「モジュール側は `rest` を import しない」が、ディレクトリの見た目上は崩れる。循環は起きない（`gen` はこちらのコードを import しない）が、読み手が規則の例外を1つ覚える必要がある
- **enum の値が検証されない。** `status: "OK"` 以外の値を入れてもコンパイルは通り、実行時にも弾かれない。生成される `Valid()` メソッドを自分で呼ばない限り効かない
- **埋め込みは同名メソッドで曖昧になる。** 2つのモジュールが同じ名前のメソッドを持つと、`server` の埋め込みが曖昧になりコンパイルエラーになる。`operationId` が API 全体で一意である限り起きないが、`interface` 名を揃えるなどの規律が要る
- **ハンドラが `http.ResponseWriter` を直接触らない。** レスポンスヘッダを個別に足したくなったとき、`strict-server` の枠の外に出る手数が発生する
- **生成物のパスが長い。** `../../packages/api-spec/tsp-output/@typespec/openapi3/openapi.yaml` を `apps/api/package.json` に書くことになる（[20260823-1423](./20260823-1423-openapi-output-location.md) のトレードオフ）

## 結果として必要になる決定

- **リクエストボディのバリデーションをどこで行うか。** oapi-codegen は enum を検証しない。投稿機能で入力を受けるようになった時点で、生成コードの `Valid()` を使うか、別の仕組みを足すかを決める（v0.1 の投稿機能の前）
- **ミドルウェア（ログ・パニックリカバリ）をどこに挟むか。** `gen.StdHTTPServerOptions.Middlewares` と `gen.NewStrictHandler` の第2引数、どちらにも挟める。[20260815-2315](./20260815-2315-router-placement.md) が残した宿題の続き
- **エラーレスポンス（4xx / 5xx）の契約をどう書くか。** `/healthz` に失敗系が無いため未着手。エラー表現が決まると `strict-server` の戻り値の型が増える
- **TypeScript 側（`@repo/api-client`）の生成ツール。** フロントを書く段階で決める
- **CI で再生成して差分が出たら失敗させる仕組み**（チケット 006）
