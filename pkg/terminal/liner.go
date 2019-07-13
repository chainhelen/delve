package terminal

import (
	"io"
	"strings"
)

type Liner interface {
	Close() error
	Prompt(string) (string, error)
	SetTermForComplete(t *Term)
	ReadHistory(io.Reader)(int, error)
	AppendHistory(item string)
	WriteHistory(io.Writer)(int, error)
	KindName() string
}


func newLiner(linerKind string) Liner{
	lK := strings.ToLower(strings.TrimSpace(linerKind))
	switch lK{
	case "gp":
		return Newgpliner()
	case "pl":
		return Newpliner()
	}
	return Newpliner()
}