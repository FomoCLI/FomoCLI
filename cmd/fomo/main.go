// Command fomo is fomo.family in your terminal.
package main

import (
	"os"

	"github.com/FomoCLI/FomoCLI/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
