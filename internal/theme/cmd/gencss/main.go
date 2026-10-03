// Command gencss prints a theme's :root token block to stdout.
// Development helper: gencss <theme> [full]
//
//	without "full": token block only
//	with "full":    default.css + tokens.css + theme block, i.e. the shell
//	                stylesheet minus animations and user overrides
package main

import (
	"fmt"
	"os"

	"phalune/internal/shell"
	"phalune/internal/theme"
)

func main() {
	name := ""
	if len(os.Args) > 1 {
		name = os.Args[1]
	}
	th, err := theme.Get(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(os.Args) > 2 && os.Args[2] == "full" {
		fmt.Print(shell.DefaultCSS() + "\n" + shell.TokensCSS() + "\n" + th.CSS(shell.CSSDefaults, nil) + "\n")
		return
	}
	fmt.Print(th.CSS(shell.CSSDefaults, nil))
}
