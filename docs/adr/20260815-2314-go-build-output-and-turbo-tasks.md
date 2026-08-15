# Go のビルド成果物を `apps/api/dist/bootstrap` に出し、パッケージ単位の `turbo.json` を作らない

- ステータス: 承認
- 日付: 2026-08-15 23:14
- 関連: [20260812-1305 Go を Turborepo に含める](./20260812-1305-go-in-turborepo-workspace.md)、[20260812-1324 バージョン固定と audit](./20260812-1324-version-pinning-and-audit.md)、[20260815-1420 Go のバージョンとモジュールパス](./20260815-1420-go-version-and-module-path.md)、[20260815-1906 Lambda アダプタとランタイム](./20260815-1906-lambda-adapter-and-runtime.md)、[チケット 003](../tickets/003-go-api-skeleton.md)

## コンテキスト

[20260812-1305](./20260812-1305-go-in-turborepo-workspace.md) と [20260815-1420](./20260815-1420-go-version-and-module-path.md) が、ともに「**`apps/api` の turbo タスク設定 — ビルド成果物の出力先と `outputs` の指定、`test` タスクのキャッシュ方針**」を宿題として残していた。チケット 003 のステップ6で、`apps/api/package.json` のダミータスクを実際の Go のコマンドに差し替えるにあたり、これを決める必要が出た。

制約は次のとおり。

- **ルートの `turbo.jsonc` は `outputs: ["dist/**"]` を全パッケージに一律で適用している。** 002 で作った定義で、Node 側の3パッケージがこれに従っている
- **Lambda のバイナリ名は `bootstrap` で固定。** [20260815-1906](./20260815-1906-lambda-adapter-and-runtime.md) で `provided.al2023` を選んでおり、このランタイムはエントリポイントのバイナリ名として `bootstrap` を要求する。名前は変えられない
- **`turbo` の `outputs` は「タスクの実行後に、そのパスに存在するファイルをキャッシュへ保存する」仕組みである。** 「そのコマンドが作ったファイル」を判別しているわけではない。申告漏れがあると、キャッシュヒット時に成果物が復元されないまま**タスクが成功扱いになる**

最後の点は 002 の学習TODOで既に目視確認しており、本チケットでも実際に踏んだ（後述「トレードオフ」）。

## 決定

### 1. 成果物は `apps/api/dist/bootstrap` に出す

```jsonc
// apps/api/package.json
{
  "scripts": {
    "build": "mkdir -p dist && GOOS=linux GOARCH=arm64 go build -o dist/bootstrap ./cmd/lambda",
    "test": "go test ./..."
  }
}
```

`mkdir -p dist` が必要なのは、**`go build -o <パス>` が `<パス>` の親ディレクトリを作らない**ため。存在しなければ `open dist/bootstrap: no such file or directory` で失敗する。

### 2. `apps/api/turbo.json` は作らない

ルートの `turbo.jsonc` の `outputs: ["dist/**"]` をそのまま使う。

### 3. `test` タスクはルートの `"test": {}` のまま。`outputs` を指定せず、キャッシュも切らない

## 理由

### 出力先を `dist/` にした理由

- **設定ファイルが増えない。** ルートの定義がそのまま当たるため、`apps/api/turbo.json` を作らずに済む
- **`outputs` の定義を1箇所に保てる。** ルートとパッケージの2箇所を照合しなくてよい
- **Node 側のパッケージと出力先の規約が揃う。** モノレポ内で「成果物は `dist/`」が一貫する
- **`.gitignore` の既存の `dist/` 行がそのまま効く。** Go のバイナリを別途無視する設定が要らない

### `apps/api/turbo.json` を作らなかった理由

[20260812-1305](./20260812-1305-go-in-turborepo-workspace.md) の決定の3点目は「パッケージ単位の `turbo.json` を `"extends": ["//"]` で置き、`build` の `outputs` に Go のビルド成果物を指定する」だった。**本 ADR はこの一点を更新する。**

理由は、同 ADR の主眼が「**公式パターンを正とし、独自の工夫を持ち込まない**」ことにあり、パッケージ単位の `turbo.json` はその手段として挙げられていたに過ぎないため。成果物を `dist/` に出せばルートの定義で足りるので、設定ファイルを1枚増やす理由が消える。**手段が不要になっただけで、方針自体は保たれている。**

### `test` にキャッシュ設定を足さなかった理由

[20260812-1305](./20260812-1305-go-in-turborepo-workspace.md):79 が既に整理しているとおり、`go test` はファイルを吐かないため `outputs` に書くものが無く、キャッシュされるのはログのみになる。これは正しい挙動である。

`cache: false` にする案は同 ADR の案 B として却下済み。ルート `turbo.jsonc` の `"test": {}` に手を入れる理由が無い。

なお **turbo の入力ハッシュには `.go` ファイルが含まれる**（`.gitignore` されていない Git 管理下のファイルはすべて材料になる）ため、コードを変えれば必ず再実行される。turbo がスキップする状況は「何も変わっていないとき」に限られ、変更を見逃す経路は無い。

## 却下した案

### B: `apps/api/turbo.json` を作り、`outputs` をパッケージ直下のバイナリに上書きする

```jsonc
{ "extends": ["//"], "tasks": { "build": { "outputs": ["bootstrap"] } } }
```

`go build -o bootstrap ./cmd/lambda` と書け、Lambda が要求する `bootstrap` という名前がパスの読み替えなしにそのまま現れる利点がある。デプロイ時に zip を作る際も、パッケージ直下のほうが素直になる可能性がある。

**却下の理由**: 得られるのは「`-o` に `dist/` を書かなくて済む」ことだけで、失うのは「設定ファイルが1枚増え、`outputs` の定義がルートとパッケージの2箇所に分かれる」こと。デプロイ時の zip 化（007）は `dist/` の中から固めれば済み、この案でなければ困る場面が無い。**トレードオフが一方的に不利。**

### `test` タスクに `cache: false` を指定する

Go 側に正確なキャッシュがあるのだから、turbo 側は必ず実行させるべきという案。

**却下の理由**: [20260812-1305](./20260812-1305-go-in-turborepo-workspace.md) の案 B として既に却下している。同 ADR が引く公式の整理では `cache: false` は「長時間走る開発タスク」「実行グラフに含まれたら必ず走らせたいタスク」向けであり、テストはそのどちらでもない。

### `cmd/local` のバイナリも `build` で作る

`go build -o dist/local ./cmd/local` を併せて実行する案。

**却下の理由**: ローカル実行は `go run ./cmd/local` で足り、バイナリを置く用途が無い。作れば `outputs` に含まれてキャッシュ容量を消費し、クロスコンパイル対象（darwin/arm64）が Lambda 用（linux/arm64）と混ざる。**必要になってから足す。**

## トレードオフとして受け入れること

- **`go build` の `-o` に `dist/` を書く必要があり、`mkdir -p dist` も要る。** `go build` は出力先の親ディレクトリを作らないため、この2つは省略できない
- **`outputs` は「実行後にそこに在るファイル」を保存するため、前の世代の残骸が混入しうる。** 実際に本チケットで踏んだ。002 のダミータスクが残した `dist/out.txt` が `dist/` に残ったまま `go build` に差し替えたため、cache miss で実行した際に `bootstrap` と `out.txt` の両方がキャッシュへ同梱され、以降キャッシュヒットのたびに `out.txt` が復元され続けた。**復旧手順は「`dist/` を空にしてから `turbo run build --force` でキャッシュを取り直す」**
- **成果物の出力先が Lambda の要求と1段ずれる。** zip のトップレベルに `bootstrap` が来る必要があるため、007 では `dist/` の中から固める形になる
- **Go のビルドキャッシュと turbo のキャッシュが二重になる。** [20260812-1305](./20260812-1305-go-in-turborepo-workspace.md) が既に受け入れている。単位が違う（Go はパッケージ単位、turbo はタスク単位）ため層として重なるだけで、衝突はしない

## 結果として必要になる決定

- **007 のデプロイ手順で、`dist/bootstrap` をどう zip に固めるか**（`dist/` を作業ディレクトリにする形になる見込み）
- **CI で `turbo run build` を流すときのキャッシュ戦略**（リモートキャッシュを使うか。チケット 006）
- **`go vet` を turbo のタスクとして独立させるか**（現状 `test` にも `build` にも含まれていない）
