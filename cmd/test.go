package main

import (
	"fmt"
	"math/rand"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/driver"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func main() {
	like()
	zero.RunAndBlock(&zero.Config{
		NickName:      []string{"bot"},
		CommandPrefix: "/",
		SuperUsers:    []int64{123456},
		Driver: []zero.Driver{
			driver.NewWebSocketClient("ws://127.0.0.1:3001", "laoinnb"),
		},
	}, nil)
}

func like() {
	zero.OnFullMatch("赞我").Handle(func(ctx *zero.Ctx) {
		userID := ctx.Event.UserID
		rsp, err := ctx.SendLike(userID, 10)
		if err != nil {
			fmt.Println(rsp)
			ctx.Send(message.At(userID).String() + rsp.Resp.Message)
			return
		}
		ctx.Send(message.At(userID).String() + " 已点赞，注意查收" + message.Face(rand.Intn(200)).String())
	})
}
