package zero

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	"github.com/laoin114514/NovaBot/message"
	"github.com/laoin114514/NovaBot/utils/helper"
)

var base64Reg = regexp.MustCompile(`"type":"image","data":\{"file":"base64://[\w/\+=]+`)

// formatMessage 格式化消息数组
//
//	仅用在 log 打印
func formatMessage(msg interface{}) string {
	switch m := msg.(type) {
	case string:
		return m
	case message.CQCoder:
		return m.CQCode()
	case fmt.Stringer:
		return m.String()
	default:
		s, _ := json.Marshal(msg)
		return helper.BytesToString(base64Reg.ReplaceAllFunc(s, func(b []byte) []byte {
			buf := bytes.NewBuffer([]byte(`"type":"image","data":{"file":"`))
			b = b[40:]
			b, err := base64.StdEncoding.DecodeString(helper.BytesToString(b))
			if err != nil {
				buf.WriteString(err.Error())
			} else {
				m := md5.Sum(b)
				_, _ = hex.NewEncoder(buf).Write(m[:])
				buf.WriteString(".image")
			}
			return buf.Bytes()
		}))
	}
}

// CallAction 调用 cqhttp API
func (ctx *Ctx) CallAction(action string, params Params) (APIResponse, error) {
	c, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return ctx.CallActionWithContext(c, action, params)
}

// CallActionWithContext 使用 context 调用 cqhttp API
func (ctx *Ctx) CallActionWithContext(c context.Context, action string, params Params) (APIResponse, error) {
	rsp, err := ctx.caller.CallAPI(c, APIRequest{
		Action: action,
		Params: params,
	})
	if err != nil {
		log.Errorln("[api] 调用", action, "时出现错误: ", err)
		return rsp, err
	}
	if rsp.RetCode != 0 {
		err = fmt.Errorf("api %s failed: retcode=%d message=%s wording=%s", action, rsp.RetCode, rsp.Message, rsp.Wording)
		log.Errorln("[api] 调用", action, "时出现错误, 返回值:", rsp.RetCode, ", 信息:", rsp.Message, "解释:", rsp.Wording)
		return rsp, err
	}
	return rsp, nil
}

func (ctx *Ctx) callActionData(action string, params Params) (APIResult[gjson.Result], error) {
	rsp, err := ctx.CallAction(action, params)
	if err != nil {
		return APIResult[gjson.Result]{}, err
	}
	return APIResult[gjson.Result]{Value: rsp.Data, Resp: rsp}, nil
}


func (ctx *Ctx) callActionOnlyResult(action string, params Params) (APIResult[struct{}], error) {
	rsp, err := ctx.CallAction(action, params)
	result := APIResult[struct{}]{Value: struct{}{}, Resp: rsp}
	if err != nil {
		return result, err
	}
	return result, nil
}

// SendGroupMessage 发送群消息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#send_group_msg-%E5%8F%91%E9%80%81%E7%BE%A4%E6%B6%88%E6%81%AF
func (ctx *Ctx) SendGroupMessage(groupID int64, message interface{}) (APIResult[int64], error) {
	rsp, err := ctx.CallAction("send_group_msg", Params{ // 调用并保存返回值
		"group_id": groupID,
		"message":  message,
	})
	if err != nil {
		return APIResult[int64]{}, err
	}
	mid := rsp.Data.Get("message_id")
	if mid.Exists() {
		log.Infof("[api] 发送群消息(%v): %v (id=%v)", groupID, formatMessage(message), mid.Int())
		return APIResult[int64]{Value: mid.Int(), Resp: rsp}, nil
	}
	return APIResult[int64]{}, errors.New("missing message_id")
}

// SendPrivateMessage 发送私聊消息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#send_private_msg-%E5%8F%91%E9%80%81%E7%A7%81%E8%81%8A%E6%B6%88%E6%81%AF
func (ctx *Ctx) SendPrivateMessage(userID int64, message interface{}) (APIResult[int64], error) {
	rsp, err := ctx.CallAction("send_private_msg", Params{
		"user_id": userID,
		"message": message,
	})
	if err != nil {
		return APIResult[int64]{}, err
	}
	mid := rsp.Data.Get("message_id")
	if mid.Exists() {
		log.Infof("[api] 发送私聊消息(%v): %v (id=%v)", userID, formatMessage(message), mid.Int())
		return APIResult[int64]{Value: mid.Int(), Resp: rsp}, nil
	}
	return APIResult[int64]{}, errors.New("missing message_id")
}

// DeleteMessage 撤回消息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#delete_msg-%E6%92%A4%E5%9B%9E%E6%B6%88%E6%81%AF
//
//nolint:interfacer
func (ctx *Ctx) DeleteMessage(messageID interface{}) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("delete_msg", Params{
		"message_id": messageID,
	})
}

// GetMessage 获取消息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_msg-%E8%8E%B7%E5%8F%96%E6%B6%88%E6%81%AF
//
//nolint:interfacer
func (ctx *Ctx) GetMessage(messageID interface{}, nologreply ...bool) (APIResult[Message], error) {
	params := Params{
		"message_id": messageID,
	}
	if len(nologreply) > 0 && nologreply[0] {
		params[stateKeyNoLogMseeageID] = true
	}
	rsp, err := ctx.CallAction("get_msg", params)
	if err != nil {
		return APIResult[Message]{}, err
	}
	data := rsp.Data
	m := Message{
		Elements:    message.ParseMessage(helper.StringToBytes(data.Get("message").Raw)),
		MessageID:   message.NewMessageIDFromInteger(data.Get("message_id").Int()),
		MessageType: data.Get("message_type").String(),
		Sender:      &User{},
	}
	err = json.Unmarshal(helper.StringToBytes(data.Get("sender").Raw), m.Sender)
	if err != nil {
		return APIResult[Message]{}, err
	}
	return APIResult[Message]{Value: m, Resp: rsp}, nil
}

// GetForwardMessage 获取合并转发消息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_forward_msg-%E8%8E%B7%E5%8F%96%E5%90%88%E5%B9%B6%E8%BD%AC%E5%8F%91%E6%B6%88%E6%81%AF
func (ctx *Ctx) GetForwardMessage(id string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_forward_msg", Params{
		"id": id,
	})
}

// SendLike 发送好友赞
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#send_like-%E5%8F%91%E9%80%81%E5%A5%BD%E5%8F%8B%E8%B5%9E
func (ctx *Ctx) SendLike(userID int64, times int) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("send_like", Params{
		"user_id": userID,
		"times":   times,
	})
}

// SetGroupKick 群组踢人
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_kick-%E7%BE%A4%E7%BB%84%E8%B8%A2%E4%BA%BA
func (ctx *Ctx) SetGroupKick(groupID, userID int64, rejectAddRequest bool) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_kick", Params{
		"group_id":           groupID,
		"user_id":            userID,
		"reject_add_request": rejectAddRequest,
	})
}

// SetThisGroupKick 本群组踢人
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_kick-%E7%BE%A4%E7%BB%84%E8%B8%A2%E4%BA%BA
func (ctx *Ctx) SetThisGroupKick(userID int64, rejectAddRequest bool) (APIResult[struct{}], error) {
	return ctx.SetGroupKick(ctx.Event.GroupID, userID, rejectAddRequest)
}

// SetGroupBan 群组单人禁言
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_ban-%E7%BE%A4%E7%BB%84%E5%8D%95%E4%BA%BA%E7%A6%81%E8%A8%80
func (ctx *Ctx) SetGroupBan(groupID, userID, duration int64) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_ban", Params{
		"group_id": groupID,
		"user_id":  userID,
		"duration": duration,
	})
}

// SetThisGroupBan 本群组单人禁言
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_ban-%E7%BE%A4%E7%BB%84%E5%8D%95%E4%BA%BA%E7%A6%81%E8%A8%80
func (ctx *Ctx) SetThisGroupBan(userID, duration int64) (APIResult[struct{}], error) {
	return ctx.SetGroupBan(ctx.Event.GroupID, userID, duration)
}

// SetGroupWholeBan 群组全员禁言
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_whole_ban-%E7%BE%A4%E7%BB%84%E5%85%A8%E5%91%98%E7%A6%81%E8%A8%80
func (ctx *Ctx) SetGroupWholeBan(groupID int64, enable bool) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_whole_ban", Params{
		"group_id": groupID,
		"enable":   enable,
	})
}

// SetThisGroupWholeBan 本群组全员禁言
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_whole_ban-%E7%BE%A4%E7%BB%84%E5%85%A8%E5%91%98%E7%A6%81%E8%A8%80
func (ctx *Ctx) SetThisGroupWholeBan(enable bool) (APIResult[struct{}], error) {
	return ctx.SetGroupWholeBan(ctx.Event.GroupID, enable)
}

// SetGroupAdmin 群组设置管理员
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_whole_ban-%E7%BE%A4%E7%BB%84%E5%85%A8%E5%91%98%E7%A6%81%E8%A8%80
func (ctx *Ctx) SetGroupAdmin(groupID, userID int64, enable bool) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_admin", Params{
		"group_id": groupID,
		"user_id":  userID,
		"enable":   enable,
	})
}

// SetThisGroupAdmin 本群组设置管理员
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_whole_ban-%E7%BE%A4%E7%BB%84%E5%85%A8%E5%91%98%E7%A6%81%E8%A8%80
func (ctx *Ctx) SetThisGroupAdmin(userID int64, enable bool) (APIResult[struct{}], error) {
	return ctx.SetGroupAdmin(ctx.Event.GroupID, userID, enable)
}

// SetGroupAnonymous 群组匿名
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_anonymous-%E7%BE%A4%E7%BB%84%E5%8C%BF%E5%90%8D
func (ctx *Ctx) SetGroupAnonymous(groupID int64, enable bool) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_anonymous", Params{
		"group_id": groupID,
		"enable":   enable,
	})
}

// SetThisGroupAnonymous 群组匿名
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_anonymous-%E7%BE%A4%E7%BB%84%E5%8C%BF%E5%90%8D
func (ctx *Ctx) SetThisGroupAnonymous(enable bool) (APIResult[struct{}], error) {
	return ctx.SetGroupAnonymous(ctx.Event.GroupID, enable)
}

// SetGroupCard 设置群名片（群备注）
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_card-%E8%AE%BE%E7%BD%AE%E7%BE%A4%E5%90%8D%E7%89%87%E7%BE%A4%E5%A4%87%E6%B3%A8
func (ctx *Ctx) SetGroupCard(groupID, userID int64, card string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_card", Params{
		"group_id": groupID,
		"user_id":  userID,
		"card":     card,
	})
}

// SetThisGroupCard 设置本群名片（群备注）
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_card-%E8%AE%BE%E7%BD%AE%E7%BE%A4%E5%90%8D%E7%89%87%E7%BE%A4%E5%A4%87%E6%B3%A8
func (ctx *Ctx) SetThisGroupCard(userID int64, card string) (APIResult[struct{}], error) {
	return ctx.SetGroupCard(ctx.Event.GroupID, userID, card)
}

// SetGroupName 设置群名
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_name-%E8%AE%BE%E7%BD%AE%E7%BE%A4%E5%90%8D
func (ctx *Ctx) SetGroupName(groupID int64, groupName string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_name", Params{
		"group_id":   groupID,
		"group_name": groupName,
	})
}

// SetThisGroupName 设置本群名
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_name-%E8%AE%BE%E7%BD%AE%E7%BE%A4%E5%90%8D
func (ctx *Ctx) SetThisGroupName(groupName string) (APIResult[struct{}], error) {
	return ctx.SetGroupName(ctx.Event.GroupID, groupName)
}

// SetGroupLeave 退出群组
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_leave-%E9%80%80%E5%87%BA%E7%BE%A4%E7%BB%84
func (ctx *Ctx) SetGroupLeave(groupID int64, isDismiss bool) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_leave", Params{
		"group_id":   groupID,
		"is_dismiss": isDismiss,
	})
}

// SetThisGroupLeave 退出本群组
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_leave-%E9%80%80%E5%87%BA%E7%BE%A4%E7%BB%84
func (ctx *Ctx) SetThisGroupLeave(isDismiss bool) (APIResult[struct{}], error) {
	return ctx.SetGroupLeave(ctx.Event.GroupID, isDismiss)
}

// SetGroupSpecialTitle 设置群组专属头衔
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_special_title-%E8%AE%BE%E7%BD%AE%E7%BE%A4%E7%BB%84%E4%B8%93%E5%B1%9E%E5%A4%B4%E8%A1%94
func (ctx *Ctx) SetGroupSpecialTitle(groupID, userID int64, specialTitle string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_special_title", Params{
		"group_id":      groupID,
		"user_id":       userID,
		"special_title": specialTitle,
	})
}

// SetThisGroupSpecialTitle 设置本群组专属头衔
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_special_title-%E8%AE%BE%E7%BD%AE%E7%BE%A4%E7%BB%84%E4%B8%93%E5%B1%9E%E5%A4%B4%E8%A1%94
func (ctx *Ctx) SetThisGroupSpecialTitle(userID int64, specialTitle string) (APIResult[struct{}], error) {
	return ctx.SetGroupSpecialTitle(ctx.Event.GroupID, userID, specialTitle)
}

// SetFriendAddRequest 处理加好友请求
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_friend_add_request-%E5%A4%84%E7%90%86%E5%8A%A0%E5%A5%BD%E5%8F%8B%E8%AF%B7%E6%B1%82
func (ctx *Ctx) SetFriendAddRequest(flag string, approve bool, remark string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_friend_add_request", Params{
		"flag":    flag,
		"approve": approve,
		"remark":  remark,
	})
}

// SetGroupAddRequest 处理加群请求／邀请
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#set_group_add_request-%E5%A4%84%E7%90%86%E5%8A%A0%E7%BE%A4%E8%AF%B7%E6%B1%82%E9%82%80%E8%AF%B7
func (ctx *Ctx) SetGroupAddRequest(flag string, subType string, approve bool, reason string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_add_request", Params{
		"flag":     flag,
		"sub_type": subType,
		"approve":  approve,
		"reason":   reason,
	})
}

// GetLoginInfo 获取登录号信息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_login_info-%E8%8E%B7%E5%8F%96%E7%99%BB%E5%BD%95%E5%8F%B7%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetLoginInfo() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_login_info", Params{})
}

// GetStrangerInfo 获取陌生人信息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_stranger_info-%E8%8E%B7%E5%8F%96%E9%99%8C%E7%94%9F%E4%BA%BA%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetStrangerInfo(userID int64, noCache bool) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_stranger_info", Params{
		"user_id":  userID,
		"no_cache": noCache,
	})
}

// GetFriendList 获取好友列表
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_friend_list-%E8%8E%B7%E5%8F%96%E5%A5%BD%E5%8F%8B%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetFriendList() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_friend_list", Params{})
}

// GetGroupInfo 获取群信息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_group_info-%E8%8E%B7%E5%8F%96%E7%BE%A4%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetGroupInfo(groupID int64, noCache bool) (APIResult[Group], error) {
	rsp, err := ctx.CallAction("get_group_info", Params{
		"group_id": groupID,
		"no_cache": noCache,
	})
	if err != nil {
		return APIResult[Group]{}, err
	}
	group := Group{}
	if err = json.Unmarshal([]byte(rsp.Data.Raw), &group); err != nil {
		return APIResult[Group]{}, err
	}
	return APIResult[Group]{Value: group, Resp: rsp}, nil
}

// GetThisGroupInfo 获取本群信息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_group_info-%E8%8E%B7%E5%8F%96%E7%BE%A4%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetThisGroupInfo(noCache bool) (APIResult[Group], error) {
	return ctx.GetGroupInfo(ctx.Event.GroupID, noCache)
}

// GetGroupList 获取群列表
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_group_list-%E8%8E%B7%E5%8F%96%E7%BE%A4%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetGroupList() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_list", Params{})
}

// GetGroupMemberInfo 获取群成员信息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_group_member_info-%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%88%90%E5%91%98%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetGroupMemberInfo(groupID int64, userID int64, noCache bool) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_member_info", Params{
		"group_id": groupID,
		"user_id":  userID,
		"no_cache": noCache,
	})
}

// GetThisGroupMemberInfo 获取本群成员信息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_group_member_info-%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%88%90%E5%91%98%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetThisGroupMemberInfo(userID int64, noCache bool) (APIResult[gjson.Result], error) {
	return ctx.GetGroupMemberInfo(ctx.Event.GroupID, userID, noCache)
}

// GetGroupMemberList 获取群成员列表
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_group_member_list-%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%88%90%E5%91%98%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetGroupMemberList(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_member_list", Params{
		"group_id": groupID,
	})
}

// GetThisGroupMemberList 获取本群成员列表
func (ctx *Ctx) GetThisGroupMemberList() (APIResult[gjson.Result], error) {
	return ctx.GetGroupMemberList(ctx.Event.GroupID)
}

// GetGroupMemberListNoCache 无缓存获取群员列表
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_group_member_list-%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%88%90%E5%91%98%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetGroupMemberListNoCache(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_member_list", Params{
		"group_id": groupID,
		"no_cache": true,
	})
}

// GetThisGroupMemberListNoCache 无缓存获取本群员列表
func (ctx *Ctx) GetThisGroupMemberListNoCache() (APIResult[gjson.Result], error) {
	return ctx.GetGroupMemberListNoCache(ctx.Event.GroupID)
}

// GetGroupHonorInfo 获取群荣誉信息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_group_honor_info-%E8%8E%B7%E5%8F%96%E7%BE%A4%E8%8D%A3%E8%AA%89%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetGroupHonorInfo(groupID int64, hType string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_honor_info", Params{
		"group_id": groupID,
		"type":     hType,
	})
}

// GetThisGroupHonorInfo 获取本群荣誉信息
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_group_honor_info-%E8%8E%B7%E5%8F%96%E7%BE%A4%E8%8D%A3%E8%AA%89%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetThisGroupHonorInfo(hType string) (APIResult[gjson.Result], error) {
	return ctx.GetGroupHonorInfo(ctx.Event.GroupID, hType)
}

// GetRecord 获取语音
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_record-%E8%8E%B7%E5%8F%96%E8%AF%AD%E9%9F%B3
func (ctx *Ctx) GetRecord(file string, outFormat string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_record", Params{
		"file":       file,
		"out_format": outFormat,
	})
}

// GetImage 获取图片
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_image-%E8%8E%B7%E5%8F%96%E5%9B%BE%E7%89%87
func (ctx *Ctx) GetImage(file string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_image", Params{
		"file": file,
	})
}

// GetVersionInfo 获取运行状态
// https://github.com/botuniverse/onebot-11/blob/master/api/public.md#get_status-%E8%8E%B7%E5%8F%96%E8%BF%90%E8%A1%8C%E7%8A%B6%E6%80%81
func (ctx *Ctx) GetVersionInfo() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_version_info", Params{})
}

// Expand API

// SetGroupPortrait 设置群头像
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%AE%BE%E7%BD%AE%E7%BE%A4%E5%A4%B4%E5%83%8F
func (ctx *Ctx) SetGroupPortrait(groupID int64, file string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_portrait", Params{
		"group_id": groupID,
		"file":     file,
	})
}

// SetThisGroupPortrait 设置本群头像
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%AE%BE%E7%BD%AE%E7%BE%A4%E5%A4%B4%E5%83%8F
func (ctx *Ctx) SetThisGroupPortrait(file string) (APIResult[struct{}], error) {
	return ctx.SetGroupPortrait(ctx.Event.GroupID, file)
}

// OCRImage 图片OCR
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E5%9B%BE%E7%89%87ocr
func (ctx *Ctx) OCRImage(file string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("ocr_image", Params{
		"image": file,
	})
}

// SendGroupForwardMessage 发送合并转发(群)
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E5%9B%BE%E7%89%87ocr
func (ctx *Ctx) SendGroupForwardMessage(groupID int64, message message.Message) (APIResult[gjson.Result], error) {
	return ctx.callActionData("send_group_forward_msg", Params{
		"group_id": groupID,
		"messages": message,
	})
}

// SendPrivateForwardMessage 发送合并转发(私聊)
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E5%9B%BE%E7%89%87ocr
func (ctx *Ctx) SendPrivateForwardMessage(userID int64, message message.Message) (APIResult[gjson.Result], error) {
	return ctx.callActionData("send_private_forward_msg", Params{
		"user_id":  userID,
		"messages": message,
	})
}

// ForwardFriendSingleMessage 转发单条消息到好友
//
// https://llonebot.github.io/zh-CN/develop/extends_api
func (ctx *Ctx) ForwardFriendSingleMessage(userID int64, messageID interface{}) (APIResponse, error) {
	return ctx.CallAction("forward_friend_single_msg", Params{
		"user_id":    userID,
		"message_id": messageID,
	})
}

// ForwardGroupSingleMessage 转发单条消息到群
//
// https://llonebot.github.io/zh-CN/develop/extends_api
func (ctx *Ctx) ForwardGroupSingleMessage(groupID int64, messageID interface{}) (APIResponse, error) {
	return ctx.CallAction("forward_group_single_msg", Params{
		"group_id":   groupID,
		"message_id": messageID,
	})
}

// GetGroupSystemMessage 获取群系统消息
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E7%B3%BB%E7%BB%9F%E6%B6%88%E6%81%AF
func (ctx *Ctx) GetGroupSystemMessage() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_system_msg", Params{})
}

// MarkMessageAsRead 标记消息已读
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E6%A0%87%E8%AE%B0%E6%B6%88%E6%81%AF%E5%B7%B2%E8%AF%BB
func (ctx *Ctx) MarkMessageAsRead(messageID int64) (APIResponse, error) {
	return ctx.CallAction("mark_msg_as_read", Params{
		"message_id": messageID,
	})
}

// MarkThisMessageAsRead 标记本消息已读
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E6%A0%87%E8%AE%B0%E6%B6%88%E6%81%AF%E5%B7%B2%E8%AF%BB
func (ctx *Ctx) MarkThisMessageAsRead() (APIResponse, error) {
	return ctx.CallAction("mark_msg_as_read", Params{
		"message_id": ctx.Event.MessageID,
	})
}

// GetOnlineClients 获取当前账号在线客户端列表
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E5%BD%93%E5%89%8D%E8%B4%A6%E5%8F%B7%E5%9C%A8%E7%BA%BF%E5%AE%A2%E6%88%B7%E7%AB%AF%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetOnlineClients(noCache bool) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_online_clients", Params{
		"no_cache": noCache,
	})
}

// GetGroupAtAllRemain 获取群@全体成员剩余次数
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E5%85%A8%E4%BD%93%E6%88%90%E5%91%98%E5%89%A9%E4%BD%99%E6%AC%A1%E6%95%B0
func (ctx *Ctx) GetGroupAtAllRemain(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_at_all_remain", Params{
		"group_id": groupID,
	})
}

// GetThisGroupAtAllRemain 获取本群@全体成员剩余次数
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E5%85%A8%E4%BD%93%E6%88%90%E5%91%98%E5%89%A9%E4%BD%99%E6%AC%A1%E6%95%B0
func (ctx *Ctx) GetThisGroupAtAllRemain() (APIResult[gjson.Result], error) {
	return ctx.GetGroupAtAllRemain(ctx.Event.GroupID)
}

// GetGroupMessageHistory 获取群消息历史记录
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%B6%88%E6%81%AF%E5%8E%86%E5%8F%B2%E8%AE%B0%E5%BD%95
// https://napcat.apifox.cn/226657401e0
//
//	messageID: 起始消息序号, 可通过 get_msg 获得, 添加count和reverseOrder参数
func (ctx *Ctx) GetGroupMessageHistory(groupID, messageID, count int64, reverseOrder bool) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_msg_history", Params{
		"group_id":     groupID,
		"message_seq":  messageID, // 兼容旧版本
		"message_id":   messageID,
		"count":        count,        // 兼容napcat
		"reverseOrder": reverseOrder, // 兼容napcat
	})
}

// GettLatestGroupMessageHistory 获取最新群消息历史记录
func (ctx *Ctx) GetLatestGroupMessageHistory(groupID, count int64, reverseOrder bool) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_msg_history", Params{
		"group_id":     groupID,
		"count":        count,        // 兼容napcat
		"reverseOrder": reverseOrder, // 兼容napcat
	})
}

// GetThisGroupMessageHistory 获取本群消息历史记录
//
//	messageID: 起始消息序号, 可通过 get_msg 获得
func (ctx *Ctx) GetThisGroupMessageHistory(messageID, count int64, reverseOrder bool) (APIResult[gjson.Result], error) {
	return ctx.GetGroupMessageHistory(ctx.Event.GroupID, messageID, count, reverseOrder)
}

// GettLatestThisGroupMessageHistory 获取最新本群消息历史记录
func (ctx *Ctx) GetLatestThisGroupMessageHistory(count int64, reverseOrder bool) (APIResult[gjson.Result], error) {
	return ctx.GetLatestGroupMessageHistory(ctx.Event.GroupID, count, reverseOrder)
}

// GetGroupEssenceMessageList 获取群精华消息列表
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%B2%BE%E5%8D%8E%E6%B6%88%E6%81%AF%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetGroupEssenceMessageList(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_essence_msg_list", Params{
		"group_id": groupID,
	})
}

// GetThisGroupEssenceMessageList 获取本群精华消息列表
func (ctx *Ctx) GetThisGroupEssenceMessageList() (APIResult[gjson.Result], error) {
	return ctx.GetGroupEssenceMessageList(ctx.Event.GroupID)
}

// SetGroupEssenceMessage 设置群精华消息
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%AE%BE%E7%BD%AE%E7%B2%BE%E5%8D%8E%E6%B6%88%E6%81%AF
func (ctx *Ctx) SetGroupEssenceMessage(messageID int64) (APIResponse, error) {
	return ctx.CallAction("set_essence_msg", Params{
		"message_id": messageID,
	})
}

// DeleteGroupEssenceMessage 移出群精华消息
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E7%A7%BB%E5%87%BA%E7%B2%BE%E5%8D%8E%E6%B6%88%E6%81%AF
func (ctx *Ctx) DeleteGroupEssenceMessage(messageID int64) (APIResponse, error) {
	return ctx.CallAction("delete_essence_msg", Params{
		"message_id": messageID,
	})
}

// GetWordSlices 获取中文分词
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E4%B8%AD%E6%96%87%E5%88%86%E8%AF%8D
func (ctx *Ctx) GetWordSlices(content string) (APIResult[gjson.Result], error) {
	return ctx.callActionData(".get_word_slices", Params{
		"content": content,
	})
}

// SendGuildChannelMessage 发送频道消息
func (ctx *Ctx) SendGuildChannelMessage(guildID, channelID string, message interface{}) (APIResult[string], error) {
	rsp, err := ctx.CallAction("send_guild_channel_msg", Params{
		"guild_id":   guildID,
		"channel_id": channelID,
		"message":    message,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	mid := rsp.Data.Get("message_id")
	if mid.Exists() {
		log.Infof("[api] 发送频道消息(%v-%v): %v (id=%v)", guildID, channelID, formatMessage(message), mid.Int())
		return APIResult[string]{Value: mid.String(), Resp: rsp}, nil
	}
	return APIResult[string]{}, errors.New("missing message_id")
}

// NickName 从 args/at 获取昵称，如果都没有则获取发送者的昵称
func (ctx *Ctx) NickName() (name string) {
	name = ctx.State["args"].(string)
	if len(ctx.Event.Message) > 1 && ctx.Event.Message[1].Type == "at" {
		qq, _ := strconv.ParseInt(ctx.Event.Message[1].Data["qq"], 10, 64)
		if info, err := ctx.GetGroupMemberInfo(ctx.Event.GroupID, qq, false); err == nil {
			name = info.Value.Get("nickname").Str
		}
	} else if name == "" {
		name = ctx.Event.Sender.NickName
	}
	return
}

// CardOrNickName 从 uid 获取群名片，如果没有则获取昵称
func (ctx *Ctx) CardOrNickName(uid int64) (name string) {
	if info, err := ctx.GetGroupMemberInfo(ctx.Event.GroupID, uid, false); err == nil {
		name = info.Value.Get("card").String()
	}
	if name == "" {
		if info, err := ctx.GetStrangerInfo(uid, false); err == nil {
			name = info.Value.Get("nickname").String()
		}
	}
	return
}

// GetGroupFilesystemInfo 获取群文件系统信息
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%96%87%E4%BB%B6%E7%B3%BB%E7%BB%9F%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetGroupFilesystemInfo(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_file_system_info", Params{
		"group_id": groupID,
	})
}

// GetThisGroupFilesystemInfo 获取本群文件系统信息
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%96%87%E4%BB%B6%E7%B3%BB%E7%BB%9F%E4%BF%A1%E6%81%AF
func (ctx *Ctx) GetThisGroupFilesystemInfo() (APIResult[gjson.Result], error) {
	return ctx.GetGroupFilesystemInfo(ctx.Event.GroupID)
}

// GetGroupRootFiles 获取群根目录文件列表
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%A0%B9%E7%9B%AE%E5%BD%95%E6%96%87%E4%BB%B6%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetGroupRootFiles(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_root_files", Params{
		"group_id": groupID,
	})
}

// GetThisGroupRootFiles 获取本群根目录文件列表
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%A0%B9%E7%9B%AE%E5%BD%95%E6%96%87%E4%BB%B6%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetThisGroupRootFiles() (APIResult[gjson.Result], error) {
	return ctx.GetGroupRootFiles(ctx.Event.GroupID)
}

// GetGroupFilesByFolder 获取群子目录文件列表
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E5%AD%90%E7%9B%AE%E5%BD%95%E6%96%87%E4%BB%B6%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetGroupFilesByFolder(groupID int64, folderID string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_files_by_folder", Params{
		"group_id":  groupID,
		"folder_id": folderID,
	})
}

// GetThisGroupFilesByFolder 获取本群子目录文件列表
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E5%AD%90%E7%9B%AE%E5%BD%95%E6%96%87%E4%BB%B6%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetThisGroupFilesByFolder(folderID string) (APIResult[gjson.Result], error) {
	return ctx.GetGroupFilesByFolder(ctx.Event.GroupID, folderID)
}

// GetGroupFileURL 获取群文件资源链接
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%96%87%E4%BB%B6%E8%B5%84%E6%BA%90%E9%93%BE%E6%8E%A5
func (ctx *Ctx) GetGroupFileURL(groupID, busid int64, fileID string) (APIResult[string], error) {
	dataRes, err := ctx.callActionData("get_group_file_url", Params{
		"group_id": groupID,
		"file_id":  fileID,
		"busid":    busid,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.Get("url").Str, Resp: dataRes.Resp}, nil
}

// GetThisGroupFileURL 获取本群文件资源链接
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E8%8E%B7%E5%8F%96%E7%BE%A4%E6%96%87%E4%BB%B6%E8%B5%84%E6%BA%90%E9%93%BE%E6%8E%A5
func (ctx *Ctx) GetThisGroupFileURL(busid int64, fileID string) (APIResult[string], error) {
	return ctx.GetGroupFileURL(ctx.Event.GroupID, busid, fileID)
}

// UploadGroupFile 上传群文件
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E4%B8%8A%E4%BC%A0%E7%BE%A4%E6%96%87%E4%BB%B6
//
//	msg: FILE_NOT_FOUND FILE_SYSTEM_UPLOAD_API_ERROR ...
func (ctx *Ctx) UploadGroupFile(groupID int64, file, name, folder string) (APIResponse, error) {
	return ctx.CallAction("upload_group_file", Params{
		"group_id": groupID,
		"file":     file,
		"name":     name,
		"folder":   folder,
	})
}

// UploadThisGroupFile 上传本群文件
// https://github.com/Mrs4s/go-cqhttp/blob/master/docs/cqhttp.md#%E4%B8%8A%E4%BC%A0%E7%BE%A4%E6%96%87%E4%BB%B6
//
//	msg: FILE_NOT_FOUND FILE_SYSTEM_UPLOAD_API_ERROR ...
func (ctx *Ctx) UploadThisGroupFile(file, name, folder string) (APIResponse, error) {
	return ctx.UploadGroupFile(ctx.Event.GroupID, file, name, folder)
}

// SetMyAvatar 设置我的头像
//
// https://llonebot.github.io/zh-CN/develop/extends_api
func (ctx *Ctx) SetMyAvatar(file string) (APIResponse, error) {
	return ctx.CallAction("set_qq_avatar", Params{
		"file": file,
	})
}

// GetFile 下载收到的群文件或私聊文件
//
// https://llonebot.github.io/zh-CN/develop/extends_api
func (ctx *Ctx) GetFile(fileID string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_file", Params{
		"file_id": fileID,
	})
}

// SetMessageEmojiLike 发送表情回应
//
// https://llonebot.github.io/zh-CN/develop/extends_api
//
// emoji_id 参考 https://bot.q.qq.com/wiki/develop/api-v2/openapi/emoji/model.html#EmojiType
func (ctx *Ctx) SetMessageEmojiLike(messageID interface{}, emojiID rune) error {
	dataRes, err := ctx.callActionData("set_msg_emoji_like", Params{
		"message_id": messageID,
		"emoji_id":   strconv.Itoa(int(emojiID)),
	})
	if err != nil {
		return err
	}
	ret := dataRes.Value.Get("errMsg").Str
	if ret != "" {
		return errors.New(ret)
	}
	return nil
}

// SetGroupSign 群签到
//
// https://napneko.github.io/develop/api/doc#set-group-sign-%E7%BE%A4%E7%AD%BE%E5%88%B0
func (ctx *Ctx) SetGroupSign(groupID int64) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_sign", Params{
		"group_id": groupID,
	})
}

// GroupPoke 群聊戳一戳
//
// https://napneko.github.io/develop/api/doc#group-poke-%E7%BE%A4%E8%81%8A%E6%88%B3%E4%B8%80%E6%88%B3
func (ctx *Ctx) GroupPoke(groupID, userID int64) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("group_poke", Params{
		"group_id": groupID,
		"user_id":  userID,
	})
}

// FriendPoke 私聊戳一戳
//
// https://napneko.github.io/develop/api/doc#friend-poke-%E7%A7%81%E8%81%8A%E6%88%B3%E4%B8%80%E6%88%B3
func (ctx *Ctx) FriendPoke(userID int64) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("friend_poke", Params{
		"user_id": userID,
	})
}

// ArkSharePeer 获取推荐好友/群聊卡片
//
// c
func (ctx *Ctx) ArkSharePeer(userID, groupID string) (APIResult[string], error) {
	dataRes, err := ctx.callActionData("ArkSharePeer", Params{
		"user_id":  userID,
		"group_id": groupID,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.Get("arkJson").String(), Resp: dataRes.Resp}, nil
}

// ArkShareGroup 获取推荐群聊卡片
//
// https://napneko.github.io/develop/api/doc#arksharegroup-%E8%8E%B7%E5%8F%96%E6%8E%A8%E8%8D%90%E7%BE%A4%E8%81%8A%E5%8D%A1%E7%89%87
func (ctx *Ctx) ArkShareGroup(groupID string) (APIResult[string], error) {
	dataRes, err := ctx.callActionData("ArkShareGroup", Params{
		"group_id": groupID,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.String(), Resp: dataRes.Resp}, nil
}

// GetRobotUinRange 获取机器人账号范围
//
// https://napneko.github.io/develop/api/doc#get-robot-uin-range-%E8%8E%B7%E5%8F%96%E6%9C%BA%E5%99%A8%E4%BA%BA%E8%B4%A6%E5%8F%B7%E8%8C%83%E5%9B%B4
func (ctx *Ctx) GetRobotUinRange() (start, end int64, err error) {
	dataRes, err := ctx.callActionData("get_robot_uin_range", Params{})
	if err != nil {
		return 0, 0, err
	}
	arr := dataRes.Value.Array()
	if len(arr) != 2 {
		return 0, 0, errors.New("invalid robot uin range response")
	}
	start = arr[0].Int()
	end = arr[1].Int()
	return start, end, nil
}

// SetOnlineStatus 设置在线状态
//
// https://napneko.github.io/develop/api/doc#set-online-status-%E8%AE%BE%E7%BD%AE%E5%9C%A8%E7%BA%BF%E7%8A%B6%E6%80%81
func (ctx *Ctx) SetOnlineStatus(status, extStatus, batteryStatus int) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_online_status", Params{
		"status":         status,
		"ext_status":     extStatus,
		"battery_status": batteryStatus,
	})
}

// GetFriendsWithCategory 获取分类的好友列表
//
// https://napneko.github.io/develop/api/doc#get-friends-with-category-%E8%8E%B7%E5%8F%96%E5%88%86%E7%B1%BB%E7%9A%84%E5%A5%BD%E5%8F%8B%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetFriendsWithCategory() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_friends_with_category", Params{})
}

// TranslateEn2Zh 英译中
//
// https://napneko.github.io/develop/api/doc#translate-en2zh-%E8%8B%B1%E8%AF%91%E4%B8%AD
func (ctx *Ctx) TranslateEn2Zh(words []string) (APIResult[[]string], error) {
	dataRes, err := ctx.callActionData("translate_en2zh", Params{
		"words": words,
	})
	if err != nil {
		return APIResult[[]string]{}, err
	}
	arr := dataRes.Value.Array()
	result := make([]string, len(arr))
	for i, v := range arr {
		result[i] = v.String()
	}
	return APIResult[[]string]{Value: result, Resp: dataRes.Resp}, nil
}

// SendForwardMessage 发送合并转发
//
// https://napneko.github.io/develop/api/doc#send-forward-msg-%E5%8F%91%E9%80%81%E5%90%88%E5%B9%B6%E8%BD%AC%E5%8F%91
func (ctx *Ctx) SendForwardMessage(messageType string, userID, groupID int64, messages message.Message) (messageID int64, resID string, err error) {
	dataRes, err := ctx.callActionData("send_forward_msg", Params{
		"message_type": messageType,
		"user_id":      userID,
		"group_id":     groupID,
		"messages":     messages,
	})
	if err != nil {
		return 0, "", err
	}
	return dataRes.Value.Get("message_id").Int(), dataRes.Value.Get("res_id").String(), nil
}

// MarkPrivateMessageAsRead 设置私聊已读
//
// https://napneko.github.io/develop/api/doc#mark-private-msg-as-read-%E8%AE%BE%E7%BD%AE%E7%A7%81%E8%81%8A%E5%B7%B2%E8%AF%BB
func (ctx *Ctx) MarkPrivateMessageAsRead(userID int64) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("mark_private_msg_as_read", Params{
		"user_id": userID,
	})
}

// MarkGroupMessageAsRead 设置群聊已读
//
// https://napneko.github.io/develop/api/doc#mark-group-msg-as-read-%E8%AE%BE%E7%BD%AE%E7%BE%A4%E8%81%8A%E5%B7%B2%E8%AF%BB
func (ctx *Ctx) MarkGroupMessageAsRead(groupID int64) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("mark_group_msg_as_read", Params{
		"group_id": groupID,
	})
}

// GetFriendMessageHistory 获取私聊历史记录
//
// https://napneko.github.io/develop/api/doc#get-friend-msg-history-%E8%8E%B7%E5%8F%96%E7%A7%81%E8%81%8A%E5%8E%86%E5%8F%B2%E8%AE%B0%E5%BD%95
func (ctx *Ctx) GetFriendMessageHistory(userID, messageSeq string, count int, reverseOrder bool) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_friend_msg_history", Params{
		"user_id":      userID,
		"message_seq":  messageSeq,
		"count":        count,
		"reverseOrder": reverseOrder,
	})
}

// CreateCollection 创建收藏
//
// https://napneko.github.io/develop/api/doc#create-collection-%E5%88%9B%E5%BB%BA%E6%94%B6%E8%97%8F
func (ctx *Ctx) CreateCollection() (APIResult[gjson.Result], error) {
	return ctx.callActionData("create_collection", Params{})
}

// GetCollectionList 获取收藏
//
// https://napneko.github.io/develop/api/doc#get-collection-list-%E8%8E%B7%E5%8F%96%E6%94%B6%E8%97%8F
func (ctx *Ctx) GetCollectionList() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_collection_list", Params{})
}

// SetSelfLongNick 设置签名
//
// https://napneko.github.io/develop/api/doc#set-self-longnick-%E8%AE%BE%E7%BD%AE%E7%AD%BE%E5%90%8D
func (ctx *Ctx) SetSelfLongNick(longNick string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("set_self_longnick", Params{
		"longNick": longNick,
	})
}

// GetRecentContact 获取私聊历史记录
//
// https://napneko.github.io/develop/api/doc#get-recent-contact-%E8%8E%B7%E5%8F%96%E7%A7%81%E8%81%8A%E5%8E%86%E5%8F%B2%E8%AE%B0%E5%BD%95
func (ctx *Ctx) GetRecentContact(count int) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_recent_contact", Params{
		"count": count,
	})
}

// MarkAllAsRead 标记所有已读
//
// https://napneko.github.io/develop/api/doc#_mark-all-as-read-%E6%A0%87%E8%AE%B0%E6%89%80%E6%9C%89%E5%B7%B2%E8%AF%BB
func (ctx *Ctx) MarkAllAsRead() (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("_mark_all_as_read", Params{})
}

// GetProfileLike 获取自身点赞列表
//
// https://napneko.github.io/develop/api/doc#get-profile-like-%E8%8E%B7%E5%8F%96%E8%87%AA%E8%BA%AB%E7%82%B9%E8%B5%9E%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetProfileLike() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_profile_like", Params{})
}

// FetchCustomFace 获取自定义表情
//
// https://napneko.github.io/develop/api/doc#fetch-custom-face-%E8%8E%B7%E5%8F%96%E8%87%AA%E5%AE%9A%E4%B9%89%E8%A1%A8%E6%83%85
func (ctx *Ctx) FetchCustomFace(count int) (APIResult[gjson.Result], error) {
	return ctx.callActionData("fetch_custom_face", Params{
		"count": count,
	})
}

// GetAIRecord AI文字转语音
//
// https://napneko.github.io/develop/api/doc#get-ai-record-ai%E6%96%87%E5%AD%97%E8%BD%AC%E8%AF%AD%E9%9F%B3
func (ctx *Ctx) GetAIRecord(character string, groupID int64, text string) (APIResult[string], error) {
	dataRes, err := ctx.callActionData("get_ai_record", Params{
		"character": character,
		"group_id":  groupID,
		"text":      text,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.String(), Resp: dataRes.Resp}, nil
}

// GetAICharacters 获取AI语音角色列表
//
// https://napneko.github.io/develop/api/doc#get-ai-characters-%E8%8E%B7%E5%8F%96ai%E8%AF%AD%E9%9F%B3%E8%A7%92%E8%89%B2%E5%88%97%E8%A1%A8
func (ctx *Ctx) GetAICharacters(groupID int64, chatType int) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_ai_characters", Params{
		"group_id":  groupID,
		"chat_type": chatType,
	})
}

// SendGroupAIRecord 群聊发送AI语音
//
// https://napneko.github.io/develop/api/doc#send-group-ai-record-%E7%BE%A4%E8%81%8A%E5%8F%91%E9%80%81ai%E8%AF%AD%E9%9F%B3
func (ctx *Ctx) SendGroupAIRecord(character string, groupID int64, text string) (APIResult[string], error) {
	dataRes, err := ctx.callActionData("send_group_ai_record", Params{
		"character": character,
		"group_id":  groupID,
		"text":      text,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.Get("message_id").String(), Resp: dataRes.Resp}, nil
}

// SendPoke 群聊/私聊戳一戳
//
// https://napneko.github.io/develop/api/doc#send-poke-%E7%BE%A4%E8%81%8A-%E7%A7%81%E8%81%8A%E6%88%B3%E4%B8%80%E6%88%B3
func (ctx *Ctx) SendPoke(groupID, userID int64) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("send_poke", Params{
		"group_id": groupID,
		"user_id":  userID,
	})
}

// ═══════════════════════════════════════════════════════════
// NapCat 补充 API — 基于 https://napcat.apifox.cn
// ═══════════════════════════════════════════════════════════

// ── 文件操作（NapCat 扩展）──

// UploadPrivateFile 上传私聊文件
//
// https://napcat.apifox.cn/226658883e0.md
func (ctx *Ctx) UploadPrivateFile(userID int64, file, name string) (APIResult[string], error) {
	dataRes, err := ctx.callActionData("upload_private_file", Params{
		"user_id": userID,
		"file":    file,
		"name":    name,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.Get("file_id").Str, Resp: dataRes.Resp}, nil
}

// DeleteGroupFile 删除群文件
//
// https://napcat.apifox.cn/226658755e0.md
func (ctx *Ctx) DeleteGroupFile(groupID int64, fileID string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("delete_group_file", Params{
		"group_id": groupID,
		"file_id":  fileID,
	})
}

// DeleteThisGroupFile 删除本群文件
func (ctx *Ctx) DeleteThisGroupFile(fileID string) (APIResult[struct{}], error) {
	return ctx.DeleteGroupFile(ctx.Event.GroupID, fileID)
}

// CreateGroupFileFolder 创建群文件目录
//
// https://napcat.apifox.cn/226658773e0.md
func (ctx *Ctx) CreateGroupFileFolder(groupID int64, folderName string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("create_group_file_folder", Params{
		"group_id":    groupID,
		"folder_name": folderName,
	})
}

// CreateThisGroupFileFolder 创建本群文件目录
func (ctx *Ctx) CreateThisGroupFileFolder(folderName string) (APIResult[gjson.Result], error) {
	return ctx.CreateGroupFileFolder(ctx.Event.GroupID, folderName)
}

// DeleteGroupFileFolder 删除群文件目录
//
// https://napcat.apifox.cn/226658779e0.md
func (ctx *Ctx) DeleteGroupFileFolder(groupID int64, folderID string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("delete_group_folder", Params{
		"group_id":  groupID,
		"folder_id": folderID,
	})
}

// DeleteThisGroupFileFolder 删除本群文件目录
func (ctx *Ctx) DeleteThisGroupFileFolder(folderID string) (APIResult[struct{}], error) {
	return ctx.DeleteGroupFileFolder(ctx.Event.GroupID, folderID)
}

// DownloadFile 下载文件到本地临时目录
//
// https://napcat.apifox.cn/226658887e0.md
func (ctx *Ctx) DownloadFile(url, name, headers string) (APIResult[string], error) {
	dataRes, err := ctx.callActionData("download_file", Params{
		"url":     url,
		"name":    name,
		"headers": headers,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.Get("file").Str, Resp: dataRes.Resp}, nil
}

// GetPrivateFileURL 获取私聊文件下载链接
//
// https://napcat.apifox.cn/266151849e0.md
func (ctx *Ctx) GetPrivateFileURL(fileID string) (APIResult[string], error) {
	dataRes, err := ctx.callActionData("get_private_file_url", Params{
		"file_id": fileID,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.Get("url").Str, Resp: dataRes.Resp}, nil
}

// MoveGroupFile 移动群文件
//
// https://napcat.apifox.cn/283136359e0.md
func (ctx *Ctx) MoveGroupFile(groupID int64, fileID, parentFolderID, targetFolderID string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("move_group_file", Params{
		"group_id":         groupID,
		"file_id":          fileID,
		"parent_folder_id": parentFolderID,
		"target_folder_id": targetFolderID,
	})
}

// RenameGroupFile 重命名群文件
//
// https://napcat.apifox.cn/283136375e0.md
func (ctx *Ctx) RenameGroupFile(groupID int64, fileID, newName string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("rename_group_file", Params{
		"group_id": groupID,
		"file_id":  fileID,
		"new_name": newName,
	})
}

// TransGroupFile 传输群文件
//
// https://napcat.apifox.cn/283136366e0.md
func (ctx *Ctx) TransGroupFile(groupID int64, fileID, targetGroupID string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("trans_group_file", Params{
		"group_id":        groupID,
		"file_id":         fileID,
		"target_group_id": targetGroupID,
	})
}

// ── 群公告（NapCat 扩展）──

// SendGroupNotice 发送群公告
//
// https://napcat.apifox.cn/226658740e0.md
func (ctx *Ctx) SendGroupNotice(groupID int64, content, image string, pinned int) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("_send_group_notice", Params{
		"group_id": groupID,
		"content":  content,
		"image":    image,
		"pinned":   pinned,
	})
}

// SendThisGroupNotice 发送本群公告
func (ctx *Ctx) SendThisGroupNotice(content, image string, pinned int) (APIResult[struct{}], error) {
	return ctx.SendGroupNotice(ctx.Event.GroupID, content, image, pinned)
}

// GetGroupNotice 获取群公告列表
//
// https://napcat.apifox.cn/226658742e0.md
func (ctx *Ctx) GetGroupNotice(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("_get_group_notice", Params{
		"group_id": groupID,
	})
}

// GetThisGroupNotice 获取本群公告列表
func (ctx *Ctx) GetThisGroupNotice() (APIResult[gjson.Result], error) {
	return ctx.GetGroupNotice(ctx.Event.GroupID)
}

// DeleteGroupNotice 删除群公告
//
// https://napcat.apifox.cn/226659240e0.md
func (ctx *Ctx) DeleteGroupNotice(groupID int64, noticeID string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("_del_group_notice", Params{
		"group_id":  groupID,
		"notice_id": noticeID,
	})
}

// DeleteThisGroupNotice 删除本群公告
func (ctx *Ctx) DeleteThisGroupNotice(noticeID string) (APIResult[struct{}], error) {
	return ctx.DeleteGroupNotice(ctx.Event.GroupID, noticeID)
}

// ── 好友管理（NapCat 扩展）──

// DeleteFriend 删除好友
//
// https://napcat.apifox.cn/227237873e0.md
func (ctx *Ctx) DeleteFriend(userID int64, tempBlock, tempBothDel bool) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("delete_friend", Params{
		"user_id":       userID,
		"temp_block":    tempBlock,
		"temp_both_del": tempBothDel,
	})
}

// SetFriendRemark 设置好友备注
//
// https://napcat.apifox.cn/298305173e0.md
func (ctx *Ctx) SetFriendRemark(userID int64, remark string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_friend_remark", Params{
		"user_id": userID,
		"remark":  remark,
	})
}

// GetUnidirectionalFriendList 获取单向好友列表
//
// https://napcat.apifox.cn/266151878e0.md
func (ctx *Ctx) GetUnidirectionalFriendList() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_unidirectional_friend_list", Params{})
}

// GetDoubtFriendsAddRequest 获取可疑好友申请列表
//
// https://napcat.apifox.cn/289565516e0.md
func (ctx *Ctx) GetDoubtFriendsAddRequest() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_doubt_friends_add_request", Params{})
}

// SetDoubtFriendsAddRequest 处理可疑好友申请
//
// https://napcat.apifox.cn/289565525e0.md
func (ctx *Ctx) SetDoubtFriendsAddRequest(flag string, approve bool, remark string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_doubt_friends_add_request", Params{
		"flag":    flag,
		"approve": approve,
		"remark":  remark,
	})
}

// ── 用户资料（NapCat 扩展）──

// SetQQProfile 设置QQ资料（昵称、个性签名、性别）
//
// https://napcat.apifox.cn/226657374e0.md
//
// sex: 0=未知, 1=男, 2=女
func (ctx *Ctx) SetQQProfile(nickname, personalNote string, sex int) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_qq_profile", Params{
		"nickname":      nickname,
		"personal_note": personalNote,
		"sex":           sex,
	})
}

// SetInputStatus 设置输入状态
//
// https://napcat.apifox.cn/226659225e0.md
//
// eventType: 事件类型
func (ctx *Ctx) SetInputStatus(userID int64, eventType int) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_input_status", Params{
		"user_id":    userID,
		"event_type": eventType,
	})
}

// GetUserStatus 获取用户在线状态
//
// https://napcat.apifox.cn/226659292e0.md
//
// 返回 data: status(在线状态), ext_status(扩展状态)
func (ctx *Ctx) GetUserStatus(userID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("nc_get_user_status", Params{
		"user_id": userID,
	})
}

// SetCustomOnlineStatus 设置自定义在线状态
//
// https://napcat.apifox.cn/266151905e0.md
func (ctx *Ctx) SetCustomOnlineStatus(faceID int, faceType int, wording string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_custom_online_status", Params{
		"face_id":   faceID,
		"face_type": faceType,
		"wording":   wording,
	})
}

// ── 群管理扩展（NapCat 扩展）──

// GetGroupShutList 获取群禁言列表
//
// https://napcat.apifox.cn/226659300e0.md
func (ctx *Ctx) GetGroupShutList(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_shut_list", Params{
		"group_id": groupID,
	})
}

// GetThisGroupShutList 获取本群禁言列表
func (ctx *Ctx) GetThisGroupShutList() (APIResult[gjson.Result], error) {
	return ctx.GetGroupShutList(ctx.Event.GroupID)
}

// GetGroupInfoEx 获取群详细信息（扩展）
//
// https://napcat.apifox.cn/226659229e0.md
func (ctx *Ctx) GetGroupInfoEx(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_info_ex", Params{
		"group_id": groupID,
	})
}

// GetThisGroupInfoEx 获取本群详细信息（扩展）
func (ctx *Ctx) GetThisGroupInfoEx() (APIResult[gjson.Result], error) {
	return ctx.GetGroupInfoEx(ctx.Event.GroupID)
}

// SetGroupRemark 设置群备注
//
// https://napcat.apifox.cn/283136268e0.md
func (ctx *Ctx) SetGroupRemark(groupID int64, remark string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_remark", Params{
		"group_id": groupID,
		"remark":   remark,
	})
}

// SetThisGroupRemark 设置本群备注
func (ctx *Ctx) SetThisGroupRemark(remark string) (APIResult[struct{}], error) {
	return ctx.SetGroupRemark(ctx.Event.GroupID, remark)
}

// GetGroupIgnoredNotifies 获取群忽略通知（被忽略的入群申请和邀请）
//
// https://napcat.apifox.cn/226659323e0.md
func (ctx *Ctx) GetGroupIgnoredNotifies(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_ignored_notifies", Params{
		"group_id": groupID,
	})
}

// GetThisGroupIgnoredNotifies 获取本群忽略通知
func (ctx *Ctx) GetThisGroupIgnoredNotifies() (APIResult[gjson.Result], error) {
	return ctx.GetGroupIgnoredNotifies(ctx.Event.GroupID)
}

// SetGroupAddOption 设置群加群选项
//
// https://napcat.apifox.cn/301542178e0.md
func (ctx *Ctx) SetGroupAddOption(groupID int64, addOption int) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_add_option", Params{
		"group_id":   groupID,
		"add_option": addOption,
	})
}

// SetGroupSearchOption 设置群搜索选项
//
// https://napcat.apifox.cn/301542170e0.md
func (ctx *Ctx) SetGroupSearchOption(groupID int64, enabled bool) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_search_option", Params{
		"group_id": groupID,
		"enabled":  enabled,
	})
}

// GroupKickBatch 批量踢出群成员
//
// https://napcat.apifox.cn/301542209e0.md
func (ctx *Ctx) GroupKickBatch(groupID int64, userIDs []int64, rejectAddRequest bool) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_kick", Params{
		"group_id":           groupID,
		"user_ids":           userIDs,
		"reject_add_request": rejectAddRequest,
	})
}

// SetGroupTodo 设置群待办
//
// https://napcat.apifox.cn/395460568e0.md
func (ctx *Ctx) SetGroupTodo(groupID, messageID int64) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_group_todo", Params{
		"group_id":   groupID,
		"message_id": messageID,
	})
}

// SetThisGroupTodo 设置本群待办
func (ctx *Ctx) SetThisGroupTodo(messageID int64) (APIResult[struct{}], error) {
	return ctx.SetGroupTodo(ctx.Event.GroupID, messageID)
}

// ── 群相册（NapCat 扩展）──

// GetGroupAlbumList 获取群相册列表
//
// https://napcat.apifox.cn/395460287e0.md
func (ctx *Ctx) GetGroupAlbumList(groupID int64) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_album_list", Params{
		"group_id": groupID,
	})
}

// GetGroupAlbumMediaList 获取群相册媒体列表
//
// https://napcat.apifox.cn/395459066e0.md
func (ctx *Ctx) GetGroupAlbumMediaList(groupID int64, albumID string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_group_album_media_list", Params{
		"group_id": groupID,
		"album_id": albumID,
	})
}

// UploadGroupAlbum 上传图片到群相册
//
// https://napcat.apifox.cn/395459739e0.md
func (ctx *Ctx) UploadGroupAlbum(groupID int64, albumID, file string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("upload_group_album", Params{
		"group_id": groupID,
		"album_id": albumID,
		"file":     file,
	})
}

// DeleteGroupAlbumMedia 删除群相册媒体
//
// https://napcat.apifox.cn/395455119e0.md
func (ctx *Ctx) DeleteGroupAlbumMedia(groupID int64, albumID, mediaID string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("delete_group_album_media", Params{
		"group_id": groupID,
		"album_id": albumID,
		"media_id": mediaID,
	})
}

// LikeGroupAlbumMedia 点赞群相册媒体
//
// https://napcat.apifox.cn/395457331e0.md
func (ctx *Ctx) LikeGroupAlbumMedia(groupID int64, albumID, mediaID string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("like_group_album_media", Params{
		"group_id": groupID,
		"album_id": albumID,
		"media_id": mediaID,
	})
}

// ── 消息扩展（NapCat 扩展）──

// SendGroupMusic 发送群聊音乐卡片
//
// https://napcat.apifox.cn
//
// musicType: "qq", "163", "custom"
func (ctx *Ctx) SendGroupMusic(groupID int64, musicType string, id int64) (APIResult[int64], error) {
	dataRes, err := ctx.callActionData("send_group_msg", Params{
		"group_id": groupID,
		"message":  message.Message{message.Music(musicType, id)},
	})
	if err != nil {
		return APIResult[int64]{}, err
	}
	rsp := dataRes.Value.Get("message_id")
	if rsp.Exists() {
		return APIResult[int64]{Value: rsp.Int(), Resp: dataRes.Resp}, nil
	}
	return APIResult[int64]{}, errors.New("missing message_id")
}

// SendGroupCustomMusic 发送群聊自定义音乐卡片
func (ctx *Ctx) SendGroupCustomMusic(groupID int64, url, audio, title string) (APIResult[int64], error) {
	dataRes, err := ctx.callActionData("send_group_msg", Params{
		"group_id": groupID,
		"message":  message.Message{message.CustomMusic(url, audio, title)},
	})
	if err != nil {
		return APIResult[int64]{}, err
	}
	rsp := dataRes.Value.Get("message_id")
	if rsp.Exists() {
		return APIResult[int64]{Value: rsp.Int(), Resp: dataRes.Resp}, nil
	}
	return APIResult[int64]{}, errors.New("missing message_id")
}

// GetEmojiLikeList 获取消息表情点赞列表
//
// https://napcat.apifox.cn/410334663e0.md
func (ctx *Ctx) GetEmojiLikeList(messageID interface{}, emojiID string, count int) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_msg_emoji_like_list", Params{
		"message_id": messageID,
		"emoji_id":   emojiID,
		"count":      count,
	})
}

// GetMiniAppArk 获取小程序 Ark
//
// https://napcat.apifox.cn/227738594e0.md
func (ctx *Ctx) GetMiniAppArk(appID, title, desc, iconURL, webURL string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_mini_app_ark", Params{
		"app_id":   appID,
		"title":    title,
		"desc":     desc,
		"icon_url": iconURL,
		"web_url":  webURL,
	})
}

// ── 系统 / 安全（NapCat 扩展）──

// CheckURLSafely 检查URL安全性
//
// https://napcat.apifox.cn/228534361e0.md
//
// 返回安全等级: 1=安全, 2=未知, 3=危险
func (ctx *Ctx) CheckURLSafely(url string) (APIResult[int64], error) {
	dataRes, err := ctx.callActionData("check_url_safely", Params{
		"url": url,
	})
	if err != nil {
		return APIResult[int64]{}, err
	}
	return APIResult[int64]{Value: dataRes.Value.Get("level").Int(), Resp: dataRes.Resp}, nil
}

// CanSendImage 检查是否可以发送图片
//
// https://napcat.apifox.cn/226657071e0.md
func (ctx *Ctx) CanSendImage() (APIResult[bool], error) {
	dataRes, err := ctx.callActionData("can_send_image", Params{})
	if err != nil {
		return APIResult[bool]{}, err
	}
	return APIResult[bool]{Value: dataRes.Value.Get("yes").Bool(), Resp: dataRes.Resp}, nil
}

// CanSendRecord 检查是否可以发送语音
//
// https://napcat.apifox.cn/226657080e0.md
func (ctx *Ctx) CanSendRecord() (APIResult[bool], error) {
	dataRes, err := ctx.callActionData("can_send_record", Params{})
	if err != nil {
		return APIResult[bool]{}, err
	}
	return APIResult[bool]{Value: dataRes.Value.Get("yes").Bool(), Resp: dataRes.Resp}, nil
}

// GetCSRFToken 获取 CSRF Token
//
// https://napcat.apifox.cn/226657044e0.md
func (ctx *Ctx) GetCSRFToken() (APIResult[int64], error) {
	dataRes, err := ctx.callActionData("get_csrf_token", Params{})
	if err != nil {
		return APIResult[int64]{}, err
	}
	return APIResult[int64]{Value: dataRes.Value.Get("token").Int(), Resp: dataRes.Resp}, nil
}

// GetCredentials 获取登录凭证（Cookies + CSRF Token）
//
// https://napcat.apifox.cn/226657054e0.md
func (ctx *Ctx) GetCredentials(domain string) (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_credentials", Params{
		"domain": domain,
	})
}

// GetCookies 获取指定域名的 Cookies
//
// https://napcat.apifox.cn/226657041e0.md
func (ctx *Ctx) GetCookies(domain string) (APIResult[string], error) {
	dataRes, err := ctx.callActionData("get_cookies", Params{
		"domain": domain,
	})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.Get("cookies").Str, Resp: dataRes.Resp}, nil
}

// GetClientKey 获取当前登录帐号的 ClientKey
//
// https://napcat.apifox.cn/250286915e0.md
func (ctx *Ctx) GetClientKey() (APIResult[string], error) {
	dataRes, err := ctx.callActionData("get_clientkey", Params{})
	if err != nil {
		return APIResult[string]{}, err
	}
	return APIResult[string]{Value: dataRes.Value.Get("clientkey").Str, Resp: dataRes.Resp}, nil
}

// GetStatus 获取运行状态
//
// https://napcat.apifox.cn/226657083e0.md
func (ctx *Ctx) GetStatus() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_status", Params{})
}

// CleanCache 清理缓存
//
// https://napcat.apifox.cn/298305106e0.md
func (ctx *Ctx) CleanCache() (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("clean_cache", Params{})
}

// Restart 重启服务
//
// https://napcat.apifox.cn/410334662e0.md
func (ctx *Ctx) Restart() (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("set_restart", Params{})
}

// GetPacketStatus 获取Packet状态
//
// https://napcat.apifox.cn/226659280e0.md
func (ctx *Ctx) GetPacketStatus() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_packet_status", Params{})
}

// Logout 退出登录
//
// https://napcat.apifox.cn/283136399e0.md
func (ctx *Ctx) Logout() (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("nc_logout", Params{})
}

// ── 频道（NapCat 扩展）──

// GetGuildList 获取频道列表
//
// https://napcat.apifox.cn/226659311e0.md
func (ctx *Ctx) GetGuildList() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_guild_list", Params{})
}

// GetGuildServiceProfile 获取频道个人信息
//
// https://napcat.apifox.cn/226659317e0.md
func (ctx *Ctx) GetGuildServiceProfile() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_guild_service_profile", Params{})
}

// ── RKey（NapCat 扩展）──

// GetRKey 获取 RKey
//
// https://napcat.apifox.cn/226659297e0.md
func (ctx *Ctx) GetRKey() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_rkey", Params{})
}

// NcGetRKey 获取扩展RKey
//
// https://napcat.apifox.cn/283136230e0.md
func (ctx *Ctx) NcGetRKey() (APIResult[gjson.Result], error) {
	return ctx.callActionData("nc_get_rkey", Params{})
}

// GetRKeyServer 获取RKey服务器
//
// https://napcat.apifox.cn/283136236e0.md
func (ctx *Ctx) GetRKeyServer() (APIResult[gjson.Result], error) {
	return ctx.callActionData("get_rkey_server", Params{})
}

// ── 其他（NapCat 扩展）──

// ClickInlineKeyboardButton 点击内联键盘按钮
//
// https://napcat.apifox.cn/266151864e0.md
func (ctx *Ctx) ClickInlineKeyboardButton(groupID int64, botAppid string, buttonID, callbackData string) (APIResult[struct{}], error) {
	return ctx.callActionOnlyResult("click_inline_keyboard_button", Params{
		"group_id":      groupID,
		"bot_appid":     botAppid,
		"button_id":     buttonID,
		"callback_data": callbackData,
	})
}
