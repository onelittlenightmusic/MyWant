package mywant

// LocateCharacter answers where a character is standing on the canvas, in
// grid cells. The server knows (it holds every player's cursor); the want
// types do not, so the server installs it at startup. "cursor" names whoever
// is at the controls. Nil — as in a test without a server — means nobody can
// be found, and a robot told to follow them stays put.
var LocateCharacter func(target string) (x, y int, ok bool)
