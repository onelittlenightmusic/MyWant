/*
 * The guide in Japanese. content.en.js is the same guide in English: both keep
 * the same topic ids, icons and colours, so a link (#install) opens the same
 * topic in either language. `ui` is the page's own wording; `sections` are the
 * rows of cards, each topic one card and the sidebar page it opens. `body` is
 * HTML — a `.code` block's "#" lines are drawn as comments, and every block
 * gets a copy button (app.js). Accent colours are mywant-gui's menu colours.
 */
window.GUIDE = window.GUIDE || {};
window.GUIDE.ja = {
  ui: {
    htmlLang: 'ja',
    langName: '日本語',
    docTitle: 'MyWant ガイド',
    subtitle: 'やさしいガイド',
    topics: n => `${n} topics`,
    heroLead: '「やってほしいこと」を書くと、エージェントが代わりに動いてくれる仕組みです。',
    heroSub: 'カードを押すと、右側にくわしい説明が開きます。上から順に読めば、インストールから最初の一歩まで進めます。',
    menu: 'メニュー',
    sections: 'Sections',
    startHere: 'Start here',
    theme: '明るさの切り替え',
    lang: 'Switch to English',
    close: '閉じる',
    copy: 'コピー',
    prev: '前へ',
    grid: '一覧',
    next: '次へ',
  },
  sections: [
    {
      id: 'start',
      title: 'はじめる',
      note: '上から順に読めば、最初の一歩まで進めます',
      icon: 'rocket',
      topics: [
        {
          id: 'what',
          step: 'Step 1',
          title: 'MyWant ってなに？',
          sub: '「やってほしいこと」を書くと、代わりに動いてくれる',
          icon: 'heart',
          color: '#ec4899',
          body: `
<p>ふだん、コンピュータに何かをさせるときは「<strong>どうやるか</strong>」を順番に書きます。
MyWant では逆に、「<strong>何がほしいか</strong>」だけを書きます。</p>
<div class="box">
  <p class="box-title"><i data-lucide="message-circle"></i>たとえば</p>
  <p>「15分後にコーヒー休憩を知らせてほしい」「東京の今日の天気を知りたい」</p>
</div>
<p>この「ほしいこと」ひとつひとつを <strong>Want（ウォント）</strong> と呼びます。
Want を置くと、それを得意とする <strong>Agent（エージェント）</strong> が動いて、結果を Want に書き込んでくれます。</p>
<h4>できること</h4>
<ul>
  <li>リマインダー・天気・経路検索・旅行の予約など、いろいろな Want をすぐ使える</li>
  <li>Want どうしをつないで、流れ作業にできる</li>
  <li>うまくいった組み合わせを <strong>Recipe</strong> として保存し、何度でも使える</li>
  <li>ブラウザの画面で、いま何が起きているかをカードで眺められる</li>
  <li>サーバーを止めても、状態はちゃんと残る</li>
</ul>
<h4>ふたつの部品</h4>
<dl class="terms">
  <dt>mywant</dt><dd>本体のサーバーと、操作用のコマンド</dd>
  <dt>mywant-gui</dt><dd>ブラウザで見る画面（ダッシュボード）</dd>
</dl>
`,
        },
        {
          id: 'install',
          step: 'Step 2',
          title: 'インストール',
          sub: 'Mac なら Homebrew で 3 行',
          icon: 'download',
          color: '#10b981',
          body: `
<p>Mac では <a href="https://brew.sh/ja/" target="_blank" rel="noopener">Homebrew</a> を使うのがいちばん簡単です。
「ターミナル」アプリを開いて、下のコマンドを順に貼りつけてください。</p>
<ol class="steps">
  <li><strong>配布元を登録する</strong>
    <div class="code"><pre>brew tap onelittlenightmusic/mywant</pre></div>
  </li>
  <li><strong>配布元を信頼する</strong>（はじめの 1 回だけ）
    <div class="code"><pre>brew trust onelittlenightmusic/mywant</pre></div>
  </li>
  <li><strong>本体と画面を入れる</strong>
    <div class="code"><pre>brew install mywant mywant-gui</pre></div>
  </li>
</ol>
<div class="box">
  <p class="box-title"><i data-lucide="circle-help"></i>うまく入ったか確かめる</p>
  <p>次のコマンドでバージョンが表示されれば成功です。</p>
</div>
<div class="code"><pre>mywant version</pre></div>
<h4>ソースから作る場合</h4>
<p>Go が入っていれば、リポジトリから自分でビルドすることもできます。できあがったコマンドは <code>./bin/mywant</code> にあります。</p>
<div class="code"><pre>git clone https://github.com/onelittlenightmusic/mywant.git
cd mywant
make release</pre></div>
`,
        },
        {
          id: 'run',
          step: 'Step 3',
          title: '起動と停止',
          sub: 'サーバーと画面を動かす',
          icon: 'power',
          color: '#14b8a6',
          body: `
<p>MyWant は裏で動き続ける<strong>サーバー</strong>です。まずサーバーを、次に画面を起動します。
<code>-D</code> は「裏で動かしておく」という意味です。</p>
<ol class="steps">
  <li><strong>サーバーを起動する</strong>
    <div class="code"><pre>mywant start -D</pre></div>
  </li>
  <li><strong>画面を起動する</strong>
    <div class="code"><pre>mywant-gui start -D</pre></div>
  </li>
  <li><strong>ブラウザで開く</strong><br />
    <a href="http://localhost:8081" target="_blank" rel="noopener">http://localhost:8081</a> を開くと、ダッシュボードが表示されます。
  </li>
</ol>
<h4>動いているか確かめる</h4>
<div class="code"><pre>mywant ps</pre></div>
<h4>止める</h4>
<div class="code"><pre>mywant-gui stop
mywant stop</pre></div>
<div class="box">
  <p class="box-title"><i data-lucide="info"></i>ポート番号</p>
  <p>サーバーは 8080 番、画面は 8081 番を使います。ほかのアプリと重なるときは <code>mywant start -D --port 9090</code> のように変えられます。</p>
</div>
`,
        },
        {
          id: 'first-want',
          step: 'Step 4',
          title: 'はじめての Want',
          sub: '天気とリマインダーを置いてみる',
          icon: 'sparkles',
          color: '#ec4899',
          body: `
<p>サーバーが動いたら、Want を置いてみましょう。</p>
<h4>いちばん短い方法</h4>
<p>「東京の今日の天気」を知りたい Want です。種類（<code>weather</code>）と、場所だけを渡します。</p>
<div class="code"><pre>mywant wants create -t weather --param at=Tokyo</pre></div>
<p>少し待ってから一覧を見ると、結果が入っています。</p>
<div class="code"><pre>mywant wants list</pre></div>
<h4>ファイルに書く方法</h4>
<p>ほしいことを YAML というかんたんな書式でファイルに書いておくこともできます。
<code>coffee.yaml</code> という名前で保存してください。</p>
<div class="code"><pre>wants:
  - metadata:
      name: coffee-break
      type: reminder
    spec:
      params:
        message: "コーヒー休憩の時間です ☕"
        duration_from_now: "15 minutes"</pre></div>
<div class="code"><pre>mywant wants create -f coffee.yaml</pre></div>
<p>15 分たつと、ダッシュボードのカードがお知らせしてくれます。</p>
<h4>画面から置く</h4>
<p>ヘッダ右端の <strong>＋</strong> ボタンからも Want を追加できます。種類を選んで、項目を埋めるだけです。</p>
<h4>片づける</h4>
<div class="code"><pre>mywant wants delete coffee-break</pre></div>
`,
        },
        {
          id: 'dashboard',
          step: 'Step 5',
          title: '画面の見かた',
          sub: 'ヘッダ・カード・サイドバー',
          icon: 'layout-dashboard',
          color: '#6366f1',
          body: `
<p>ダッシュボードは、このページと同じ 3 つの部分でできています。</p>
<dl class="terms">
  <dt>ヘッダ</dt><dd>いちばん上の帯。左の <strong>Menu</strong> からページを切り替え、右のボタンで Want を追加します。</dd>
  <dt>カード</dt><dd>Want やエージェントが 1 枚ずつカードになって並びます。色やアイコンで状態がわかります。</dd>
  <dt>サイドバー</dt><dd>カードを押すと右側に開き、くわしい中身や設定が見られます。</dd>
</dl>
<h4>Menu の中のページ</h4>
<dl class="terms">
  <dt>Wants</dt><dd>置いた Want の一覧</dd>
  <dt>Thing</dt><dd>名前をつけて覚えておいた値</dd>
  <dt>Want Types</dt><dd>置ける Want の種類</dd>
  <dt>Worlds</dt><dd>保存した Want の組み合わせ</dd>
  <dt>Agents</dt><dd>働いているエージェント</dd>
  <dt>Recipes</dt><dd>再利用できるひな形</dd>
  <dt>Logs</dt><dd>何が起きたかの記録</dd>
</dl>
<div class="box">
  <p class="box-title"><i data-lucide="smartphone"></i>スマホでも</p>
  <p>同じネットワークにあるスマホからも開けます。画面はスマホの幅に合わせて並び替わります。</p>
</div>
`,
        },
      ],
    },
    {
      id: 'ideas',
      title: 'しくみ',
      note: 'MyWant に出てくることば',
      icon: 'lightbulb',
      topics: [
        {
          id: 'want',
          title: 'Want',
          sub: 'やってほしいこと、ひとつ分',
          icon: 'heart',
          color: '#ec4899',
          body: `
<p>Want は「こうなってほしい」というお願いをひとつ書いたものです。書くことは 3 つだけです。</p>
<dl class="terms">
  <dt>name</dt><dd>名前（自分でわかれば何でもよい）</dd>
  <dt>type</dt><dd>種類（天気なら <code>weather</code>）</dd>
  <dt>params</dt><dd>その種類に渡す値（場所、時刻など）</dd>
</dl>
<h4>Want の一生</h4>
<p>置かれた Want は、だいたい次のように進みます。</p>
<dl class="terms">
  <dt>待機</dt><dd>置かれたばかり</dd>
  <dt>実行中</dt><dd>エージェントが取り組んでいる</dd>
  <dt>達成</dt><dd>ほしかったものが手に入った</dd>
  <dt>失敗</dt><dd>うまくいかなかった（理由がカードに出ます）</dd>
</dl>
<p>途中で止めたり、また動かしたりもできます。</p>
<div class="code"><pre>mywant wants suspend coffee-break
mywant wants resume coffee-break</pre></div>
`,
        },
        {
          id: 'want-type',
          title: 'Want Type',
          sub: 'Want の種類と、受け取る値',
          icon: 'zap',
          color: '#a855f7',
          body: `
<p>Want Type は「どんな Want が置けるか」のカタログです。
<code>weather</code>（天気）、<code>reminder</code>（リマインダー）、<code>timer</code>（タイマー）、<code>checklist</code>（チェックリスト）などがはじめから入っています。</p>
<h4>一覧を見る</h4>
<div class="code"><pre>mywant types list</pre></div>
<h4>どんな値を渡せばいいか調べる</h4>
<div class="code"><pre>mywant types get reminder</pre></div>
<p>説明と、そのまま使える例が表示されます。例をそのまま置くこともできます。</p>
<div class="code"><pre>mywant wants create -t reminder -e</pre></div>
<div class="box">
  <p class="box-title"><i data-lucide="puzzle"></i>自分で増やせる</p>
  <p>種類は追加できます。くわしくは「追加機能を入れる」を見てください。</p>
</div>
`,
        },
        {
          id: 'agent',
          title: 'Agent',
          sub: 'Want のために実際に動く係',
          icon: 'bot',
          color: '#3b82f6',
          body: `
<p>Agent（エージェント）は、Want をかなえるために実際に手を動かす係です。
それぞれ「できること」（<strong>Capability</strong>）を持っていて、Want に必要なことができる Agent が自動で選ばれます。</p>
<h4>3 つのタイプ</h4>
<dl class="terms">
  <dt>Do</dt><dd>1 回だけやる（予約する、送信する など）</dd>
  <dt>Monitor</dt><dd>見張り続ける（値が変わったら知らせる など）</dd>
  <dt>Think</dt><dd>考える（予算の配分を決める など）</dd>
</dl>
<h4>一覧を見る</h4>
<div class="code"><pre>mywant agents list
mywant capabilities list</pre></div>
<p>自分で Agent を書く必要はありません。はじめは、すでにある Agent に任せておけば大丈夫です。</p>
`,
        },
        {
          id: 'chain',
          title: 'つなげる',
          sub: 'Want の結果を次の Want へ',
          icon: 'link',
          color: '#0ea5e9',
          body: `
<p>Want は単独で動くだけでなく、<strong>ほかの Want の結果を受け取る</strong>こともできます。
工場のベルトコンベアのように、前の工程の出力が次の工程に流れていきます。</p>
<p>つなぎ方は <code>using</code> に「どの Want から受け取るか」を書くだけです。</p>
<div class="code"><pre>wants:
  - metadata: {name: numbers, type: numbers, labels: {role: source}}
  - metadata: {name: queue, type: queue}
    spec:
      using: [{role: source}]</pre></div>
<p>ここでは「<code>role: source</code> というラベルのついた Want から受け取る」と書いています。名前ではなくラベルで選ぶので、つなぎ先を後から入れ替えやすくなっています。</p>
<h4>3 つの並び方</h4>
<dl class="terms">
  <dt>ばらばら</dt><dd>つながず、同時に動く（旅行の飛行機・ホテル・レストラン）</dd>
  <dt>一列</dt><dd>前の結果を次が使う（数を作る → 並べる → 数える）</dd>
  <dt>まとめ役</dt><dd>いくつかの結果を 1 つにまとめる</dd>
</dl>
`,
        },
        {
          id: 'recipe',
          title: 'Recipe',
          sub: 'Want の組み合わせをひな形に',
          icon: 'book-open',
          color: '#10b981',
          body: `
<p>いくつかの Want をいつも一緒に置くなら、その組み合わせを <strong>Recipe（レシピ）</strong> として保存できます。
料理のレシピと同じで、材料（値）だけ変えれば何度でも作れます。</p>
<h4>一覧を見る</h4>
<div class="code"><pre>mywant recipes list</pre></div>
<h4>いま動いている Want からレシピを作る</h4>
<div class="code"><pre>mywant recipes create --from-want &lt;Want の ID&gt; --name my-recipe</pre></div>
<h4>質問に答えながら作る</h4>
<div class="code"><pre>mywant recipes create -i</pre></div>
<p>ダッシュボードでは、選んだ Want をまとめてレシピとして保存することもできます。</p>
`,
        },
        {
          id: 'thing',
          title: 'Thing',
          sub: '名前をつけて覚えておく値',
          icon: 'circle',
          color: '#f59e0b',
          body: `
<p>Thing は、よく使う値に名前をつけて覚えておく場所です。
「自宅の駅」「よく行くお店」などを入れておくと、Want を作るときにすぐ選べます。</p>
<div class="code"><pre># 覚えさせる
mywant thing add station 中野
# 見る
mywant thing list</pre></div>
<p>ダッシュボードでは、Thing のカードから直接 Want を作れます。関係のある Thing どうしはまとめて星座（constellation）にできます。</p>
`,
        },
        {
          id: 'world',
          title: 'World',
          sub: '置いた Want をまるごと保存',
          icon: 'layers',
          color: '#6366f1',
          body: `
<p>World は、いま置いている Want 全部のスナップショットです。
「旅行の準備」「仕事の見張り」のように場面ごとに保存しておき、切り替えて使えます。</p>
<div class="code"><pre>mywant world save travel
mywant world list
mywant world open travel</pre></div>
<p>別の World を開くとき、いまの World は自動で保存されるので、作業が消えることはありません。</p>
<h4>ほかの人に渡す</h4>
<div class="code"><pre>mywant world export travel -o travel.yaml
mywant world import travel -f travel.yaml</pre></div>
`,
        },
      ],
    },
    {
      id: 'more',
      title: 'もっと使う',
      note: '慣れてきたら',
      icon: 'wrench',
      topics: [
        {
          id: 'cli',
          title: 'よく使うコマンド',
          sub: 'これだけ覚えれば大丈夫',
          icon: 'terminal',
          color: '#64748b',
          body: `
<p>どのコマンドも、後ろに <code>--help</code> をつけると使い方が出ます。</p>
<h4>Want</h4>
<div class="code"><pre>mywant wants list                 # 一覧
mywant wants get coffee-break     # くわしく見る
mywant wants create -f want.yaml  # ファイルから置く
mywant wants delete coffee-break  # 消す</pre></div>
<h4>サーバー</h4>
<div class="code"><pre>mywant start -D   # 起動
mywant ps         # 状態
mywant logs       # 記録を見る
mywant stop       # 停止</pre></div>
<h4>設定</h4>
<div class="code"><pre>mywant config get                    # いまの設定を見る
mywant config set server_port 9090   # 1 つ変える
mywant config set                    # 質問に答えて設定する</pre></div>
<p>設定は <code>~/.mywant/config.yaml</code> に保存されます。</p>
`,
        },
        {
          id: 'custom',
          title: '追加機能を入れる',
          sub: 'Want Type やレシピを足す',
          icon: 'puzzle',
          color: '#a855f7',
          body: `
<p><strong>Custom</strong> は、MyWant に新しい Want Type・レシピ・見た目などを足すための追加パックです。
GitHub のリポジトリ名を指定するだけで入ります。</p>
<div class="code"><pre># 入れる
mywant custom install owner/repo
# 入っているものを見る
mywant custom list
# 外す
mywant custom uninstall &lt;名前&gt;</pre></div>
<div class="box">
  <p class="box-title"><i data-lucide="key-round"></i>API キーが必要なとき</p>
  <p>外部サービスを使う Custom には鍵が必要なことがあります。鍵は次のように設定し、サーバーを起動し直すと読み込まれます。</p>
</div>
<div class="code"><pre>mywant config env set NAME --stdin</pre></div>
`,
        },
        {
          id: 'remote',
          title: '別のサーバーを使う',
          sub: 'クラウドのサーバーに切り替える',
          icon: 'globe',
          color: '#14b8a6',
          body: `
<p>MyWant のサーバーは、自分の Mac だけでなくクラウドでも動かせます。
コマンドの行き先は <strong>コンテキスト</strong> で切り替えます（kubectl と同じ考え方です）。</p>
<div class="code"><pre># 行き先を登録する
mywant config set-context cloud \\
  --server https://example.fly.dev \\
  --username mywant --password-env MYWANT_AUTH_PASSWORD

# 切り替える
mywant config use-context cloud

# 一覧と、いまの行き先
mywant config get-contexts
mywant config current-context</pre></div>
<p>パスワードそのものは設定ファイルに書かず、環境変数の名前だけを登録します。</p>
<p>1 回だけ別の行き先に送りたいときは <code>--context local</code> のように付けます。</p>
`,
        },
        {
          id: 'update',
          title: 'アップデート',
          sub: '新しい版にする',
          icon: 'refresh-cw',
          color: '#10b981',
          body: `
<p>Homebrew で入れた場合は、次のコマンドで新しい版になります。</p>
<div class="code"><pre>brew update
brew upgrade mywant mywant-gui</pre></div>
<p>更新が終わったら、サーバーと画面を起動し直してください。</p>
<div class="code"><pre>mywant-gui stop && mywant stop
mywant start -D && mywant-gui start -D</pre></div>
<p>置いてあった Want は保存されているので、起動し直してもそのまま戻ってきます。</p>
`,
        },
        {
          id: 'trouble',
          title: '困ったとき',
          sub: 'よくあるつまずき',
          icon: 'life-buoy',
          color: '#6b7280',
          body: `
<h4>brew install で権限のエラーが出る</h4>
<p><code>brew trust onelittlenightmusic/mywant</code> を先に実行してから、もう一度インストールしてください。</p>
<h4>画面が開かない</h4>
<p><code>mywant ps</code> でサーバーが動いているか確かめます。止まっていたら <code>mywant start -D</code>、画面だけなら <code>mywant-gui start -D</code> です。</p>
<h4>Want が失敗になる</h4>
<p>どこでつまずいたかを見るには、Want のくわしい状態と、サーバーの記録を見ます。</p>
<div class="code"><pre>mywant wants get coffee-break
mywant logs</pre></div>
<h4>使っているポートを変えたい</h4>
<p><code>mywant start -D --port 9090</code> のように起動します。</p>
<h4>もっと知りたい</h4>
<ul>
  <li><a href="https://github.com/onelittlenightmusic/mywant" target="_blank" rel="noopener">MyWant のリポジトリ</a></li>
  <li><a href="https://github.com/onelittlenightmusic/mywant/blob/master/docs/MYWANT_CLI_USAGE.md" target="_blank" rel="noopener">コマンドの説明（英語）</a></li>
  <li><a href="https://github.com/onelittlenightmusic/mywant-gui" target="_blank" rel="noopener">mywant-gui のリポジトリ</a></li>
</ul>
`,
        },
      ],
    },
  ],
};
