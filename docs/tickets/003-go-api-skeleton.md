# 003: Go の API 雛形を作り、ローカルで叩ける状態にする

- ステータス: 未着手（**着手可能**）
- 見積: 3h / 実績: -h
- 依存: **002（完了）** — `apps/api` のパッケージ境界と turbo のタスク定義がある前提
- 関連: [ADR 20260812-0712 インフラ構成](../adr/20260812-0712-infrastructure-cloudflare-aws-hybrid.md)、[ADR 20260812-0659 Go採用](../adr/20260812-0659-backend-go-openapi-contract.md)、[ADR 20260812-1305 Go の置き方](../adr/20260812-1305-go-in-turborepo-workspace.md)、[ADR 20260812-1324 バージョン固定](../adr/20260812-1324-version-pinning-and-audit.md)、[ADR 20260814-1826 商標の繰り延べ](../adr/20260814-1826-defer-trademark-research.md)

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
    "test": "echo test @repo/api"                          // ← 差し替える
  },
  "devDependencies": {
    "@repo/api-spec": "workspace:*" // ← 004 で TypeSpec が入ると効いてくる順序制御
  }
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
    "test": "go test ./..."
  }
}
```

**ここで 002 で体験した罠がそのまま当てはまる。**

ルートの `turbo.jsonc` は `outputs: ["dist/**"]` を全パッケージに適用している。Go のビルド成果物を **`dist/` 以外の場所に出すなら、`outputs` の指定が実態と食い違う**。食い違うとどうなるかは 002 の学習TODOで確認済み——**キャッシュヒット時にバイナリが復元されず、緑色で成功したのに成果物が無い**という形で表面化する。

対処は2通り。**どちらを採るかはこのチケットの判断。**

| | 内容 |
| --- | --- |
| A | Go の成果物も `dist/` に出し、ルートの定義をそのまま使う |
| B | `apps/api/turbo.json` を作り、`extends` でこのパッケージだけ `outputs` を上書きする |

```jsonc
// apps/api/turbo.json（B を採る場合）
{
  "extends": ["//"], // ← ルートの定義を継承し、必要な部分だけ上書きする
  "tasks": {
    "build": { "outputs": ["???"] }
  }
}
```

> ルートは `turbo.jsonc`（コメント可）にした。パッケージ側の拡張子を揃えるかも判断すること。turbo は両方読む。

もう一点、**`test` タスクのキャッシュ**に論点がある。Go は `go test` に自前のキャッシュを持ち、2回目は `(cached)` と表示される。turbo のキャッシュはその上に重なる（ADR 1305 の「キャッシュが二重になる論点」）。**turbo 側で `test` に何を指定するか**（`outputs` は要るか、`cache` を切るか）を決める。

### Lambda 向けのビルド

`.gitignore` には既に `bootstrap` が入っている。これは Lambda のカスタムランタイムが**エントリポイントのバイナリ名として固定で要求する名前**であることに由来する。

→ 調べること: `provided.al2023` ランタイムが要求するバイナリ名と配置。クロスコンパイルで指定すべき環境変数（開発機は darwin/arm64、Lambda は linux）。

## 4. 学習TODO

- [ ] Lambda のコールドスタートとウォームスタートで、何が再利用されるか説明できる
- [ ] `http.Handler` を挟むことで、何が可逆になるのかを説明できる
- [ ] `internal/` パッケージの制約を説明できる
- [ ] `httptest` を使ったテストが、Lambda のイベント型を直接扱うテストより優れている理由を説明できる
- [ ] Go のクロスコンパイルで Lambda 向けバイナリを作るとき、何を指定する必要があるか説明できる
- [ ] **Go 自身のビルド／テストキャッシュと、Turborepo のキャッシュが、それぞれ何を単位にしているか説明できる**
- [ ] **`go.mod` の `go` ディレクティブが何を意味するか**（インストールされている Go のバージョンとの関係）説明できる

## 5. 不足情報TODO

- [ ] **Go のモジュールパスを決める。** `github.com/<user>/copan/apps/api` 形式が慣例だが、**このリポジトリにはまだリモートが無い**（`git remote` が空）。また名称は [ADR 20260814-1826](../adr/20260814-1826-defer-trademark-research.md) により v0.1 の間は「仮の COPAN」で、**ドメインと SNS は取得しない**方針。GitHub リポジトリ名は改名可能（＝可逆）なので、**名称確定を待つ理由があるかを判断する**
- [ ] **Go のバージョンをどれにし、`mise.toml` に含めるか。** [ADR 20260812-1324](../adr/20260812-1324-version-pinning-and-audit.md) の「結果として必要になる決定」に挙がっている宿題。ローカルは 1.25.1（mise のグローバル設定）、mise からは 1.26.6 まで取得可能。**含めるなら CI（006）の Go セットアップも `jdx/mise-action` に寄る**。あわせて `go.mod` の `go` ディレクティブとの関係を整理する
- [ ] Lambda アダプタを自前で書くか、既存ライブラリを使うか
- [ ] Lambda のランタイムに何を使うか（`provided.al2023` 等）とバイナリ名の規約
- [ ] **`apps/api` の turbo タスク設定** — 成果物の出力先（上記 A / B）、`test` タスクのキャッシュ方針

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

- [ ] `go test ./...` が通る
- [ ] `go run ./cmd/local` で起動し、`curl localhost:<port>/ping` が 200 を返す
- [ ] `go build ./cmd/lambda` が成功する
- [ ] ローカルと Lambda が同一のハンドラを共有している（実装が二重になっていない）
- [ ] ルートから `pnpm exec turbo run test` を流すと、Go のテストが実行される
- [ ] ルートから `pnpm exec turbo run build` を流すと、Go のビルドが実行される
- [ ] **ビルド成果物を削除してから `turbo run build` を流すと、キャッシュヒット（`FULL TURBO`）した上で成果物が復元される**
- [ ] `.gitignore` が Go のビルド成果物と整合している（バイナリがコミット対象に入らない）
- [ ] Go のバージョンとモジュールパスの決定が ADR に残っている
- [ ] 学習TODOがすべて埋まっている

## 8. 振り返り（完了時に本人が記入）

- 詰まった点:
- 分かったこと:
- 見積とのズレと、その原因:
