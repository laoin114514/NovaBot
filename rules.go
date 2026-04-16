package nova

import (
	"hash/crc64"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/laoin114514/NovaBot/message"
	"github.com/laoin114514/NovaBot/utils/helper"
)

// Type check the ctx.Event's type
func Type(typ string) Rule {
	t := strings.SplitN(typ, "/", 3)
	return func(ctx *Ctx) bool {
		if len(t) > 0 && t[0] != ctx.Event.PostType {
			return false
		}
		if len(t) > 1 && t[1] != ctx.Event.DetailType {
			return false
		}
		if len(t) > 2 && t[2] != ctx.Event.SubType {
			return false
		}
		return true
	}
}

// PrefixRule check if the message has the prefix and trim the prefix
//
// 检查消息前缀
func PrefixRule(prefixes ...string) Rule {
	return func(ctx *Ctx) bool {
		msg := ctx.MessageString()
		if msg == "" {
			return false
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(msg, prefix) {
				ctx.State["prefix"] = prefix
				ctx.State["args"] = strings.TrimLeft(msg[len(prefix):], " ")
				return true
			}
		}
		return false
	}
}

// SuffixRule check if the message has the suffix and trim the suffix
//
// 检查消息后缀
func SuffixRule(suffixes ...string) Rule {
	return func(ctx *Ctx) bool {
		msg := ctx.MessageString()
		for _, suffix := range suffixes {
			if strings.HasSuffix(msg, suffix) {
				ctx.State["suffix"] = suffix
				arg := strings.TrimRight(msg[:len(msg)-len(suffix)], " ")
				ctx.State["args"] = arg
				return true
			}
		}
		return false
	}
}

// CommandRule check if the message is a command and trim the command name
func CommandRule(commands ...string) Rule {
	return func(ctx *Ctx) bool {
		msg := ctx.MessageString()
		if !strings.HasPrefix(msg, BotConfig.CommandPrefix) {
			return false
		}
		cmdMessage := msg[len(BotConfig.CommandPrefix):]
		for _, command := range commands {
			if strings.HasPrefix(cmdMessage, command) {
				ctx.State["command"] = command
				ctx.State["args"] = strings.TrimLeft(cmdMessage[len(command):], " ")
				return true
			}
		}
		return false
	}
}

// RegexRule check if the message can be matched by the regex pattern
func RegexRule(regexPattern string) Rule {
	regex := regexp.MustCompile(regexPattern)
	return func(ctx *Ctx) bool {
		msg := ctx.MessageString()
		if matched := regex.FindStringSubmatch(msg); matched != nil {
			ctx.State["regex_matched"] = matched
			return true
		}
		return false
	}
}

// ReplyRule check if the message is replying some message
func ReplyRule(messageID int64) Rule {
	return func(ctx *Ctx) bool {
		if len(ctx.Event.Message) == 0 {
			return false
		}
		for _, seg := range ctx.Event.Message {
			if seg.Type != "reply" {
				continue
			}
			replyID := seg.Data["id"]
			if id, err := strconv.ParseInt(replyID, 10, 64); err == nil {
				return id == messageID
			}
			c := crc64.New(crc64.MakeTable(crc64.ISO))
			c.Write(helper.StringToBytes(replyID))
			return int64(c.Sum64()) == messageID
		}
		return false
	}
}

// KeywordRule check if the message has a keyword or keywords
func KeywordRule(src ...string) Rule {
	return func(ctx *Ctx) bool {
		msg := ctx.MessageString()
		for _, str := range src {
			if strings.Contains(msg, str) {
				ctx.State["keyword"] = str
				return true
			}
		}
		return false
	}
}

// FullMatchRule check if src has the same copy of the message
func FullMatchRule(src ...string) Rule {
	return func(ctx *Ctx) bool {
		msg := ctx.MessageString()
		for _, str := range src {
			if str == msg {
				ctx.State["matched"] = msg
				return true
			}
		}
		return false
	}
}

// OnlyToMe only triggered in conditions of @bot or begin with the nicknames
func OnlyToMe(ctx *Ctx) bool {
	return ctx.Event.IsToMe
}

// CheckUser only triggered by specific person
func CheckUser(userID ...int64) Rule {
	return func(ctx *Ctx) bool {
		for _, uid := range userID {
			if ctx.Event.UserID == uid {
				return true
			}
		}
		return false
	}
}

// CheckGroup only triggered in specific group
func CheckGroup(grpID ...int64) Rule {
	return func(ctx *Ctx) bool {
		for _, gid := range grpID {
			if ctx.Event.GroupID == gid {
				return true
			}
		}
		return false
	}
}

// OnlyPrivate requires that the ctx.Event is private message
func OnlyPrivate(ctx *Ctx) bool {
	return ctx.Event.PostType == "message" && ctx.Event.DetailType == "private"
}

// OnlyPublic requires that the ctx.Event is public/group or public/guild message
func OnlyPublic(ctx *Ctx) bool {
	return ctx.Event.PostType == "message" && (ctx.Event.DetailType == "group" || ctx.Event.DetailType == "guild")
}

// OnlyGroup requires that the ctx.Event is public/group message
func OnlyGroup(ctx *Ctx) bool {
	return ctx.Event.PostType == "message" && ctx.Event.DetailType == "group"
}

// OnlyGuild requires that the ctx.Event is public/guild message
func OnlyGuild(ctx *Ctx) bool {
	return ctx.Event.PostType == "message" && ctx.Event.DetailType == "guild"
}

func issu(id int64) bool {
	for _, su := range BotConfig.SuperUsers {
		if su == id {
			return true
		}
	}
	return false
}

// SuperUserPermission only triggered by the bot's owner
func SuperUserPermission(ctx *Ctx) bool {
	return issu(ctx.Event.UserID)
}

// AdminPermission only triggered by the group admins or higher permission
func AdminPermission(ctx *Ctx) bool {
	return SuperUserPermission(ctx) || ctx.Event.Sender.Role == "owner" || ctx.Event.Sender.Role == "admin"
}

// OwnerPermission only triggered by the group owner or higher permission
func OwnerPermission(ctx *Ctx) bool {
	return SuperUserPermission(ctx) || ctx.Event.Sender.Role == "owner"
}

// UserOrGrpAdmin 允许用户单独使用或群管使用
func UserOrGrpAdmin(ctx *Ctx) bool {
	if OnlyGroup(ctx) {
		return AdminPermission(ctx)
	}
	return OnlyToMe(ctx)
}

// GroupHigherPermission 群发送者权限高于 target
//
// 隐含 OnlyGroup 判断
func GroupHigherPermission(gettarget func(ctx *Ctx) int64) Rule {
	return func(ctx *Ctx) bool {
		if !OnlyGroup(ctx) {
			return false
		}
		target := gettarget(ctx)
		if target == ctx.Event.UserID { // 特判, 自己和自己比
			return false
		}
		if SuperUserPermission(ctx) {
			sender := ctx.Event.UserID
			return BotConfig.GetFirstSuperUser(sender, target) == sender
		}
		if ctx.Event.Sender.Role == "owner" {
			info, err := ctx.GetThisGroupMemberInfo(target, false)
			if err != nil {
				return false
			}
			return !issu(target) && info.Value.Get("role").Str != "owner"
		}
		if ctx.Event.Sender.Role == "admin" {
			info, err := ctx.GetThisGroupMemberInfo(target, false)
			if err != nil {
				return false
			}
			tgtrole := info.Value.Get("role").Str
			return !issu(target) && tgtrole != "owner" && tgtrole != "admin"
		}
		return false // member is the lowest
	}
}

// HasPicture 消息含有图片返回 true
func HasPicture(ctx *Ctx) bool {
	var urls = []string{}
	for _, elem := range ctx.Event.Message {
		if elem.Type == "image" {
			if elem.Data["url"] != "" {
				urls = append(urls, elem.Data["url"])
			}
		}
	}
	if len(urls) > 0 {
		ctx.State["image_url"] = urls
		return true
	}
	return false
}

// MustProvidePicture 消息不存在图片阻塞120秒至有图片，超时返回 false
func MustProvidePicture(ctx *Ctx) bool {
	if HasPicture(ctx) {
		return true
	}
	// 没有图片就索取
	ctx.SendChain(message.Text("请发送一张图片"))
	next := NewFutureEvent("message", 999, true, ctx.CheckSession(), HasPicture).Next()
	select {
	case <-time.After(time.Second * 120):
		return false
	case newCtx := <-next:
		ctx.State["image_url"] = newCtx.State["image_url"]
		ctx.Event.MessageID = newCtx.Event.MessageID
		return true
	}
}
