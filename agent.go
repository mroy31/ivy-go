package ivy

import (
	"bufio"
	"net"
	"regexp"
	"sync"

	"github.com/sirupsen/logrus"
)

type AgentState int

const (
	AGENT_NOT_INIT AgentState = iota
	AGENT_INIT_PROGRESS
	AGENT_INIT
)

type AgentT struct {
	ID              int
	AgentID         string
	Name            string
	SubcriptionLock *sync.Mutex
	Subscriptions   []SubscriptionT
	MsgCh           chan MessageT
	Conn            net.Conn
	State           AgentState
	Bus             *BusT
	Logger          *logrus.Entry
}

func (c *AgentT) Init() error {
	c.Bus.SubcriptionLock.Lock()
	defer c.Bus.SubcriptionLock.Unlock()

	if _, err := c.Conn.Write(encogeIvyMsg(MessageT{
		Type:   START_REGEXP,
		NumId:  c.Bus.GetBusPort(),
		Params: []string{c.Bus.Name},
	})); err != nil {
		return err
	}

	for _, subscription := range c.Bus.Subscriptions {
		if _, err := c.Conn.Write(encogeIvyMsg(MessageT{
			Type:   ADD_REGEXP,
			NumId:  subscription.Id,
			Params: []string{subscription.Text},
		})); err != nil {
			return err
		}
	}

	if _, err := c.Conn.Write(encogeIvyMsg(MessageT{
		Type:   END_REGEXP,
		NumId:  0,
		Params: []string{},
	})); err != nil {
		return err
	}

	go c.handleMsg()
	return nil
}

func (c *AgentT) SendMsg(msg string) {
	c.SubcriptionLock.Lock()
	defer c.SubcriptionLock.Unlock()

	for _, sub := range c.Subscriptions {
		if groups := sub.Re.FindStringSubmatch(msg); groups != nil {
			ivyMsg := MessageT{
				Type:  MSG,
				NumId: sub.Id,
			}
			if len(groups) > 1 {
				ivyMsg.Params = groups[1:]
			}
			c.MsgCh <- ivyMsg
		}
	}
}

func (c *AgentT) SendDirectMsg(numId int, msg string) {
	c.MsgCh <- MessageT{DIRECT_MSG, numId, []string{msg}}
}

func (c *AgentT) SendSubscription(id int, re string) {
	c.Logger.Debugf("Send subscription %d - %s", id, re)
	c.MsgCh <- MessageT{ADD_REGEXP, id, []string{re}}
}

func (c *AgentT) SendDelSubscription(id int) {
	c.Logger.Debugf("Send delete subscription %d", id)
	c.MsgCh <- MessageT{DEL_REGEXP, id, []string{}}
}

func (c *AgentT) addSubscription(subId int, subRegExp string) {
	c.SubcriptionLock.Lock()
	defer c.SubcriptionLock.Unlock()

	re, err := regexp.Compile(subRegExp)
	if err != nil {
		c.Logger.Errorf("Regexp %s not valid: %v", subRegExp, err)
		return
	}

	logrus.Debugf("Add regexp %d - %s", subId, subRegExp)
	c.Subscriptions = append(c.Subscriptions, SubscriptionT{
		Re:   re,
		Text: subRegExp,
		Id:   subId,
	})
}

func (c *AgentT) removeSubscription(subId int) {
	c.SubcriptionLock.Lock()
	defer c.SubcriptionLock.Unlock()

	found := -1
	for idx, sub := range c.Subscriptions {
		if sub.Id == subId {
			found = idx
			break
		}
	}

	if found != -1 {
		c.Subscriptions[found] = c.Subscriptions[len(c.Subscriptions)-1]
		c.Subscriptions = c.Subscriptions[:len(c.Subscriptions)-1]
	}
}

func (c *AgentT) handleMsg() {
	go func() {
		c.MsgCh = make(chan MessageT)
		for {
			msg, ok := <-c.MsgCh
			if !ok {
				// channel is close
				break
			}

			if _, err := c.Conn.Write(encogeIvyMsg(msg)); err != nil {
				c.Logger.Warnf("Unable to send msg: %v", err)
				continue
			}

			if msg.Type == BYE {
				// we want to leave, close the connection
				c.Conn.Close()
			}
		}
	}()

	buffer := bufio.NewReader(c.Conn)
	for {
		msg, err := buffer.ReadBytes('\n')
		if err != nil {
			// connection is close
			c.Bus.unregisterAgent(c)
			close(c.MsgCh)
			break
		}

		c.processMsg(string(msg))
	}
}

func (c *AgentT) processMsg(raw string) {
	logrus.Debugf("Receive raw msg: %s", raw)

	msg, err := decodeIvyMsg(raw)
	if err != nil {
		c.Logger.Errorf("Receive bad msg from client %s: %v", c.Name, err)
	}

	switch msg.Type {
	case START_REGEXP:
		c.State = AGENT_INIT_PROGRESS
		if len(msg.Params) > 0 {
			c.Name = msg.Params[0]
			c.Logger = c.Logger.WithField("name", c.Name)
		}
	case END_REGEXP:
		c.State = AGENT_INIT
		if c.Bus.OnCnxFunc != nil {
			c.Bus.OnCnxFunc(IvyApplication{c.ID, c.Name, c.AgentID})
		}
		if c.Bus.ReadyMsg != "" {
			c.SendMsg(c.Bus.ReadyMsg)
		}
	case ADD_REGEXP:
		if len(msg.Params) == 0 {
			c.Logger.Errorf("Receive ADD_REGEXP message without rexgexp param")
			return
		}
		c.addSubscription(msg.NumId, msg.Params[0])
	case DEL_REGEXP:
		c.removeSubscription(msg.NumId)
	case DIRECT_MSG:
		if len(msg.Params) == 0 {
			c.Logger.Errorf("Receive DIRECT_MSG without msg data, ignore it")
			return
		}
		if c.Bus.DirectMsgCb != nil {
			c.Bus.DirectMsgCb(
				IvyApplication{c.ID, c.Name, c.AgentID},
				msg.NumId, msg.Params[0])
		}
	case MSG:
		c.Bus.SubcriptionLock.Lock()
		defer c.Bus.SubcriptionLock.Unlock()

		for _, sub := range c.Bus.Subscriptions {
			if sub.Id == msg.NumId {
				sub.Cb(IvyApplication{c.ID, c.Name, c.AgentID}, msg.Params)
			}
		}
	case BYE:
		c.Logger.Debugf("Receive BYE message")
	}
}

func (c *AgentT) Close() {
	// send BYE message
	c.MsgCh <- MessageT{BYE, 0, []string{}}
}
