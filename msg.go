package ivy

import (
	"fmt"
	"strconv"
	"strings"
)

type MsgType int

const (
	BYE MsgType = iota
	ADD_REGEXP
	MSG
	ERROR
	DEL_REGEXP
	END_REGEXP
	START_REGEXP
	DIRECT_MSG
	DIE
	PING
	PONG
)

const (
	ARG_START = "\002"
	ARG_END   = "\003"
)

type MessageT struct {
	Type   MsgType
	NumId  int
	Params []string
}

func encogeIvyMsg(msg MessageT) []byte {
	encodedMsg := fmt.Sprintf("%d %d", msg.Type, msg.NumId) + ARG_START
	encodedMsg += strings.Join(msg.Params, ARG_END)
	if msg.Type == MSG {
		encodedMsg += ARG_END
	}

	return []byte(encodedMsg + "\n")
}

func decodeIvyMsg(msg string) (MessageT, error) {
	msgArgs := strings.SplitN(strings.Trim(msg, "\n"), " ", 2)
	if len(msgArgs) != 2 {
		return MessageT{}, fmt.Errorf("Wrong format msgArgs %d != 2", len(msgArgs))
	}

	msgId, err := strconv.Atoi(msgArgs[0])
	if err != nil {
		return MessageT{}, fmt.Errorf("Wrong msg type")
	}

	msgParams := strings.SplitN(msgArgs[1], ARG_START, 2)
	if len(msgParams) != 2 {
		return MessageT{}, fmt.Errorf("Wrong format msgParams %d != 2", len(msgParams))
	}

	numId, err := strconv.Atoi(msgParams[0])
	if err != nil {
		return MessageT{}, fmt.Errorf("Wrong msg numId")
	}

	params := strings.Split(msgParams[1], ARG_END)
	filteredParams := make([]string, 0)
	for _, p := range params {
		if p != "" {
			filteredParams = append(filteredParams, p)
		}
	}

	return MessageT{
		Type:   MsgType(msgId),
		NumId:  numId,
		Params: filteredParams,
	}, nil
}
