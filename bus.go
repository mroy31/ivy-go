package ivy

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

var (
	subId           = 0
	agentInternalId = 0
)

type SubscriptionT struct {
	Id   int
	Text string
	Re   *regexp.Regexp
	Cb   IvyBindCallback
}

type BusT struct {
	ID              string
	Name            string
	ReadyMsg        string
	OnCnxFunc       IvyAppCallback
	OnDieFunc       IvyAppCallback
	DirectMsgCb     IvyBindDirectCallback
	Conn            net.Listener
	WaitCh          chan error
	UDPConn         *net.UDPConn
	UDPWaitCh       chan error
	Agents          []*AgentT
	SubcriptionLock *sync.Mutex
	Subscriptions   []SubscriptionT
	Logger          *logrus.Entry
}

func (b *BusT) Start(busId string) error {
	if err := b.serve(); err != nil {
		return err
	}

	if err := b.serveUDPDiscovery(busId, b.GetBusPort()); err != nil {
		return err
	}

	return nil
}

func (b *BusT) Stop() error {
	for _, agent := range b.Agents {
		agent.Close()
	}
	b.Agents = make([]*AgentT, 0)

	if b.Conn != nil {
		b.Conn.Close()
	}
	if b.UDPConn != nil {
		b.UDPConn.Close()
	}

	return nil
}

func (b *BusT) SendMessage(msg string) {
	for _, agent := range b.Agents {
		agent.SendMsg(msg)
	}
}

func (b *BusT) BindMsg(re string, cb IvyBindCallback) (int, error) {
	b.SubcriptionLock.Lock()
	defer b.SubcriptionLock.Unlock()

	reObj, err := regexp.Compile(re)
	if err != nil {
		return -1, fmt.Errorf("Bad formated regexp: %w", err)
	}

	subId++
	b.Subscriptions = append(b.Subscriptions, SubscriptionT{
		Id:   subId,
		Text: re,
		Re:   reObj,
		Cb:   cb,
	})

	for _, agent := range b.Agents {
		agent.SendSubscription(subId, re)
	}

	return subId, nil
}

func (b *BusT) UnbindMsg(subId int) (string, error) {
	b.SubcriptionLock.Lock()
	defer b.SubcriptionLock.Unlock()

	found := -1
	for idx, sub := range b.Subscriptions {
		if sub.Id == subId {
			found = idx
			break
		}
	}

	if found == -1 {
		return "", fmt.Errorf("Subscription %d nod found", subId)
	}

	b.Subscriptions[found] = b.Subscriptions[len(b.Subscriptions)-1]
	b.Subscriptions = b.Subscriptions[:len(b.Subscriptions)-1]

	for _, agent := range b.Agents {
		agent.SendDelSubscription(subId)
	}

	return "", nil
}

func (b *BusT) GetBusPort() int {
	if b.Conn != nil {
		return b.Conn.Addr().(*net.TCPAddr).Port
	}
	return -1
}

func (b *BusT) serve() error {
	b.Logger.Debugf("Start Bus in :0")

	var err error
	b.Conn, err = net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("Unable to start bus: %v", err)
	}

	// construct agent ID
	port := b.GetBusPort()
	now := time.Now()
	b.ID = strings.ReplaceAll(b.Name, " ", "") + now.Format("20060102150405") + fmt.Sprintf("%05d%d", rand.Intn(99999), port)

	go func() {
		// accept connection
		for {
			conn, err := b.Conn.Accept()
			if err != nil {
				if neterr, ok := err.(net.Error); ok {
					if !neterr.Temporary() {
						break
					}
				}
				return
			}

			b.Logger.Debugf("Accept connection from %s", conn.RemoteAddr().String())
			if err := b.registerAgent("", "", conn); err != nil {
				b.Logger.Errorf("Unable to register new agent: %v", err)
				continue
			}
		}
	}()

	return nil
}

func (b *BusT) serveUDPDiscovery(busId string, tcpPort int) error {
	b.Logger.Debugf("UDP: Start Server in %s", busId)
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if opErr != nil && runtime.GOOS == "linux" {
					opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
				}
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}

	lp, err := lc.ListenPacket(context.Background(), "udp", busId)
	if err != nil {
		return fmt.Errorf("Unable to listen on %s: %w", busId, err)
	}
	b.UDPConn = lp.(*net.UDPConn)

	go func() {
		buffer := make([]byte, 1024)
		for {
			n, addr, err := b.UDPConn.ReadFromUDP(buffer)
			if err != nil {
				b.UDPWaitCh <- err
				return
			}

			b.Logger.Debugf("UDP: receive msg %s from %s", string(buffer[:n]), addr.IP)
			agentInfos := strings.Split(string(buffer[:n]), " ")
			if len(agentInfos) < 4 {
				b.Logger.Errorf("Wrong UDP message receive from %s", addr.IP)
				continue
			}

			protocolVersion, err := strconv.Atoi(agentInfos[0])
			if err != nil {
				b.Logger.Errorf("UDP: receive wrong protocol version from %s", addr.IP)
				continue
			}
			port, err := strconv.Atoi(agentInfos[1])
			if err != nil {
				b.Logger.Errorf("UDP: receive wrong port number from %s", addr.IP)
				continue
			}
			appId := agentInfos[2]
			appName := strings.Join(agentInfos[3:], " ")

			if protocolVersion != PROTOCOL_VERSION {
				b.Logger.Errorf(
					"UDP: Received a broadcast msg. w/ protocol version:%d , expected: %d",
					protocolVersion, PROTOCOL_VERSION)
				continue
			}

			if err := b.ConnectToClient(addr.IP.String(), port, appName, appId); err != nil {
				b.Logger.Errorf("UDP: unable to connect to client %s: %v", addr.IP, err)
				continue
			}
		}
	}()

	// send broadcast message to other agents
	listenAddr, err := net.ResolveUDPAddr("udp", busId)
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("%d %d %s %s\n", PROTOCOL_VERSION, tcpPort, b.ID, b.Name)
	if _, err := b.UDPConn.WriteTo([]byte(msg), listenAddr); err != nil {
		return err
	}

	return nil
}

func (b *BusT) ConnectToClient(addr string, port int, appName, appId string) error {
	if appId == b.ID { // this is us
		return nil
	}

	if b.isAgentExist(appId) {
		b.Logger.Warnf(
			"Strange, we receive a message from an already known client: %s-%s",
			appId, appName)
		return nil
	}

	b.Logger.Debugf("TCP: initiate connection to client %s", appName)
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		return err
	}

	return b.registerAgent(appId, appName, conn)
}

func (b *BusT) registerAgent(appId, appName string, conn net.Conn) error {
	agentInternalId++
	agent := &AgentT{
		ID:              agentInternalId,
		AgentID:         appId,
		Name:            appName,
		Conn:            conn,
		SubcriptionLock: &sync.Mutex{},
		Subscriptions:   make([]SubscriptionT, 0),
		Bus:             b,
		State:           AGENT_NOT_INIT,
		Logger:          logrus.WithField("name", appName),
	}

	if err := agent.Init(); err != nil {
		conn.Close()
		return err
	}

	b.Agents = append(b.Agents, agent)

	return nil
}

func (b *BusT) unregisterAgent(client *AgentT) {
	found := -1
	for idx, a := range b.Agents {
		if a == client {
			found = idx
			break
		}
	}

	if found != -1 {
		b.Agents[found] = b.Agents[len(b.Agents)-1]
		b.Agents = b.Agents[:len(b.Agents)-1]

		if b.OnDieFunc != nil {
			b.OnDieFunc(IvyApplication{client.ID, client.Name, client.AgentID})
		}
	}
}

func (b *BusT) GetAgent(agentId int) *AgentT {
	for _, agent := range bus.Agents {
		if agent.ID == agentId {
			return agent
		}
	}

	return nil
}

func (b *BusT) isAgentExist(appId string) bool {
	for _, client := range b.Agents {
		if client.AgentID == appId {
			return true
		}
	}

	return false
}
