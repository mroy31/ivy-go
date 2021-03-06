package ivy

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"
)

type IvyAppCallback func(agent IvyApplication)
type IvyBindCallback func(agent IvyApplication, params []string)
type IvyBindDirectCallback func(agent IvyApplication, numId int, msg string)
type IvyApplication struct {
	ID      int
	Name    string
	AgentId string
}
type IvySubscription struct {
	ID     int
	Regexp string
}

type BusState int

const (
	CLOSE BusState = iota
	INIT
	STARTED
)

const (
	PROTOCOL_VERSION = 3
)

var (
	state BusState = CLOSE
	bus   *BusT
	log   = logrus.New()
)

func GetLogger() *logrus.Logger {
	return log
}

func SetLogger(w io.Writer, level logrus.Level) {
	log.SetFormatter(&logrus.TextFormatter{})
	log.SetOutput(w)
	log.SetLevel(level)
}

func IvyInit(agentName, readyMsg string, mainLoop int, onCnxFunc, onDieFunc IvyAppCallback) error {
	if state != CLOSE {
		return fmt.Errorf("The bus is already initialized")
	}

	bus = &BusT{
		Name:            agentName,
		ReadyMsg:        readyMsg,
		OnCnxFunc:       onCnxFunc,
		OnDieFunc:       onDieFunc,
		SubcriptionLock: &sync.Mutex{},
		Subscriptions:   make([]SubscriptionT, 0),
		Logger:          log.WithField("ivy", agentName),
	}
	state = INIT
	return nil
}

func IvyStart(busId string) error {
	switch state {
	case CLOSE:
		return fmt.Errorf("The bus is not initialized, call IvyInit first")
	case STARTED:
		return fmt.Errorf("The bus is already started")
	case INIT:
		err := bus.Start(busId)
		state = STARTED
		return err
	}
	return fmt.Errorf("Unknown state")
}

func IvyStop() error {
	if state != STARTED {
		return fmt.Errorf("The bus is not running, did you call IvyStart ?")
	}

	state = INIT
	return bus.Stop()
}

func IvyBindMsg(cb IvyBindCallback, re string) (int, error) {
	if state == CLOSE {
		return -1, fmt.Errorf("The bus is not initialized, call IvyInit first")
	}

	return bus.BindMsg(re, cb)
}

func IvyUnbindMsg(subId int) (string, error) {
	if state == CLOSE {
		return "", fmt.Errorf("The bus is not initialized, call IvyInit first")
	}

	return bus.UnbindMsg(subId)
}

func IvySendMsg(msg string) error {
	if state != STARTED {
		return fmt.Errorf("The bus is not running, did you call IvyStart ?")
	}

	bus.SendMessage(msg)
	return nil
}

func IvyBindDirectMsg(cb IvyBindDirectCallback) error {
	if state == CLOSE {
		return fmt.Errorf("The bus is not initialized, call IvyInit first")
	}

	bus.DirectMsgCb = cb
	return nil
}

func IvySendDirectMsg(agentId int, numId int, msg string) error {
	if state != STARTED {
		return fmt.Errorf("The bus is not running, did you call IvyStart ?")
	}

	agent := bus.GetAgent(agentId)
	if agent == nil {
		return fmt.Errorf("Agent with id %d not found", agentId)
	}

	agent.SendDirectMsg(numId, msg)
	return nil
}

func IvyGetApplicationByName(appName string) (IvyApplication, error) {
	if state == CLOSE {
		return IvyApplication{}, fmt.Errorf("The bus is not initialized, call IvyInit first")
	}

	for _, agent := range bus.Agents {
		if agent.Name == appName {
			return IvyApplication{agent.ID, agent.Name, agent.AgentID}, nil
		}
	}

	return IvyApplication{}, fmt.Errorf("Application %s not found", appName)
}

func IvyGetApplicationList() ([]IvyApplication, error) {
	list := make([]IvyApplication, len(bus.Agents))
	if state == CLOSE {
		return list, fmt.Errorf("The bus is not initialized, call IvyInit first")
	}

	for idx, agent := range bus.Agents {
		list[idx] = IvyApplication{
			ID:      agent.ID,
			AgentId: agent.AgentID,
			Name:    agent.Name,
		}
	}

	return list, nil
}

func IvyGetApplicationMessages(agentId int) ([]IvySubscription, error) {
	list := make([]IvySubscription, 0)
	if state == CLOSE {
		return list, fmt.Errorf("The bus is not initialized, call IvyInit first")
	}

	agent := bus.GetAgent(agentId)
	if agent == nil {
		return list, fmt.Errorf("Agent with id %d not found", agentId)
	}

	agent.SubcriptionLock.Lock()
	defer agent.SubcriptionLock.Unlock()

	for _, sub := range agent.Subscriptions {
		list = append(list, IvySubscription{sub.Id, sub.Text})
	}

	return list, nil
}

func IvyMainLoop() error {
	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)

	<-interrupt
	logrus.Warn("Close Ivy bus")
	return IvyStop()
}
