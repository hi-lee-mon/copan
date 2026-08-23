# 生成した OpenAPI を emitter の既定の出力先に出し、そのままコミットする

- ステータス: 承認
- 日付: 2026-08-23 14:23
- 関連: [20260812-0659 バックエンドを Go とし、型共有を TypeSpec 起点の OpenAPI で行う](./20260812-0659-backend-go-openapi-contract.md)、[20260812-1313 ディレクトリ構成](./20260812-1313-monorepo-directory-layout.md)、[20260815-1420 Go のバージョンとモジュールパス](./20260815-1420-go-version-and-module-path.md)、[20260815-2314 ビルド成果物と turbo タスク](./20260815-2314-go-build-output-and-turbo-tasks.md)、[チケット 004](../tickets/004-typespec-openapi-health.md)、[チケット 005](../tickets/005-health-from-generated-types.md)

## コンテキスト

[チケット 004](../tickets/004-typespec-openapi-health.md) で TypeSpec から OpenAPI を生成するにあたり、**生成した `openapi.yaml` をどこに出し、コミットするか**が論点になった。チケットは**3つの決定が正面からぶつかる**と整理していた。

| 決定 | 内容 |
| --- | --- |
| [CLAUDE.md の制約](../../CLAUDE.md) | 生成物は**コミットし**、CI で再生成して差分が出たら失敗させる |
| ルートの `turbo.jsonc` | `build` の `outputs` は `["dist/**"]`。ここに載らない成果物はキャッシュから復元されない |
| `.gitignore` | `dist/` は**無視する** |

つまり「`dist/` に出す」と `outputs` はそのまま効くがコミットできず、「コミット対象の場所に出す」と `outputs` に載らない、という対立である。

**ただしこの対立は、「生成を turbo の `build` タスクで回す」ことを暗黙の前提にしていた。** 実装を進める中で、その前提自体が選択肢であることが分かった。

## 決定

### 1. 出力先は `@typespec/openapi3` の既定のまま変えない

```text
packages/api-spec/tsp-output/@typespec/openapi3/openapi.yaml
```

**`tspconfig.yaml` を作らない。** `emitter-output-dir` も `output-file` も指定しない。

### 2. 生成物をコミットする

`.gitignore` に手を加えない。`tsp-output/` は元から無視の対象ではないため、追加の記述なしで追跡対象になる。

### 3. 生成は turbo の `build` タスクに載せない

`packages/api-spec/package.json` の `build` は 002 のダミーのままとし、生成は独立したスクリプトから回す。

```jsonc
// packages/api-spec/package.json
{ "scripts": { "gen": "tsp compile . --emit=@typespec/openapi3" } }
// package.json（ルート）
{ "scripts": { "gen-oa": "pnpm --filter @repo/api-spec gen" } }
```

**これは暫定の措置であり、恒久的な決定ではない。** 生成タスクを turbo にどう載せるかは [チケット 005](../tickets/005-health-from-generated-types.md) で、OpenAPI → Go の生成と合わせて決める。

## 理由

### 決定3が、決定1と2を可能にしている

**3つの決定の対立は、生成を `build` に載せないことで消える。** `outputs: ["dist/**"]` が問題になるのは `build` タスクの成果物だけであり、`build` が生成しないなら `tsp-output/` が `outputs` に載っていなくても何も壊れない。

チケット 004 が挙げた A〜D は、いずれも「`build` で生成する」前提で `outputs` か `.gitignore` のどちらかを崩す案だった。**前提を外すと、どちらも崩さずに済む。**

その代わり「生成し忘れると古いまま通る」という穴が開くが、これは**チケット 006 の CI（再生成して差分が出たら失敗させる）が塞ぐことになっている**。[CLAUDE.md の制約](../../CLAUDE.md) が「生成物はコミットし、CI で再生成して差分が出たら失敗させる」とセットで書いているのは、まさにこの穴を前提にしているためである。

### 既定値をそのまま使う

[ADR 20260815-1420](./20260815-1420-go-version-and-module-path.md) が確立した方針——**「公式ツールが生成した既定値を、この構成で実際に何が良くなるかを示せないなら手で書き換えない」**——をそのまま適用した。

出力先を `packages/api-spec/openapi/openapi.yaml` のような短いパスに変えても、得られるのは「パスが短い」ことだけである。**参照するのは設定ファイルと CI であって人間が毎日開くファイルではない**ため、短さの利益がほぼ無い。

副次的に、`tspconfig.yaml` というファイルを1枚増やさずに済む。設定ファイルは「既定と違うことをしている」という情報を持つが、既定と同じことしか書かない設定ファイルはその情報を持たない。

### コミットする理由

- **API の変更が差分としてレビューできる。** `main.tsp` の変更が OpenAPI にどう波及したかが、コミットの差分に並んで見える
- **`packages/api-client` と `apps/api` の生成物が、生成器を持たない環境でも読める**。`git clone` した直後に契約が確認できる
- [CLAUDE.md の制約](../../CLAUDE.md)「生成物はコミットし、CI で再生成して差分が出たら失敗させる」がそもそもこれを要求している

## 却下した案

チケット 004 が提示した4案は、いずれも「生成を `build` タスクで回す」前提に立っている。**その前提を外した時点で、4案とも解こうとしていた問題が消えた。**

### A: `dist/` に出し、`.gitignore` に例外行（`!packages/api-spec/dist/`）を足す

`outputs: ["dist/**"]` がそのまま効き、キャッシュから復元される。

**却下の理由**: 「`dist/` は生成物なので無視する」という規約に穴が開く。`.gitignore` の否定パターンは**親ディレクトリが無視されていると効かない**という直感に反する挙動があり、読み手が「なぜここだけコミットされるのか」を追うコストが継続的に発生する。得られる利益（turbo のキャッシュ復元）は、`build` に載せない今は存在しない。

### B: コミット対象の場所に出し、ルートの `turbo.jsonc` の `outputs` に足す

**却下の理由**: ルートの定義に1パッケージのためのパスが混ざる。加えて `build` で生成しない以上、`outputs` に足す必要そのものが無い。

### C: `packages/api-spec/turbo.json` を `extends` で作って上書きする

**却下の理由**: B と同じく、`build` で生成しないなら不要。[ADR 20260815-2314](./20260815-2314-go-build-output-and-turbo-tasks.md) の「パッケージ単位の `turbo.json` を作らない」とも衝突する。**必要になっていない例外を先に作らない。**

### D: 生成物をコミットしない（CI で毎回生成する）

**却下の理由**: [CLAUDE.md の制約](../../CLAUDE.md) と [ADR 20260812-0659](./20260812-0659-backend-go-openapi-contract.md) を覆すことになる。差分で API の変更が見えなくなる利益は無い。

### 出力先だけ短いパスに変える（`tspconfig.yaml` を書く）

`packages/api-spec/openapi/openapi.yaml` に出す案。パスが短く、emitter 名がパスに現れないため emitter を差し替えても場所が変わらない。

**却下の理由**: 上記「既定値をそのまま使う」のとおり、この構成で実際に良くなるものを示せない。**なお emitter を差し替える可能性は現実に存在する**（トレードオフに記載）ため、そのときに改めて判断する。

## トレードオフとして受け入れること

- **パスが深く、`@` を含む。** `packages/api-spec/tsp-output/@typespec/openapi3/openapi.yaml` を、005 の Go 生成器の設定と 006 の CI に書くことになる。`apps/api` からの相対パスは `../../packages/api-spec/tsp-output/@typespec/openapi3/openapi.yaml` になる
- **出力先が emitter の名前に依存する。** ディレクトリ名 `@typespec/openapi3` は emitter のパッケージ名そのもの。emitter を差し替えるとパスが変わり、それを参照している箇所を全部直すことになる
- **生成し忘れると、古い `openapi.yaml` のまま通る。** `build` に載せていないため、turbo は生成の要否を判断しない。006 の CI が入るまでは人間の責任
- **turbo のキャッシュが効かない。** 生成は毎回まるごと走る。`/health` 1本の規模では体感できないが、契約が育つと効いてくる
- **`build` に載せるかの判断が残る。** ただし 004 が整理した「`outputs` に載らないとキャッシュから復元されない」という害は、**生成物がコミットされている限り発生しない**（復元すべき対象が git に在るため）。載せるかどうかは、この害とは別の理由——生成し忘れを構造的に防げるか——で判断する。[チケット 005](../tickets/005-health-from-generated-types.md) で扱う

## 結果として必要になる決定

- **生成タスクを turbo にどう載せるか**（`build` に載せるか、`generate` を新設するか、手動のままにするか）。**OpenAPI → Go の生成と同じ問いなので、[チケット 005](../tickets/005-health-from-generated-types.md) でまとめて決める**
- **CI で再生成して差分が出たら失敗させる仕組み**（チケット 006）
- **`info.version` を埋めるか**（埋めるなら `@typespec/openapi` の追加が要る。現在は `0.0.0`）
