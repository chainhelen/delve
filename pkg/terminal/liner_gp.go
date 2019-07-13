package terminal

import (
	Gp "github.com/c-bata/go-prompt"
	"github.com/c-bata/go-prompt/completer"
	"io"
	"strings"
)

var commands []Gp.Suggest
var t *Term


type gpliner struct {
	p *Gp.Prompt
}

func (l *gpliner) Close() error {
	return nil
}

func (l *gpliner)Prompt(s string) (string, error) {
	return l.p.Input(), nil
}

func (l *gpliner)SetTermForComplete(pt *Term) {
	t = pt

	commands = make([]Gp.Suggest, 0)
	for _, cmd := range t.cmds.cmds {
		h := cmd.helpMsg
		if idx := strings.Index(h, "\n"); idx >= 0 {
			h = h[:idx]
		}

		if len(cmd.aliases) > 1 {
			for i := len(cmd.aliases) - 1;i >= 0 ;i-- {
				commands = append(commands, Gp.Suggest{Text: cmd.aliases[i], Description: h})
			}
		} else {
			commands = append(commands, Gp.Suggest{Text: cmd.aliases[0], Description: h})
		}
	}
}

func (l *gpliner)ReadHistory(r io.Reader) (int, error){
	return 0, nil
}

func (l *gpliner) AppendHistory(item string){
	return
}

func (l *gpliner) WriteHistory(w io.Writer)(int, error) {
	return 0, nil
}

func (l *gpliner)KindName() string {
	return "Gp"
}

func completeCommand(d Gp.Document, cmd string) []Gp.Suggest {
	if len(cmd) == 0 {
		return nil
	}
	return Gp.FilterHasPrefix(commands, d.GetWordBeforeCursor(), true)
}

func completeLinespec(d Gp.Document, cmdStr string, param string) []Gp.Suggest{
	if len(strings.TrimSpace(param)) == 0 || t == nil {
		return nil
	}

	sgs := make([]Gp.Suggest, 0)

	sources,_ := t.client.ListSources(param)
	for i := 0; i < 10 && i < len(sources);i++ {
		sgs = append(sgs, Gp.Suggest{Text: sources[i], Description:"file"})
	}

	funcs, _ := t.client.ListFunctions(param)
	for i := 0;i < 10 && i < len(funcs);i++ {
		sgs = append(sgs, Gp.Suggest{Text: funcs[i], Description:"function"})
	}


	return Gp.FilterContains(sgs, d.GetWordBeforeCursor(), true)
}

func complete(d Gp.Document)[] Gp.Suggest {
	args := strings.Split(d.Text, " ")
	if len(args) == 1 {
		return completeCommand(d, args[0])
	}

	// help command
	if len(args) == 2 && (args[0] == "help"){
		helpMsgCmd := make([]Gp.Suggest, 0, len(commands))
		for _, v := range commands {
			if v.Text != "help" {
				helpMsgCmd = append(helpMsgCmd, v)
			}
		}
		return Gp.FilterHasPrefix(helpMsgCmd, d.GetWordBeforeCursor(), true)
	}

	// break [name] <linespec>
	if len(args) == 2 &&(args[0] == "b" || args[0] == "break") {
		return completeLinespec(d, args[0], args[1])
	}
	if len(args) == 3 && (args[0] == "b" || args[0] == "break") {
		return completeLinespec(d, args[0], args[2])
	}

	// list usage: [goroutine <n>] [frame <m>] list [<linespec>]
	// so enumeration:
	// 1. list [<linespec>]
	// 2. [goroutine <n>] list [<linespec>]
	// 3. [frame <m>] list [<linespec>]
	// 4. [goroutine <n>][frame <m>] list [<linespec>]
	if (len(args) == 2) && (args[0] == "list" || args[0] == "l" || args[0] == "ls") {
		return completeLinespec(d, args[0], args[1])
	}
	if (len(args) == 4) && (args[2] == "list" ||args[2] == "l" || args[2] == "ls"){
		return completeLinespec(d, args[2], args[3])
	}
	if (len(args) == 6) && (args[4] == "list" ||args[4] == "l" || args[4] == "ls"){
		return completeLinespec(d, args[4], args[5])
	}
	return nil
}

func Newgpliner() *gpliner{
	l := &gpliner{p : Gp.New(
		nil,
		complete,
		Gp.OptionTitle("dlv client"),
		Gp.OptionPrefix("(dlv) "),
		Gp.OptionInputTextColor(Gp.Yellow),
		Gp.OptionCompletionWordSeparator(completer.FilePathCompletionSeparator),
	)}


	return l
}