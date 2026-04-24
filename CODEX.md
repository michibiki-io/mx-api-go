# 目的

- ../go-max-apiserver-old/ にある go / gogin で書かれた sendmail server を go versin 1.26.2 の最新に更新しつつベストプラクティスな内容に変更していきます

### 要件

- 言語要件は go です
- マイクロサービスの名前は mx-api にします。起動する bin ファイルの名前も、それにしてください。ただし、パッケージ名は、今の mx-api-go です。
- ../go-max-apiserver-old/ を使っている静的サイトを置き換えるので、互換性は完全に維持してください。API やリクエスト内容です
- api は subpath を想定して作ってください
- ただし、3年も前の実装でいろいろと古いので、最新のベストプラクティスにのっとって修正して下さい
- ../go-max-apiserver-old/ は Firebase App check の内容を含んでいますが、これは [turnstile-appcheck-gateway](https://github.com/michibiki-io/turnstile-appcheck-gateway) に置き換えるので完全に削除してください
- すでにあるコードは build アクション用のものです。基本的に変えないでください (必要に応じて修正する程度)
- ../go-max-apiserver-old/ が受け取る入力フォーマットの入力値は、現在、かなり固定的になっています。今、すでにあるものは規定値として残しますが、これらも config 値として別出ししてください。また、その config を変えることで、後からユーザが、config で追加できるようにしたいです。
- ../go-max-apiserver-old/ では簡易的な validate もありますが、この validate を、外部の有名な validator ライブラリ等に移行したいです。
- validate のルールも既存の項目もユーザが追加する項目も config で制御できるようにしてください。この項目は、この validate ルールを使う　などのように yaml の config で制御できるようにしてください。
- validator はエラーを返すようになっていると思います。このエラーの仕様も現状に合わせてください。
- config は編集がわかりやすい yaml 形式にしてください
- ../go-max-apiserver-old/ には HTML メール用のテンプレートを2つ持っています。このメールテンプレートを、より分かりやすく、また、モダンなものにしてください。このテンプレートも上記 config 経由で差し替え可能にしてください ( テンプレートそのものは別ファイルを外部からマウントするのでテンプレートの参照パスが返られる程度 )
- メールのテンプレートは設計容易性を考え jinja template にします。
- ../go-max-apiserver-old/ は開発・動作確認用のフロントエンドを Hugo で持っていますが、これは不要です。
- 上記 Hugo のフロントエンドの代わりに、開発用のフロントエンドを作ってください。api server のチェックができるシンプルなものでよいです。ただし、上記、別に用意される API server 側の config 内容に基づき、フロントエンド側の入力項目が可変で変わるように工夫してください。
- 開発用のフロントエンドは、最も軽量でポータビリティの良い GUI framework を使ってください
- 開発用フロントエンドと go backend を同時に立ち上げる docker-compose を作ってください
- go は test も通してください
- README.md も用意してください
- ISSUE.md を用意してください。go-max-apiserver-old からの移植であるという文言等はいりません。
```md
### 背景
### 目的
### Tasks
- []
```