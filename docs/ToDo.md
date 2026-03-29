# やりたいこと

- とにかく最初にやりたいことをwantに記載しておき、そのタイミングではwantを実現できなかったとしても、reachingのままwaitしておき、achievementが増えてunlockされるcapabilityが増えてくると、自然とwantが実行できるようになる、というゲームの感じをこのmywantで実現したい。
- やりたいことはまず自然言語で書いておいていい。ただの思いつきメモ。
- その思いつきメモを徐々にブレークダウンしていく。そこはThinkerの仕事。
- 最初の思いつきメモ(whim)をemptyの代わりに作りたい. whimはemptyのrecipeにデフォルトでwantというstate fieldを追加したもの。 
- 全てのrecipeにデフォルトで登録するstate fieldにwantというフィールドを追加したい。
- フロントエンドでは、+bottleのアイコンのクリックイベントをemptyレシピの代わりにwhimを追加するショートカットとしたい。
- emptyレシピは排除してwhimで置き換えたい。

---
- 上記までは成功。




---

- want detailやadd wantなどのサイドバーの表示をヘッダの上に被せないようにしたい。つまりヘッダエリアをexcludeした画面高さを利用するようにしたい。これはヘッダが常に見えるようにしたいからである。
- want detailサイドバーの表示、非表示のアニメーションは排除（即座に表示する）したい。
- その他のサイドバーの表示アニメーションは３倍速度にしたい。
- add wantサイドバー内のwant type選択後のエリアの配置を、最初にパラメータにしてパラメータのchevronを開いた状態としたい。want nameは一番下に配置したい。tabキーでのフォーカス順序もそれに合わせて変更したい。
- add want内のパラメータ入力用のテキストボックスは縦に拡大可能にしたい。


----
以下は後回し
- whimのparameter "want"に記載したものに対して、アイディアを書き込み、その後にrobotからサジェスチョンをして子Wantを足していくというのをやりたい。その時にThinkerを一度追加しておき、そのThinkerとやりとりをしながら、Wantを追加させたい。
- 現在Draftの仕組みがすでにあるが、そのDraftを子Wantとして利用するようなイメージ。
- 最初にThinker子Wantを追加する。ThinkerはSiblingを追加する権利を有する。それはDispatchと同じ。
- ThinkerはDraftと同じようにアイディアの候補がいくつか表示される。（これはすでにある実装）
- アイディアを選択すると、Siblingが必要に応じて追加される。
- 既存のDraftとは異なり、Thinkerはずっと消えない。
- 新たなアイディアを親のwant parameterに追記することができる。
- ThinkerはInteract bubbleのようにインタラクティブにやりとりをすることができる。履歴も保持している。

---

iPhone表示の際のサイドバーが下から上がってくるようになったのは良い。
この下から上がってくるのはLayoutがbottomの時だけにする。
逆にBottomの時には上から下に下げる。
そのレイアウト変更に加えて、サイドバー内の"new want"やadd wantボタンなどのサイドバー内のヘッダの位置を統一的にbottom時には下、top時には上にしたい。一貫性が生まれるはず。

---

add wantサイドバー内のadd wantボタン、closeボタン(x)をheaderOverlayやwant card overlay controlなどと同じスタイルにしたい。

---

インタラクティブなWant（current.Interactive=true）のwant cardには、Agentアイコンの左にChatアイコンを入れたい。クリックした時にはwant detailのチャットタブを開くようにしたい。

---

マルチセレクトモードでは、カードの選択のみとし、AgentやChatなどのタブを開くアクションなどはできないようにしたい。

---

