package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "fail":
			os.Exit(42)
		case "health-fail":
			os.Exit(1)
		case "health-ok":
			return
		case "read":
			b, e := os.ReadFile(os.Args[2])
			if e != nil {
				fmt.Println(e)
				os.Exit(2)
			}
			fmt.Print(string(b))
			return
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	<-ctx.Done()
}
