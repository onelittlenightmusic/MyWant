# Demos (moved)

This directory used to contain many `package main` demo files. To avoid multiple `package main` declarations in the same directory (which Go disallows), each demo has been moved into its own subdirectory under this folder and kept as an progressable `package main`.

How to run a demo

- Run a demo with `go run`, pointing to the demo directory or the `main.go` file, for example:

```sh
go run ./engine/demos/debug_simple
go run ./engine/demos/demo_flight
```

Top-level placeholders

The files remaining directly under `engine/demos` are simple placeholder files in `package demos_moved` that point to the new locations. This avoids having multiple `package main` files in this directory and keeps the repository buildable.

If you add a new runnable demo, please create a new subdirectory and put a single `main.go` with `package main` in it.
