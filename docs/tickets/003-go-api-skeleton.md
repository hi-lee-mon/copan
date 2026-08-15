# 003: Go の API 雛形を作り、ローカルで叩ける状態にする

- ステータス: **完了**（2026-08-15）
- 見積: 3h / 実績: 3h
- 依存: **002（完了）** — `apps/api` のパッケージ境界と turbo のタスク定義がある前提
- 関連: [ADR 20260812-0712 インフラ構成](../adr/20260812-0712-infrastructure-cloudflare-aws-hybrid.md)、[ADR 20260812-0659 Go採用](../adr/20260812-0659-backend-go-openapi-contract.md)、[ADR 20260812-1305 Go の置き方](../adr/20260812-1305-go-in-turborepo-workspace.md)、[ADR 20260812-1324 バージョン固定](../adr/20260812-1324-version-pinning-and-audit.md)、[ADR 20260814-1826 商標の繰り延べ](../adr/20260814-1826-defer-trademark-research.md)
- 本チケットで生まれた ADR: [20260815-1420 バージョンとモジュールパス](../adr/20260815-1420-go-version-and-module-path.md)、[20260815-1521 内部構成](../adr/20260815-1521-go-layered-architecture.md)、[20260815-1548 ルーター](../adr/20260815-1548-router-stdlib-servemux.md)、[20260815-1906 Lambda アダプタとランタイム](../adr/20260815-1906-lambda-adapter-and-runtime.md)、[20260815-2314 ビルド成果物と turbo タスク](../adr/20260815-2314-go-build-output-and-turbo-tasks.md)、[20260815-2315 ルーターの置き場所](../adr/20260815-2315-router-placement.md)

## 1. 目的

Go の API を **ローカルで HTTP サーバーとして起動して叩ける**状態にし、同時に **Lambda 用のエントリポイントもビルドできる**状態にする。あわせて、002 で仮置きした `apps/api` のダミータスクを**実際の Go のビルド・テストに差し替える**。

## 2. 背景

[ADR 20260812-0712](../adr/20260812-0712-infrastructure-cloudflare-aws-hybrid.md) で、API の実行環境は **AWS Lambda（VPC外）+ Function URL** に決まっている。

しかし **デプロイより先にローカル実行を確立する**。プロジェクトの方針は TDD であり、テストが Lambda 上でしか回らない状態では TDD のサイクルが成立しないため。

このチケットで置くエンドポイントは **仮の `/ping`** とする。本命の `/health` は 004 で TypeSpec の契約として定義し、005 で生成された型から実装する。**手書きしたものが契約駆動に置き換わる過程**を、そのまま体験の対象にする。

## 3. 事前調査

### 002 の結果として、すでにあるもの

`apps/api` には **`package.json` が1枚だけ**置かれている。中身は turbo にパッケージとして認識させるための殻で、**タスクはダミー**（`dist/` にファイルを吐くだけ）。

```jsonc
// apps/api/package.json（現状）
{
  "name": "@repo/api",
  "private": true,
  "scripts": {
    "build": "mkdir -p dist && echo built > dist/out.txt", // ← 差し替える
    "test": "echo test @repo/api", // ← 差し替える
  },
  "devDependencies": {
    "@repo/api-spec": "workspace:*", // ← 004 で TypeSpec が入ると効いてくる順序制御
  },
}
```

**`apps/api/turbo.json` はまだ無い。** ルートの `turbo.jsonc` の定義（`outputs: ["dist/**"]`）が全パッケージに一律で適用されている状態。

Go のコード（`go.mod` 含む）は1行も無い。

### Lambda の実行モデル

Lambda は常駐サーバーではない。理解しておくべきは次の点。

- **コールドスタート**: 実行環境が新規に作られ、初期化コードが走る
- **ウォームスタート**: 直前の実行環境が再利用され、初期化コードは走らない
- したがって **`main()` や初期化処理はリクエストごとには走らない**。ここに何を置くかが、後で DB 接続の使い回し（ADR 20260812-0712 のレイテンシ緩和策）に直結する

### aws-lambda-go の骨組み

```go
func main() {
    lambda.Start(handler)
}

func handler(
    ctx context.Context,
    req events.LambdaFunctionURLRequest,
) (events.LambdaFunctionURLResponse, error) {
    // TODO: ここを書く
}
```

→ 調べること: なぜ `ctx` が第1引数なのか。ハンドラが取れるシグネチャは何通りあるのか。

### 最大の設計論点 — Lambda に密結合させない

ハンドラを上の形で直接書くと、次の3つが同時に起きる。

1. ローカルで起動できない（Lambda のイベント型を組み立てないと呼べない）
2. テストが書きにくい（`events.LambdaFunctionURLRequest` を毎回組み立てることになる）
3. Lambda から離れられない（[ADR 20260812-0712](../adr/20260812-0712-infrastructure-cloudflare-aws-hybrid.md) は Lambda を選んだが、可逆性は保っておきたい）

対策として **アプリ本体を標準の `net/http.Handler` として書き、Lambda 側は薄いアダプタにする**構成がある。

```text
apps/api/
├── internal/         ← アプリ本体（http.Handler として書く）
├── cmd/local/        ← ローカル用。http.ListenAndServe で起動
└── cmd/lambda/       ← Lambda用。アダプタで http.Handler を包んで lambda.Start
```

こうすると、テストは `net/http/httptest` で普通に書ける。

```go
// テストの骨組み（中身は自分で書く）
func TestPing(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/ping", nil)
    rec := httptest.NewRecorder()

    // TODO: ハンドラを呼ぶ

    // TODO: 検証する
}
```

→ 調べること: `http.Handler` インターフェースが要求するメソッドは何か。`httptest.NewRecorder()` は何を記録するのか。

### Go のディレクトリ規約

- `cmd/<name>/` — 実行可能バイナリのエントリポイント。名前がバイナリ名になる
- `internal/` — **同一モジュールの外からインポートできない**、Go コンパイラが強制する仕組み

→ 調べること: `internal/` の制約は具体的にどのパスから効くのか。

### Turborepo から Go を叩く（ADR 1305 の実装）

[ADR 20260812-1305](../adr/20260812-1305-go-in-turborepo-workspace.md) で **A（Turborepo 公式の多言語パターン）** に決定済み。`package.json` の `scripts` を Go のコマンドに差し替えるだけでよい。

```jsonc
// apps/api/package.json（差し替え後の形）
{
  "scripts": {
    "build": "go build -o ??? ???",
    "test": "go test ./...",
  },
}
```

**ここで 002 で体験した罠がそのまま当てはまる。**

ルートの `turbo.jsonc` は `outputs: ["dist/**"]` を全パッケージに適用している。Go のビルド成果物を **`dist/` 以外の場所に出すなら、`outputs` の指定が実態と食い違う**。食い違うとどうなるかは 002 の学習TODOで確認済み——**キャッシュヒット時にバイナリが復元されず、緑色で成功したのに成果物が無い**という形で表面化する。

対処は2通り。**どちらを採るかはこのチケットの判断。**

|     | 内容                                                                                |
| --- | ----------------------------------------------------------------------------------- |
| A   | Go の成果物も `dist/` に出し、ルートの定義をそのまま使う                            |
| B   | `apps/api/turbo.json` を作り、`extends` でこのパッケージだけ `outputs` を上書きする |

```jsonc
// apps/api/turbo.json（B を採る場合）
{
  "extends": ["//"], // ← ルートの定義を継承し、必要な部分だけ上書きする
  "tasks": {
    "build": { "outputs": ["???"] },
  },
}
```

> ルートは `turbo.jsonc`（コメント可）にした。パッケージ側の拡張子を揃えるかも判断すること。turbo は両方読む。

もう一点、**`test` タスクのキャッシュ**に論点がある。Go は `go test` に自前のキャッシュを持ち、2回目は `(cached)` と表示される。turbo のキャッシュはその上に重なる（ADR 1305 の「キャッシュが二重になる論点」）。**turbo 側で `test` に何を指定するか**（`outputs` は要るか、`cache` を切るか）を決める。

### Lambda 向けのビルド

`.gitignore` には既に `bootstrap` が入っている。これは Lambda のカスタムランタイムが**エントリポイントのバイナリ名として固定で要求する名前**であることに由来する。

→ 調べること: `provided.al2023` ランタイムが要求するバイナリ名と配置。クロスコンパイルで指定すべき環境変数（開発機は darwin/arm64、Lambda は linux）。

## 4. 学習TODO

- [x] Lambda のコールドスタートとウォームスタートで、何が再利用されるか説明できる
  - **コールドスタート**: 実行環境（コンテナ）が新規に作られ、バイナリがロードされ、**`main()` が走る**
  - **ウォームスタート**: 直前の実行環境がそのまま生きており、**`main()` は走らない**。`lambda.Start()` に渡したハンドラだけが、既に立ち上がっているプロセスの中で呼ばれる
  - 再利用されるのは**プロセスのメモリ上の状態**（グローバル変数、接続オブジェクト等）。したがって `main()` の中で1回だけ張った DB 接続を、ウォームの間は使い回せる（[ADR 20260812-0712](../adr/20260812-0712-infrastructure-cloudflare-aws-hybrid.md) のレイテンシ緩和策）
- [x] `http.Handler` を挟むことで、何が可逆になるのかを説明できる
  - 可逆になるのは **実行基盤（どこで動かすか）**。`rest.NewRouter()` が標準の `http.Handler` を返すため、**アプリ本体は自分が Lambda で動いていることを知らない**
  - Lambda をやめて EC2 / Cloud Run / 自前のサーバーに移す場合、書き換えるのは `cmd/lambda/main.go` の数行だけで、`internal/` は一切触らない
- [x] `internal/` パッケージの制約を説明できる
  - `internal/` ディレクトリの**親**をルートとして、そのルート配下以外からはパッケージを import できない（基準は `internal/` 自身ではなく、その親）
- [x] `httptest` を使ったテストが、Lambda のイベント型を直接扱うテストより優れている理由を説明できる
  - 比較対象は「**ローカルで** `events.LambdaFunctionURLRequest` を組み立てるテスト」。どちらもローカルで走るため、実行速度や実行費用の差は無い
  - イベント型を使うと **テストが Lambda に依存する**。実行基盤を変えた瞬間にテストを書き直すことになり、`internal/` を基盤から切り離した意味が失われる
  - `httptest.NewRequest(http.MethodGet, "/ping", nil)` は HTTP の語彙だけで書けるため、基盤を変えてもそのまま残る（**上記「可逆になるもの」と同じ論点の裏返し**）
- [x] Go のクロスコンパイルで Lambda 向けバイナリを作るとき、何を指定する必要があるか説明できる
  - `GOOS=linux GOARCH=arm64`（開発機は darwin/arm64、Lambda は linux/arm64）
- [x] **Go 自身のビルド／テストキャッシュと、Turborepo のキャッシュが、それぞれ何を単位にしているか説明できる**
  - **単位**: Go は**パッケージ単位**（`internal/rest` と `internal/health/...` を別々に判定する）、Turborepo は**タスク単位**（`@repo/api` の `test` 全体で1つ）
  - **Go のハッシュ材料**: そのパッケージと依存パッケージのソースの内容（タイムスタンプではなく中身）、コンパイラのバージョン・ビルドフラグ・`GOOS` / `GOARCH`、`go test` の引数、実行中に読んだ環境変数とファイル
  - **Turborepo のハッシュ材料**: パッケージ内の Git 管理下のファイル、**そのパッケージが依存しているパッケージ**のハッシュ（`dependsOn: ["^build"]` の `^` は依存先を先に走らせる印。`@repo/api` のビルド時に `@repo/api-spec` が先に走るのがその実例）、宣言した環境変数、`package.json` の `scripts` の内容
  - **二層になる**: turbo が「コマンドを起動するか」、Go が「起動された中で何を実際に走らせるか」を決める。`turbo run test` で `cache miss, executing`（turbo）と `ok ... (cached)`（Go）が同時に出るのは、この粒度差による
- [x] **`go.mod` の `go` ディレクティブが何を意味するか**（インストールされている Go のバージョンとの関係）説明できる
  - このモジュールが**要求する最低の Go バージョン**であり、同時に使う言語仕様のバージョン（言語仕様を決めるのは**マイナーまで**。パッチ部分は最低要求の判定にしか使われない）
  - **手元の Go が要求より古いときだけ**、`GOTOOLCHAIN`（既定値 `auto`）が必要なツールチェーンを自動ダウンロードして使う。**手元が新しい場合は手元のものをそのまま使う**
  - この非対称性のため、`go.mod` を `mise.toml` より新しい値にすると mise の管理外（`~/.cache/go/toolchain`）にツールチェーンが落ち、「バージョンの真実は `mise.toml` に集約する」前提が静かに破れる。**Go を下げるときは `go.mod` を先に下げる**（[ADR 20260815-1420](../adr/20260815-1420-go-version-and-module-path.md) の「トレードオフ」参照）

## 5. 不足情報TODO

- [x] ~~**Go のモジュールパスを決める。**~~ → `github.com/hi-lee-mon/copan/apps/api`（[ADR 20260815-1420](../adr/20260815-1420-go-version-and-module-path.md)）
- [x] ~~**Go のバージョンをどれにし、`mise.toml` に含めるか。**~~ → `mise.toml` に `go = "1.26.6"`、`go.mod` も `go mod init` の既定値 `1.26.6` のまま（同 ADR）
- [x] ~~Lambda アダプタを自前で書くか、既存ライブラリを使うか~~ → `aws-lambda-go-api-proxy/httpadapter` の `NewV2`（[ADR 20260815-1906](../adr/20260815-1906-lambda-adapter-and-runtime.md)）
- [x] ~~Lambda のランタイムに何を使うか（`provided.al2023` 等）とバイナリ名の規約~~ → `provided.al2023` / arm64 / `bootstrap`（同 ADR）
- [x] ~~**`apps/api` の turbo タスク設定** — 成果物の出力先（上記 A / B）、`test` タスクのキャッシュ方針~~ → **A**（`dist/bootstrap`、`apps/api/turbo.json` は作らない）、`test` はキャッシュ有効のまま（[ADR 20260815-2314](../adr/20260815-2314-go-build-output-and-turbo-tasks.md)）
- [x] ~~ルーターの組み立てをどこに置くか~~ → `internal/rest/router.go`（[ADR 20260815-2315](../adr/20260815-2315-router-placement.md)）

## 6. 実装ステップ

1. **Go のバージョンとモジュールパスを決める**（不足情報TODO の1・2）。決めたら `mise.toml` と `go.mod` に反映する。**ADR を書く対象**
2. **失敗するテスト**: `/ping` に GET したら 200 と期待するボディが返ることを `httptest` で検証する。ハンドラが無いのでコンパイルが通らない、または落ちる
3. **通す**: `internal/` にハンドラを実装し、テストを緑にする
4. **ローカル起動**: `cmd/local` を追加し、`go run` で起動して curl で確認する
5. **Lambda 側**: `cmd/lambda` を追加し、アダプタ経由で同じハンドラを包む。**ビルドが通ることまで**を確認する（デプロイは 007）
6. **turbo に載せる**: `apps/api/package.json` のダミーを実際の `go build` / `go test` に差し替える。成果物の出力先に応じて `apps/api/turbo.json` を作る
7. **キャッシュを検証する**: 成果物を消した状態で `turbo run build` を流し、**キャッシュから復元されること**を確認する（002 で扱った `outputs` の罠を踏んでいないかの確認）
8. **リファクタ**: ルーティングの置き場所を整理する。004 で生成コードに置き換わることを念頭に、置き換えやすい形にしておく

### テスト観点

- 正常系: `/ping` が 200 を返す
- レスポンスの Content-Type が意図どおりか
- 未定義のパスが 404 を返すか
- 許可していない HTTP メソッドのときどうなるか
- ハンドラがローカル用と Lambda 用で**同一のもの**を指しているか（分岐して二重実装になっていないか）

## 7. 完了条件

- [x] `go test ./...` が通る
- [x] `go run ./cmd/local` で起動し、`curl localhost:8081/ping` が 200 と `pong` を返す
- [x] `go build ./cmd/lambda` が成功する
- [x] ローカルと Lambda が同一のハンドラを共有している（実装が二重になっていない）— 両者とも `rest.NewRouter()` を呼ぶ
- [x] ルートから `pnpm exec turbo run test` を流すと、Go のテストが実行される
- [x] ルートから `pnpm exec turbo run build` を流すと、Go のビルドが実行される
- [x] **ビルド成果物を削除してから `turbo run build` を流すと、キャッシュヒット（`FULL TURBO`）した上で成果物が復元される**
- [x] `.gitignore` が Go のビルド成果物と整合している（バイナリがコミット対象に入らない）— 成果物は `dist/` 配下なので既存の `dist/` 行が効く。`bootstrap` 行は手動ビルド時の保険として残す
- [x] Go のバージョンとモジュールパスの決定が ADR に残っている
- [x] 学習TODOがすべて埋まっている

## 8. 振り返り（完了時に本人が記入）

- 詰まった点:golangやturborepoのキャッシュの仕組み。golangのそもそもの書き方。golangのアーキテクチャ。
- 分かったこと:golangのことがあまりわかっていないし、フォルダ構成の決め方もわからない。キャッシュの仕組みはある程度理解できた。
- 見積とのズレと、その原因:時間は見積もり通りだったと思うけど、golangもturborepoも初めてで理解二時間がかかった。
