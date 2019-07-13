package terminal

import (
	pl "github.com/peterh/liner"
	"io"
	"strings"
)

type pliner struct {
	state *pl.State
}

func (l *pliner) Close() error {
	return l.state.Close()
}

func (l *pliner)Prompt(s string) (string, error) {
	return l.state.Prompt(s)
}

func (l *pliner)SetTermForComplete(t *Term) {
	l.state.SetCompleter(func(line string) (c []string) {
		if strings.HasPrefix(line, "break ") || strings.HasPrefix(line, "b ") {
			filter := line[strings.Index(line, " ")+1:]
			funcs, _ := t.client.ListFunctions(filter)
			for _, f := range funcs {
				c = append(c, "break "+f)
			}
			return
		}
		for _, cmd := range t.cmds.cmds {
			for _, alias := range cmd.aliases {
				if strings.HasPrefix(alias, strings.ToLower(line)) {
					c = append(c, alias)
				}
			}
		}
		return
	})
}

func (l *pliner)ReadHistory(r io.Reader) (int, error){
	return l.state.ReadHistory(r)
}

func (l *pliner) AppendHistory(item string){
	l.state.AppendHistory(item)
}

func (l *pliner) WriteHistory(w io.Writer)(int, error) {
	return l.state.WriteHistory(w)
}

func (l *pliner) KindName()string {
	return "Pl"
}


func Newpliner () *pliner {
	return &pliner{state: pl.NewLiner()}
}