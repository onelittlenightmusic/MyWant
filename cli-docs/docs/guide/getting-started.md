# Getting started

`mywant` is the command line of MyWant: it starts and stops the server, and talks to it — placing wants, reading their answers, naming things.

## Install

On a Mac, with [Homebrew](https://brew.sh/):

```sh
brew tap onelittlenightmusic/mywant
brew trust onelittlenightmusic/mywant   # once
brew install mywant mywant-gui
```

Check it:

```sh
mywant version
```

## Start the server

```sh
mywant start -D     # in the background
mywant ps           # what is running
mywant stop
```

## Place a want and read it

```sh
mywant wants create -t weather --param at=Tokyo
mywant wants list
mywant wants get weather-instance
```

Or say it in a few words, when the want type has a [magic sentence](/guide/magic):

```sh
mywant m transit 荻窪 中野坂上
```

## Find your way around

Every command explains itself with `--help`:

```sh
mywant wants --help
mywant wants search --help
```

The [command index](/reference/) lists all of them, by group, with what running each one does: <Badge type="tip" text="reads" /> only looks, <Badge type="warning" text="changes" /> changes something, <Badge type="danger" text="destroys" /> removes something. <Badge type="info" text="canvas" /> marks the commands about what is drawn on the board.

`mywant commands --json` prints the same tree as JSON, for a script or an agent.

Next: [turn on Tab completion](/guide/completion).
