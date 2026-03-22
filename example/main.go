package main

import (
	log "github.com/sirupsen/logrus"
	easy "github.com/t-tomalak/logrus-easy-formatter"

	zero "github.com/laoin114514/NovaBot"
	"github.com/laoin114514/NovaBot/driver"
	_ "github.com/laoin114514/NovaBot/example/command"
	_ "github.com/laoin114514/NovaBot/example/music"
	_ "github.com/laoin114514/NovaBot/example/napcat"
	_ "github.com/laoin114514/NovaBot/example/repeat"
)

func init() {
	log.SetFormatter(&easy.Formatter{
		TimestampFormat: "2006-01-02 15:04:05",
		LogFormat:       "[zero][%time%][%lvl%]: %msg% \n",
	})
	log.SetLevel(log.DebugLevel)
}

func main() {
	zero.RunAndBlock(&zero.Config{
		NickName:      []string{"bot"},
		CommandPrefix: "/",
		SuperUsers:    []int64{123456},
		Driver: []zero.Driver{
			// 正向 WS
			driver.NewWebSocketClient("ws://127.0.0.1:6700", ""),
			// 反向 WS
			driver.NewWebSocketServer(16, "ws://127.0.0.1:6701", ""),
			// HTTP
			driver.NewHTTPClient("http://127.0.0.1:6701", "", "http://127.0.0.1:6700", ""),
		},
	}, nil)
}
