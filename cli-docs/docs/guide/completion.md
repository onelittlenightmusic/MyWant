# Shell completion

With completion on, <kbd>Tab</kbd> completes commands, flags and — where a command knows them — what goes after: want names, the aliases of a [magic sentence](/guide/magic) and the things that fill it.

```text
mywant m tra⇥                              → transit
mywant m transit 荻⇥                       → 荻窪        (from)
mywant m transit 荻窪 中野⇥                → 中野坂上    (to)
mywant m transit 荻窪 中野坂上 --field dep⇥ → departure
```

The candidates come from the server, so it has to be running (for the commands that ask it).

## Set it up

Once, for the shell you use. It works in terminals opened afterwards.

::: code-group

```sh [zsh]
echo 'source <(mywant completion zsh)' >> ~/.zshrc
```

```sh [bash]
# needs bash-completion
echo 'source <(mywant completion bash)' >> ~/.bashrc
```

```sh [fish]
mywant completion fish > ~/.config/fish/completions/mywant.fish
```

:::

::: tip Nothing happens on Tab?
In zsh, the line has to come after `compinit` in `~/.zshrc`. If you see `command not found: compdef`, put `autoload -Uz compinit && compinit` before it.
:::
