package command

import (
	"flag"

	zero "github.com/laoin114514/NovaBot"
	"github.com/laoin114514/NovaBot/extension/shell"
	"github.com/laoin114514/NovaBot/message"
)

func init() {
	zero.OnCommand("github").Handle(func(ctx *zero.Ctx) {
		fset := flag.FlagSet{}
		var (
			owner string
			repo  string
		)
		fset.StringVar(&owner, "o", "wdvxdr1123", "")
		fset.StringVar(&repo, "r", "ZeroBot", "")
		arguments := shell.Parse(ctx.State["args"].(string))
		err := fset.Parse(arguments)
		if err != nil {
			return
		}
		ctx.Send(message.Text("github\n" +
			"owner: " + owner + "\n" +
			"repo: " + repo,
		))
	})
}
