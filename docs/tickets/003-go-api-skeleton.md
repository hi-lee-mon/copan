# 003: Go の API 雛形を作り、ローカルで叩ける状態にする

- ステータス: 未着手
- 見積: 3h / 実績: -h
- 依存: 002（ディレクトリ構成が決まっていること）
- 関連: [ADR 20260812-0712 インフラ構成](../../adr/20260812-0712-infrastructure-cloudflare-aws-hybrid.md)、[ADR 20260812-0659 Go採用](../../adr/20260812-0659-backend-go-openapi-contract.md)

## 1. 目的

Go の API を **ローカルで HTTP サーバーとして起動して叩ける**状態にし、同時に **Lambda 用のエントリポイントもビルドできる**状態にする。

## 2. 背景

[ADR 20260812-0712](../../adr/20260812-0712-infrastructure-cloudflare-aws-hybrid.md) で、API の実行環境は **AWS Lambda（VPC外）+ Function URL** に決まっている。

しかし **デプロイより先にローカル実行を確立する**。プロジェクトの方針は TDD であり、テストが Lambda 上でしか回らない状態では TDD のサイクルが成立しないため。

このチケットで置くエンドポイントは **仮の `/ping`** とする。本命の `/health` は 004 で TypeSpec の契約として定義し、005 で生成された型から実装する。**手書きしたものが契約駆動に置き換わる過程**を、そのまま体験の対象にする。

## 3. 事前調査

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
3. Lambda から離れられない（[ADR 20260812-0712](../../adr/20260812-0712-infrastructure-cloudflare-aws-hybrid.md) は Lambda を選んだが、可逆性は保っておきたい）

対策として **アプリ本体を標準の `net/http.Handler` として書き、Lambda 側は薄いアダプタにする**構成がある。

```text
internal/         ← アプリ本体（http.Handler として書く）
cmd/local/        ← ローカル用。http.ListenAndServe で起動
cmd/lambda/       ← Lambda用。アダプタで http.Handler を包んで lambda.Start
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

## 4. 学習TODO

- [ ] Lambda のコールドスタートとウォームスタートで、何が再利用されるか説明できる
- [ ] `http.Handler` を挟むことで、何が可逆になるのかを説明できる
- [ ] `internal/` パッケージの制約を説明できる
- [ ] `httptest` を使ったテストが、Lambda のイベント型を直接扱うテストより優れている理由を説明できる
- [ ] Go のクロスコンパイルで Lambda 向けバイナリを作るとき、何を指定する必要があるか説明できる

## 5. 不足情報TODO

- [ ] **Go のモジュールパスを決める**（`github.com/<user>/copan/...` 形式にするならリモートリポジトリが必要 → 001 の名称確定とセットで判断）
- [ ] Lambda アダプタを自前で書くか、既存ライブラリを使うか
- [ ] Go のバージョン固定方法（`go.mod` の `go` ディレクティブと、CI・Lambda ランタイムを揃える）
- [ ] Lambda のランタイムに何を使うか（`provided.al2023` 等）とバイナリ名の規約

## 6. 実装ステップ

1. **失敗するテスト**: `/ping` に GET したら 200 と期待するボディが返ることを `httptest` で検証する。ハンドラが無いのでコンパイルが通らない、または落ちる
2. **通す**: `internal/` にハンドラを実装し、テストを緑にする
3. **ローカル起動**: `cmd/local` を追加し、`go run` で起動して curl で確認する
4. **Lambda 側**: `cmd/lambda` を追加し、アダプタ経由で同じハンドラを包む。**ビルドが通ることまで**を確認する（デプロイは 007）
5. **リファクタ**: ルーティングの置き場所を整理する。004 で生成コードに置き換わることを念頭に、置き換えやすい形にしておく

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
- [ ] ルートから `turbo run test` で Go のテストも走る
- [ ] 学習TODOがすべて埋まっている

## 8. 振り返り（完了時に本人が記入）

- 詰まった点:
- 分かったこと:
- 見積とのズレと、その原因:
