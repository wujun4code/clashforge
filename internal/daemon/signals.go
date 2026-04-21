package daemon

import (
	"os"
	"os/signal"
	"syscall"
)

func Wait() os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	return <-ch
}
