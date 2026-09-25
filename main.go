package main

import (
	"github.com/svpc-ai/svpc/cmd"
	"github.com/svpc-ai/svpc/internal/logging"
)

func main() {
	defer logging.RecoverPanic("main", func() {
		logging.ErrorPersist("Application terminated due to unhandled panic")
	})

	cmd.Execute()
}


