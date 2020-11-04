# Go Chain

A concept for golang to create chain programming with functions (originally created in Sep 2015)

## Concept

Create functions like

```go
init_func := func (in, out Chan) bool {...}
```

And chain them like

```go
var c chain.C_chain
c.Add(init_func(3.0, 1000))
c.Add	(queue(0.5))
c.Add	(queue(0.9))
```

## Example

```sh
# build package
cd chain
go build
cd ..

# run qnet app using chain package
go run chain_qnet.go
```