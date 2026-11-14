/*
 * The guide in English — the same topics, ids, icons and colours as
 * content.ja.js (see the note there). Change both together.
 */
window.GUIDE = window.GUIDE || {};
window.GUIDE.en = {
  ui: {
    htmlLang: 'en',
    langName: 'English',
    docTitle: 'MyWant Guide',
    subtitle: 'A gentle guide',
    topics: n => `${n} topics`,
    heroLead: 'Write down what you want done, and agents do it for you.',
    heroSub: 'Press a card to open its explanation on the right. Read them from the top and you will go from installing to your first step.',
    menu: 'Menu',
    sections: 'Sections',
    startHere: 'Start here',
    theme: 'Switch light and dark',
    lang: '日本語に切り替える',
    close: 'Close',
    copy: 'Copy',
    prev: 'Prev',
    grid: 'All',
    next: 'Next',
    guides: 'Guides',
    guidesNote: 'More guides to MyWant',
  },
  sections: [
    {
      id: 'start',
      title: 'Getting started',
      note: 'Read from the top to take your first step',
      icon: 'rocket',
      topics: [
        {
          id: 'what',
          step: 'Step 1',
          title: 'What is MyWant?',
          sub: 'Say what you want, and it gets done for you',
          icon: 'heart',
          color: '#ec4899',
          body: `
<p>Usually, to make a computer do something, you write down <strong>how</strong> to do it, step by step.
In MyWant you do the opposite: you only write down <strong>what you want</strong>.</p>
<div class="box">
  <p class="box-title"><i data-lucide="message-circle"></i>For example</p>
  <p>"Remind me to take a coffee break in 15 minutes." "Tell me today's weather in Tokyo."</p>
</div>
<p>Each of these wishes is called a <strong>Want</strong>.
When you place a Want, an <strong>Agent</strong> that is good at it gets to work and writes the result back into the Want.</p>
<h4>What you can do</h4>
<ul>
  <li>Use many ready-made Wants right away: reminders, weather, route search, travel bookings and more</li>
  <li>Connect Wants to each other, like an assembly line</li>
  <li>Save a combination that worked as a <strong>Recipe</strong> and use it again and again</li>
  <li>Watch what is happening as cards in your browser</li>
  <li>Stop the server whenever you like — the state is kept</li>
</ul>
<h4>Two parts</h4>
<dl class="terms">
  <dt>mywant</dt><dd>The server itself, and the command you control it with</dd>
  <dt>mywant-gui</dt><dd>The screen you look at in your browser (the dashboard)</dd>
</dl>
${shot("dashboard.jpg", "MyWant in the browser: every Want you place becomes a card")}
`,
        },
        {
          id: 'install',
          step: 'Step 2',
          title: 'Install',
          sub: 'Three lines with Homebrew on a Mac — or Docker',
          icon: 'download',
          color: '#10b981',
          body: `
<p>On a Mac, <a href="https://brew.sh/" target="_blank" rel="noopener">Homebrew</a> is the easiest way.
Open the Terminal app and paste the commands below, one at a time.</p>
<ol class="steps">
  <li><strong>Add the source</strong>
    <div class="code"><pre>brew tap onelittlenightmusic/mywant</pre></div>
  </li>
  <li><strong>Trust the source</strong> (only the first time)
    <div class="code"><pre>brew trust onelittlenightmusic/mywant</pre></div>
  </li>
  <li><strong>Install the server and the screen</strong>
    <div class="code"><pre>brew install mywant mywant-gui</pre></div>
  </li>
</ol>
<div class="box">
  <p class="box-title"><i data-lucide="circle-help"></i>Did it work?</p>
  <p>If this command prints a version, you are all set.</p>
</div>
<div class="code"><pre>mywant version</pre></div>
<h4>Run it with Docker</h4>
<p>With Docker, you can run it without installing anything: one file (<code>docker-compose.yml</code>) starts the two containers, MyWant itself and the screen.</p>
<ol class="steps">
  <li><strong>Get the file</strong>
    <div class="code"><pre>curl -O https://raw.githubusercontent.com/onelittlenightmusic/MyWant/master/docker-compose.yml</pre></div>
  </li>
  <li><strong>Start it</strong>
    <div class="code"><pre>docker compose up -d</pre></div>
  </li>
  <li><strong>Open <a href="http://localhost:8081" target="_blank" rel="noopener">http://localhost:8081</a></strong></li>
</ol>
<ul>
  <li>Your Wants and the rest are kept in the <code>mywant-data</code> volume. <code>docker compose down</code> keeps them; <code>down -v</code> erases them</li>
  <li>The MyWant server is published on this machine only (<code>127.0.0.1:8080</code>), so the <code>mywant</code> command works as usual</li>
  <li>For reminders at the right time, set <code>TZ</code> in the file to your time zone (e.g. <code>Europe/London</code>)</li>
  <li>To open it from a phone or another machine, first set <code>MYWANT_AUTH_PASSWORD</code> (a password), then change the port to <code>"8081:8080"</code></li>
  <li>Upgrade with <code>docker compose pull &amp;&amp; docker compose up -d</code></li>
  <li>The canvas (mywant-guiex) is not in the images; install it with Homebrew to use it</li>
</ul>
<h4>Want the canvas too?</h4>
<p>The <strong>mywant-guiex</strong> extension adds the <strong>canvas</strong> (Wants as tiles on a board) and <strong>Web Wants</strong> (the sites you use, as Wants) to the screen. MyWant works fine without it.</p>
<div class="code"><pre>brew install mywant-guiex</pre></div>
<p>Then just reload the screen in your browser. Upgrade it together with mywant-gui — see <a href="https://onelittlenightmusic.github.io/mywant-gui/?lang=en#install">Install in the mywant-gui guide</a>.</p>
<h4>Building from source</h4>
<p>If you have Go, you can build it yourself from the repository. The command ends up in <code>./bin/mywant</code>.</p>
<div class="code"><pre>git clone https://github.com/onelittlenightmusic/mywant.git
cd mywant
make release</pre></div>
`,
        },
        {
          id: 'run',
          step: 'Step 3',
          title: 'Start and stop',
          sub: 'Run the server and the screen',
          icon: 'power',
          color: '#14b8a6',
          body: `
<p>MyWant is a <strong>server</strong> that keeps running in the background. Start the server first, then the screen.
<code>-D</code> means "keep it running in the background".</p>
<ol class="steps">
  <li><strong>Start the server</strong>
    <div class="code"><pre>mywant start -D</pre></div>
  </li>
  <li><strong>Start the screen</strong>
    <div class="code"><pre>mywant-gui start -D</pre></div>
  </li>
  <li><strong>Open it in your browser</strong><br />
    Go to <a href="http://localhost:8081" target="_blank" rel="noopener">http://localhost:8081</a> and the dashboard appears.
  </li>
</ol>
<h4>Is it running?</h4>
<div class="code"><pre>mywant ps</pre></div>
<h4>Stop</h4>
<div class="code"><pre>mywant-gui stop
mywant stop</pre></div>
<div class="box">
  <p class="box-title"><i data-lucide="info"></i>Ports</p>
  <p>The server uses port 8080 and the screen uses 8081. If another app already uses one, change it, like <code>mywant start -D --port 9090</code>.</p>
</div>
${shot("dashboard.jpg", "What you see at http://localhost:8081")}
`,
        },
        {
          id: 'first-want',
          step: 'Step 4',
          title: 'Your first Want',
          sub: 'Try the weather and a reminder',
          icon: 'sparkles',
          color: '#ec4899',
          body: `
<p>Once the server is running, let's place a Want.</p>
<h4>The shortest way</h4>
<p>A Want for today's weather in Tokyo. You give it only its kind (<code>weather</code>) and the place.</p>
<div class="code"><pre>mywant wants create -t weather --param at=Tokyo</pre></div>
<p>Wait a moment, then look at the list — the result is in.</p>
<div class="code"><pre>mywant wants list</pre></div>
<h4>Writing it in a file</h4>
<p>You can also write what you want in a file, in a simple format called YAML.
Save this as <code>coffee.yaml</code>.</p>
<div class="code"><pre>wants:
  - metadata:
      name: coffee-break
      type: reminder
    spec:
      params:
        message: "Time for a coffee break ☕"
        duration_from_now: "15 minutes"</pre></div>
<div class="code"><pre>mywant wants create -f coffee.yaml</pre></div>
<p>After 15 minutes, its card on the dashboard lets you know.</p>
<h4>From the screen</h4>
<p>You can also add a Want with the <strong>+</strong> button at the right end of the header. Pick a kind and fill in the fields.</p>
<h4>Clean up</h4>
<div class="code"><pre>mywant wants delete coffee-break</pre></div>
${shot("dashboard-detail.jpg", "A Want for the weather in Tokyo. Press its card and the details open on the right")}
${shot("add-want-form.jpg", "The form behind the + button")}
`,
        },
        {
          id: 'dashboard',
          step: 'Step 5',
          title: 'Reading the screen',
          sub: 'Header, cards and sidebar',
          icon: 'layout-dashboard',
          color: '#6366f1',
          body: `
<p>The dashboard is made of the same three parts as this page.</p>
<dl class="terms">
  <dt>Header</dt><dd>The bar at the top. Change pages from <strong>Menu</strong> on the left; add Wants with the buttons on the right.</dd>
  <dt>Cards</dt><dd>Each Want or agent is one card. Colours and icons show how it is doing.</dd>
  <dt>Sidebar</dt><dd>Press a card and it opens on the right, with the details and settings.</dd>
</dl>
<h4>Pages in the Menu</h4>
<dl class="terms">
  <dt>Wants</dt><dd>The Wants you have placed</dd>
  <dt>Thing</dt><dd>Values you have named and kept</dd>
  <dt>Want Types</dt><dd>The kinds of Want you can place</dd>
  <dt>Worlds</dt><dd>Saved sets of Wants</dd>
  <dt>Agents</dt><dd>The agents at work</dd>
  <dt>Recipes</dt><dd>Reusable templates</dd>
  <dt>Logs</dt><dd>A record of what happened</dd>
</dl>
<div class="box">
  <p class="box-title"><i data-lucide="smartphone"></i>On your phone too</p>
  <p>A phone on the same network can open it as well. The screen rearranges itself to fit.</p>
</div>
<p>The <a href="https://onelittlenightmusic.github.io/mywant-gui/?lang=en">mywant-gui guide</a> explains the screen in detail.</p>
${shot("menu.jpg", "Menu, at the top left, leads to every page")}
${shot("canvas-detail.jpg", "With the canvas extension (mywant-guiex), Wants become tiles on a board")}
`,
        },
      ],
    },
    {
      id: 'ideas',
      title: 'How it works',
      note: 'The words you will meet in MyWant',
      icon: 'lightbulb',
      topics: [
        {
          id: 'want',
          title: 'Want',
          sub: 'One thing you want done',
          icon: 'heart',
          color: '#ec4899',
          body: `
<p>A Want is one wish: "I'd like this to happen". You write only three things.</p>
<dl class="terms">
  <dt>name</dt><dd>A name (anything that makes sense to you)</dd>
  <dt>type</dt><dd>Its kind (<code>weather</code> for the weather)</dd>
  <dt>params</dt><dd>The values that kind takes (a place, a time, …)</dd>
</dl>
<h4>A Want's life</h4>
<p>A Want you place goes roughly like this.</p>
<dl class="terms">
  <dt>Idle</dt><dd>Just placed</dd>
  <dt>Running</dt><dd>An agent is working on it</dd>
  <dt>Achieved</dt><dd>You got what you wanted</dd>
  <dt>Failed</dt><dd>It didn't work out (the card says why)</dd>
</dl>
<p>You can pause it and carry on later.</p>
<div class="code"><pre>mywant wants suspend coffee-break
mywant wants resume coffee-break</pre></div>
${shot("dashboard-results.jpg", "The Results tab: what the agents wrote back")}
`,
        },
        {
          id: 'want-type',
          title: 'Want Type',
          sub: 'Kinds of Want, and the values they take',
          icon: 'zap',
          color: '#a855f7',
          body: `
<p>Want Types are the catalogue of Wants you can place.
<code>weather</code>, <code>reminder</code>, <code>timer</code>, <code>checklist</code> and many more come built in.</p>
<h4>See them all</h4>
<div class="code"><pre>mywant types list</pre></div>
<h4>Find out what values to give</h4>
<div class="code"><pre>mywant types get reminder</pre></div>
<p>It shows a description and examples you can use as they are. You can even place the example directly.</p>
<div class="code"><pre>mywant wants create -t reminder -e</pre></div>
<div class="box">
  <p class="box-title"><i data-lucide="puzzle"></i>You can add more</p>
  <p>New kinds can be added — see "Add-ons".</p>
</div>
${shot("want-types.jpg", "The Want Types page. Press a kind to see its description and examples")}
`,
        },
        {
          id: 'magic-sentence',
          title: 'Make one in a few words',
          sub: 'Magic sentences and aliases',
          icon: 'wand-sparkles',
          color: '#8b5cf6',
          body: `
<p>A <strong>magic sentence</strong> makes a Want from a short, fixed phrase.
Give the train route Want Type the sentence “<code>&lt;from:station&gt;から&lt;to:station&gt;</code>”,
and saying “荻窪から中野坂上” places a Want with <code>from</code> = 荻窪 and <code>to</code> = 中野坂上.
No AI is asked, so it is instant and always does the same thing.</p>
<div class="box">
  <p class="box-title"><i data-lucide="message-circle"></i>Where can I say it?</p>
  <ul>
    <li>In the robot's chat (app or browser)</li>
    <li>In the MyWant keyboard (iPhone, Mac) — type an alias and it is offered</li>
    <li>With <code>mywant magic</code> (see “Ask in a few words”, after “Everyday commands”)</li>
  </ul>
</div>
<p>If a Want with the same values is already there, nothing new is made; it is shown instead.</p>
<h4>Set it with labels</h4>
<p>Put two <strong>labels</strong> on a Want Type or a Want:</p>
<ul>
  <li><code>magic-sentence</code> — the phrase's form. You can give several.</li>
  <li><code>aliases</code> — the type's other names, comma-separated. Used by <code>&lt;alias&gt;</code>.</li>
</ul>
<div class="code"><pre>wantType:
  metadata:
    name: transit_search
    labels:
      aliases: "乗り換え,乗換,行き方,transit"
      magic-sentence: "&lt;from:station&gt;から&lt;to:station&gt;|&lt;alias&gt; &lt;from:station&gt; &lt;to:station&gt;"</pre></div>
<p>On a Want Type it works for every Want of that type (the usual place).
You can also put it on a single Want; then it wins over the type's.</p>
<p>To give one Want its own, add a <code>magic-sentence</code> label from the Want's Labels on screen (e.g. <code>&lt;from:station&gt;から帰る</code>).</p>
<h4>Notation</h4>
<table>
  <tr><th>Write</th><th>Means</th></tr>
  <tr><td><code>&lt;from&gt;</code></td><td>The words there go into the parameter <code>from</code></td></tr>
  <tr><td><code>&lt;from:station&gt;</code></td><td>The same, and <code>station</code> Things are preferred; the keyboard and completion offer only stations</td></tr>
  <tr><td><code>&lt;alias&gt;</code></td><td>Any of the <code>aliases</code> (case ignored); fills no parameter</td></tr>
  <tr><td><code>|</code> or a new line</td><td>Separates phrases; matching any one is enough</td></tr>
  <tr><td>A space</td><td>One or more spaces are needed there (<code>transit 荻窪 中野坂上</code>)</td></tr>
  <tr><td>Other characters</td><td>Matched as written (<code>から</code>, <code>まで</code>…)</td></tr>
</table>
<ul>
  <li>It works only when the whole phrase fits a form. A closing “。” or “?” is ignored.</li>
  <li>When several forms fit, the one that names more known Things wins.</li>
  <li>The keyboard and <code>mywant m</code>'s completion use a form that starts with <code>&lt;alias&gt;</code> with its parts separated by spaces (<code>&lt;alias&gt; &lt;from:station&gt; &lt;to:station&gt;</code>).</li>
</ul>
`,
        },
        {
          id: 'agent',
          title: 'Agent',
          sub: 'The one who actually does the work',
          icon: 'bot',
          color: '#3b82f6',
          body: `
<p>An Agent is what actually does the work to make a Want come true.
Each has things it can do (<strong>Capabilities</strong>), and an Agent that can do what the Want needs is picked for it automatically.</p>
<h4>Three kinds</h4>
<dl class="terms">
  <dt>Do</dt><dd>Does it once (makes a booking, sends a message, …)</dd>
  <dt>Monitor</dt><dd>Keeps watching (tells you when a value changes, …)</dd>
  <dt>Think</dt><dd>Thinks it through (decides how to split a budget, …)</dd>
</dl>
<h4>See them all</h4>
<div class="code"><pre>mywant agents list
mywant capabilities list</pre></div>
<p>You don't need to write Agents yourself. To begin with, leave it to the ones already there.</p>
${shot("agents.jpg", "The Agents page. Press an agent to see what it can do")}
`,
        },
        {
          id: 'chain',
          title: 'Connecting',
          sub: "One Want's result goes to the next",
          icon: 'link',
          color: '#0ea5e9',
          body: `
<p>Wants don't only work alone — they can <strong>take the results of other Wants</strong>.
Like a conveyor belt, what one step makes flows on to the next.</p>
<p>To connect them, write in <code>using</code> which Wants to take from.</p>
<div class="code"><pre>wants:
  - metadata: {name: numbers, type: numbers, labels: {role: source}}
  - metadata: {name: queue, type: queue}
    spec:
      using: [{role: source}]</pre></div>
<p>This says "take from the Wants labelled <code>role: source</code>". Choosing by label rather than by name makes it easy to swap what is connected later.</p>
<h4>Three shapes</h4>
<dl class="terms">
  <dt>Side by side</dt><dd>Not connected, running at once (flight, hotel and restaurant for a trip)</dd>
  <dt>In a line</dt><dd>Each uses the one before (make numbers → queue them → count them)</dd>
  <dt>Coordinator</dt><dd>Gathers several results into one</dd>
</dl>
`,
        },
        {
          id: 'recipe',
          title: 'Recipe',
          sub: 'A set of Wants, as a template',
          icon: 'book-open',
          color: '#10b981',
          body: `
<p>If you always place a few Wants together, save the set as a <strong>Recipe</strong>.
Just like a cooking recipe, change only the ingredients (values) and make it as often as you like.</p>
<h4>See them all</h4>
<div class="code"><pre>mywant recipes list</pre></div>
<h4>Make one from Wants that are running</h4>
<div class="code"><pre>mywant recipes create --from-want &lt;want ID&gt; --name my-recipe</pre></div>
<h4>Make one by answering questions</h4>
<div class="code"><pre>mywant recipes create -i</pre></div>
<p>On the dashboard you can also save the Wants you have selected as a recipe.</p>
${shot("recipes.jpg", "The Recipes page")}
`,
        },
        {
          id: 'thing',
          title: 'Thing',
          sub: 'Values you name and keep',
          icon: 'circle',
          color: '#f59e0b',
          body: `
<p>Things are where you keep values you use often, under a name.
Put in "my home station" or "my favourite shop", and you can pick them straight away when you make a Want.</p>
<div class="code"><pre># keep one
mywant things add station Nakano
# look at them
mywant things list</pre></div>
<p>On the dashboard you can make a Want right from a Thing's card. Related Things can be grouped into a constellation.</p>
${shot("thing.jpg", "The Thing page: the values you have kept, as cards")}
`,
        },
        {
          id: 'world',
          title: 'World',
          sub: 'Save all your Wants at once',
          icon: 'layers',
          color: '#6366f1',
          body: `
<p>A World is a snapshot of every Want you have placed.
Save one per situation — "trip planning", "watching work" — and switch between them.</p>
<div class="code"><pre>mywant world save travel
mywant world list
mywant world open travel</pre></div>
<p>Opening another World saves the current one first, so nothing you were doing is lost.</p>
<h4>Hand it to someone else</h4>
<div class="code"><pre>mywant world export travel -o travel.yaml
mywant world import travel -f travel.yaml</pre></div>
${shot("worlds.jpg", "The Worlds page. Press one to switch to it")}
`,
        },
      ],
    },
    {
      id: 'more',
      title: 'Going further',
      note: 'Once you are used to it',
      icon: 'wrench',
      topics: [
        {
          id: 'cli',
          title: 'Everyday commands',
          sub: 'This is all you need to remember',
          icon: 'terminal',
          color: '#64748b',
          body: `
<p>Add <code>--help</code> to any command to see how to use it.</p>
<h4>Wants</h4>
<div class="code"><pre>mywant wants list                 # list them
mywant wants get coffee-break     # look closer
mywant wants create -f want.yaml  # place from a file
mywant wants delete coffee-break  # remove</pre></div>
<h4>Server</h4>
<div class="code"><pre>mywant start -D   # start
mywant ps         # status
mywant logs       # see the record
mywant stop       # stop</pre></div>
<h4>Settings</h4>
<div class="code"><pre>mywant config get                    # see the settings
mywant config set server_port 9090   # change one
mywant config set                    # set up by answering questions</pre></div>
<p>Settings are saved in <code>~/.mywant/config.yaml</code>.</p>
`,
        },
        {
          id: 'magic',
          title: 'Ask in a few words',
          sub: 'mywant magic (m)',
          icon: 'wand-sparkles',
          color: '#8b5cf6',
          body: `
<p><strong>magic</strong> places a Want from a short, fixed phrase.
For a train route, type <code>transit 荻窪 中野坂上</code>: a Want that looks up the route is placed and its result is shown.
If a Want with the same conditions is already there, nothing new is made and its result is shown instead.</p>
<div class="code"><pre>$ mywant m transit 荻窪 中野坂上
「荻窪→中野坂上」を作りました。
荻窪→中野坂上 (transit_search, want-c8ad…)
  departure: 16:49
  arrival: 16:59
  fare: 178
  legs_text: 荻窪(16:49) ─[東京メトロ丸ノ内線]→ 中野坂上(16:59)</pre></div>
<p><code>m</code> is short for <code>magic</code>. Separate the words with spaces; no quotes needed.</p>
<p>Each Want Type sets its own phrases (its label <code>magic-sentence</code>). For how to write them, see “Make one in a few words” under concepts.</p>
<h4>Take out just the values you want</h4>
<div class="code"><pre>mywant m transit 荻窪 中野坂上 --field departure        # only 16:49
mywant m transit 荻窪 中野坂上 --field departure,fare   # two of them
mywant m transit 荻窪 中野坂上 --json                   # as JSON</pre></div>
<p>If the result isn't in yet, it waits up to 15 seconds. Change that with <code>--wait 30</code>.</p>
<h4>Act on the Want it placed</h4>
<p><code>--id</code> prints only the Want's ID (without waiting for the result), to hand to another command.</p>
<div class="code"><pre>mywant wants restart $(mywant m transit 荻窪 中野坂上 --id)   # look it up again
mywant wants get $(mywant m transit 荻窪 中野坂上 --id)       # look closer</pre></div>
<h4>Complete with Tab</h4>
<p>With completion turned on, <kbd>Tab</kbd> offers the other names, then station names.</p>
<div class="code"><pre>mywant m tra⇥ → transit
mywant m transit 荻⇥ → 荻窪 (for from)
mywant m transit 荻窪 中野⇥ → 中野坂上 (for to)
mywant m transit 荻窪 中野坂上 --field dep⇥ → departure</pre></div>
<p>Set it up once, for the shell you use. It works in terminals opened afterwards.</p>
<ol class="steps">
  <li><strong>zsh</strong> (the Mac's usual terminal): add a line at the end of <code>~/.zshrc</code>
    <div class="code"><pre>echo 'source <(mywant completion zsh)' >> ~/.zshrc</pre></div>
  </li>
  <li><strong>bash</strong>: add a line to <code>~/.bashrc</code> (needs bash-completion)
    <div class="code"><pre>echo 'source <(mywant completion bash)' >> ~/.bashrc</pre></div>
  </li>
  <li><strong>fish</strong>: put the completion file in place
    <div class="code"><pre>mywant completion fish > ~/.config/fish/completions/mywant.fish</pre></div>
  </li>
</ol>
<div class="box">
  <p class="box-title"><i data-lucide="circle-help"></i>Nothing happens on Tab?</p>
  <p>In zsh, the line has to come after <code>compinit</code> in <code>~/.zshrc</code>.
  If you see <code>command not found: compdef</code>, add <code>autoload -Uz compinit && compinit</code> before it.
  The candidates come from the server, so it has to be running.</p>
</div>
`,
        },
        {
          id: 'custom',
          title: 'Add-ons',
          sub: 'Add Want Types and recipes',
          icon: 'puzzle',
          color: '#a855f7',
          body: `
<p>A <strong>Custom</strong> is an add-on pack that gives MyWant new Want Types, recipes, looks and more.
Name a GitHub repository and it is installed.</p>
<div class="code"><pre># install
mywant custom install owner/repo
# see what is installed
mywant custom list
# remove
mywant custom uninstall &lt;name&gt;</pre></div>
<div class="box">
  <p class="box-title"><i data-lucide="key-round"></i>When an API key is needed</p>
  <p>A Custom that uses an outside service may need a key. Set it like this; the server picks it up when it restarts.</p>
</div>
<div class="code"><pre>mywant config env set NAME --stdin</pre></div>
`,
        },
        {
          id: 'remote',
          title: 'Another server',
          sub: 'Switch to a server in the cloud',
          icon: 'globe',
          color: '#14b8a6',
          body: `
<p>A MyWant server can run in the cloud as well as on your Mac.
Where your commands go is switched with a <strong>context</strong> (the same idea as kubectl).</p>
<div class="code"><pre># add a destination
mywant config set-context cloud \\
  --server https://example.fly.dev \\
  --username mywant --password-env MYWANT_AUTH_PASSWORD

# switch to it
mywant config use-context cloud

# all of them, and the current one
mywant config get-contexts
mywant config current-context</pre></div>
<p>The password itself never goes in the settings file — only the name of the environment variable that holds it.</p>
<p>To send just one command somewhere else, add <code>--context local</code> (or another name).</p>
`,
        },
        {
          id: 'update',
          title: 'Update',
          sub: 'Get the newest version',
          icon: 'refresh-cw',
          color: '#10b981',
          body: `
<p>If you installed with Homebrew, this brings you up to date.</p>
<div class="code"><pre>brew update
brew upgrade mywant mywant-gui</pre></div>
<p>When it is done, restart the server and the screen.</p>
<div class="code"><pre>mywant-gui stop && mywant stop
mywant start -D && mywant-gui start -D</pre></div>
<p>Your Wants were saved, so they come back just as they were.</p>
`,
        },
        {
          id: 'trouble',
          title: 'When stuck',
          sub: 'Common stumbles',
          icon: 'life-buoy',
          color: '#6b7280',
          body: `
<h4>brew install fails with a permission error</h4>
<p>Run <code>brew trust onelittlenightmusic/mywant</code> first, then install again.</p>
<h4>The screen won't open</h4>
<p>Check that the server is running with <code>mywant ps</code>. If it has stopped, run <code>mywant start -D</code>; for just the screen, <code>mywant-gui start -D</code>.</p>
<h4>A Want failed</h4>
<p>To see where it went wrong, look at the Want's state and at the server's record.</p>
<div class="code"><pre>mywant wants get coffee-break
mywant logs</pre></div>
<h4>I want to use a different port</h4>
<p>Start it like <code>mywant start -D --port 9090</code>.</p>
<h4>Learn more</h4>
<ul>
  <li><a href="https://github.com/onelittlenightmusic/mywant" target="_blank" rel="noopener">The MyWant repository</a></li>
  <li><a href="https://github.com/onelittlenightmusic/mywant/blob/master/docs/MYWANT_CLI_USAGE.md" target="_blank" rel="noopener">The command reference</a></li>
  <li><a href="https://github.com/onelittlenightmusic/mywant-gui" target="_blank" rel="noopener">The mywant-gui repository</a></li>
</ul>
`,
        },
      ],
    },
  ],
};
